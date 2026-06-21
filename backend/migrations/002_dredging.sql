-- Channel sediment and dredging planning schema

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
