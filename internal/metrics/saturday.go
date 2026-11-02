package metrics

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Субботние отработки: в пятницу вечером бот пишет в группу «Adata Group», кто отрабатывает в субботу —
// со скольки до скольки (если человек написал время) или сколько часов. Источник — тот же, что «План отработок»
// на странице посещаемости: события, у которых дата отработки (od) — эта суббота.

type SaturdayLine struct {
	Name    string
	Time    string // «10:00–14:00», «с 10:00» или ""
	Minutes int    // сколько отрабатывает (сумма пропусков), если время не указано
}

var (
	reOtRange = regexp.MustCompile(`(\d{1,2})[:.](\d{2})\s*[-–—]\s*(\d{1,2})[:.](\d{2})`)
	reOtFrom  = regexp.MustCompile(`(?i)(?:с|после)\s*(\d{1,2})[:.](\d{2})`)
)

// makeupTime — время отработки из текста сообщения (как otT на странице посещаемости).
func makeupTime(s string) string {
	pad := func(h string) string { return fmt.Sprintf("%02s", h) }
	if m := reOtRange.FindStringSubmatch(s); m != nil {
		return pad(m[1]) + ":" + m[2] + "–" + pad(m[3]) + ":" + m[4]
	}
	if m := reOtFrom.FindStringSubmatch(s); m != nil {
		return "с " + pad(m[1]) + ":" + m[2]
	}
	return ""
}

// SaturdayMakeups — кто отрабатывает в субботу sat (YYYY-MM-DD): по человеку — время, если написано, и сумма часов.
func SaturdayMakeups(ev []AttEvent, sat string) []SaturdayLine {
	by := map[string]*SaturdayLine{}
	for _, e := range ev {
		if e.Od != sat || e.T == "sick" || e.T == "vac" {
			continue
		}
		l := by[e.N]
		if l == nil {
			l = &SaturdayLine{Name: e.N}
			by[e.N] = l
		}
		m := e.M
		if e.T == "day" {
			m = 480
		}
		l.Minutes += m
		if t := makeupTime(e.Otr); t != "" && l.Time == "" {
			l.Time = t
		}
	}
	out := make([]SaturdayLine, 0, len(by))
	for _, l := range by {
		out = append(out, *l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// NextSaturday — ближайшая суббота после now (в пятницу — завтра), по Алматы.
func NextSaturday(now time.Time) string {
	d := now.In(Almaty)
	for {
		d = d.AddDate(0, 0, 1)
		if d.Weekday() == time.Saturday {
			return d.Format("2006-01-02")
		}
	}
}

func hoursText(m int) string {
	h, r := m/60, m%60
	switch {
	case h == 0:
		return fmt.Sprintf("%d мин", r)
	case r == 0:
		return fmt.Sprintf("%d ч", h)
	}
	return fmt.Sprintf("%d ч %d мин", h, r)
}

// SaturdayMessage — текст в группу. Пустой список — «отработок нет».
func SaturdayMessage(sat string, lines []SaturdayLine) string {
	d, _ := time.Parse("2006-01-02", sat)
	head := "Отработки в субботу, " + d.Format("02.01")
	if len(lines) == 0 {
		return head + ": никто не отрабатывает."
	}
	var b strings.Builder
	b.WriteString(head + ":\n")
	for _, l := range lines {
		if l.Time != "" {
			fmt.Fprintf(&b, "• %s — %s\n", l.Name, l.Time)
		} else {
			fmt.Fprintf(&b, "• %s — %s\n", l.Name, hoursText(l.Minutes))
		}
	}
	fmt.Fprintf(&b, "Всего: %d чел.", len(lines))
	return b.String()
}
