package db

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adata/reports/internal/leadcontrol"
)

// LeadControlLeads — руководители из lead_projects с проектами в порядке чипов.
func LeadControlLeads(ctx context.Context, pool *pgxpool.Pool) ([]string, map[string][]leadcontrol.LeadProject, error) {
	rows, err := ListLeadProjects(ctx, pool)
	if err != nil {
		return nil, nil, err
	}
	var leads []string
	by := map[string][]leadcontrol.LeadProject{}
	for _, r := range rows {
		if _, ok := by[r.Lead]; !ok {
			leads = append(leads, r.Lead)
		}
		title := r.Title
		if title == "" {
			title = r.ProjectKey
		}
		by[r.Lead] = append(by[r.Lead], leadcontrol.LeadProject{Key: r.ProjectKey, Title: title, Assigned: r.UserID != nil})
	}
	return leads, by, nil
}

// leadControlData — общее для всех руководителей: задачи года и открытых/будущих спринтов с историей и связями.
type leadControlData struct {
	issues  map[string]*leadcontrol.Issue
	names   map[string]string
	roles   map[string]string
	leadOf  map[string]string
	ignored map[string]map[string]leadcontrol.Ignore // руководитель → скрытые из «Разрывов»
}

func loadLeadControlData(ctx context.Context, pool *pgxpool.Pool, leads map[string][]leadcontrol.LeadProject) (*leadControlData, error) {
	d := &leadControlData{issues: map[string]*leadcontrol.Issue{}, roles: map[string]string{}, leadOf: map[string]string{},
		ignored: map[string]map[string]leadcontrol.Ignore{}}
	var err error
	if d.names, err = JiraNames(ctx, pool); err != nil {
		return nil, err
	}

	sprints := map[int64]leadcontrol.Sprint{}
	srows, err := pool.Query(ctx, `SELECT id, name, state FROM jira_sprints`)
	if err != nil {
		return nil, err
	}
	for srows.Next() {
		var id int64
		var s leadcontrol.Sprint
		if err := srows.Scan(&id, &s.Name, &s.State); err != nil {
			srows.Close()
			return nil, err
		}
		sprints[id] = s
	}
	srows.Close()
	if err := srows.Err(); err != nil {
		return nil, err
	}

	const scope = `(updated >= date_trunc('year', now() AT TIME ZONE 'Asia/Almaty') AT TIME ZONE 'Asia/Almaty' OR in_open_sprints OR in_future_sprints)`
	rows, err := pool.Query(ctx, `SELECT key, project_key, coalesce(summary, ''), coalesce(type, ''), coalesce(status_name, ''),
			coalesce(assignee, ''), coalesce(tester, ''), coalesce(position, ''), created, sprint_ids, in_open_sprints, in_future_sprints,
			coalesce(raw->'fields'->'project'->>'id', ''), coalesce(raw->'fields'->'issuetype'->>'id', '')
		FROM jira_issues WHERE `+scope)
	if err != nil {
		return nil, err
	}
	posCount := map[string]map[string]int{}
	for rows.Next() {
		is := &leadcontrol.Issue{}
		var ids []int64
		if err := rows.Scan(&is.Key, &is.Project, &is.Summary, &is.Type, &is.Status, &is.Assignee, &is.Tester, &is.Position,
			&is.Created, &ids, &is.InOpenSprint, &is.InFutureSprint, &is.ProjectID, &is.TypeID); err != nil {
			rows.Close()
			return nil, err
		}
		for _, id := range ids {
			if s, ok := sprints[id]; ok {
				is.Sprints = append(is.Sprints, s)
			}
		}
		d.issues[is.Key] = is
		if is.Assignee != "" && is.Position != "" {
			if posCount[is.Assignee] == nil {
				posCount[is.Assignee] = map[string]int{}
			}
			posCount[is.Assignee][is.Position]++
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Должность человека — самая частая «Позиция разработчика» в его задачах, иначе из справочника.
	for n, m := range posCount {
		best := ""
		for p, c := range m {
			if best == "" || c > m[best] || (c == m[best] && p < best) {
				best = p
			}
		}
		d.roles[n] = best
	}
	erows, err := pool.Query(ctx, `SELECT full_name, coalesce(position, '') FROM employees WHERE position IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	for erows.Next() {
		var n, p string
		if err := erows.Scan(&n, &p); err != nil {
			erows.Close()
			return nil, err
		}
		if d.roles[n] == "" {
			d.roles[n] = p
		}
	}
	erows.Close()

	crows, err := pool.Query(ctx, `SELECT c.issue_key, c.at, c.field, coalesce(c.from_val, ''), coalesce(c.to_val, ''), coalesce(c.author, '')
		FROM jira_changelog c JOIN jira_issues i ON i.key = c.issue_key WHERE i.`+scope[1:len(scope)-1]+` ORDER BY c.at`)
	if err != nil {
		return nil, fmt.Errorf("история задач: %w", err)
	}
	for crows.Next() {
		var key string
		var c leadcontrol.Change
		if err := crows.Scan(&key, &c.At, &c.Field, &c.From, &c.To, &c.Author); err != nil {
			crows.Close()
			return nil, err
		}
		if is := d.issues[key]; is != nil {
			is.Changes = append(is.Changes, leadcontrol.CanonChange(c))
		}
	}
	crows.Close()
	if err := crows.Err(); err != nil {
		return nil, err
	}

	lrows, err := pool.Query(ctx, `SELECT issue_key, link_type, direction, linked_key FROM jira_issue_links ORDER BY issue_key, linked_key`)
	if err != nil {
		return nil, err
	}
	for lrows.Next() {
		var key string
		var l leadcontrol.Link
		if err := lrows.Scan(&key, &l.Type, &l.Direction, &l.Key); err != nil {
			lrows.Close()
			return nil, err
		}
		if is := d.issues[key]; is != nil {
			is.Links = append(is.Links, l)
		}
	}
	lrows.Close()
	if err := lrows.Err(); err != nil {
		return nil, err
	}

	prows, err := pool.Query(ctx, `SELECT key, coalesce(lead, '') FROM jira_projects`)
	if err != nil {
		return nil, err
	}
	for prows.Next() {
		var k, l string
		if err := prows.Scan(&k, &l); err != nil {
			prows.Close()
			return nil, err
		}
		if l != "—" {
			d.leadOf[k] = l
		}
	}
	prows.Close()
	// Проект может быть ещё и назначен пользователю (ПМу): руководителем в подписях остаётся тот, кто записан по Jira.
	names := make([]string, 0, len(leads))
	for l := range leads {
		names = append(names, l)
	}
	sort.Strings(names)
	claimed := map[string]bool{}
	for _, assigned := range []bool{false, true} {
		for _, lead := range names {
			for _, p := range leads[lead] {
				if p.Assigned != assigned || (assigned && claimed[p.Key]) {
					continue
				}
				d.leadOf[p.Key], claimed[p.Key] = lead, true
			}
		}
	}

	irows, err := pool.Query(ctx, `SELECT g.lead, g.issue_key, coalesce(u.full_name, ''), g.at FROM lead_gap_ignored g LEFT JOIN users u ON u.id = g.user_id`)
	if err != nil {
		return nil, err
	}
	for irows.Next() {
		var lead, key string
		var ig leadcontrol.Ignore
		if err := irows.Scan(&lead, &key, &ig.By, &ig.At); err != nil {
			irows.Close()
			return nil, err
		}
		if d.ignored[lead] == nil {
			d.ignored[lead] = map[string]leadcontrol.Ignore{}
		}
		d.ignored[lead][key] = ig
	}
	irows.Close()
	return d, irows.Err()
}

// RefreshLeadControl пересчитывает и сохраняет отчёт по всем руководителям из lead_projects
// (после каждой синхронизации Jira). Возвращает отчёты — по ним ищутся события для уведомлений.
func RefreshLeadControl(ctx context.Context, pool *pgxpool.Pool, now time.Time, jiraBase string) (map[string]leadcontrol.Report, error) {
	leads, by, err := LeadControlLeads(ctx, pool)
	if err != nil || len(leads) == 0 {
		return nil, err
	}
	reps, err := BuildLeadControl(ctx, pool, leads, by, now, jiraBase)
	if err != nil {
		return nil, err
	}
	for _, lead := range leads {
		if err := SaveLeadControl(ctx, pool, lead, reps[lead]); err != nil {
			return nil, err
		}
	}
	return reps, nil
}

// RebuildLead пересчитывает и сохраняет отчёт одного руководителя (после «Игнорировать» / «Вернуть»).
func RebuildLead(ctx context.Context, pool *pgxpool.Pool, lead, jiraBase string) error {
	_, by, err := LeadControlLeads(ctx, pool)
	if err != nil {
		return err
	}
	if _, ok := by[lead]; !ok {
		return nil
	}
	reps, err := BuildLeadControl(ctx, pool, []string{lead}, by, time.Now(), jiraBase)
	if err != nil {
		return err
	}
	return SaveLeadControl(ctx, pool, lead, reps[lead])
}

// BuildLeadControl считает отчёт для указанных руководителей по данным в БД.
func BuildLeadControl(ctx context.Context, pool *pgxpool.Pool, leads []string, by map[string][]leadcontrol.LeadProject, now time.Time, jiraBase string) (map[string]leadcontrol.Report, error) {
	d, err := loadLeadControlData(ctx, pool, by)
	if err != nil {
		return nil, err
	}
	updated, err := LastSync(ctx, pool, "jira")
	if err != nil {
		return nil, err
	}
	at := now
	if updated != nil {
		at = *updated
	}
	out := map[string]leadcontrol.Report{}
	for _, lead := range leads {
		in := &leadcontrol.Input{Now: now, Lead: lead, Projects: by[lead], LeadOf: d.leadOf, Issues: d.issues, Names: d.names, Roles: d.roles,
			Ignored: d.ignored[lead], JiraBase: jiraBase}
		out[lead] = leadcontrol.Build(in, at)
	}
	return out, nil
}
