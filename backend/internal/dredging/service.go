package dredging

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"port-traffic/internal/config"
	"port-traffic/internal/model"
)

// Service is the business logic layer for the dredging module.
type Service struct {
	repo    *Repository
	cfgSvc  *config.Service
}

// NewService builds a dredging Service.
func NewService(repo *Repository, cfgSvc *config.Service) *Service {
	return &Service{repo: repo, cfgSvc: cfgSvc}
}

// Migrate runs DB schema setup.
func (s *Service) Migrate(ctx context.Context) error {
	return s.repo.Migrate(ctx)
}

// ---------- Sediment Parameter Management ----------

// EnsureDefaults initialises missing sediment rows from the port model segments.
func (s *Service) EnsureDefaults(ctx context.Context) error {
	cfg := s.cfgSvc.Get()
	stored, err := s.repo.ListSediments(ctx)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, r := range stored {
		have[r.SegmentID] = true
	}
	now := time.Now()
	for _, seg := range cfg.Port.Segments {
		if have[seg.ID] {
			continue
		}
		// Default decay 0.05 m/month, cost 8 万元/m/km, restricted draft = baseDepth/1.4
		defaultDecay := 0.05
		defaultUnitCost := 8.0
		restrictedDraft := math.Round(seg.BaseDepth/1.4*100) / 100
		if restrictedDraft <= 0 {
			restrictedDraft = 10.0
		}
		row := &ChannelSediment{
			SegmentID:            seg.ID,
			DecayRate:            defaultDecay,
			LastDredgedAt:        now,
			CurrentEffectiveDepth: seg.BaseDepth,
			UnitDredgingCost:     defaultUnitCost,
			RestrictedDraft:      restrictedDraft,
		}
		if err := s.repo.UpsertSediment(ctx, row); err != nil {
			return err
		}
	}
	return nil
}

// RefreshSedimentFromTime recalculates effective depth for all segments using decay from last_dredged_at to now.
func (s *Service) RefreshSedimentFromTime(ctx context.Context, simDate time.Time) error {
	rows, err := s.repo.ListSediments(ctx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		months := simDate.Sub(r.LastDredgedAt).Hours() / 24.0 / 30.0
		if months < 0 {
			months = 0
		}
		newDepth := r.CurrentEffectiveDepth
		// Always recompute the effective depth from base: we treat LastDredgedAt + decay as authoritative.
		seg, ok := s.segmentByID(r.SegmentID)
		base := seg.BaseDepth
		if !ok {
			base = r.CurrentEffectiveDepth
		}
		calc := base - months*r.DecayRate
		if calc < 0 {
			calc = 0
		}
		newDepth = math.Round(calc*100) / 100
		if math.Abs(newDepth-r.CurrentEffectiveDepth) > 0.001 {
			depth := newDepth
			if _, err := s.repo.UpdateSediment(ctx, r.SegmentID, &UpdateSedimentRequest{CurrentEffectiveDepth: &depth}); err != nil {
				return err
			}
		}
	}
	return nil
}

// ListChannelStatuses returns the runtime-computed view of every segment's siltation state.
func (s *Service) ListChannelStatuses(ctx context.Context, asOf time.Time) ([]ChannelStatus, error) {
	cfg := s.cfgSvc.Get()
	rows, err := s.repo.ListSediments(ctx)
	if err != nil {
		return nil, err
	}
	byID := map[string]*ChannelSediment{}
	for i := range rows {
		byID[rows[i].SegmentID] = &rows[i]
	}
	out := make([]ChannelStatus, 0, len(cfg.Port.Segments))
	for _, seg := range cfg.Port.Segments {
		sed, ok := byID[seg.ID]
		if !ok {
			// Default synthetic record
			sed = &ChannelSediment{
				SegmentID:            seg.ID,
				DecayRate:            0.05,
				LastDredgedAt:        asOf,
				CurrentEffectiveDepth: seg.BaseDepth,
				UnitDredgingCost:     8.0,
				RestrictedDraft:      math.Round(seg.BaseDepth/1.4*100) / 100,
			}
		}
		thresholdDepth := sed.RestrictedDraft * 1.3
		monthsSinceDredge := asOf.Sub(sed.LastDredgedAt).Hours() / 24.0 / 30.0
		if monthsSinceDredge < 0 {
			monthsSinceDredge = 0
		}
		// Recompute depth on the fly: decay is m/month, so depth reduces by DecayRate per month.
		effectiveDepth := seg.BaseDepth - monthsSinceDredge*sed.DecayRate
		if effectiveDepth < 0 {
			effectiveDepth = 0
		}
		effectiveDepth = math.Round(effectiveDepth*100) / 100
		// days_to_threshold = (effective - threshold) / (decay/30) * -1 if already below
		dailyDecay := sed.DecayRate / 30.0
		var daysToThreshold int
		if dailyDecay <= 0 {
			daysToThreshold = 9999
		} else {
			depthGap := effectiveDepth - thresholdDepth
			if depthGap <= 0 {
				daysToThreshold = 0
			} else {
				daysToThreshold = int(math.Ceil(depthGap / dailyDecay))
			}
		}
		status := StatusNormal
		switch {
		case effectiveDepth < thresholdDepth:
			status = StatusNeedsDredge
		case daysToThreshold < 90:
			status = StatusWarning
		}
		out = append(out, ChannelStatus{
			SegmentID:             seg.ID,
			CurrentEffectiveDepth: effectiveDepth,
			BaseDepth:             seg.BaseDepth,
			DecayRate:             sed.DecayRate,
			RestrictedDraft:       sed.RestrictedDraft,
			ThresholdDepth:        math.Round(thresholdDepth*100) / 100,
			DaysToThreshold:       daysToThreshold,
			Status:                status,
			LengthKm:              math.Round(seg.Length()/1000.0*100) / 100,
			UnitDredgingCost:      sed.UnitDredgingCost,
			LastDredgedAt:         sed.LastDredgedAt,
		})
	}
	return out, nil
}

