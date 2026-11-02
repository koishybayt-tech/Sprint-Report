package api

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/adata/reports/internal/api/lcapi"
	"github.com/adata/reports/internal/leadcontrol"
)

// Ответ /lead-control собирается из leadcontrol.Report — он должен совпадать со схемой LeadControl в api/openapi.yaml:
// без лишних полей (DisallowUnknownFields) и без потерянных (повторная сериализация даёт тот же JSON).
func TestLeadControlMatchesOpenAPI(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	is := func(key, summary, status string) *leadcontrol.Issue {
		return &leadcontrol.Issue{Key: key, Project: "AC", Summary: summary, Type: "Задача", Status: status, Assignee: "Иванов",
			Created: now.Add(-240 * time.Hour), InOpenSprint: true, Sprints: []leadcontrol.Sprint{{Name: "AC 21", State: "ACTIVE"}},
			Changes: []leadcontrol.Change{{At: now.Add(-120 * time.Hour), Field: "status", From: "Сделать", To: status, Author: "ivanov"}}}
	}
	in := &leadcontrol.Input{Now: now, Lead: "Лид", Projects: []leadcontrol.LeadProject{{Key: "AC", Title: "Комплаенс"}},
		Issues: map[string]*leadcontrol.Issue{
			"AC-1": is("AC-1", "Фронт - a", "Blocked"), "AC-2": is("AC-2", "x", "В работе"), "AC-3": is("AC-3", "БЭК - b", "Test"),
		}, Names: map[string]string{"ivanov": "Иванов"}, Roles: map[string]string{}, LeadOf: map[string]string{}}
	rep := leadcontrol.Build(in, now)
	for name, n := range map[string]int{"blocked": len(rep.Blocked), "in_work": len(rep.InWork), "people": len(rep.People),
		"gaps": len(rep.Gaps), "testing": len(rep.Testing), "sprints": len(rep.Sprints), "totals": len(rep.Totals)} {
		if n == 0 {
			t.Fatalf("фикстура не покрывает %s", name)
		}
	}
	b, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var gen lcapi.LeadControl
	if err := dec.Decode(&gen); err != nil {
		t.Fatalf("лишнее поле относительно openapi: %v", err)
	}
	b2, _ := json.Marshal(gen)
	var a, c any
	_ = json.Unmarshal(b, &a)
	_ = json.Unmarshal(b2, &c)
	if !reflect.DeepEqual(a, c) {
		t.Errorf("поля расходятся со схемой:\nотчёт: %s\nсхема: %s", b, b2)
	}
}
