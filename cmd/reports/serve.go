package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/adata/reports/internal/api"
	"github.com/adata/reports/internal/config"
	"github.com/adata/reports/internal/db"
	"github.com/adata/reports/internal/jira"
)

func runServe(ctx context.Context, log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.NewRouter(pool, log),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("serve: слушаю", "addr", cfg.HTTPAddr)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("serve: остановка")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// runWorker синхронизирует Jira при старте и затем каждые JIRA_SYNC_EVERY (по умолчанию 15m).
// Без JIRA_PAT синхронизация выключена, worker только пишет в лог, что жив.
func runWorker(ctx context.Context, log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	client, err := jira.New(cfg.JiraBaseURL, cfg.JiraPAT)
	if err != nil {
		log.Warn("worker: синхронизация Jira выключена", "reason", err.Error())
	}
	log.Info("worker: запущен", "jira_sync_every", cfg.JiraSyncEvery.String())
	runJira := func() {
		if client == nil {
			return
		}
		if err := syncJira(ctx, log, cfg, client, pool); err != nil && ctx.Err() == nil {
			log.Error("worker: синхронизация Jira", "err", err)
		}
	}
	runJira()

	heartbeat := time.NewTicker(time.Minute)
	defer heartbeat.Stop()
	jiraTick := time.NewTicker(cfg.JiraSyncEvery)
	defer jiraTick.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info("worker: остановка")
			return nil
		case <-heartbeat.C:
			log.Info("worker: жив")
		case <-jiraTick.C:
			runJira()
		}
	}
}

func runMigrate(ctx context.Context, log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}
	log.Info("migrate: готово")
	return nil
}

// runHealthcheck дёргает healthz локального serve; используется в HEALTHCHECK (в distroless нет curl).
func runHealthcheck(ctx context.Context) error {
	port := "8080"
	if addr := os.Getenv("HTTP_ADDR"); len(addr) > 1 && addr[0] == ':' {
		port = addr[1:]
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+port+"/api/v1/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz: HTTP %d", resp.StatusCode)
	}
	return nil
}
