package jira

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Запас перекрытия окна изменений: правки на границе запусков не теряются (как lastRun − 5 мин в inc.js).
const incOverlap = 5 * time.Minute

// ErrNoBaseline — инкрементальной синхронизации не от чего считать: успешных запусков ещё не было.
var ErrNoBaseline = errors.New("нет прошлой успешной синхронизации — нужна полная")

type IncStats struct {
	Open        int    `json:"open"`    // задач в активных спринтах сейчас
	Future      int    `json:"future"`  // в будущих
	Changed     int    `json:"changed"` // загружено изменённых с прошлого запуска
	Added       int    `json:"added"`   // появились в спринтах без изменения — догружены по ключам
	Removed     int    `json:"removed"` // ушли из открытых и будущих спринтов
	Since       string `json:"since"`   // окно изменений, JQL
	Sprints     int    `json:"sprints"`
	BoardErrors int    `json:"board_errors"`
}

// IncrementalSync загружает целиком только задачи, изменённые или добавленные с прошлого запуска.
//  1. Лёгкий запрос (ключ + «Спринт») по задачам открытых и будущих спринтов: состав спринтов и их состояния.
//     Закрытие спринта или старт будущего не меняют updated у задач — поэтому состав берём отсюда, а не по дате.
//  2. Полностью — задачи с updated за окно от прошлого запуска (с запасом 5 минут).
//  3. Задачи, которых в БД нет (добавлены в спринт, но не попали в окно), — догружаются по ключам.
//  4. Ушедшие из спринтов снимаются с флагов; будущие спринты досок обновляются.
//
// Проекты, тимлиды и справочник сотрудников — только в полной (раз в сутки).
func IncrementalSync(ctx context.Context, pool *pgxpool.Pool, c *Client, log *slog.Logger) (IncStats, error) {
	var st IncStats
	var lastStart *time.Time
	if err := pool.QueryRow(ctx, `SELECT max(started_at) FROM sync_runs WHERE source = 'jira' AND ok`).Scan(&lastStart); err != nil {
		return st, err
	}
	if lastStart == nil {
		return st, ErrNoBaseline
	}
	var runID int64
	if err := pool.QueryRow(ctx, `INSERT INTO sync_runs (source, mode) VALUES ('jira', 'inc') RETURNING id`).Scan(&runID); err != nil {
		return st, err
	}
	err := incSync(ctx, pool, c, log, *lastStart, &st)
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

func incSync(ctx context.Context, pool *pgxpool.Pool, c *Client, log *slog.Logger, lastStart time.Time, st *IncStats) error {
	// 1. Состав спринтов — только ключ и поле «Спринт».
	open, err := c.Search(ctx, OpenSprintsJQL, "customfield_10104")
	if err != nil {
		return err
	}
	future, err := c.Search(ctx, FutureSprintsJQL, "customfield_10104")
	if err != nil {
		return err
	}
	st.Open, st.Future = len(open), len(future)
	openKeys, futureKeys := keysOf(open), keysOf(future)

	// 2. Изменённые за окно. Относительное время в JQL не зависит от часового пояса профиля в Jira.
	mins := int(math.Ceil(time.Since(lastStart.Add(-incOverlap)).Minutes()))
	st.Since = fmt.Sprintf("updated >= -%dm", mins)
	changed, err := c.Search(ctx, fmt.Sprintf("(%s OR %s) AND %s", OpenSprintsJQL, FutureSprintsJQL, st.Since), IssueFields)
	if err != nil {
		return err
	}
	st.Changed = len(changed)

	// 3. В спринтах, но в БД нет — догружаем по ключам (пачками по 100, как в inc.js).
	inChanged := map[string]bool{}
	for _, is := range changed {
		inChanged[is.Key] = true
	}
	all := append(append([]string{}, openKeys...), futureKeys...)
	rows, err := pool.Query(ctx, `SELECT k FROM unnest($1::text[]) k WHERE NOT EXISTS (SELECT 1 FROM jira_issues WHERE key = k)`, all)
	if err != nil {
		return err
	}
	var missing []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			rows.Close()
			return err
		}
		if !inChanged[k] {
			missing = append(missing, k)
		}
	}
	rows.Close()
	for i := 0; i < len(missing); i += 100 {
		part := missing[i:min(i+100, len(missing))]
		extra, err := c.Search(ctx, "key in ("+strings.Join(part, ",")+")", IssueFields)
		if err != nil {
			return err
		}
		changed = append(changed, extra...)
	}
	st.Added = len(missing)

	// Будущие спринты досок — по проектам с задачами в активных спринтах.
	projects := map[string]bool{}
	for _, k := range openKeys {
		if p, _, ok := strings.Cut(k, "-"); ok {
			projects[p] = true
		}
	}
	var fst SyncStats
	futures := loadFutures(ctx, c, log, projects, &fst)
	st.BoardErrors = fst.BoardErrors

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if st.Sprints, err = writeSprints(ctx, tx, append(append(open, future...), changed...), futures); err != nil {
		return err
	}
	if err := upsertIssues(ctx, tx, changed, ""); err != nil {
		return err
	}
	// Флаги — по текущему составу спринтов; ушедшие снимаются.
	var removed int
	if err := tx.QueryRow(ctx, `WITH upd AS (
			UPDATE jira_issues SET in_open_sprints = key = ANY($1), in_future_sprints = key = ANY($2)
			WHERE (in_open_sprints OR in_future_sprints OR key = ANY($1) OR key = ANY($2))
			  AND (in_open_sprints IS DISTINCT FROM (key = ANY($1)) OR in_future_sprints IS DISTINCT FROM (key = ANY($2)))
			RETURNING in_open_sprints OR in_future_sprints AS still)
		SELECT count(*) FILTER (WHERE NOT still) FROM upd`, openKeys, futureKeys).Scan(&removed); err != nil {
		return err
	}
	st.Removed = removed
	if err := writeFutures(ctx, tx, futures); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func keysOf(issues []Issue) []string {
	out := make([]string, len(issues))
	for i, is := range issues {
		out[i] = is.Key
	}
	return out
}
