// Package tgparse разбирает сообщения группы Adata Group (посещаемость) в события.
//
// Чистая функция: без сети и БД. Правила описаны в docs/telegram-parsing.md,
// проверяются тестами на testdata/telegram_corpus.jsonl (5 619 реальных сообщений).
package tgparse

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Version меняется при любом изменении правил — по ней сервис пересчитывает старые события.
const Version = "1.0.0"

// Рабочий день, минуты от полуночи.
const (
	DayStart = 9 * 60  // 09:00
	DayEnd   = 18 * 60 // 18:00
	slack    = 5       // допуск: 09:05 считается началом дня, 17:55 — концом
)

type MsgType string

const (
	TypeAbsence  MsgType = "absence"  // «Пропуск»: отпросился
	TypeArrived  MsgType = "arrived"  // «upd»: пришёл
	TypeSick     MsgType = "sick"     // больничный
	TypeVacation MsgType = "vacation" // отпуск
	TypeNone     MsgType = "none"     // не про посещаемость
)

type Kind string

const (
	KindLate  Kind = "late"  // опоздание: с начала дня до B
	KindEarly Kind = "early" // ушёл раньше: с A до конца дня
	KindMid   Kind = "mid"   // отлучка днём
	KindDay   Kind = "day"   // весь день
	KindSick  Kind = "sick"
	KindVac   Kind = "vac"
)

type Status string

const (
	StatusOK     Status = "ok"
	StatusManual Status = "manual" // правила не разобрали — в очередь на ручную правку
)

// Event — одно отсутствие в один день.
type Event struct {
	Kind    Kind   `json:"kind"`
	Date    string `json:"date"` // YYYY-MM-DD
	FromMin int    `json:"from_min,omitempty"`
	ToMin   int    `json:"to_min,omitempty"`
	Minutes int    `json:"minutes"`
	Unpaid  bool   `json:"unpaid,omitempty"` // «бс» — без содержания
}

type MakeupKind string

const (
	MakeupNone      MakeupKind = ""
	MakeupToday     MakeupKind = "today"
	MakeupTomorrow  MakeupKind = "tomorrow"
	MakeupYesterday MakeupKind = "yesterday" // отработал заранее
	MakeupSchedule  MakeupKind = "schedule"  // «по расписанию» (учёба)
	MakeupSaturday  MakeupKind = "saturday"
	MakeupDate      MakeupKind = "date"
	MakeupUnpaid    MakeupKind = "unpaid"
	MakeupOther     MakeupKind = "other"
)

type Makeup struct {
	Text string     `json:"text,omitempty"`
	Kind MakeupKind `json:"kind,omitempty"`
	Date string     `json:"date,omitempty"`
}

type Result struct {
	Type       MsgType  `json:"type"`
	Status     Status   `json:"status"`
	Events     []Event  `json:"events,omitempty"`
	Makeup     Makeup   `json:"makeup"`
	Reason     string   `json:"reason,omitempty"`
	ReasonCat  string   `json:"reason_cat,omitempty"` // study | health | family | personal | other
	ApprovedBy []string `json:"approved_by,omitempty"`
	FIO        string   `json:"fio,omitempty"`        // первая строка, если похожа на ФИО
	ArrivedAt  int      `json:"arrived_at,omitempty"` // для upd: минуты от полуночи
}

