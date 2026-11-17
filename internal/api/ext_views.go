package api

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/adata/reports/internal/leadcontrol"
	"github.com/adata/reports/internal/metrics"
)

// Представления для внешнего API: те же расчёты, что в отчётах сервиса, но с понятными именами полей.
// Здесь только чистые функции (без базы и HTTP) — они покрыты тестами в ext_test.go.

// Пороги совпадают с экраном ТВ и вкладками отчёта «Спринты» (web/utils/tv.ts, sprintsPeople.ts).
const (
	extNeedDefault = 2 // «Нужны задачи»: в очереди (бэклог + в работе + реворк) не больше
	extOverDefault = 8 // «Перегружены»: бэклог не меньше
	extLongWork    = 5 // «Долго в работе», дн.
	extLongTest    = 3 // «Долго на тесте», дн.
	extLag         = 25
	extLagWarn     = 10
	extMinTime     = 20
	extDoneBad     = 30
	extDoneWarn    = 50
	extTestWarn    = 8
	extTestHigh    = 15
	extSpikeMin    = 10 // всплеск созданных задач: от стольких за день
)

var (
	extExclRe = regexp.MustCompile(`Project Manager|QA Engineer`)
	extPMRe   = regexp.MustCompile(`Project Manager`)
	extQARe   = regexp.MustCompile(`QA Engineer`)
)

// ---------- задачи ----------

type extTasks struct {
	Total      int `json:"total"`
	Backlog    int `json:"backlog"`     // Сделать
	InProgress int `json:"in_progress"` // В работе
	CodeReview int `json:"code_review"`
	Rework     int `json:"rework"` // Need Rework
	Testing    int `json:"testing"`
	Done       int `json:"done"`
	Bugs       int `json:"bugs"`
}

func (t *extTasks) add(o extTasks) {
	t.Total += o.Total
	t.Backlog += o.Backlog
	t.InProgress += o.InProgress
	t.CodeReview += o.CodeReview
	t.Rework += o.Rework
	t.Testing += o.Testing
	t.Done += o.Done
	t.Bugs += o.Bugs
}

// Queue — сколько задач сотрудник ещё может взять в работу: бэклог + в работе + реворк («Нужны задачи»).
func (t extTasks) queue() int { return t.Backlog + t.InProgress + t.Rework }

func tasksOfProject(p metrics.ProjectSprint) extTasks {
	return extTasks{Total: p.Total, Backlog: p.Todo, InProgress: p.Wk, CodeReview: p.Cr, Rework: p.Rw, Testing: p.Ts, Done: p.Dn, Bugs: p.Bug}
}

func tasksOfPerson(p metrics.PersonProject) extTasks {
	return extTasks{Total: p.Total, Backlog: p.Todo, InProgress: p.Wk, CodeReview: p.Cr, Rework: p.Rw, Testing: p.Ts, Done: p.Dn, Bugs: p.Bug}
}

func pctOf(a, b int) int {
	if b == 0 {
		return 0
	}
	return int(float64(a)/float64(b)*100 + 0.5)
}

// ---------- проекты ----------

type extSprint struct {
	Name     string `json:"name"`
	Start    string `json:"start,omitempty"`
	End      string `json:"end,omitempty"`
	DaysLeft *int   `json:"days_left"` // отрицательное — спринт просрочен
	TimePct  int    `json:"time_pct"`  // доля прошедшего времени спринта
}

type extProject struct {
	Key             string                `json:"key"`
	Name            string                `json:"name"`
	Lead            string                `json:"lead"`
	Sprint          *extSprint            `json:"sprint"` // null — у проекта нет активного спринта
	Tasks           extTasks              `json:"tasks"`
	DonePct         int                   `json:"done_pct"`
	CarriedOver     int                   `json:"carried_over"`      // задач, прошедших больше одного спринта
	CarriedOpen     int                   `json:"carried_open"`      // из них незакрытых
	AddedAfterStart int                   `json:"added_after_start"` // создано после старта спринта
	Unplanned       int                   `json:"unplanned"`         // внеплановых
	UnplannedPct    int                   `json:"unplanned_pct"`
	Blocked         int                   `json:"blocked"`
	Stale           int                   `json:"stale"` // незакрытых без обновлений больше 14 дн.
	ReadyToDeploy   int                   `json:"ready_to_deploy"`
	InTest          int                   `json:"in_test"` // в статусе Test
	People          int                   `json:"people"`
	Hours           float64               `json:"hours"`
	OldestOpen      *metrics.OldestOpen   `json:"oldest_open"`
	CreatedDaily    *metrics.CreatedDaily `json:"created_daily"`
}

