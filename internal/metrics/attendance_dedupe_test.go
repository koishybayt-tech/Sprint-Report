package metrics

import (
	"testing"
	"time"
)

// Одно отсутствие, написанное двумя сообщениями, считается один раз (отработка в субботу не удваивается).
func TestDedupeAttRows(t *testing.T) {
	d19 := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
	d20 := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	sat := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	i := func(v int) *int { return &v }
	at := func(h int) time.Time { return time.Date(2026, 11, 20, h, 0, 0, 0, Almaty) }
	rows := []AttRow{
		// Ережепов: «плохое самочувствие» за 19.11 — два сообщения подряд
		{ID: 1, EmployeeID: 1, Name: "Ережепов Руслан", Day: d19, Kind: "day", FromMin: i(540), ToMin: i(1080), Minutes: 540, MakeupKind: "date", MakeupText: "21.11", MakeupDate: &sat, SentAt: at(16)},
		{ID: 2, EmployeeID: 1, Name: "Ережепов Руслан", Day: d19, Kind: "day", FromMin: i(540), ToMin: i(1080), Minutes: 540, MakeupKind: "date", MakeupText: "21.11", MakeupDate: &sat, SentAt: at(18)},
		// Данияров: отлучка 14:30–16:40 — два сообщения; плюс другое опоздание в тот же день — остаётся
		{ID: 3, EmployeeID: 2, Name: "Данияров Мадияр", Day: d20, Kind: "mid", FromMin: i(870), ToMin: i(1000), Minutes: 130, MakeupKind: "saturday", MakeupDate: &sat, SentAt: at(16)},
		{ID: 4, EmployeeID: 2, Name: "Данияров Мадияр", Day: d20, Kind: "mid", FromMin: i(870), ToMin: i(1000), Minutes: 130, ApprovedBy: "Лид", MakeupKind: "saturday", MakeupDate: &sat, SentAt: at(18)},
		{ID: 5, EmployeeID: 2, Name: "Данияров Мадияр", Day: d20, Kind: "late", FromMin: i(540), ToMin: i(570), Minutes: 30, SentAt: at(9)},
		// Архат: два разных дня — это не дубль
		{ID: 6, EmployeeID: 3, Name: "Архат Ерасыл", Day: d19, Kind: "day", FromMin: i(540), ToMin: i(1080), Minutes: 540, MakeupKind: "date", MakeupDate: &sat, SentAt: at(10)},
		{ID: 7, EmployeeID: 3, Name: "Архат Ерасыл", Day: d20, Kind: "day", FromMin: i(540), ToMin: i(1080), Minutes: 540, MakeupKind: "date", MakeupDate: &sat, SentAt: at(10)},
	}
	got := dedupeAttRows(rows)
	if len(got) != 5 {
		t.Fatalf("строк %d, ожидали 5 (дубли Ережепова и Данияров убраны)", len(got))
	}
	if got[1].ID != 3 || got[1].ApprovedBy != "Лид" {
		t.Errorf("остаётся первое сообщение, согласование добирается из повтора: %+v", got[1])
	}

	att := BuildAttendance(rows, &SprintReport{Roles: map[string]string{}, Logins: map[string]string{}}, nil, nil, time.Date(2026, 11, 20, 20, 0, 0, 0, Almaty))
	by := map[string]int{}
	for _, l := range SaturdayMakeups(att.Ev, "2026-11-21") {
		by[l.Name] = l.Minutes
	}
	if by["Ережепов Руслан"] != 480 {
		t.Errorf("Ережепов: %d мин, ожидали 480 (8 ч)", by["Ережепов Руслан"])
	}
	if by["Данияров Мадияр"] != 130 {
		t.Errorf("Данияров: %d мин, ожидали 130 (2 ч 10 мин)", by["Данияров Мадияр"])
	}
	if by["Архат Ерасыл"] != 960 {
		t.Errorf("Архат: %d мин, ожидали 960 (16 ч, два разных дня)", by["Архат Ерасыл"])
	}
}
