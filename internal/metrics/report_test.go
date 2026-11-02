package metrics

import "testing"

func TestBuildSprintReport(t *testing.T) {
	active := Sprint{ID: 10, Name: "ABC 2026-19", State: "ACTIVE", Start: ptr(at("2026-10-26 09:00")), End: ptr(at("2026-11-06 18:00"))}
	next := Sprint{ID: 11, Name: "ABC 2026-20", State: "FUTURE", Start: ptr(at("2026-11-09 09:00"))}
	asOf := at("2026-11-02 10:30")
	issues := []Issue{
		{Key: "ABC-1", ProjectKey: "ABC", StatusID: "10100", StatusName: "Сделать", Type: "Задача", Summary: "Старая задача",
			Assignee: "Иванов", AssigneeLogin: "ivanov", Position: "Backend Developer",
			Created: at("2026-09-01 10:00"), Updated: at("2026-09-20 10:00"), Sprints: []Sprint{active}},
		{Key: "ABC-2", ProjectKey: "ABC", StatusID: "10011", StatusName: "В работе", Type: "Ошибка",
			Assignee: "Иванов", AssigneeLogin: "ivanov", Position: "QA Engineer",
			Created: at("2026-10-27 10:00"), Updated: at("2026-10-30 10:00"), Sprints: []Sprint{active}},
		{Key: "ABC-3", ProjectKey: "ABC", StatusID: "777", StatusName: "Test", Type: "Задача",
			Assignee: "Петров", AssigneeLogin: "petrov", Position: "Backend Developer",
			Tester: " Сидорова ", TesterLogin: "sidorova",
			Created: at("2026-10-27 11:00"), Updated: at("2026-11-01 10:00"), Sprints: []Sprint{active}},
		{Key: "ABC-6", ProjectKey: "ABC", StatusID: "10001", Assignee: "Уволенный", AssigneeInactive: true,
			Created: at("2026-10-28 10:00"), Updated: at("2026-10-29 10:00"), Sprints: []Sprint{active}},
		{Key: "ABC-4", ProjectKey: "ABC", StatusID: "888", StatusName: "Blocked", Type: "Задача", Priority: "Высокий",
			Created: at("2026-10-28 11:00"), Updated: at("2026-11-01 10:00"), Sprints: []Sprint{active}},
		{Key: "ABC-5", ProjectKey: "ABC", StatusID: "777", StatusName: "Test", Type: "Epic",
			Created: at("2026-10-28 12:00"), Updated: at("2026-11-01 10:00"), Sprints: []Sprint{active}},
	}
	futureIssues := []SprintIssue{
		{SprintIDs: []int64{11}, Status: "Сделать", Assignee: "Иванов"},
		{SprintIDs: []int64{10, 11}, Status: "Сделать"},
		{SprintIDs: []int64{10}, Status: "Test", Assignee: "Петров"},
	}
	r := BuildSprintReport(issues, map[string]string{"ABC": "Лид"}, []string{"Тимлид"},
		map[string][]Sprint{"ABC": {next}}, futureIssues, asOf)

	var iv *PersonProject
	for i := range r.People {
		if r.People[i].Name == "Иванов" {
			iv = &r.People[i]
		}
	}
	if iv == nil {
		t.Fatal("нет строки Иванова")
	}
	if iv.Total != 2 || iv.Todo != 1 || iv.Wk != 1 || iv.Bug != 1 || iv.TodoStale != 1 ||
		iv.TodoAge != 62 || iv.TodoUpd != 43 || iv.ActDays != 3 || iv.AnyDays != 3 {
		t.Errorf("Иванов: %+v", *iv)
	}
	// Должность — самая частая, при равенстве первая встреченная.
	if r.Roles["Иванов"] != "Backend Developer" || r.Logins["Иванов"] != "ivanov" || r.Logins[" Сидорова "] != "sidorova" {
		t.Errorf("roles=%v logins=%v", r.Roles, r.Logins)
	}
	if len(r.Testers) != 1 || r.Testers[0].InTest != 1 || r.Testers[0].Total != 1 {
		t.Errorf("testers: %+v", r.Testers)
	}
	// Эпик в Test не считается; тестировщик — по обрезанному имени.
	if c := r.Testing["ABC"]["Сидорова"]; c == nil || c.N != 1 || c.Logins[0] != "sidorova" || len(r.Testing["ABC"]) != 1 {
		t.Errorf("testing: %+v", r.Testing["ABC"])
	}
	// Узкие места: ABC-4 (Blocked) и ABC-1 (без обновлений 43 дн.), по убыванию дней.
	if len(r.Bottlenecks) != 2 || r.Bottlenecks[0].Key != "ABC-1" || r.Bottlenecks[1].Assignee != Unassigned {
		t.Errorf("bottlenecks: %+v", r.Bottlenecks)
	}
	if len(r.Inactive) != 1 || r.Inactive[0] != "Уволенный" {
		t.Errorf("inactive: %v", r.Inactive)
	}
	n := r.Next["ABC"]
	if n.NextID == nil || *n.NextID != 11 || n.NextTotal != 2 || n.Statuses[0] != (NameCount{"Сделать", 2}) || len(n.By) != 2 {
		t.Errorf("next: %+v", n)
	}
}
