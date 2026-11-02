package metrics

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// AbsenceRow — запись absences с сотрудником (из БД).
type AbsenceRow struct {
	Login    string // jira_login сотрудника
	FullName string
	Kind     string // vacation | unpaid | sick | off (отпросился, из Telegram)
	From     time.Time
	To       *time.Time
	Source   string // bitrix | telegram | manual
	Note     string
}

// Absence — отсутствие в формате ABS из reference/absences/build_abs.js: [имя, тип, с, по, источник, примечание].
// Имя — как у исполнителя в Jira, чтобы совпадать со строками «Загрузки ресурсов».
type Absence struct {
	Name   string  `json:"name"`
	Type   string  `json:"type"` // vac | bs | sick | off
	From   string  `json:"from"`
	To     *string `json:"to"`
	Source string  `json:"source"` // Bitrix | Telegram
	Note   string  `json:"note"`
}

var absType = map[string]string{"vacation": "vac", "unpaid": "bs", "sick": "sick", "off": "off"}
var absNote = map[string]string{"vacation": "Трудовой отпуск", "unpaid": "Отпуск без содержания"}
var absSource = map[string]string{"bitrix": "Bitrix", "telegram": "Telegram", "manual": "Вручную"}

// AttachAbsences добавляет в отчёт отсутствия (ABS) и время их обновления по источникам (ABSAT).
// Имя берётся из Jira по логину сотрудника; если у него нет задач в спринтах — ФИО из справочника.
// Строки Telegram, пересекающиеся с уже добавленным отпуском того же человека, пропускаются (build_abs.js):
// больничный — если пересекается с отпуском/БС, «отпросился» — с любым отсутствием. Поэтому Bitrix передаётся первым.
func AttachAbsences(r *SprintReport, rows []AbsenceRow, updated map[string]time.Time) {
	byLogin := map[string]string{}
	for name, login := range r.Logins {
		if login != "" {
			byLogin[login] = name
		}
	}
	r.Absences = []Absence{}
	for _, a := range rows {
		name := byLogin[a.Login]
		if name == "" {
			name = a.FullName
		}
		t, ok := absType[a.Kind]
		if !ok {
			continue
		}
		note := a.Note
		if n, ok := absNote[a.Kind]; ok && a.Source == "bitrix" {
			note = n
		}
		x := Absence{Name: name, Type: t, From: a.From.Format("2006-01-02"), Source: absSource[a.Source], Note: note}
		if a.To != nil {
			s := a.To.Format("2006-01-02")
			x.To = &s
		}
		if a.Source == "telegram" && overlapsExisting(r.Absences, x) {
			continue
		}
		r.Absences = append(r.Absences, x)
	}
	r.AbsencesAt = updated
}

func overlapsExisting(have []Absence, x Absence) bool {
	to := "9999-12-31"
	if x.To != nil {
		to = *x.To
	}
	for _, a := range have {
		if a.Name != x.Name || (x.Type == "sick" && a.Type == "sick") {
			continue
		}
		ato := "9999-12-31"
		if a.To != nil {
			ato = *a.To
		}
		if a.From <= to && ato >= x.From {
			return true
		}
	}
	return false
}

// TgEvent — событие посещаемости из Telegram (attendance_events, source='telegram').
type TgEvent struct {
	Login    string
	FullName string
	Day      time.Time
	Kind     string // sick | late | early | mid | day
	FromMin  *int
	ToMin    *int
	Minutes  int
}

// TelegramAbsences превращает события Telegram в отсутствия для отчёта, как старые скрипты:
//   - больничный — подряд идущие дни (разрыв только на выходные) → один период (build_abs.js, sick.txt);
//   - «отпросился» — за день сумма опозданий, уходов и отлучек ≥ 2 ч или «весь день» (attendance/build.py, off.json).
//
// Причины не попадают в примечание: «Спринты» видят и lead, и employee, а причины — только admin/hr (docs/security.md).
// Возвращаются только отсутствия, не закончившиеся к дате day.
func TelegramAbsences(events []TgEvent, day time.Time) []AbsenceRow {
	type person struct{ login, name string }
	key := func(e TgEvent) person { return person{e.Login, e.FullName} }
	sick := map[person][]time.Time{}
	type offDay struct {
		minutes int
		whole   bool
		parts   []string
	}
	off := map[person]map[time.Time]*offDay{}
	var order []person
	seen := map[person]bool{}
	for _, e := range events {
		p := key(e)
		if !seen[p] {
			seen[p] = true
			order = append(order, p)
		}
		if e.Kind == "sick" {
			sick[p] = append(sick[p], e.Day)
			continue
		}
		if off[p] == nil {
			off[p] = map[time.Time]*offDay{}
		}
		o := off[p][e.Day]
		if o == nil {
			o = &offDay{}
			off[p][e.Day] = o
		}
		o.minutes += e.Minutes
		o.whole = o.whole || e.Kind == "day"
		if e.FromMin != nil && e.ToMin != nil {
			o.parts = append(o.parts, fmtMin(*e.FromMin)+"–"+fmtMin(*e.ToMin))
		}
	}

	var out []AbsenceRow
	for _, p := range order {
		days := sortTimes(sick[p])
		for i := 0; i < len(days); {
			j := i
			for j+1 < len(days) && onlyWeekendBetween(days[j], days[j+1]) {
				j++
			}
			if !days[j].Before(day) {
				to := days[j]
				out = append(out, AbsenceRow{Login: p.login, FullName: p.name, Kind: "sick", From: days[i], To: &to, Source: "telegram", Note: "Больничный"})
			}
			i = j + 1
		}
		var offDays []time.Time
		for d := range off[p] {
			offDays = append(offDays, d)
		}
		offDays = sortTimes(offDays)
		for _, d := range offDays {
			o := off[p][d]
			if d.Before(day) || !(o.whole || o.minutes >= 120) {
				continue
			}
			note := "Отпросился: весь день"
			if !o.whole {
				note = "Отпросился: " + strings.Join(o.parts, ", ") + " (" + fmtDuration(o.minutes) + ")"
			}
			to := d
			out = append(out, AbsenceRow{Login: p.login, FullName: p.name, Kind: "off", From: d, To: &to, Source: "telegram", Note: note})
		}
	}
	return out
}

// onlyWeekendBetween: b — следующий день после a или между ними только суббота и воскресенье.
func onlyWeekendBetween(a, b time.Time) bool {
	for d := a.AddDate(0, 0, 1); d.Before(b); d = d.AddDate(0, 0, 1) {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			return false
		}
	}
	return !b.Equal(a)
}

// sortTimes сортирует дни и схлопывает дубли (несколько сообщений про один день).
func sortTimes(ts []time.Time) []time.Time {
	sort.Slice(ts, func(i, j int) bool { return ts[i].Before(ts[j]) })
	out := ts[:0]
	for i, t := range ts {
		if i == 0 || !t.Equal(out[len(out)-1]) {
			out = append(out, t)
		}
	}
	return out
}

func fmtMin(m int) string { return fmt.Sprintf("%d:%02d", m/60, m%60) }

func fmtDuration(m int) string {
	s := fmt.Sprintf("%d ч", m/60)
	if m%60 != 0 {
		s += fmt.Sprintf(" %d мин", m%60)
	}
	return s
}
