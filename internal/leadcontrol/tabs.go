package leadcontrol

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Общие поля строк: руководитель и проект — для фильтров страницы.
type rowBase struct {
	Lead    string `json:"lead"`
	Project string `json:"project"`
}

// ---------- 1. Заблокированные задачи ----------

type BlockedRow struct {
	rowBase
	Key             string `json:"key"`
	Assignee        string `json:"assignee"`
	Role            string `json:"role"`
	DaysBlocked     int    `json:"days_blocked"`
	BlockState      string `json:"block_state"` // «Можно снимать блок · с 30.10» | «Ждёт …» | нет связей
	WaitingKey      string `json:"waiting_key"`
	WaitingRole     string `json:"waiting_role"`
	WaitingStatus   string `json:"waiting_status"`
	WaitingAssignee string `json:"waiting_assignee"`
	NotStarted      string `json:"not_started"` // «да» — задача, которую ждёт, в «Сделать»; «» — ждать некого
	Blockers        string `json:"blockers"`    // блокирующие задачи: статус, когда ушла на тест
	Summary         string `json:"summary"`
	Related         string `json:"related"` // прочие связанные
}

const (
	StateCanUnblock = "Можно снимать блок"
	stateNoLinks    = "Нет связанных задач, которых она ждёт"
)

// waits — какие роли ждёт задача: фронт — дизайн и бэк (сначала бэк), бэк и ИИ — только дизайн, дизайн — никого.
func waits(role string) []string {
	switch kind(role) {
	case roleFront:
		return []string{roleBack, roleDesign}
	case roleBack:
		return []string{roleDesign}
	}
	return nil
}

// Blocked — задачи в статусе Blocked: кого ждут и можно ли снимать блок.
func Blocked(in *Input) []BlockedRow {
	out := []BlockedRow{}
	for _, is := range in.sprintIssues() {
		if is.Status != stBlocked {
			continue
		}
		role := Role(is)
		r := BlockedRow{rowBase: rowBase{in.Lead, is.Project}, Key: is.Key, Assignee: is.Assignee, Role: role,
			DaysBlocked: days(in.Now, lastEnter(is, stBlocked)), Summary: is.Summary}

		// Связанные задачи: блокирующие — нужных ролей (в порядке ожидания), остальные — «прочие».
		want := waits(role)
		byKind := map[string][]*Issue{}
		var related []string
		seen := map[string]bool{}
		for _, l := range is.Links {
			if seen[l.Key] || l.Key == is.Key {
				continue
			}
			seen[l.Key] = true
			li := in.Issues[l.Key]
			if li == nil {
				related = append(related, fmt.Sprintf("%s (%s)", l.Key, l.Type))
				continue
			}
			k := kind(Role(li))
			if contains(want, k) {
				byKind[k] = append(byKind[k], li)
			} else {
				related = append(related, fmt.Sprintf("%s (%s, %s)", li.Key, l.Type, li.Status))
			}
		}
		var blockers []*Issue
		for _, k := range want {
			sort.Slice(byKind[k], func(i, j int) bool { return keyLess(byKind[k][i].Key, byKind[k][j].Key) })
			blockers = append(blockers, byKind[k]...)
		}
		var parts []string
		var waiting *Issue
		var lastTest *time.Time
		for _, b := range blockers {
			p := fmt.Sprintf("%s %s — %s", b.Key, Role(b), b.Status)
			if atTest(b.Status) {
				if t := testSince(b); t != nil {
					p += " с " + ddmm(*t)
					if lastTest == nil || t.After(*lastTest) {
						lastTest = t
					}
				}
			} else if waiting == nil {
				waiting = b
			}
			parts = append(parts, p)
		}
		r.Blockers = strings.Join(parts, "; ")
		r.Related = strings.Join(related, "; ")
		switch {
		case len(blockers) == 0:
			r.BlockState = stateNoLinks
		case waiting == nil:
			r.BlockState = StateCanUnblock
			if lastTest != nil {
				r.BlockState += " · с " + ddmm(*lastTest)
			}
		default:
			r.WaitingKey, r.WaitingRole, r.WaitingStatus, r.WaitingAssignee = waiting.Key, Role(waiting), waiting.Status, waiting.Assignee
			r.NotStarted = yesNo(rank(waiting.Status) == 0)
			r.BlockState = fmt.Sprintf("Ждёт: %s %s (%s)", strings.ToLower(Role(waiting)), waiting.Key, waiting.Status)
		}
		out = append(out, r)
	}
	return out
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// ---------- 2. Долго в работе ----------

type InWorkRow struct {
	rowBase
	Key               string `json:"key"`
	Status            string `json:"status"`
	Assignee          string `json:"assignee"`
	Role              string `json:"role"`
	DaysInStatus      int    `json:"days_in_status"`
	DaysSinceStart    *int   `json:"days_since_start"` // с первого перехода в «В работе»; null — не было
	TotalDaysInStatus int    `json:"total_days_in_status"`
	Reworks           int    `json:"reworks"` // переходов в Need Rework
	Sprints           int    `json:"sprints"`
	Summary           string `json:"summary"`
}

const (
	LongWorkDays = 5 // «Долго в работе»: от 5 дн. — оранжевым и в счётчике, от 10 — красным
	LongTestDays = 3 // «Долго на тесте»: от 3 дн. — оранжевым и в счётчике, от 7 — красным
)

// InWork — все задачи в «В работе», Code review и Need Rework (порог 5 дн. применяет страница и счётчик).
func InWork(in *Input) []InWorkRow {
	out := []InWorkRow{}
	for _, is := range in.sprintIssues() {
		if is.Status != stWork && is.Status != stReview && is.Status != stRework {
			continue
		}
		out = append(out, InWorkRow{rowBase: rowBase{in.Lead, is.Project}, Key: is.Key, Status: is.Status, Assignee: is.Assignee,
			Role: Role(is), DaysInStatus: days(in.Now, lastEnter(is, is.Status)), DaysSinceStart: sinceStart(in.Now, is),
			TotalDaysInStatus: totalIn(is, is.Status, in.Now), Reworks: countTo(is, stRework), Sprints: len(is.Sprints), Summary: is.Summary})
	}
	return out
}

func sinceStart(now time.Time, is *Issue) *int {
	if t := firstEnter(is, stWork); t != nil {
		d := days(now, *t)
		return &d
	}
	return nil
}

// ---------- 5. Долго на тесте ----------

type TestingRow struct {
	rowBase
	Key            string `json:"key"`
	Assignee       string `json:"assignee"`
	Role           string `json:"role"`
	DaysInTest     int    `json:"days_in_test"`
	Tester         string `json:"tester"`
	DaysSinceStart *int   `json:"days_since_start"`
	Sprints        int    `json:"sprints"`
	IsBug          string `json:"is_bug"`
	Summary        string `json:"summary"`
}

// Testing — задачи в статусе Test (порог 3 дн. применяет страница и счётчик).
func Testing(in *Input) []TestingRow {
	out := []TestingRow{}
	for _, is := range in.sprintIssues() {
		if is.Status != stTest {
			continue
		}
		out = append(out, TestingRow{rowBase: rowBase{in.Lead, is.Project}, Key: is.Key, Assignee: is.Assignee, Role: Role(is),
			DaysInTest: days(in.Now, lastEnter(is, stTest)), Tester: is.Tester, DaysSinceStart: sinceStart(in.Now, is),
			Sprints: len(is.Sprints), IsBug: yesNo(is.Type == "Ошибка"), Summary: is.Summary})
	}
	return out
}
