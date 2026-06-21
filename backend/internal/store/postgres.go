// Package store persists simulation runs, trajectories and reports.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"port-traffic/internal/sim"
)

// RunMeta is a simulation run descriptor.
type RunMeta struct {
	ID              int64     `json:"id"`
	ParamsJSON      []byte    `json:"paramsJson"`
	StartedAt       time.Time `json:"startedAt"`
	DurationMinutes int       `json:"durationMinutes"`
	Status          string    `json:"status"`
}

// Store is the persistence interface.
type Store interface {
	Migrate() error
	SaveRun(paramsJSON []byte, durationMin int) (int64, error)
	UpdateStatus(id int64, status string) error
	SaveTrajectory(id int64, rows []sim.TrajectoryRow) error
	SaveReport(id int64, rep sim.Report) error
	ListRuns() ([]RunMeta, error)
	GetRun(id int64) (*RunMeta, error)
	GetTrajectory(id int64, fromMin, toMin int) ([]sim.TrajectoryRow, error)
	GetReport(id int64) (*sim.Report, error)
	Pool() any
	Close()
}

const schemaSQL = `
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
`

type pgStore struct {
	pool *pgxpool.Pool
}

// New creates a PostgreSQL store from a DSN.
func New(ctx context.Context, dsn string) (Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &pgStore{pool: pool}, nil
}

func (s *pgStore) Migrate() error {
	_, err := s.pool.Exec(context.Background(), schemaSQL)
	return err
}

func (s *pgStore) SaveRun(paramsJSON []byte, durationMin int) (int64, error) {
	var id int64
	err := s.pool.QueryRow(context.Background(),
		`INSERT INTO sim_runs (params_json, duration_minutes, status) VALUES ($1,$2,'running') RETURNING id`,
		paramsJSON, durationMin).Scan(&id)
	return id, err
}

func (s *pgStore) UpdateStatus(id int64, status string) error {
	_, err := s.pool.Exec(context.Background(),
		`UPDATE sim_runs SET status=$1 WHERE id=$2`, status, id)
	return err
}

func (s *pgStore) SaveTrajectory(id int64, rows []sim.TrajectoryRow) error {
	if len(rows) == 0 {
		return nil
	}
	ctx := context.Background()
	batch := make([][]any, 0, len(rows))
	for _, r := range rows {
		batch = append(batch, []any{id, r.ShipID, r.Minute, r.X, r.Y, r.State, r.Speed})
	}
	_, err := s.pool.CopyFrom(ctx,
		pgx.Identifier{"ship_trajectories"},
		[]string{"run_id", "ship_id", "sim_minute", "x", "y", "state", "speed"},
		pgx.CopyFromRows(batch))
	return err
}

func (s *pgStore) SaveReport(id int64, rep sim.Report) error {
	summary, _ := json.Marshal(rep.Summary)
	metrics, _ := json.Marshal(rep.Metrics)
	events, _ := json.Marshal(rep.Events)
	bn, _ := json.Marshal(rep.Bottlenecks)
	adv, _ := json.Marshal(rep.Advice)
	ctx := context.Background()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO reports (run_id, summary, metrics, events, bottlenecks, advice)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 ON CONFLICT (run_id) DO UPDATE SET summary=EXCLUDED.summary, metrics=EXCLUDED.metrics,
		   events=EXCLUDED.events, bottlenecks=EXCLUDED.bottlenecks, advice=EXCLUDED.advice, created_at=now()`,
		id, summary, metrics, events, bn, adv)
	return err
}

func (s *pgStore) ListRuns() ([]RunMeta, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT id, params_json, started_at, duration_minutes, status FROM sim_runs ORDER BY started_at DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RunMeta{}
	for rows.Next() {
		var r RunMeta
		if err := rows.Scan(&r.ID, &r.ParamsJSON, &r.StartedAt, &r.DurationMinutes, &r.Status); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *pgStore) GetRun(id int64) (*RunMeta, error) {
	var r RunMeta
	err := s.pool.QueryRow(context.Background(),
		`SELECT id, params_json, started_at, duration_minutes, status FROM sim_runs WHERE id=$1`, id).
		Scan(&r.ID, &r.ParamsJSON, &r.StartedAt, &r.DurationMinutes, &r.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &r, err
}

func (s *pgStore) GetTrajectory(id int64, fromMin, toMin int) ([]sim.TrajectoryRow, error) {
	q := `SELECT ship_id, sim_minute, x, y, state, speed FROM ship_trajectories WHERE run_id=$1`
	args := []any{id}
	if fromMin >= 0 && toMin > fromMin {
		q += ` AND sim_minute BETWEEN $2 AND $3`
		args = append(args, fromMin, toMin)
	}
	q += ` ORDER BY sim_minute, ship_id`
	rows, err := s.pool.Query(context.Background(), q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []sim.TrajectoryRow{}
	for rows.Next() {
		var r sim.TrajectoryRow
		if err := rows.Scan(&r.ShipID, &r.Minute, &r.X, &r.Y, &r.State, &r.Speed); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *pgStore) GetReport(id int64) (*sim.Report, error) {
	var summary, metrics, events, bn, adv []byte
	err := s.pool.QueryRow(context.Background(),
		`SELECT summary, metrics, events, bottlenecks, advice FROM reports WHERE run_id=$1`, id).
		Scan(&summary, &metrics, &events, &bn, &adv)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rep := &sim.Report{}
	if err := json.Unmarshal(summary, &rep.Summary); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(metrics, &rep.Metrics); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(events, &rep.Events); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(bn, &rep.Bottlenecks); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(adv, &rep.Advice); err != nil {
		return nil, err
	}
	return rep, nil
}

func (s *pgStore) Pool() any       { return s.pool }
func (s *pgStore) Close()         { s.pool.Close() }

// ErrNotFound is returned when an entity is missing.
var ErrNotFound = fmt.Errorf("not found")
