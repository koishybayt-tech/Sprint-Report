package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adata/reports/internal/metrics"
)

// JQL задач активных спринтов (openSprints() в Jira — только начатые и не закрытые).
// `sprint in openSprints()` на нашей Jira даёт 400 — только cf[10104].
const OpenSprintsJQL = "cf[10104] in openSprints()"

// JQL задач будущих спринтов — для числа задач в «следующем спринте» (ns_fetch.js считает cf[10104] = id).
const FutureSprintsJQL = "cf[10104] in futureSprints()"

// JQL тимлидов (TL в inc.js): исполнители из группы adata-team-leads с задачами за 180 дней.
const TeamLeadsJQL = `assignee in membersOf("adata-team-leads") AND updated >= -180d`

// Доска 140 пропускается при поиске следующего спринта (как в reference/sprints/ns_fetch.js).
const skipBoardID = 140

type SyncStats struct {
	Issues        int `json:"issues"`
	FutureIssues  int `json:"future_issues"`
	Projects      int `json:"projects"`
	Sprints       int `json:"sprints"`
	FutureSprints int `json:"future_sprints"`
	BoardErrors   int `json:"board_errors"`
	TeamLeads     int `json:"team_leads"`
	NewEmployees  int `json:"new_employees"` // добавлено в справочник из Jira
}

// FullSync забирает задачи активных и будущих спринтов, проекты с лидами и будущие спринты досок
// и пишет их в БД. Каждый запуск фиксируется в sync_runs.
func FullSync(ctx context.Context, pool *pgxpool.Pool, c *Client, log *slog.Logger) (SyncStats, error) {
	var st SyncStats
	var runID int64
	if err := pool.QueryRow(ctx, `INSERT INTO sync_runs (source, mode) VALUES ('jira', 'full') RETURNING id`).Scan(&runID); err != nil {
		return st, err
	}
	err := fullSync(ctx, pool, c, log, &st)
	stats, _ := json.Marshal(st)
	var errText *string
	if err != nil {
		s := err.Error()
		errText = &s
	}
	if _, e := pool.Exec(context.WithoutCancel(ctx),
		`UPDATE sync_runs SET finished_at = now(), ok = $2, stats = $3, error = $4 WHERE id = $1`,
		runID, err == nil, stats, errText); e != nil && err == nil {
		err = e
	}
	return st, err
}

func fullSync(ctx context.Context, pool *pgxpool.Pool, c *Client, log *slog.Logger, st *SyncStats) error {
	t0 := time.Now()
	// Задачи открытых спринтов — с историей статуса и исполнителя (нужна «Контролю спринта руководителей»).
	issues, err := c.SearchChanges(ctx, OpenSprintsJQL+" ORDER BY created ASC", IssueFields)
	if err != nil {
		return err
	}
	log.Info("jira: задачи активных спринтов", "count", len(issues), "took", time.Since(t0).Round(time.Millisecond).String())
	t0 = time.Now()
	futureIssues, err := c.Search(ctx, FutureSprintsJQL+" ORDER BY created ASC", IssueFields)
	if err != nil {
		return err
	}
	log.Info("jira: задачи будущих спринтов", "count", len(futureIssues), "took", time.Since(t0).Round(time.Millisecond).String())
	projects, err := c.Projects(ctx)
	if err != nil {
		return err
	}
	// Тимлиды. Если группы нет или JQL упал — оставляем прежний список, синк не валим.
	var teamLeads []string
	tlIssues, tlErr := c.Search(ctx, TeamLeadsJQL, "assignee")
	if tlErr != nil {
		log.Warn("jira: тимлиды", "err", tlErr)
	} else {
		seen := map[string]bool{}
		for _, is := range tlIssues {
			if is.Assignee == nil {
				continue
			}
			if n := strings.TrimSpace(is.Assignee.DisplayName); n != "" && !seen[n] {
				seen[n] = true
				teamLeads = append(teamLeads, n)
			}
		}
	}

	inUse := map[string]bool{}
	for _, is := range issues {
		inUse[is.ProjectKey] = true
	}
	futures := loadFutures(ctx, c, log, inUse, st)

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, p := range projects {
		lead := "—"
		if p.Lead != nil && p.Lead.DisplayName != "" {
			lead = p.Lead.DisplayName
		}
		if _, err := tx.Exec(ctx, `INSERT INTO jira_projects (key, name, lead) VALUES ($1, $2, $3)
			ON CONFLICT (key) DO UPDATE SET name = EXCLUDED.name, lead = EXCLUDED.lead`, p.Key, p.Name, lead); err != nil {
			return fmt.Errorf("проект %s: %w", p.Key, err)
		}
	}
	st.Projects = len(projects)

	if st.Sprints, err = writeSprints(ctx, tx, append(issues, futureIssues...), futures); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `UPDATE jira_issues SET in_open_sprints = false, in_future_sprints = false
		WHERE in_open_sprints OR in_future_sprints`); err != nil {
		return err
	}
	if err := upsertIssues(ctx, tx, issues, "in_open_sprints"); err != nil {
		return err
	}
	if err := upsertIssues(ctx, tx, futureIssues, "in_future_sprints"); err != nil {
		return err
	}
	if err := writeChanges(ctx, tx, issues); err != nil {
		return err
	}
	st.Issues, st.FutureIssues = len(issues), len(futureIssues)

	if err := writeFutures(ctx, tx, futures); err != nil {
		return err
	}
	st.FutureSprints = len(futures)

	n, err := upsertPeople(ctx, tx, append(issues, futureIssues...))
	if err != nil {
		return fmt.Errorf("справочник сотрудников: %w", err)
	}
	st.NewEmployees = n

	if tlErr == nil {
		if _, err := tx.Exec(ctx, `DELETE FROM jira_team_leads`); err != nil {
			return err
		}
		for _, n := range teamLeads {
			if _, err := tx.Exec(ctx, `INSERT INTO jira_team_leads (name) VALUES ($1)`, n); err != nil {
				return err
			}
		}
		st.TeamLeads = len(teamLeads)
	}
	return tx.Commit(ctx)
}

