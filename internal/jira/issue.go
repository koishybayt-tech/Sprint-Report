package jira

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type User struct {
	Name         string `json:"name"` // логин
	DisplayName  string `json:"displayName"`
	EmailAddress string `json:"emailAddress"`
	Active       bool   `json:"active"`
}

// Sprint — разобранное значение поля «Спринт» (customfield_10104).
type Sprint struct {
	ID    int64
	State string // ACTIVE | FUTURE | CLOSED
	Name  string
	Start *time.Time
	End   *time.Time
	Done  *time.Time // completeDate
}

// Issue — задача с выделенными полями; Raw — JSON как пришёл из Jira (храним всегда).
type Issue struct {
	Key         string
	Raw         json.RawMessage
	ProjectKey  string
	ProjectName string
	Summary     string
	Type        string
	StatusID    string
	StatusName  string
	Assignee    *User
	Reporter    *User
	Tester      *User  // customfield_10902
	Plan        string // customfield_11204: Плановая | Внеплановая
	Position    string // customfield_11101
	EpicKey     string // customfield_10100
	Priority    string
	Created     time.Time
	Updated     time.Time
	Resolved    *time.Time
	TimespentS  int
	Sprints     []Sprint // в порядке, в котором их отдаёт Jira
	// История статуса и исполнителя — только если задача запрошена с expand=changelog (синк истории).
	Changes        []Change
	ChangesPartial bool // Jira отдала не всю историю — догрузить отдельно
}

// Change — переход из истории задачи. Статус — названия (как в reference/employee/build_emp.py),
// исполнитель — логины.
type Change struct {
	At    time.Time
	Field string // status | assignee
	From  string
	To    string
}

type rawChangelog struct {
	Total     int `json:"total"`
	Histories []struct {
		Created string `json:"created"`
		Items   []struct {
			Field      string  `json:"field"`
			FieldType  string  `json:"fieldtype"`
			From       *string `json:"from"`
			FromString *string `json:"fromString"`
			To         *string `json:"to"`
			ToString   *string `json:"toString"`
		} `json:"items"`
	} `json:"histories"`
}

