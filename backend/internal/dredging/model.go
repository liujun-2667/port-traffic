// Package dredging implements the channel maintenance planning module.
package dredging

import "time"

// SedimentStatus enumerates channel sedimentation states.
type SedimentStatus string

const (
	StatusNormal      SedimentStatus = "normal"
	StatusWarning     SedimentStatus = "warning"
	StatusNeedsDredge SedimentStatus = "needs_dredge"
)

// BatchStatus enumerates dredging batch lifecycle states.
type BatchStatus string

const (
	BatchPlanned   BatchStatus = "planned"
	BatchOngoing   BatchStatus = "ongoing"
	BatchCompleted BatchStatus = "completed"
)

// ChannelSediment holds per-segment siltation parameters.
type ChannelSediment struct {
	SegmentID          string    `json:"segmentId"`
	DecayRate          float64   `json:"decayRate"`
	LastDredgedAt      time.Time `json:"lastDredgedAt"`
	CurrentEffectiveDepth float64 `json:"currentEffectiveDepth"`
	UnitDredgingCost   float64   `json:"unitDredgingCost"`
	RestrictedDraft    float64   `json:"restrictedDraft"`
}

// ChannelStatus is a runtime-computed view of a channel segment.
type ChannelStatus struct {
	SegmentID              string         `json:"segmentId"`
	CurrentEffectiveDepth  float64        `json:"currentEffectiveDepth"`
	BaseDepth              float64        `json:"baseDepth"`
	DecayRate              float64        `json:"decayRate"`
	RestrictedDraft        float64        `json:"restrictedDraft"`
	ThresholdDepth         float64        `json:"thresholdDepth"`
	DaysToThreshold        int            `json:"daysToThreshold"`
	Status                 SedimentStatus `json:"status"`
	LengthKm               float64        `json:"lengthKm"`
	UnitDredgingCost       float64        `json:"unitDredgingCost"`
	LastDredgedAt          time.Time      `json:"lastDredgedAt"`
}

// DredgingBatch is a scheduled dredging operation covering one or more segments.
type DredgingBatch struct {
	ID                     int64           `json:"id"`
	Name                   string          `json:"name"`
	Status                 BatchStatus     `json:"status"`
	PlannedStartDate       time.Time       `json:"plannedStartDate"`
	EstimatedDurationDays  int             `json:"estimatedDurationDays"`
	TargetDepth            float64         `json:"targetDepth"`
	TotalCost              float64         `json:"totalCost"`
	ActualStartDate        *time.Time      `json:"actualStartDate,omitempty"`
	ActualEndDate          *time.Time      `json:"actualEndDate,omitempty"`
	Notes                  string          `json:"notes"`
	Segments               []BatchSegment  `json:"segments"`
}

// BatchSegment links a segment to a batch with per-segment cost details.
type BatchSegment struct {
	ID            int64   `json:"id"`
	BatchID       int64   `json:"batchId"`
	SegmentID     string  `json:"segmentId"`
	OriginalDepth float64 `json:"originalDepth"`
	SegmentCost   float64 `json:"segmentCost"`
}

// CreateBatchRequest is the input payload for scheduling a batch.
type CreateBatchRequest struct {
	Name                  string    `json:"name"`
	SegmentIDs            []string  `json:"segmentIds"`
	PlannedStartDate      time.Time `json:"plannedStartDate"`
	EstimatedDurationDays int       `json:"estimatedDurationDays"`
	TargetDepth           float64   `json:"targetDepth"`
	Notes                 string    `json:"notes"`
}

// CostPreview is the real-time cost breakdown shown while editing a batch.
type CostPreview struct {
	TotalCost   float64            `json:"totalCost"`
	PerSegment  []SegmentCostItem  `json:"perSegment"`
}

type SegmentCostItem struct {
	SegmentID       string  `json:"segmentId"`
	CurrentDepth    float64 `json:"currentDepth"`
	TargetDepth     float64 `json:"targetDepth"`
	DepthIncrease   float64 `json:"depthIncrease"`
	LengthKm        float64 `json:"lengthKm"`
	UnitCost        float64 `json:"unitCost"`
	Cost            float64 `json:"cost"`
}

// OptimizeRequest asks for budget-limited dredging recommendations.
type OptimizeRequest struct {
	AnnualBudget float64 `json:"annualBudget"`
}

// OptimizeResult is the greedy-strategy recommendation list.
type OptimizeResult struct {
	Budget       float64              `json:"budget"`
	TotalSpent   float64              `json:"totalSpent"`
	Recommendations []RecommendItem `json:"recommendations"`
}

type RecommendItem struct {
	Rank          int     `json:"rank"`
	SegmentID     string  `json:"segmentId"`
	Urgency       float64 `json:"urgency"`
	DaysLeft      int     `json:"daysLeft"`
	Cost          float64 `json:"cost"`
	CumulativeCost float64 `json:"cumulativeCost"`
	OverBudget    bool    `json:"overBudget"`
	CurrentDepth  float64 `json:"currentDepth"`
	TargetDepth   float64 `json:"targetDepth"`
	UrgencyCostRatio float64 `json:"urgencyCostRatio"`
}

// UpdateSedimentRequest allows hot-updating per-segment siltation parameters.
type UpdateSedimentRequest struct {
	DecayRate        *float64   `json:"decayRate,omitempty"`
	LastDredgedAt    *time.Time `json:"lastDredgedAt,omitempty"`
	CurrentEffectiveDepth *float64 `json:"currentEffectiveDepth,omitempty"`
	UnitDredgingCost *float64   `json:"unitDredgingCost,omitempty"`
	RestrictedDraft  *float64   `json:"restrictedDraft,omitempty"`
}

// BatchConflict describes an overlapping batch for conflict detection.
type BatchConflict struct {
	BatchID      int64  `json:"batchId"`
	BatchName    string `json:"batchName"`
	SegmentID    string `json:"segmentId"`
	OverlapDays  int    `json:"overlapDays"`
	ExistingStart time.Time `json:"existingStart"`
	ExistingEnd   time.Time `json:"existingEnd"`
	Status       BatchStatus `json:"status"`
}

// ConflictError is returned when batch scheduling conflicts are detected.
type ConflictError struct {
	Conflicts []BatchConflict `json:"conflicts"`
	Message   string          `json:"message"`
}

func (e *ConflictError) Error() string {
	return e.Message
}
