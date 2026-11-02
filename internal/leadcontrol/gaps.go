package leadcontrol

import (
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"time"
)

// ---------- 4. Дизайн или бэк готов, а продолжения нет ----------

type GapRow struct {
	rowBase
	Key      string `json:"key"`
	Priority string `json:"priority"` // Будущий спринт | Высокий | Проверить | Низкий
	Kind     string `json:"kind"`     // Дизайн | Бэк
	Status   string `json:"status"`
	Assignee string `json:"assignee"`
	Missing  string `json:"missing"` // Нет фронта и бэка | Нет фронта | Нет бэка
	Problem  string `json:"problem"`
	Front    string `json:"front"`
	Back     string `json:"back"`
	Sprint   string `json:"sprint"`
	Summary  string `json:"summary"`
	// CreateURL — форма создания продолжения в Jira: тот же проект и тип, название «Фронт - …» (или «БЭК - …»).
	CreateURL string `json:"create_url"`
}

// IgnoredRow — задача, скрытая из «Разрывов» («продолжение не нужно»); её можно вернуть.
type IgnoredRow struct {
	rowBase
	Key     string    `json:"key"`
	Summary string    `json:"summary"`
	By      string    `json:"by"`
	At      time.Time `json:"at"`
}

// createURL — ссылка на форму создания задачи в Jira Server с заполненными проектом, типом, названием и описанием.
func createURL(in *Input, is *Issue, noFront bool) string {
	if in.JiraBase == "" || is.ProjectID == "" {
		return ""
	}
	prefix := "Фронт - "
	if !noFront {
		prefix = "БЭК - "
	}
	q := url.Values{"pid": {is.ProjectID}, "summary": {prefix + strings.TrimSpace(rePrefix.ReplaceAllString(is.Summary, ""))},
		"description": {"Продолжение задачи " + is.Key + ": " + is.Summary}}
	if is.TypeID != "" {
		q.Set("issuetype", is.TypeID)
	}
	return strings.TrimRight(in.JiraBase, "/") + "/secure/CreateIssueDetails!init.jspa?" + q.Encode()
}

const (
	PrioFuture = "Будущий спринт"
	PrioHigh   = "Высокий"
	PrioCheck  = "Проверить"
	PrioLow    = "Низкий"

	missBoth  = "Нет фронта и бэка"
	missFront = "Нет фронта"
	missBack  = "Нет бэка"

	// Похожее название: коэффициент Дайса по словам от 3 букв не меньше 0,5 — «похоже N%», проверить вручную.
	SimilarMin = 0.5
)

// pair — найденная парная задача.
type pair struct {
	is  *Issue
	how string // relate | title | similar
	pct int    // для similar
}

// where — где парная задача: present (в открытом спринте или уже на тесте/готова), future (в будущем спринте), backlog.
func where(is *Issue) string {
	switch {
	case is.InOpenSprint || atTest(is.Status):
		return "present"
	case is.InFutureSprint:
		return "future"
	}
	return "backlog"
}

var whereText = map[string]string{"present": "текущий спринт", "future": "будущий спринт", "backlog": "вне спринта"}

// findPair ищет задачу роли k к задаче is: по связи Relate, по одинаковому названию после префикса, по похожему названию.
// Из нескольких точных совпадений берётся та, что ближе к делу: в спринте или готова, затем будущий спринт, затем бэклог.
func findPair(in *Input, is *Issue, k string) *pair {
	var cands []*Issue
	for _, c := range in.Issues {
		if c.Key != is.Key && c.Project == is.Project && c.Type != "Epic" && kind(Role(c)) == k {
			cands = append(cands, c)
		}
	}
	sort.Slice(cands, func(i, j int) bool { return keyLess(cands[i].Key, cands[j].Key) })
	best := func(xs []*Issue) *Issue {
		order := map[string]int{"present": 0, "future": 1, "backlog": 2}
		var b *Issue
		for _, x := range xs {
			if b == nil || order[where(x)] < order[where(b)] {
				b = x
			}
		}
		return b
	}
	var rel, same []*Issue
	core := titleCore(is.Summary)
	for _, c := range cands {
		if relates(is, c) || relates(c, is) {
			rel = append(rel, c)
		} else if core != "" && titleCore(c.Summary) == core {
			same = append(same, c)
		}
	}
	if b := best(rel); b != nil {
		return &pair{b, "relate", 100}
	}
	if b := best(same); b != nil {
		return &pair{b, "title", 100}
	}
	var sim *Issue
	bestS := 0.0
	for _, c := range cands {
		if s := similarity(is.Summary, c.Summary); s >= SimilarMin && s > bestS {
			sim, bestS = c, s
		}
	}
	if sim != nil {
		return &pair{sim, "similar", int(math.Round(bestS * 100))}
	}
	return nil
}

func relates(a, b *Issue) bool {
	for _, l := range a.Links {
		if l.Key == b.Key && l.Type == "Relate" {
			return true
		}
	}
	return false
}