// UpdateSediment patches per-segment siltation parameters via the API.
func (s *Service) UpdateSediment(ctx context.Context, segmentID string, req *UpdateSedimentRequest) (*ChannelSediment, error) {
	return s.repo.UpdateSediment(ctx, segmentID, req)
}

// GetSediment fetches a single row.
func (s *Service) GetSediment(ctx context.Context, segmentID string) (*ChannelSediment, error) {
	return s.repo.GetSediment(ctx, segmentID)
}

// ---------- Dredging Batch Management ----------

// ComputeCostPreview calculates costs before the user commits a batch.
func (s *Service) ComputeCostPreview(ctx context.Context, segmentIDs []string, targetDepth float64) (*CostPreview, error) {
	statuses, err := s.ListChannelStatuses(ctx, time.Now())
	if err != nil {
		return nil, err
	}
	byID := map[string]ChannelStatus{}
	for _, st := range statuses {
		byID[st.SegmentID] = st
	}
	total := 0.0
	items := make([]SegmentCostItem, 0, len(segmentIDs))
	for _, sid := range segmentIDs {
		st, ok := byID[sid]
		if !ok {
			return nil, fmt.Errorf("segment %s not found", sid)
		}
		delta := targetDepth - st.CurrentEffectiveDepth
		if delta < 0 {
			delta = 0
		}
		cost := delta * st.LengthKm * st.UnitDredgingCost
		cost = math.Round(cost*100) / 100
		total += cost
		items = append(items, SegmentCostItem{
			SegmentID:     sid,
			CurrentDepth:  st.CurrentEffectiveDepth,
			TargetDepth:   targetDepth,
			DepthIncrease: math.Round(delta*100) / 100,
			LengthKm:      st.LengthKm,
			UnitCost:      st.UnitDredgingCost,
			Cost:          cost,
		})
	}
	return &CostPreview{TotalCost: math.Round(total*100) / 100, PerSegment: items}, nil
}

// CreateBatch schedules a new dredging batch.
func (s *Service) CreateBatch(ctx context.Context, req *CreateBatchRequest) (*DredgingBatch, error) {
	if len(req.SegmentIDs) == 0 {
		return nil, fmt.Errorf("at least one segment required")
	}
	if req.TargetDepth <= 0 {
		return nil, fmt.Errorf("target depth must be positive")
	}
	if req.EstimatedDurationDays <= 0 {
		return nil, fmt.Errorf("estimated duration must be positive")
	}
	if req.Name == "" {
		return nil, fmt.Errorf("name required")
	}
	preview, err := s.ComputeCostPreview(ctx, req.SegmentIDs, req.TargetDepth)
	if err != nil {
		return nil, err
	}
	b := &DredgingBatch{
		Name:                  req.Name,
		Status:                BatchPlanned,
		PlannedStartDate:      req.PlannedStartDate,
		EstimatedDurationDays: req.EstimatedDurationDays,
		TargetDepth:           req.TargetDepth,
		TotalCost:             preview.TotalCost,
		Notes:                 req.Notes,
		Segments:              make([]BatchSegment, 0, len(preview.PerSegment)),
	}
	for _, it := range preview.PerSegment {
		b.Segments = append(b.Segments, BatchSegment{
			SegmentID:     it.SegmentID,
			OriginalDepth: it.CurrentDepth,
			SegmentCost:   it.Cost,
		})
	}
	id, err := s.repo.CreateBatch(ctx, b)
	if err != nil {
		return nil, err
	}
	b.ID = id
	return b, nil
}

