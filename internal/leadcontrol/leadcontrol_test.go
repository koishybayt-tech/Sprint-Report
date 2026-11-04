package leadcontrol

import (
	"testing"
	"time"
)

// Фикстуры: «сейчас» — 02.11.2026 12:00 по Алматы; ago(n) — n дней назад.
var now = time.Date(2026, 11, 2, 12, 0, 0, 0, almaty)

func ago(n float64) time.Time { return now.Add(-time.Duration(n * 24 * float64(time.Hour))) }

// st — переход статуса: дни назад, откуда, куда, кто (логин).
func st(n float64, from, to, author string) Change {
	return Change{At: ago(n), Field: "status", From: from, To: to, Author: author}
}

// iss — задача открытого спринта проекта AC (проект руководителя «Лид»).
func iss(key, summary, status string, created float64, ch ...Change) *Issue {
	return &Issue{Key: key, Project: "AC", Summary: summary, Type: "Задача", Status: status, Created: ago(created),
		Sprints: []Sprint{{Name: "AC 21", State: "ACTIVE"}}, InOpenSprint: true, Changes: ch}
}

func input(issues ...*Issue) *Input {
	in := &Input{Now: now, Lead: "Лид", Projects: []LeadProject{{Key: "AC", Title: "Комплаенс"}, {Key: "EDO", Title: "ЭДО"}},
		LeadOf: map[string]string{"AC": "Лид", "EDO": "Лид", "ADTC": "Другой"},
		Issues: map[string]*Issue{}, Names: map[string]string{"ivanov": "Иванов", "petrov": "Петров", "qa": "Тестов"},
		Roles: map[string]string{"Иванов": "Backend Developer", "Петров": "Frontend Developer", "Тестов": "QA Engineer"}}
	for _, is := range issues {
		in.Issues[is.Key] = is
	}
	return in
}

func link(is *Issue, typ string, keys ...string) *Issue {
	for _, k := range keys {
		is.Links = append(is.Links, Link{Type: typ, Direction: "outward", Key: k})
	}
	return is
}

func TestRole(t *testing.T) {
	cases := []struct {
		summary, position, want string
	}{
		{"Дизайн - экран входа", "", roleDesign},
		{"Фронт - экран входа", "Backend Developer", roleFront}, // префикс главнее должности
		{"БЭК - API входа", "", roleBack},
		{"Бэк - API входа", "", roleBack},
		{"БЭК. Открыть создание документов", "", roleBack}, // разделитель — точка
		{"ИИ - ассистент", "", roleAI},
		{"Сделать экран", "Frontend Developer", roleFront},
		{"Сделать API", "AI Engineer", roleAI},
		{"Сделать витрину", "Data Engineer", roleBack},
		{"Макет", "Designer", roleDesign},
		{"Проверить", "QA Engineer", "QA"},
		{"Что-то", "", ""},
	}
	for _, c := range cases {
		if got := Role(&Issue{Summary: c.summary, Position: c.position}); got != c.want {
			t.Errorf("%q / %q: %q, want %q", c.summary, c.position, got, c.want)
		}
	}
	if kind(roleAI) != roleBack {
		t.Error("ИИ считается бэком")
	}
}

func TestDaysInStatus(t *testing.T) {
	cases := []struct {
		name       string
		is         *Issue
		status     string
		last, tot  int
		sinceStart *int
	}{
		{"последний заход в статус", iss("AC-1", "x", stWork, 20, st(15, stTodo, stWork, ""), st(12, stWork, stReview, ""), st(4, stReview, stWork, "")),
			stWork, 4, 3 + 4, ip(15)},
		{"создана сразу в статусе — от создания", iss("AC-2", "x", stTodo, 6), stTodo, 6, 6, nil},
		{"«с начала» — первый переход в «В работе»", iss("AC-3", "x", stRework, 10, st(9, stTodo, stWork, ""), st(2.5, stWork, stRework, "")),
			stRework, 2, 2, ip(9)},
	}
	for _, c := range cases {
		if got := days(now, lastEnter(c.is, c.status)); got != c.last {
			t.Errorf("%s: в статусе %d, want %d", c.name, got, c.last)
		}
		if got := totalIn(c.is, c.status, now); got != c.tot {
			t.Errorf("%s: всего %d, want %d", c.name, got, c.tot)
		}
		got := sinceStart(now, c.is)
		if (got == nil) != (c.sinceStart == nil) || (got != nil && *got != *c.sinceStart) {
			t.Errorf("%s: с начала %v, want %v", c.name, got, c.sinceStart)
		}
	}
}

