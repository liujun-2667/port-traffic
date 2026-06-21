package sim

import (
	"sort"

	"port-traffic/internal/model"
	"port-traffic/internal/safety"
)

// Frame returns a renderable snapshot of the current state.
func (e *Engine) Frame() Frame {
	e.mu.Lock()
	defer e.mu.Unlock()

	ships := make([]*model.Ship, 0, len(e.ships))
	for _, s := range e.ships {
		if s.State == model.StateDeparted {
			continue
		}
		cp := *s
		ships = append(ships, &cp)
	}
	berths := make([]BerthState, 0, len(e.port.Berths))
	for _, b := range e.port.Berths {
		berths = append(berths, BerthState{
			ID: b.ID, Type: b.Type, Occupied: e.berthShip[b.ID] != "", ShipID: e.berthShip[b.ID],
		})
	}
	anc := AnchorageState{}
	if len(e.port.Anchorages) > 0 {
		anc = AnchorageState{ID: e.port.Anchorages[0].ID, Count: e.anchorageCount, Capacity: e.anchorageCap}
	}
	segCong := append([]SegCong(nil), e.curSegCong...)
	encs := append([]safety.Encounter(nil), e.curEncounters...)
	thru := append([]ThroughputPoint(nil), e.throughput...)

	depth := 0.0
	if seg, ok := e.port.SegmentByID("S1"); ok {
		depth = e.tide.NavigableDepth(seg.BaseDepth, e.hours())
	}

	return Frame{
		Minute: e.minute, Clock: clock(e.minute), Done: e.done,
		Ships: ships, SegmentCongestion: segCong, Encounters: encs,
		KPI: e.kpiLocked(), Throughput: thru,
		TideLevel: e.tide.Level(e.hours()), NavigableDepth: depth,
		Berths: berths, Anchorage: anc,
	}
}

func (e *Engine) kpiLocked() KPI {
	inPort, queueLen := 0, 0
	for _, s := range e.ships {
		if s.State == model.StateDeparted {
			continue
		}
		inPort++
		if s.State == model.StateArrived || s.State == model.StateWaiting {
			queueLen++
		}
	}
	congested := 0
	for _, sc := range e.curSegCong {
		if sc.Congested {
			congested++
		}
	}
	avg, mx := waitStats(e.waitTimes)
	return KPI{
		InPort: inPort, QueueLength: queueLen, CongestedSegments: congested,
		CumDangerous: e.dangerousCount, CumWarnings: e.warningCount,
		ThroughputIn: e.thruIn, ThroughputOut: e.thruOut,
		AvgWait: avg, MaxWait: mx,
	}
}

func waitStats(ws []int) (float64, int) {
	if len(ws) == 0 {
		return 0, 0
	}
	sum := 0
	mx := 0
	for _, w := range ws {
		sum += w
		if w > mx {
			mx = w
		}
	}
	return float64(sum) / float64(len(ws)), mx
}

// Trajectory returns a copy of all recorded trajectory rows.
func (e *Engine) Trajectory() []TrajectoryRow {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := append([]TrajectoryRow(nil), e.trajectory...)
	return out
}

// buildBottlenecks ranks segments by average congestion.
func buildBottlenecks(segCong []SegCongAvg) []Bottleneck {
	sorted := append([]SegCongAvg(nil), segCong...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].AvgCongestion > sorted[j].AvgCongestion
	})
	out := make([]Bottleneck, 0, 3)
	for i, sc := range sorted {
		if i >= 3 {
			break
		}
		priority := "低"
		switch {
		case sc.AvgCongestion > 0.9:
			priority = "高"
		case sc.AvgCongestion > 0.7:
			priority = "中"
		}
		out = append(out, Bottleneck{
			Rank: i + 1, SegID: sc.SegID,
			AvgCongestion: sc.AvgCongestion, PeakCongestion: sc.PeakCongestion,
			Priority: priority,
		})
	}
	return out
}
