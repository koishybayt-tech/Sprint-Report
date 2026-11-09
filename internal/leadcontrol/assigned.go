package leadcontrol

import (
	"sort"
	"time"
)

// ---------- Задачу назначили на руководителя ----------

// AssignedWindow — за какой срок смотрим назначения: о более старых не уведомляем и в отчёте их нет.
const AssignedWindow = 72 * time.Hour

// AssignedRow — задача проекта руководителя, которую назначили на него самого (например, вернули ему).
type AssignedRow struct {
	rowBase
	Key     string    `json:"key"`
	Status  string    `json:"status"`
	Summary string    `json:"summary"`
	From    string    `json:"from"` // кто был исполнителем (пусто — не назначена или неизвестно)
	By      string    `json:"by"`   // кто назначил
	At      time.Time `json:"at"`
}

// Assigned — открытые задачи проектов руководителя, у которых исполнитель — он сам и последнее изменение исполнителя
// сделал не он сам и не раньше AssignedWindow назад. Исполнитель в истории Jira записан внутренним ключом пользователя
// (JIRAUSER…): сверяем его с ключом текущего исполнителя задачи.
func Assigned(in *Input) []AssignedRow {
	out := []AssignedRow{}
	for _, is := range in.Issues {
		if !in.leadProject(is.Project) || is.Assignee != in.Lead || rank(is.Status) >= 5 {
			continue
		}
		var last *Change
		for i := range is.Changes {
			if is.Changes[i].Field == "assignee" {
				last = &is.Changes[i] // история по времени — берём последнюю
			}
		}
		if last == nil || last.To == "" || in.Now.Sub(last.At) > AssignedWindow {
			continue
		}
		if is.AssigneeKey != "" && last.To != is.AssigneeKey {
			continue // исполнитель сменился позже записи, на которую смотрим
		}
		by := in.name(last.Author)
		if by == in.Lead {
			continue // руководитель назначил задачу на себя сам
		}
		out = append(out, AssignedRow{rowBase: rowBase{in.Lead, is.Project}, Key: is.Key, Status: is.Status, Summary: is.Summary,
			From: in.KeyNames[last.From], By: by, At: last.At})
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].At.Equal(out[j].At) {
			return out[i].At.After(out[j].At)
		}
		return out[i].Key < out[j].Key
	})
	return out
}