func projectView(p metrics.ProjectSprint) extProject {
	v := extProject{Key: p.Key, Name: p.Name, Lead: p.Lead, Tasks: tasksOfProject(p), DonePct: p.ReadyPct,
		CarriedOver: p.Carry, CarriedOpen: p.CarryOpen, AddedAfterStart: p.Added, Unplanned: p.Unplanned,
		UnplannedPct: pctOf(p.Unplanned, p.Total), Blocked: p.Blocked, Stale: p.Stale, ReadyToDeploy: p.ReadyDeploy,
		InTest: p.InTest, People: p.People, Hours: p.Hours, OldestOpen: p.OldestOpen, CreatedDaily: p.CreatedDaily}
	if p.Sprint != "" {
		v.Sprint = &extSprint{Name: p.Sprint, Start: p.Start, End: p.End, DaysLeft: p.LeftDays, TimePct: p.TimePct}
	}
	return v
}

// ---------- контекст расчёта ----------

// extCtx — отчёт «Спринты» плюс быстрые справочники: кто сейчас отсутствует, руководители проектов.
type extCtx struct {
	Rep    *metrics.SprintReport
	Today  string // YYYY-MM-DD по Алматы
	Absent map[string]metrics.Absence
	Leads  map[string]bool // руководители проектов (из Jira)
	Inact  map[string]bool
	TL     map[string]bool
}

func newExtCtx(rep *metrics.SprintReport, now time.Time) *extCtx {
	c := &extCtx{Rep: rep, Today: now.In(metrics.Almaty).Format("2006-01-02"),
		Absent: map[string]metrics.Absence{}, Leads: map[string]bool{}, Inact: map[string]bool{}, TL: map[string]bool{}}
	for _, a := range rep.Absences {
		if a.From <= c.Today && (a.To == nil || *a.To >= c.Today) {
			if _, ok := c.Absent[a.Name]; !ok {
				c.Absent[a.Name] = a
			}
		}
	}
	for _, p := range rep.Projects {
		if p.Lead != "" && p.Lead != "—" {
			c.Leads[p.Lead] = true
		}
	}
	for _, n := range rep.Inactive {
		c.Inact[n] = true
	}
	for _, n := range rep.TeamLeads {
		c.TL[n] = true
	}
	return c
}

func (c *extCtx) role(name string) string { return c.Rep.Roles[name] }

// excluded — тимлиды, ПМы и QA в «Нужны задачи» не считаются (если не включено «показать всех»).
func (c *extCtx) excluded(name string) bool {
	return c.TL[name] || extExclRe.MatchString(c.role(name)) || name == metrics.Unassigned
}

func (c *extCtx) pmLike(name string) bool { return extPMRe.MatchString(c.role(name)) || c.Leads[name] }

// ---------- сотрудники ----------

type extAbsence struct {
	Type   string  `json:"type"` // vac | bs | sick | off
	From   string  `json:"from"`
	To     *string `json:"to"`
	Source string  `json:"source"`
	Note   string  `json:"note,omitempty"`
}

func absenceView(a metrics.Absence, withNote bool) extAbsence {
	v := extAbsence{Type: a.Type, From: a.From, To: a.To, Source: a.Source}
	if withNote {
		v.Note = a.Note
	}
	return v
}

type extPersonProject struct {
	Project   string   `json:"project"`
	Lead      string   `json:"lead"`
	Tasks     extTasks `json:"tasks"`
	StaleTodo int      `json:"stale_backlog"` // в бэклоге без обновлений больше 7 дн.
}

type extEmployeeLoad struct {
	Name         string             `json:"name"`
	Login        string             `json:"login"`
	Position     string             `json:"position"`
	TeamLead     bool               `json:"team_lead"`
	ProjectLead  bool               `json:"project_lead"`
	JiraInactive bool               `json:"jira_inactive"`
	AbsentNow    *extAbsence        `json:"absent_now"`
	Tasks        extTasks           `json:"tasks"` // по всем проектам активных спринтов
	Queue        int                `json:"queue"` // бэклог + в работе + реворк
	NeedTasks    bool               `json:"need_tasks"`
	Overloaded   bool               `json:"overloaded"`
	Projects     []extPersonProject `json:"projects"`
}

