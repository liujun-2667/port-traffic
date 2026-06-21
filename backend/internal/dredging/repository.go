package dredging

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const dredgeSchemaSQL = `
CREATE TABLE IF NOT EXISTS channel_sediment (
    segment_id TEXT PRIMARY KEY,
    decay_rate DOUBLE PRECISION NOT NULL DEFAULT 0.05,
    last_dredged_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    current_effective_depth DOUBLE PRECISION NOT NULL DEFAULT 15.0,
    unit_dredging_cost DOUBLE PRECISION NOT NULL DEFAULT 8.0,
    restricted_draft DOUBLE PRECISION NOT NULL DEFAULT 10.0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS dredging_batches (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'planned',
    planned_start_date DATE NOT NULL,
    estimated_duration_days INT NOT NULL,
    target_depth DOUBLE PRECISION NOT NULL,
    total_cost DOUBLE PRECISION NOT NULL DEFAULT 0,
    actual_start_date DATE,
    actual_end_date DATE,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS dredging_batch_segments (
    id BIGSERIAL PRIMARY KEY,
    batch_id BIGINT NOT NULL REFERENCES dredging_batches(id) ON DELETE CASCADE,
    segment_id TEXT NOT NULL,
    original_depth DOUBLE PRECISION NOT NULL,
    segment_cost DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(batch_id, segment_id)
);

CREATE INDEX IF NOT EXISTS idx_dredging_batches_status ON dredging_batches(status);
CREATE INDEX IF NOT EXISTS idx_dredging_batches_dates ON dredging_batches(planned_start_date);
CREATE INDEX IF NOT EXISTS idx_dredging_batch_segments_batch ON dredging_batch_segments(batch_id);
CREATE INDEX IF NOT EXISTS idx_dredging_batch_segments_segment ON dredging_batch_segments(segment_id);
`

// Repository provides persistence for the dredging module.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a dredging repository from a pgx pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Migrate runs the dredging schema DDL.
func (r *Repository) Migrate(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, dredgeSchemaSQL)
	return err
}

// ---------- Channel Sediment ----------

