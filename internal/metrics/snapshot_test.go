package metrics

import (
	"reflect"
	"testing"
)

func TestBuildSnapshot(t *testing.T) {
	sp := []SnapSprint{{Name: "AB 1", State: "ACTIVE", Start: "2026-10-19", End: "2026-10-30"}}
	in := SnapInput{
		Today: "2026-10-23", HistEnd: "2026-10-22",
		Projects: []ProjectSprint{{Key: "AB", Lead: "Лид", Sprint: "AB 1", Start: "2026-10-19", End: "2026-10-30", Total: 2}},
		Issues: []SnapIssue{
			{ // руководитель отдал задачу Иванову, та ушла в работу и вернулась на доработку
				Key: "AB-1", Status: "Need Rework", StatusID: "10000", Assignee: "Иванов", Created: at("2026-10-15 10:00"), Sprints: sp,
				Changes: []SnapChange{
					{At: at("2026-10-20 10:00"), Field: "assignee", From: "Лид", To: "Иванов", Author: "Лид"},
					{At: at("2026-10-21 10:00"), Field: "status", From: "Бэклог", To: "В работе", Author: "Иванов"},
					{At: at("2026-10-22 10:00"), Field: "status", From: "Тестирование", To: "Need Rework", Author: "QA"},
				},
			},
			{Key: "AB-2", Status: "Бэклог", StatusID: "10100", Created: at("2026-10-19 10:00"), Sprints: sp},
			{Key: "AB-3", Status: "Бэклог", StatusID: "10100", Assignee: "Лид", Created: at("2026-10-01 10:00")},
		},
		Staff: []string{"Иванов", "Лид"},
		Pos:   map[string]string{"Иванов": "Backend Developer"},
	}
	s := BuildSnapshot(in)
	c := s.Cur["AB"]
	if c.NI != 2 || c.RwEv != 1 || c.Gave != 1 || c.Time != 36 || c.Ended {
		t.Errorf("проект: ni=%d rw=%d gave=%d time=%d", c.NI, c.RwEv, c.Gave, c.Time)
	}
	want := [][]any{
		{"2026-10-19", 1, 1, 0, 0, 0},
		{"2026-10-20", 0, 1, 1, 0, 0},
		{"2026-10-21", 0, 1, 0, 1, 0},
		{"2026-10-22", 0, 1, 0, 1, 0},
	}
	if !reflect.DeepEqual(c.Ser, want) {
		t.Errorf("ряд: %v", c.Ser)
	}
	if c.BlStart != [3]int{1, 1, 0} || c.BlNow != [3]int{1, 1, 0} {
		t.Errorf("бэклог: %v %v", c.BlStart, c.BlNow)
	}
	if len(s.People) != 1 {
		t.Fatalf("люди: %+v", s.People)
	}
	p := s.People[0]
	if p.Name != "Иванов" || p.Main != "AB" || p.Days != 4 || p.Idle != 1 || p.None != 1 || p.Quiet != 1 || p.Cur != 0 || p.Rw != 1 || p.Left != 1 {
		t.Errorf("простой: %+v", p)
	}
	if s.Unas["AB"] != [2]int{1, 1} || s.AXP["Иванов"]["AB"].Rw != 1 {
		t.Errorf("unas=%v axp=%v", s.Unas, s.AXP)
	}
}
