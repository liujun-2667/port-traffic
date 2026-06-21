// Package api exposes the REST + SSE HTTP API for the simulation.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"

	"port-traffic/internal/config"
	"port-traffic/internal/dredging"
	"port-traffic/internal/sensitivity"
	"port-traffic/internal/sim"
	"port-traffic/internal/store"
	"port-traffic/internal/tide"
)

// Server wires dependencies and routes.
type Server struct {
	cfg     *config.Service
	mgr     *Manager
	st      store.Store
	sens    *sensitivity.Runner
	dredge  *dredging.Service
}

// NewServer constructs the API server.
func NewServer(cfg *config.Service, mgr *Manager, st store.Store, sens *sensitivity.Runner, dr *dredging.Service) *Server {
	return &Server{cfg: cfg, mgr: mgr, st: st, sens: sens, dredge: dr}
}

// Router builds the chi router with all routes.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Requested-With", "Last-Event-ID"},
		ExposedHeaders:   []string{"Content-Type", "X-Request-Id"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	r.Get("/api/health", s.health)
	r.Get("/api/config", s.getConfig)
	r.Put("/api/config", s.putConfig)
	r.Get("/api/tide", s.getTide)
	r.Post("/api/sim/run", s.startRun)
	r.Post("/api/sim/{runId}/control", s.controlRun)
	r.Get("/api/sim/{runId}/state", s.stateRun)
	r.Get("/api/sim/{runId}/stream", s.streamRun)
	r.Post("/api/sensitivity/single", s.sensitivitySingle)
	r.Post("/api/sensitivity/dual", s.sensitivityDual)
	r.Get("/api/runs", s.listRuns)
	r.Get("/api/runs/{runId}", s.getRun)
	r.Get("/api/runs/{runId}/trajectory", s.getTrajectory)
	r.Get("/api/runs/{runId}/report", s.getReport)
	r.Get("/api/sim/{runId}/ship/{shipId}", s.getShipDetail)

	// Dredging module routes
	r.Get("/api/dredging/channels", s.listChannels)
	r.Get("/api/dredging/channels/{segmentId}", s.getSediment)
	r.Put("/api/dredging/channels/{segmentId}", s.updateSediment)
	r.Post("/api/dredging/cost-preview", s.costPreview)
	r.Post("/api/dredging/batches/check-conflicts", s.checkConflicts)
	r.Post("/api/dredging/batches", s.createBatch)
	r.Get("/api/dredging/batches", s.listBatches)
	r.Get("/api/dredging/batches/{batchId}", s.getBatch)
	r.Post("/api/dredging/batches/{batchId}/start", s.startBatch)
	r.Post("/api/dredging/batches/{batchId}/complete", s.completeBatch)
	r.Delete("/api/dredging/batches/{batchId}", s.deleteBatch)
	r.Post("/api/dredging/optimize", s.optimize)
	return r
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.Get())
}

type configUpdate struct {
	Sim     *config.SimConfig     `json:"sim"`
	Weather *config.WeatherConfig `json:"weather"`
}

func (s *Server) putConfig(w http.ResponseWriter, r *http.Request) {
	var u configUpdate
	if err := decodeJSON(r, &u); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.cfg.Update(u.Sim, u.Weather)
	writeJSON(w, http.StatusOK, s.cfg.Get())
}

func (s *Server) getTide(w http.ResponseWriter, r *http.Request) {
	hours := 24
	if v := r.URL.Query().Get("hours"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			hours = n
		}
	}
	cfg := s.cfg.Get()
	m := tide.New(cfg.Tide)
	writeJSON(w, http.StatusOK, map[string]any{
		"series":   m.Series(float64(hours), hours*4+1),
		"margin":   cfg.Tide.DraftMargin,
		"meanSeaLevel": cfg.Tide.MeanSeaLevel,
	})
}

func (s *Server) startRun(w http.ResponseWriter, r *http.Request) {
	var p sim.Params
	if err := decodeJSON(r, &p); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id, err := s.mgr.StartRun(p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runId": id})
}

type controlReq struct {
	Action string `json:"action"`
	Rate   int    `json:"rate"`
}

func (s *Server) controlRun(w http.ResponseWriter, r *http.Request) {
	id, ok := runID(w, r)
	if !ok {
		return
	}
	var c controlReq
	if err := decodeJSON(r, &c); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if s.mgr.Control(id, c.Action, c.Rate) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	} else {
		writeError(w, http.StatusNotFound, fmt.Errorf("run not found"))
	}
}