// upsertIssues пишет задачи и выставляет флаг выборки (flag — имя колонки in_open_sprints | in_future_sprints).
// Пустой flag — флаги не трогаем (инкрементальная синхронизация выставляет их отдельно по составу спринтов).
func upsertIssues(ctx context.Context, tx pgx.Tx, issues []Issue, flag string) error {
	flagCol, flagVal, flagSet := "", "", ""
	if flag != "" {
		flagCol, flagVal, flagSet = ", "+flag, ", true", ", "+flag+" = true"
	}
	q := `INSERT INTO jira_issues (key, project_key, summary, type, status_id, status_name, status_group,
			assignee, assignee_login, reporter, tester, plan, position, epic_key, priority,
			created, updated, resolved, timespent_s, sprint_ids, raw, synced_at` + flagCol + `)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21, now()` + flagVal + `)
		ON CONFLICT (key) DO UPDATE SET project_key = EXCLUDED.project_key, summary = EXCLUDED.summary,
		  type = EXCLUDED.type, status_id = EXCLUDED.status_id, status_name = EXCLUDED.status_name,
		  status_group = EXCLUDED.status_group, assignee = EXCLUDED.assignee, assignee_login = EXCLUDED.assignee_login,
		  reporter = EXCLUDED.reporter, tester = EXCLUDED.tester, plan = EXCLUDED.plan, position = EXCLUDED.position,
		  epic_key = EXCLUDED.epic_key, priority = EXCLUDED.priority, created = EXCLUDED.created,
		  updated = EXCLUDED.updated, resolved = EXCLUDED.resolved, timespent_s = EXCLUDED.timespent_s,
		  sprint_ids = EXCLUDED.sprint_ids, raw = EXCLUDED.raw, synced_at = now()` + flagSet
	batch := &pgx.Batch{}
	for _, is := range issues {
		ids := make([]int64, len(is.Sprints))
		for i, s := range is.Sprints {
			ids[i] = s.ID
		}
		batch.Queue(q,
			is.Key, is.ProjectKey, is.Summary, is.Type, is.StatusID, is.StatusName, metrics.StatusGroup(is.StatusID),
			displayName(is.Assignee), login(is.Assignee), displayName(is.Reporter), displayName(is.Tester),
			nullStr(is.Plan), nullStr(is.Position), nullStr(is.EpicKey), nullStr(is.Priority),
			is.Created, is.Updated, is.Resolved, is.TimespentS, ids, is.Raw)
	}
	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("запись задач (%s): %w", flag, err)
	}
	return writeLinks(ctx, tx, issues)
}

// writeLinks заменяет связи задач (issuelinks).
func writeLinks(ctx context.Context, tx pgx.Tx, issues []Issue) error {
	if _, err := tx.Exec(ctx, `DELETE FROM jira_issue_links WHERE issue_key = ANY($1)`, keysOf(issues)); err != nil {
		return err
	}
	b := &pgx.Batch{}
	for _, is := range issues {
		for _, l := range is.Links {
			b.Queue(`INSERT INTO jira_issue_links (issue_key, link_type, direction, phrase, linked_key) VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT DO NOTHING`, is.Key, l.Type, l.Direction, l.Phrase, l.Key)
		}
	}
	if err := tx.SendBatch(ctx, b).Close(); err != nil {
		return fmt.Errorf("запись связей задач: %w", err)
	}
	return nil
}

func displayName(u *User) *string {
	if u == nil {
		return nil
	}
	return &u.DisplayName
}