var (
	reEdited   = regexp.MustCompile(`\n(edited )?\d{1,2}:\d{2}\s*$`)
	reUpd      = regexp.MustCompile(`(?i)^upd\.?$`)
	reSick     = regexp.MustCompile(`(?i)больнич`)
	reVac      = regexp.MustCompile(`(?i)отпуск`)
	reAbs      = regexp.MustCompile(`(?i)пропуск`)
	reMakeupLn = regexp.MustCompile(`(?i)отработ|отрабат`)
	reReasonLn = regexp.MustCompile(`(?i)причин`)
	reLabel    = regexp.MustCompile(`(?i)^\s*(пропуск|отработ\S*|отрабат\S*|причин\S*)\s*[:\-–]?\s*`)
	reMention  = regexp.MustCompile(`@([\w.]+)`)
	reFIO      = regexp.MustCompile(`^[A-Za-zА-Яа-яЁёӘәҒғҚқҢңӨөҰұҮүҺһІі\- ]{3,40}$`)
	reKeyword  = regexp.MustCompile(`(?i)пропуск|отпуск|больнич|отработ|причин|upd`)
	reUnpaid   = regexp.MustCompile(`(?i)(^|[^а-яё])бс([^а-яё]|$)|без содерж`)

	// время: 9 | 9:00 | 9.00 (после нормализации ; и ": " → ":")
	reColonRange = regexp.MustCompile(`(\d{1,2}(?::\d{2})?)\s*-\s*(\d{1,2}:\d{2})`)
	reHourRange  = regexp.MustCompile(`(?:^|[^\d.:/])(\d{1,2})\s*-\s*(\d{1,2})(?:[^\d.:/]|$)`)
	reDotRange   = regexp.MustCompile(`(\d{1,2}(?:\.\d{2})?)\s*-\s*(\d{1,2}\.\d{2})`)
	reTimeTyp1   = regexp.MustCompile(`(\d{1,2}:\d{2}):(\d{1,2}:\d{2})`)     // 9:00:9:30
	reTimeTyp2   = regexp.MustCompile(`(\d{1,2}:\d{2})-(\d{1,2})-(\d{2})\b`) // 9:00-9-30
	reTwoTimes   = regexp.MustCompile(`^(\d{1,2}:\d{2})\s+(\d{1,2}:\d{2})$`) // 9:00 9:30
	reOneTime    = regexp.MustCompile(`^(\d{1,2}:\d{2})$`)
	reColonTime  = regexp.MustCompile(`\d{1,2}:\d{2}`)

	// даты: 05.01 | 5.01 | 05/01 | 05.01.2026 | 05.01.26
	reDate      = regexp.MustCompile(`(\d{1,2})[./](\d{1,2})(?:[./](\d{2,4}))?`)
	reDateRange = regexp.MustCompile(`(\d{1,2})(?:[./](\d{1,2}))?(?:[./]\d{2,4})?\s*-\s*(\d{1,2})[./](\d{1,2})(?:[./](\d{2,4}))?`)
)

var reasonCats = []struct {
	cat string
	re  *regexp.Regexp
}{
	{"study", regexp.MustCompile(`(?i)универ|учеб|учёб|экзам|диплом|колледж|сесси|лекци|пары|магистр|защит`)},
	{"health", regexp.MustCompile(`(?i)врач|здоров|самочув|больн|стоматол|зуб|анализ|операц|клиник|поликлин|давлен|температур|медосмотр|мед\.`)},
	{"family", regexp.MustCompile(`(?i)семей|семья|ребен|ребён|сад|школ|родител|мама|папа|жена|муж|сын|доч`)},
	{"personal", regexp.MustCompile(`(?i)личн|посылк|цон|документ|банк|переезд|ремонт|машин|авто`)},
}

// Parse разбирает текст одного сообщения. sentAt — время отправки (Asia/Almaty).
func Parse(text string, sentAt time.Time) Result {
	text = reEdited.ReplaceAllString(text, "")
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	res := Result{Type: TypeNone, Status: StatusOK}
	if len(lines) == 0 {
		return res
	}
	joined := strings.Join(lines, " / ")
	switch {
	case reUpd.MatchString(lines[len(lines)-1]):
		res.Type = TypeArrived
		res.ArrivedAt = sentAt.Hour()*60 + sentAt.Minute()
		res.FIO = fioLine(lines)
		return res
	case reSick.MatchString(joined):
		res.Type = TypeSick
	case reVac.MatchString(joined):
		res.Type = TypeVacation
	case reAbs.MatchString(joined):
		res.Type = TypeAbsence
	default:
		return res
	}
	res.FIO = fioLine(lines)
	for _, m := range reMention.FindAllStringSubmatch(joined, -1) {
		res.ApprovedBy = append(res.ApprovedBy, m[1])
	}
	if l := findLine(lines, reReasonLn); l != "" {
		res.Reason = strings.TrimSpace(reLabel.ReplaceAllString(l, ""))
	}
	res.ReasonCat = reasonCat(res.Reason)
	if l := findLine(lines, reMakeupLn); l != "" {
		res.Makeup = parseMakeup(strings.TrimSpace(reLabel.ReplaceAllString(l, "")), sentAt)
		if res.ReasonCat == "" && res.Makeup.Kind == MakeupSchedule {
			res.ReasonCat = "study"
		}
	}
	if res.ReasonCat == "" && res.Reason != "" {
		res.ReasonCat = "other"
	}

	day := sentAt.Format("2006-01-02")
	switch res.Type {
	case TypeSick, TypeVacation:
		k := KindSick
		if res.Type == TypeVacation {
			k = KindVac
		}
		dates := parseDates(joined, sentAt)
		if len(dates) == 0 {
			dates = []string{day}
		}
		for _, d := range dates {
			res.Events = append(res.Events, Event{Kind: k, Date: d})
		}
		return res
	}

	absLine := findLine(lines, reAbs)
	body := normalize(strings.TrimSpace(reLabel.ReplaceAllString(absLine, "")))
	unpaid := reUnpaid.MatchString(body)
	ranges, rest := parseRanges(body)
	dates := parseDates(rest, sentAt)
	switch {
	case len(ranges) > 0:
		ds := dates
		if len(ds) == 0 {
			ds = []string{day}
		}
		for _, d := range ds {
			for _, r := range ranges {
				res.Events = append(res.Events, classify(d, r[0], r[1], unpaid))
			}
		}
	case len(dates) > 0:
		for _, d := range dates {
			res.Events = append(res.Events, Event{Kind: KindDay, Date: d, FromMin: DayStart, ToMin: DayEnd, Minutes: DayEnd - DayStart, Unpaid: unpaid})
		}
	default:
		if m := reOneTime.FindStringSubmatch(body); m != nil {
			if a, ok := toMin(m[1]); ok && a > DayStart && a < DayEnd {
				res.Events = append(res.Events, classify(day, a, DayEnd, unpaid))
				break
			}
		}
		res.Status = StatusManual
	}
	return res
}

