package notify

import (
	"fmt"
	"sort"
	"strings"

	"github.com/adata/reports/internal/leadcontrol"
)

// Kind — события, на которые руководитель может подписаться.
const (
	KindBlocked  = "blocked"  // задача попала в Blocked
	KindUnblock  = "unblock"  // задачу можно вытащить из блока
	KindGap      = "gap"      // задача завершилась без продолжения (дизайн или бэк готов, продолжения нет)
	KindAssigned = "assigned" // задачу назначили на самого руководителя (например, вернули ему)
)

// markerAssigned — в lead_notify_state: по руководителю уже фиксировались назначения. Пока метки нет (событие появилось
// позже остальных), назначения только запоминаются, а не рассылаются: иначе придёт всё, что накопилось за 3 дня.
const markerAssigned = "_init_assigned"

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
	st := State{KindBlocked: {}, KindUnblock: {}, KindGap: {}, KindAssigned: {}, markerAssigned: {"": true}}
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
	// Назначена на руководителя: событие на каждое назначение (ключ + время), чтобы повторное назначение снова уведомило.
	for _, a := range r.Assigned {
		sk := fmt.Sprintf("%s@%d", a.Key, a.At.Unix())
		st[KindAssigned][sk] = true
		ev[KindAssigned+sk] = Event{Kind: KindAssigned, Lead: r.Lead, Key: a.Key, Text: assignedText(a)}
	}
	return st, ev
}

func assignedText(a leadcontrol.AssignedRow) string {
	was := ""
	if a.From != "" {
		was = "было: " + a.From + ", "
	}
	by := a.By
	if by == "" {
		by = "—"
	}
	return fmt.Sprintf("👤 %s назначена на вас (%sназначил: %s) · %s", a.Key, was, by, a.Summary)
}

// dropUnseen убирает назначения, если по руководителю их ещё не фиксировали (первый расчёт после появления этого события).
func dropUnseen(prev State, events []Event) []Event {
	if prev[markerAssigned][""] {
		return events
	}
	out := events[:0:0]
	for _, e := range events {
		if e.Kind != KindAssigned {
			out = append(out, e)
		}
	}
	return out
}

// Diff — события, которых не было в прошлом состоянии. Задача, которая ушла из списка и вернулась, снова даёт событие.
func Diff(prev, cur State, ev map[string]Event) []Event {
	var out []Event
	for _, kind := range []string{KindBlocked, KindUnblock, KindGap, KindAssigned} {
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