func ip(v int) *int { return &v }

func TestBlocked(t *testing.T) {
	design := iss("AC-10", "Дизайн - форма", "Готово", 30, st(8, stWork, stTest, ""), st(5, stTest, "Готово", ""))
	backTodo := iss("AC-11", "БЭК - форма", stTodo, 30)
	backWork := iss("AC-12", "БЭК - отчёт", stWork, 30, st(3, stTodo, stWork, ""))
	backTest := iss("AC-13", "БЭК - выгрузка", stTest, 30, st(2, stReview, stTest, ""))
	front := func(key string, links ...string) *Issue {
		return link(iss(key, "Фронт - "+key, stBlocked, 30, st(6, stTodo, stBlocked, "")), "Relate", links...)
	}
	cases := []struct {
		name                                 string
		is                                   *Issue
		state, waiting, notStarted, blockers string
	}{
		{"фронт ждёт не начатый бэк — красная строка", front("AC-1", "AC-10", "AC-11"),
			"Ждёт: бэк AC-11 (Сделать)", "AC-11", "да", "AC-11 Бэк — Сделать; AC-10 Дизайн — Готово с 25.10"},
		{"сначала проверяется бэк, потом дизайн", front("AC-2", "AC-12"),
			"Ждёт: бэк AC-12 (В работе)", "AC-12", "нет", "AC-12 Бэк — В работе"},
		{"все блокирующие на тесте — можно снимать с датой последнего ухода на тест", front("AC-3", "AC-10", "AC-13"),
			"Можно снимать блок · с 31.10", "", "", "AC-13 Бэк — Test с 31.10; AC-10 Дизайн — Готово с 25.10"},
		{"бэк ждёт только дизайн: связанный бэк — прочий", link(iss("AC-4", "БЭК - API", stBlocked, 30), "Relate", "AC-11", "AC-10"),
			"Можно снимать блок · с 25.10", "", "", "AC-10 Дизайн — Готово с 25.10"},
		{"дизайн никого не ждёт", link(iss("AC-5", "Дизайн - макет", stBlocked, 30), "Relate", "AC-11"),
			stateNoLinks, "", "", ""},
	}
	for _, c := range cases {
		in := input(design, backTodo, backWork, backTest, c.is)
		var row *BlockedRow
		for _, r := range Blocked(in) {
			if r.Key == c.is.Key {
				row = &r
			}
		}
		if row == nil {
			t.Fatalf("%s: строки нет", c.name)
		}
		if row.BlockState != c.state || row.WaitingKey != c.waiting || row.NotStarted != c.notStarted || row.Blockers != c.blockers {
			t.Errorf("%s:\n got state=%q waiting=%q not_started=%q blockers=%q", c.name, row.BlockState, row.WaitingKey, row.NotStarted, row.Blockers)
		}
	}
	// Дни в Blocked — от последнего перехода; прочие связанные попадают в свою колонку.
	r := Blocked(input(backTodo, link(iss("AC-4", "БЭК - API", stBlocked, 30, st(6, stTodo, stBlocked, "")), "Relate", "AC-11")))
	if len(r) != 1 || r[0].DaysBlocked != 6 || r[0].Related != "AC-11 (Relate, Сделать)" {
		t.Errorf("дни/прочие: %+v", r)
	}
}

