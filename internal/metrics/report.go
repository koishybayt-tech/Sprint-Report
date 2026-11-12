package metrics

import (
	"math"
	"sort"
	"strings"
	"time"
)

// Unassigned — имя строки для задач без исполнителя (как в reference/sprints/inc.js).
const Unassigned = "— не назначен"

// PersonProject — строка «исполнитель × проект» (AX в inc.js). -1 — значения нет.
type PersonProject struct {
	Name       string `json:"name"`
	ProjectKey string `json:"project_key"`
	Total      int    `json:"t"`
	Todo       int    `json:"todo"`
	Wk         int    `json:"wk"`
	Cr         int    `json:"cr"`
	Rw         int    `json:"rw"`
	Ts         int    `json:"ts"`
	Dn         int    `json:"dn"`
	Bug        int    `json:"bug"`
	TodoStale  int    `json:"todo_stale"` // в бэклоге и без обновлений > 7 дн.
	ActDays    int    `json:"act_days"`   // дней с последнего движения задач в работе/ревью/реворке
	AnyDays    int    `json:"any_days"`   // дней с последнего обновления любой незакрытой
	TodoAge    int    `json:"todo_age"`   // возраст самой старой задачи в бэклоге, дн.
	TodoUpd    int    `json:"todo_upd"`   // дней с последнего обновления задачи в бэклоге
}

// TesterProject — строка «тестировщик × проект» по полю «Тестировщик» (QX в inc.js).
type TesterProject struct {
	Name       string `json:"name"`
	ProjectKey string `json:"project_key"`
	Total      int    `json:"t"`
	InTest     int    `json:"tst"`
	Dn         int    `json:"dn"`
}

// Bottleneck — незакрытая задача в Blocked / Need Rework / «Требует уточнения» или без обновлений > 30 дн. (X в inc.js).
type Bottleneck struct {
	Key      string `json:"key"`
	Summary  string `json:"summary"`
	Status   string `json:"status"`
	Assignee string `json:"assignee"`
	Days     int    `json:"days"`
	Priority string `json:"priority"`
}

// TestingCount — задачи в Test текущего спринта у одного тестировщика (TST из tst_fetch.js).
type TestingCount struct {
	N      int      `json:"n"`
	Logins []string `json:"logins"`
}

// SprintIssue — задача активного или будущего спринта для разбора следующего спринта.
type SprintIssue struct {
	SprintIDs []int64
	Status    string
	Assignee  string
}

type NameCount struct {
	Name string `json:"name"`
	N    int    `json:"n"`
}

// SprintReport — все данные отчёта «Спринты» на момент синхронизации.
type SprintReport struct {
	UpdatedAt   time.Time                           `json:"updated_at"`
	Projects    []ProjectSprint                     `json:"projects"`
	People      []PersonProject                     `json:"people"`
	Testers     []TesterProject                     `json:"testers"`
	Bottlenecks []Bottleneck                        `json:"bottlenecks"`
	Roles       map[string]string                   `json:"roles"`  // POS: самая частая «Должность» по задачам человека
	Logins      map[string]string                   `json:"logins"` // U: имя → логин Jira
	TeamLeads   []string                            `json:"team_leads"`
	Inactive    []string                            `json:"inactive"`    // INACT: деактивированные в Jira исполнители
	Absences    []Absence                           `json:"absences"`    // ABS: текущие и будущие отсутствия
	AbsencesAt  map[string]time.Time                `json:"absences_at"` // ABSAT: последняя синхронизация по источнику
	Testing     map[string]map[string]*TestingCount `json:"testing"`     // проект → тестировщик ("" — пусто) → задачи в Test
	Next        map[string]NextSprint               `json:"next"`
	// Queue — задачи «Сделать / В работе / Rework» вне активных спринтов (будущий/закрытый спринт, без спринта): имя → число.
	// Входят в очередь «Нужны задачи» наравне с задачами активных спринтов.
	Queue map[string]int `json:"queue"`
}

var bottleneckStatus = map[string]bool{"Blocked": true, "Need Rework": true, "Требует уточнения": true}

