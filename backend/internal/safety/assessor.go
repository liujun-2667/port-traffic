// Package safety computes DCPA/TCPA encounter risk and channel congestion.
package safety

import (
	"math"

	"port-traffic/internal/model"
)

const knotToMPerMin = 1852.0 / 60.0

// Encounter describes a pairwise close-quarters situation.
type Encounter struct {
	ShipA     string      `json:"shipA"`
	ShipB     string      `json:"shipB"`
	DCPA      float64     `json:"dcpa"`      // meters, closest passing distance
	TCPA      float64     `json:"tcpa"`      // minutes to closest approach
	Dangerous bool        `json:"dangerous"` // DCPA below safe threshold
	Warning   bool        `json:"warning"`   // imminent collision warning
	Position  model.Point `json:"position"`
	Minute    int         `json:"minute"`
}

// Assessor evaluates safety metrics for the fleet.
type Assessor struct {
	port      *model.PortModel
	safeRatio float64 // DCPA unsafe when < safeRatio*(beamA+beamB)
	tcpaMin   float64 // minutes
}

// New creates an Assessor.
func New(port *model.PortModel, safeRatio, tcpaMin float64) *Assessor {
	return &Assessor{port: port, safeRatio: safeRatio, tcpaMin: tcpaMin}
}

// velocity returns a ship's velocity vector in m/min.
func (a *Assessor) velocity(s *model.Ship) (vx, vy float64) {
	if s.RouteIdx >= len(s.Route) {
		return 0, 0
	}
	seg, ok := a.port.SegmentByID(s.Route[s.RouteIdx])
	if !ok {
		return 0, 0
	}
	h := seg.Heading()
	sign := float64(s.Direction)
	sp := s.SpeedKn * knotToMPerMin
	return h.X * sign * sp, h.Y * sign * sp
}

// Assess computes dangerous encounters among in-transit ships.
func (a *Assessor) Assess(ships []*model.Ship, minute int) []Encounter {
	moving := make([]*model.Ship, 0, len(ships))
	for _, s := range ships {
		if s.State == model.StateInbound || s.State == model.StateOutbound || s.State == model.StateHolding {
			moving = append(moving, s)
		}
	}
	out := make([]Encounter, 0)
	for i := 0; i < len(moving); i++ {
		for j := i + 1; j < len(moving); j++ {
			A, B := moving[i], moving[j]
			d := model.Dist(A.Position, B.Position)
			if d > 4000 {
				continue
			}
			enc := a.pair(A, B, minute, d)
			if enc != nil {
				out = append(out, *enc)
			}
		}
	}
	return out
}

func (a *Assessor) pair(A, B *model.Ship, minute int, d float64) *Encounter {
	ax, ay := a.velocity(A)
	bx, by := a.velocity(B)
	rx := B.Position.X - A.Position.X
	ry := B.Position.Y - A.Position.Y
	vx := bx - ax
	vy := by - ay
	v2 := vx*vx + vy*vy

	var tcpa, dcpa float64
	if v2 > 1e-6 {
		tcpa = -(rx*vx + ry*vy) / v2
		dcpa = math.Abs(rx*vy-ry*vx) / math.Sqrt(v2)
	} else {
		tcpa = 0
		dcpa = d
	}

	threshold := a.safeRatio * (A.Beam + B.Beam)
	mid := model.Point{X: (A.Position.X + B.Position.X) / 2, Y: (A.Position.Y + B.Position.Y) / 2}
	enc := &Encounter{
		ShipA: A.ID, ShipB: B.ID, DCPA: dcpa, TCPA: tcpa,
		Position: mid, Minute: minute,
	}
	enc.Dangerous = dcpa < threshold
	enc.Warning = enc.Dangerous && tcpa >= 0 && tcpa < a.tcpaMin
	if !enc.Dangerous && d > 1500 {
		return nil // drop distant safe pairs to reduce noise
	}
	return enc
}

// Capacity returns the theoretical max ships a segment can hold.
// capacity = floor(width/maxBeam) lanes * floor(length/(safeSpacing*avgLen)).
func Capacity(seg model.Segment, avgLen, maxBeam, safeSpacing float64) int {
	if maxBeam <= 0 {
		maxBeam = 30
	}
	if avgLen <= 0 {
		avgLen = 200
	}
	lanes := int(math.Floor(seg.Width / maxBeam))
	if lanes < 1 {
		lanes = 1
	}
	longi := int(math.Floor(seg.Length() / (safeSpacing * avgLen)))
	if longi < 1 {
		longi = 1
	}
	cap := lanes * longi
	if cap < 1 {
		cap = 1
	}
	return cap
}

// Congestion returns the occupancy ratio (0..>1) of a segment.
func Congestion(count, capacity int) float64 {
	if capacity <= 0 {
		capacity = 1
	}
	return float64(count) / float64(capacity)
}