func login(u *User) *string {
	if u == nil {
		return nil
	}
	return &u.Name
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// upsertPeople пополняет справочник сотрудников всеми, кто встречается в задачах (исполнители и тестировщики),
// чтобы их можно было выбрать при привязке Telegram. Ручные данные из seed не перетираются:
//   - сотрудник с этим логином есть — обновляется только jira_active;
//   - есть сотрудник с тем же ФИО без логина — ему проставляется логин;
//   - иначе добавляется новый.
//
// Возвращает число добавленных.
func upsertPeople(ctx context.Context, tx pgx.Tx, issues []Issue) (int, error) {
	people := map[string]User{}
	for _, is := range issues {
		for _, u := range []*User{is.Assignee, is.Tester} {
			if u != nil && u.Name != "" && strings.TrimSpace(u.DisplayName) != "" {
				people[u.Name] = *u
			}
		}
	}
	added := 0
	for login, u := range people {
		name := strings.TrimSpace(u.DisplayName)
		tag, err := tx.Exec(ctx, `UPDATE employees SET jira_active = $2, updated_at = now() WHERE jira_login = $1 AND jira_active <> $2`, login, u.Active)
		if err != nil {
			return added, err
		}
		if tag.RowsAffected() > 0 {
			continue
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM employees WHERE jira_login = $1)`, login).Scan(&exists); err != nil {
			return added, err
		}
		if exists {
			continue
		}
		tag, err = tx.Exec(ctx, `UPDATE employees SET jira_login = $1, jira_active = $3, updated_at = now() WHERE full_name = $2 AND jira_login IS NULL`, login, name, u.Active)
		if err != nil {
			return added, err
		}
		if tag.RowsAffected() > 0 {
			continue
		}
		tag, err = tx.Exec(ctx, `INSERT INTO employees (full_name, jira_login, jira_active, staff) VALUES ($1, $2, $3, false)
			ON CONFLICT (full_name) DO NOTHING`, name, login, u.Active)
		if err != nil {
			return added, err
		}
		added += int(tag.RowsAffected())
	}
	return added, nil
}

type future struct {
	project string
	board   int64
	sp      AgileSprint
}

// loadFutures — будущие спринты scrum-досок проектов (кандидаты в «следующий спринт»).
// Ошибка по одному проекту не валит синк.
func loadFutures(ctx context.Context, c *Client, log *slog.Logger, projects map[string]bool, st *SyncStats) []future {
	var futures []future
	for key := range projects {
		boards, err := c.Boards(ctx, key)
		if err != nil {
			log.Warn("jira: доски проекта", "project", key, "err", err)
			st.BoardErrors++
			continue
		}
		for _, b := range boards {
			if b.Type != "scrum" || b.ID == skipBoardID {
				continue
			}
			sps, err := c.FutureSprints(ctx, b.ID)
			if err != nil {
				log.Warn("jira: будущие спринты доски", "project", key, "board", b.ID, "err", err)
				st.BoardErrors++
				continue
			}
			for _, s := range sps {
				futures = append(futures, future{key, b.ID, s})
			}
		}
	}
	return futures
}

// writeSprints сохраняет спринты из поля «Спринт» задач и будущие спринты досок; возвращает их число.
func writeSprints(ctx context.Context, tx pgx.Tx, issues []Issue, futures []future) (int, error) {
	sprints := map[int64]Sprint{}
	for _, is := range issues {
		for _, s := range is.Sprints {
			sprints[s.ID] = s
		}
	}
	for _, f := range futures {
		if _, ok := sprints[f.sp.ID]; !ok {
			sprints[f.sp.ID] = Sprint{ID: f.sp.ID, State: "FUTURE", Name: f.sp.Name,
				Start: optTime(f.sp.StartDate), End: optTime(f.sp.EndDate)}
		}
	}
	for _, s := range sprints {
		if _, err := tx.Exec(ctx, `INSERT INTO jira_sprints (id, name, state, start_date, end_date, complete_date)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, state = EXCLUDED.state,
			  start_date = EXCLUDED.start_date, end_date = EXCLUDED.end_date, complete_date = EXCLUDED.complete_date`,
			s.ID, s.Name, s.State, s.Start, s.End, s.Done); err != nil {
			return 0, fmt.Errorf("спринт %d: %w", s.ID, err)
		}
	}
	return len(sprints), nil
}

// writeFutures заменяет связи «проект → будущий спринт доски».
func writeFutures(ctx context.Context, tx pgx.Tx, futures []future) error {
	if _, err := tx.Exec(ctx, `DELETE FROM jira_project_future_sprints`); err != nil {
		return err
	}
	for _, f := range futures {
		if _, err := tx.Exec(ctx, `INSERT INTO jira_project_future_sprints (project_key, board_id, sprint_id)
			VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, f.project, f.board, f.sp.ID); err != nil {
			return err
		}
	}
	return nil
}
