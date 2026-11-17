package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adata/reports/internal/db"
	"github.com/adata/reports/internal/metrics"
)

// loadReport считает отчёт «Спринты» на момент последней успешной синхронизации Jira (nil — синхронизаций не было).
func loadReport(r *http.Request, pool *pgxpool.Pool) (*metrics.SprintReport, error) {
	ctx := r.Context()
	last, err := db.LastSync(ctx, pool, "jira")
	if err != nil || last == nil {
		return nil, err
	}
	issues, err := db.OpenSprintIssues(ctx, pool)
	if err != nil {
		return nil, err
	}
	leads, err := db.ProjectLeads(ctx, pool)
	if err != nil {
		return nil, err
	}
	tl, err := db.TeamLeads(ctx, pool)
	if err != nil {
		return nil, err
	}
	futures, err := db.FutureSprints(ctx, pool)
	if err != nil {
		return nil, err
	}
	si, err := db.SprintIssues(ctx, pool)
	if err != nil {
		return nil, err
	}
	rep := metrics.BuildSprintReport(issues, leads, tl, futures, si, *last)

	// Отсутствия, не закончившиеся к дате среза: отпуска Bitrix, затем больничные и «отпросился» из Telegram
	// (порядок важен — Telegram, пересекающийся с отпуском, не дублируется).
	local := last.In(metrics.Almaty)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	abs, err := db.Absences(ctx, pool, day)
	if err != nil {
		return nil, err
	}
	tg, err := db.TelegramEvents(ctx, pool, day.AddDate(0, 0, -31)) // с запасом, чтобы видеть начало длинного больничного
	if err != nil {
		return nil, err
	}
	abs = append(abs, metrics.TelegramAbsences(tg, day)...)
	syncs, err := db.LastSyncs(ctx, pool)
	if err != nil {
		return nil, err
	}
	delete(syncs, "jira")
	if t, err := db.LastTelegramMessage(ctx, pool); err != nil {
		return nil, err
	} else if t != nil {
		syncs["telegram"] = *t
	}
	metrics.AttachAbsences(&rep, abs, syncs)
	return &rep, nil
}

// GET /api/v1/sprints/report — все данные отчёта «Спринты»; вкладки считаются на клиенте, как в page_template.html.
func sprintReport(pool *pgxpool.Pool, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rep, err := loadReport(r, pool)
		if err != nil {
			fail(w, log, "sprints/report", err)
			return
		}
		if rep == nil {
			writeJSON(w, http.StatusOK, map[string]any{"updated_at": nil})
			return
		}
		writeJSON(w, http.StatusOK, rep)
	}
}

// GET /api/v1/sprints?lead= — вкладка «Статус спринтов».
func sprints(pool *pgxpool.Pool, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rep, err := loadReport(r, pool)
		if err != nil {
			fail(w, log, "sprints", err)
			return
		}
		if rep == nil {
			writeJSON(w, http.StatusOK, map[string]any{"updated_at": nil, "projects": []metrics.ProjectSprint{}})
			return
		}
		projects := rep.Projects
		if lead := r.URL.Query().Get("lead"); lead != "" {
			projects = projects[:0:0]
			for _, p := range rep.Projects {
				if p.Lead == lead {
					projects = append(projects, p)
				}
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"updated_at": rep.UpdatedAt, "projects": projects})
	}
}

// GET /api/v1/sprints/next — следующий спринт по каждому проекту (номер +1 к текущему).
func nextSprints(pool *pgxpool.Pool, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rep, err := loadReport(r, pool)
		if err != nil {
			fail(w, log, "sprints/next", err)
			return
		}
		out := []metrics.NextSprint{}
		if rep != nil {
			for _, n := range rep.Next {
				out = append(out, n)
			}
			sort.Slice(out, func(a, b int) bool { return out[a].ProjectKey < out[b].ProjectKey })
		}
		writeJSON(w, http.StatusOK, out)
	}
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
