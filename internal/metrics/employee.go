package metrics

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Карточка сотрудника — порт reference/employee/build_emp.py (EMP) и page_template.html.
// Задачи — все задачи Jira, обновлявшиеся с начала года; закрытая задача засчитывается текущему исполнителю
// в месяц закрытия. Возвраты (переходы в Need Rework) — тому, кто был исполнителем в момент возврата.

// Статусы «ещё не начата» (TODO в build_emp.py).
var empTodo = map[string]bool{"Сделать": true, "To Do": true, "Open": true, "Backlog": true, "Бэклог": true, "Открыт": true}

const (
	empRework = "Need Rework"
	empInWork = "В работе"
	empBug    = "Ошибка"
)

type EmpSprint struct {
	State string
	Start *time.Time
	End   *time.Time
}

type EmpChange struct {
	At    time.Time
	Field string // status (названия) | assignee (логины)
	From  string
	To    string
}

// EmpIssue — задача года; люди — логины Jira.
type EmpIssue struct {
	Key, Summary, Type, Status string
	Done                       bool // категория статуса «done»
	Assignee, Reporter, Tester string
	Created                    time.Time
	Resolved                   *time.Time
	TimespentS                 int
	Sprints                    []EmpSprint
	Changes                    []EmpChange // по времени
}

// EmpPerson — сотрудник из справочника.
type EmpPerson struct {
	ID    int64
	Name  string
	Login string
	Pos   string
	TL    bool
	Lead  bool
}

// Индексы помесячного ряда m[месяц] (IX в page_template.html).
const (
	ixDone = iota
	ixBug
	ixH
	ixCarry
	ixPlan
	ixPlanDone
	ixRw
	ixCyc
	ixRep
	ixQA
	ixLate
	ixEarly
	ixMid
	ixDay
	ixSick
	ixMin
	ixLateStudy
	ixLen
)

// EmpRow — строка списка (EMP.ppl[n] без задач и журнала).
type EmpRow struct {
	ID    int64            `json:"id"`
	Name  string           `json:"n"`
	Pos   string           `json:"pos"`
	TL    bool             `json:"tl"`
	Lead  bool             `json:"lead"`
	Login string           `json:"login"`
	Proj  [][2]any         `json:"proj"` // [проект, задач] по убыванию
	Open  [2]int           `json:"open"` // [не начаты, в работе]
	M     map[string][]any `json:"m"`
	Cyc   *float64         `json:"cyc"` // медиана цикла по закрытым задачам (S().cyc), дн.
}

// EmpCard — задачи и журнал посещаемости одного сотрудника.
//
//	t:    [ключ, название, тип, статус, создана, закрыта|"", спринтов, возвратов, цикл дн.|null, часы, проект]
//	alog: [дата, что, минут, подробности]
type EmpCard struct {
	Tasks [][]any `json:"t"`
	Alog  [][]any `json:"alog"`
}

type Employees struct {
	Stamp   string             `json:"stamp"`
	Mon     []string           `json:"mon"`
	AttFrom string             `json:"att_from"`
	Ppl     []EmpRow           `json:"ppl"`
	Cards   map[int64]*EmpCard `json:"-"`
}

type empAcc struct {
	m       map[string]*[ixLen]float64
	cyc     map[string][]float64
	tasks   [][]any
	proj    map[string]int
	open    [2]int
	alog    [][]any
	hasJira bool
	hasAtt  bool
}

func (a *empAcc) mon(m string) *[ixLen]float64 {
	if a.m[m] == nil {
		a.m[m] = &[ixLen]float64{}
	}
	return a.m[m]
}

