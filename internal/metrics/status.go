// Package metrics — формулы отчётов (docs/metrics.md), перенесённые 1-в-1 из reference/.
package metrics

// Группы статусов Jira по status.id (docs/metrics.md §1).
const (
	Todo = "todo" // 10100 «Сделать» (в reference — bl, бэклог)
	Wk   = "wk"   // 10011 «В работе»
	Cr   = "cr"   // 10008 Code review
	Rw   = "rw"   // 10000 Need Rework
	Dn   = "dn"   // 10001 «Готово»
	Ts   = "ts"   // всё остальное: Test, In Staging, Ready for deploy, Blocked…
)

var statusGroups = map[string]string{"10100": Todo, "10011": Wk, "10008": Cr, "10000": Rw, "10001": Dn}

func StatusGroup(statusID string) string {
	if g, ok := statusGroups[statusID]; ok {
		return g
	}
	return Ts
}
