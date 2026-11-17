package api

import (
	"io"
	"log/slog"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/adata/reports/internal/config"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/adata/reports/internal/leadcontrol"
	"github.com/adata/reports/internal/metrics"
)

const extTestToken = "0123456789abcdef0123456789abcdef"

func extCall(token string, hdr func(*http.Request), url string) *httptest.ResponseRecorder {
	h := requireExtToken(token)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	r := httptest.NewRequest(http.MethodGet, url, nil)
	r.RemoteAddr = "10.1.1." + "1:1234"
	if hdr != nil {
		hdr(r)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestExtToken(t *testing.T) {
	bearer := func(v string) func(*http.Request) { return func(r *http.Request) { r.Header.Set("Authorization", v) } }
	cases := []struct {
		name  string
		token string
		hdr   func(*http.Request)
		url   string
		want  int
	}{
		{"выключен — 404", "", bearer("Bearer " + extTestToken), "/x", 404},
		{"без токена", extTestToken, nil, "/x", 401},
		{"Bearer верный", extTestToken, bearer("Bearer " + extTestToken), "/x", 200},
		{"bearer в нижнем регистре", extTestToken, bearer("bearer " + extTestToken), "/x", 200},
		{"X-API-Key верный", extTestToken, func(r *http.Request) { r.Header.Set("X-API-Key", extTestToken) }, "/x", 200},
		{"неверный токен", extTestToken, bearer("Bearer wrong-token"), "/x", 401},
		{"токен в query не принимается", extTestToken, nil, "/x?token=" + extTestToken, 401},
		{"пустой Bearer", extTestToken, bearer("Bearer "), "/x", 401},
	}
	for _, c := range cases {
		extFails.Reset("10.1.1.1:1234")
		if got := extCall(c.token, c.hdr, c.url).Code; got != c.want {
			t.Errorf("%s: код %d, ожидали %d", c.name, got, c.want)
		}
	}
}

func TestExtTokenBlocksBruteForce(t *testing.T) {
	ip := "10.9.9.9:1"
	extFails.Reset(ip)
	h := requireExtToken(extTestToken)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	call := func(tok string) int {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.RemoteAddr = ip
		r.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	for i := 0; i < 20; i++ {
		if c := call("bad"); c != 401 {
			t.Fatalf("попытка %d: %d", i, c)
		}
	}
	if c := call(extTestToken); c != http.StatusTooManyRequests {
		t.Errorf("после 20 неверных даже верный токен должен ждать: %d", c)
	}
	extFails.Reset(ip)
}

func ip(n int) *int { return &n }

// Отчёт: два проекта (Иванов — руководитель P1), люди, тимлид, ПМ, QA, отсутствие и неактивный пользователь.
func extTestReport() *metrics.SprintReport {
	left3, leftNeg := 3, -2
	to := "2026-11-20"
	return &metrics.SprintReport{
		UpdatedAt: time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC),
		Projects: []metrics.ProjectSprint{
			{Key: "P1", Name: "Проект 1", Lead: "Иванов", Sprint: "P1 2026-5", Start: "2026-11-02", End: "2026-11-15", LeftDays: &leftNeg, Total: 20, Todo: 5, Wk: 3, Cr: 1, Rw: 1, Ts: 4, Dn: 6, Bug: 2, Added: 6, Unplanned: 4, Blocked: 3, Stale: 2, InTest: 4, ReadyPct: 30, TimePct: 100,
				CreatedDaily: &metrics.CreatedDaily{From: "2026-11-09", Total: []int{3, 12, 1}, Unplanned: []int{0, 9, 1}}},
			{Key: "P2", Name: "Проект 2", Lead: "Иванов", Sprint: "P2 2026-5", LeftDays: &left3, Total: 10, Todo: 1, Dn: 9, ReadyPct: 90, TimePct: 50},
			{Key: "P3", Name: "Проект 3", Lead: "Петрова", Total: 0},
		},
		People: []metrics.PersonProject{
			{Name: "Сидоров", ProjectKey: "P1", Total: 4, Todo: 1, Wk: 1, Dn: 2},
			{Name: "Сидоров", ProjectKey: "P2", Total: 3, Todo: 0, Dn: 3},
			{Name: "Кузнецов", ProjectKey: "P1", Total: 12, Todo: 9, Wk: 1, Dn: 2},
			{Name: "Отпускников", ProjectKey: "P1", Total: 1, Todo: 0, Dn: 1},
			{Name: "Тимлидов", ProjectKey: "P1", Total: 1, Todo: 0, Dn: 1},
			{Name: "Иванов", ProjectKey: "P1", Total: 1, Todo: 0, Dn: 1},
			{Name: "Тестеров", ProjectKey: "P1", Total: 1, Todo: 0, Dn: 1},
			{Name: "Ушедший", ProjectKey: "P1", Total: 1, Todo: 0, Dn: 1},
			{Name: metrics.Unassigned, ProjectKey: "P1", Total: 2, Todo: 2},
		},
		Roles:     map[string]string{"Сидоров": "Backend Developer", "Кузнецов": "Frontend Developer", "Тестеров": "QA Engineer", "Иванов": "Project Manager", "Отпускников": "Designer"},
		Logins:    map[string]string{"Сидоров": "sidorov", "Кузнецов": "kuznetsov", "Тестеров": "testerov"},
		TeamLeads: []string{"Тимлидов"},
		Inactive:  []string{"Ушедший"},
		Absences:  []metrics.Absence{{Name: "Отпускников", Type: "vac", From: "2026-11-10", To: &to, Source: "Bitrix", Note: "Трудовой отпуск"}},
		Testers:   []metrics.TesterProject{{Name: "Тестеров", ProjectKey: "P1", Total: 6, InTest: 4, Dn: 1}, {Name: "Тестеров", ProjectKey: "P2", Total: 2, Dn: 2}},
		Testing:   map[string]map[string]*metrics.TestingCount{"P1": {"Тестеров": {N: 3}}},
		Next:      map[string]metrics.NextSprint{"P2": {ProjectKey: "P2", NextTotal: 4}},
	}
}

func extTestCtx() *extCtx {
	return newExtCtx(extTestReport(), time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC))
}

func TestExtEmployeeLoads(t *testing.T) {
	c := extTestCtx()
	by := map[string]extEmployeeLoad{}
	for _, e := range c.employeeLoads(2, 8) {
		by[e.Name] = e
	}
	if _, ok := by[metrics.Unassigned]; ok {
		t.Error("«не назначен» не сотрудник")
	}
	s := by["Сидоров"]
	if s.Tasks.Total != 7 || s.Queue != 2 || len(s.Projects) != 2 || s.Projects[0].Project != "P1" || s.Login != "sidorov" {
		t.Errorf("Сидоров: %+v", s)
	}
	if !s.NeedTasks || s.Overloaded {
		t.Errorf("Сидорову нужны задачи (очередь 2 ≤ 2): %+v", s)
	}
	if k := by["Кузнецов"]; !k.Overloaded || k.NeedTasks || k.Queue != 10 {
		t.Errorf("Кузнецов перегружен (бэклог 9 ≥ 8): %+v", k)
	}
	if o := by["Отпускников"]; o.NeedTasks || o.AbsentNow == nil || o.AbsentNow.Type != "vac" || o.AbsentNow.Note != "" {
		t.Errorf("в отпуске — не «нужны задачи», причина не раскрывается: %+v", o)
	}
	if by["Тимлидов"].NeedTasks || !by["Тимлидов"].TeamLead {
		t.Error("тимлид не попадает в «Нужны задачи»")
	}
	if by["Иванов"].NeedTasks || !by["Иванов"].ProjectLead {
		t.Error("ПМ/руководитель проекта не попадает в «Нужны задачи»")
	}
	if by["Тестеров"].NeedTasks {
		t.Error("QA не попадает в «Нужны задачи»")
	}
	if by["Ушедший"].NeedTasks || !by["Ушедший"].JiraInactive {
		t.Error("неактивный в Jira не попадает в «Нужны задачи»")
	}
	if got := c.employeeLoads(0, 8); func() bool {
		for _, e := range got {
			if e.Name == "Сидоров" && e.NeedTasks {
				return true
			}
		}
		return false
	}() {
		t.Error("порог 0: у Сидорова очередь 2 — не подходит")
	}
}

func TestExtSummary(t *testing.T) {
	s := extTestCtx().summary()
	if s.Projects != 3 || s.Tasks.Total != 30 || s.Tasks.Done != 15 || s.DonePct != 50 || s.Unplanned != 4 || s.Blocked != 3 || s.InTest != 4 {
		t.Errorf("summary: %+v", s)
	}
	if s.OverdueSprint != 1 || s.AbsentNow != 1 || s.Overloaded != 1 {
		t.Errorf("просрочено/отсутствуют/перегружены: %+v", s)
	}
	if s.NeedTasks != 1 {
		t.Errorf("нужны задачи: только Сидоров, а не %d", s.NeedTasks)
	}
}

func TestExtProjectViewAndLeads(t *testing.T) {
	c := extTestCtx()
	v := projectView(c.Rep.Projects[0])
	if v.Sprint == nil || v.Sprint.Name != "P1 2026-5" || v.Tasks.Backlog != 5 || v.Tasks.InProgress != 3 || v.Tasks.Rework != 1 || v.UnplannedPct != 20 || v.DonePct != 30 {
		t.Errorf("проект: %+v", v)
	}
	if projectView(c.Rep.Projects[2]).Sprint != nil {
		t.Error("у проекта без спринта sprint = null")
	}
	ls := c.leadStats()
	if len(ls) != 2 || ls[0].Name != "Иванов" || ls[0].ProjectCount != 2 || ls[0].Tasks.Total != 30 || ls[0].DonePct != 50 || ls[0].OverdueCount != 1 {
		t.Errorf("руководители: %+v", ls)
	}
	if ls[0].People != 7 {
		t.Errorf("люди проектов Иванова без «не назначен»: %d", ls[0].People)
	}
}

func TestExtTesters(t *testing.T) {
	ts := extTestCtx().testers()
	if len(ts) != 1 {
		t.Fatalf("тестировщиков: %d", len(ts))
	}
	x := ts[0]
	if x.Tasks != 8 || x.InTest != 4 || x.Done != 3 || x.Open != 5 || x.Projects[0].Project != "P1" || x.Projects[0].Waiting != 3 || x.Position != "QA Engineer" {
		t.Errorf("тестировщик: %+v", x)
	}
}

func TestExtRisksLowSprintsSpikes(t *testing.T) {
	c := extTestCtx()
	r := c.projectRisks()
	if len(r) == 0 || r[0].Project != "P1" || r[0].Score != 3+2+2+1 {
		t.Errorf("P1: просрочен (3) + отстаёт (2) + Blocked 3 (2) + нет следующего спринта (1): %+v", r)
	}
	for _, x := range r {
		if x.Project == "P2" {
			t.Errorf("P2 здоров, следующий спринт есть: %+v", x)
		}
	}
	low := c.lowSprints()
	if len(low) != 1 || low[0].Project != "P1" || low[0].Level != "bad" {
		t.Errorf("низкое закрытие: %+v", low)
	}
	sp := c.creationSpikes()
	if len(sp) != 1 || sp[0].Date != "2026-11-10" || sp[0].Created != 12 || sp[0].Unplanned != 9 {
		t.Errorf("всплески: %+v", sp)
	}
}

func TestExtAttendanceByPerson(t *testing.T) {
	c := extTestCtx()
	att := metrics.Attendance{
		Ev: []metrics.AttEvent{
			{Date: "2026-11-03", N: "Сидоров", T: "late", M: 30},
			{Date: "2026-11-04", N: "Сидоров", T: "early", M: 60},
			{Date: "2026-11-04", N: "Сидоров", T: "upd", M: 0},
			{Date: "2026-10-30", N: "Сидоров", T: "late", M: 99}, // вне периода
			{Date: "2026-11-05", N: "Кузнецов", T: "day", M: 480},
		},
		Ppl: map[string]metrics.AttPerson{"Кузнецов": {Pos: "Frontend Developer", Login: "kuznetsov"}},
		Abs: []metrics.Absence{{Name: "Отпускников", Type: "vac", From: "2026-11-09", To: func() *string { s := "2026-11-13"; return &s }(), Source: "Bitrix"}},
	}
	p := c.attendanceByPerson(att, "2026-11-01", "2026-11-12")
	by := map[string]extAttPerson{}
	for _, x := range p {
		by[x.Name] = x
	}
	s := by["Сидоров"]
	if s.Counts.Events["late"] != 1 || s.Counts.Events["early"] != 1 || s.Counts.Minutes["early"] != 60 || s.Counts.Total != 90 {
		t.Errorf("Сидоров: %+v", s.Counts)
	}
	if by["Кузнецов"].Counts.Total != 480 || by["Кузнецов"].Login != "kuznetsov" {
		t.Errorf("Кузнецов: %+v", by["Кузнецов"])
	}
	// отпуск 9–13 ноября, период до 12-го: пн–чт = 4 рабочих дня
	if o := by["Отпускников"]; o.AbsenceDays != 4 || o.AbsentNow == nil {
		t.Errorf("Отпускников: %+v", o)
	}
	if weekdays("2026-11-07", "2026-11-08") != 0 || weekdays("2026-11-09", "2026-11-09") != 1 || weekdays("2026-11-10", "2026-11-09") != 0 {
		t.Error("weekdays")
	}
	if hm(ip(9*60+5)) != "09:05" || hm(nil) != "" {
		t.Errorf("hm: %q", hm(ip(9*60+5)))
	}
}

func TestExtMonthMapping(t *testing.T) {
	cyc := 4.5
	a := []any{5, 1, 12.5, 2, 6, 4, 1, cyc, 3, 2, 1, 0, 1, 0, 1, 120, 1}
	m := monthOf("2026-10", a)
	if m.Done != 5 || m.Bugs != 1 || m.Hours != 12.5 || m.Carried != 2 || m.Planned != 6 || m.PlannedOnTime != 4 || m.Reworks != 1 ||
		m.CycleDays == nil || *m.CycleDays != 4.5 || m.Reported != 3 || m.Tested != 2 || m.Late != 1 || m.Mid != 1 || m.Sick != 1 || m.Minutes != 120 || m.LateStudy != 1 {
		t.Errorf("месяц: %+v", m)
	}
	if got := monthOf("2026-10", []any{1}); got.Done != 0 {
		t.Error("короткий ряд не падает")
	}
	p := perfOf([]extMonth{m, m}, false)
	if p.Done != 10 || p.Planned != 12 || p.PlannedOnTime != 8 || p.OnTimePct != 67 || p.ReworkPct != 20 || p.Minutes != 240 || p.Months != nil {
		t.Errorf("итоги: %+v", p)
	}
}

func TestExtLongTasks(t *testing.T) {
	three, nine := 3, 9
	r1 := leadcontrol.Report{
		InWork:  []leadcontrol.InWorkRow{{Key: "P1-1", Status: "В работе", Assignee: "А", DaysInStatus: 2, DaysSinceStart: &nine, Summary: "долго"}, {Key: "P1-2", DaysInStatus: 1}, {Key: "P1-3", DaysInStatus: 6}},
		Testing: []leadcontrol.TestingRow{{Key: "P1-4", Assignee: "Б", Tester: "Т", DaysInTest: 3}, {Key: "P1-5", DaysInTest: 2}},
		Blocked: []leadcontrol.BlockedRow{{Key: "P1-6", DaysBlocked: 1}},
	}
	r2 := leadcontrol.Report{ // тот же проект у другого руководителя — дубли не считаются
		InWork:  []leadcontrol.InWorkRow{{Key: "P1-1", DaysInStatus: 2, DaysSinceStart: &three}},
		Testing: []leadcontrol.TestingRow{{Key: "P1-4", DaysInTest: 3}},
		Blocked: []leadcontrol.BlockedRow{{Key: "P1-6", DaysBlocked: 1}},
	}
	w, ts, b := longTasks([]leadcontrol.Report{r1, r2})
	if len(w) != 2 || w[0].Key != "P1-1" || w[0].Days != 9 || w[1].Key != "P1-3" || w[1].Days != 6 || w[0].Project != "P1" {
		t.Errorf("долго в работе (от 5 дн., от момента взятия): %+v", w)
	}
	if len(ts) != 1 || ts[0].Key != "P1-4" || ts[0].Tester != "Т" {
		t.Errorf("долго на тесте: %+v", ts)
	}
	if len(b) != 1 {
		t.Errorf("blocked: %+v", b)
	}
}

// Каждый маршрут внешнего API описан в extdocs/openapi.yaml и наоборот (документация не расходится с кодом).
func TestExtSpecMatchesRoutes(t *testing.T) {
	r := chi.NewRouter()
	mountExt(r, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), config.Config{ExtAPIToken: extTestToken})
	routes := map[string]bool{}
	err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		route = strings.TrimPrefix(route, "/api/ext/v1")
		if method == http.MethodGet && route != "/openapi.yaml" && route != "/docs" {
			routes[route] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	spec := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^  (/[^:\s]*):\s*$`).FindAllStringSubmatch(string(extOpenAPI), -1) {
		spec[m[1]] = true
	}
	for p := range routes {
		if !spec[p] {
			t.Errorf("маршрута %s нет в openapi.yaml", p)
		}
	}
	for p := range spec {
		if !routes[p] {
			t.Errorf("в openapi.yaml есть %s, а маршрута нет", p)
		}
	}
	if len(routes) < 20 {
		t.Errorf("маршрутов подозрительно мало: %d", len(routes))
	}
}
