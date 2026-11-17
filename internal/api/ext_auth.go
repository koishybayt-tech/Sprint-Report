package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/adata/reports/internal/auth"
)

// Внешний API (/api/ext/v1) — только чтение, доступ по статичному токену (EXT_API_TOKEN).
// Токен передаётся заголовком `Authorization: Bearer <токен>` или `X-API-Key: <токен>`; в query не принимается (попадает в логи).
// Токен даёт доступ ко всем данным отчётов (как роль admin), поэтому выдавать его нужно только доверенным системам.

// extFails — ограничитель подбора токена: 20 неверных попыток за 10 минут с одного адреса → 429.
var extFails = auth.NewLimiter(20, 10*time.Minute)

func extToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return strings.TrimSpace(r.Header.Get("X-API-Key"))
}

// requireExtToken пропускает запросы с верным токеном. Пустой token в настройках — API выключен (404, как будто его нет).
func requireExtToken(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "внешний API выключен"})
				return
			}
			ip := r.RemoteAddr
			if extFails.Blocked(ip) {
				w.Header().Set("Retry-After", "600")
				writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "слишком много неверных токенов, попробуйте позже"})
				return
			}
			got := extToken(r)
			if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				extFails.Fail(ip)
				w.Header().Set("WWW-Authenticate", `Bearer realm="adata-reports"`)
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "нужен токен: Authorization: Bearer <токен> или X-API-Key"})
				return
			}
			extFails.Reset(ip)
			next.ServeHTTP(w, r)
		})
	}
}
