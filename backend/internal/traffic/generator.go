// Package traffic generates ship arrival events via a Poisson process.
package traffic

import (
	"fmt"
	"math"
	"math/rand"

	"port-traffic/internal/config"
	"port-traffic/internal/model"
)

// Arrival is a scheduled ship arrival at a simulation minute.
type Arrival struct {
	Minute int          `json:"minute"`
	Ship   *model.Ship  `json:"ship"`
}

// Generator builds ship arrivals from configuration.
type Generator struct {
	cfg    config.TrafficConfig
	sim    config.SimConfig
	port   *model.PortModel
	rnd    *rand.Rand
	counter int
}

// New creates a generator.
func New(tc config.TrafficConfig, sc config.SimConfig, port *model.PortModel, seed int64) *Generator {
	return &Generator{cfg: tc, sim: sc, port: port, rnd: rand.New(rand.NewSource(seed))}
}

// Arrivals produces Poisson-distributed arrivals over durationMinutes.
func (g *Generator) Arrivals(durationMinutes int) []Arrival {
	out := make([]Arrival, 0, 64)
	lambda := g.sim.ArrivalRate / 60.0 // arrivals per minute
	if lambda <= 0 {
		return out
	}
	t := 0.0
	for {
		u := g.rnd.Float64()
		if u <= 1e-9 {
			u = 1e-9
		}
		dt := -math.Log(u) / lambda
		t += dt
		minute := int(math.Round(t))
		if minute >= durationMinutes {
			break
		}
		out = append(out, Arrival{Minute: minute, Ship: g.newShip(minute)})
	}
	return out
}

func (g *Generator) newShip(minute int) *model.Ship {
	stype := g.pickType()
	length := g.sampleLength()
	entry := g.lookupDraft(length)
	beam := length * entry.BeamRatio
	limit := g.approachLimit()
	man := g.cfg.Maneuver[string(stype)]

	s := &model.Ship{
		ID:            fmt.Sprintf("V-%04d", g.counter+1),
		Type:          stype,
		Length:        length,
		Beam:          beam,
		Draft:         entry.Draft,
		DWT:           entry.DWT,
		SpeedKn:       0,
		PlannedSpeed:  clampSpeed(limit + g.jitter(1.0)),
		Maneuver: model.Maneuverability{
			TurningRadius: man.TurningRadius,
			StopDistance:  man.StopDistance,
			AccelRate:     man.AccelRate,
			DecelRate:     man.DecelRate,
		},
		State:         model.StateArrived,
		ArrivalMinute: minute,
		Direction:     1, // inbound traverses each segment From->To
	}
	g.counter++
	return s
}

func (g *Generator) pickType() model.ShipType {
	r := g.rnd.Float64()
	cum := 0.0
	for name, w := range g.cfg.TypeWeights {
		cum += w
		if r <= cum {
			return model.ShipType(name)
		}
	}
	return model.TypeOther
}

// sampleLength draws from a mildly right-skewed distribution in [min,max].
func (g *Generator) sampleLength() float64 {
	lo := g.cfg.LengthMin
	hi := g.cfg.LengthMax
	// bias toward smaller vessels using a sqrt transform
	u := g.rnd.Float64()
	v := 1 - math.Sqrt(u)
	return lo + (hi-lo)*v
}

func (g *Generator) lookupDraft(length float64) config.DraftEntry {
	for _, e := range g.cfg.DraftTable {
		if length >= e.LenMin && length < e.LenMax {
			return e
		}
	}
	last := g.cfg.DraftTable[len(g.cfg.DraftTable)-1]
	return last
}

func (g *Generator) approachLimit() float64 {
	if len(g.port.Segments) > 0 {
		return g.port.Segments[0].SpeedLimit
	}
	return 12
}

func (g *Generator) jitter(amp float64) float64 {
	return (g.rnd.Float64()*2 - 1) * amp
}

func clampSpeed(v float64) float64 {
	if v < 3 {
		return 3
	}
	if v > 25 {
		return 25
	}
	return v
}
