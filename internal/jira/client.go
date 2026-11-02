// Package jira — клиент Jira Server/DC (REST API v2 + Agile 1.0) по Personal Access Token.
package jira

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Поля задачи, которые забираем (как FIELDS в reference/sprints/inc.js + дата решения, эпик, автор).
const IssueFields = "summary,status,assignee,reporter,issuetype,project,priority,created,updated,resolutiondate,timespent," +
	"customfield_10104,customfield_11204,customfield_10902,customfield_11101,customfield_10100"

type Client struct {
	base string
	pat  string
	http *http.Client
}

func New(baseURL, pat string) (*Client, error) {
	if pat == "" {
		return nil, errors.New("JIRA_PAT не задан")
	}
	return &Client{base: strings.TrimRight(baseURL, "/"), pat: pat, http: &http.Client{Timeout: 60 * time.Second}}, nil
}

// get выполняет GET и декодирует JSON. Токен в ошибки не попадает.
func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.pat)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("jira %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("jira %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Myself проверяет токен.
func (c *Client) Myself(ctx context.Context) (User, error) {
	var u User
	err := c.get(ctx, "/rest/api/2/myself", nil, &u)
	return u, err
}

// Search забирает все задачи по JQL пачками (startAt), как search() в inc.js.
func (c *Client) Search(ctx context.Context, jql, fields string) ([]Issue, error) {
	var all []Issue
	err := c.SearchEach(ctx, jql, fields, "", func(page []Issue) error {
		all = append(all, page...)
		return nil
	})
	return all, err
}

// SearchEach отдаёт задачи по страницам, не держа в памяти всю выборку (история за год с changelog — сотни МБ).
// JQL должен сортировать стабильно (ORDER BY key), иначе startAt пропустит задачи.
func (c *Client) SearchEach(ctx context.Context, jql, fields, expand string, fn func([]Issue) error) error {
	for start := 0; ; {
		var page struct {
			Total  int               `json:"total"`
			Issues []json.RawMessage `json:"issues"`
		}
		q := url.Values{"jql": {jql}, "startAt": {strconv.Itoa(start)}, "maxResults": {"1000"}, "fields": {fields}}
		if expand != "" {
			// С историей 1000 задач — ~16 МБ и почти минута ответа; пачки поменьше укладываются в таймаут.
			q.Set("expand", expand)
			q.Set("maxResults", "250")
		}
		if err := c.get(ctx, "/rest/api/2/search", q, &page); err != nil {
			return err
		}
		issues := make([]Issue, 0, len(page.Issues))
		for _, raw := range page.Issues {
			is, err := parseIssue(raw)
			if err != nil {
				return err
			}
			issues = append(issues, is)
		}
		if len(issues) > 0 {
			if err := fn(issues); err != nil {
				return err
			}
		}
		start += len(page.Issues)
		if len(page.Issues) == 0 || start >= page.Total {
			return nil
		}
	}
}

// IssueChanges — полная история задачи, когда поиск отдал её обрезанной.
func (c *Client) IssueChanges(ctx context.Context, key string) ([]Change, error) {
	var r struct {
		Changelog rawChangelog `json:"changelog"`
	}
	if err := c.get(ctx, "/rest/api/2/issue/"+url.PathEscape(key), url.Values{"expand": {"changelog"}, "fields": {"status"}}, &r); err != nil {
		return nil, err
	}
	return parseChanges(&r.Changelog)
}

type Project struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Lead *User  `json:"lead"`
}

func (c *Client) Projects(ctx context.Context) ([]Project, error) {
	var ps []Project
	err := c.get(ctx, "/rest/api/2/project", url.Values{"expand": {"lead"}}, &ps)
	return ps, err
}

type Board struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

func (c *Client) Boards(ctx context.Context, projectKey string) ([]Board, error) {
	var r struct {
		Values []Board `json:"values"`
	}
	err := c.get(ctx, "/rest/agile/1.0/board", url.Values{"projectKeyOrId": {projectKey}}, &r)
	return r.Values, err
}

type AgileSprint struct {
	ID        int64  `json:"id"`
	State     string `json:"state"`
	Name      string `json:"name"`
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}

func (c *Client) FutureSprints(ctx context.Context, boardID int64) ([]AgileSprint, error) {
	var r struct {
		Values []AgileSprint `json:"values"`
	}
	err := c.get(ctx, fmt.Sprintf("/rest/agile/1.0/board/%d/sprint", boardID),
		url.Values{"state": {"future"}, "maxResults": {"50"}}, &r)
	return r.Values, err
}

// UserByLogin — пользователь Jira по логину (нужен email: из него берётся табельный ID).
func (c *Client) UserByLogin(ctx context.Context, login string) (User, error) {
	var u User
	err := c.get(ctx, "/rest/api/2/user", url.Values{"username": {login}}, &u)
	return u, err
}
