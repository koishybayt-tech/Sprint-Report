package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adata/reports/internal/db"
	"github.com/adata/reports/internal/metrics"
)

type sprintsData struct {
	updatedAt *time.Time
	projects  []metrics.ProjectSprint
}

// loadSprints считает строки проектов на момент последней успешной синхронизации Jira.
func loadSprints(r *http.Request, pool *pgxpool.Pool) (sprintsData, error) {
	ctx := r.Context()
	last, err := db.LastSync(ctx, pool, "jira")
	if err != nil || last == nil {
		return sprintsData{projects: []metrics.ProjectSprint{}}, err
	}
	issues, err := db.OpenSprintIssues(ctx, pool)
	if err != nil {
		return sprintsData{}, err
	}
	leads, err := db.ProjectLeads(ctx, pool)
	if err != nil {
		return sprintsData{}, err
	}
	return sprintsData{updatedAt: last, projects: metrics.ProjectSprints(issues, leads, *last)}, nil
}

// GET /api/v1/sprints?lead= — вкладка «Статус спринтов».
func sprints(pool *pgxpool.Pool, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d, err := loadSprints(r, pool)
		if err != nil {
			fail(w, log, "sprints", err)
			return
		}
		projects := d.projects
		if lead := r.URL.Query().Get("lead"); lead != "" {
			projects = projects[:0:0]
			for _, p := range d.projects {
				if p.Lead == lead {
					projects = append(projects, p)
				}
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"updated_at": d.updatedAt, "projects": projects})
	}
}

// GET /api/v1/sprints/next — следующий спринт по каждому проекту (номер +1 к текущему).
func nextSprints(pool *pgxpool.Pool, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d, err := loadSprints(r, pool)
		if err != nil {
			fail(w, log, "sprints/next", err)
			return
		}
		futures, err := db.FutureSprints(r.Context(), pool)
		if err != nil {
			fail(w, log, "sprints/next", err)
			return
		}
		inSprint, err := db.SprintIssueCounts(r.Context(), pool)
		if err != nil {
			fail(w, log, "sprints/next", err)
			return
		}
		out := []metrics.NextSprint{}
		for _, p := range d.projects {
			n := metrics.PickNext(p.Sprint, futures[p.Key])
			if n == nil {
				continue
			}
			name := n.Name
			out = append(out, metrics.NextSprint{ProjectKey: p.Key, Current: p.Sprint, NextID: &n.ID, NextName: &name,
				NextStart: dateStr(n.Start), NextEnd: dateStr(n.End), NextTotal: inSprint[n.ID]})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func dateStr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.In(metrics.Almaty).Format("2006-01-02")
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, log *slog.Logger, what string, err error) {
	log.Error(what, "err", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "внутренняя ошибка"})
}