func TestInWorkAndTesting(t *testing.T) {
	work := iss("AC-1", "x", stWork, 20, st(12, stTodo, stWork, ""), st(10, stWork, stRework, ""), st(7, stRework, stWork, ""))
	work.Sprints = append(work.Sprints, Sprint{"AC 20", "CLOSED"})
	review := iss("AC-2", "x", stReview, 5, st(1, stWork, stReview, ""))
	todo := iss("AC-3", "x", stTodo, 5)
	test := iss("AC-4", "x", stTest, 10, st(4, stReview, stTest, ""))
	test.Tester, test.Type = "Тестов", "Ошибка"
	in := input(work, review, todo, test)

	w := InWork(in)
	if len(w) != 2 || w[0].Key != "AC-1" || w[0].DaysInStatus != 7 || w[0].Reworks != 1 || w[0].Sprints != 2 || *w[0].DaysSinceStart != 12 ||
		w[0].TotalDaysInStatus != 2+7 || w[1].Status != stReview {
		t.Errorf("в работе: %+v", w)
	}
	ts := Testing(in)
	if len(ts) != 1 || ts[0].DaysInTest != 4 || ts[0].Tester != "Тестов" || ts[0].IsBug != "да" || ts[0].DaysSinceStart != nil {
		t.Errorf("на тесте: %+v", ts)
	}
}

func TestPeople(t *testing.T) {
	other := func(key string, ch ...Change) *Issue {
		is := iss(key, "x", stTest, 30, ch...)
		is.Project = "ADTC"
		return is
	}
	cases := []struct {
		name    string
		issues  []*Issue
		person  string
		signal  string
		stale   int
		mine    int
		otherSt int
		otherFn int
	}{
		{"здесь стоит, у другого берёт и завершает — тормозит", []*Issue{
			func() *Issue {
				x := iss("AC-1", "x", stWork, 10, st(5, stTodo, stWork, "ivanov"))
				x.Assignee = "Иванов"
				return x
			}(),
			other("ADTC-1", st(3, stTodo, stWork, "ivanov"), st(1, stWork, stTest, "ivanov")),
		}, "Иванов", SignalStalls, 1, 1, 1, 1}, // взял AC-1 5 дн. назад — тоже действие здесь, но у других их 2
		{"здесь тоже работает не меньше — нет сигнала", []*Issue{
			func() *Issue {
				x := iss("AC-1", "x", stWork, 10, st(5, stTodo, stWork, "ivanov"))
				x.Assignee = "Иванов"
				return x
			}(),
			func() *Issue {
				x := iss("AC-2", "x", stTest, 10, st(2, stTodo, stWork, "ivanov"), st(1, stWork, stReview, "ivanov"))
				x.Assignee = "Иванов"
				return x
			}(),
			other("ADTC-1", st(1, stTodo, stWork, "ivanov")),
		}, "Иванов", "", 1, 3, 1, 0},
		{"задача двигалась меньше 3 дней назад — не стоит", []*Issue{
			func() *Issue {
				x := iss("AC-1", "x", stWork, 10, st(2, stTodo, stWork, "ivanov"))
				x.Assignee = "Иванов"
				return x
			}(),
			other("ADTC-1", st(1, stTodo, stWork, "ivanov"), st(0.5, stWork, stTest, "ivanov")),
		}, "Иванов", "", 0, 1, 1, 1},
		{"старше 7 дней у других не считается", []*Issue{
			func() *Issue { x := iss("AC-1", "x", stTodo, 10); x.Assignee = "Иванов"; return x }(),
			other("ADTC-1", st(8, stTodo, stWork, "ivanov")),
		}, "Иванов", "", 1, 0, 0, 0},
		{"тестировщик не помечается", []*Issue{
			func() *Issue { x := iss("AC-1", "x", stTodo, 10); x.Assignee = "Тестов"; return x }(),
			other("ADTC-1", st(1, stTodo, stWork, "qa")),
		}, "Тестов", "", 1, 0, 1, 0},
		{"чужие правки статуса — не его действия", []*Issue{
			func() *Issue { x := iss("AC-1", "x", stTodo, 10); x.Assignee = "Петров"; return x }(),
			other("ADTC-1", st(1, stTodo, stWork, "ivanov")),
		}, "Петров", "", 1, 0, 0, 0},
	}
	for _, c := range cases {
		var row *PeopleRow
		for _, r := range People(input(c.issues...)) {
			if r.Person == c.person {
				row = &r
			}
		}
		if row == nil {
			t.Fatalf("%s: нет строки", c.name)
		}
		if row.Signal != c.signal || row.MyStale != c.stale || row.MyActions7d != c.mine || row.OtherStarted7d != c.otherSt || row.OtherFinished7d != c.otherFn {
			t.Errorf("%s: %+v", c.name, row)
		}
	}
	r := People(input(
		func() *Issue {
			x := iss("AC-1", "x", stWork, 10, st(5, stTodo, stWork, "ivanov"))
			x.Assignee = "Иванов"
			return x
		}(),
		other("ADTC-1", st(3, stTodo, stWork, "ivanov"), st(1, stWork, stTest, "ivanov"))))[0]
	if r.MyTasks != "AC-1 (5)" || r.OtherProjects != "ADTC (Другой) — 2" || r.OtherTasks != "завершил ADTC-1, взял ADTC-1" ||
		r.MyLastActionDays == nil || *r.MyLastActionDays != 5 || r.Projects != "AC" || r.Lead != "Лид" {
		t.Errorf("колонки: %+v", r)
	}
}

