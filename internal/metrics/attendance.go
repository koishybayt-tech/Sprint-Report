package metrics

import (
	"sort"
	"strings"
	"time"

	"github.com/adata/reports/internal/tgparse"
)

// AttRow — событие посещаемости из Telegram с сотрудником и временем сообщения (из БД).
type AttRow struct {
	ID         int64 // attendance_events.id
	EmployeeID int64
	Name       string // employees.full_name
	Login      string
	Day        time.Time
	Kind       string // late | early | mid | day | sick | vac | arrived
	FromMin    *int
	ToMin      *int
	Minutes    int
	Reason     string
	ReasonCat  string
	MakeupKind string
	MakeupText string
	MakeupDate *time.Time
	ApprovedBy string // через запятую
	SentAt     time.Time
}

// AttEvent — событие в формате ATT.ev из reference/attendance/build.py.
type AttEvent struct {
	ID   int64  `json:"id"`
	Date string `json:"date"`
	Post string `json:"post"` // время сообщения, ЧЧ:ММ
	N    string `json:"n"`
	T    string `json:"t"`
	M    int    `json:"m"`
	A    *int   `json:"a,omitempty"`
	B    *int   `json:"b,omitempty"`
	Arr  *int   `json:"arr,omitempty"` // «upd» о приходе после заявленного опоздания
	Otr  string `json:"otr"`
	Rsn  string `json:"rsn"`
	Ag   string `json:"ag"`
	C    string `json:"c"`            // study | health | family | personal | other
	Od   string `json:"od,omitempty"` // дата отработки
}

// AttPerson — ATT.ppl: роль, тимлид, есть ли в Jira, задачи по проектам (pr) и логин.
// pr: [проект, конец спринта, открытых (бэклог+работа+реворк+Blocked), руководитель, всего, бэклог, в работе, реворк, Blocked].
type AttPerson struct {
	Pos   string  `json:"pos"`
	TL    bool    `json:"tl"`
	Jira  bool    `json:"jira"`
	Pr    [][]any `json:"pr"`
	Login string  `json:"login"`
}

// Attendance — данные отчёта «Посещаемость» (ATT).
type Attendance struct {
	Ev    []AttEvent           `json:"ev"`
	Ppl   map[string]AttPerson `json:"ppl"`
	Abs   []Absence            `json:"abs"`
	AbsAt map[string]time.Time `json:"abs_at"`
	Coll  []string             `json:"coll"` // коллектив: штат без руководящего состава
	Lead  []string             `json:"lead"` // руководящий состав
	Cpos  map[string]string    `json:"cpos"`
	From  string               `json:"from"`
	To    string               `json:"to"`
	Stamp string               `json:"stamp"`
}

// StaffRow — сотрудник для коллектива.
type StaffRow struct {
	Name       string
	Login      string
	Staff      bool
	Management bool
	Left       bool
}

