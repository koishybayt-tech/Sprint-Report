package metrics

import (
	"testing"
	"time"
)

func TestBuildAttendance(t *testing.T) {
	day := func(s string) time.Time { x, _ := time.Parse("2006-01-02", s); return x }
	i := func(v int) *int { return &v }
	tm := func(s string) *time.Time { x := day(s); return &x }
	rows := []AttRow{
		// опоздание до 9:30, «upd» в 9:45 → пришёл позже заявленного
		{ID: 1, EmployeeID: 1, Name: "Иванов", Login: "ivanov", Day: day("2026-11-02"), Kind: "late", FromMin: i(540), ToMin: i(570), Minutes: 30,
			Reason: "врач", ReasonCat: "health", MakeupKind: "tomorrow", MakeupText: "завтра", MakeupDate: tm("2026-11-03"), SentAt: at("2026-11-02 08:50")},
		{ID: 2, EmployeeID: 1, Name: "Иванов", Day: day("2026-11-02"), Kind: "arrived", FromMin: i(585), SentAt: at("2026-11-02 09:45")},
		// учёба по расписанию, отработка «сегодня» в план не попадает
		{ID: 3, EmployeeID: 2, Name: "Петров", Login: "petrov", Day: day("2026-11-02"), Kind: "mid", FromMin: i(600), ToMin: i(720), Minutes: 120,
			MakeupKind: "schedule", MakeupText: "по расписанию", SentAt: at("2026-11-02 09:00")},
		{ID: 4, EmployeeID: 2, Name: "Петров", Login: "petrov", Day: day("2026-10-30"), Kind: "sick", Minutes: 0, SentAt: at("2026-10-30 09:00")},
	}
	rep := &SprintReport{
		Logins:      map[string]string{"Иванов Jira": "ivanov"},
		Roles:       map[string]string{"Иванов Jira": "Backend Developer"},
		Projects:    []ProjectSprint{{Key: "ABC", Lead: "Лид", End: "2026-11-06"}},
		People:      []PersonProject{{Name: "Иванов Jira", ProjectKey: "ABC", Total: 5, Todo: 2, Wk: 1, Rw: 1}},
		Bottlenecks: []Bottleneck{{Key: "ABC-9", Status: "Blocked", Assignee: "Иванов Jira"}},
	}
	staff := []StaffRow{
		{Name: "Иванов", Login: "ivanov", Staff: true},
		{Name: "Петров", Login: "petrov", Staff: true},
		{Name: "Лид", Login: "lead", Staff: true, Management: true},
		{Name: "Ушёл", Staff: true, Left: true},
	}
	a := BuildAttendance(rows, rep, nil, staff, at("2026-11-02 12:00"))

	if len(a.Ev) != 3 || a.From != "2026-10-30" || a.To != "2026-11-02" {
		t.Fatalf("ev=%d from=%s to=%s", len(a.Ev), a.From, a.To)
	}
	iv := a.Ev[0] // порядок — как пришли строки (БД отдаёт по времени сообщения)
	if iv.N != "Иванов" || iv.Arr == nil || *iv.Arr != 585 || iv.Od != "2026-11-03" || iv.C != "health" || iv.Post != "08:50" {
		t.Errorf("Иванов: %+v", iv)
	}
	pt := a.Ev[1]
	if pt.C != "study" || pt.Od != "" {
		t.Errorf("Петров: %+v", pt)
	}
	if a.Ev[2].T != "sick" || a.Ev[2].M != 0 {
		t.Errorf("больничный: %+v", a.Ev[2])
	}
	p := a.Ppl["Иванов"]
	if p.Pos != "Backend Developer" || !p.Jira || len(p.Pr) != 1 || p.Pr[0][2] != 5 || p.Pr[0][8] != 1 || p.Pr[0][3] != "Лид" {
		t.Errorf("ppl Иванов: %+v", p)
	}
	if len(a.Coll) != 2 || len(a.Lead) != 1 || a.Lead[0] != "Лид" {
		t.Errorf("coll=%v lead=%v", a.Coll, a.Lead)
	}

	// Руководитель видит людей своих проектов и себя, причины других скрыты.
	l := a
	l.Ev = append([]AttEvent(nil), a.Ev...)
	l.Ppl = map[string]AttPerson{}
	for k, v := range a.Ppl {
		l.Ppl[k] = v
	}
	RestrictAttendance(&l, LeadScope(&l, "Лид"), "Лид")
	if len(l.Ev) != 1 || l.Ev[0].N != "Иванов" || l.Ev[0].Rsn != "" {
		t.Errorf("lead: %+v", l.Ev)
	}
	// Сотрудник видит только себя и свою причину.
	e := a
	e.Ev = append([]AttEvent(nil), a.Ev...)
	e.Ppl = map[string]AttPerson{}
	for k, v := range a.Ppl {
		e.Ppl[k] = v
	}
	RestrictAttendance(&e, map[string]bool{"Иванов": true}, "Иванов")
	if len(e.Ev) != 1 || e.Ev[0].Rsn != "врач" || len(e.Ppl) != 1 {
		t.Errorf("employee: %+v", e.Ev)
	}
}