// ListBatches returns all batches.
func (s *Service) ListBatches(ctx context.Context) ([]DredgingBatch, error) {
	return s.repo.ListBatches(ctx)
}

// GetBatch returns a single batch.
func (s *Service) GetBatch(ctx context.Context, id int64) (*DredgingBatch, error) {
	return s.repo.GetBatch(ctx, id)
}

// StartBatch transitions a batch from planned -> ongoing.
func (s *Service) StartBatch(ctx context.Context, id int64) error {
	return s.repo.UpdateBatchStatus(ctx, id, BatchOngoing)
}

// CompleteBatch finishes a batch and updates each segment's effective depth + last dredged.
func (s *Service) CompleteBatch(ctx context.Context, id int64) error {
	return s.repo.CompleteBatchAndUpdateSediment(ctx, id)
}

// DeleteBatch removes a batch (must be planned).
func (s *Service) DeleteBatch(ctx context.Context, id int64) error {
	b, err := s.repo.GetBatch(ctx, id)
	if err != nil {
		return err
	}
	if b == nil {
		return fmt.Errorf("batch not found")
	}
	if b.Status == BatchCompleted {
		return fmt.Errorf("cannot delete a completed batch")
	}
	return s.repo.DeleteBatch(ctx, id)
}

// ---------- Optimization ----------

// Optimize computes the greedy "urgency/cost" ranking within a budget.
// Target depth is assumed to be base depth for all segments (full restoration).
func (s *Service) Optimize(ctx context.Context, budget float64) (*OptimizeResult, error) {
	statuses, err := s.ListChannelStatuses(ctx, time.Now())
	if err != nil {
		return nil, err
	}
	// Build candidate list: any channel that isn't at full base depth.
	type candidate struct {
		st    ChannelStatus
		cost  float64
		urg   float64
		ratio float64
	}
	cands := []candidate{}
	for _, st := range statuses {
		delta := st.BaseDepth - st.CurrentEffectiveDepth
		if delta <= 0 {
			continue
		}
		cost := delta * st.LengthKm * st.UnitDredgingCost
		cost = math.Round(cost*100) / 100
		// Urgency = 1 / max(daysLeft, 1); assign a large but finite value if already over threshold.
		var urg float64
		if st.DaysToThreshold <= 0 {
			urg = 1.0 / 0.5 // weight already-over channels heavily
		} else {
			urg = 1.0 / float64(st.DaysToThreshold)
		}
		var ratio float64
		if cost <= 0 {
			ratio = urg * 1e6
		} else {
			ratio = urg / cost
		}
		cands = append(cands, candidate{st: st, cost: cost, urg: urg, ratio: ratio})
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].ratio != cands[j].ratio {
			return cands[i].ratio > cands[j].ratio
		}
		return cands[i].st.DaysToThreshold < cands[j].st.DaysToThreshold
	})
	result := OptimizeResult{Budget: budget, TotalSpent: 0, Recommendations: []RecommendItem{}}
	cumulative := 0.0
	for i, c := range cands {
		cumulative += c.cost
		over := cumulative > budget
		if !over {
			result.TotalSpent = math.Round(cumulative*100) / 100
		}
		result.Recommendations = append(result.Recommendations, RecommendItem{
			Rank:             i + 1,
			SegmentID:        c.st.SegmentID,
			Urgency:          math.Round(c.urg*10000) / 10000,
			DaysLeft:         c.st.DaysToThreshold,
			Cost:             c.cost,
			CumulativeCost:   math.Round(cumulative*100) / 100,
			OverBudget:       over,
			CurrentDepth:     c.st.CurrentEffectiveDepth,
			TargetDepth:      c.st.BaseDepth,
			UrgencyCostRatio: math.Round(c.ratio*10000) / 10000,
		})
	}
	return &result, nil
}

// ---------- Simulation Engine Integration ----------

// ActiveClosedSegments returns the set of segment IDs currently blocked by ongoing dredging.
// It also advances planned -> ongoing whose start date has arrived (best effort).
func (s *Service) ActiveClosedSegments(ctx context.Context, simStart time.Time) (map[string]bool, error) {
	// Auto-start planned batches whose planned_start_date <= simStart
	batches, err := s.repo.ListBatches(ctx)
	if err != nil {
		return nil, err
	}
	simStartDay := toDate(simStart)
	for _, b := range batches {
		if b.Status != BatchPlanned {
			continue
		}
		startDay := toDate(b.PlannedStartDate)
		if !startDay.After(simStartDay) {
			_ = s.repo.UpdateBatchStatus(ctx, b.ID, BatchOngoing)
		}
	}
	return s.repo.ListOngoingSegmentIDs(ctx)
}

func (s *Service) segmentByID(id string) (model.Segment, bool) {
	cfg := s.cfgSvc.Get()
	return cfg.Port.SegmentByID(id)
}
