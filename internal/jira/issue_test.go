package jira

import (
	"encoding/json"
	"testing"
)

func TestParseSprintValue(t *testing.T) {
	raw := `"com.atlassian.greenhopper.service.sprint.Sprint@6f1d[id=1234,rapidViewId=56,state=ACTIVE,name=Adata, 2026-19,startDate=2026-10-26T09:00:00.000+05:00,endDate=2026-11-06T18:00:00.000+05:00,completeDate=<null>,activatedDate=2026-10-26T09:05:00.000+05:00,sequence=1234,goal=релиз, и ещё,autoStartStop=false,synced=false]"`
	s, err := parseSprintValue(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != 1234 || s.State != "ACTIVE" || s.Name != "Adata, 2026-19" || s.Start == nil || s.End == nil || s.Done != nil {
		t.Fatalf("разобрали %+v", s)
	}
	if got := s.Start.Format("2006-01-02 15:04"); got != "2026-10-26 09:00" {
		t.Errorf("start = %s", got)
	}

	obj := `{"id":7,"state":"future","name":"X 8","startDate":"2026-11-09T09:00:00.000+05:00"}`
	s, err = parseSprintValue(json.RawMessage(obj))
	if err != nil || s.ID != 7 || s.State != "FUTURE" || s.Start == nil {
		t.Fatalf("объект: %+v %v", s, err)
	}
}

func TestParseIssue(t *testing.T) {
	raw := `{"key":"ABC-1","fields":{"summary":"S","issuetype":{"name":"Ошибка"},"status":{"id":"10011","name":"В работе"},
	"project":{"key":"ABC","name":"Abc"},"priority":{"name":"High"},"assignee":{"name":"ivanov","displayName":"Иванов"},
	"created":"2026-10-01T10:00:00.000+0500","updated":"2026-10-02T10:00:00.000+0500","resolutiondate":null,"timespent":3600,
	"customfield_10104":null,"customfield_11204":{"value":"Внеплановая"},"customfield_11101":"Backend","customfield_10100":"ABC-0"}}`
	is, err := parseIssue(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	if is.Plan != "Внеплановая" || is.Position != "Backend" || is.EpicKey != "ABC-0" || is.Assignee.Name != "ivanov" ||
		is.TimespentS != 3600 || is.Resolved != nil || len(is.Sprints) != 0 || is.Created.Hour() != 10 {
		t.Fatalf("разобрали %+v", is)
	}
}
