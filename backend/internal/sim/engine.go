// Package sim implements the discrete-event port traffic simulation engine.
package sim

import (
	"fmt"
	"math"
	"math/rand"
	"sync"

	"port-traffic/internal/config"
	"port-traffic/internal/model"
	"port-traffic/internal/safety"
	"port-traffic/internal/tide"
	"port-traffic/internal/traffic"
)

const knotToMPerMin = 1852.0 / 60.0

// Params are the per-run overrides supplied by the API.
type Params struct {
	DurationHours  int     `json:"durationHours"`
	ArrivalRate    float64 `json:"arrivalRate"`
	Seed           int64   `json:"seed"`
	WindSpeed      float64 `json:"windSpeed"`
	Visibility     float64 `json:"visibility"`
	SpeedFactor    float64 `json:"speedFactor"`
	SpeedLimitScale float64 `json:"speedLimitScale"` // multiplier on segment speed limits
	Strategy       StrategyConfig `json:"strategy"`
	ClosedSegments []string      `json:"closedSegments,omitempty"`
}

// TrajectoryRow is one ship position sample.
type TrajectoryRow struct {
	ShipID string  `json:"shipId"`
	Minute int     `json:"minute"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	State  string  `json:"state"`
	Speed  float64 `json:"speed"`
}

// Engine runs a single simulation.
type Engine struct {
	mu     sync.Mutex
	cfg    *config.Config
	port   *model.PortModel
	tide   *tide.Model
	gen    *traffic.Generator
	assess *safety.Assessor
	rnd    *rand.Rand

	berthOffset map[string]float64
	berthShip   map[string]string
	workEnd     map[string]int
	hold        map[string]bool
	activeDanger map[string]bool
	activeWarn  map[string]bool

	arrivals []traffic.Arrival
	arrIdx   int

	ships []*model.Ship
	byID  map[string]*model.Ship

	anchorageCount int
	anchorageCap   int

	minute      int
	durationMin int
	done        bool

	thruIn, thruOut int
	throughput      []ThroughputPoint
	waitTimes       []int
	curEncounters   []safety.Encounter
	curSegCong      []SegCong
	dangerousCount int
	warningCount    int
	events          []TimelineEvent
	segCongSum      map[string]float64
	segCongCount    map[string]int
	segCongPeak     map[string]float64
	trajectory      []TrajectoryRow

	strategy    StrategyConfig
	stateHist   map[string][]StateChange
	recentEvents []TimelineEvent

	closedSegments map[string]bool
}

// NewEngine builds an engine from config and run params.
func NewEngine(cfg *config.Config, p Params) *Engine {
	c := cfg.Clone()
	if p.DurationHours > 0 {
		c.Sim.DurationHours = p.DurationHours
	}
	if p.ArrivalRate > 0 {
		c.Sim.ArrivalRate = p.ArrivalRate
	}
	if p.Seed != 0 {
		c.Sim.Seed = p.Seed
	}
	if p.WindSpeed > 0 {
		c.Weather.WindSpeed = p.WindSpeed
	}
	if p.Visibility > 0 {
		c.Weather.Visibility = p.Visibility
	}
	if p.SpeedFactor > 0 {
		c.Sim.SpeedFactor = p.SpeedFactor
	}
	if p.SpeedLimitScale > 0 {
		for i := range c.Port.Segments {
			c.Port.Segments[i].SpeedLimit *= p.SpeedLimitScale
			if c.Port.Segments[i].SpeedLimit < 1 {
				c.Port.Segments[i].SpeedLimit = 1
			}
		}
	}
	strategy := p.Strategy
	if strategy.Strategy == "" {
		strategy.Strategy = StrategyFreeFlow
	}
	if strategy.TidalThresholdMeters <= 0 {
		strategy.TidalThresholdMeters = 5.0
	}
	if strategy.OneWaySwitchMinutes <= 0 {
		strategy.OneWaySwitchMinutes = 30
	}
	if len(strategy.OneWaySegments) == 0 {
		strategy.OneWaySegments = []string{"S1", "S2", "S3"}
	}
	closed := map[string]bool{}
	for _, id := range p.ClosedSegments {
		closed[id] = true
	}
	e := &Engine{
		cfg:    c,
		port:   &c.Port,
		tide:   tide.New(c.Tide),
		gen:    traffic.New(c.Traffic, c.Sim, &c.Port, c.Sim.Seed),
		assess: safety.New(&c.Port, c.Sim.EncounterSafeRatio, 5),
		rnd:    rand.New(rand.NewSource(c.Sim.Seed + 7)),
		byID:   map[string]*model.Ship{},
		berthOffset: map[string]float64{},
		berthShip:   map[string]string{},
		workEnd:     map[string]int{},
		activeDanger: map[string]bool{},
		activeWarn:   map[string]bool{},
		segCongSum:   map[string]float64{},
		segCongCount: map[string]int{},
		segCongPeak:  map[string]float64{},
		strategy:     strategy,
		stateHist:    map[string][]StateChange{},
		recentEvents: []TimelineEvent{},
		closedSegments: closed,
	}
	e.init()
	return e
}

func (e *Engine) init() {
	for i := range e.port.Berths {
		b := &e.port.Berths[i]
		seg, ok := e.port.SegmentByID(b.BranchSeg)
		if !ok {
			continue
		}
		h := seg.Heading()
		off := (b.Position.X-seg.From.X)*h.X + (b.Position.Y-seg.From.Y)*h.Y
		if off < 0 {
			off = 0
		}
		if off > seg.Length() {
			off = seg.Length()
		}
		e.berthOffset[b.ID] = off
	}
	if len(e.port.Anchorages) > 0 {
		e.anchorageCap = e.port.Anchorages[0].Capacity
	}
	e.durationMin = e.cfg.Sim.DurationHours * 60
	e.arrivals = e.gen.Arrivals(e.durationMin)
	e.throughput = []ThroughputPoint{{Minute: 0, In: 0, Out: 0}}
}

// Step advances the simulation by one minute. Returns false when finished.
func (e *Engine) Step() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.done {
		return false
	}
	e.minute++
	e.processArrivals()
	e.tryEnterChannel()
	e.updateHolding()
	e.moveShips()
	e.handleWork()
	e.assessSafety()
	e.recordCongestion()
	e.recordThroughput()
	e.recordTrajectory()
	if e.minute >= e.durationMin {
		e.done = true
	}
	return !e.done
}

// Done reports whether the simulation has finished.
func (e *Engine) Done() bool { e.mu.Lock(); defer e.mu.Unlock(); return e.done }

// Minute returns the current simulation minute.
func (e *Engine) Minute() int { e.mu.Lock(); defer e.mu.Unlock(); return e.minute }

func (e *Engine) hours() float64 { return float64(e.minute) / 60.0 }

func (e *Engine) processArrivals() {
	for e.arrIdx < len(e.arrivals) && e.arrivals[e.arrIdx].Minute <= e.minute {
		s := e.arrivals[e.arrIdx].Ship
		e.arrIdx++
		seg, _ := e.port.SegmentByID("S1")
		if seg.Length() > 0 {
			s.Position = seg.From
		}
		prev := s.State
		s.State = model.StateArrived
		if prev != s.State {
			e.recordStateChange(s)
		}
		e.ships = append(e.ships, s)
		e.byID[s.ID] = s
		e.addEvent(TimelineEvent{
			Minute: e.minute, Clock: clock(e.minute), Type: "arrival",
			ShipA: s.ID, Desc: fmt.Sprintf("%s 到达锚地", s.ID),
		})
	}
}

func (e *Engine) tryEnterChannel() {
	for _, s := range e.ships {
		if s.State != model.StateArrived && s.State != model.StateWaiting {
			continue
		}
		seg, _ := e.port.SegmentByID("S1")
		if !e.tide.CanNavigate(seg.BaseDepth, s.Draft, e.hours()) {
			continue
		}
		if !e.spacingOK(s) || !e.s1HasCapacity(s) {
			continue
		}
		if !e.tidalWindowAllows(s) {
			continue
		}
		if e.strategy.Strategy == StrategyAlternatingOneWay && e.currentOneWayDirection() != 1 {
			continue
		}
		berth := e.pickBerth(s)
		if berth == nil {
			if s.State == model.StateArrived && e.anchorageCount < e.anchorageCap {
				e.anchorageCount++
				prev := s.State
				s.State = model.StateWaiting
				if prev != s.State {
					e.recordStateChange(s)
				}
			}
			continue
		}
		route := e.inboundRoute(berth.ID)
		if e.routeHasClosedSegment(route) {
			if s.State == model.StateArrived && e.anchorageCount < e.anchorageCap {
				e.anchorageCount++
				prev := s.State
				s.State = model.StateWaiting
				if prev != s.State {
					e.recordStateChange(s)
				}
			}
			continue
		}
		if s.State == model.StateWaiting {
			e.anchorageCount--
		}
		s.TargetBerth = berth.ID
		e.berthShip[berth.ID] = s.ID
		s.Route = e.inboundRoute(berth.ID)
		s.RouteIdx = 0
		s.SegOffset = 0
		s.Direction = 1
		prev := s.State
		s.State = model.StateInbound
		if prev != s.State {
			e.recordStateChange(s)
		}
		s.Position = seg.From
		s.EnterMinute = e.minute
		s.WaitMinutes = e.minute - s.ArrivalMinute
		e.waitTimes = append(e.waitTimes, s.WaitMinutes)
		e.thruIn++
		e.addEvent(TimelineEvent{
			Minute: e.minute, Clock: clock(e.minute), Type: "inbound_start",
			ShipA: s.ID, Desc: fmt.Sprintf("%s 开始进港 目标泊位 %s", s.ID, berth.ID),
		})
	}
}

func (e *Engine) spacingOK(s *model.Ship) bool {
	minGap := e.cfg.Sim.SafeSpacingShips * s.Length
	for _, o := range e.ships {
		if o.ID == s.ID {
			continue
		}
		if o.State != model.StateInbound && o.State != model.StateHolding {
			continue
		}
		if o.Direction != 1 || len(o.Route) == 0 || o.Route[0] != "S1" {
			continue
		}
		if o.SegOffset < minGap {
			return false
		}
	}
	return true
}

func (e *Engine) s1HasCapacity(s *model.Ship) bool {
	seg, _ := e.port.SegmentByID("S1")
	avgLen := (e.cfg.Traffic.LengthMin + e.cfg.Traffic.LengthMax) / 2
	maxBeam := e.cfg.Traffic.LengthMax * 0.19
	cap := safety.Capacity(seg, avgLen, maxBeam, e.cfg.Sim.SafeSpacingShips)
	count := 0
	for _, o := range e.ships {
		if o.RouteIdx < len(o.Route) && o.Route[o.RouteIdx] == "S1" &&
			(o.State == model.StateInbound || o.State == model.StateOutbound || o.State == model.StateHolding) {
			count++
		}
	}
	return count < cap
}

func (e *Engine) pickBerth(s *model.Ship) *model.Berth {
	var matched, others []*model.Berth
	for i := range e.port.Berths {
		b := &e.port.Berths[i]
		if e.berthShip[b.ID] != "" {
			continue
		}
		if b.MaxTonnage < s.DWT {
			continue
		}
		if b.Type == string(s.Type) {
			matched = append(matched, b)
		} else {
			others = append(others, b)
		}
	}
	pool := matched
	if len(pool) == 0 {
		pool = others
	}
	if len(pool) == 0 {
		return nil
	}
	return pool[e.rnd.Intn(len(pool))]
}

func (e *Engine) inboundRoute(berthID string) []string {
	b, _ := e.port.BerthByID(berthID)
	return []string{"S1", "S2", "S3", b.BranchSeg}
}

func (e *Engine) outboundRoute(berthID string) []string {
	b, _ := e.port.BerthByID(berthID)
	return []string{b.BranchSeg, "S3", "S2", "S1"}
}

func (e *Engine) updateHolding() {
	e.hold = map[string]bool{}
	inTransit := []*model.Ship{}
	for _, s := range e.ships {
		if s.State == model.StateInbound || s.State == model.StateOutbound || s.State == model.StateHolding {
			inTransit = append(inTransit, s)
		}
	}
	for _, s := range inTransit {
		if !e.oneWayAllows(s) {
			e.hold[s.ID] = true
		}
	}
	for i := 0; i < len(inTransit); i++ {
		for j := i + 1; j < len(inTransit); j++ {
			A, B := inTransit[i], inTransit[j]
			if A.Direction == B.Direction {
				continue
			}
			d := model.Dist(A.Position, B.Position)
			holdDist := math.Max(400, 4*(A.Beam+B.Beam))
			if d < holdDist {
				var holder *model.Ship
				if A.EnterMinute >= B.EnterMinute {
					holder = A
				} else {
					holder = B
				}
				e.hold[holder.ID] = true
			}
		}
	}
}

func (e *Engine) moveShips() {
	for _, s := range e.ships {
		if s.State != model.StateInbound && s.State != model.StateOutbound && s.State != model.StateHolding {
			continue
		}
		holding := e.hold[s.ID]
		prev := s.State
		if holding {
			s.State = model.StateHolding
		} else if s.State == model.StateHolding {
			if s.Direction > 0 {
				s.State = model.StateInbound
			} else {
				s.State = model.StateOutbound
			}
		}
		if prev != s.State {
			e.recordStateChange(s)
		}
		seg, ok := e.port.SegmentByID(s.Route[s.RouteIdx])
		if !ok {
			continue
		}
		limit := e.weatherAdjusted(seg.SpeedLimit)
		target := math.Min(s.PlannedSpeed, limit)
		if s.State == model.StateHolding {
			target = 0
		}
		if s.SpeedKn < target {
			s.SpeedKn += s.Maneuver.AccelRate
			if s.SpeedKn > target {
				s.SpeedKn = target
			}
		} else if s.SpeedKn > target {
			s.SpeedKn -= s.Maneuver.DecelRate
			if s.SpeedKn < target {
				s.SpeedKn = target
			}
		}
		s.SegOffset += s.SpeedKn * knotToMPerMin
		e.advanceAlongRoute(s)
	}
}

func (e *Engine) advanceAlongRoute(s *model.Ship) {
	for iter := 0; iter < 8; iter++ {
		seg, ok := e.port.SegmentByID(s.Route[s.RouteIdx])
		if !ok {
			return
		}
		if s.State == model.StateInbound && s.RouteIdx == len(s.Route)-1 {
			off := e.berthOffset[s.TargetBerth]
			if s.SegOffset >= off {
				s.SegOffset = off
				e.arriveAtBerth(s, seg)
				return
			}
		}
		if s.SegOffset < seg.Length() {
			s.Position = e.posOnSeg(seg, s.SegOffset, s.Direction)
			return
		}
		s.SegOffset -= seg.Length()
		s.RouteIdx++
		if s.RouteIdx >= len(s.Route) {
			if s.State == model.StateOutbound {
				s.Position = seg.From
				e.depart(s)
			}
			return
		}
	}
}

func (e *Engine) posOnSeg(seg model.Segment, off float64, dir int) model.Point {
	h := seg.Heading()
	if dir >= 0 {
		return model.Point{X: seg.From.X + off*h.X, Y: seg.From.Y + off*h.Y}
	}
	return model.Point{X: seg.To.X - off*h.X, Y: seg.To.Y - off*h.Y}
}

func (e *Engine) arriveAtBerth(s *model.Ship, seg model.Segment) {
	prev := s.State
	s.State = model.StateWorking
	if prev != s.State {
		e.recordStateChange(s)
	}
	s.BerthMinute = e.minute
	s.Position = e.posOnSeg(seg, e.berthOffset[s.TargetBerth], s.Direction)
	s.SpeedKn = 0
	dur := e.workDuration(s)
	e.workEnd[s.ID] = e.minute + dur
	e.addEvent(TimelineEvent{
		Minute: e.minute, Clock: clock(e.minute), Type: "berth",
		ShipA: s.ID, Desc: fmt.Sprintf("%s 抵达泊位 %s 开始作业 %dmin", s.ID, s.TargetBerth, dur),
	})
}

func (e *Engine) handleWork() {
	for _, s := range e.ships {
		if s.State != model.StateWorking {
			continue
		}
		if e.minute >= e.workEnd[s.ID] {
			e.startOutbound(s)
		}
	}
}

func (e *Engine) startOutbound(s *model.Ship) {
	route := e.outboundRoute(s.TargetBerth)
	if e.routeHasClosedSegment(route) {
		// Channel closed: delay departure until dredging finished; keep working state
		e.workEnd[s.ID] = e.minute + 30 // re-check in 30 minutes
		return
	}
	prev := s.State
	s.State = model.StateOutbound
	if prev != s.State {
		e.recordStateChange(s)
	}
	s.Route = route
	s.RouteIdx = 0
	seg, _ := e.port.SegmentByID(s.Route[0])
	s.Direction = -1
	off := e.berthOffset[s.TargetBerth]
	s.SegOffset = seg.Length() - off
	s.Position = e.posOnSeg(seg, s.SegOffset, -1)
	s.SpeedKn = 0
	e.berthShip[s.TargetBerth] = ""
	e.addEvent(TimelineEvent{
		Minute: e.minute, Clock: clock(e.minute), Type: "departure_start",
		ShipA: s.ID, Desc: fmt.Sprintf("%s 完成作业 离泊出港", s.ID),
	})
}

func (e *Engine) routeHasClosedSegment(route []string) bool {
	if len(e.closedSegments) == 0 {
		return false
	}
	for _, id := range route {
		if e.closedSegments[id] {
			return true
		}
	}
	return false
}

// ClosedSegments returns the list of segment IDs currently closed for dredging.
func (e *Engine) ClosedSegments() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]string, 0, len(e.closedSegments))
	for id := range e.closedSegments {
		out = append(out, id)
	}
	return out
}

func (e *Engine) depart(s *model.Ship) {
	prev := s.State
	s.State = model.StateDeparted
	if prev != s.State {
		e.recordStateChange(s)
	}
	s.Position = model.Point{}
	s.SpeedKn = 0
	e.thruOut++
	e.trajectory = append(e.trajectory, TrajectoryRow{
		ShipID: s.ID, Minute: e.minute, X: 0, Y: 0, State: "departed", Speed: 0,
	})
	e.addEvent(TimelineEvent{
		Minute: e.minute, Clock: clock(e.minute), Type: "departed",
		ShipA: s.ID, Desc: fmt.Sprintf("%s 离港", s.ID),
	})
}

func (e *Engine) assessSafety() {
	encs := e.assess.Assess(e.ships, e.minute)
	e.curEncounters = encs
	seen := map[string]bool{}
	for _, enc := range encs {
		key := pairKey(enc.ShipA, enc.ShipB)
		seen[key] = true
		if enc.Dangerous && !e.activeDanger[key] {
			e.activeDanger[key] = true
			e.dangerousCount++
			e.addEvent(TimelineEvent{
				Minute: e.minute, Clock: clock(e.minute), Type: "danger",
				ShipA: enc.ShipA, ShipB: enc.ShipB,
				Desc: fmt.Sprintf("危险会遇 %s/%s DCPA=%.0fm TCPA=%.1fmin", enc.ShipA, enc.ShipB, enc.DCPA, enc.TCPA),
			})
		}
		if enc.Warning && !e.activeWarn[key] {
			e.activeWarn[key] = true
			e.warningCount++
			e.addEvent(TimelineEvent{
				Minute: e.minute, Clock: clock(e.minute), Type: "warning",
				ShipA: enc.ShipA, ShipB: enc.ShipB,
				Desc: fmt.Sprintf("碰撞预警 %s/%s TCPA=%.1fmin", enc.ShipA, enc.ShipB, enc.TCPA),
			})
		}
	}
	for k := range e.activeDanger {
		if !seen[k] {
			delete(e.activeDanger, k)
		}
	}
	for k := range e.activeWarn {
		if !seen[k] {
			delete(e.activeWarn, k)
		}
	}
}

func (e *Engine) recordCongestion() {
	avgLen := (e.cfg.Traffic.LengthMin + e.cfg.Traffic.LengthMax) / 2
	maxBeam := e.cfg.Traffic.LengthMax * 0.19
	counts := map[string]int{}
	for _, s := range e.ships {
		if s.State != model.StateInbound && s.State != model.StateOutbound && s.State != model.StateHolding {
			continue
		}
		if s.RouteIdx < len(s.Route) {
			counts[s.Route[s.RouteIdx]]++
		}
	}
	cur := make([]SegCong, 0, len(e.port.Segments))
	for _, seg := range e.port.Segments {
		c := counts[seg.ID]
		cap := safety.Capacity(seg, avgLen, maxBeam, e.cfg.Sim.SafeSpacingShips)
		cong := safety.Congestion(c, cap)
		e.segCongSum[seg.ID] += cong
		e.segCongCount[seg.ID]++
		if cong > e.segCongPeak[seg.ID] {
			e.segCongPeak[seg.ID] = cong
		}
		cur = append(cur, SegCong{
			SegID: seg.ID, Congestion: cong, Count: c, Capacity: cap, Congested: cong > 0.7,
		})
	}
	e.curSegCong = cur
}

func (e *Engine) recordThroughput() {
	e.throughput = append(e.throughput, ThroughputPoint{Minute: e.minute, In: e.thruIn, Out: e.thruOut})
}

func (e *Engine) recordTrajectory() {
	for _, s := range e.ships {
		if s.State == model.StateDeparted {
			continue
		}
		e.trajectory = append(e.trajectory, TrajectoryRow{
			ShipID: s.ID, Minute: e.minute,
			X: s.Position.X, Y: s.Position.Y, State: string(s.State), Speed: s.SpeedKn,
		})
	}
}

func (e *Engine) weatherAdjusted(limit float64) float64 {
	f := 1.0
	if e.cfg.Weather.WindSpeed > 25 {
		f -= 0.2
	}
	if e.cfg.Weather.Visibility < 1 {
		f -= 0.3
	}
	if e.cfg.Weather.Swell > 1.5 {
		f -= 0.1
	}
	if f < 0.4 {
		f = 0.4
	}
	return limit * f
}

func (e *Engine) workDuration(s *model.Ship) int {
	mean := e.cfg.Traffic.WorkDurationMinutes[string(s.Type)]
	if mean == 0 {
		mean = 300
	}
	jit := e.cfg.Traffic.WorkJitter
	delta := (e.rnd.Float64()*2 - 1) * jit * float64(mean)
	d := int(float64(mean) + delta)
	if d < 30 {
		d = 30
	}
	return d
}

func pairKey(a, b string) string {
	if a > b {
		return a + "|" + b
	}
	return b + "|" + a
}

func clock(minute int) string {
	h := (minute / 60) % 24
	m := minute % 60
	return fmt.Sprintf("%02d:%02d", h, m)
}

func (e *Engine) addEvent(ev TimelineEvent) {
	e.events = append(e.events, ev)
	e.recentEvents = append(e.recentEvents, ev)
	if len(e.recentEvents) > 200 {
		e.recentEvents = e.recentEvents[len(e.recentEvents)-200:]
	}
}

func (e *Engine) recordStateChange(s *model.Ship) {
	sc := StateChange{
		Minute: e.minute,
		Clock:  clock(e.minute),
		State:  string(s.State),
		X:      s.Position.X,
		Y:      s.Position.Y,
	}
	e.stateHist[s.ID] = append(e.stateHist[s.ID], sc)
}

func (e *Engine) tidalWindowAllows(s *model.Ship) bool {
	if e.strategy.Strategy != StrategyTidalWindow {
		return true
	}
	largeVessel := s.Length >= 200 || s.Draft >= e.strategy.TidalThresholdMeters
	if !largeVessel {
		return true
	}
	curLevel := e.tide.Level(e.hours())
	return curLevel >= e.strategy.TidalThresholdMeters
}

func (e *Engine) currentOneWayDirection() int {
	period := e.strategy.OneWaySwitchMinutes
	if period <= 0 {
		period = 30
	}
	slot := e.minute / period
	if slot%2 == 0 {
		return 1
	}
	return -1
}

func (e *Engine) isOneWaySegment(segID string) bool {
	for _, id := range e.strategy.OneWaySegments {
		if id == segID {
			return true
		}
	}
	return false
}

func (e *Engine) oneWayAllows(s *model.Ship) bool {
	if e.strategy.Strategy != StrategyAlternatingOneWay {
		return true
	}
	if s.RouteIdx >= len(s.Route) {
		return true
	}
	curSeg := s.Route[s.RouteIdx]
	if !e.isOneWaySegment(curSeg) {
		return true
	}
	allowedDir := e.currentOneWayDirection()
	return s.Direction == allowedDir
}

func (e *Engine) GetStateHistory(shipID string) []StateChange {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]StateChange(nil), e.stateHist[shipID]...)
}

func (e *Engine) GetShipDangerousEncounters(shipID string) []TimelineEvent {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := []TimelineEvent{}
	for _, ev := range e.events {
		if ev.Type == "danger" || ev.Type == "warning" {
			if ev.ShipA == shipID || ev.ShipB == shipID {
				out = append(out, ev)
			}
		}
	}
	return out
}
