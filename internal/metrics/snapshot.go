package metrics

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

// Снепшот к созвону — порт reference/snapshot/sprint_port.js. Всё «на момент t» считается по истории задач:
// статус и исполнитель = последнее изменение ≤ t (иначе «было» у первого изменения после t, иначе текущее).
// Срез дня — 18:00 по Алматы.

// Константы процесса из sprint_port.js.
const (
	snapWDFrom    = "2026-07-01" // начало ряда рабочих дней
	snapIdleFrom  = "2026-09-14" // с этой даты задачи живут в спринтах — простой считаем не раньше
	snapIdleFallb = "2026-09-28" // начало окна простоя, если спринт основного проекта начался раньше snapIdleFrom
	snapFutFrom   = "2026-10-02" // с этой даты будущие спринты с номером считаются «запланировано»
	snapPlanAhead = 14           // «запланировано» — спринт стартует в ближайшие 14 дней
)

var (
	snapWaits = set("Test", "Тестирование", "Ready for deploy", "In Staging", "In Production", "Готово", "Done", "Закрыт", "Closed", "Resolved")
	snapTodo  = set("Сделать", "To Do", "Open", "Backlog", "Бэклог", "Открыт", "Selected For Development", "Требует уточнения")
	snapDone  = set("Готово", "Done", "In Production", "Закрыт", "Closed", "Resolved")
	snapGroup = map[string]string{"10100": "todo", "10011": "wk", "10008": "cr", "10000": "rw", "10001": "dn"}
	reSnapPM  = regexp.MustCompile(`(?i)manager|менеджер`)
	reSnapQA  = regexp.MustCompile(`(?i)qa|тест|test`)
	reDigit   = regexp.MustCompile(`\d`)
)

