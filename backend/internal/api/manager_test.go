package api

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"port-traffic/internal/config"
	"port-traffic/internal/sim"
)

func loadCfg(t *testing.T) *config.Service {
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
				t.Fatalf("load: %v", err)
			}
			return svc
		}
	}
	t.Skip("port.yaml not found")
	return nil
}

func TestStartRun_RunIdJSAndSubscribe(t *testing.T) {
	cfg := loadCfg(t)
	defer cfg.Close()

	mgr := NewManager(cfg, nil)
	const JSMaxSafe = 1 << 53 // 2^53
	id, err := mgr.StartRun(sim.Params{DurationHours: 1, ArrivalRate: 10, Seed: 7})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive id, got %d", id)
	}
	if id >= JSMaxSafe {
		t.Fatalf("id %d exceeds JS MAX_SAFE_INTEGER %d", id, JSMaxSafe)
	}
	if !mgr.HasRun(id) {
		t.Fatalf("HasRun false after StartRun")
	}
	ch, ok := mgr.Subscribe(id)
	if !ok {
		t.Fatalf("Subscribe failed")
	}
	defer mgr.Unsubscribe(id, ch)

	// The goroutine immediately sends current frame; wait up to 3s for 3 frames.
	deadline := time.Now().Add(3 * time.Second)
	frames := 0
	for time.Now().Before(deadline) && frames < 3 {
		select {
		case f, open := <-ch:
			if !open {
				goto done
			}
			frames++
			if f.Minute < 0 {
				t.Fatalf("unexpected minute %d", f.Minute)
			}
		default:
			time.Sleep(30 * time.Millisecond)
		}
	}
done:
	if frames < 1 {
		t.Fatalf("expected at least 1 frame, got %d (id=%d)", frames, id)
	}
	t.Logf("runId=%d framesReceived=%d", id, frames)

	// second StartRun with incremented nextID
	id2, _ := mgr.StartRun(sim.Params{DurationHours: 1, ArrivalRate: 10, Seed: 8})
	if id2 <= id {
		t.Fatalf("expected strictly increasing id, got id2=%d <= id=%d", id2, id)
	}
	if id2 >= JSMaxSafe {
		t.Fatalf("id2 %d exceeds JS MAX_SAFE_INTEGER", id2)
	}
}