func parseChanges(cl *rawChangelog) ([]Change, error) {
	var out []Change
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	for _, h := range cl.Histories {
		at, err := ParseTime(h.Created)
		if err != nil {
			return nil, err
		}
		for _, it := range h.Items {
			if it.FieldType != "jira" {
				continue
			}
			switch it.Field {
			case "status":
				out = append(out, Change{At: at, Field: "status", From: str(it.FromString), To: str(it.ToString)})
			case "assignee":
				out = append(out, Change{At: at, Field: "assignee", From: str(it.From), To: str(it.To)})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

type rawIssue struct {
	Key       string        `json:"key"`
	Changelog *rawChangelog `json:"changelog"`
	Fields    struct {
		Summary   string `json:"summary"`
		IssueType struct {
			Name string `json:"name"`
		} `json:"issuetype"`
		Status struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"status"`
		Project struct {
			Key  string `json:"key"`
			Name string `json:"name"`
		} `json:"project"`
		Priority *struct {
			Name string `json:"name"`
		} `json:"priority"`
		Assignee       *User             `json:"assignee"`
		Reporter       *User             `json:"reporter"`
		Created        string            `json:"created"`
		Updated        string            `json:"updated"`
		ResolutionDate string            `json:"resolutiondate"`
		Timespent      *int              `json:"timespent"`
		Sprints        []json.RawMessage `json:"customfield_10104"`
		Plan           json.RawMessage   `json:"customfield_11204"`
		Tester         *User             `json:"customfield_10902"`
		Position       json.RawMessage   `json:"customfield_11101"`
		Epic           *string           `json:"customfield_10100"`
	} `json:"fields"`
}

func parseIssue(raw json.RawMessage) (Issue, error) {
	var r rawIssue
	if err := json.Unmarshal(raw, &r); err != nil {
		return Issue{}, fmt.Errorf("разбор задачи: %w", err)
	}
	if r.Changelog != nil {
		// История лежит отдельно (jira_changelog), в raw её не храним — она в разы больше самой задачи.
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			return Issue{}, err
		}
		delete(m, "changelog")
		if b, err := json.Marshal(m); err == nil {
			raw = b
		}
	}
	f := r.Fields
	is := Issue{
		Key: r.Key, Raw: raw,
		ProjectKey: f.Project.Key, ProjectName: f.Project.Name,
		Summary: f.Summary, Type: f.IssueType.Name,
		StatusID: f.Status.ID, StatusName: f.Status.Name,
		Assignee: f.Assignee, Reporter: f.Reporter, Tester: f.Tester,
		Plan: optionValue(f.Plan), Position: optionValue(f.Position),
	}
	if f.Priority != nil {
		is.Priority = f.Priority.Name
	}
	if f.Epic != nil {
		is.EpicKey = *f.Epic
	}
	if f.Timespent != nil {
		is.TimespentS = *f.Timespent
	}
	// created/updated приходят, только если запрошены в fields (для списка тимлидов — нет).
	var err error
	if f.Created != "" {
		if is.Created, err = ParseTime(f.Created); err != nil {
			return is, fmt.Errorf("%s created: %w", r.Key, err)
		}
	}
	if f.Updated != "" {
		if is.Updated, err = ParseTime(f.Updated); err != nil {
			return is, fmt.Errorf("%s updated: %w", r.Key, err)
		}
	}
	if f.ResolutionDate != "" {
		t, err := ParseTime(f.ResolutionDate)
		if err != nil {
			return is, fmt.Errorf("%s resolutiondate: %w", r.Key, err)
		}
		is.Resolved = &t
	}
	if r.Changelog != nil {
		ch, err := parseChanges(r.Changelog)
		if err != nil {
			return is, fmt.Errorf("%s changelog: %w", r.Key, err)
		}
		is.Changes, is.ChangesPartial = ch, r.Changelog.Total > len(r.Changelog.Histories)
	}
	for _, s := range f.Sprints {
		sp, err := parseSprintValue(s)
		if err != nil {
			return is, fmt.Errorf("%s sprint: %w", r.Key, err)
		}
		is.Sprints = append(is.Sprints, sp)
	}
	return is, nil
}

// optionValue: поле-список приходит объектом {"value": "..."}, текстовое — строкой.
func optionValue(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var o struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(raw, &o) == nil {
		return o.Value
	}
	return ""
}

// ParseTime понимает форматы Jira: 2026-10-20T09:00:00.000+0500 и RFC 3339 (+05:00).
func ParseTime(s string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02T15:04:05.000-0700", time.RFC3339Nano, "2006-01-02T15:04:05-0700"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("неизвестный формат времени %q", s)
}

// Ключи внутри строки спринта Jira Server: ...Sprint@1a2b[id=1,rapidViewId=2,state=ACTIVE,name=...,startDate=...].
var sprintKeys = []string{"id", "rapidViewId", "state", "name", "startDate", "endDate", "completeDate",
	"activatedDate", "sequence", "goal", "autoStartStop", "synced", "incompleteIssuesDestinationId"}

// parseSprintValue разбирает значение поля «Спринт»: строку greenhopper (Jira 8) или объект (новые версии).
func parseSprintValue(raw json.RawMessage) (Sprint, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		var o AgileSprint
		if err := json.Unmarshal(raw, &o); err != nil {
			return Sprint{}, err
		}
		return Sprint{ID: o.ID, State: strings.ToUpper(o.State), Name: o.Name,
			Start: optTime(o.StartDate), End: optTime(o.EndDate)}, nil
	}
	m := parseSprintString(s)
	id, err := strconv.ParseInt(m["id"], 10, 64)
	if err != nil {
		return Sprint{}, fmt.Errorf("нет id в %q", s)
	}
	return Sprint{ID: id, State: m["state"], Name: m["name"],
		Start: optTime(m["startDate"]), End: optTime(m["endDate"]), Done: optTime(m["completeDate"])}, nil
}

// parseSprintString режет содержимое [...] по известным ключам, поэтому запятые в name/goal не ломают разбор.
func parseSprintString(s string) map[string]string {
	if i := strings.Index(s, "["); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSuffix(s, "]")
	type pos struct {
		key        string
		start, val int
	}
	var ps []pos
	for _, k := range sprintKeys {
		if strings.HasPrefix(s, k+"=") {
			ps = append(ps, pos{k, 0, len(k) + 1})
		}
		if i := strings.Index(s, ","+k+"="); i >= 0 {
			ps = append(ps, pos{k, i, i + len(k) + 2})
		}
	}
	sort.Slice(ps, func(a, b int) bool { return ps[a].start < ps[b].start })
	m := make(map[string]string, len(ps))
	for i, p := range ps {
		end := len(s)
		if i+1 < len(ps) {
			end = ps[i+1].start
		}
		v := s[p.val:end]
		if v != "<null>" {
			m[p.key] = v
		}
	}
	return m
}

func optTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := ParseTime(s)
	if err != nil {
		return nil
	}
	return &t
}
