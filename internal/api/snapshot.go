package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adata/reports/internal/auth"
	"github.com/adata/reports/internal/db"
	"github.com/adata/reports/internal/metrics"
)

// Снепшот — замороженная копия отчёта «Спринты» (ответ GET /sprints/report) на момент сохранения.
// Страница «Спринты» открывает его вместо живых данных со всеми вкладками; сохраняют и удаляют admin и lead.

type snapshotResp struct {
	Info     *db.SnapshotInfo `json:"info"`
	Snapshot json.RawMessage  `json:"snapshot"` // как GET /sprints/report
}

// GET /api/v1/snapshots — сохранённые снепшоты.
func snapshotsList(pool *pgxpool.Pool, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := db.ListSnapshots(r.Context(), pool)
		if err != nil {
			fail(w, log, "snapshots", err)
			return
		}
		writeJSON(w, http.StatusOK, list)
	}
}

var reDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func snapshotDate(w http.ResponseWriter, r *http.Request) (string, bool) {
	d := chi.URLParam(r, "date")
	if !reDate.MatchString(d) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "дата — ГГГГ-ММ-ДД"})
		return "", false
	}
	return d, true
}

// GET /api/v1/snapshots/{date} — сохранённый отчёт как есть, без пересчёта.
func snapshotGet(pool *pgxpool.Pool, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d, ok := snapshotDate(w, r)
		if !ok {
			return
		}
		b, info, err := db.GetSnapshot(r.Context(), pool, d)
		if err != nil {
			fail(w, log, "snapshot", err)
			return
		}
		if info == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "снепшота на эту дату нет"})
			return
		}
		writeJSON(w, http.StatusOK, snapshotResp{Info: info, Snapshot: b})
	}
}

// POST /api/v1/snapshots {"note": "..."} — сохранить текущий отчёт «Спринты» как снепшот на сегодня.
// Один на дату: повторный — 409 (чтобы пересохранить, старый удаляют).
func snapshotCreate(pool *pgxpool.Pool, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Note string `json:"note"`
		}
		if !readJSON(w, r, &body) { // только application/json — защита от CSRF через формы
			return
		}
		rep, err := loadReport(r, pool)
		if err != nil {
			fail(w, log, "snapshot create", err)
			return
		}
		if rep == nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Jira ещё не синхронизирована — сохранять нечего"})
			return
		}
		today := time.Now().In(metrics.Almaty).Format("2006-01-02")
		histEnd := rep.UpdatedAt.In(metrics.Almaty).Format("2006-01-02")
		u, _ := auth.FromContext(r.Context())
		err = db.SaveSnapshot(r.Context(), pool, today, histEnd, rep, body.Note, u.EmployeeID)
		if errors.Is(err, db.ErrSnapshotExists) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "снепшот на сегодня уже сохранён — удалите его, чтобы сохранить заново", "date": today})
			return
		}
		if err != nil {
			fail(w, log, "snapshot create", err)
			return
		}
		log.Info("snapshot: сохранён", "date", today, "user", u.Login)
		writeJSON(w, http.StatusCreated, map[string]string{"date": today})
	}
}

// DELETE /api/v1/snapshots/{date}
func snapshotDelete(pool *pgxpool.Pool, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d, ok := snapshotDate(w, r)
		if !ok {
			return
		}
		err := db.DeleteSnapshot(r.Context(), pool, d)
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "снепшота на эту дату нет"})
			return
		}
		if err != nil {
			fail(w, log, "snapshot delete", err)
			return
		}
		u, _ := auth.FromContext(r.Context())
		log.Info("snapshot: удалён", "date", d, "user", u.Login)
		w.WriteHeader(http.StatusNoContent)
	}
}
