package metrics

import (
	"testing"
	"time"
)

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, Almaty)
	if err != nil {
		panic(err)
	}
	return t
}

func ptr[T any](v T) *T { return &v }

func TestProjectSprints(t *testing.T) {
	active := Sprint{ID: 10, Name: "ABC 2026-19", State: "ACTIVE", Start: ptr(at("2026-10-26 09:00")), End: ptr(at("2026-11-06 18:00"))}
	closed := Sprint{ID: 9, Name: "ABC 2026-18", State: "CLOSED"}
	future := Sprint{ID: 11, Name: "ABC 2026-20", State: "FUTURE"}
	asOf := at("2026-11-02 10:30")

	issues := []Issue{
		// самая ранняя открытая, перенесена из прошлого спринта
		{Key: "ABC-1", ProjectKey: "ABC", ProjectName: "Abc", StatusID: "10100", StatusName: "Сделать", Type: "Задача",
			Assignee: "Иванов", Created: at("2026-09-01 10:00"), Updated: at("2026-10-01 10:00"), Sprints: []Sprint{closed, active}},
		// готовая, перенесена: carry считает, carry_open — нет
		{Key: "ABC-2", ProjectKey: "ABC", StatusID: "10001", StatusName: "Готово", Type: "Ошибка",
			Assignee: "Петров", Created: at("2026-09-02 10:00"), Updated: at("2026-10-01 10:00"), TimespentS: 5400, Sprints: []Sprint{closed, active}},
		// добавлена после старта, внеплановая, в тесте
		{Key: "ABC-3", ProjectKey: "ABC", StatusID: "12345", StatusName: "Test", Plan: "Внеплановая",
			Assignee: "Иванов", Created: at("2026-10-28 12:00"), Updated: at("2026-11-01 12:00"), TimespentS: 1800, Sprints: []Sprint{active}},
		// без активного спринта (в inc.js такая задача тоже попала бы в проект)
		{Key: "ABC-4", ProjectKey: "ABC", StatusID: "10011", StatusName: "В работе",
			Created: at("2026-10-30 12:00"), Updated: at("2026-11-02 09:00"), Sprints: []Sprint{future}},
		{Key: "ABC-5", ProjectKey: "ABC", StatusID: "777", StatusName: "Ready for deploy",
			Assignee: "Петров", Created: at("2026-10-29 12:00"), Updated: at("2026-11-02 09:00"), Sprints: []Sprint{active}},
	}
	got := ProjectSprints(issues, map[string]string{"ABC": "Сидоров"}, asOf)
	if len(got) != 1 {
		t.Fatalf("проектов %d, ждали 1", len(got))
	}
	p := got[0]
	check := func(name string, got, want any) {
		t.Helper()
		if got != want {
			t.Errorf("%s = %v, ждали %v", name, got, want)
		}
	}
	check("lead", p.Lead, "Сидоров")
	check("sprint", p.Sprint, "ABC 2026-19")
	check("start", p.Start, "2026-10-26")
	check("end", p.End, "2026-11-06")
	check("total", p.Total, 5)
	check("todo", p.Todo, 1)
	check("wk", p.Wk, 1)
	check("dn", p.Dn, 1)
	check("ts", p.Ts, 2)
	check("bug", p.Bug, 1)
	check("carry", p.Carry, 2)
	check("carry_open", p.CarryOpen, 1)
	check("added", p.Added, 2) // ABC-3 и ABC-5; у ABC-4 нет активного спринта
	check("unplanned", p.Unplanned, 1)
	check("stale", p.Stale, 1) // ABC-1: обновлена 32 дня назад
	check("hours", p.Hours, 2.0)
	check("people", p.People, 2)
	check("ready_deploy", p.ReadyDeploy, 1)
	check("in_test", p.InTest, 1)
	check("ready_pct", p.ReadyPct, 20)
	check("left_days", *p.LeftDays, 4)
	// (02.11 12:00+05 − 26.10 00:00 UTC) / 11 дней = 7,29/11 → 66 %
	check("time_pct", p.TimePct, 66)
	check("oldest key", p.OldestOpen.Key, "ABC-1")
	check("oldest age", p.OldestOpen.AgeDays, 62)
}

func TestProjectSprintsOverdue(t *testing.T) {
	sp := Sprint{ID: 1, Name: "X 5", State: "ACTIVE", Start: ptr(at("2026-10-01 09:00")), End: ptr(at("2026-10-14 18:00"))}
	got := ProjectSprints([]Issue{{Key: "X-1", ProjectKey: "X", StatusID: "10001", Created: at("2026-10-01 10:00"),
		Updated: at("2026-10-01 10:00"), Sprints: []Sprint{sp}}}, nil, at("2026-11-02 18:00"))[0]
	if *got.LeftDays != -19 || got.TimePct != 100 || got.Lead != "—" || got.OldestOpen != nil {
		t.Errorf("left=%d time=%d lead=%q oldest=%v", *got.LeftDays, got.TimePct, got.Lead, got.OldestOpen)
	}
}

func TestPickNext(t *testing.T) {
	start := ptr(at("2026-11-09 09:00"))
	cases := []struct {
		cur  string
		fut  []Sprint
		want int64
	}{
		{"Adata 2026-19", []Sprint{{ID: 3, Name: "Adata 2026-21"}, {ID: 2, Name: "Adata 2026-20"}, {ID: 5, Name: "Backlog"}}, 2},
		{"Sprint 7", []Sprint{{ID: 4, Name: "Sprint 8 (релиз)"}, {ID: 9, Name: "Бэклог 1"}}, 4},
		{"Спринт", []Sprint{{ID: 7, Name: "Без номера", Start: start}, {ID: 8, Name: "Бэклог", Start: start}}, 7},
	}
	for _, c := range cases {
		got := PickNext(c.cur, c.fut)
		if got == nil || got.ID != c.want {
			t.Errorf("PickNext(%q) = %+v, ждали id %d", c.cur, got, c.want)
		}
	}
	if PickNext("A 1", nil) != nil {
		t.Error("без будущих спринтов ждали nil")
	}
}