func (s *Server) stateRun(w http.ResponseWriter, r *http.Request) {
	id, ok := runID(w, r)
	if !ok {
		return
	}
	f, ok := s.mgr.State(id)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("run not found"))
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (s *Server) streamRun(w http.ResponseWriter, r *http.Request) {
	id, ok := runID(w, r)
	if !ok {
		return
	}

	// Always respond with SSE 200 so browser EventSource won't churn reconnects.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, _ := w.(http.Flusher)
	// send initial \n to flush headers through intermediaries
	fmt.Fprint(w, "\n\n")
	if flusher != nil {
		flusher.Flush()
	}

	ch, ok := s.mgr.Subscribe(id)
	if !ok {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", escapeJSON("run not found"))
		if flusher != nil {
			flusher.Flush()
		}
		return
	}
	defer s.mgr.Unsubscribe(id, ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case frame, open := <-ch:
			if !open {
				return
			}
			data, _ := json.Marshal(frame)
			fmt.Fprintf(w, "event: frame\ndata: %s\n\n", data)
			if flusher != nil {
				flusher.Flush()
			}
			if frame.Done {
				return
			}
		}
	}
}

// escapeJSON wraps a plain string so it's safe to embed as a JSON value token.
func escapeJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

type singleReq struct {
	Param string  `json:"param"`
	From  float64 `json:"from"`
	To    float64 `json:"to"`
	Step  float64 `json:"step"`
}

func (s *Server) sensitivitySingle(w http.ResponseWriter, r *http.Request) {
	var q singleReq
	if err := decodeJSON(r, &q); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	res, err := s.sens.Single(q.Param, q.From, q.To, q.Step)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type dualReq struct {
	ParamX string  `json:"paramX"`
	FromX  float64 `json:"fromX"`
	ToX    float64 `json:"toX"`
	StepX  float64 `json:"stepX"`
	ParamY string  `json:"paramY"`
	FromY  float64 `json:"fromY"`
	ToY    float64 `json:"toY"`
	StepY  float64 `json:"stepY"`
	Metric string  `json:"metric"`
}

func (s *Server) sensitivityDual(w http.ResponseWriter, r *http.Request) {
	var q dualReq
	if err := decodeJSON(r, &q); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if q.Metric == "" {
		q.Metric = "dangerous"
	}
	res, err := s.sens.Dual(q.ParamX, q.FromX, q.ToX, q.StepX, q.ParamY, q.FromY, q.ToY, q.StepY, q.Metric)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	if s.st == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	runs, err := s.st.ListRuns()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	id, ok := runID(w, r)
	if !ok {
		return
	}
	if s.mgr.HasRun(id) {
		if f, ok := s.mgr.State(id); ok {
			writeJSON(w, http.StatusOK, map[string]any{"runId": id, "live": true, "frame": f})
			return
		}
	}
	if s.st == nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("store unavailable"))
		return
	}
	rm, err := s.st.GetRun(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if rm == nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("run not found"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runId": id, "live": false, "meta": rm})
}

func (s *Server) getTrajectory(w http.ResponseWriter, r *http.Request) {
	id, ok := runID(w, r)
	if !ok {
		return
	}
	fromMin, toMin := -1, -1
	if v := r.URL.Query().Get("from"); v != "" {
		fromMin, _ = strconv.Atoi(v)
	}
	if v := r.URL.Query().Get("to"); v != "" {
		toMin, _ = strconv.Atoi(v)
	}
	if s.st == nil {
		writeJSON(w, http.StatusOK, []sim.TrajectoryRow{})
		return
	}
	rows, err := s.st.GetTrajectory(id, fromMin, toMin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) getReport(w http.ResponseWriter, r *http.Request) {
	id, ok := runID(w, r)
	if !ok {
		return
	}
	if s.mgr.HasRun(id) {
		if rep, ok := s.mgr.Report(id); ok {
			writeJSON(w, http.StatusOK, rep)
			return
		}
	}
	if s.st == nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("store unavailable"))
		return
	}
	rep, err := s.st.GetReport(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if rep == nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("report not found"))
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func runID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	v := chi.URLParam(r, "runId")
	id, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid runId"))
		return 0, false
	}
	return id, true
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) getShipDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := runID(w, r)
	if !ok {
		return
	}
	shipID := chi.URLParam(r, "shipId")
	if shipID == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("missing shipId"))
		return
	}
	detail, ok := s.mgr.ShipDetail(id, shipID)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("ship not found"))
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// ---------- Dredging module handlers ----------

