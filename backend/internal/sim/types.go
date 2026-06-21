package sim

import (
	"port-traffic/internal/model"
	"port-traffic/internal/safety"
)

// ThroughputPoint is cumulative in/out counts at a minute.
type ThroughputPoint struct {
	Minute int `json:"minute"`
	In     int `json:"in"`
	Out    int `json:"out"`
}

// SegCong is a segment congestion snapshot.
type SegCong struct {
	SegID       string  `json:"segId"`
	Congestion  float64 `json:"congestion"`
	Count       int     `json:"count"`
	Capacity    int     `json:"capacity"`
	Congested   bool    `json:"congested"`
}

// KPI holds headline indicators.
type KPI struct {
	InPort            int     `json:"inPort"`
	QueueLength       int     `json:"queueLength"`
	CongestedSegments int     `json:"congestedSegments"`
	CumDangerous      int     `json:"cumDangerous"`
	CumWarnings       int     `json:"cumWarnings"`
	ThroughputIn      int     `json:"throughputIn"`
	ThroughputOut     int     `json:"throughputOut"`
	AvgWait           float64 `json:"avgWait"`
	MaxWait           int     `json:"maxWait"`
}

// Frame is a complete simulation snapshot for streaming/rendering.
type Frame struct {
	Minute            int                  `json:"minute"`
	Clock             string               `json:"clock"`
	Done              bool                 `json:"done"`
	Ships             []*model.Ship        `json:"ships"`
	SegmentCongestion []SegCong            `json:"segmentCongestion"`
	Encounters        []safety.Encounter   `json:"encounters"`
	KPI               KPI                  `json:"kpi"`
	Throughput        []ThroughputPoint    `json:"throughput"`
	TideLevel         float64              `json:"tideLevel"`
	NavigableDepth    float64              `json:"navigableDepth"`
	Berths            []BerthState         `json:"berths"`
	Anchorage         AnchorageState       `json:"anchorage"`
}

// BerthState is a berth occupancy snapshot.
type BerthState struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Occupied bool   `json:"occupied"`
	ShipID   string `json:"shipId"`
}

// AnchorageState is an anchorage occupancy snapshot.
type AnchorageState struct {
	ID     string `json:"id"`
	Count  int    `json:"count"`
	Capacity int  `json:"capacity"`
}

// TimelineEvent is a notable simulation event.
type TimelineEvent struct {
	Minute int    `json:"minute"`
	Clock  string `json:"clock"`
	Type   string `json:"type"`
	ShipA  string `json:"shipA"`
	ShipB  string `json:"shipB"`
	Desc   string `json:"desc"`
}

// Bottleneck is a ranked congested segment.
type Bottleneck struct {
	Rank          int     `json:"rank"`
	SegID         string  `json:"segId"`
	AvgCongestion float64 `json:"avgCongestion"`
	PeakCongestion float64 `json:"peakCongestion"`
	Priority      string  `json:"priority"`
}

// Advice is a scheduling recommendation.
type Advice struct {
	Code string `json:"code"`
	Text string `json:"text"`
}

// Report is the full post-simulation assessment.
type Report struct {
	Summary     Summary          `json:"summary"`
	Metrics     Metrics          `json:"metrics"`
	Events      []TimelineEvent  `json:"events"`
	Bottlenecks []Bottleneck     `json:"bottlenecks"`
	Advice      []Advice         `json:"advice"`
}

// Summary captures simulation parameters.
type Summary struct {
	DurationMinutes int     `json:"durationMinutes"`
	ArrivalRate     float64 `json:"arrivalRate"`
	Seed             int64   `json:"seed"`
	WindSpeed        float64 `json:"windSpeed"`
	Visibility       float64 `json:"visibility"`
	SegmentCount     int     `json:"segmentCount"`
	BerthCount       int     `json:"berthCount"`
}

// Metrics aggregates safety/throughput statistics.
type Metrics struct {
	TotalThroughput    int     `json:"totalThroughput"`
	ThroughputIn       int     `json:"throughputIn"`
	ThroughputOut      int     `json:"throughputOut"`
	AvgWaitMinutes     float64 `json:"avgWaitMinutes"`
	MaxWaitMinutes     int     `json:"maxWaitMinutes"`
	SevereDelayCount   int     `json:"severeDelayCount"`
	DangerousEncounters int    `json:"dangerousEncounters"`
	CollisionWarnings  int    `json:"collisionWarnings"`
	SegmentCongestion  []SegCongAvg `json:"segmentCongestion"`
}

// SegCongAvg is a per-segment congestion summary.
type SegCongAvg struct {
	SegID         string  `json:"segId"`
	AvgCongestion float64 `json:"avgCongestion"`
	PeakCongestion float64 `json:"peakCongestion"`
}
