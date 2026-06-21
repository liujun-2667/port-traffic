// Package sensitivity runs batch parameter sweeps over the simulation.
package sensitivity

import (
	"fmt"
	"math"

	"port-traffic/internal/config"
	"port-traffic/internal/sim"
)

const replicas = 3

// Runner executes sweeps.
type Runner struct {
	cfg *config.Config
}

// New creates a Runner.
func New(cfg *config.Config) *Runner { return &Runner{cfg: cfg} }

// SinglePoint is one averaged metric sample in a single-param sweep.
type SinglePoint struct {
	Param        string  `json:"param"`
	Value        float64 `json:"value"`
	Dangerous    float64 `json:"dangerous"`
	Warning      float64 `json:"warning"`
	AvgWait      float64 `json:"avgWait"`
	Throughput   float64 `json:"throughput"`
	Congestion   float64 `json:"congestion"`
}

// Single runs a single-parameter sweep and returns averaged metrics.
func (r *Runner) Single(param string, from, to, step float64) ([]SinglePoint, error) {
	if err := validate(param); err != nil {
		return nil, err
	}
	vals := steps(from, to, step)
	out := make([]SinglePoint, 0, len(vals))
	for _, v := range vals {
		acc := metricsAcc{}
		for seed := int64(1); seed <= replicas; seed++ {
			rep := r.run(applyParam(sim.Params{Seed: seed}, param, v))
			acc.add(rep)
		}
		acc.norm(replicas)
		out = append(out, SinglePoint{
			Param: param, Value: v,
			Dangerous: acc.dangerous, Warning: acc.warning,
			AvgWait: acc.avgWait, Throughput: acc.throughput, Congestion: acc.congestion,
		})
	}
	return out, nil
}

// DualPoint is a 2D sweep result containing a heatmap matrix.
type DualResult struct {
	ParamX string      `json:"paramX"`
	ParamY string      `json:"paramY"`
	X      []float64   `json:"x"`
	Y      []float64   `json:"y"`
	Matrix [][]float64 `json:"matrix"` // [y][x]
	Metric string      `json:"metric"`
}

// Dual runs a two-parameter sweep returning a heatmap of the chosen metric.
func (r *Runner) Dual(paramX string, fromX, toX, stepX float64,
	paramY string, fromY, toY, stepY float64, metric string) (*DualResult, error) {
	if err := validate(paramX); err != nil {
		return nil, err
	}
	if err := validate(paramY); err != nil {
		return nil, err
	}
	xs := steps(fromX, toX, stepX)
	ys := steps(fromY, toY, stepY)
	mat := make([][]float64, len(ys))
	for iy, yv := range ys {
		row := make([]float64, len(xs))
		for ix, xv := range xs {
			acc := metricsAcc{}
			for seed := int64(1); seed <= replicas; seed++ {
				p := applyParam(sim.Params{Seed: seed}, paramX, xv)
				p = applyParam(p, paramY, yv)
				rep := r.run(p)
				acc.add(rep)
			}
			acc.norm(replicas)
			row[ix] = pickMetric(acc, metric)
		}
		mat[iy] = row
	}
	return &DualResult{ParamX: paramX, ParamY: paramY, X: xs, Y: ys, Matrix: mat, Metric: metric}, nil
}

func (r *Runner) run(p sim.Params) sim.Report {
	e := sim.NewEngine(r.cfg, p)
	for !e.Step() {
	}
	return e.Report()
}

func applyParam(p sim.Params, param string, v float64) sim.Params {
	switch param {
	case "arrivalRate":
		p.ArrivalRate = v
	case "speedLimit":
		p.SpeedLimitScale = v
	case "windSpeed":
		p.WindSpeed = v
	case "visibility":
		p.Visibility = v
	case "durationHours":
		p.DurationHours = int(v)
	}
	return p
}

func validate(param string) error {
	switch param {
	case "arrivalRate", "speedLimit", "windSpeed", "visibility", "durationHours":
		return nil
	}
	return fmt.Errorf("unsupported sweep parameter: %s", param)
}

type metricsAcc struct {
	dangerous, warning, avgWait, throughput, congestion float64
}

func (a *metricsAcc) add(rep sim.Report) {
	a.dangerous += float64(rep.Metrics.DangerousEncounters)
	a.warning += float64(rep.Metrics.CollisionWarnings)
	a.avgWait += rep.Metrics.AvgWaitMinutes
	a.throughput += float64(rep.Metrics.TotalThroughput)
	a.congestion += avgSegCong(rep.Metrics.SegmentCongestion)
}

func (a *metricsAcc) norm(n int) {
	d := float64(n)
	a.dangerous /= d
	a.warning /= d
	a.avgWait /= d
	a.throughput /= d
	a.congestion /= d
}

func pickMetric(a metricsAcc, metric string) float64 {
	switch metric {
	case "warning":
		return a.warning
	case "avgWait":
		return a.avgWait
	case "throughput":
		return a.throughput
	case "congestion":
		return a.congestion
	default:
		return a.dangerous
	}
}

func avgSegCong(segs []sim.SegCongAvg) float64 {
	if len(segs) == 0 {
		return 0
	}
	s := 0.0
	for _, sc := range segs {
		s += sc.AvgCongestion
	}
	return s / float64(len(segs))
}

func steps(from, to, step float64) []float64 {
	if step == 0 {
		return []float64{from}
	}
	out := []float64{}
	for v := from; v <= to+step*0.5; v += step {
		out = append(out, round(v))
	}
	return out
}

func round(v float64) float64 {
	return math.Round(v*1000) / 1000
}