// UpsertSediment inserts or updates a segment's sediment parameters.
func (r *Repository) UpsertSediment(ctx context.Context, s *ChannelSediment) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO channel_sediment
			(segment_id, decay_rate, last_dredged_at, current_effective_depth, unit_dredging_cost, restricted_draft, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6, now())
		ON CONFLICT (segment_id) DO UPDATE SET
			decay_rate = EXCLUDED.decay_rate,
			last_dredged_at = EXCLUDED.last_dredged_at,
			current_effective_depth = EXCLUDED.current_effective_depth,
			unit_dredging_cost = EXCLUDED.unit_dredging_cost,
			restricted_draft = EXCLUDED.restricted_draft,
			updated_at = now()`,
		s.SegmentID, s.DecayRate, s.LastDredgedAt, s.CurrentEffectiveDepth, s.UnitDredgingCost, s.RestrictedDraft)
	return err
}

// GetSediment fetches a single segment's parameters, or nil if missing.
func (r *Repository) GetSediment(ctx context.Context, segmentID string) (*ChannelSediment, error) {
	var s ChannelSediment
	err := r.pool.QueryRow(ctx, `
		SELECT segment_id, decay_rate, last_dredged_at, current_effective_depth, unit_dredging_cost, restricted_draft
		FROM channel_sediment WHERE segment_id=$1`, segmentID).
		Scan(&s.SegmentID, &s.DecayRate, &s.LastDredgedAt, &s.CurrentEffectiveDepth, &s.UnitDredgingCost, &s.RestrictedDraft)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ListSediments returns all stored sediment rows.
func (r *Repository) ListSediments(ctx context.Context) ([]ChannelSediment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT segment_id, decay_rate, last_dredged_at, current_effective_depth, unit_dredging_cost, restricted_draft
		FROM channel_sediment ORDER BY segment_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChannelSediment{}
	for rows.Next() {
		var s ChannelSediment
		if err := rows.Scan(&s.SegmentID, &s.DecayRate, &s.LastDredgedAt, &s.CurrentEffectiveDepth, &s.UnitDredgingCost, &s.RestrictedDraft); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// UpdateSediment partially updates a row; only non-nil fields are modified.
func (r *Repository) UpdateSediment(ctx context.Context, segmentID string, req *UpdateSedimentRequest) (*ChannelSediment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if req.DecayRate != nil {
		if _, err := tx.Exec(ctx, `UPDATE channel_sediment SET decay_rate=$1, updated_at=now() WHERE segment_id=$2`, *req.DecayRate, segmentID); err != nil {
			return nil, err
		}
	}
	if req.LastDredgedAt != nil {
		if _, err := tx.Exec(ctx, `UPDATE channel_sediment SET last_dredged_at=$1, updated_at=now() WHERE segment_id=$2`, *req.LastDredgedAt, segmentID); err != nil {
			return nil, err
		}
	}
	if req.CurrentEffectiveDepth != nil {
		if _, err := tx.Exec(ctx, `UPDATE channel_sediment SET current_effective_depth=$1, updated_at=now() WHERE segment_id=$2`, *req.CurrentEffectiveDepth, segmentID); err != nil {
			return nil, err
		}
	}
	if req.UnitDredgingCost != nil {
		if _, err := tx.Exec(ctx, `UPDATE channel_sediment SET unit_dredging_cost=$1, updated_at=now() WHERE segment_id=$2`, *req.UnitDredgingCost, segmentID); err != nil {
			return nil, err
		}
	}
	if req.RestrictedDraft != nil {
		if _, err := tx.Exec(ctx, `UPDATE channel_sediment SET restricted_draft=$1, updated_at=now() WHERE segment_id=$2`, *req.RestrictedDraft, segmentID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.GetSediment(ctx, segmentID)
}

// ---------- Dredging Batches ----------

// CreateBatch inserts a new batch with its associated segments (in a transaction).
func (r *Repository) CreateBatch(ctx context.Context, b *DredgingBatch) (int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO dredging_batches
			(name, status, planned_start_date, estimated_duration_days, target_depth, total_cost, notes)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		b.Name, string(b.Status), toDate(b.PlannedStartDate), b.EstimatedDurationDays, b.TargetDepth, b.TotalCost, b.Notes).Scan(&id)
	if err != nil {
		return 0, err
	}
	for _, seg := range b.Segments {
		if _, err := tx.Exec(ctx, `
			INSERT INTO dredging_batch_segments (batch_id, segment_id, original_depth, segment_cost)
			VALUES ($1,$2,$3,$4)`,
			id, seg.SegmentID, seg.OriginalDepth, seg.SegmentCost); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return id, nil
}

// UpdateBatchStatus changes the batch lifecycle state and sets actual start/end dates.
func (r *Repository) UpdateBatchStatus(ctx context.Context, id int64, status BatchStatus) error {
	now := time.Now()
	switch status {
	case BatchOngoing:
		_, err := r.pool.Exec(ctx, `UPDATE dredging_batches SET status=$1, actual_start_date=$2, updated_at=now() WHERE id=$3`,
			string(status), toDate(now), id)
		return err
	case BatchCompleted:
		_, err := r.pool.Exec(ctx, `UPDATE dredging_batches SET status=$1, actual_end_date=$2, updated_at=now() WHERE id=$3`,
			string(status), toDate(now), id)
		return err
	default:
		_, err := r.pool.Exec(ctx, `UPDATE dredging_batches SET status=$1, updated_at=now() WHERE id=$2`, string(status), id)
		return err
	}
}

// ListBatches returns all batches with segment details.
func (r *Repository) ListBatches(ctx context.Context) ([]DredgingBatch, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, status, planned_start_date, estimated_duration_days, target_depth,
		       total_cost, actual_start_date, actual_end_date, notes
		FROM dredging_batches ORDER BY planned_start_date DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DredgingBatch{}
	for rows.Next() {
		var b DredgingBatch
		var statusStr string
		var plannedStart time.Time
		var actualStart, actualEnd *time.Time
		if err := rows.Scan(&b.ID, &b.Name, &statusStr, &plannedStart, &b.EstimatedDurationDays,
			&b.TargetDepth, &b.TotalCost, &actualStart, &actualEnd, &b.Notes); err != nil {
			return nil, err
		}
		b.Status = BatchStatus(statusStr)
		b.PlannedStartDate = plannedStart
		b.ActualStartDate = actualStart
		b.ActualEndDate = actualEnd
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		segs, err := r.listBatchSegments(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Segments = segs
	}
	return out, nil
}

// GetBatch returns one batch with segments.
func (r *Repository) GetBatch(ctx context.Context, id int64) (*DredgingBatch, error) {
	var b DredgingBatch
	var statusStr string
	var plannedStart time.Time
	var actualStart, actualEnd *time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, status, planned_start_date, estimated_duration_days, target_depth,
		       total_cost, actual_start_date, actual_end_date, notes
		FROM dredging_batches WHERE id=$1`, id).
		Scan(&b.ID, &b.Name, &statusStr, &plannedStart, &b.EstimatedDurationDays,
			&b.TargetDepth, &b.TotalCost, &actualStart, &actualEnd, &b.Notes)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b.Status = BatchStatus(statusStr)
	b.PlannedStartDate = plannedStart
	b.ActualStartDate = actualStart
	b.ActualEndDate = actualEnd
	segs, err := r.listBatchSegments(ctx, id)
	if err != nil {
		return nil, err
	}
	b.Segments = segs
	return &b, nil
}

