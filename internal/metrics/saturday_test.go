package metrics

import "testing"

func TestSaturdayMakeups(t *testing.T) {
	sat := "2026-11-07"
	ev := []AttEvent{
		{N: "Иванов", T: "late", M: 80, Otr: "9:00-10:20 (Отработан суббота)", Od: sat},
		{N: "Петров", T: "mid", M: 105, Otr: "в субботу", Od: sat},
		{N: "Петров", T: "late", M: 30, Otr: "07.11", Od: sat},
		{N: "Сидоров", T: "day", M: 540, Otr: "07.11", Od: sat},
		{N: "Кузнецов", T: "late", M: 60, Otr: "с 10.30", Od: sat},
		{N: "Другой", T: "late", M: 60, Otr: "31.10", Od: "2026-10-31"}, // другая суббота
		{N: "Больной", T: "sick", Od: sat},
	}
	got := SaturdayMakeups(ev, sat)
	want := []SaturdayLine{{"Иванов", "09:00–10:20", 80}, {"Кузнецов", "с 10:30", 60}, {"Петров", "", 135}, {"Сидоров", "", 480}}
	if len(got) != len(want) {
		t.Fatalf("%+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d: %+v, want %+v", i, got[i], want[i])
		}
	}
	msg := SaturdayMessage(sat, got)
	wantMsg := "Отработки в субботу, 07.11:\n• Иванов — 09:00–10:20\n• Кузнецов — с 10:30\n• Петров — 2 ч 15 мин\n• Сидоров — 8 ч\nВсего: 4 чел."
	if msg != wantMsg {
		t.Errorf("сообщение:\n%s", msg)
	}
	if SaturdayMessage(sat, nil) != "Отработки в субботу, 07.11: никто не отрабатывает." {
		t.Error("пустой список")
	}
	if s := NextSaturday(at("2026-11-06 20:30")); s != "2026-11-07" {
		t.Errorf("из пятницы: %s", s)
	}
	if s := NextSaturday(at("2026-11-07 10:00")); s != "2026-11-14" {
		t.Errorf("из субботы: %s", s)
	}
}
