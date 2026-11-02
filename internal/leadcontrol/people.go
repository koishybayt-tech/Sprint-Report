package leadcontrol

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ---------- 3. Сотрудники: задачи руководителя стоят, у другого двигаются ----------

type PeopleRow struct {
	Lead             string `json:"lead"`
	Person           string `json:"person"`
	Role             string `json:"role"`
	Signal           string `json:"signal"` // «Тормозит» | ""
	MyStale          int    `json:"my_stale"`
	MyOpen           int    `json:"my_open"`
	MyActions7d      int    `json:"my_actions_7d"`
	MyLastActionDays *int   `json:"my_last_action_days"`
	OtherStarted7d   int    `json:"other_started_7d"`
	OtherFinished7d  int    `json:"other_finished_7d"`
	MyTasks          string `json:"my_tasks"`       // задачи здесь (дн. без движения)
	OtherProjects    string `json:"other_projects"` // другие проекты (руководитель) — действий
	OtherTasks       string `json:"other_tasks"`    // что делал у других
	Projects         string `json:"projects"`       // проекты руководителя, где у него открытые задачи, через пробел
}

const (
	SignalStalls = "Тормозит"
	StaleDays    = 3 // без смены статуса столько дней — задача «стоит»
	actionWindow = 7 * 24 * time.Hour
)

// Тестировщики и ПМ не помечаются.
var reNoSignal = regexp.MustCompile(`(?i)qa|тест|test|manager|менеджер`)

type action struct {
	at      time.Time
	key     string
	project string
	started bool // «взял» — перевёл в «В работе»; иначе «завершил» — в Code review, Test или дальше
}

// actionOf — действие сотрудника по переходу статуса (только «взял» и «завершил»).
func actionOf(c Change) (started, ok bool) {
	switch {
	case c.To == stWork:
		return true, true
	case rank(c.To) >= 2 && rank(c.From) < 2:
		return false, true
	}
	return false, false
}

// People — по исполнителям открытых задач руководителя: стоят ли его задачи, пока человек работает у других.
func People(in *Input) []PeopleRow {
	since := in.Now.Add(-actionWindow)
	// Действия всех людей за 7 дней и последнее действие по проектам руководителя (за всё время).
	acts := map[string][]action{}
	lastHere := map[string]time.Time{}
	for _, is := range in.Issues {
		for _, c := range is.Changes {
			if c.Field != "status" || c.Author == "" {
				continue
			}
			started, ok := actionOf(c)
			if !ok {
				continue
			}
			who := in.name(c.Author)
			if in.leadProject(is.Project) && c.At.After(lastHere[who]) {
				lastHere[who] = c.At
			}
			if c.At.After(since) && !c.At.After(in.Now) {
				acts[who] = append(acts[who], action{c.At, is.Key, is.Project, started})
			}
		}
	}
	// Открытые задачи руководителя по исполнителям.
	type task struct {
		key   string
		days  int
		stale bool
	}
	mine := map[string][]task{}
	projs := map[string]map[string]bool{}
	for _, is := range in.sprintIssues() {
		if is.Assignee == "" || !open(is.Status) {
			continue
		}
		d := days(in.Now, lastStatusChange(is))
		st := (is.Status == stTodo || is.Status == stWork || is.Status == stRework) && d >= StaleDays
		mine[is.Assignee] = append(mine[is.Assignee], task{is.Key, d, st})
		if projs[is.Assignee] == nil {
			projs[is.Assignee] = map[string]bool{}
		}
		projs[is.Assignee][is.Project] = true
	}

	out := []PeopleRow{}
	for person, tasks := range mine {
		r := PeopleRow{Lead: in.Lead, Person: person, Role: in.Roles[person], MyOpen: len(tasks)}
		sort.Slice(tasks, func(i, j int) bool {
			if tasks[i].days != tasks[j].days {
				return tasks[i].days > tasks[j].days
			}
			return keyLess(tasks[i].key, tasks[j].key)
		})
		var tl []string
		for _, t := range tasks {
			if t.stale {
				r.MyStale++
			}
			tl = append(tl, fmt.Sprintf("%s (%d)", t.key, t.days))
		}
		r.MyTasks = strings.Join(tl, ", ")
		if t, ok := lastHere[person]; ok {
			d := days(in.Now, t)
			r.MyLastActionDays = &d
		}
		otherCnt := map[string]int{}
		var otherList []string
		seenKey := map[string]bool{}
		as := acts[person]
		sort.Slice(as, func(i, j int) bool { return as[i].at.After(as[j].at) })
		for _, a := range as {
			if in.leadProject(a.project) {
				r.MyActions7d++
				continue
			}
			if a.started {
				r.OtherStarted7d++
			} else {
				r.OtherFinished7d++
			}
			otherCnt[a.project]++
			verb := "завершил"
			if a.started {
				verb = "взял"
			}
			if k := verb + " " + a.key; !seenKey[k] {
				seenKey[k] = true
				otherList = append(otherList, k)
			}
		}
		type pc struct {
			p string
			n int
		}
		var pcs []pc
		for p, n := range otherCnt {
			pcs = append(pcs, pc{p, n})
		}
		sort.Slice(pcs, func(i, j int) bool { return pcs[i].n > pcs[j].n || (pcs[i].n == pcs[j].n && pcs[i].p < pcs[j].p) })
		op := make([]string, len(pcs))
		for i, x := range pcs {
			lead := in.LeadOf[x.p]
			if lead == "" {
				lead = "—"
			}
			op[i] = fmt.Sprintf("%s (%s) — %d", x.p, lead, x.n)
		}
		r.OtherProjects = strings.Join(op, "; ")
		r.OtherTasks = strings.Join(otherList, ", ")
		var ps []string
		for p := range projs[person] {
			ps = append(ps, p)
		}
		sort.Strings(ps)
		r.Projects = strings.Join(ps, " ")
		if r.MyStale > 0 && r.OtherStarted7d+r.OtherFinished7d > r.MyActions7d && !reNoSignal.MatchString(r.Role) {
			r.Signal = SignalStalls
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Person < out[j].Person })
	return out
}