// DeleteBatch removes a batch (only if planned/ongoing).
func (r *Repository) DeleteBatch(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM dredging_batches WHERE id=$1`, id)
	return err
}

// ListBatchesByStatus returns batches filtered by status.
func (r *Repository) ListBatchesByStatus(ctx context.Context, status BatchStatus) ([]DredgingBatch, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, status, planned_start_date, estimated_duration_days, target_depth,
		       total_cost, actual_start_date, actual_end_date, notes
		FROM dredging_batches WHERE status=$1 ORDER BY planned_start_date DESC, id DESC`, string(status))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DredgingBatch{}
	for rows.Next() {
		var b DredgingBatch
		var statusStr string
		var plannedStart time.Time
		var actualStart, actualEnd *time.Time
		if err := rows.Scan(&b.ID, &b.Name, &statusStr, &plannedStart, &b.EstimatedDurationDays,
			&b.TargetDepth, &b.TotalCost, &actualStart, &actualEnd, &b.Notes); err != nil {
			return nil, err
		}
		b.Status = BatchStatus(statusStr)
		b.PlannedStartDate = plannedStart
		b.ActualStartDate = actualStart
		b.ActualEndDate = actualEnd
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		segs, err := r.listBatchSegments(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Segments = segs
	}
	return out, nil
}

// ListOngoingSegmentIDs returns the set of segment IDs currently being dredged (ongoing batches).
func (r *Repository) ListOngoingSegmentIDs(ctx context.Context) (map[string]bool, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT bs.segment_id
		FROM dredging_batch_segments bs
		JOIN dredging_batches b ON b.id = bs.batch_id
		WHERE b.status='ongoing'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// CompleteBatchAndUpdateSediment applies the depth update to each segment when a batch finishes.
func (r *Repository) CompleteBatchAndUpdateSediment(ctx context.Context, batchID int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	endDate := toDate(time.Now())
	if _, err := tx.Exec(ctx, `UPDATE dredging_batches SET status='completed', actual_end_date=$1, updated_at=now() WHERE id=$2`,
		endDate, batchID); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT segment_id FROM dredging_batch_segments WHERE batch_id=$1`, batchID)
	if err != nil {
		return err
	}
	var segIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		segIDs = append(segIDs, id)
	}
	rows.Close()

	b, err := r.GetBatch(ctx, batchID)
	if err != nil {
		return err
	}
	if b == nil {
		return nil
	}
	for _, sid := range segIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO channel_sediment (segment_id, current_effective_depth, last_dredged_at, updated_at)
			VALUES ($1, $2, $3, now())
			ON CONFLICT (segment_id) DO UPDATE SET
				current_effective_depth = EXCLUDED.current_effective_depth,
				last_dredged_at = EXCLUDED.last_dredged_at,
				updated_at = now()`,
			sid, b.TargetDepth, time.Now()); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *Repository) listBatchSegments(ctx context.Context, batchID int64) ([]BatchSegment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, batch_id, segment_id, original_depth, segment_cost
		FROM dredging_batch_segments WHERE batch_id=$1 ORDER BY segment_id`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BatchSegment{}
	for rows.Next() {
		var s BatchSegment
		if err := rows.Scan(&s.ID, &s.BatchID, &s.SegmentID, &s.OriginalDepth, &s.SegmentCost); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func toDate(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
