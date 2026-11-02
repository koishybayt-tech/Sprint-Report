package metrics

import (
	"testing"
	"time"
)

func TestBuildEmployees(t *testing.T) {
	tp := func(s string) *time.Time { x := at(s); return &x }
	people := []EmpPerson{
		{ID: 1, Name: "Иванов", Login: "ivanov", Pos: "Backend Developer"},
		{ID: 2, Name: "Петров", Login: "petrov", Pos: "Backend Developer"},
		{ID: 3, Name: "Тестов", Login: "qa"},
		{ID: 4, Name: "Без данных", Login: "none"},
	}
	issues := []EmpIssue{
		{ // закрыта Ивановым в марте; в работу 01.03, закрыта 04.03 → цикл 3,0; возврат 02.03 при исполнителе Петрове
			Key: "AB-1", Summary: "Задача", Type: "Ошибка", Status: "Done", Done: true,
			Assignee: "ivanov", Reporter: "petrov", Tester: "qa",
			Created: at("2026-02-20 10:00"), Resolved: tp("2026-03-04 09:00"), TimespentS: 5400,
			Sprints: []EmpSprint{
				{State: "CLOSED", Start: tp("2026-02-23 09:00"), End: tp("2026-03-06 18:00")},
				{State: "CLOSED", Start: tp("2026-02-09 09:00"), End: tp("2026-02-20 18:00")},
			},
			Changes: []EmpChange{
				{At: at("2026-03-01 09:00"), Field: "assignee", From: "", To: "petrov"},
				{At: at("2026-03-01 09:00"), Field: "status", From: "Бэклог", To: "В работе"},
				{At: at("2026-03-02 09:00"), Field: "status", From: "Тестирование", To: "Need Rework"},
				{At: at("2026-03-03 09:00"), Field: "assignee", From: "petrov", To: "ivanov"},
			},
		},
		{Key: "CD-2", Summary: "Открытая", Status: "Бэклог", Assignee: "ivanov", Created: at("2026-03-10 10:00")},
	}
	late, b := 600, 630
	nine := 540
	ev := []AttEvent{
		{Date: "2026-03-05", N: "Иванов", T: "late", M: 90, A: &nine, B: &late, Rsn: "врач"},
		{Date: "2026-03-06", N: "Иванов", T: "mid", M: 30, A: &late, B: &b, C: "study"},
		{Date: "2026-03-07", N: "Иванов", T: "day", M: 0, Rsn: "семья"},
		{Date: "2026-03-09", N: "Иванов", T: "sick"},
	}
	e := BuildEmployees(issues, ev, people, at("2026-04-15 12:00"))

	if len(e.Mon) != 4 || e.Mon[3] != "2026-04" || len(e.Ppl) != 3 {
		t.Fatalf("mon=%v ppl=%d", e.Mon, len(e.Ppl))
	}
	iv := e.Ppl[0]
	if iv.Name != "Иванов" {
		t.Fatalf("order: %+v", e.Ppl)
	}
	m3, m2 := iv.M["2026-03"], iv.M["2026-02"]
	if m3[ixDone] != 1 || m3[ixBug] != 1 || m3[ixH] != 1.5 || m3[ixCarry] != 1 || m3[ixCyc] != 3.0 {
		t.Errorf("март задачи: %v", m3)
	}
	// План: мартовский спринт — в срок (закрыта до конца+2 дн.), февральский — нет.
	if m3[ixPlan] != 1 || m3[ixPlanDone] != 1 || m2[ixPlan] != 1 || m2[ixPlanDone] != 0 {
		t.Errorf("план: март %v февраль %v", m3, m2)
	}
	if m3[ixRw] != 0 || e.Ppl[1].M["2026-03"][ixRw] != 1 {
		t.Errorf("возврат должен уйти Петрову: %v / %v", m3, e.Ppl[1].M["2026-03"])
	}
	if e.Ppl[1].M["2026-02"][ixRep] != 1 || e.Ppl[2].M["2026-03"][ixQA] != 1 {
		t.Errorf("автор/тестировщик: %v %v", e.Ppl[1].M, e.Ppl[2].M)
	}
	if m3[ixLate] != 1 || m3[ixMid] != 1 || m3[ixDay] != 1 || m3[ixSick] != 1 || m3[ixMin] != 90+30+480 || m3[ixLateStudy] != 0 {
		t.Errorf("посещаемость: %v", m3)
	}
	if iv.Open != [2]int{1, 0} || len(iv.Proj) != 2 {
		t.Errorf("open=%v proj=%v", iv.Open, iv.Proj)
	}
	c := e.Cards[1] // открытые задачи — сверху (закрыта = «9999»), как в build_emp.py
	if len(c.Tasks) != 2 || c.Tasks[0][0] != "CD-2" || c.Tasks[1][7] != 1 || len(c.Alog) != 4 || c.Alog[0][1] != "Больничный" {
		t.Errorf("card: %+v", c)
	}
	if c.Alog[3][3] != "9:00–10:00 · врач" {
		t.Errorf("alog: %v", c.Alog[3])
	}

	// HR — без задач; исходные данные не меняются.
	hr := RestrictEmployees(e, map[int64]bool{1: true}, true)
	if len(hr.Ppl) != 1 || hr.Ppl[0].M["2026-03"][ixDone] != 0 || len(hr.Cards[1].Tasks) != 0 || len(hr.Cards[1].Alog) != 4 {
		t.Errorf("hr: %+v", hr.Ppl)
	}
	if e.Ppl[0].M["2026-03"][ixDone] != 1 || len(e.Cards[1].Tasks) != 2 {
		t.Errorf("исходные данные изменились")
	}
	// Руководитель — время без причин.
	HideAlogReasons(hr.Cards[1])
	if hr.Cards[1].Alog[3][3] != "9:00–10:00" || hr.Cards[1].Alog[1][3] != "" || e.Cards[1].Alog[3][3] != "9:00–10:00 · врач" {
		t.Errorf("hide: %v / %v", hr.Cards[1].Alog, e.Cards[1].Alog[3])
	}
}
