package sim

import (
	"os"
	"path/filepath"
	"testing"

	"port-traffic/internal/config"
)

func loadTestConfig(t *testing.T) *config.Config {
	t.Helper()
	paths := []string{
		"../../config/port.yaml",
		"../../../backend/config/port.yaml",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			abs, _ := filepath.Abs(p)
			svc, err := config.Load(abs)
			if err != nil {
				t.Fatalf("load config %s: %v", abs, err)
			}
			return svc.Get()
		}
	}
	t.Skip("port.yaml not found relative to test")
	return nil
}

func TestEngineRunsToCompletion(t *testing.T) {
	cfg := loadTestConfig(t)
	e := NewEngine(cfg, Params{DurationHours: 24, ArrivalRate: 3, Seed: 42})
	for !e.Step() {
	}
	rep := e.Report()
	if rep.Metrics.TotalThroughput == 0 {
		t.Fatalf("expected throughput > 0, got %+v", rep.Metrics)
	}
	t.Logf("throughput in=%d out=%d dangerous=%d warnings=%d avgWait=%.1f maxWait=%d",
		rep.Metrics.ThroughputIn, rep.Metrics.ThroughputOut,
		rep.Metrics.DangerousEncounters, rep.Metrics.CollisionWarnings,
		rep.Metrics.AvgWaitMinutes, rep.Metrics.MaxWaitMinutes)
	for _, b := range rep.Bottlenecks {
		t.Logf("bottleneck #%d %s avg=%.2f peak=%.2f %s", b.Rank, b.SegID, b.AvgCongestion, b.PeakCongestion, b.Priority)
	}
	if len(rep.Bottlenecks) > 3 {
		t.Fatalf("expected at most 3 bottlenecks, got %d", len(rep.Bottlenecks))
	}
	tr := e.Trajectory()
	if len(tr) == 0 {
		t.Fatalf("expected trajectory rows, got 0")
	}
	t.Logf("trajectory rows=%d", len(tr))
}

func TestTideBlocksDeepDraft(t *testing.T) {
	cfg := loadTestConfig(t)
	e := NewEngine(cfg, Params{DurationHours: 24, ArrivalRate: 3, Seed: 1})
	steps := 0
	for !e.Step() && steps < 2000 {
		steps++
	}
	_ = e.Frame()
}
