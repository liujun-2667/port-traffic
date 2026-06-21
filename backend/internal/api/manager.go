package api

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"port-traffic/internal/config"
	"port-traffic/internal/sim"
	"port-traffic/internal/store"
)

// Manager owns live simulation runs and streams frames to subscribers.
type Manager struct {
	mu     sync.Mutex
	runs   map[int64]*runState
	cfg    *config.Service
	store  store.Store
	nextID atomic.Int64
}

type runState struct {
	id     int64
	engine *sim.Engine
	params sim.Params

	mu     sync.Mutex
	rate   int
	paused bool
	done   bool
	cancel chan struct{}
	subs   []chan sim.Frame
}

// NewManager creates a Manager.
func NewManager(cfg *config.Service, st store.Store) *Manager {
	m := &Manager{runs: map[int64]*runState{}, cfg: cfg, store: st}
	// Seed the counter with a value that won't collide with DB BIGSERIAL (starts at 1).
	// Use time offset + monotonic counter; keep values well below 2^53 for JS Number safety.
	m.nextID.Store(1_000_000_000 + time.Now().Unix()%1_000_000_000)
	return m
}

// StartRun creates a run, persists it (if store available) and starts stepping.
func (m *Manager) StartRun(p sim.Params) (int64, error) {
	cfg := m.cfg.Get()
	engine := sim.NewEngine(cfg, p)

	paramsJSON, _ := json.Marshal(p)
	id := m.nextID.Add(1) // JS-safe: well below 2^53
	if m.store != nil {
		if rid, err := m.store.SaveRun(paramsJSON, cfg.Sim.DurationHours*60); err == nil && rid > 0 {
			id = rid
		}
	}
	r := &runState{
		id: id, engine: engine, params: p,
		rate: 1, cancel: make(chan struct{}),
	}
	// Translate float speedFactor from Params to the discrete playback rates.
	switch {
	case p.SpeedFactor <= 0:
		r.rate = 0 // "fastest"
	case p.SpeedFactor >= 20:
		r.rate = 20
	case p.SpeedFactor >= 5:
		r.rate = 5
	default:
		r.rate = int(p.SpeedFactor + 0.5)
		if r.rate < 1 {
			r.rate = 1
		}
	}
	m.mu.Lock()
	m.runs[id] = r
	m.mu.Unlock()
	go m.loop(r)
	return id, nil
}

func (m *Manager) loop(r *runState) {
	for {
		select {
		case <-r.cancel:
			return
		default:
		}
		r.mu.Lock()
		paused := r.paused
		rate := r.rate
		done := r.done
		r.mu.Unlock()
		if done {
			return
		}
		if paused {
			time.Sleep(120 * time.Millisecond)
			continue
		}
		more := r.engine.Step()
		r.broadcast(r.engine.Frame())
		if !more {
			r.mu.Lock()
			r.done = true
			r.mu.Unlock()
			m.finalize(r)
			return
		}
		time.Sleep(stepDelay(rate))
	}
}

func stepDelay(rate int) time.Duration {
	switch rate {
	case 1:
		return 700 * time.Millisecond
	case 5:
		return 140 * time.Millisecond
	case 20:
		return 35 * time.Millisecond
	default: // 0 or anything else -> fastest
		return 2 * time.Millisecond
	}
}

func (r *runState) broadcast(f sim.Frame) {
	r.mu.Lock()
	subs := append([]chan sim.Frame(nil), r.subs...)
	r.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- f:
		default:
		}
	}
}

// Subscribe returns a channel receiving frames; the current frame is sent first.
func (m *Manager) Subscribe(id int64) (<-chan sim.Frame, bool) {
	m.mu.Lock()
	r, ok := m.runs[id]
	m.mu.Unlock()
	if !ok {
		return nil, false
	}
	ch := make(chan sim.Frame, 8)
	r.mu.Lock()
	r.subs = append(r.subs, ch)
	cur := r.engine.Frame()
	r.mu.Unlock()
	go func() { ch <- cur }()
	return ch, true
}

// Unsubscribe removes a subscriber channel.
func (m *Manager) Unsubscribe(id int64, ch <-chan sim.Frame) {
	m.mu.Lock()
	r, ok := m.runs[id]
	m.mu.Unlock()
	if !ok {
		return
	}
	r.mu.Lock()
	out := r.subs[:0]
	for _, c := range r.subs {
		if c != ch {
			out = append(out, c)
		}
	}
	r.subs = out
	r.mu.Unlock()
}

// State returns the latest frame for a run.
func (m *Manager) State(id int64) (sim.Frame, bool) {
	m.mu.Lock()
	r, ok := m.runs[id]
	m.mu.Unlock()
	if !ok {
		return sim.Frame{}, false
	}
	return r.engine.Frame(), true
}

// Control changes a run's playback.
func (m *Manager) Control(id int64, action string, rate int) bool {
	m.mu.Lock()
	r, ok := m.runs[id]
	m.mu.Unlock()
	if !ok {
		return false
	}
	switch action {
	case "pause":
		r.mu.Lock()
		r.paused = true
		r.mu.Unlock()
	case "resume":
		r.mu.Lock()
		r.paused = false
		r.mu.Unlock()
	case "set_rate":
		r.mu.Lock()
		if rate <= 0 {
			r.rate = 0 // 0 means "fastest"
		} else {
			r.rate = rate
		}
		r.mu.Unlock()
	case "reset":
		m.reset(r)
	}
	return true
}

func (m *Manager) reset(r *runState) {
	r.mu.Lock()
	if r.done {
		r.mu.Unlock()
		return
	}
	close(r.cancel)
	r.paused = false
	r.mu.Unlock()
	cfg := m.cfg.Get()
	engine := sim.NewEngine(cfg, r.params)
	newR := &runState{
		id: r.id, engine: engine, params: r.params,
		rate: 1, cancel: make(chan struct{}),
	}
	m.mu.Lock()
	m.runs[r.id] = newR
	m.mu.Unlock()
	go m.loop(newR)
}

func (m *Manager) finalize(r *runState) {
	if m.store == nil {
		return
	}
	_ = m.store.UpdateStatus(r.id, "completed")
	tr := r.engine.Trajectory()
	if err := m.store.SaveTrajectory(r.id, tr); err != nil {
		return
	}
	rep := r.engine.Report()
	_ = m.store.SaveReport(r.id, rep)
}

// HasRun reports whether a run is in memory.
func (m *Manager) HasRun(id int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.runs[id]
	return ok
}

// Report returns the engine report for a live run.
func (m *Manager) Report(id int64) (sim.Report, bool) {
	m.mu.Lock()
	r, ok := m.runs[id]
	m.mu.Unlock()
	if !ok {
		return sim.Report{}, false
	}
	return r.engine.Report(), true
}

// ShipDetail returns the state history and dangerous encounters for a specific ship.
func (m *Manager) ShipDetail(id int64, shipID string) (sim.ShipDetail, bool) {
	m.mu.Lock()
	r, ok := m.runs[id]
	m.mu.Unlock()
	if !ok {
		return sim.ShipDetail{}, false
	}
	return sim.ShipDetail{
		ShipID:              shipID,
		StateHistory:        r.engine.GetStateHistory(shipID),
		DangerousEncounters: r.engine.GetShipDangerousEncounters(shipID),
	}, true
}