// BuildAttendance собирает ATT как reference/attendance/build.py:
//   - событие «upd» (пришёл) до 12:00 после заявленного опоздания того же дня → arr у опоздания;
//   - учёба (c=study) — категория причины study или отработка «по расписанию»;
//   - дата отработки — для «завтра», «суббота» и явной даты не раньше дня пропуска;
//   - задачи (pr) — из отчёта «Спринты» на тот же срез.
func BuildAttendance(rows []AttRow, rep *SprintReport, abs []AbsenceRow, staff []StaffRow, now time.Time) Attendance {
	a := Attendance{Ev: []AttEvent{}, Ppl: map[string]AttPerson{}, Abs: []Absence{}, Coll: []string{}, Lead: []string{}, Cpos: map[string]string{}}
	a.Stamp = now.In(Almaty).Format("2006-01-02T15:04")
	rows = splitMakeupDates(dedupeAttRows(rows))

	type key struct {
		emp int64
		day string
	}
	arrived := map[key][]AttRow{}
	logins := map[string]string{} // ФИО → логин
	for _, r := range rows {
		if r.Login != "" {
			logins[r.Name] = r.Login
		}
		if r.Kind == "arrived" {
			k := key{r.EmployeeID, r.Day.Format("2006-01-02")}
			arrived[k] = append(arrived[k], r)
		}
	}
	for _, r := range rows {
		if r.Kind == "arrived" {
			continue
		}
		day := r.Day.Format("2006-01-02")
		e := AttEvent{ID: r.ID, Date: day, Post: r.SentAt.In(Almaty).Format("15:04"), N: r.Name, T: r.Kind, M: r.Minutes,
			A: r.FromMin, B: r.ToMin, Otr: r.MakeupText, Rsn: r.Reason, Ag: r.ApprovedBy, C: r.ReasonCat}
		if e.T == "sick" || e.T == "vac" {
			e.M = 0
		}
		if r.ReasonCat == "study" || r.MakeupKind == "schedule" {
			e.C = "study"
		}
		if r.MakeupDate != nil && (r.MakeupKind == "tomorrow" || r.MakeupKind == "saturday" || r.MakeupKind == "date") {
			if od := r.MakeupDate.Format("2006-01-02"); od >= day {
				e.Od = od
			}
		}
		if e.T == "late" && e.B != nil {
			for _, u := range arrived[key{r.EmployeeID, day}] {
				if u.FromMin != nil && *u.FromMin <= 720 && u.SentAt.After(r.SentAt) {
					v := *u.FromMin
					e.Arr = &v // как в build.py: последний подходящий «upd»
				}
			}
		}
		a.Ev = append(a.Ev, e)
		if a.From == "" || day < a.From {
			a.From = day
		}
		if day > a.To {
			a.To = day
		}
	}

	// Задачи по проектам — из «Спринтов»: имена там — как в Jira, связываем по логину.
	jiraName := map[string]string{} // логин → имя в Jira
	for n, l := range rep.Logins {
		if l != "" {
			jiraName[l] = n
		}
	}
	blk := map[string]int{} // имя в Jira|проект → Blocked
	for _, b := range rep.Bottlenecks {
		if b.Status == "Blocked" {
			blk[b.Assignee+"|"+strings.SplitN(b.Key, "-", 2)[0]]++
		}
	}
	proj := map[string]ProjectSprint{}
	for _, p := range rep.Projects {
		proj[p.Key] = p
	}
	person := func(name, login string) {
		if _, ok := a.Ppl[name]; ok {
			return
		}
		jn := name
		if v := jiraName[login]; v != "" {
			jn = v
		}
		p := AttPerson{Pos: rep.Roles[jn], Jira: login != "", Login: login, Pr: [][]any{}}
		for _, t := range rep.TeamLeads {
			if t == jn {
				p.TL = true
			}
		}
		for _, x := range rep.People {
			if x.Name != jn {
				continue
			}
			pr, ok := proj[x.ProjectKey]
			if !ok {
				continue
			}
			b := blk[jn+"|"+x.ProjectKey]
			p.Pr = append(p.Pr, []any{x.ProjectKey, pr.End, x.Todo + x.Wk + x.Rw + b, pr.Lead, x.Total, x.Todo, x.Wk, x.Rw, b})
		}
		a.Ppl[name] = p
	}
	for _, e := range a.Ev {
		person(e.N, logins[e.N])
	}

	left := map[string]bool{}
	for _, s := range staff {
		if s.Left {
			left[s.Name] = true
			continue
		}
		if s.Management {
			a.Lead = append(a.Lead, s.Name)
		} else if s.Staff {
			a.Coll = append(a.Coll, s.Name)
		}
		if s.Staff || s.Management {
			person(s.Name, s.Login)
		}
	}
	sort.Strings(a.Coll)
	sort.Strings(a.Lead)
	for _, n := range a.Coll {
		a.Cpos[n] = a.Ppl[n].Pos
	}

	// Отпуска, БС и больничные (build.py берёт из ABS только vac, bs, sick); уволенных не показываем.
	for _, r := range abs {
		t, ok := absType[r.Kind]
		if !ok || t == "off" || left[r.FullName] {
			continue
		}
		note := r.Note
		if n, ok := absNote[r.Kind]; ok && r.Source == "bitrix" {
			note = n
		}
		x := Absence{Name: r.FullName, Type: t, From: r.From.Format("2006-01-02"), Source: absSource[r.Source], Note: note}
		if r.To != nil {
			s := r.To.Format("2006-01-02")
			x.To = &s
		}
		if r.Source == "telegram" && overlapsExisting(a.Abs, x) {
			continue
		}
		a.Abs = append(a.Abs, x)
		person(r.FullName, r.Login)
	}
	return a
}

