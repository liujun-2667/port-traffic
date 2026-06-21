// Command server runs the port traffic simulation API.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"port-traffic/internal/api"
	"port-traffic/internal/config"
	"port-traffic/internal/dredging"
	"port-traffic/internal/sensitivity"
	"port-traffic/internal/store"
)

func main() {
	cfgPath := envOr("CONFIG_PATH", "config/port.yaml")
	dsn := envOr("DATABASE_URL", "postgres://port:port@localhost:5432/porttraffic?sslmode=disable")
	addr := ":" + envOr("PORT", "8080")

	cfgSvc, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("config load: %v", err)
	}
	defer cfgSvc.Close()

	var st store.Store
	var drSvc *dredging.Service
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if s, err := store.New(ctx, dsn); err == nil {
		cancel()
		// 先赋值 st，再 Migrate——即使 Migrate 失败，pool 依然可用
		// dredging 模块需要 pool 来执行自己独立的 schema
		st = s
		if err := s.Migrate(); err != nil {
			log.Printf("store migrate: %v (sim persistence may be limited)", err)
		} else {
			log.Printf("store: connected, schema migrated")
		}
	} else {
		cancel()
		log.Printf("store: unavailable (%v) — running without persistence", err)
	}
	if st == nil {
		log.Printf("store: persistence disabled; replay/report-from-db unavailable")
	}

	// Initialise the dredging module if DB is available
	if st != nil {
		if pool, ok := st.Pool().(*pgxpool.Pool); ok {
			repo := dredging.NewRepository(pool)
			migCtx, migCancel := context.WithTimeout(context.Background(), 3*time.Second)
			if err := repo.Migrate(migCtx); err != nil {
				log.Printf("dredging: migrate error: %v", err)
			} else {
				log.Printf("dredging: schema migrated")
			}
			migCancel()
			svc := dredging.NewService(repo, cfgSvc)
			initCtx, initCancel := context.WithTimeout(context.Background(), 3*time.Second)
			_ = svc.EnsureDefaults(initCtx)
			initCancel()
			drSvc = svc
		}
	}
	// drSvc remains nil if DB unavailable; handlers degrade gracefully

	var mgr *api.Manager
	if drSvc != nil {
		mgr = api.NewManager(cfgSvc, st, drSvc)
	} else {
		mgr = api.NewManager(cfgSvc, st)
	}
	sens := sensitivity.New(cfgSvc.Get())
	srv := api.NewServer(cfgSvc, mgr, st, sens, drSvc)

	httpSrv := &http.Server{
		Addr:    addr,
		Handler: srv.Router(),
	}

	go func() {
		log.Printf("port-traffic API listening on %s", addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Printf("shutting down...")
	shCtx, shCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shCancel()
	_ = httpSrv.Shutdown(shCtx)
	if st != nil {
		st.Close()
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
