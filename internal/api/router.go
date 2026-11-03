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

	"github.com/adata/reports/internal/api/lcapi"
	"github.com/adata/reports/internal/auth"
	"github.com/adata/reports/internal/config"
	"github.com/adata/reports/internal/db"
	"github.com/adata/reports/internal/notify"
)

// NewRouter. Без входа доступны только /healthz и /auth/*; остальное требует сессию, а права — по роли (docs/security.md).
func NewRouter(pool *pgxpool.Pool, log *slog.Logger, cfg config.Config) http.Handler {
	nt := notify.New(pool, cfg.SecretsKey, cfg.JiraBaseURL, log)
	nt.Rebuild = func(ctx context.Context, lead string) error { return db.RebuildLead(ctx, pool, lead, cfg.JiraBaseURL) }
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

			r.With(requireModule(auth.ModAttendance)).Get("/attendance", attendance(pool, log)) // видимость — по роли внутри
			// Колокольчик — свои уведомления у любого пользователя.
			r.Get("/notifications", notifyList(nt, log))
			r.Post("/notifications/read", notifyRead(nt, log))
			empCache := &employeesCache{}
			r.With(requireModule(auth.ModEmployees)).Get("/employees", employeesList(pool, log, empCache))
			r.With(requireModule(auth.ModEmployees)).Get("/employees/{id}", employeeCard(pool, log, empCache))

			// Спринты: все, кроме hr.
			r.Group(func(r chi.Router) {
				r.Use(requireRole(auth.RoleAdmin, auth.RoleLead, auth.RoleEmployee), requireModule(auth.ModSprints))
				r.Get("/sprints", sprints(pool, log))
				r.Get("/sprints/next", nextSprints(pool, log))
				r.Get("/sprints/report", sprintReport(pool, log))
			})

			// Контроль спринта руководителей: admin и lead; хендлеры — по сгенерированному lcapi (api/openapi.yaml).
			r.Group(func(r chi.Router) {
				r.Use(requireLeadAccess, requireModule(auth.ModLeadControl))
				lcapi.HandlerFromMux(&leadControlServer{pool: pool, log: log, jiraBase: cfg.JiraBaseURL, notify: nt}, r)

				// Уведомления руководителя: какие события присылать и личный Telegram-бот.
				r.Get("/notifications/settings", notifySettings(nt, log))
				r.Put("/notifications/settings", notifySaveSettings(nt, log))
				r.Post("/notifications/telegram", notifyConnect(nt, log))
				r.Delete("/notifications/telegram", notifyDisconnect(nt, log))
				r.Post("/notifications/test", notifyTest(nt, log))
			})

			// Снепшоты — замороженные копии отчёта «Спринты»: смотрят все, кому видны «Спринты» (не hr);
			// сохраняют и удаляют admin и lead.
			r.Route("/snapshots", func(r chi.Router) {
				r.Use(requireRole(auth.RoleAdmin, auth.RoleLead, auth.RoleEmployee), requireModule(auth.ModSprints))
				r.Get("/", snapshotsList(pool, log))
				r.Get("/{date}", snapshotGet(pool, log))
				r.Group(func(r chi.Router) {
					r.Use(requireRole(auth.RoleAdmin, auth.RoleLead))
					r.Post("/", snapshotCreate(pool, log))
					r.Delete("/{date}", snapshotDelete(pool, log))
				})
			})

			// Очередь «Кто это?» (Telegram → сотрудник): admin и hr.
			r.Route("/admin/tg", func(r chi.Router) {
				r.Use(requireRole(auth.RoleAdmin, auth.RoleHR), requireModule(auth.ModAttendance))
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

			// Справочник «руководитель → проекты» (Контроль спринта руководителей): только admin.
			r.Route("/admin/lead-projects", func(r chi.Router) {
				r.Use(requireRole(auth.RoleAdmin))
				r.Get("/", leadProjectsList(pool, log))
				r.Post("/", leadProjectsSave(pool, log))
				r.Delete("/", leadProjectsDelete(pool, log))
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
