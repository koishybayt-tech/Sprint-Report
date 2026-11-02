package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adata/reports/internal/auth"
	"github.com/adata/reports/internal/db"
	"github.com/adata/reports/internal/metrics"
)

// GET /api/v1/attendance — отчёт «Посещаемость» (ATT). Видимость по docs/security.md:
//   - admin, hr — все сотрудники и причины;
//   - lead — люди своих проектов (где он руководитель в Jira) и он сам; причины других скрыты;
//   - employee — только он сам.
func attendance(pool *pgxpool.Pool, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		rep, err := loadReport(r, pool)
		if err != nil {
			fail(w, log, "attendance", err)
			return
		}
		if rep == nil {
			rep = &metrics.SprintReport{Roles: map[string]string{}, Logins: map[string]string{}}
		}
		rows, err := db.AttendanceRows(ctx, pool)
		if err != nil {
			fail(w, log, "attendance", err)
			return
		}
		staff, err := db.StaffRows(ctx, pool)
		if err != nil {
			fail(w, log, "attendance", err)
			return
		}
		since := time.Now().AddDate(-1, 0, 0)
		abs, err := db.Absences(ctx, pool, since)
		if err != nil {
			fail(w, log, "attendance", err)
			return
		}
		tg, err := db.TelegramEvents(ctx, pool, since)
		if err != nil {
			fail(w, log, "attendance", err)
			return
		}
		abs = append(abs, metrics.TelegramAbsences(tg, since)...)
		att := metrics.BuildAttendance(rows, rep, abs, staff, time.Now())
		if syncs, err := db.LastSyncs(ctx, pool); err == nil {
			delete(syncs, "jira")
			if t, err := db.LastTelegramMessage(ctx, pool); err == nil && t != nil {
				syncs["telegram"] = *t
			}
			att.AbsAt = syncs
		}

		u, _ := auth.FromContext(ctx)
		self, err := selfName(r, pool, u)
		if err != nil {
			fail(w, log, "attendance", err)
			return
		}
		switch u.Role {
		case auth.RoleAdmin, auth.RoleHR:
		case auth.RoleLead:
			metrics.RestrictAttendance(&att, metrics.LeadScope(&att, self), self)
		default:
			metrics.RestrictAttendance(&att, map[string]bool{self: self != ""}, self)
		}
		writeJSON(w, http.StatusOK, att)
	}
}

// selfName — ФИО сотрудника, с которым связан пользователь сервиса ("" — не связан).
func selfName(r *http.Request, pool *pgxpool.Pool, u auth.User) (string, error) {
	if u.EmployeeID == nil {
		return "", nil
	}
	var n string
	err := pool.QueryRow(r.Context(), `SELECT full_name FROM employees WHERE id = $1`, *u.EmployeeID).Scan(&n)
	return n, err
}
