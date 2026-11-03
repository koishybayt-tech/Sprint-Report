package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/adata/reports/internal/auth"
)

type authHandlers struct {
	users   *auth.Store
	limiter *auth.Limiter
	log     *slog.Logger
}

// readJSON принимает только application/json (вместе с SameSite=Lax это закрывает CSRF через формы) и не больше 64 КБ.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "ожидается application/json"})
		return false
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "некорректный JSON"})
		return false
	}
	return true
}

func clientIP(r *http.Request) string {
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return h
	}
	return r.RemoteAddr
}

// POST /api/v1/auth/login {login, password} → 204 + cookie.
func (h *authHandlers) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	// Ключ — только логин: IP за прокси подделывается через X-Forwarded-For, и лимит по IP обходился бы.
	key := strings.ToLower(strings.TrimSpace(req.Login))
	if h.limiter.Blocked(key) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "слишком много попыток, попробуйте через 10 минут"})
		return
	}
	u, err := h.users.Authenticate(r.Context(), req.Login, req.Password)
	if errors.Is(err, auth.ErrBadCredentials) {
		h.limiter.Fail(key)
		h.log.Warn("auth: неудачный вход", "login", req.Login, "ip", clientIP(r))
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	if err != nil {
		fail(w, h.log, "auth/login", err)
		return
	}
	h.limiter.Reset(key)
	token, exp, err := h.users.NewSession(r.Context(), u.ID)
	if err != nil {
		fail(w, h.log, "auth/login", err)
		return
	}
	auth.SetCookie(w, r, token, exp)
	h.log.Info("auth: вход", "login", u.Login, "ip", clientIP(r))
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/auth/logout
func (h *authHandlers) logout(w http.ResponseWriter, r *http.Request) {
	if t := auth.Token(r); t != "" {
		if err := h.users.DeleteSession(r.Context(), t); err != nil {
			fail(w, h.log, "auth/logout", err)
			return
		}
	}
	auth.ClearCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

// requireUser пропускает только запросы с действующей сессией активного пользователя.
func (h *authHandlers) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := h.users.UserBySession(r.Context(), auth.Token(r))
		if errors.Is(err, auth.ErrNotFound) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "нужно войти"})
			return
		}
		if err != nil {
			fail(w, h.log, "auth/session", err)
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), u)))
	})
}

// requireRole — доступ только перечисленным ролям (docs/security.md).
func requireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, _ := auth.FromContext(r.Context())
			for _, x := range roles {
				if u.Role == x {
					next.ServeHTTP(w, r)
					return
				}
			}
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "недостаточно прав"})
		})
	}
}

// requireModule — доступ к модулю шапки: админу всегда, остальным — если модуль выдан пользователю.
// Работает вместе с requireRole: роль ограничивает, модуль выдаёт.
func requireModule(mod string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if u, _ := auth.FromContext(r.Context()); !u.HasModule(mod) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "нет доступа к модулю"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// GET /api/v1/me
func (h *authHandlers) me(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.FromContext(r.Context())
	writeJSON(w, http.StatusOK, u)
}

// POST /api/v1/me/password {current, new}
func (h *authHandlers) changeOwnPassword(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.FromContext(r.Context())
	var req struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := h.users.CheckPassword(r.Context(), u.ID, req.Current); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "текущий пароль неверный"})
		return
	}
	if err := h.users.SetPassword(r.Context(), u.ID, req.New, auth.Token(r)); err != nil {
		h.userErr(w, "me/password", err)
		return
	}
	h.log.Info("auth: пароль изменён", "login", u.Login)
	w.WriteHeader(http.StatusNoContent)
}

// ---------- администрирование пользователей (только admin) ----------

// GET /api/v1/admin/users
func (h *authHandlers) listUsers(w http.ResponseWriter, r *http.Request) {
	us, err := h.users.List(r.Context())
	if err != nil {
		fail(w, h.log, "admin/users", err)
		return
	}
	writeJSON(w, http.StatusOK, us)
}

// POST /api/v1/admin/users {login, full_name, role, password?, employee_login?}
// Без password генерируется случайный; он возвращается один раз в ответе.
func (h *authHandlers) createUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Login         string   `json:"login"`
		FullName      string   `json:"full_name"`
		Role          string   `json:"role"`
		Password      string   `json:"password"`
		EmployeeLogin string   `json:"employee_login"`
		Modules       []string `json:"modules"` // не указано — все модули
	}
	if !readJSON(w, r, &req) {
		return
	}
	generated := req.Password == ""
	if generated {
		req.Password = auth.GeneratePassword()
	}
	u, err := h.users.Create(r.Context(), req.Login, req.FullName, req.Role, req.Password, req.EmployeeLogin, req.Modules)
	if err != nil {
		h.userErr(w, "admin/users create", err)
		return
	}
	actor, _ := auth.FromContext(r.Context())
	h.log.Info("auth: пользователь создан", "login", u.Login, "role", u.Role, "by", actor.Login)
	resp := map[string]any{"user": u}
	if generated {
		resp["password"] = req.Password
	}
	writeJSON(w, http.StatusCreated, resp)
}

// PATCH /api/v1/admin/users/{id} {full_name?, role?, active?}
func (h *authHandlers) updateUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "некорректный id"})
		return
	}
	var req struct {
		Login         *string   `json:"login"`
		FullName      *string   `json:"full_name"`
		Role          *string   `json:"role"`
		Active        *bool     `json:"active"`
		EmployeeLogin *string   `json:"employee_login"` // "" — отвязать от сотрудника
		Modules       *[]string `json:"modules"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	u, err := h.users.Update(r.Context(), id, auth.UserPatch{Login: req.Login, FullName: req.FullName, Role: req.Role, Active: req.Active, EmployeeLogin: req.EmployeeLogin, Modules: req.Modules})
	if err != nil {
		h.userErr(w, "admin/users update", err)
		return
	}
	actor, _ := auth.FromContext(r.Context())
	h.log.Info("auth: пользователь изменён", "login", u.Login, "role", u.Role, "active", u.Active, "employee", u.EmployeeLogin, "modules", u.Modules, "by", actor.Login)
	writeJSON(w, http.StatusOK, u)
}

// POST /api/v1/admin/users/{id}/password {password?} — сброс пароля; без password генерируется и возвращается один раз.
func (h *authHandlers) resetPassword(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "некорректный id"})
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	generated := req.Password == ""
	if generated {
		req.Password = auth.GeneratePassword()
	}
	if err := h.users.SetPassword(r.Context(), id, req.Password, ""); err != nil {
		h.userErr(w, "admin/users password", err)
		return
	}
	actor, _ := auth.FromContext(r.Context())
	h.log.Info("auth: пароль сброшен", "user_id", id, "by", actor.Login)
	resp := map[string]any{}
	if generated {
		resp["password"] = req.Password
	}
	writeJSON(w, http.StatusOK, resp)
}

// userErr: ошибки ввода — 400/404/409 с понятным текстом, остальное — 500.
func (h *authHandlers) userErr(w http.ResponseWriter, what string, err error) {
	switch {
	case errors.Is(err, auth.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, auth.ErrLoginTaken):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, auth.ErrWeakPassword), errors.Is(err, auth.ErrNoEmployee), strings.HasPrefix(err.Error(), "укажите"), strings.HasPrefix(err.Error(), "неизвестная роль"), strings.HasPrefix(err.Error(), "неизвестный модуль"),
		strings.HasPrefix(err.Error(), "должен остаться"):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		fail(w, h.log, what, err)
	}
}
