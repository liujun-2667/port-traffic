package sim

import (
	"fmt"
	"strconv"
)

// Report builds the full post-simulation assessment.
func (e *Engine) Report() Report {
	e.mu.Lock()
	defer e.mu.Unlock()

	avg, mx := waitStats(e.waitTimes)
	severe := 0
	for _, w := range e.waitTimes {
		if w > 60 {
			severe++
		}
	}
	segCong := make([]SegCongAvg, 0, len(e.port.Segments))
	for _, seg := range e.port.Segments {
		avgC := 0.0
		if c := e.segCongCount[seg.ID]; c > 0 {
			avgC = e.segCongSum[seg.ID] / float64(c)
		}
		segCong = append(segCong, SegCongAvg{
			SegID: seg.ID, AvgCongestion: avgC, PeakCongestion: e.segCongPeak[seg.ID],
		})
	}

	return Report{
		Summary: Summary{
			DurationMinutes: e.durationMin, ArrivalRate: e.cfg.Sim.ArrivalRate,
			Seed: e.cfg.Sim.Seed, WindSpeed: e.cfg.Weather.WindSpeed,
			Visibility: e.cfg.Weather.Visibility, SegmentCount: len(e.port.Segments),
			BerthCount: len(e.port.Berths), Strategy: e.strategy,
		},
		Metrics: Metrics{
			TotalThroughput:     e.thruIn + e.thruOut,
			ThroughputIn:        e.thruIn,
			ThroughputOut:       e.thruOut,
			AvgWaitMinutes:      avg,
			MaxWaitMinutes:      mx,
			SevereDelayCount:    severe,
			DangerousEncounters: e.dangerousCount,
			CollisionWarnings:   e.warningCount,
			SegmentCongestion:   segCong,
		},
		Events:      append([]TimelineEvent(nil), e.events...),
		Bottlenecks: buildBottlenecks(segCong),
		Advice:      e.advice(segCong),
	}
}

func (e *Engine) advice(segCong []SegCongAvg) []Advice {
	out := []Advice{}
	avgWait, _ := waitStats(e.waitTimes)
	maxPeak := 0.0
	var peakSeg string
	for _, sc := range segCong {
		if sc.PeakCongestion > maxPeak {
			maxPeak = sc.PeakCongestion
			peakSeg = sc.SegID
		}
	}
	if maxPeak > 0.7 {
		out = append(out, Advice{Code: "WIDEN", Text: "航段 " + peakSeg + " 峰值拥堵达 " + pct(maxPeak) + ",建议优先扩容或增设支航道"})
	}
	if e.cfg.Sim.ArrivalRate >= 4 && avgWait > 30 {
		out = append(out, Advice{Code: "TIDE_WINDOW", Text: "到达率较高且平均等待偏长,建议启用潮汐窗口调度,利用高潮位集中放行深吃水船舶"})
	}
	if e.dangerousCount > 0 {
		out = append(out, Advice{Code: "ONE_WAY", Text: fmt.Sprintf("累计危险会遇 %d 次,建议在 EZ 会遇区域实施单向通航或降低到达率", e.dangerousCount)})
	}
	if avgWait > 60 {
		out = append(out, Advice{Code: "ANCHORAGE", Text: "平均等待超过 60 分钟,建议增设锚地容量或分散靠泊窗口"})
	}
	if len(out) == 0 {
		out = append(out, Advice{Code: "OK", Text: "当前参数下各项安全指标处于正常区间,可维持现有调度策略"})
	}
	return out
}

func pct(v float64) string {
	return strconv.Itoa(int(v*100+0.5)) + "%"
}
