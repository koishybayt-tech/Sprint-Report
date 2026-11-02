package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/adata/reports/internal/auth"
	"github.com/adata/reports/internal/notify"
)

// Уведомления «Контроля спринта руководителей»: колокольчик (у всех свой), настройки и личный бот (admin, lead).

func notifyErr(w http.ResponseWriter, log *slog.Logger, what string, err error) {
	switch {
	case errors.Is(err, notify.ErrNoKey), errors.Is(err, notify.ErrBadToken):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, notify.ErrNotConnected):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	default:
		fail(w, log, what, err)
	}
}

// GET /api/v1/notifications
func notifyList(nt *notify.Service, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.FromContext(r.Context())
		l, err := nt.List(r.Context(), u.ID)
		if err != nil {
			fail(w, log, "notifications", err)
			return
		}
		writeJSON(w, http.StatusOK, l)
	}
}

// POST /api/v1/notifications/read {ids?, all?}
func notifyRead(nt *notify.Service, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			IDs []int64 `json:"ids"`
			All bool    `json:"all"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		u, _ := auth.FromContext(r.Context())
		if err := nt.MarkRead(r.Context(), u.ID, body.IDs, body.All); err != nil {
			fail(w, log, "notifications read", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeSettings(w http.ResponseWriter, r *http.Request, nt *notify.Service, log *slog.Logger) {
	u, _ := auth.FromContext(r.Context())
	st, err := nt.Settings(r.Context(), u.ID)
	if err != nil {
		fail(w, log, "notifications settings", err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// GET /api/v1/notifications/settings
func notifySettings(nt *notify.Service, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { writeSettings(w, r, nt, log) }
}

// PUT /api/v1/notifications/settings {on_blocked?, on_unblock?, on_gap?}
func notifySaveSettings(nt *notify.Service, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			OnBlocked *bool `json:"on_blocked"`
			OnUnblock *bool `json:"on_unblock"`
			OnGap     *bool `json:"on_gap"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		u, _ := auth.FromContext(r.Context())
		if err := nt.SaveFlags(r.Context(), u.ID, body.OnBlocked, body.OnUnblock, body.OnGap); err != nil {
			fail(w, log, "notifications settings", err)
			return
		}
		writeSettings(w, r, nt, log)
	}
}

// POST /api/v1/notifications/telegram {token} — токен личного бота; в ответе и логах его нет.
func notifyConnect(nt *notify.Service, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Token string `json:"token"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		u, _ := auth.FromContext(r.Context())
		if err := nt.Connect(r.Context(), u.ID, body.Token); err != nil {
			notifyErr(w, log, "notifications telegram", err)
			return
		}
		log.Info("notify: личный бот подключён", "user", u.Login)
		writeSettings(w, r, nt, log)
	}
}

// DELETE /api/v1/notifications/telegram
func notifyDisconnect(nt *notify.Service, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.FromContext(r.Context())
		if err := nt.Disconnect(r.Context(), u.ID); err != nil {
			fail(w, log, "notifications telegram", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// POST /api/v1/notifications/test {kind?: blocked|unblock|gap} — пример события: колокольчик и, если привязан, бот.
func notifyTest(nt *notify.Service, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Kind string `json:"kind"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		u, _ := auth.FromContext(r.Context())
		res, err := nt.Test(r.Context(), u.ID, body.Kind)
		if err != nil {
			notifyErr(w, log, "notifications test", err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}