func (s *Server) listChannels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	asOf := time.Now()
	if v := r.URL.Query().Get("date"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			asOf = t
		}
	}
	if s.dredge == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	out, err := s.dredge.ListChannelStatuses(ctx, asOf)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getSediment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sid := chi.URLParam(r, "segmentId")
	if sid == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("missing segmentId"))
		return
	}
	if s.dredge == nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("dredging service unavailable"))
		return
	}
	out, err := s.dredge.GetSediment(ctx, sid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if out == nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("segment not found"))
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) updateSediment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sid := chi.URLParam(r, "segmentId")
	if sid == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("missing segmentId"))
		return
	}
	var req dredging.UpdateSedimentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if s.dredge == nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("dredging service unavailable"))
		return
	}
	out, err := s.dredge.UpdateSediment(ctx, sid, &req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if out == nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("segment not found"))
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type costPreviewReq struct {
	SegmentIDs  []string `json:"segmentIds"`
	TargetDepth float64  `json:"targetDepth"`
}

func (s *Server) costPreview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req costPreviewReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if s.dredge == nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("dredging service unavailable"))
		return
	}
	out, err := s.dredge.ComputeCostPreview(ctx, req.SegmentIDs, req.TargetDepth)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type createBatchReq struct {
	dredging.CreateBatchRequest
	AllowConflict bool `json:"allowConflict"`
}

func (s *Server) createBatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createBatchReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if s.dredge == nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("dredging service unavailable"))
		return
	}
	b, err := s.dredge.CreateBatch(ctx, &req.CreateBatchRequest, req.AllowConflict)
	if err != nil {
		if ce, ok := err.(*dredging.ConflictError); ok {
			writeJSON(w, http.StatusConflict, ce)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

type checkConflictsReq struct {
	SegmentIDs            []string `json:"segmentIds"`
	PlannedStartDate      string   `json:"plannedStartDate"`
	EstimatedDurationDays int      `json:"estimatedDurationDays"`
}

func (s *Server) checkConflicts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req checkConflictsReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if s.dredge == nil {
		writeJSON(w, http.StatusServiceUnavailable, fmt.Errorf("dredging service unavailable"))
		return
	}
	startDate, err := time.Parse(time.RFC3339, req.PlannedStartDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid plannedStartDate"))
		return
	}
	conflicts, err := s.dredge.CheckConflicts(ctx, req.SegmentIDs, startDate, req.EstimatedDurationDays)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"conflicts": conflicts, "hasConflict": len(conflicts) > 0})
}

func (s *Server) listBatches(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if s.dredge == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	out, err := s.dredge.ListBatches(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getBatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := batchID(w, r)
	if !ok {
		return
	}
	if s.dredge == nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("dredging service unavailable"))
		return
	}
	b, err := s.dredge.GetBatch(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if b == nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("batch not found"))
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) startBatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := batchID(w, r)
	if !ok {
		return
	}
	if s.dredge == nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("dredging service unavailable"))
		return
	}
	if err := s.dredge.StartBatch(ctx, id); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) completeBatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := batchID(w, r)
	if !ok {
		return
	}
	if s.dredge == nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("dredging service unavailable"))
		return
	}
	if err := s.dredge.CompleteBatch(ctx, id); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) deleteBatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := batchID(w, r)
	if !ok {
		return
	}
	if s.dredge == nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("dredging service unavailable"))
		return
	}
	if err := s.dredge.DeleteBatch(ctx, id); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) optimize(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req dredging.OptimizeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if s.dredge == nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("dredging service unavailable"))
		return
	}
	out, err := s.dredge.Optimize(ctx, req.AnnualBudget)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func batchID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	v := chi.URLParam(r, "batchId")
	id, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid batchId"))
		return 0, false
	}
	return id, true
}
