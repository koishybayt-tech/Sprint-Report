package api

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adata/reports/internal/db"
)

// Справочник «руководитель → проекты» для «Контроля спринта руководителей»: только admin.

// GET /api/v1/admin/lead-projects
func leadProjectsList(pool *pgxpool.Pool, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := db.ListLeadProjects(r.Context(), pool)
		if err != nil {
			fail(w, log, "lead projects", err)
			return
		}
		writeJSON(w, http.StatusOK, list)
	}
}

var reProjectKey = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,19}$`)

// POST /api/v1/admin/lead-projects {lead, project_key, title, position} — добавить или изменить пару.
func leadProjectsSave(pool *pgxpool.Pool, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var p db.LeadProject
		if !readJSON(w, r, &p) {
			return
		}
		p.Lead, p.ProjectKey, p.Title = strings.TrimSpace(p.Lead), strings.ToUpper(strings.TrimSpace(p.ProjectKey)), strings.TrimSpace(p.Title)
		if p.UserID != nil {
			// Проект назначен пользователю: без явного имени руководитель берётся из его сотрудника или ФИО.
			label, err := db.UserLeadLabel(r.Context(), pool, *p.UserID)
			if errors.Is(err, db.ErrNotFound) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "пользователь не найден или заблокирован"})
				return
			}
			if err != nil {
				fail(w, log, "lead projects", err)
				return
			}
			if p.Lead == "" {
				p.Lead = label
			}
		}
		if p.Lead == "" || !reProjectKey.MatchString(p.ProjectKey) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "укажите руководителя или пользователя и ключ проекта Jira (латиницей, например AC)"})
			return
		}
		if err := db.SaveLeadProject(r.Context(), pool, p); err != nil {
			fail(w, log, "lead projects", err)
			return
		}
		log.Info("lead projects: сохранено", "lead", p.Lead, "project", p.ProjectKey, "user_id", p.UserID)
		writeJSON(w, http.StatusOK, p)
	}
}

// DELETE /api/v1/admin/lead-projects?lead=&project=
func leadProjectsDelete(pool *pgxpool.Pool, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lead, project := r.URL.Query().Get("lead"), r.URL.Query().Get("project")
		err := db.DeleteLeadProject(r.Context(), pool, lead, project)
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "такой пары нет"})
			return
		}
		if err != nil {
			fail(w, log, "lead projects", err)
			return
		}
		log.Info("lead projects: удалено", "lead", lead, "project", project)
		w.WriteHeader(http.StatusNoContent)
	}
}
