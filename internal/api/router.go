// Package api — HTTP-хендлеры. Пока только служебные; остальное придёт из api/openapi.yaml (oapi-codegen).
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewRouter(pool *pgxpool.Pool, log *slog.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/healthz", healthz(pool, log))
		r.Get("/sprints", sprints(pool, log))
		r.Get("/sprints/next", nextSprints(pool, log))
		r.Get("/sprints/report", sprintReport(pool, log))
	})
	return r
}

// healthz отвечает 200 {"status":"ok","db":"ok"}, если база отвечает, иначе 503.
func healthz(pool *pgxpool.Pool, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		status, code := map[string]string{"status": "ok", "db": "ok"}, http.StatusOK
		if err := pool.Ping(ctx); err != nil {
			log.Warn("healthz: база недоступна", "err", err)
			status, code = map[string]string{"status": "error", "db": "unavailable"}, http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(status)
	}
}
