package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adata/reports/internal/auth"
)

func TestRequireModule(t *testing.T) {
	h := requireModule(auth.ModSprints)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	cases := []struct {
		name string
		u    auth.User
		want int
	}{
		{"админ без списка", auth.User{Role: auth.RoleAdmin}, http.StatusOK},
		{"модуль выдан", auth.User{Role: auth.RoleLead, Modules: []string{auth.ModSprints}}, http.StatusOK},
		{"другой модуль", auth.User{Role: auth.RoleLead, Modules: []string{auth.ModAttendance}}, http.StatusForbidden},
		{"нет модулей", auth.User{Role: auth.RoleEmployee}, http.StatusForbidden},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req = req.WithContext(auth.WithUser(req.Context(), c.u))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s: код %d, ждали %d", c.name, rec.Code, c.want)
		}
	}
}