// BuildSprintReport считает отчёт 1-в-1 как inc.js (PJ, AX, QX, X, POS, U), tst_fetch.js (TST) и ns_fetch.js (NS).
func BuildSprintReport(issues []Issue, leads map[string]string, teamLeads []string,
	futures map[string][]Sprint, sprintIssues []SprintIssue, asOf time.Time) SprintReport {
	iss := append([]Issue(nil), issues...)
	sort.SliceStable(iss, func(a, b int) bool {
		if !iss[a].Created.Equal(iss[b].Created) {
			return iss[a].Created.Before(iss[b].Created)
		}
		return iss[a].Key < iss[b].Key
	})
	dd := func(t time.Time) int { return int(math.Floor(asOf.Sub(t).Hours() / 24)) }

	r := SprintReport{UpdatedAt: asOf, Projects: ProjectSprints(issues, leads, asOf),
		Roles: map[string]string{}, Logins: map[string]string{}, TeamLeads: teamLeads,
		Testing: map[string]map[string]*TestingCount{}, Next: map[string]NextSprint{}}
	if r.TeamLeads == nil {
		r.TeamLeads = []string{}
	}
	cur := map[string]string{}
	for _, p := range r.Projects {
		cur[p.Key] = p.Sprint
	}

	ax := map[string]*PersonProject{}
	qx := map[string]*TesterProject{}
	type posCount struct {
		order []string
		n     map[string]int
	}
	posc := map[string]*posCount{}
	for _, f := range iss {
		g := StatusGroup(f.StatusID)
		up, cr := dd(f.Updated), dd(f.Created)
		n := f.Assignee
		if n == "" {
			n = Unassigned
		} else {
			r.Logins[n] = f.AssigneeLogin
			if f.AssigneeInactive && !contains(r.Inactive, n) {
				r.Inactive = append(r.Inactive, n)
			}
		}
		if f.Position != "" {
			c := posc[n]
			if c == nil {
				c = &posCount{n: map[string]int{}}
				posc[n] = c
			}
			if c.n[f.Position] == 0 {
				c.order = append(c.order, f.Position)
			}
			c.n[f.Position]++
		}

		x := ax[n+"|"+f.ProjectKey]
		if x == nil {
			x = &PersonProject{Name: n, ProjectKey: f.ProjectKey, ActDays: -1, AnyDays: -1, TodoUpd: -1}
			ax[n+"|"+f.ProjectKey] = x
		}
		x.Total++
		switch g {
		case Todo:
			x.Todo++
			if up > 7 {
				x.TodoStale++
			}
			x.TodoAge = max(x.TodoAge, cr)
			x.TodoUpd = minSet(x.TodoUpd, up)
		case Wk:
			x.Wk++
		case Cr:
			x.Cr++
		case Rw:
			x.Rw++
		case Dn:
			x.Dn++
		default:
			x.Ts++
		}
		if f.Type == "Ошибка" {
			x.Bug++
		}
		if g == Wk || g == Cr || g == Rw {
			x.ActDays = minSet(x.ActDays, up)
		}
		if g != Dn {
			x.AnyDays = minSet(x.AnyDays, up)
		}

		if f.Tester != "" {
			if _, ok := r.Logins[f.Tester]; !ok {
				r.Logins[f.Tester] = f.TesterLogin
			}
			q := qx[f.Tester+"|"+f.ProjectKey]
			if q == nil {
				q = &TesterProject{Name: f.Tester, ProjectKey: f.ProjectKey}
				qx[f.Tester+"|"+f.ProjectKey] = q
			}
			q.Total++
			if f.StatusName == "Test" {
				q.InTest++
			}
			if g == Dn {
				q.Dn++
			}
		}

		if g != Dn && (bottleneckStatus[f.StatusName] || up > 30) {
			r.Bottlenecks = append(r.Bottlenecks, Bottleneck{Key: f.Key, Summary: truncRunes(f.Summary, 90),
				Status: f.StatusName, Assignee: n, Days: up, Priority: f.Priority})
		}

		// TST: задачи в Test текущего спринта проекта, без эпиков, по полю «Тестировщик».
		if c := cur[f.ProjectKey]; c != "" && f.StatusName == "Test" && f.Type != "Epic" && inActiveSprint(f, c) {
			t := strings.TrimSpace(f.Tester)
			m := r.Testing[f.ProjectKey]
			if m == nil {
				m = map[string]*TestingCount{}
				r.Testing[f.ProjectKey] = m
			}
			e := m[t]
			if e == nil {
				e = &TestingCount{Logins: []string{}}
				m[t] = e
			}
			e.N++
			if f.TesterLogin != "" && !contains(e.Logins, f.TesterLogin) {
				e.Logins = append(e.Logins, f.TesterLogin)
			}
		}
	}
	for n, c := range posc {
		best := c.order[0]
		for _, p := range c.order[1:] {
			if c.n[p] > c.n[best] {
				best = p
			}
		}
		r.Roles[n] = best
	}
	for _, x := range ax {
		r.People = append(r.People, *x)
	}
	sort.Slice(r.People, func(a, b int) bool {
		if r.People[a].Name != r.People[b].Name {
			return r.People[a].Name < r.People[b].Name
		}
		return r.People[a].ProjectKey < r.People[b].ProjectKey
	})
	for _, q := range qx {
		r.Testers = append(r.Testers, *q)
	}
	sort.Slice(r.Testers, func(a, b int) bool {
		if r.Testers[a].Name != r.Testers[b].Name {
			return r.Testers[a].Name < r.Testers[b].Name
		}
		return r.Testers[a].ProjectKey < r.Testers[b].ProjectKey
	})
	sort.SliceStable(r.Bottlenecks, func(a, b int) bool { return r.Bottlenecks[a].Days > r.Bottlenecks[b].Days })
	if r.People == nil {
		r.People = []PersonProject{}
	}
	if r.Testers == nil {
		r.Testers = []TesterProject{}
	}
	sort.Strings(r.Inactive)
	if r.Inactive == nil {
		r.Inactive = []string{}
	}
	if r.Bottlenecks == nil {
		r.Bottlenecks = []Bottleneck{}
	}

	// NS: следующий спринт, его задачи по статусам и исполнителям.
	for _, p := range r.Projects {
		n := PickNext(p.Sprint, futures[p.Key])
		if n == nil {
			continue
		}
		ns := NextSprint{ProjectKey: p.Key, Current: p.Sprint, NextID: &n.ID, NextName: &n.Name,
			NextStart: dateOf(n.Start), NextEnd: dateOf(n.End), Statuses: []NameCount{}, By: []NameCount{}}
		st, by := map[string]int{}, map[string]int{}
		for _, si := range sprintIssues {
			if !containsID(si.SprintIDs, n.ID) {
				continue
			}
			ns.NextTotal++
			st[si.Status]++
			a := si.Assignee
			if a == "" {
				a = Unassigned
			}
			by[a]++
		}
		ns.Statuses, ns.By = sortedCounts(st), sortedCounts(by)
		r.Next[p.Key] = ns
	}
	return r
}

// inActiveSprint: среди активных спринтов задачи есть текущий спринт проекта (tst_fetch.js).
func inActiveSprint(f Issue, name string) bool {
	for _, s := range f.Sprints {
		if s.State == "ACTIVE" && s.Name == name {
			return true
		}
	}
	return false
}

func minSet(cur, v int) int {
	if cur < 0 {
		return v
	}
	return min(cur, v)
}

// truncRunes обрезает как String.slice в JS (по символам; суррогатных пар в наших текстах не ждём).
func truncRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func containsID(xs []int64, id int64) bool {
	for _, x := range xs {
		if x == id {
			return true
		}
	}
	return false
}

func sortedCounts(m map[string]int) []NameCount {
	out := make([]NameCount, 0, len(m))
	for k, v := range m {
		out = append(out, NameCount{k, v})
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].N != out[b].N {
			return out[a].N > out[b].N
		}
		return out[a].Name < out[b].Name
	})
	return out
}
