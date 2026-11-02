package notify

import (
	"fmt"
	"sort"
	"strings"

	"github.com/adata/reports/internal/leadcontrol"
)

// Kind — события, на которые руководитель может подписаться.
const (
	KindBlocked = "blocked" // задача попала в Blocked
	KindUnblock = "unblock" // задачу можно вытащить из блока
	KindGap     = "gap"     // задача завершилась без продолжения (дизайн или бэк готов, продолжения нет)
)

// Event — новое событие по задаче руководителя.
type Event struct {
	Kind      string
	Lead      string
	Key       string
	Text      string
	CreateURL string // для gap — «Создать продолжение»
}

// State — что сейчас по руководителю: вид события → ключи задач.
type State map[string]map[string]bool

// Current — состояние по отчёту: заблокированные, с которых можно снимать блок, разрывы (без скрытых).
func Current(r leadcontrol.Report) (State, map[string]Event) {
	st := State{KindBlocked: {}, KindUnblock: {}, KindGap: {}}
	ev := map[string]Event{}
	for _, b := range r.Blocked {
		st[KindBlocked][b.Key] = true
		ev[KindBlocked+b.Key] = Event{Kind: KindBlocked, Lead: r.Lead, Key: b.Key,
			Text: fmt.Sprintf("🔒 %s попала в блок · %s · %s", b.Key, orDash(b.Assignee), b.Summary)}
		if strings.HasPrefix(b.BlockState, leadcontrol.StateCanUnblock) {
			st[KindUnblock][b.Key] = true
			ev[KindUnblock+b.Key] = Event{Kind: KindUnblock, Lead: r.Lead, Key: b.Key,
				Text: fmt.Sprintf("🔓 %s можно вытащить из блока: %s · %s", b.Key, strings.ToLower(b.BlockState), b.Summary)}
		}
	}
	for _, g := range r.Gaps {
		st[KindGap][g.Key] = true
		ev[KindGap+g.Key] = Event{Kind: KindGap, Lead: r.Lead, Key: g.Key, CreateURL: g.CreateURL,
			Text: fmt.Sprintf("➡️ %s завершилась без продолжения (%s, приоритет «%s»): %s · %s", g.Key, strings.ToLower(g.Missing), g.Priority, g.Problem, g.Summary)}
	}
	return st, ev
}

// Diff — события, которых не было в прошлом состоянии. Задача, которая ушла из списка и вернулась, снова даёт событие.
func Diff(prev, cur State, ev map[string]Event) []Event {
	var out []Event
	for _, kind := range []string{KindBlocked, KindUnblock, KindGap} {
		for key := range cur[kind] {
			if !prev[kind][key] {
				out = append(out, ev[kind+key])
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func orDash(s string) string {
	if s == "" {
		return "не назначена"
	}
	return s
}