func TestGaps(t *testing.T) {
	type tc struct {
		name           string
		self           *Issue
		others         []*Issue
		priority, miss string // "" — строки нет
	}
	ready := func(key, summary string) *Issue { return iss(key, summary, stTest, 20) }
	fut := func(is *Issue) *Issue {
		is.InOpenSprint, is.InFutureSprint, is.Sprints = false, true, []Sprint{{"AC 22", "FUTURE"}}
		return is
	}
	backlog := func(is *Issue) *Issue { is.InOpenSprint, is.Sprints = false, nil; return is }
	bug := func(is *Issue) *Issue { is.Type = "Ошибка"; return is }
	cases := []tc{
		{"дизайн: бэк есть (по названию), фронта нет — высокий", ready("AC-1", "Дизайн - Отчёт по рискам"),
			[]*Issue{iss("AC-2", "БЭК - Отчёт по рискам", stWork, 5)}, PrioHigh, missFront},
		{"дизайн: фронт по связи Relate, бэка нет — низкий", link(ready("AC-1", "Дизайн - Отчёт"), "Relate", "AC-3"),
			[]*Issue{iss("AC-3", "Фронт - совсем другое название", stTodo, 5)}, PrioLow, missBack},
		{"дизайн без разработки (нет ни фронта, ни бэка) — низкий", ready("AC-1", "Дизайн - Баннер"),
			nil, PrioLow, missBoth},
		{"всё на месте — строки нет", ready("AC-1", "Дизайн - Отчёт"),
			[]*Issue{iss("AC-2", "БЭК - Отчёт", stWork, 5), iss("AC-3", "Фронт - Отчёт", stTodo, 5)}, "", ""},
		{"фронт только в будущем спринте", ready("AC-1", "БЭК - Выгрузка в Excel"),
			[]*Issue{fut(iss("AC-4", "Фронт - Выгрузка в Excel", stTodo, 5))}, PrioFuture, missFront},
		{"фронт в бэклоге вне спринта — бэк готов без фронта", ready("AC-1", "БЭК - Выгрузка"),
			[]*Issue{backlog(iss("AC-4", "Фронт - Выгрузка", stTodo, 5))}, PrioCheck, missFront},
		{"найдена только похожая — проверить", ready("AC-1", "Дизайн - Карточка контрагента новая"),
			[]*Issue{iss("AC-2", "БЭК - Карточка контрагента новая", stWork, 5), iss("AC-5", "Фронт - Карточка контрагента", stTodo, 5)}, PrioCheck, missFront},
		{"бэк готов без фронта — проверить", ready("AC-1", "БЭК - Новый эндпоинт"), nil, PrioCheck, missFront},
		{"исправление ошибки — низкий", bug(ready("AC-1", "БЭК - Падает выгрузка")), nil, PrioLow, missFront},
		{"готовый фронт без бэка — не проблема", ready("AC-1", "Фронт - Отчёт"), nil, "", ""},
		{"дизайн ещё не на тесте — не считается", iss("AC-1", "Дизайн - Отчёт", stWork, 5), nil, "", ""},
		{"ИИ — это бэк", ready("AC-1", "ИИ - Ассистент"), nil, PrioCheck, missFront},
		{"пара из другого проекта не считается", ready("AC-1", "БЭК - Отчёт"),
			[]*Issue{func() *Issue { x := iss("EDO-1", "Фронт - Отчёт", stTodo, 5); x.Project = "EDO"; return x }()}, PrioCheck, missFront},
	}
	for _, c := range cases {
		in := input(append([]*Issue{c.self}, c.others...)...)
		var row *GapRow
		for _, r := range Gaps(in) {
			if r.Key == c.self.Key {
				row = &r
			}
		}
		if c.priority == "" {
			if row != nil {
				t.Errorf("%s: лишняя строка %+v", c.name, row)
			}
			continue
		}
		if row == nil {
			t.Errorf("%s: строки нет", c.name)
			continue
		}
		if row.Priority != c.priority || row.Missing != c.miss {
			t.Errorf("%s: приоритет %q, чего нет %q (%s)", c.name, row.Priority, row.Missing, row.Problem)
		}
	}
	// Колонки «Фронт»/«Бэк» и «Проблема».
	in := input(ready("AC-1", "Дизайн - Карточка контрагента новая"),
		iss("AC-2", "БЭК - Карточка контрагента новая", stWork, 5), iss("AC-5", "Фронт - Карточка контрагента", stTodo, 5))
	r := Gaps(in)[0]
	if r.Front != "похоже 80%: AC-5 · Сделать · текущий спринт" || r.Back != "AC-2 · В работе · текущий спринт" ||
		r.Problem != "Дизайн готов: на фронт найдена только похожая задача AC-5 (80%) — проверить" || r.Kind != roleDesign || r.Sprint != "AC 21" {
		t.Errorf("колонки: %+v", r)
	}
}

