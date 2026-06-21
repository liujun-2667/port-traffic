-- Port traffic simulation initial schema
CREATE TABLE IF NOT EXISTS sim_runs (
    id BIGSERIAL PRIMARY KEY,
    params_json JSONB NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    duration_minutes INT NOT NULL,
    status TEXT NOT NULL DEFAULT 'running'
);
CREATE INDEX IF NOT EXISTS idx_sim_runs_started ON sim_runs(started_at DESC);

CREATE TABLE IF NOT EXISTS ship_trajectories (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL REFERENCES sim_runs(id) ON DELETE CASCADE,
    ship_id TEXT NOT NULL,
    sim_minute INT NOT NULL,
    x DOUBLE PRECISION NOT NULL,
    y DOUBLE PRECISION NOT NULL,
    state TEXT NOT NULL,
    speed DOUBLE PRECISION NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_traj_run_time ON ship_trajectories(run_id, sim_minute);
CREATE INDEX IF NOT EXISTS idx_traj_ship ON ship_trajectories(run_id, ship_id, sim_minute);

CREATE TABLE IF NOT EXISTS reports (
    run_id BIGINT PRIMARY KEY REFERENCES sim_runs(id) ON DELETE CASCADE,
    summary JSONB NOT NULL,
    metrics JSONB NOT NULL,
    events JSONB NOT NULL,
    bottlenecks JSONB NOT NULL,
    advice JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