func describe(p *pair) string {
	if p == nil {
		return ""
	}
	s := fmt.Sprintf("%s · %s · %s", p.is.Key, p.is.Status, whereText[where(p.is)])
	if p.how == "similar" {
		s = fmt.Sprintf("похоже %d%%: %s", p.pct, s)
	}
	return s
}

// lacks — пары нет в работе: не найдена, найдена только похожая, в будущем спринте или в бэклоге.
func lacks(p *pair) bool { return p == nil || p.how == "similar" || where(p.is) != "present" }

// Gaps — готовые (Test или дальше) задачи дизайна и бэка текущего спринта без парной задачи на фронт и/или бэк.
// Готовый фронт без бэка проблемой не считается.
func Gaps(in *Input) []GapRow {
	out := []GapRow{}
	for _, is := range in.sprintIssues() {
		k := kind(Role(is))
		if (k != roleDesign && k != roleBack) || !atTest(is.Status) {
			continue
		}
		if _, ok := in.Ignored[is.Key]; ok {
			continue
		}
		front := findPair(in, is, roleFront)
		var back *pair
		if k == roleDesign {
			back = findPair(in, is, roleBack)
		}
		noFront, noBack := lacks(front), k == roleDesign && lacks(back)
		if !noFront && !noBack {
			continue
		}
		r := GapRow{rowBase: rowBase{in.Lead, is.Project}, Key: is.Key, Kind: k, Status: is.Status, Assignee: is.Assignee,
			Front: describe(front), Back: describe(back), Sprint: activeSprint(is), Summary: is.Summary}
		switch {
		case noFront && noBack:
			r.Missing = missBoth
		case noFront:
			r.Missing = missFront
		default:
			r.Missing = missBack
		}
		r.Priority = priority(is, k, front, back, noFront, noBack)
		r.Problem = problem(k, front, back, noFront, noBack)
		r.CreateURL = createURL(in, is, noFront)
		out = append(out, r)
	}
	return out
}

func similarOnly(p *pair) bool { return p != nil && p.how == "similar" }

// priority — см. docs/lead-control.md, «Решения»:
//   - исправления ошибок — «Низкий»;
//   - нашлась только похожая задача — «Проверить»;
//   - фронт есть, но только в будущем спринте — «Будущий спринт»;
//   - бэк готов без фронта — «Проверить»;
//   - дизайн: нет фронта, бэк есть — «Высокий»; нет ни фронта, ни бэка (дизайн-задача без разработки) и нет бэка — «Низкий».
func priority(is *Issue, k string, front, back *pair, noFront, noBack bool) string {
	switch {
	case is.Type == "Ошибка":
		return PrioLow
	case (noFront && similarOnly(front)) || (noBack && similarOnly(back)):
		return PrioCheck
	case noFront && front != nil && where(front.is) == "future":
		return PrioFuture
	case noFront && k == roleBack:
		return PrioCheck
	case noFront && !noBack:
		return PrioHigh
	}
	return PrioLow
}

func problem(k string, front, back *pair, noFront, noBack bool) string {
	what := map[string]string{roleDesign: "Дизайн готов", roleBack: "Бэк готов"}[k]
	part := func(role string, p *pair) string {
		switch {
		case p == nil:
			return "задачи на " + role + " нет"
		case p.how == "similar":
			return fmt.Sprintf("на %s найдена только похожая задача %s (%d%%) — проверить", role, p.is.Key, p.pct)
		case where(p.is) == "future":
			return fmt.Sprintf("%s %s только в будущем спринте", role, p.is.Key)
		}
		return fmt.Sprintf("%s %s вне спринта", role, p.is.Key)
	}
	var ps []string
	if noFront {
		ps = append(ps, part("фронт", front))
	}
	if noBack {
		ps = append(ps, part("бэк", back))
	}
	return what + ": " + strings.Join(ps, "; ")
}

// ---------- итоги, спринты, отчёт ----------

// Totals — счётчики плиток и вывода по руководителю: по всем проектам (project = "all") и по каждому.
type Totals struct {
	Lead          string `json:"lead"`
	Project       string `json:"project"`
	Blocked       int    `json:"blocked"`
	BlockedWait   int    `json:"blocked_wait"` // ждут задачу, которая ещё не начата
	CanUnblock    int    `json:"can_unblock"`
	LongWork      int    `json:"long_work"`
	LongTest      int    `json:"long_test"`
	StalledPeople int    `json:"stalled_people"`
	GapsAll       int    `json:"gaps_all"`
	GapsHigh      int    `json:"gaps_high"`
}

