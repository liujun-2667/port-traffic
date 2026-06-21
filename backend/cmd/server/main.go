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

	"port-traffic/internal/api"
	"port-traffic/internal/config"
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if s, err := store.New(ctx, dsn); err == nil {
		cancel()
		if err := s.Migrate(); err != nil {
			log.Printf("store migrate: %v (persistence disabled)", err)
		} else {
			st = s
			log.Printf("store: connected, schema migrated")
		}
	} else {
		cancel()
		log.Printf("store: unavailable (%v) — running without persistence", err)
	}
	if st == nil {
		log.Printf("store: persistence disabled; replay/report-from-db unavailable")
	}

	mgr := api.NewManager(cfgSvc, st)
	sens := sensitivity.New(cfgSvc.Get())
	srv := api.NewServer(cfgSvc, mgr, st, sens)

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