// employeeLoads — нагрузка людей в текущих спринтах: по строкам «исполнитель × проект» отчёта.
func (c *extCtx) employeeLoads(needTh, overTh int) []extEmployeeLoad {
	leadOf := map[string]string{}
	for _, p := range c.Rep.Projects {
		leadOf[p.Key] = p.Lead
	}
	by := map[string]*extEmployeeLoad{}
	var order []string
	for _, r := range c.Rep.People {
		if r.Name == metrics.Unassigned {
			continue
		}
		e := by[r.Name]
		if e == nil {
			e = &extEmployeeLoad{Name: r.Name, Login: c.Rep.Logins[r.Name], Position: c.role(r.Name), Projects: []extPersonProject{}}
			by[r.Name] = e
			order = append(order, r.Name)
		}
		t := tasksOfPerson(r)
		e.Tasks.add(t)
		e.Projects = append(e.Projects, extPersonProject{Project: r.ProjectKey, Lead: leadOf[r.ProjectKey], Tasks: t, StaleTodo: r.TodoStale})
	}
	out := make([]extEmployeeLoad, 0, len(order))
	for _, n := range order {
		e := by[n]
		e.TeamLead, e.ProjectLead, e.JiraInactive = c.TL[n], c.Leads[n], c.Inact[n]
		if a, ok := c.Absent[n]; ok {
			v := absenceView(a, false)
			e.AbsentNow = &v
		}
		e.Queue = e.Tasks.queue()
		e.NeedTasks = e.AbsentNow == nil && !e.JiraInactive && e.Queue <= needTh && !c.excluded(n)
		e.Overloaded = e.Tasks.Backlog >= overTh && !(c.TL[n] || c.pmLike(n) || extQARe.MatchString(c.role(n)))
		sort.SliceStable(e.Projects, func(i, j int) bool {
			if e.Projects[i].Tasks.Total != e.Projects[j].Tasks.Total {
				return e.Projects[i].Tasks.Total > e.Projects[j].Tasks.Total
			}
			return e.Projects[i].Project < e.Projects[j].Project
		})
		out = append(out, *e)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ---------- руководители ----------

type extLeadStat struct {
	Name         string   `json:"name"`
	Projects     []string `json:"projects"`
	ProjectCount int      `json:"project_count"`
	Tasks        extTasks `json:"tasks"`
	DonePct      int      `json:"done_pct"`
	Unplanned    int      `json:"unplanned"`
	UnplannedPct int      `json:"unplanned_pct"`
	AddedAfter   int      `json:"added_after_start"`
	Blocked      int      `json:"blocked"`
	Stale        int      `json:"stale"`
	InTest       int      `json:"in_test"`
	OverdueCount int      `json:"overdue_sprints"` // спринтов не закрыто к сроку
	People       int      `json:"people"`
}

// leadStats группирует проекты по руководителю из Jira («Ресурсы по руководителям»).
func (c *extCtx) leadStats() []extLeadStat {
	by := map[string]*extLeadStat{}
	for _, p := range c.Rep.Projects {
		if p.Lead == "" || p.Lead == "—" {
			continue
		}
		s := by[p.Lead]
		if s == nil {
			s = &extLeadStat{Name: p.Lead, Projects: []string{}}
			by[p.Lead] = s
		}
		s.Projects = append(s.Projects, p.Key)
		s.Tasks.add(tasksOfProject(p))
		s.Unplanned += p.Unplanned
		s.AddedAfter += p.Added
		s.Blocked += p.Blocked
		s.Stale += p.Stale
		s.InTest += p.InTest
		if p.LeftDays != nil && *p.LeftDays < 0 {
			s.OverdueCount++
		}
	}
	people := map[string]map[string]bool{}
	for _, r := range c.Rep.People {
		if r.Name == metrics.Unassigned {
			continue
		}
		for _, s := range by {
			for _, k := range s.Projects {
				if k == r.ProjectKey {
					if people[s.Name] == nil {
						people[s.Name] = map[string]bool{}
					}
					people[s.Name][r.Name] = true
				}
			}
		}
	}
	out := make([]extLeadStat, 0, len(by))
	for _, s := range by {
		sort.Strings(s.Projects)
		s.ProjectCount = len(s.Projects)
		s.DonePct = pctOf(s.Tasks.Done, s.Tasks.Total)
		s.UnplannedPct = pctOf(s.Unplanned, s.Tasks.Total)
		s.People = len(people[s.Name])
		out = append(out, *s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Tasks.Total != out[j].Tasks.Total {
			return out[i].Tasks.Total > out[j].Tasks.Total
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// ---------- тестировщики ----------

type extTesterProject struct {
	Project string `json:"project"`
	Tasks   int    `json:"tasks"`    // задач проекта, где человек указан тестировщиком
	InTest  int    `json:"in_test"`  // из них сейчас в статусе Test
	Done    int    `json:"done"`     // из них закрыто
	Waiting int    `json:"waiting"`  // в Test у него по отчёту «Загрузка QA» (задачи текущего спринта, без эпиков)
	Open    int    `json:"open_all"` // не закрыто всего (tasks − done)
}

type extTester struct {
	Name     string             `json:"name"`
	Login    string             `json:"login"`
	Position string             `json:"position"`
	Tasks    int                `json:"tasks"`
	InTest   int                `json:"in_test"`
	Done     int                `json:"done"`
	Open     int                `json:"open"`
	Projects []extTesterProject `json:"projects"`
}

func (c *extCtx) testers() []extTester {
	by := map[string]*extTester{}
	for _, q := range c.Rep.Testers {
		t := by[q.Name]
		if t == nil {
			t = &extTester{Name: q.Name, Login: c.Rep.Logins[q.Name], Position: c.role(q.Name), Projects: []extTesterProject{}}
			by[q.Name] = t
		}
		waiting := 0
		if m := c.Rep.Testing[q.ProjectKey]; m != nil {
			if tc := m[q.Name]; tc != nil {
				waiting = tc.N
			}
		}
		t.Projects = append(t.Projects, extTesterProject{Project: q.ProjectKey, Tasks: q.Total, InTest: q.InTest, Done: q.Dn, Waiting: waiting, Open: q.Total - q.Dn})
		t.Tasks += q.Total
		t.InTest += q.InTest
		t.Done += q.Dn
	}
	out := make([]extTester, 0, len(by))
	for _, t := range by {
		t.Open = t.Tasks - t.Done
		sort.SliceStable(t.Projects, func(i, j int) bool {
			if t.Projects[i].InTest != t.Projects[j].InTest {
				return t.Projects[i].InTest > t.Projects[j].InTest
			}
			return t.Projects[i].Project < t.Projects[j].Project
		})
		out = append(out, *t)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].InTest != out[j].InTest {
			return out[i].InTest > out[j].InTest
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// ---------- посещаемость ----------

type extAttCounts struct {
	Events  map[string]int `json:"events"`  // тип → число записей: late, early, mid, day, sick, vac
	Minutes map[string]int `json:"minutes"` // тип → минуты (late, early, mid, day)
	Total   int            `json:"total_minutes"`
}

type extAttPerson struct {
	Name        string       `json:"name"`
	Login       string       `json:"login"`
	Position    string       `json:"position"`
	TeamLead    bool         `json:"team_lead"`
	Counts      extAttCounts `json:"counts"`
	AbsenceDays int          `json:"absence_days"` // рабочие дни (пн–пт, без праздников) в отпуске, больничном и БС за период
	AbsentNow   *extAbsence  `json:"absent_now"`
}

// weekdays — число рабочих дней (пн–пт) в [a, b], даты YYYY-MM-DD.
func weekdays(a, b string) int {
	x, err1 := time.Parse("2006-01-02", a)
	y, err2 := time.Parse("2006-01-02", b)
	if err1 != nil || err2 != nil || y.Before(x) {
		return 0
	}
	n := 0
	for d := x; !d.After(y); d = d.AddDate(0, 0, 1) {
		if wd := d.Weekday(); wd != time.Saturday && wd != time.Sunday {
			n++
		}
	}
	return n
}

// attendanceByPerson сводит события посещаемости и отсутствия за период [from, to] (YYYY-MM-DD) по людям.
func (c *extCtx) attendanceByPerson(att metrics.Attendance, from, to string) []extAttPerson {
	by := map[string]*extAttPerson{}
	get := func(n string) *extAttPerson {
		p := by[n]
		if p == nil {
			p = &extAttPerson{Name: n, Login: c.Rep.Logins[n], Position: c.role(n), TeamLead: c.TL[n],
				Counts: extAttCounts{Events: map[string]int{}, Minutes: map[string]int{}}}
			if pp, ok := att.Ppl[n]; ok {
				if p.Login == "" {
					p.Login = pp.Login
				}
				if p.Position == "" {
					p.Position = pp.Pos
				}
				p.TeamLead = p.TeamLead || pp.TL
			}
			if a, ok := c.Absent[n]; ok {
				v := absenceView(a, false)
				p.AbsentNow = &v
			}
			by[n] = p
		}
		return p
	}
	for _, e := range att.Ev {
		if e.Date < from || e.Date > to {
			continue
		}
		p := get(e.N)
		p.Counts.Events[e.T]++
		if e.M > 0 && e.T != "upd" {
			p.Counts.Minutes[e.T] += e.M
			p.Counts.Total += e.M
		}
	}
	for _, a := range att.Abs {
		end := to
		if a.To != nil && *a.To < end {
			end = *a.To
		}
		start := from
		if a.From > start {
			start = a.From
		}
		if d := weekdays(start, end); d > 0 {
			get(a.Name).AbsenceDays += d
		}
	}
	out := make([]extAttPerson, 0, len(by))
	for _, p := range by {
		out = append(out, *p)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Counts.Total != out[j].Counts.Total {
			return out[i].Counts.Total > out[j].Counts.Total
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// ---------- успеваемость (помесячно за год) ----------

type extMonth struct {
	Month         string   `json:"month"` // YYYY-MM
	Done          int      `json:"done"`
	Bugs          int      `json:"bugs"`
	Hours         float64  `json:"hours"`
	Carried       int      `json:"carried"`
	Planned       int      `json:"planned"`
	PlannedOnTime int      `json:"planned_on_time"`
	Reworks       int      `json:"reworks"`
	CycleDays     *float64 `json:"cycle_days"` // медиана цикла, дн. (по сотруднику; у сводки — null)
	Reported      int      `json:"reported"`   // создано задач как автор (reporter)
	Tested        int      `json:"tested"`     // закрыто задач как тестировщик
	Late          int      `json:"late"`
	Early         int      `json:"early"`
	Mid           int      `json:"mid"`
	Day           int      `json:"day"`
	Sick          int      `json:"sick"`
	Minutes       int      `json:"minutes"`
	LateStudy     int      `json:"late_study"`
}

func num(v any) float64 {
	switch x := v.(type) {
	case int:
		return float64(x)
	case float64:
		return x
	}
	return 0
}

func monthOf(month string, a []any) extMonth {
	m := extMonth{Month: month}
	if len(a) < 17 {
		return m
	}
	m.Done, m.Bugs, m.Hours, m.Carried = int(num(a[0])), int(num(a[1])), num(a[2]), int(num(a[3]))
	m.Planned, m.PlannedOnTime, m.Reworks = int(num(a[4])), int(num(a[5])), int(num(a[6]))
	if f, ok := a[7].(float64); ok {
		m.CycleDays = &f
	}
	m.Reported, m.Tested = int(num(a[8])), int(num(a[9]))
	m.Late, m.Early, m.Mid, m.Day, m.Sick = int(num(a[10])), int(num(a[11])), int(num(a[12])), int(num(a[13])), int(num(a[14]))
	m.Minutes, m.LateStudy = int(num(a[15])), int(num(a[16]))
	return m
}

func (m *extMonth) add(o extMonth) {
	m.Done += o.Done
	m.Bugs += o.Bugs
	m.Hours += o.Hours
	m.Carried += o.Carried
	m.Planned += o.Planned
	m.PlannedOnTime += o.PlannedOnTime
	m.Reworks += o.Reworks
	m.Reported += o.Reported
	m.Tested += o.Tested
	m.Late += o.Late
	m.Early += o.Early
	m.Mid += o.Mid
	m.Day += o.Day
	m.Sick += o.Sick
	m.Minutes += o.Minutes
	m.LateStudy += o.LateStudy
}

type extPerfOut struct {
	Done          int        `json:"done"`
	Hours         float64    `json:"hours"`
	Planned       int        `json:"planned"`
	PlannedOnTime int        `json:"planned_on_time"`
	OnTimePct     int        `json:"on_time_pct"`
	Reworks       int        `json:"reworks"`
	ReworkPct     int        `json:"rework_pct"`
	Bugs          int        `json:"bugs"`
	Carried       int        `json:"carried"`
	Minutes       int        `json:"attendance_minutes"`
	Months        []extMonth `json:"months,omitempty"`
}

func perfOf(months []extMonth, withMonths bool) extPerfOut {
	var t extMonth
	for _, m := range months {
		t.add(m)
	}
	o := extPerfOut{Done: t.Done, Hours: round1(t.Hours), Planned: t.Planned, PlannedOnTime: t.PlannedOnTime,
		OnTimePct: pctOf(t.PlannedOnTime, t.Planned), Reworks: t.Reworks, ReworkPct: pctOf(t.Reworks, t.Done),
		Bugs: t.Bugs, Carried: t.Carried, Minutes: t.Minutes}
	if withMonths {
		o.Months = months
	}
	return o
}

func round1(v float64) float64 { return float64(int(v*10+0.5)) / 10 }

// employeeMonths — помесячные показатели сотрудника за год (в порядке месяцев).
func employeeMonths(emp metrics.Employees, row metrics.EmpRow) []extMonth {
	out := make([]extMonth, 0, len(emp.Mon))
	for _, m := range emp.Mon {
		out = append(out, monthOf(m, row.M[m]))
	}
	return out
}

// orgMonths — сводка по всем сотрудникам помесячно.
func orgMonths(emp metrics.Employees) []extMonth {
	out := make([]extMonth, 0, len(emp.Mon))
	for _, m := range emp.Mon {
		t := extMonth{Month: m}
		for _, r := range emp.Ppl {
			t.add(monthOf(m, r.M[m]))
		}
		t.Hours = round1(t.Hours)
		out = append(out, t)
	}
	return out
}

// ---------- сигналы дэшборда ----------

type extSignal struct {
	Text     string `json:"text"`
	Severity int    `json:"severity"` // 1 — внимание, 2 — важно, 3 — тревога
}

type extRisk struct {
	Project string      `json:"project"`
	Name    string      `json:"name"`
	Lead    string      `json:"lead"`
	DonePct int         `json:"done_pct"`
	TimePct int         `json:"time_pct"`
	Score   int         `json:"score"`
	Reasons []extSignal `json:"reasons"`
}

// projectRisks — те же правила, что «Риски по спринтам» в дэшбордах и на ТВ: не закрыт в срок, отстаёт, Blocked, нет следующего спринта.
func (c *extCtx) projectRisks() []extRisk {
	out := []extRisk{}
	for _, p := range c.Rep.Projects {
		var rs []extSignal
		left := 0
		if p.LeftDays != nil {
			left = *p.LeftDays
		}
		if p.LeftDays != nil && left < 0 {
			rs = append(rs, extSignal{Text: "Спринт не закрыт, просрочен дней: " + strconv.Itoa(-left), Severity: 3})
		}
		if p.TimePct-p.ReadyPct > extLag {
			rs = append(rs, extSignal{Text: "Отстаёт: готово " + strconv.Itoa(p.ReadyPct) + "% при прошедших " + strconv.Itoa(p.TimePct) + "% времени", Severity: 2})
		}
		if p.Blocked > 0 {
			sev := 1
			if p.Blocked >= 3 {
				sev = 2
			}
			rs = append(rs, extSignal{Text: "Задач в Blocked: " + strconv.Itoa(p.Blocked), Severity: sev})
		}
		if ns, ok := c.Rep.Next[p.Key]; (!ok || ns.NextTotal == 0) && p.LeftDays != nil && left <= 3 {
			rs = append(rs, extSignal{Text: "Нет следующего спринта", Severity: 1})
		}
		if len(rs) == 0 {
			continue
		}
		score := 0
		for _, r := range rs {
			score += r.Severity
		}
		out = append(out, extRisk{Project: p.Key, Name: p.Name, Lead: p.Lead, DonePct: p.ReadyPct, TimePct: p.TimePct, Score: score, Reasons: rs})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if len(out[i].Reasons) != len(out[j].Reasons) {
			return len(out[i].Reasons) > len(out[j].Reasons)
		}
		return out[i].Project < out[j].Project
	})
	return out
}

type extLowSprint struct {
	Project  string `json:"project"`
	Name     string `json:"name"`
	Lead     string `json:"lead"`
	Sprint   string `json:"sprint"`
	DonePct  int    `json:"done_pct"`
	TimePct  int    `json:"time_pct"`
	DaysLeft *int   `json:"days_left"`
	Level    string `json:"level"` // bad | warn
}

// lowSprints — спринты с низким процентом закрытия (прошло не меньше 20% времени), самые низкие сверху.
func (c *extCtx) lowSprints() []extLowSprint {
	out := []extLowSprint{}
	for _, p := range c.Rep.Projects {
		if p.Total == 0 || p.TimePct < extMinTime {
			continue
		}
		lag := p.TimePct - p.ReadyPct
		level := ""
		switch {
		case p.ReadyPct < extDoneBad || lag > extLag:
			level = "bad"
		case p.ReadyPct < extDoneWarn || lag > extLagWarn:
			level = "warn"
		}
		if level != "" {
			out = append(out, extLowSprint{Project: p.Key, Name: p.Name, Lead: p.Lead, Sprint: p.Sprint, DonePct: p.ReadyPct, TimePct: p.TimePct, DaysLeft: p.LeftDays, Level: level})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DonePct != out[j].DonePct {
			return out[i].DonePct < out[j].DonePct
		}
		if out[i].TimePct != out[j].TimePct {
			return out[i].TimePct > out[j].TimePct
		}
		return out[i].Project < out[j].Project
	})
	return out
}

type extSpike struct {
	Project   string `json:"project"`
	Date      string `json:"date"`
	Created   int    `json:"created"`
	Unplanned int    `json:"unplanned"`
}

// creationSpikes — дни, когда в спринт проекта «залетело» много новых задач (от extSpike за день), крупнейшие первыми.
func (c *extCtx) creationSpikes() []extSpike {
	out := []extSpike{}
	for _, p := range c.Rep.Projects {
		cd := p.CreatedDaily
		if cd == nil {
			continue
		}
		start, err := time.Parse("2006-01-02", cd.From)
		if err != nil {
			continue
		}
		for i, n := range cd.Total {
			if n >= extSpikeMin {
				u := 0
				if i < len(cd.Unplanned) {
					u = min(n, cd.Unplanned[i])
				}
				out = append(out, extSpike{Project: p.Key, Date: start.AddDate(0, 0, i).Format("2006-01-02"), Created: n, Unplanned: u})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Created != out[j].Created {
			return out[i].Created > out[j].Created
		}
		return out[i].Project < out[j].Project
	})
	return out
}

// ---------- контроль руководителей: долгие задачи ----------

type extLongTask struct {
	Key      string `json:"key"`
	Project  string `json:"project"`
	Status   string `json:"status,omitempty"`
	Assignee string `json:"assignee"`
	Tester   string `json:"tester,omitempty"`
	Days     int    `json:"days"`
	Summary  string `json:"summary"`
}

// longTasks собирает «долго в работе» (от 5 дн.), «долго на тесте» (от 3 дн.) и заблокированные по всем руководителям;
// один проект может быть у нескольких руководителей — задача не считается дважды.
func longTasks(reps []leadcontrol.Report) (inWork, testing, blocked []extLongTask) {
	seenW, seenT, seenB := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, r := range reps {
		for _, x := range r.InWork {
			d := x.DaysInStatus
			if x.DaysSinceStart != nil {
				d = *x.DaysSinceStart
			}
			if d >= extLongWork && !seenW[x.Key] {
				seenW[x.Key] = true
				inWork = append(inWork, extLongTask{Key: x.Key, Project: projectOfKey(x.Key), Status: x.Status, Assignee: x.Assignee, Days: d, Summary: x.Summary})
			}
		}
		for _, x := range r.Testing {
			if x.DaysInTest >= extLongTest && !seenT[x.Key] {
				seenT[x.Key] = true
				testing = append(testing, extLongTask{Key: x.Key, Project: projectOfKey(x.Key), Status: "Test", Assignee: x.Assignee, Tester: x.Tester, Days: x.DaysInTest, Summary: x.Summary})
			}
		}
		for _, x := range r.Blocked {
			if !seenB[x.Key] {
				seenB[x.Key] = true
				blocked = append(blocked, extLongTask{Key: x.Key, Project: projectOfKey(x.Key), Status: "Blocked", Assignee: x.Assignee, Days: x.DaysBlocked, Summary: x.Summary})
			}
		}
	}
	byDays := func(a []extLongTask) {
		sort.SliceStable(a, func(i, j int) bool {
			if a[i].Days != a[j].Days {
				return a[i].Days > a[j].Days
			}
			return a[i].Key < a[j].Key
		})
	}
	byDays(inWork)
	byDays(testing)
	byDays(blocked)
	return
}

func projectOfKey(key string) string {
	if i := strings.LastIndex(key, "-"); i > 0 {
		return key[:i]
	}
	return key
}