func TestTotalsAndSprints(t *testing.T) {
	in := input(
		iss("AC-1", "Фронт - a", stBlocked, 10, st(2, stTodo, stBlocked, "")),
		iss("AC-2", "x", stWork, 10, st(6, stTodo, stWork, "")),
		iss("AC-3", "x", stWork, 10, st(4, stTodo, stWork, "")),
		iss("AC-4", "x", stTest, 10, st(3, stReview, stTest, "")),
		iss("AC-5", "x", "Готово", 10),
		iss("AC-6", "x", "Ready for deploy", 10),
		func() *Issue {
			x := iss("EDO-1", "БЭК - b", stTest, 10, st(1, stReview, stTest, ""))
			x.Project = "EDO"
			return x
		}(),
	)
	rep := Build(in, now)
	all, ac := rep.Totals[0], rep.Totals[1]
	if all.Project != "all" || all.Blocked != 1 || all.LongWork != 1 || all.LongTest != 1 || all.GapsAll != 1 || all.GapsHigh != 0 {
		t.Errorf("все проекты: %+v", all)
	}
	if ac.Project != "AC" || ac.GapsAll != 0 || ac.LongTest != 1 {
		t.Errorf("AC: %+v", ac)
	}
	if len(rep.Sprints) != 2 {
		t.Fatalf("спринты: %+v", rep.Sprints)
	}
	s := rep.Sprints[0]
	if s.Project != "AC" || s.Total != 6 || s.Blocked != 1 || s.InWork != 2 || s.Test != 1 || s.Done != 1 || s.Ready != 1 || s.Snapshot != "02.11.2026 12:00" {
		t.Errorf("полоса: %+v", s)
	}
}

