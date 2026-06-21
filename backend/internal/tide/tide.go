// Package tide implements a simplified harmonic tide model (M2/S2/K1).
package tide

import (
	"math"

	"port-traffic/internal/config"
)

// Model computes tide height from harmonic constituents.
type Model struct {
	cfg config.TideConfig
}

// New creates a tide model from configuration.
func New(cfg config.TideConfig) *Model { return &Model{cfg: cfg} }

// Level returns the tide height (meters above datum) at time tHours.
// h(t) = MSL + Σ H_i · cos(σ_i·t + φ_i)
func (m *Model) Level(tHours float64) float64 {
	h := m.cfg.MeanSeaLevel + m.cfg.Datum
	for _, c := range m.cfg.Components {
		h += c.Amplitude * math.Cos(c.Speed*tHours+c.Phase)
	}
	return h
}

// NavigableDepth returns the actual water depth for a segment base depth at t.
func (m *Model) NavigableDepth(baseDepth, tHours float64) float64 {
	return baseDepth + m.Level(tHours)
}

// CanNavigate reports whether a vessel of the given draft may transit a
// segment with baseDepth at time t: depth must be >= margin * draft.
func (m *Model) CanNavigate(baseDepth, draft, tHours float64) bool {
	return m.NavigableDepth(baseDepth, tHours) >= m.cfg.DraftMargin*draft
}

// Margin returns the configured draft safety margin multiplier.
func (m *Model) Margin() float64 { return m.cfg.DraftMargin }

// Series samples the tide level over n points across hoursTotal.
func (m *Model) Series(hoursTotal float64, n int) []Point {
	if n < 2 {
		n = 2
	}
	pts := make([]Point, n)
	for i := 0; i < n; i++ {
		t := hoursTotal * float64(i) / float64(n-1)
		pts[i] = Point{T: t, H: m.Level(t)}
	}
	return pts
}

// Point is a tide sample.
type Point struct {
	T float64 `json:"t"` // hours
	H float64 `json:"h"` // meters
}
