// Package model defines the core domain types for the port traffic simulation.
package model

import "math"

// Point is a 2D coordinate in meters (port local frame, origin bottom-left).
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Segment is a directed channel segment.
type Segment struct {
	ID         string  `yaml:"id" json:"id"`
	From       Point   `yaml:"from" json:"from"`
	To         Point   `yaml:"to" json:"to"`
	Width      float64 `yaml:"width" json:"width"`           // meters
	BaseDepth  float64 `yaml:"baseDepth" json:"baseDepth"`   // meters at chart datum
	SpeedLimit float64 `yaml:"speedLimit" json:"speedLimit"` // knots
	MaxTonnage float64 `yaml:"maxTonnage" json:"maxTonnage"` // tonnes
}

// Length returns the segment length in meters.
func (s Segment) Length() float64 {
	dx := s.To.X - s.From.X
	dy := s.To.Y - s.From.Y
	return dist(dx, dy)
}

// Heading returns unit direction vector along the segment.
func (s Segment) Heading() Point {
	l := s.Length()
	if l == 0 {
		return Point{0, 0}
	}
	return Point{(s.To.X - s.From.X) / l, (s.To.Y - s.From.Y) / l}
}

// Berth is a mooring position.
type Berth struct {
	ID         string  `yaml:"id" json:"id"`
	Type       string  `yaml:"type" json:"type"` // container/bulk/tanker/general
	MaxTonnage float64 `yaml:"maxTonnage" json:"maxTonnage"`
	Position   Point   `yaml:"position" json:"position"`
	BranchSeg  string  `yaml:"branchSeg" json:"branchSeg"`
}

// Anchorage is a waiting area.
type Anchorage struct {
	ID           string `yaml:"id" json:"id"`
	Capacity     int    `yaml:"capacity" json:"capacity"`
	CurrentCount int    `yaml:"currentCount" json:"currentCount"`
	Position     Point  `yaml:"position" json:"position"`
}

// TurningArea is a maneuvering basin.
type TurningArea struct {
	ID       string  `yaml:"id" json:"id"`
	Diameter float64 `yaml:"diameter" json:"diameter"`
	Position Point   `yaml:"position" json:"position"`
}

// EncounterZone marks where two channels merge (conflict detection).
type EncounterZone struct {
	ID         string   `yaml:"id" json:"id"`
	Position   Point    `yaml:"position" json:"position"`
	SegmentIDs []string `yaml:"segmentIds" json:"segmentIds"`
	Radius     float64  `yaml:"radius" json:"radius"` // detection radius meters
}

// PortModel is the abstract port water area.
type PortModel struct {
	Segments      []Segment      `yaml:"segments" json:"segments"`
	Berths        []Berth        `yaml:"berths" json:"berths"`
	Anchorages    []Anchorage    `yaml:"anchorages" json:"anchorages"`
	TurningAreas  []TurningArea  `yaml:"turningAreas" json:"turningAreas"`
	EncounterZones []EncounterZone `yaml:"encounterZones" json:"encounterZones"`
	Bounds        [2]Point       `yaml:"bounds" json:"bounds"` // [min, max]
}

// SegmentByID looks up a segment by id.
func (p *PortModel) SegmentByID(id string) (Segment, bool) {
	for _, s := range p.Segments {
		if s.ID == id {
			return s, true
		}
	}
	return Segment{}, false
}

// BerthByID looks up a berth by id.
func (p *PortModel) BerthByID(id string) (Berth, bool) {
	for _, b := range p.Berths {
		if b.ID == id {
			return b, true
		}
	}
	return Berth{}, false
}

// ShipType enumerates vessel categories.
type ShipType string

const (
	TypeContainer ShipType = "container"
	TypeBulk      ShipType = "bulk"
	TypeTanker    ShipType = "tanker"
	TypeOther     ShipType = "other"
)

// Maneuverability holds handling parameters affecting safety spacing.
type Maneuverability struct {
	TurningRadius float64 `json:"turningRadius"` // meters
	StopDistance  float64 `json:"stopDistance"`  // meters
	AccelRate     float64 `json:"accelRate"`     // knots per minute
	DecelRate     float64 `json:"decelRate"`     // knots per minute
}

// ShipState enumerates the simulation state machine.
type ShipState string

const (
	StateArrived    ShipState = "arrived"    // generated, in arrival queue
	StateWaiting    ShipState = "waiting"    // in anchorage waiting for berth
	StateInbound    ShipState = "inbound"   // transiting inbound channel
	StateBerthing   ShipState = "berthing"  // maneuvering to berth
	StateWorking    ShipState = "working"   // cargo operation at berth
	StateOutbound   ShipState = "outbound"  // transiting outbound channel
	StateDeparted   ShipState = "departed"  // left the port
	StateHolding     ShipState = "holding"   // slowed/stopped for encounter conflict
)

// Ship is a vessel instance in the simulation.
type Ship struct {
	ID            string         `json:"id"`
	Type          ShipType       `json:"type"`
	Length        float64        `json:"length"`  // meters
	Beam          float64       `json:"beam"`   // meters
	Draft         float64        `json:"draft"`  // meters
	DWT           float64        `json:"dwt"`    // tonnes
	TargetBerth   string         `json:"targetBerth"`
	SpeedKn       float64        `json:"speedKn"`    // current speed knots
	PlannedSpeed  float64        `json:"plannedSpeed"` // intended speed knots
	Maneuver      Maneuverability `json:"maneuver"`
	State         ShipState      `json:"state"`
	Position      Point          `json:"position"`
	Route         []string       `json:"route"`    // segment ids
	RouteIdx      int            `json:"routeIdx"` // current segment index
	SegOffset     float64        `json:"segOffset"` // distance traveled along current segment (m)
	ArrivalMinute int            `json:"arrivalMinute"`
	EnterMinute   int            `json:"enterMinute"` // minute entered channel
	BerthMinute   int            `json:"berthMinute"`
	WorkDuration  int            `json:"workDuration"` // minutes
	WaitMinutes   int            `json:"waitMinutes"`
	Direction     int            `json:"direction"` // +1 travel From->To, -1 travel To->From along current segment
}

// dist returns Euclidean distance of a delta.
func dist(dx, dy float64) float64 {
	return math.Sqrt(dx*dx + dy*dy)
}

// Dist returns the distance between two points.
func Dist(a, b Point) float64 { return dist(b.X-a.X, b.Y-a.Y) }