func TestGapIgnoreAndCreateURL(t *testing.T) {
	self := iss("AC-1", "Дизайн - Отчёт", stTest, 20)
	self.ProjectID, self.TypeID = "10200", "3"
	back := iss("AC-2", "БЭК - Отчёт", stWork, 5)
	in := input(self, back)
	in.JiraBase = "https://jira.adata.kz/"
	g := Gaps(in)
	if len(g) != 1 || g[0].CreateURL != "https://jira.adata.kz/secure/CreateIssueDetails!init.jspa?description=%D0%9F%D1%80%D0%BE%D0%B4%D0%BE%D0%BB%D0%B6%D0%B5%D0%BD%D0%B8%D0%B5+%D0%B7%D0%B0%D0%B4%D0%B0%D1%87%D0%B8+AC-1%3A+%D0%94%D0%B8%D0%B7%D0%B0%D0%B9%D0%BD+-+%D0%9E%D1%82%D1%87%D1%91%D1%82&issuetype=3&pid=10200&summary=%D0%A4%D1%80%D0%BE%D0%BD%D1%82+-+%D0%9E%D1%82%D1%87%D1%91%D1%82" {
		t.Errorf("ссылка: %+v", g)
	}
	// «Игнорировать» убирает задачу из разрывов и итогов, но показывает её в списке скрытых.
	in.Ignored = map[string]Ignore{"AC-1": {By: "Лид", At: now}}
	rep := Build(in, now)
	if len(rep.Gaps) != 0 || rep.Totals[0].GapsAll != 0 || len(rep.Ignored) != 1 || rep.Ignored[0].Key != "AC-1" || rep.Ignored[0].By != "Лид" {
		t.Errorf("игнор: gaps=%+v ignored=%+v", rep.Gaps, rep.Ignored)
	}
}

func TestCanonStatus(t *testing.T) {
	for in, want := range map[string]string{"In Progress": "В работе", "To Do": "Сделать", "Done": "Готово", "Need clarification": "Требует уточнения",
		"Test": "Test", "Code review": "Code review", "В работе": "В работе"} {
		if got := CanonStatus(in); got != want {
			t.Errorf("CanonStatus(%q) = %q, want %q", in, got, want)
		}
	}
	if c := CanonChange(Change{Field: "assignee", From: "To Do", To: "Done"}); c.From != "To Do" || c.To != "Done" {
		t.Error("не статусные поля не должны меняться")
	}
}

// Реальная история Jira хранит статусы по-английски; дни «в работе» считаются от момента, когда задачу взяли, а не от создания.
func TestInWorkCountsFromTaken(t *testing.T) {
	row := func(is *Issue) InWorkRow {
		for i, c := range is.Changes {
			is.Changes[i] = CanonChange(c)
		}
		return InWork(input(is))[0]
	}
	// создана 28 дней назад, взята в работу 5 дней назад
	r := row(iss("AC-1", "Бэк - API", "В работе", 28, st(5, "To Do", "In Progress", "ivanov")))
	if r.DaysInStatus != 5 || r.DaysSinceStart == nil || *r.DaysSinceStart != 5 {
		t.Errorf("взята 5 дней назад: в статусе %d, с начала %v", r.DaysInStatus, r.DaysSinceStart)
	}
	// возврат в «Сделать» обнуляет: взята 20 дн. назад, возвращена 15, взята снова 3 дн. назад
	r = row(iss("AC-2", "Бэк - API", "В работе", 40, st(20, "To Do", "In Progress", "ivanov"), st(15, "In Progress", "To Do", "ivanov"), st(3, "To Do", "In Progress", "ivanov")))
	if r.DaysSinceStart == nil || *r.DaysSinceStart != 3 {
		t.Errorf("после возврата в «Сделать»: %v, ждали 3", r.DaysSinceStart)
	}
	// круги ревью и доработки момент взятия не сдвигают: взята 10 дн. назад, сейчас снова «В работе» после Need Rework
	r = row(iss("AC-3", "Бэк - API", "В работе", 40, st(10, "To Do", "In Progress", "ivanov"), st(6, "In Progress", "Code review", "petrov"),
		st(4, "Code review", "Need Rework", "petrov"), st(2, "Need Rework", "In Progress", "ivanov")))
	if r.DaysSinceStart == nil || *r.DaysSinceStart != 10 || r.DaysInStatus != 2 {
		t.Errorf("круги ревью: с начала %v (ждали 10), в статусе %d (ждали 2)", r.DaysSinceStart, r.DaysInStatus)
	}
	// нет истории — момент неизвестен, дни «с начала» не показываем
	if r = row(iss("AC-4", "Бэк - API", "В работе", 9)); r.DaysSinceStart != nil {
		t.Errorf("без истории: %v, ждали nil", r.DaysSinceStart)
	}
}
