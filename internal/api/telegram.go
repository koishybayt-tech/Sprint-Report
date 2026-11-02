package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adata/reports/internal/auth"
	"github.com/adata/reports/internal/tgbot"
)

// Очередь «Кто это?»: аккаунты Telegram без сотрудника и ручная привязка к пользователю Jira (admin, hr).

// GET /api/v1/admin/tg/unknown → {authors, employees}: неизвестные авторы и ещё не привязанные пользователи Jira.
func tgUnknown(pool *pgxpool.Pool, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, err := tgbot.NewStore(r.Context(), pool)
		if err != nil {
			fail(w, log, "tg/unknown", err)
			return
		}
		// Сначала перепроверяем очередь: после новых привязок и правил часть авторов находится сама.
		if n, err := s.ReprocessUnknown(r.Context()); err != nil {
			fail(w, log, "tg/unknown", err)
			return
		} else if n > 0 {
			log.Info("tg: авторы найдены при перепроверке очереди", "messages", n)
		}
		authors, err := s.UnknownAuthors(r.Context())
		if err != nil {
			fail(w, log, "tg/unknown", err)
			return
		}
		emps, err := s.UnmappedJiraUsers(r.Context())
		if err != nil {
			fail(w, log, "tg/unknown", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"authors": authors, "employees": emps})
	}
}

// POST /api/v1/admin/tg/aliases {from_id?, from_name, employee_id} → {messages, events}
func tgMapAuthor(pool *pgxpool.Pool, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			FromID     *int64 `json:"from_id"`
			FromName   string `json:"from_name"`
			EmployeeID int64  `json:"employee_id"`
		}
		if !readJSON(w, r, &req) {
			return
		}
		if req.EmployeeID == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "выберите пользователя Jira"})
			return
		}
		s, err := tgbot.NewStore(r.Context(), pool)
		if err != nil {
			fail(w, log, "tg/aliases", err)
			return
		}
		msgs, events, err := s.MapAuthor(r.Context(), req.FromID, req.FromName, req.EmployeeID)
		switch {
		case errors.Is(err, tgbot.ErrAliasConflict):
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		case err != nil && (err.Error() == "не указан аккаунт Telegram" || err.Error() == "сотрудник с логином Jira не найден"):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		case err != nil:
			fail(w, log, "tg/aliases", err)
			return
		}
		actor, _ := auth.FromContext(r.Context())
		log.Info("tg: аккаунт привязан вручную", "from_name", req.FromName, "employee_id", req.EmployeeID, "messages", msgs, "events", events, "by", actor.Login)
		writeJSON(w, http.StatusOK, map[string]int{"messages": msgs, "events": events})
	}
}
