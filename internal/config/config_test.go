package config

import (
	"testing"
	"time"
)

func TestInSyncHours(t *testing.T) {
	alm := time.FixedZone("Almaty", 5*3600)
	at := func(h, m int) time.Time { return time.Date(2026, 11, 2, h, m, 0, 0, alm) }
	from, to, err := hoursRange("", "08:30-20:00")
	if err != nil {
		t.Fatal(err)
	}
	c := Config{SyncFrom: from, SyncTo: to}
	for _, x := range []struct {
		h, m int
		want bool
	}{{8, 29, false}, {8, 30, true}, {12, 0, true}, {19, 59, true}, {20, 0, false}, {3, 0, false}} {
		if got := c.InSyncHours(at(x.h, x.m)); got != x.want {
			t.Errorf("%02d:%02d: %v", x.h, x.m, got)
		}
	}
	if !c.InSyncHours(time.Date(2026, 11, 2, 4, 0, 0, 0, time.UTC)) { // 09:00 по Алматы
		t.Error("время в UTC должно переводиться в Алматы")
	}
	if f, tt, _ := hoursRange("off", "08:30-20:00"); !(Config{SyncFrom: f, SyncTo: tt}).InSyncHours(at(3, 0)) {
		t.Error("off — круглосуточно")
	}
	if _, _, err := hoursRange("8-20", "08:30-20:00"); err == nil {
		t.Error("ожидалась ошибка формата")
	}
}
