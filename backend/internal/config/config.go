// Package config loads and hot-reloads the port simulation configuration.
package config

import (
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"

	"port-traffic/internal/model"
)

// SimConfig holds simulation-wide tunables.
type SimConfig struct {
	DurationHours        int     `yaml:"durationHours" json:"durationHours"`
	ArrivalRate          float64 `yaml:"arrivalRate" json:"arrivalRate"`
	TimeStepMinutes      int     `yaml:"timeStepMinutes" json:"timeStepMinutes"`
	SpeedFactor          float64 `yaml:"speedFactor" json:"speedFactor"`
	Seed                 int64   `yaml:"seed" json:"seed"`
	SafeSpacingShips     float64 `yaml:"safeSpacingShips" json:"safeSpacingShips"`
	EncounterSafeRatio   float64 `yaml:"encounterSafeRatio" json:"encounterSafeRatio"`
}

// TideComponent is a single harmonic constituent.
type TideComponent struct {
	Name      string  `yaml:"name" json:"name"`
	Amplitude float64 `yaml:"amplitude" json:"amplitude"` // meters
	Phase     float64 `yaml:"phase" json:"phase"`         // radians
	Speed     float64 `yaml:"speed" json:"speed"`          // radians/hour
}

// TideConfig drives the harmonic tide model.
type TideConfig struct {
	Datum         float64         `yaml:"datum" json:"datum"`
	MeanSeaLevel  float64         `yaml:"meanSeaLevel" json:"meanSeaLevel"`
	Components    []TideComponent `yaml:"components" json:"components"`
	DraftMargin   float64         `yaml:"draftMargin" json:"draftMargin"`
}

// DraftEntry maps a length range to typical draft/beam ratio/dwt.
type DraftEntry struct {
	LenMin    float64 `yaml:"lenMin" json:"lenMin"`
	LenMax    float64 `yaml:"lenMax" json:"lenMax"`
	Draft     float64 `yaml:"draft" json:"draft"`
	BeamRatio float64 `yaml:"beamRatio" json:"beamRatio"`
	DWT       float64 `yaml:"dwt" json:"dwt"`
}

// ManeuverEntry holds per-type maneuvering parameters.
type ManeuverEntry struct {
	TurningRadius float64 `yaml:"turningRadius" json:"turningRadius"`
	StopDistance  float64 `yaml:"stopDistance" json:"stopDistance"`
	AccelRate     float64 `yaml:"accelRate" json:"accelRate"`
	DecelRate     float64 `yaml:"decelRate" json:"decelRate"`
}

// TrafficConfig drives the Poisson ship generator.
type TrafficConfig struct {
	TypeWeights        map[string]float64   `yaml:"typeWeights" json:"typeWeights"`
	LengthMin          float64             `yaml:"lengthMin" json:"lengthMin"`
	LengthMax          float64             `yaml:"lengthMax" json:"lengthMax"`
	DraftTable         []DraftEntry        `yaml:"draftTable" json:"draftTable"`
	Maneuver           map[string]ManeuverEntry `yaml:"maneuver" json:"maneuver"`
	WorkDurationMinutes map[string]int      `yaml:"workDurationMinutes" json:"workDurationMinutes"`
	WorkJitter         float64             `yaml:"workJitter" json:"workJitter"`
}

// WeatherConfig captures environmental conditions.
type WeatherConfig struct {
	WindSpeed  float64 `yaml:"windSpeed" json:"windSpeed"`
	Visibility float64 `yaml:"visibility" json:"visibility"`
	Swell      float64 `yaml:"swell" json:"swell"`
}

// Config is the full application configuration.
type Config struct {
	Sim     SimConfig     `yaml:"sim" json:"sim"`
	Tide    TideConfig    `yaml:"tide" json:"tide"`
	Traffic TrafficConfig `yaml:"traffic" json:"traffic"`
	Weather WeatherConfig `yaml:"weather" json:"weather"`
	Port    model.PortModel `yaml:"port" json:"port"`
}

// Service manages the live configuration with hot reload.
type Service struct {
	mu      sync.RWMutex
	cfg     *Config
	path    string
	subs    []chan *Config
	stop    chan struct{}
}

// Load reads the YAML file and starts a hot-reload watcher.
func Load(path string) (*Service, error) {
	cfg, err := read(path)
	if err != nil {
		return nil, err
	}
	s := &Service{cfg: cfg, path: path, stop: make(chan struct{})}
	if err := s.watch(); err != nil {
		log.Printf("config: hot-reload disabled: %v", err)
	}
	return s, nil
}

func read(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := &Config{}
	if err := yaml.Unmarshal(data, c); err != nil {
		return nil, err
	}
	c.applyDefaults()
	return c, nil
}

func (c *Config) applyDefaults() {
	if c.Sim.TimeStepMinutes <= 0 {
		c.Sim.TimeStepMinutes = 1
	}
	if c.Sim.DurationHours <= 0 {
		c.Sim.DurationHours = 24
	}
	if c.Sim.ArrivalRate <= 0 {
		c.Sim.ArrivalRate = 3
	}
	if c.Sim.SafeSpacingShips <= 0 {
		c.Sim.SafeSpacingShips = 3
	}
	if c.Sim.EncounterSafeRatio <= 0 {
		c.Sim.EncounterSafeRatio = 2
	}
	if c.Tide.DraftMargin <= 0 {
		c.Tide.DraftMargin = 1.2
	}
}

// Get returns a deep copy of the current configuration.
func (s *Service) Get() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Clone()
}

// Reload forces a re-read from disk (used by tests and explicit reload).
func (s *Service) Reload() error {
	cfg, err := read(s.path)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg = cfg
	subs := append([]chan *Config(nil), s.subs...)
	s.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- cfg.Clone():
		default:
		}
	}
	log.Printf("config: reloaded from %s", s.path)
	return nil
}

// Update applies API-driven hot updates to sim and/or weather parameters.
// Nil arguments leave the corresponding section unchanged.
func (s *Service) Update(sim *SimConfig, weather *WeatherConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sim != nil {
		s.cfg.Sim = *sim
	}
	if weather != nil {
		s.cfg.Weather = *weather
	}
}

// Subscribe returns a channel that receives a new config copy on each reload.
func (s *Service) Subscribe() chan *Config {
	ch := make(chan *Config, 1)
	s.mu.Lock()
	s.subs = append(s.subs, ch)
	s.mu.Unlock()
	return ch
}

func (s *Service) watch() error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := w.Add(dir); err != nil {
		return err
	}
	go func() {
		defer w.Close()
		for {
			select {
			case <-s.stop:
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if ev.Has(fsnotify.Write|fsnotify.Create) && filepath.Clean(ev.Name) == filepath.Clean(s.path) {
					if err := s.Reload(); err != nil {
						log.Printf("config: reload error: %v", err)
					}
				}
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				log.Printf("config: watcher error: %v", err)
			}
		}
	}()
	return nil
}

// Close stops the hot-reload watcher.
func (s *Service) Close() {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
}

// Clone returns a deep copy of the configuration.
func (c *Config) Clone() *Config {
	if c == nil {
		return nil
	}
	out, err := yaml.Marshal(c)
	if err != nil {
		return c // best effort
	}
	clone := &Config{}
	if err := yaml.Unmarshal(out, clone); err != nil {
		return c
	}
	return clone
}
