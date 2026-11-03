package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adata/reports/internal/db"
	"github.com/adata/reports/internal/leadcontrol"
)

// Данные для экрана ТВ (/tv). Доступ — по модулю tv (requireModule в роутере), независимо от роли и других модулей:
// ТВ-пользователю не нужны ни «Спринты», ни «Контроль спринта» — экран показывает сводку по всем проектам.

// tvLeadControl — GET /api/v1/tv/lead-control: долгие, тестовые и заблокированные задачи всех руководителей одним ответом
// (готовый расчёт worker'а; если его ещё нет — считается и сохраняется).
func tvLeadControl(pool *pgxpool.Pool, log *slog.Logger, jiraBase string) http.HandlerFunc {
	type part struct {
		InWork  []leadcontrol.InWorkRow  `json:"in_work"`
		Testing []leadcontrol.TestingRow `json:"testing"`
		Blocked []leadcontrol.BlockedRow `json:"blocked"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		leads, by, err := db.LeadControlLeads(r.Context(), pool)
		if err != nil {
			fail(w, log, "tv/lead-control", err)
			return
		}
		out := make([]part, 0, len(leads))
		for _, lead := range leads {
			var rep leadcontrol.Report
			raw, _, err := db.LeadControl(r.Context(), pool, lead)
			if err == nil && raw != nil {
				err = json.Unmarshal(raw, &rep)
			} else if err == nil {
				var reps map[string]leadcontrol.Report
				if reps, err = db.BuildLeadControl(r.Context(), pool, []string{lead}, by, time.Now(), jiraBase); err == nil {
					rep = reps[lead]
					err = db.SaveLeadControl(r.Context(), pool, lead, rep)
				}
			}
			if err != nil {
				fail(w, log, "tv/lead-control", err)
				return
			}
			out = append(out, part{InWork: rep.InWork, Testing: rep.Testing, Blocked: rep.Blocked})
		}
		writeJSON(w, http.StatusOK, out)
	}
}