func classify(date string, a, b int, unpaid bool) Event {
	e := Event{Date: date, FromMin: a, ToMin: b, Unpaid: unpaid}
	switch {
	case a <= DayStart+slack && b >= DayEnd-slack:
		e.Kind, e.Minutes = KindDay, DayEnd-DayStart
	case a <= DayStart+slack:
		e.Kind, e.Minutes = KindLate, max(0, b-DayStart)
	case b >= DayEnd-slack:
		e.Kind, e.Minutes = KindEarly, max(0, DayEnd-a)
	default:
		e.Kind, e.Minutes = KindMid, b-a
	}
	return e
}

// normalize приводит запись времени к виду «H:MM-H:MM».
func normalize(s string) string {
	r := strings.NewReplacer("–", "-", "—", "-", "−", "-", ";", ":", "：", ":", "до", "-")
	s = r.Replace(strings.ToLower(s))
	s = regexp.MustCompile(`(\d)\s*:\s+(\d)`).ReplaceAllString(s, "$1:$2") // "10: 30"
	s = regexp.MustCompile(`-+\s*:?`).ReplaceAllString(s, "-")             // "--", "-:"
	s = regexp.MustCompile(`(^|\s)с\s+(\d)`).ReplaceAllString(s, "$1$2")   // "с 9:00"
	s = strings.ReplaceAll(s, "примерно", "")
	s = strings.ReplaceAll(s, "около", "")
	return s
}

// parseRanges ищет интервалы времени; возвращает их и строку без них (для поиска дат).
func parseRanges(s string) ([][2]int, string) {
	var out [][2]int
	add := func(a, b int) bool {
		if a >= 6*60 && b <= 24*60 && b > a {
			out = append(out, [2]int{a, b})
			return true
		}
		return false
	}
	cut := func(re *regexp.Regexp, f func(m []string) bool) {
		s = re.ReplaceAllStringFunc(s, func(x string) string {
			if f(re.FindStringSubmatch(x)) {
				return " "
			}
			return x
		})
	}
	cut(reTimeTyp1, func(m []string) bool { a, _ := toMin(m[1]); b, _ := toMin(m[2]); return add(a, b) })
	cut(reTimeTyp2, func(m []string) bool { a, _ := toMin(m[1]); b, _ := toMin(m[2] + ":" + m[3]); return add(a, b) })
	cut(reColonRange, func(m []string) bool { a, ok1 := toMin(m[1]); b, ok2 := toMin(m[2]); return ok1 && ok2 && add(a, b) })
	if t := strings.TrimSpace(s); reTwoTimes.MatchString(t) {
		m := reTwoTimes.FindStringSubmatch(t)
		a, _ := toMin(m[1])
		b, _ := toMin(m[2])
		if add(a, b) {
			s = ""
		}
	}
	if len(out) == 0 && !reColonTime.MatchString(s) {
		// «9.00-10.00» — время через точку, если концы не похожи на даты
		cut(reDotRange, func(m []string) bool {
			if looksLikeDate(m[1]) && looksLikeDate(m[2]) {
				return false
			}
			a, ok1 := toMin(strings.ReplaceAll(m[1], ".", ":"))
			b, ok2 := toMin(strings.ReplaceAll(m[2], ".", ":"))
			return ok1 && ok2 && add(a, b)
		})
		// «9-10» — часы
		if !reDate.MatchString(s) {
			cut(reHourRange, func(m []string) bool {
				a, _ := strconv.Atoi(m[1])
				b, _ := strconv.Atoi(m[2])
				return a <= 23 && b <= 24 && add(a*60, b*60)
			})
		}
	}
	return out, s
}