// LeadScope — кого видит руководитель: людей с задачами в проектах, где он руководитель в Jira, и себя.
func LeadScope(a *Attendance, self string) map[string]bool {
	allowed := map[string]bool{}
	if self == "" {
		return allowed
	}
	allowed[self] = true
	for n, p := range a.Ppl {
		for _, x := range p.Pr {
			if lead, _ := x[3].(string); lead == self {
				allowed[n] = true
			}
		}
	}
	return allowed
}

// RestrictAttendance оставляет только разрешённых людей; причины видны только про себя (для lead и employee).
func RestrictAttendance(a *Attendance, allowed map[string]bool, self string) {
	ev := a.Ev[:0]
	for _, e := range a.Ev {
		if !allowed[e.N] {
			continue
		}
		if e.N != self {
			e.Rsn = "" // причина — только admin, hr и сам сотрудник
		}
		ev = append(ev, e)
	}
	a.Ev = ev
	abs := a.Abs[:0]
	for _, x := range a.Abs {
		if allowed[x.Name] {
			abs = append(abs, x)
		}
	}
	a.Abs = abs
	for n := range a.Ppl {
		if !allowed[n] {
			delete(a.Ppl, n)
		}
	}
	keep := func(xs []string) []string {
		out := []string{}
		for _, x := range xs {
			if allowed[x] {
				out = append(out, x)
			}
		}
		return out
	}
	a.Coll, a.Lead = keep(a.Coll), keep(a.Lead)
	for n := range a.Cpos {
		if !allowed[n] {
			delete(a.Cpos, n)
		}
	}
}

// dedupeAttRows убирает повторы одного и того же отсутствия, записанного в нескольких сообщениях
// (человек написал заново или ответил на своё сообщение): тот же сотрудник, день, вид и время — одно событие.
// Остаётся первое сообщение; пустые поля (причина, отработка, согласование) добираются из повторов.
func dedupeAttRows(rows []AttRow) []AttRow {
	type key struct {
		emp      int64
		day      string
		kind     string
		from, to int
		minutes  int
	}
	opt := func(v *int) int {
		if v == nil {
			return -1
		}
		return *v
	}
	idx := map[key]int{}
	out := make([]AttRow, 0, len(rows))
	for _, r := range rows {
		k := key{r.EmployeeID, r.Day.Format("2006-01-02"), r.Kind, opt(r.FromMin), opt(r.ToMin), r.Minutes}
		i, dup := idx[k]
		if !dup {
			idx[k] = len(out)
			out = append(out, r)
			continue
		}
		f := &out[i]
		if f.Reason == "" {
			f.Reason, f.ReasonCat = r.Reason, r.ReasonCat
		}
		if f.MakeupDate == nil && r.MakeupDate != nil {
			f.MakeupKind, f.MakeupText, f.MakeupDate = r.MakeupKind, r.MakeupText, r.MakeupDate
		}
		if f.ApprovedBy == "" {
			f.ApprovedBy = r.ApprovedBy
		}
	}
	return out
}

// splitMakeupDates: «Пропуск 13.11, 16.11 / Отработка 21.11, 28.11» — отработка разделена на разные дни:
// первый пропуск отрабатывается в первую дату, второй — во вторую. В базе у всех событий сообщения одна дата отработки
// (первая), поэтому раскладываем даты по пропускам по порядку; только если дат и пропусков поровну.
func splitMakeupDates(rows []AttRow) []AttRow {
	type key struct {
		emp  int64
		sent time.Time
		text string
	}
	groups := map[key][]int{}
	for i, r := range rows {
		if r.MakeupKind != "date" || r.MakeupText == "" || r.MakeupDate == nil || r.Kind == "sick" || r.Kind == "vac" || r.Kind == "arrived" {
			continue
		}
		k := key{r.EmployeeID, r.SentAt, r.MakeupText}
		groups[k] = append(groups[k], i)
	}
	out := append([]AttRow(nil), rows...)
	for k, idx := range groups {
		dates := tgparse.MakeupDates(k.text, k.sent.In(Almaty))
		if len(dates) < 2 || len(dates) != len(idx) {
			continue
		}
		sort.SliceStable(idx, func(a, b int) bool { return out[idx[a]].Day.Before(out[idx[b]].Day) })
		for n, i := range idx {
			if d, err := time.Parse("2006-01-02", dates[n]); err == nil {
				out[i].MakeupDate = &d
			}
		}
	}
	return out
}