// SprintRow — полоса статусов открытого спринта.
type SprintRow struct {
	Lead     string `json:"lead"`
	Project  string `json:"project"`
	Sprint   string `json:"sprint"`
	Total    int    `json:"total"`
	Done     int    `json:"done"`
	Ready    int    `json:"ready"`
	Test     int    `json:"test"`
	Review   int    `json:"review"`
	InWork   int    `json:"in_work"`
	Rework   int    `json:"rework"`
	Blocked  int    `json:"blocked"`
	Todo     int    `json:"todo"`
	Snapshot string `json:"snapshot"` // время данных Jira, Алматы
}

type Report struct {
	UpdatedAt time.Time     `json:"updated_at"`
	Lead      string        `json:"lead"`
	Projects  []LeadProject `json:"projects"`
	Totals    []Totals      `json:"totals"`
	Sprints   []SprintRow   `json:"sprints"`
	Blocked   []BlockedRow  `json:"blocked"`
	InWork    []InWorkRow   `json:"in_work"`
	People    []PeopleRow   `json:"people"`
	Gaps      []GapRow      `json:"gaps"`
	Testing   []TestingRow  `json:"testing"`
	Ignored   []IgnoredRow  `json:"ignored"`
}

// IgnoredOf — скрытые задачи руководителя (по проектам руководителя), новые сверху.
func IgnoredOf(in *Input) []IgnoredRow {
	out := []IgnoredRow{}
	for key, ig := range in.Ignored {
		is := in.Issues[key]
		if is == nil || !in.leadProject(is.Project) {
			continue
		}
		out = append(out, IgnoredRow{rowBase: rowBase{in.Lead, is.Project}, Key: key, Summary: is.Summary, By: ig.By, At: ig.At})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

// Sprints — по каждому открытому спринту проектов руководителя: сколько задач в каком статусе.
func Sprints(in *Input, updatedAt time.Time) []SprintRow {
	rows := map[[2]string]*SprintRow{}
	for _, is := range in.sprintIssues() {
		sp := activeSprint(is)
		if sp == "" {
			continue
		}
		k := [2]string{is.Project, sp}
		r := rows[k]
		if r == nil {
			r = &SprintRow{Lead: in.Lead, Project: is.Project, Sprint: sp, Snapshot: updatedAt.In(almaty).Format("02.01.2006 15:04")}
			rows[k] = r
		}
		r.Total++
		switch {
		case is.Status == stBlocked:
			r.Blocked++
		case rank(is.Status) == 5:
			r.Done++
		case rank(is.Status) == 4:
			r.Ready++
		case is.Status == stReview:
			r.Review++
		case is.Status == stWork:
			r.InWork++
		case is.Status == stRework:
			r.Rework++
		case rank(is.Status) == 0:
			r.Todo++
		default:
			r.Test++
		}
	}
	out := make([]SprintRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, *r)
	}
	pos := map[string]int{}
	for i, p := range in.Projects {
		pos[p.Key] = i
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Project != out[j].Project {
			return pos[out[i].Project] < pos[out[j].Project]
		}
		return out[i].Sprint < out[j].Sprint
	})
	return out
}

// TotalsOf — счётчики по всем проектам руководителя и по каждому.
func TotalsOf(in *Input, rep *Report) []Totals {
	keys := []string{"all"}
	for _, p := range in.Projects {
		keys = append(keys, p.Key)
	}
	out := make([]Totals, 0, len(keys))
	for _, p := range keys {
		m := func(project string) bool { return p == "all" || project == p }
		t := Totals{Lead: in.Lead, Project: p}
		for _, r := range rep.Blocked {
			if m(r.Project) {
				t.Blocked++
				if r.NotStarted == "да" {
					t.BlockedWait++
				}
				if strings.HasPrefix(r.BlockState, StateCanUnblock) {
					t.CanUnblock++
				}
			}
		}
		for _, r := range rep.InWork {
			if m(r.Project) && r.DaysInStatus >= LongWorkDays {
				t.LongWork++
			}
		}
		for _, r := range rep.Testing {
			if m(r.Project) && r.DaysInTest >= LongTestDays {
				t.LongTest++
			}
		}
		for _, r := range rep.People {
			if r.Signal == SignalStalls && (p == "all" || contains(strings.Fields(r.Projects), p)) {
				t.StalledPeople++
			}
		}
		for _, r := range rep.Gaps {
			if m(r.Project) {
				t.GapsAll++
				if r.Priority == PrioHigh || r.Priority == PrioFuture {
					t.GapsHigh++
				}
			}
		}
		out = append(out, t)
	}
	return out
}

// Build — весь отчёт по руководителю.
func Build(in *Input, updatedAt time.Time) Report {
	rep := Report{UpdatedAt: updatedAt, Lead: in.Lead, Projects: in.Projects,
		Blocked: Blocked(in), InWork: InWork(in), People: People(in), Gaps: Gaps(in), Testing: Testing(in), Sprints: Sprints(in, updatedAt),
		Ignored: IgnoredOf(in)}
	rep.Totals = TotalsOf(in, &rep)
	if rep.Projects == nil {
		rep.Projects = []LeadProject{}
	}
	return rep
}
