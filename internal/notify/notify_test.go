package notify

import (
	"testing"

	"github.com/adata/reports/internal/leadcontrol"
)

func TestDiffAndCurrent(t *testing.T) {
	rep := leadcontrol.Report{Lead: "Лид",
		Blocked: []leadcontrol.BlockedRow{{Key: "AC-1", BlockState: "Ждёт: бэк AC-2 (Сделать)"}, {Key: "AC-3", BlockState: "Можно снимать блок · с 30.10"}},
		Gaps:    []leadcontrol.GapRow{{Key: "AC-5", Missing: "Нет фронта", Priority: "Высокий", CreateURL: "https://jira/x"}}}
	cur, ev := Current(rep)
	if len(cur[KindBlocked]) != 2 || !cur[KindUnblock]["AC-3"] || cur[KindUnblock]["AC-1"] || !cur[KindGap]["AC-5"] {
		t.Fatalf("состояние: %v", cur)
	}
	// AC-1 уже был в блоке; AC-3 — новый в блоке и новый «можно снимать»; AC-5 — новый разрыв.
	prev := State{KindBlocked: {"AC-1": true}}
	got := Diff(prev, cur, ev)
	want := []string{"blocked AC-3", "gap AC-5", "unblock AC-3"}
	if len(got) != len(want) {
		t.Fatalf("события: %+v", got)
	}
	for i, e := range got {
		if e.Kind+" "+e.Key != want[i] {
			t.Errorf("%d: %s %s, want %s", i, e.Kind, e.Key, want[i])
		}
	}
	if got[1].CreateURL != "https://jira/x" {
		t.Error("ссылка «Создать продолжение» потерялась")
	}
	if d := Diff(cur, cur, ev); len(d) != 0 {
		t.Errorf("лишние события: %+v", d)
	}
}

func TestBox(t *testing.T) {
	if _, err := NewBox("short"); err != ErrNoKey {
		t.Fatal("короткий ключ должен отклоняться")
	}
	b, _ := NewBox("0123456789abcdef-secret")
	s, err := b.Seal("123:ABC")
	if err != nil {
		t.Fatal(err)
	}
	if p, err := b.Open(s); err != nil || p != "123:ABC" {
		t.Fatalf("open: %q %v", p, err)
	}
	other, _ := NewBox("другой-ключ-0123456789")
	if _, err := other.Open(s); err == nil {
		t.Error("чужой ключ не должен расшифровывать")
	}
}
