package tgparse

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"
)

var almaty = time.FixedZone("Asia/Almaty", 5*3600)

type corpusRow struct {
	ID       int    `json:"id"`
	SentAt   string `json:"sent_at"`
	Text     string `json:"text"`
	Baseline struct {
		Type   string   `json:"type"`
		Ranges [][2]int `json:"ranges_min"`
		Makeup string   `json:"makeup"`
	} `json:"baseline"`
}

func loadCorpus(t *testing.T) []corpusRow {
	f, err := os.Open("../../testdata/telegram_corpus.jsonl")
	if err != nil {
		t.Skip("corpus not found:", err)
	}
	defer f.Close()
	var out []corpusRow
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var r corpusRow
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func ranges(r Result) [][2]int {
	var out [][2]int
	seen := map[[2]int]bool{}
	for _, e := range r.Events {
		if e.Kind == KindSick || e.Kind == KindVac || e.Kind == KindDay {
			continue
		}
		k := [2]int{e.FromMin, e.ToMin}
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

func normBase(b [][2]int) [][2]int {
	seen := map[[2]int]bool{}
	var out [][2]int
	for _, x := range b {
		a, c := x[0], x[1]
		if a <= DayStart+slack && c >= DayEnd-slack { // целый день — сравниваем отдельно
			continue
		}
		if a <= DayStart+slack {
			a = DayStart
		}
		if c >= DayEnd-slack {
			c = DayEnd
		}
		k := [2]int{a, c}
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

// TestCorpus — «золотой» прогон по реальной истории группы.
// Пороги: тип ≥ 99%, диапазоны пропусков совпадают с эталоном ≥ 97%,
// ручная очередь ≤ 3% пропусков.
func TestCorpus(t *testing.T) {
	rows := loadCorpus(t)
	var nType, okType, nAbs, okRng, manual int
	var bad []string
	for _, r := range rows {
		ts, err := time.ParseInLocation("2006-01-02T15:04:05", r.SentAt, almaty)
		if err != nil {
			t.Fatal(err)
		}
		res := Parse(r.Text, ts)
		nType++
		if string(res.Type) == r.Baseline.Type {
			okType++
		} else if len(bad) < 40 {
			bad = append(bad, fmt.Sprintf("TYPE %d want=%s got=%s %q", r.ID, r.Baseline.Type, res.Type, r.Text))
		}
		if r.Baseline.Type != "absence" {
			continue
		}
		nAbs++
		if res.Status == StatusManual {
			manual++
		}
		want, got := normBase(r.Baseline.Ranges), ranges(res)
		if fmt.Sprint(want) == fmt.Sprint(got) {
			okRng++
		} else if len(bad) < 80 {
			bad = append(bad, fmt.Sprintf("RNG %d want=%v got=%v %q", r.ID, want, got, r.Text))
		}
	}
	pt := 100 * float64(okType) / float64(nType)
	pr := 100 * float64(okRng) / float64(nAbs)
	pm := 100 * float64(manual) / float64(nAbs)
	t.Logf("messages=%d type=%.2f%% absences=%d ranges=%.2f%% manual=%.2f%%", nType, pt, nAbs, pr, pm)
	if pt < 99 || pr < 97 || pm > 3 {
		for _, b := range bad {
			t.Log(b)
		}
		t.Fatalf("below threshold")
	}
}

func d(s string) time.Time {
	ts, _ := time.ParseInLocation("2006-01-02 15:04", s, almaty)
	return ts
}

// TestVariations — по одному примеру на каждый вариант записи из ТЗ.
func TestVariations(t *testing.T) {
	sent := d("2026-10-05 08:30") // понедельник
	cases := []struct {
		name, text string
		typ        MsgType
		kind       Kind
		from, to   int
		makeup     MakeupKind
	}{
		{"обычный", "Иванов Иван\nПропуск 9:00-10:00\nОтработка сегодня", TypeAbsence, KindLate, 540, 600, MakeupToday},
		{"длинное тире", "Пропуск 9:00–10:30\nОтработка завтра", TypeAbsence, KindLate, 540, 630, MakeupTomorrow},
		{"с ... до", "Пропуск с 14:00 до 15:00", TypeAbsence, KindMid, 840, 900, ""},
		{"точки", "Пропуск 9.00-9.30", TypeAbsence, KindLate, 540, 570, ""},
		{"двоеточие-опечатка", "Пропуск 9:00:9:30", TypeAbsence, KindLate, 540, 570, ""},
		{"дефис в минутах", "Пропуск 9:00-9-30", TypeAbsence, KindLate, 540, 570, ""},
		{"точка с запятой", "Пропуск 9;00-10;00", TypeAbsence, KindLate, 540, 600, ""},
		{"часы", "Пропуск 9-10", TypeAbsence, KindLate, 540, 600, ""},
		{"уход раньше", "Пропуск 16:00-18:00\nОтработка завтра", TypeAbsence, KindEarly, 960, 1080, MakeupTomorrow},
		{"одно время = уход", "Пропуск 17:00", TypeAbsence, KindEarly, 1020, 1080, ""},
		{"весь день", "Пропуск 9:00-18:00", TypeAbsence, KindDay, 540, 1080, ""},
		{"по расписанию", "Пропуск 9:00-11:00\nОтработка по расписанию", TypeAbsence, KindLate, 540, 660, MakeupSchedule},
		{"бс", "Пропуск 9:00-18:00 бс", TypeAbsence, KindDay, 540, 1080, ""},
		{"upd", "upd", TypeArrived, "", 0, 0, ""},
		{"больничный", "Больничный", TypeSick, KindSick, 0, 0, ""},
		{"отпуск даты", "Отпуск 12.10-16.10", TypeVacation, KindVac, 0, 0, ""},
		{"не про посещаемость", "Коллеги, кто брал зарядку?", TypeNone, "", 0, 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Parse(c.text, sent)
			if r.Type != c.typ {
				t.Fatalf("type: want %s got %s (%+v)", c.typ, r.Type, r)
			}
			if c.kind != "" {
				if len(r.Events) == 0 {
					t.Fatalf("no events: %+v", r)
				}
				e := r.Events[0]
				if e.Kind != c.kind {
					t.Fatalf("kind: want %s got %s (%+v)", c.kind, e.Kind, e)
				}
				if c.from != 0 && (e.FromMin != c.from || e.ToMin != c.to) {
					t.Fatalf("range: want %d-%d got %d-%d", c.from, c.to, e.FromMin, e.ToMin)
				}
			}
			if c.makeup != "" && r.Makeup.Kind != c.makeup {
				t.Fatalf("makeup: want %s got %s", c.makeup, r.Makeup.Kind)
			}
		})
	}
	// отпуск по датам — 5 событий (пн-пт)
	if r := Parse("Отпуск 12.10-16.10", sent); len(r.Events) != 5 {
		t.Fatalf("vacation days: %d %+v", len(r.Events), r.Events)
	}
	// бс → Unpaid
	if r := Parse("Пропуск 9:00-18:00 бс", sent); !r.Events[0].Unpaid {
		t.Fatal("unpaid not set")
	}
	// upd → время прихода из sentAt
	if r := Parse("upd", d("2026-10-05 09:12")); r.ArrivedAt != 9*60+12 {
		t.Fatalf("arrived_at %d", r.ArrivedAt)
	}
	// год: январь, отправлено в декабре → следующий год
	if r := Parse("Отпуск 05.01-06.01", d("2026-12-20 10:00")); len(r.Events) == 0 || r.Events[0].Date != "2027-01-05" {
		t.Fatalf("year inference: %+v", r.Events)
	}
}
