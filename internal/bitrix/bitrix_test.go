package bitrix

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMask(t *testing.T) {
	c, _ := New("https://bitrix.example/api/v1/vacation_api.php", "s3cr3t+key/=")
	for _, s := range []string{
		`Get "https://bitrix.example/api/v1/vacation_api.php?api_key=s3cr3t%2Bkey%2F%3D&id=1": timeout`,
		`ответ: {"error":"bad key s3cr3t+key/="}`,
		`url ...?id=1&api_key=другой_ключ&x=1`,
	} {
		got := c.Mask(s)
		if strings.Contains(got, "s3cr3t") || strings.Contains(got, "другой_ключ") {
			t.Errorf("ключ не замаскирован: %s", got)
		}
	}
}

func TestFetchMasksErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key") != "k3y" || r.URL.Query().Get("id") != "20980" {
			t.Errorf("запрос: %s", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"key k3y denied"}`))
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "k3y")
	_, err := c.Fetch(context.Background(), "20980")
	if err == nil || strings.Contains(err.Error(), "k3y") || !strings.Contains(err.Error(), "403") {
		t.Fatalf("ошибка: %v", err)
	}
	srv.Close()
	_, err = c.Fetch(context.Background(), "20980")
	if err == nil || strings.Contains(err.Error(), "k3y") {
		t.Fatalf("сетевая ошибка с ключом: %v", err)
	}
}

func TestParse(t *testing.T) {
	// Формат как в ответе vacation_api.php (reports bitrix probe), персональные поля опущены.
	body := `{"employee_id":20980,"count":4,"items":[
		{"element_id":"356424","vocation_type":"Трудовой отпуск","date_start":"19.12.2025","date_end":"19.12.2025","status":"Завершен"},
		{"element_id":"375337","vocation_type":"Трудовой отпуск","date_start":"29.01.2026","date_end":"29.01.2026","status":"Прерван"},
		{"element_id":"400001","vocation_type":"Отпуск б/с","date_start":"01.12.2026","date_end":"02.12.2026","status":"Согласован"},
		{"element_id":"400002","vocation_type":"Больничный","date_start":"01.09.2026","date_end":"03.09.2026","status":"Завершен"}
	]}`
	vac, skipped, seen, err := Parse([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(vac) != 2 || skipped != 2 {
		t.Fatalf("vac=%+v skipped=%d", vac, skipped)
	}
	if vac[0].Kind != "vacation" || vac[0].ID != "356424" || vac[0].From.Format("2006-01-02") != "2025-12-19" ||
		vac[1].Kind != "unpaid" || vac[1].To.Format("2006-01-02") != "2026-12-02" {
		t.Errorf("vac=%+v", vac)
	}
	if seen["Трудовой отпуск"]["Прерван"] != 1 || seen["Больничный"]["Завершен"] != 1 {
		t.Errorf("seen=%v", seen)
	}
	if externalID("20980", vac[0]) != "bx:356424" {
		t.Errorf("external id: %s", externalID("20980", vac[0]))
	}
	vac[0].ID = ""
	if got := externalID("20980", vac[0]); got != "20980:vacation:2025-12-19:2025-12-19" {
		t.Errorf("external id без id: %s", got)
	}
	if _, _, _, err := Parse([]byte(`<html>error</html>`)); err == nil {
		t.Error("ждали ошибку на не-JSON")
	}
}

func TestNoVacations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id") == "1" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"Not Found: отпуска не найдены"}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"Forbidden: нет доступа"}`))
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "k")
	_, err := c.Fetch(context.Background(), "1")
	if !NoVacations(err) || !strings.Contains(err.Error(), "отпуска не найдены") {
		t.Errorf("404: %v", err)
	}
	_, err = c.Fetch(context.Background(), "2")
	if NoVacations(err) || !strings.Contains(err.Error(), "HTTP 403: Forbidden: нет доступа") {
		t.Errorf("403: %v", err)
	}
}