// BuildEmployees считает карточки за календарный год now (по Алматы).
func BuildEmployees(issues []EmpIssue, ev []AttEvent, people []EmpPerson, now time.Time) Employees {
	now = now.In(Almaty)
	year := now.Format("2006")
	mon := func(t time.Time) string {
		s := t.In(Almaty).Format("2006-01")
		if strings.HasPrefix(s, year) {
			return s
		}
		return ""
	}
	day := func(t time.Time) string { return t.In(Almaty).Format("2006-01-02") }

	byLogin, byName := map[string]*EmpPerson{}, map[string]*EmpPerson{}
	for i := range people {
		p := &people[i]
		if p.Login != "" {
			byLogin[p.Login] = p
		}
		byName[p.Name] = p
	}
	acc := map[int64]*empAcc{}
	get := func(p *EmpPerson) *empAcc {
		if acc[p.ID] == nil {
			acc[p.ID] = &empAcc{m: map[string]*[ixLen]float64{}, cyc: map[string][]float64{}, proj: map[string]int{}}
		}
		return acc[p.ID]
	}
	who := func(login string) *empAcc {
		if p := byLogin[login]; p != nil && login != "" {
			a := get(p)
			a.hasJira = true // в списке — все, кто встречается в задачах (P в build_emp.py)
			return a
		}
		return nil
	}

	for _, r := range issues {
		pk, _, _ := strings.Cut(r.Key, "-")
		var chs, asg []EmpChange
		for _, c := range r.Changes {
			switch c.Field {
			case "status":
				chs = append(chs, c)
			case "assignee":
				asg = append(asg, c)
			}
		}
		var rw []EmpChange
		for _, c := range chs {
			if c.To == empRework {
				rw = append(rw, c)
			}
		}
		var start *time.Time
		for _, c := range chs {
			if empTodo[c.From] && !empTodo[c.To] {
				start = &c.At
				break
			}
		}
		if start == nil {
			for _, c := range chs {
				if c.To == empInWork {
					start = &c.At
					break
				}
			}
		}
		done := r.Done && r.Resolved != nil
		var cyc *float64
		if done && start != nil && r.Resolved.After(*start) {
			v := round1(r.Resolved.Sub(*start).Hours() / 24)
			cyc = &v
		}
		nsp := len(r.Sprints)

		if p := who(r.Assignee); p != nil {
			p.proj[pk]++
			res := ""
			if r.Resolved != nil {
				res = day(*r.Resolved)
			}
			var cv any
			if cyc != nil {
				cv = *cyc
			}
			sum := []rune(r.Summary)
			if len(sum) > 90 {
				sum = sum[:90]
			}
			p.tasks = append(p.tasks, []any{r.Key, string(sum), r.Type, r.Status, day(r.Created), res, nsp, len(rw), cv,
				round1(float64(r.TimespentS) / 3600), pk})
			if done {
				if m := mon(*r.Resolved); m != "" {
					mm := p.mon(m)
					mm[ixDone]++
					if r.Type == empBug {
						mm[ixBug]++
					}
					mm[ixH] += float64(r.TimespentS) / 3600
					if nsp > 1 {
						mm[ixCarry]++
					}
					if cyc != nil {
						p.cyc[m] = append(p.cyc[m], *cyc)
					}
				}
			} else if empTodo[r.Status] {
				p.open[0]++
			} else {
				p.open[1]++
			}
			// План спринта: задачи в закрытых спринтах года; в срок — закрыта не позже 2 дней после конца спринта.
			for _, s := range r.Sprints {
				if s.State != "CLOSED" || s.End == nil {
					continue
				}
				m := mon(*s.End)
				if m == "" {
					continue
				}
				mm := p.mon(m)
				mm[ixPlan]++
				ss := "2000-01-01"
				if s.Start != nil {
					ss = day(*s.Start)
				}
				if done {
					rd := day(*r.Resolved)
					if rd <= day(s.End.AddDate(0, 0, 2)) && rd >= ss {
						mm[ixPlanDone]++
					}
				}
			}
		}
		for _, c := range rw {
			if p := who(assigneeAt(r, asg, c.At)); p != nil {
				if m := mon(c.At); m != "" {
					p.mon(m)[ixRw]++
				}
			}
		}
		if p := who(r.Reporter); p != nil {
			if m := mon(r.Created); m != "" {
				p.mon(m)[ixRep]++
			}
		}
		if p := who(r.Tester); p != nil && done {
			if m := mon(*r.Resolved); m != "" {
				p.mon(m)[ixQA]++
			}
		}
	}

	// Посещаемость (по сообщениям группы).
	attFrom := ""
	labels := map[string]string{"late": "Опоздание", "early": "Ушёл раньше", "mid": "Отлучка", "day": "Весь день"}
	for _, e := range ev {
		if attFrom == "" || e.Date < attFrom {
			attFrom = e.Date
		}
		pp := byName[e.N]
		if pp == nil || !strings.HasPrefix(e.Date, year) {
			continue
		}
		p := get(pp)
		p.hasAtt = true
		a := p.mon(e.Date[:7])
		switch e.T {
		case "sick":
			a[ixSick]++
			p.alog = append(p.alog, []any{e.Date, "Больничный", 0, cut(e.Rsn, 40)})
		case "vac":
		case "day":
			a[ixDay]++
			a[ixMin] += 480
			p.alog = append(p.alog, []any{e.Date, labels["day"], 480, cut(e.Rsn, 40)})
		case "late", "early", "mid":
			if e.T == "late" {
				a[ixLate]++
				if e.C == "study" {
					a[ixLateStudy]++
				}
			} else if e.T == "early" {
				a[ixEarly]++
			} else {
				a[ixMid]++
			}
			a[ixMin] += float64(e.M)
			det := ""
			if e.A != nil && e.B != nil {
				det = fmt.Sprintf("%d:%02d–%d:%02d", *e.A/60, *e.A%60, *e.B/60, *e.B%60)
			}
			if e.Rsn != "" {
				if det != "" {
					det += " · "
				}
				det += e.Rsn
			}
			p.alog = append(p.alog, []any{e.Date, labels[e.T], e.M, det})
		}
	}

	out := Employees{Stamp: now.Format("2006-01-02T15:04"), AttFrom: attFrom, Cards: map[int64]*EmpCard{}}
	for m := 1; m <= int(now.Month()); m++ {
		out.Mon = append(out.Mon, fmt.Sprintf("%s-%02d", year, m))
	}
	for _, pp := range people {
		p := acc[pp.ID]
		if p == nil || (!p.hasJira && !p.hasAtt) {
			continue
		}
		row := EmpRow{ID: pp.ID, Name: pp.Name, Pos: pp.Pos, TL: pp.TL, Lead: pp.Lead, Login: pp.Login, Open: p.open, M: map[string][]any{}}
		for k, c := range p.proj {
			row.Proj = append(row.Proj, [2]any{k, c})
		}
		sort.SliceStable(row.Proj, func(i, j int) bool {
			if row.Proj[i][1].(int) != row.Proj[j][1].(int) {
				return row.Proj[i][1].(int) > row.Proj[j][1].(int)
			}
			return row.Proj[i][0].(string) < row.Proj[j][0].(string)
		})
		for _, m := range out.Mon {
			v := [ixLen]float64{}
			if p.m[m] != nil {
				v = *p.m[m]
			}
			arr := make([]any, ixLen)
			for i := range arr {
				arr[i] = int(v[i])
			}
			arr[ixH] = round1(v[ixH])
			arr[ixCyc] = nil
			if c := p.cyc[m]; len(c) > 0 {
				arr[ixCyc] = round1(median(c))
			}
			row.M[m] = arr
		}
		var cy []float64
		for _, t := range p.tasks {
			if c, ok := t[8].(float64); ok && t[5] != "" {
				cy = append(cy, c)
			}
		}
		if len(cy) > 0 {
			v := median(cy)
			row.Cyc = &v
		}
		out.Ppl = append(out.Ppl, row)
		sort.SliceStable(p.tasks, func(i, j int) bool {
			a, b := p.tasks[i], p.tasks[j]
			ra, rb := a[5].(string), b[5].(string)
			if ra == "" {
				ra = "9999"
			}
			if rb == "" {
				rb = "9999"
			}
			if ra != rb {
				return ra > rb
			}
			return a[4].(string) > b[4].(string)
		})
		sort.SliceStable(p.alog, func(i, j int) bool { return p.alog[i][0].(string) > p.alog[j][0].(string) })
		out.Cards[pp.ID] = &EmpCard{Tasks: nonNil(p.tasks), Alog: nonNil(p.alog)}
	}
	sort.SliceStable(out.Ppl, func(i, j int) bool { return out.Ppl[i].Name < out.Ppl[j].Name })
	return out
}

