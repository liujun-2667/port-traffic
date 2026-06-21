package api

import (
	"encoding/json"
	"sync"
	"time"

	"port-traffic/internal/config"
	"port-traffic/internal/sim"
	"port-traffic/internal/store"
)

// Manager owns live simulation runs and streams frames to subscribers.
type Manager struct {
	mu    sync.Mutex
	runs  map[int64]*runState
	cfg   *config.Service
	store store.Store
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
	return &Manager{runs: map[int64]*runState{}, cfg: cfg, store: st}
}

// StartRun creates a run, persists it (if store available) and starts stepping.
func (m *Manager) StartRun(p sim.Params) (int64, error) {
	cfg := m.cfg.Get()
	engine := sim.NewEngine(cfg, p)

	var id int64 = time.Now().UnixNano()
	paramsJSON, _ := json.Marshal(p)
	if m.store != nil {
		rid, err := m.store.SaveRun(paramsJSON, cfg.Sim.DurationHours*60)
		if err == nil {
			id = rid
		}
	}
	r := &runState{
		id: id, engine: engine, params: p,
		rate: 1, cancel: make(chan struct{}),
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
	default:
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
		if rate > 0 {
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
