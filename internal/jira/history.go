package jira

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HistoryFullJQL — задачи для карточки сотрудника: все, что обновлялись с начала года
// («Задачи 2026 — все задачи Jira, обновлявшиеся с 01.01.2026» в reference/employee/page_template.html).
const HistoryFullJQL = "updated >= startOfYear() ORDER BY key ASC"

// Пока полная загрузка истории идёт (≈20 минут на 23 тыс. задач), вторую не запускаем.
const historyRunningFor = 3 * time.Hour

var ErrHistoryRunning = errors.New("загрузка истории Jira уже идёт")

type HistStats struct {
	Issues  int    `json:"issues"`
	Changes int    `json:"changes"`
	Refetch int    `json:"refetch"` // задач с обрезанной историей, догруженных по одной
	Since   string `json:"since,omitempty"`
}

// HistorySync загружает задачи года вместе с историей статуса и исполнителя (jira_issues + jira_changelog).
// full — все задачи с начала года; иначе — изменённые с прошлого успешного запуска (без него — ErrNoBaseline).
// Флаги спринтов не трогает: этим занимаются FullSync/IncrementalSync.
func HistorySync(ctx context.Context, pool *pgxpool.Pool, c *Client, log *slog.Logger, full bool) (HistStats, error) {
	var st HistStats
	var lastOK, running *time.Time
	if err := pool.QueryRow(ctx, `SELECT
			(SELECT max(started_at) FROM sync_runs WHERE source = 'jira_history' AND ok),
			(SELECT max(started_at) FROM sync_runs WHERE source = 'jira_history' AND mode = 'full' AND finished_at IS NULL
			   AND started_at > now() - make_interval(secs => $1))`, historyRunningFor.Seconds()).Scan(&lastOK, &running); err != nil {
		return st, err
	}
	if running != nil {
		return st, ErrHistoryRunning
	}
	jql := HistoryFullJQL
	mode := "full"
	if !full {
		if lastOK == nil {
			return st, ErrNoBaseline
		}
		mins := int(math.Ceil(time.Since(lastOK.Add(-incOverlap)).Minutes()))
		st.Since = fmt.Sprintf("updated >= -%dm", mins)
		jql = st.Since + " ORDER BY key ASC"
		mode = "inc"
	}
	var runID int64
	if err := pool.QueryRow(ctx, `INSERT INTO sync_runs (source, mode) VALUES ('jira_history', $1) RETURNING id`, mode).Scan(&runID); err != nil {
		return st, err
	}
	err := c.SearchEach(ctx, jql, IssueFields, "changelog", func(page []Issue) error {
		for i := range page {
			if !page[i].ChangesPartial {
				continue
			}
			ch, err := c.IssueChanges(ctx, page[i].Key)
			if err != nil {
				return err
			}
			page[i].Changes = ch
			st.Refetch++
		}
		if err := writeHistory(ctx, pool, page); err != nil {
			return err
		}
		st.Issues += len(page)
		for _, is := range page {
			st.Changes += len(is.Changes)
		}
		if full {
			log.Info("sync jira history: пачка", "issues", st.Issues)
		}
		return nil
	})
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

// writeHistory — одна пачка в своей транзакции: задачи, их спринты и история (заменяется целиком).
func writeHistory(ctx context.Context, pool *pgxpool.Pool, issues []Issue) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := writeSprints(ctx, tx, issues, nil); err != nil {
		return err
	}
	if err := upsertIssues(ctx, tx, issues, ""); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM jira_changelog WHERE issue_key = ANY($1)`, keysOf(issues)); err != nil {
		return err
	}
	b := &pgx.Batch{}
	for _, is := range issues {
		for _, ch := range is.Changes {
			b.Queue(`INSERT INTO jira_changelog (issue_key, at, field, from_val, to_val) VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT DO NOTHING`, is.Key, ch.At, ch.Field, nullStr(ch.From), nullStr(ch.To))
		}
	}
	if err := tx.SendBatch(ctx, b).Close(); err != nil {
		return fmt.Errorf("запись истории задач: %w", err)
	}
	return tx.Commit(ctx)
}
