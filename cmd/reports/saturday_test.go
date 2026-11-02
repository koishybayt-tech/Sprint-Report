package main

import (
	"testing"
	"time"

	"github.com/adata/reports/internal/metrics"
)

func TestSaturdayDue(t *testing.T) {
	at := func(s string) time.Time {
		x, _ := time.ParseInLocation("2006-01-02 15:04", s, metrics.Almaty)
		return x
	}
	for s, want := range map[string]bool{
		"2026-11-06 20:29": false, // пятница, ещё рано
		"2026-11-06 20:30": true,
		"2026-11-06 23:50": true,  // worker поднялся поздно — ещё успеваем
		"2026-11-07 09:00": false, // суббота — уже поздно
		"2026-11-05 21:00": false, // четверг
	} {
		if got := saturdayDue(at(s), "20:30"); got != want {
			t.Errorf("%s: %v", s, got)
		}
	}
}
