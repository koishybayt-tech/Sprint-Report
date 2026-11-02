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

	"github.com/adata/reports/internal/auth"
)

// NewRouter. Без входа доступны только /healthz и /auth/*; остальное требует сессию, а права — по роли (docs/security.md).
func NewRouter(pool *pgxpool.Pool, log *slog.Logger) http.Handler {
	ah := &authHandlers{users: auth.NewStore(pool), limiter: auth.NewLimiter(5, 10*time.Minute), log: log}

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/healthz", healthz(pool, log))
		r.Post("/auth/login", ah.login)
		r.Post("/auth/logout", ah.logout)

		r.Group(func(r chi.Router) {
			r.Use(ah.requireUser)
			r.Get("/me", ah.me)
			r.Post("/me/password", ah.changeOwnPassword)

			r.Get("/attendance", attendance(pool, log)) // видимость — по роли внутри
			empCache := &employeesCache{}
			r.Get("/employees", employeesList(pool, log, empCache))
			r.Get("/employees/{id}", employeeCard(pool, log, empCache))

			// Спринты: все, кроме hr.
			r.Group(func(r chi.Router) {
				r.Use(requireRole(auth.RoleAdmin, auth.RoleLead, auth.RoleEmployee))
				r.Get("/sprints", sprints(pool, log))
				r.Get("/sprints/next", nextSprints(pool, log))
				r.Get("/sprints/report", sprintReport(pool, log))
			})

			// Снепшоты к созвонам: admin и lead (docs/security.md).
			r.Route("/snapshots", func(r chi.Router) {
				r.Use(requireRole(auth.RoleAdmin, auth.RoleLead))
				r.Get("/", snapshotsList(pool, log))
				r.Post("/", snapshotCreate(pool, log))
				r.Get("/live", snapshotLive(pool, log))
				r.Get("/{date}", snapshotGet(pool, log))
			})

			// Очередь «Кто это?» (Telegram → сотрудник): admin и hr.
			r.Route("/admin/tg", func(r chi.Router) {
				r.Use(requireRole(auth.RoleAdmin, auth.RoleHR))
				r.Get("/unknown", tgUnknown(pool, log))
				r.Post("/aliases", tgMapAuthor(pool, log))
			})

			// Управление пользователями: только admin.
			r.Route("/admin/users", func(r chi.Router) {
				r.Use(requireRole(auth.RoleAdmin))
				r.Get("/", ah.listUsers)
				r.Post("/", ah.createUser)
				r.Patch("/{id}", ah.updateUser)
				r.Post("/{id}/password", ah.resetPassword)
			})
		})
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