// assigneeAt — исполнитель задачи в момент t (asg_at в build_emp.py): последняя смена до t,
// иначе «было» у первой смены после t, иначе текущий.
func assigneeAt(r EmpIssue, asg []EmpChange, t time.Time) string {
	set, prev := false, ""
	for _, c := range asg {
		if !c.At.After(t) {
			set, prev = true, c.To
		} else {
			if set {
				return prev
			}
			return c.From
		}
	}
	if set {
		return prev
	}
	return r.Assignee
}

// RestrictEmployees — копия только с разрешёнными людьми; исходные данные (кэш) не меняются.
// noTasks — HR: без задач Jira (docs/security.md).
func RestrictEmployees(e Employees, allowed map[int64]bool, noTasks bool) Employees {
	out := e
	out.Ppl, out.Cards = nil, map[int64]*EmpCard{}
	for _, r := range e.Ppl {
		if !allowed[r.ID] {
			continue
		}
		if noTasks {
			r.Proj, r.Open, r.Cyc = nil, [2]int{}, nil
			m := make(map[string][]any, len(r.M))
			for k, v := range r.M {
				w := append([]any(nil), v...)
				for i := ixDone; i <= ixQA; i++ {
					w[i] = 0
				}
				w[ixCyc] = nil
				m[k] = w
			}
			r.M = m
		}
		out.Ppl = append(out.Ppl, r)
		if c := e.Cards[r.ID]; c != nil {
			cc := &EmpCard{Tasks: c.Tasks, Alog: append(make([][]any, 0, len(c.Alog)), c.Alog...)}
			if noTasks {
				cc.Tasks = [][]any{}
			}
			out.Cards[r.ID] = cc
		}
	}
	if out.Ppl == nil {
		out.Ppl = []EmpRow{}
	}
	return out
}

// HideAlogReasons убирает причины из журнала (руководитель видит время, но не причину — docs/security.md).
func HideAlogReasons(c *EmpCard) {
	for i, a := range c.Alog {
		d, _ := a[3].(string)
		if t, _, ok := strings.Cut(d, " · "); ok {
			d = t
		} else if a[1] == "Больничный" || a[1] == "Весь день" || !strings.ContainsAny(d, "–") {
			d = ""
		}
		c.Alog[i] = []any{a[0], a[1], a[2], d}
	}
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func median(a []float64) float64 {
	s := append([]float64(nil), a...)
	sort.Float64s(s)
	h := len(s) / 2
	if len(s)%2 == 1 {
		return s[h]
	}
	return (s[h-1] + s[h]) / 2
}

func cut(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func nonNil(a [][]any) [][]any {
	if a == nil {
		return [][]any{}
	}
	return a
}