func looksLikeDate(t string) bool {
	p := strings.Split(t, ".")
	if len(p) != 2 {
		return false
	}
	d, _ := strconv.Atoi(p[0])
	m, _ := strconv.Atoi(p[1])
	return d >= 1 && d <= 31 && m >= 1 && m <= 12
}

func toMin(t string) (int, bool) {
	p := strings.SplitN(t, ":", 2)
	h, err := strconv.Atoi(p[0])
	if err != nil || h > 24 {
		return 0, false
	}
	m := 0
	if len(p) == 2 {
		if m, err = strconv.Atoi(p[1]); err != nil || m > 59 {
			return 0, false
		}
	}
	return h*60 + m, true
}

// parseDates возвращает даты (YYYY-MM-DD) из текста: одиночные, списки, диапазоны.
func parseDates(s string, sentAt time.Time) []string {
	seen := map[string]bool{}
	var out []string
	push := func(t time.Time) {
		k := t.Format("2006-01-02")
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	for _, m := range reDateRange.FindAllStringSubmatch(s, -1) {
		d1, _ := strconv.Atoi(m[1])
		d2, _ := strconv.Atoi(m[3])
		mo2, _ := strconv.Atoi(m[4])
		mo1 := mo2
		if m[2] != "" {
			mo1, _ = strconv.Atoi(m[2])
		}
		a, ok1 := mkDate(d1, mo1, m[5], sentAt)
		b, ok2 := mkDate(d2, mo2, m[5], sentAt)
		if !ok1 || !ok2 || b.Before(a) || b.Sub(a) > 62*24*time.Hour {
			continue
		}
		for t := a; !t.After(b); t = t.AddDate(0, 0, 1) {
			push(t)
		}
		s = strings.Replace(s, m[0], " ", 1)
	}
	for _, m := range reDate.FindAllStringSubmatch(s, -1) {
		d, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		if t, ok := mkDate(d, mo, m[3], sentAt); ok {
			push(t)
		}
	}
	sort.Strings(out)
	return out
}

// mkDate: год не указан → год сообщения; дата раньше сообщения больше чем на полгода → следующий год.
func mkDate(d, mo int, yr string, sentAt time.Time) (time.Time, bool) {
	if d < 1 || d > 31 || mo < 1 || mo > 12 {
		return time.Time{}, false
	}
	y := sentAt.Year()
	if yr != "" {
		v, _ := strconv.Atoi(yr)
		if v < 100 {
			v += 2000
		}
		y = v
	}
	t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, sentAt.Location())
	if t.Day() != d {
		return time.Time{}, false
	}
	if yr == "" && sentAt.Sub(t) > 183*24*time.Hour {
		t = t.AddDate(1, 0, 0)
	}
	return t, true
}

func parseMakeup(s string, sentAt time.Time) Makeup {
	mk := Makeup{Text: s}
	l := strings.ToLower(s)
	day := func(n int) string { return sentAt.AddDate(0, 0, n).Format("2006-01-02") }
	switch {
	case l == "" || l == "-" || l == "нет":
		mk.Kind = MakeupNone
	case strings.Contains(l, "сегодня"):
		mk.Kind, mk.Date = MakeupToday, day(0)
	case strings.Contains(l, "завтра"):
		mk.Kind, mk.Date = MakeupTomorrow, day(1)
	case strings.Contains(l, "вчера"):
		mk.Kind, mk.Date = MakeupYesterday, day(-1)
	case strings.Contains(l, "расписан"):
		mk.Kind = MakeupSchedule
	case strings.Contains(l, "суббот"):
		n := (int(time.Saturday) - int(sentAt.Weekday()) + 7) % 7
		if n == 0 {
			n = 7
		}
		mk.Kind, mk.Date = MakeupSaturday, day(n)
	case reUnpaid.MatchString(l):
		mk.Kind = MakeupUnpaid
	default:
		if ds := parseDates(l, sentAt); len(ds) > 0 {
			mk.Kind, mk.Date = MakeupDate, ds[0]
		} else {
			mk.Kind = MakeupOther
		}
	}
	return mk
}

func reasonCat(r string) string {
	if r == "" {
		return ""
	}
	for _, c := range reasonCats {
		if c.re.MatchString(r) {
			return c.cat
		}
	}
	return "other"
}

func findLine(lines []string, re *regexp.Regexp) string {
	for _, l := range lines {
		if re.MatchString(l) {
			return l
		}
	}
	return ""
}

func fioLine(lines []string) string {
	l := lines[0]
	if reFIO.MatchString(l) && !reKeyword.MatchString(l) {
		if n := len(strings.Fields(l)); n == 2 || n == 3 {
			return l
		}
	}
	return ""
}