func set(xs ...string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

type SnapSprint struct {
	Name, State string
	Start, End  string // даты YYYY-MM-DD, "" — нет
}

// SnapIssue — задача с историей; люди — отображаемые имена Jira.
type SnapIssue struct {
	Key, Type, Status, StatusID string
	Done                        bool
	Assignee, Reporter          string
	Created                     time.Time
	Resolved                    *time.Time // только у задач в категории «done»
	Sprints                     []SnapSprint
	Changes                     []SnapChange // по времени
}

type SnapChange struct {
	At               time.Time
	Field            string // status | assignee
	From, To, Author string
	t                string // At по Алматы, YYYY-MM-DDTHH:MM — считается один раз
}

type SnapInput struct {
	Today, HistEnd string // YYYY-MM-DD: на какую дату снепшот и до какого дня есть история
	Issues         []SnapIssue
	Projects       []ProjectSprint
	Staff          []string            // в штате (имена Jira)
	Pos            map[string]string   // должность: из Jira, иначе из справочника
	AbsDays        map[string][]string // имя → дни отсутствия (отпуск, БС, больничный, отпросился)
	AbsNow         []Absence
	Holidays       map[string]bool
}

// SnapProject — CUR[k] из sprint_port.js: строка «Статуса спринтов» + исторические поля.
type SnapProject struct {
	ProjectSprint
	Ended   bool    `json:"ended"`
	Overdue int     `json:"overdue"`
	Time    int     `json:"time"`  // % прошедшего времени спринта на дату снепшота
	Ready   int     `json:"ready"` // % готовых задач
	NI      int     `json:"n_issues"`
	RwEv    int     `json:"rw_ev"`    // задач спринта, уходивших в Need Rework после старта
	Gave    int     `json:"gave"`     // переназначений руководителем с себя на других
	Ser     [][]any `json:"ser"`      // [день, todo у руководителя, todo без исполнителя, todo у других, в работе, готово]
	BlStart [3]int  `json:"bl_start"` // бэклог вне спринта на старте: всего, у руководителя, без исполнителя
	BlNow   [3]int  `json:"bl_now"`
}

// SnapPerson — строка PP (простой сотрудника).
type SnapPerson struct {
	Name     string   `json:"name"`
	Pos      string   `json:"pos"`
	Main     string   `json:"main"`
	Sprint   string   `json:"sprint"`
	From     string   `json:"from"`
	To       string   `json:"to"`
	Days     int      `json:"days"`  // рабочих дней без отсутствий
	Idle     int      `json:"idle"`  // из них без открытой задачи в спринте вне статусов ожидания
	None     int      `json:"none"`  // без открытых задач вообще
	Quiet    int      `json:"quiet"` // без активности в Jira
	Cur      int      `json:"cur"`   // текущая серия дней простоя
	T        int      `json:"t"`
	Dn       int      `json:"dn"`
	Todo     int      `json:"todo"`
	Wk       int      `json:"wk"`
	Rw       int      `json:"rw"`
	TsCr     int      `json:"ts_cr"`
	Left     int      `json:"left"`
	AbsNow   bool     `json:"abs_now"`
	Projects []string `json:"projects"`
}

type SnapCount struct {
	T, Todo, Wk, Cr, Rw, Ts, Dn int
}

type Snapshot struct {
	Today   string                          `json:"today"`
	HistEnd string                          `json:"hist_end"`
	Cur     map[string]*SnapProject         `json:"cur"`
	People  []SnapPerson                    `json:"people"`
	AXP     map[string]map[string]SnapCount `json:"axp"`  // открытые спринты: человек → проект → по статусам
	Unas    map[string][2]int               `json:"unas"` // без исполнителя: проект → [всего, в todo]
}

type daySet map[string]map[string]bool

func (m daySet) add(k, d string) {
	if m[k] == nil {
		m[k] = map[string]bool{}
	}
	m[k][d] = true
}
func (m daySet) has(k, d string) bool { return m[k][d] }

type snapX struct {
	SnapIssue
	pk     string
	sp     []SnapSprint // со стартом и концом
	fut    bool
	active bool
	a, s   []SnapChange
	cr     string // время создания, YYYY-MM-DDTHH:MM (Алматы)
	res    string // время решения ("" — не решена)
}

func atF(l []SnapChange, t, cur string) string {
	seen, prev := false, ""
	for _, c := range l {
		if c.t <= t {
			seen, prev = true, c.To
		} else {
			if seen {
				return prev
			}
			return c.From
		}
	}
	if seen {
		return prev
	}
	return cur
}

func hmAlmaty(t time.Time) string { return t.In(Almaty).Format("2006-01-02T15:04") }

func addDay(d string, n int) string {
	t, _ := time.Parse("2006-01-02", d)
	return t.AddDate(0, 0, n).Format("2006-01-02")
}

func daysBetween(a, b string) int {
	x, _ := time.Parse("2006-01-02", a)
	y, _ := time.Parse("2006-01-02", b)
	return int(y.Sub(x).Hours()/24 + 0.5)
}

func BuildSnapshot(in SnapInput) Snapshot {
	today, histEnd := in.Today, in.HistEnd
	var wd []string
	for d := snapWDFrom; d <= histEnd; d = addDay(d, 1) {
		t, _ := time.Parse("2006-01-02", d)
		if w := t.Weekday(); w != time.Saturday && w != time.Sunday && !in.Holidays[d] {
			wd = append(wd, d)
		}
	}
	wdays := func(a, b string) []string {
		var out []string
		for _, d := range wd {
			if a <= d && d <= b {
				out = append(out, d)
			}
		}
		return out
	}
	wd0 := histEnd
	if len(wd) > 0 {
		wd0 = wd[0]
	}

	X := make([]*snapX, 0, len(in.Issues))
	XK := map[string]*snapX{}
	for _, r := range in.Issues {
		x := &snapX{SnapIssue: r, cr: hmAlmaty(r.Created)}
		x.pk, _, _ = strings.Cut(r.Key, "-")
		if r.Done && r.Resolved != nil {
			x.res = hmAlmaty(*r.Resolved)
		}
		for _, s := range r.Sprints {
			if s.Start != "" && s.End != "" {
				x.sp = append(x.sp, s)
			}
			if s.State == "FUTURE" && reDigit.MatchString(s.Name) {
				x.fut = true
			}
			if s.State == "ACTIVE" {
				x.active = true
			}
		}
		for _, c := range r.Changes {
			c.t = hmAlmaty(c.At)
			if c.Field == "assignee" {
				x.a = append(x.a, c)
			} else if c.Field == "status" {
				x.s = append(x.s, c)
			}
		}
		X = append(X, x)
		XK[r.Key] = x
	}
	asgAt := func(r *snapX, t string) string { return atF(r.a, t, r.Assignee) }
	stAt := func(r *snapX, t string) string { return atF(r.s, t, r.Status) }
	openAt := func(r *snapX, t string) bool { return r.cr <= t && !(r.res != "" && r.res <= t) }
	inSprint := func(r *snapX, d string) bool {
		for _, s := range r.sp {
			end := s.End
			if s.State == "ACTIVE" && end < histEnd {
				end = histEnd
			}
			if s.Start <= d && d <= end {
				return true
			}
		}
		return false
	}
	planned := func(r *snapX, d string) bool {
		lim := addDay(d, snapPlanAhead)
		for _, s := range r.sp {
			if d < s.Start && s.Start <= lim {
				return true
			}
		}
		return d >= snapFutFrom && r.fut
	}
	// Дни, когда на человеке была хоть одна открытая задача, и дни его активности в Jira.
	hasAny, activity := daySet{}, daySet{}
	resDay := func(r *snapX) string {
		if r.res != "" && r.res[:10] < histEnd {
			return r.res[:10]
		}
		return histEnd
	}
	for _, r := range X {
		if r.Type == "Epic" {
			continue
		}
		for _, c := range r.Changes {
			activity.add(c.Author, hmAlmaty(c.At)[:10])
		}
		if r.Reporter != "" {
			activity.add(r.Reporter, r.cr[:10])
		}
		lo, hi := max(r.cr[:10], wd0), resDay(r)
		for _, d := range wd {
			if d < lo {
				continue
			}
			if d > hi {
				break
			}
			t := d + "T18:00"
			if !openAt(r, t) {
				continue
			}
			if w := asgAt(r, t); w != "" {
				hasAny.add(w, d)
			}
		}
	}
	absd := daySet{}
	for n, ds := range in.AbsDays {
		for _, d := range ds {
			absd.add(n, d)
		}
	}

	// CUR — проекты с активным спринтом.
	cur := map[string]*SnapProject{}
	leads := map[string]bool{}
	for _, p := range in.Projects {
		cur[p.Key] = &SnapProject{ProjectSprint: p}
		if p.Lead != "" {
			leads[p.Lead] = true
		}
	}
	spi := map[string][]string{}
	for _, r := range X {
		c := cur[r.pk]
		if c == nil || r.Type == "Epic" {
			continue
		}
		for _, s := range r.Sprints {
			if s.Name == c.Sprint && s.State == "ACTIVE" {
				spi[r.pk] = append(spi[r.pk], r.Key)
				break
			}
		}
	}
	for k, c := range cur {
		if c.Start == "" || c.End == "" {
			c.Ser = [][]any{}
			continue
		}
		c.Ended = c.End < today
		if c.Ended {
			c.Overdue = daysBetween(c.End, today)
		}
		ln, el := max(1, daysBetween(c.Start, c.End)), daysBetween(c.Start, today)
		c.Time = max(0, min(100, roundInt(100*float64(el)/float64(ln))))
		if c.Total > 0 {
			c.Ready = roundInt(100 * float64(c.Dn) / float64(c.Total))
		}
		keys := spi[k]
		c.NI = len(keys)
		for _, key := range keys {
			r := XK[key]
			for _, ch := range r.s {
				if ch.To == "Need Rework" && ch.t[:10] >= c.Start {
					c.RwEv++
					break
				}
			}
			for _, ch := range r.a {
				if ch.t[:10] >= c.Start && ch.Author == c.Lead && ch.To != "" && ch.To != c.Lead {
					c.Gave++
				}
			}
		}
		hi := c.End
		if histEnd < hi {
			hi = histEnd
		}
		c.Ser = [][]any{}
		for _, d := range wdays(c.Start, hi) {
			t := d + "T18:00"
			row := [6]int{}
			for _, key := range keys {
				r := XK[key]
				if r.cr > t {
					continue
				}
				if r.res != "" && r.res <= t {
					row[5]++
					continue
				}
				s := stAt(r, t)
				switch {
				case snapDone[s]:
					row[5]++
				case snapTodo[s]:
					switch w := asgAt(r, t); {
					case w == c.Lead:
						row[1]++
					case w == "":
						row[2]++
					default:
						row[3]++
					}
				default:
					row[4]++
				}
			}
			c.Ser = append(c.Ser, []any{d, row[1], row[2], row[3], row[4], row[5]})
		}
	}

	// AXP — открытые спринты по людям и проектам; без исполнителя — отдельно.
	pp := map[string]map[string]SnapCount{}
	unas := map[string][2]int{}
	for _, r := range X {
		if !r.active {
			continue
		}
		g := snapGroup[r.StatusID]
		if g == "" {
			g = "ts"
		}
		if r.Assignee == "" {
			u := unas[r.pk]
			u[0]++
			if g == "todo" {
				u[1]++
			}
			unas[r.pk] = u
			continue
		}
		if pp[r.Assignee] == nil {
			pp[r.Assignee] = map[string]SnapCount{}
		}
		o := pp[r.Assignee][r.pk]
		o.T++
		switch g {
		case "todo":
			o.Todo++
		case "wk":
			o.Wk++
		case "cr":
			o.Cr++
		case "rw":
			o.Rw++
		case "dn":
			o.Dn++
		default:
			o.Ts++
		}
		pp[r.Assignee][r.pk] = o
	}

	// SA — дни, когда на человеке была открытая задача в спринте вне статусов ожидания.
	sa := daySet{}
	for _, r := range X {
		if r.Type == "Epic" || len(r.sp) == 0 {
			continue
		}
		lo, hi := max(r.cr[:10], snapIdleFrom), resDay(r)
		for _, d := range wd {
			if d < lo {
				continue
			}
			if d > hi {
				break
			}
			t := d + "T18:00"
			if !openAt(r, t) || !inSprint(r, d) || snapWaits[stAt(r, t)] {
				continue
			}
			if w := asgAt(r, t); w != "" {
				sa.add(w, d)
			}
		}
	}

	staff := set(in.Staff...)
	posOf := func(n string) string { return in.Pos[n] }
	nameSet := map[string]bool{}
	for n := range pp {
		nameSet[n] = true
	}
	for _, n := range in.Staff {
		if in.Pos[n] != "" {
			nameSet[n] = true
		}
	}
	names := make([]string, 0, len(nameSet))
	for n := range nameSet {
		names = append(names, n)
	}
	sort.Strings(names)
	people := []SnapPerson{}
	for _, n := range names {
		if !staff[n] {
			continue
		}
		pr := pp[n]
		pks := make([]string, 0, len(pr))
		for k := range pr {
			pks = append(pks, k)
		}
		sort.Strings(pks)
		main := ""
		for _, k := range pks {
			if main == "" || pr[k].T > pr[main].T {
				main = k
			}
		}
		if leads[n] || reSnapPM.MatchString(posOf(n)) || reSnapQA.MatchString(posOf(n)) || (posOf(n) == "" && main == "AQA") {
			continue
		}
		c := cur[main]
		lo := snapIdleFallb
		if c != nil && c.Start >= snapIdleFrom {
			lo = c.Start
		}
		var dd []string
		for _, d := range wdays(lo, histEnd) {
			if !absd.has(n, d) {
				dd = append(dd, d)
			}
		}
		if len(dd) == 0 {
			continue
		}
		var idle, none, quiet, streak int
		for _, d := range dd {
			if sa.has(n, d) {
				streak = 0
				continue
			}
			idle++
			streak++
			if !hasAny.has(n, d) {
				none++
			}
			if !activity.has(n, d) {
				quiet++
			}
		}
		var tot SnapCount
		for _, v := range pr {
			tot.T += v.T
			tot.Todo += v.Todo
			tot.Wk += v.Wk
			tot.Cr += v.Cr
			tot.Rw += v.Rw
			tot.Ts += v.Ts
			tot.Dn += v.Dn
		}
		absNow := false
		for _, a := range in.AbsNow {
			to := a.From
			if a.To != nil {
				to = *a.To
			}
			if a.Name == n && a.From <= today && today <= to {
				absNow = true
			}
		}
		sp := ""
		if c != nil {
			sp = c.Sprint
		}
		people = append(people, SnapPerson{Name: n, Pos: posOf(n), Main: main, Sprint: sp, From: lo, To: histEnd, Days: len(dd),
			Idle: idle, None: none, Quiet: quiet, Cur: streak, T: tot.T, Dn: tot.Dn, Todo: tot.Todo, Wk: tot.Wk, Rw: tot.Rw,
			TsCr: tot.Ts + tot.Cr, Left: tot.Todo + tot.Wk + tot.Rw, AbsNow: absNow, Projects: pks})
	}

	// Бэклог вне спринта: открытые, не в спринте и не запланированные — на старте спринта и сейчас.
	byp := map[string][]*snapX{}
	for _, r := range X {
		if r.Type != "Epic" && r.Type != "Подзадача" {
			byp[r.pk] = append(byp[r.pk], r)
		}
	}
	blcount := func(k, d string) [3]int {
		t, lead := d+"T18:00", cur[k].Lead
		var out [3]int
		for _, r := range byp[k] {
			if !openAt(r, t) || inSprint(r, d) || planned(r, d) {
				continue
			}
			w := asgAt(r, t)
			out[0]++
			if w == lead {
				out[1]++
			}
			if w == "" {
				out[2]++
			}
		}
		return out
	}
	for k, c := range cur {
		d0 := max(c.Start, wd0)
		start := histEnd
		for _, d := range wd {
			if d >= d0 {
				start = d
				break
			}
		}
		c.BlStart, c.BlNow = blcount(k, start), blcount(k, histEnd)
	}
	return Snapshot{Today: today, HistEnd: histEnd, Cur: cur, People: people, AXP: pp, Unas: unas}
}

func roundInt(v float64) int {
	if v < 0 {
		return -int(-v + 0.5)
	}
	return int(v + 0.5)
}
