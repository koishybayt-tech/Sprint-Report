package api

import (
	_ "embed"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adata/reports/internal/config"
	"github.com/adata/reports/internal/db"
	"github.com/adata/reports/internal/leadcontrol"
	"github.com/adata/reports/internal/metrics"
)

//go:embed extdocs/openapi.yaml
var extOpenAPI []byte

//go:embed extdocs/swagger.html
var extSwaggerHTML []byte

// extServer — внешний API (/api/ext/v1): только чтение, токен EXT_API_TOKEN. Описание — docs/external-api.md и extdocs/openapi.yaml.
type extServer struct {
	pool     *pgxpool.Pool
	log      *slog.Logger
	jiraBase string
	emp      *employeesCache
}

// mountExt подключает внешний API. Документация (/openapi.yaml, /docs) открыта без токена — в ней нет данных;
// данные — только с токеном.
func mountExt(r chi.Router, pool *pgxpool.Pool, log *slog.Logger, cfg config.Config) {
	s := &extServer{pool: pool, log: log, jiraBase: cfg.JiraBaseURL, emp: &employeesCache{}}
	r.Route("/api/ext/v1", func(r chi.Router) {
		r.Get("/openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
			_, _ = w.Write(extOpenAPI)
		})
		r.Get("/docs", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' https://cdnjs.cloudflare.com; style-src 'self' 'unsafe-inline' https://cdnjs.cloudflare.com; img-src 'self' data: https:; connect-src 'self'")
			_, _ = w.Write(extSwaggerHTML)
		})

		r.Group(func(r chi.Router) {
			r.Use(requireExtToken(cfg.ExtAPIToken))
			r.Get("/ping", s.ping)

			r.Get("/stats/summary", s.statsSummary)
			r.Get("/stats/performance", s.statsPerformance)

			r.Get("/projects", s.projects)
			r.Get("/projects/{key}", s.projectDetail)

			r.Get("/employees", s.employees)
			r.Get("/employees/{id}", s.employeeDetail)

			r.Get("/leads", s.leads)
			r.Get("/leads/{name}", s.leadDetail)

			r.Get("/team-leads", s.teamLeads)

			r.Get("/testers", s.testersList)
			r.Get("/testers/{name}", s.testerDetail)

			r.Get("/attendance", s.attendanceSummary)
			r.Get("/attendance/events", s.attendanceEvents)
			r.Get("/absences", s.absencesList)

			r.Get("/lists/need-tasks", s.listNeedTasks)
			r.Get("/lists/overloaded", s.listOverloaded)
			r.Get("/lists/long-in-work", s.listLongWork)
			r.Get("/lists/long-in-test", s.listLongTest)
			r.Get("/lists/blocked", s.listBlocked)
			r.Get("/lists/bottlenecks", s.listBottlenecks)
			r.Get("/lists/low-sprints", s.listLowSprints)
			r.Get("/lists/risks", s.listRisks)
			r.Get("/lists/creation-spikes", s.listSpikes)

			r.Get("/dashboards/sprints", s.dashboardSprints)
		})
	})
}

// ---------- общее ----------

type extMeta struct {
	DataUpdatedAt *time.Time `json:"data_updated_at"` // когда в последний раз синхронизирована Jira
	GeneratedAt   time.Time  `json:"generated_at"`
	Count         *int       `json:"count,omitempty"` // для списков
}

type extResp struct {
	Meta extMeta `json:"meta"`
	Data any     `json:"data"`
}

func extErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// reply — ответ с метаданными; для списков (срезов) добавляет count.
func reply(w http.ResponseWriter, updated *time.Time, data any, count int) {
	m := extMeta{DataUpdatedAt: updated, GeneratedAt: time.Now().UTC()}
	if count >= 0 {
		m.Count = &count
	}
	writeJSON(w, http.StatusOK, extResp{Meta: m, Data: data})
}

// load — отчёт «Спринты» и контекст; 503, если синхронизации Jira ещё не было.
func (s *extServer) load(w http.ResponseWriter, r *http.Request) (*extCtx, bool) {
	rep, err := loadReport(r, s.pool)
	if err != nil {
		fail(w, s.log, "ext", err)
		return nil, false
	}
	if rep == nil {
		extErr(w, http.StatusServiceUnavailable, "данные ещё не загружены: не было синхронизации Jira")
		return nil, false
	}
	return newExtCtx(rep, time.Now()), true
}

func (c *extCtx) updated() *time.Time { t := c.Rep.UpdatedAt; return &t }

func qInt(r *http.Request, key string, def int) int {
	if v := strings.TrimSpace(r.URL.Query().Get(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return def
}

func qBool(r *http.Request, key string) bool {
	switch strings.ToLower(r.URL.Query().Get(key)) {
	case "1", "true", "yes", "y":
		return true
	}
	return false
}

func qHas(r *http.Request, key, want string) bool {
	for _, p := range strings.Split(r.URL.Query().Get(key), ",") {
		if strings.EqualFold(strings.TrimSpace(p), want) {
			return true
		}
	}
	return false
}

// qDate — дата YYYY-MM-DD из query; пустая или неверная → def.
func qDate(r *http.Request, key, def string) string {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if _, err := time.Parse("2006-01-02", v); err != nil {
		return def
	}
	return v
}

func param(r *http.Request, name string) string {
	v := chi.URLParam(r, name)
	if u, err := url.PathUnescape(v); err == nil {
		return u
	}
	return v
}

func contains2(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

// limit обрезает список до ?limit= (0 — без ограничения).
func limit[T any](r *http.Request, a []T) []T {
	if n := qInt(r, "limit", 0); n > 0 && len(a) > n {
		return a[:n]
	}
	return a
}

// leadReports — готовые расчёты «Контроля спринта» по всем руководителям (как для ТВ); если расчёта ещё нет — считаем и сохраняем.
func (s *extServer) leadReports(r *http.Request) (map[string]leadcontrol.Report, []string, error) {
	ctx := r.Context()
	leads, by, err := db.LeadControlLeads(ctx, s.pool)
	if err != nil {
		return nil, nil, err
	}
	out := map[string]leadcontrol.Report{}
	for _, lead := range leads {
		rep, err := s.leadReport(r, lead, by)
		if err != nil {
			return nil, nil, err
		}
		out[lead] = rep
	}
	return out, leads, nil
}

func (s *extServer) leadReport(r *http.Request, lead string, by map[string][]leadcontrol.LeadProject) (leadcontrol.Report, error) {
	ctx := r.Context()
	var rep leadcontrol.Report
	raw, _, err := db.LeadControl(ctx, s.pool, lead)
	switch {
	case err != nil:
		return rep, err
	case raw != nil:
		return rep, json.Unmarshal(raw, &rep)
	}
	reps, err := db.BuildLeadControl(ctx, s.pool, []string{lead}, by, time.Now(), s.jiraBase)
	if err != nil {
		return rep, err
	}
	rep = reps[lead]
	return rep, db.SaveLeadControl(ctx, s.pool, lead, rep)
}

func reportsList(m map[string]leadcontrol.Report, order []string) []leadcontrol.Report {
	out := make([]leadcontrol.Report, 0, len(order))
	for _, l := range order {
		out = append(out, m[l])
	}
	return out
}

// ---------- служебное ----------

func (s *extServer) ping(w http.ResponseWriter, r *http.Request) {
	last, err := db.LastSync(r.Context(), s.pool, "jira")
	if err != nil {
		fail(w, s.log, "ext/ping", err)
		return
	}
	reply(w, last, map[string]string{"status": "ok"}, -1)
}

// ---------- статистика ----------

type extSummary struct {
	Projects      int      `json:"projects"`
	People        int      `json:"people"`
	Tasks         extTasks `json:"tasks"`
	DonePct       int      `json:"done_pct"`
	Unplanned     int      `json:"unplanned"`
	UnplannedPct  int      `json:"unplanned_pct"`
	AddedAfter    int      `json:"added_after_start"`
	Blocked       int      `json:"blocked"`
	Stale         int      `json:"stale"`
	InTest        int      `json:"in_test"`
	OverdueSprint int      `json:"overdue_sprints"`
	AbsentNow     int      `json:"absent_now"`
	NeedTasks     int      `json:"need_tasks"`
	Overloaded    int      `json:"overloaded"`
	LongInWork    *int     `json:"long_in_work"`
	LongInTest    *int     `json:"long_in_test"`
}

func (c *extCtx) summary() extSummary {
	loads := c.employeeLoads(extNeedDefault, extOverDefault)
	sm := extSummary{Projects: len(c.Rep.Projects), People: len(loads), AbsentNow: len(c.Absent)}
	for _, p := range c.Rep.Projects {
		sm.Tasks.add(tasksOfProject(p))
		sm.Unplanned += p.Unplanned
		sm.AddedAfter += p.Added
		sm.Blocked += p.Blocked
		sm.Stale += p.Stale
		sm.InTest += p.InTest
		if p.LeftDays != nil && *p.LeftDays < 0 {
			sm.OverdueSprint++
		}
	}
	sm.DonePct = pctOf(sm.Tasks.Done, sm.Tasks.Total)
	sm.UnplannedPct = pctOf(sm.Unplanned, sm.Tasks.Total)
	for _, e := range loads {
		if e.NeedTasks && !c.Leads[e.Name] {
			sm.NeedTasks++
		}
		if e.Overloaded {
			sm.Overloaded++
		}
	}
	return sm
}

// GET /stats/summary — общая картина по текущим спринтам.
func (s *extServer) statsSummary(w http.ResponseWriter, r *http.Request) {
	c, ok := s.load(w, r)
	if !ok {
		return
	}
	sm := c.summary()
	if reps, order, err := s.leadReports(r); err == nil {
		w1, t1, _ := longTasks(reportsList(reps, order))
		a, b := len(w1), len(t1)
		sm.LongInWork, sm.LongInTest = &a, &b
	} else {
		s.log.Warn("ext/summary: контроль руководителей", "err", err)
	}
	reply(w, c.updated(), sm, -1)
}

// GET /stats/performance — успеваемость в целом за год: закрытые задачи, возвраты, выполнение плана спринта, посещаемость.
func (s *extServer) statsPerformance(w http.ResponseWriter, r *http.Request) {
	emp, _, hist, err := s.emp.get(r, s.pool)
	if err != nil {
		fail(w, s.log, "ext/performance", err)
		return
	}
	months := orgMonths(emp)
	out := struct {
		HistoryLoadedAt *time.Time `json:"history_loaded_at"`
		Employees       int        `json:"employees"`
		extPerfOut
	}{hist, len(emp.Ppl), perfOf(months, !qBool(r, "totals_only"))}
	reply(w, hist, out, -1)
}

// ---------- проекты ----------

func (s *extServer) projects(w http.ResponseWriter, r *http.Request) {
	c, ok := s.load(w, r)
	if !ok {
		return
	}
	lead, q := r.URL.Query().Get("lead"), r.URL.Query().Get("q")
	out := []extProject{}
	for _, p := range c.Rep.Projects {
		if lead != "" && !strings.EqualFold(p.Lead, lead) {
			continue
		}
		if q != "" && !contains2(p.Key, q) && !contains2(p.Name, q) {
			continue
		}
		out = append(out, projectView(p))
	}
	switch r.URL.Query().Get("sort") {
	case "done_pct":
		sort.SliceStable(out, func(i, j int) bool { return out[i].DonePct < out[j].DonePct })
	case "tasks":
		sort.SliceStable(out, func(i, j int) bool { return out[i].Tasks.Total > out[j].Tasks.Total })
	case "unplanned":
		sort.SliceStable(out, func(i, j int) bool { return out[i].UnplannedPct > out[j].UnplannedPct })
	default:
		sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	}
	reply(w, c.updated(), out, len(out))
}

func (s *extServer) projectDetail(w http.ResponseWriter, r *http.Request) {
	c, ok := s.load(w, r)
	if !ok {
		return
	}
	key := strings.ToUpper(param(r, "key"))
	var pr *metrics.ProjectSprint
	for i := range c.Rep.Projects {
		if c.Rep.Projects[i].Key == key {
			pr = &c.Rep.Projects[i]
		}
	}
	if pr == nil {
		extErr(w, http.StatusNotFound, "проект не найден среди проектов с активным спринтом")
		return
	}
	type person struct {
		Name     string   `json:"name"`
		Login    string   `json:"login"`
		Position string   `json:"position"`
		Tasks    extTasks `json:"tasks"`
		Queue    int      `json:"queue"`
	}
	people := []person{}
	for _, row := range c.Rep.People {
		if row.ProjectKey != key {
			continue
		}
		t := tasksOfPerson(row)
		people = append(people, person{row.Name, c.Rep.Logins[row.Name], c.role(row.Name), t, t.queue()})
	}
	sort.SliceStable(people, func(i, j int) bool { return people[i].Tasks.Total > people[j].Tasks.Total })
	type testerRow struct {
		Name string `json:"name"`
		extTesterProject
	}
	var trs []testerRow
	for _, q := range c.Rep.Testers {
		if q.ProjectKey != key {
			continue
		}
		waiting := 0
		if m := c.Rep.Testing[key]; m != nil && m[q.Name] != nil {
			waiting = m[q.Name].N
		}
		trs = append(trs, testerRow{q.Name, extTesterProject{Project: key, Tasks: q.Total, InTest: q.InTest, Done: q.Dn, Waiting: waiting, Open: q.Total - q.Dn}})
	}
	bn := []metrics.Bottleneck{}
	for _, b := range c.Rep.Bottlenecks {
		if projectOfKey(b.Key) == key {
			bn = append(bn, b)
		}
	}
	var next *metrics.NextSprint
	if ns, ok := c.Rep.Next[key]; ok {
		next = &ns
	}
	reply(w, c.updated(), map[string]any{
		"project": projectView(*pr), "people": people, "testers": nonNilAny(trs),
		"bottlenecks": bn, "next_sprint": next,
	}, -1)
}

func nonNilAny[T any](a []T) []T {
	if a == nil {
		return []T{}
	}
	return a
}

// ---------- сотрудники ----------

func (s *extServer) employees(w http.ResponseWriter, r *http.Request) {
	c, ok := s.load(w, r)
	if !ok {
		return
	}
	qs := r.URL.Query()
	loads := c.employeeLoads(qInt(r, "need_threshold", extNeedDefault), qInt(r, "overload_threshold", extOverDefault))
	var year map[string]extPerfOut
	if qHas(r, "include", "year") {
		emp, _, _, err := s.emp.get(r, s.pool)
		if err != nil {
			fail(w, s.log, "ext/employees", err)
			return
		}
		year = map[string]extPerfOut{}
		for _, row := range emp.Ppl {
			year[row.Name] = perfOf(employeeMonths(emp, row), false)
		}
	}
	type item struct {
		extEmployeeLoad
		ID   int64       `json:"id,omitempty"`
		Year *extPerfOut `json:"year,omitempty"`
	}
	ids := map[string]int64{}
	if year != nil || qHas(r, "include", "id") {
		if emp, _, _, err := s.emp.get(r, s.pool); err == nil {
			for _, row := range emp.Ppl {
				ids[row.Name] = row.ID
			}
		}
	}
	out := []item{}
	for _, e := range loads {
		if v := qs.Get("project"); v != "" && !extHasProject(e, v) {
			continue
		}
		if v := qs.Get("lead"); v != "" && !extHasLead(e, v) {
			continue
		}
		if v := qs.Get("role"); v != "" && !contains2(e.Position, v) {
			continue
		}
		if v := qs.Get("q"); v != "" && !contains2(e.Name, v) && !contains2(e.Login, v) {
			continue
		}
		if qBool(r, "need_tasks") && !e.NeedTasks {
			continue
		}
		if qBool(r, "overloaded") && !e.Overloaded {
			continue
		}
		if qBool(r, "absent") && e.AbsentNow == nil {
			continue
		}
		it := item{extEmployeeLoad: e, ID: ids[e.Name]}
		if y, ok := year[e.Name]; ok {
			it.Year = &y
		}
		out = append(out, it)
	}
	reply(w, c.updated(), out, len(out))
}

func extHasProject(e extEmployeeLoad, key string) bool {
	for _, p := range e.Projects {
		if strings.EqualFold(p.Project, key) {
			return true
		}
	}
	return false
}

func extHasLead(e extEmployeeLoad, lead string) bool {
	for _, p := range e.Projects {
		if strings.EqualFold(p.Lead, lead) {
			return true
		}
	}
	return false
}

// employeeDetail — сотрудник по id (числовой, из GET /employees?include=id) или по логину Jira.
func (s *extServer) employeeDetail(w http.ResponseWriter, r *http.Request) {
	c, ok := s.load(w, r)
	if !ok {
		return
	}
	emp, _, hist, err := s.emp.get(r, s.pool)
	if err != nil {
		fail(w, s.log, "ext/employee", err)
		return
	}
	ref := param(r, "id")
	var row *metrics.EmpRow
	for i := range emp.Ppl {
		p := &emp.Ppl[i]
		if strconv.FormatInt(p.ID, 10) == ref || strings.EqualFold(p.Login, ref) {
			row = p
			break
		}
	}
	if row == nil {
		extErr(w, http.StatusNotFound, "сотрудник не найден")
		return
	}
	var load *extEmployeeLoad
	for _, e := range c.employeeLoads(extNeedDefault, extOverDefault) {
		if e.Name == row.Name {
			e := e
			load = &e
		}
	}
	months := employeeMonths(emp, *row)
	out := map[string]any{
		"id": row.ID, "name": row.Name, "login": row.Login, "position": row.Pos, "team_lead": row.TL, "project_lead": row.Lead,
		"sprint_load": load, "year": perfOf(months, true), "year_projects": row.Proj, "open_tasks": map[string]int{"not_started": row.Open[0], "in_progress": row.Open[1]},
		"cycle_days_median": row.Cyc,
	}
	if a, ok := c.Absent[row.Name]; ok {
		out["absent_now"] = absenceView(a, true)
	}
	abs := []extAbsence{}
	for _, a := range c.Rep.Absences {
		if a.Name == row.Name {
			abs = append(abs, absenceView(a, true))
		}
	}
	out["absences"] = abs
	if card := emp.Cards[row.ID]; card != nil {
		if qHas(r, "include", "tasks") {
			out["tasks"] = cardTasks(card)
		}
		if qHas(r, "include", "attendance_log") {
			log := make([]map[string]any, 0, len(card.Alog))
			for _, a := range card.Alog {
				if len(a) >= 4 {
					log = append(log, map[string]any{"date": a[0], "what": a[1], "minutes": a[2], "details": a[3]})
				}
			}
			out["attendance_log"] = log
		}
	}
	reply(w, hist, out, -1)
}

// cardTasks — задачи карточки сотрудника: [ключ, название, тип, статус, создана, закрыта|"", спринтов, возвратов, цикл дн.|null, часы, проект].
func cardTasks(c *metrics.EmpCard) []map[string]any {
	out := make([]map[string]any, 0, len(c.Tasks))
	for _, t := range c.Tasks {
		if len(t) < 11 {
			continue
		}
		out = append(out, map[string]any{"key": t[0], "summary": t[1], "type": t[2], "status": t[3], "created": t[4], "closed": t[5],
			"sprints": t[6], "returns": t[7], "cycle_days": t[8], "hours": t[9], "project": t[10]})
	}
	return out
}

// ---------- руководители ----------

func (s *extServer) leads(w http.ResponseWriter, r *http.Request) {
	c, ok := s.load(w, r)
	if !ok {
		return
	}
	stats := c.leadStats()
	if q := r.URL.Query().Get("q"); q != "" {
		f := stats[:0]
		for _, l := range stats {
			if contains2(l.Name, q) {
				f = append(f, l)
			}
		}
		stats = f
	}
	reply(w, c.updated(), stats, len(stats))
}

// leadDetail — проекты руководителя из Jira, люди его проектов и готовый расчёт «Контроля спринта» (если он есть).
func (s *extServer) leadDetail(w http.ResponseWriter, r *http.Request) {
	c, ok := s.load(w, r)
	if !ok {
		return
	}
	name := param(r, "name")
	var stat *extLeadStat
	for _, l := range c.leadStats() {
		if strings.EqualFold(l.Name, name) {
			l := l
			stat = &l
		}
	}
	leads, by, err := db.LeadControlLeads(r.Context(), s.pool)
	if err != nil {
		fail(w, s.log, "ext/lead", err)
		return
	}
	controlName := ""
	for _, l := range leads {
		if strings.EqualFold(l, name) {
			controlName = l
		}
	}
	if stat == nil && controlName == "" {
		extErr(w, http.StatusNotFound, "руководитель не найден")
		return
	}
	out := map[string]any{"jira_projects": stat}
	if stat != nil {
		keys := map[string]bool{}
		projects := []extProject{}
		for _, p := range c.Rep.Projects {
			if strings.EqualFold(p.Lead, stat.Name) {
				keys[p.Key] = true
				projects = append(projects, projectView(p))
			}
		}
		out["projects"] = projects
		var loads []extEmployeeLoad
		for _, e := range c.employeeLoads(extNeedDefault, extOverDefault) {
			for _, p := range e.Projects {
				if keys[p.Project] {
					loads = append(loads, e)
					break
				}
			}
		}
		out["people"] = nonNilAny(loads)
	}
	var control any
	if controlName != "" {
		rep, err := s.leadReport(r, controlName, by)
		if err != nil {
			fail(w, s.log, "ext/lead", err)
			return
		}
		control = rep
	}
	out["control"] = control
	reply(w, c.updated(), out, -1)
}

// ---------- тимлиды ----------

func (s *extServer) teamLeads(w http.ResponseWriter, r *http.Request) {
	c, ok := s.load(w, r)
	if !ok {
		return
	}
	loads := map[string]extEmployeeLoad{}
	for _, e := range c.employeeLoads(extNeedDefault, extOverDefault) {
		loads[e.Name] = e
	}
	out := []extEmployeeLoad{}
	for _, n := range c.Rep.TeamLeads {
		e, ok := loads[n]
		if !ok {
			e = extEmployeeLoad{Name: n, Login: c.Rep.Logins[n], Position: c.role(n), TeamLead: true, Projects: []extPersonProject{}}
			if a, ok := c.Absent[n]; ok {
				v := absenceView(a, false)
				e.AbsentNow = &v
			}
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	reply(w, c.updated(), out, len(out))
}

// ---------- тестировщики ----------

func (s *extServer) testersList(w http.ResponseWriter, r *http.Request) {
	c, ok := s.load(w, r)
	if !ok {
		return
	}
	out := []extTester{}
	for _, t := range c.testers() {
		if q := r.URL.Query().Get("q"); q != "" && !contains2(t.Name, q) {
			continue
		}
		out = append(out, t)
	}
	reply(w, c.updated(), out, len(out))
}

func (s *extServer) testerDetail(w http.ResponseWriter, r *http.Request) {
	c, ok := s.load(w, r)
	if !ok {
		return
	}
	name := param(r, "name")
	for _, t := range c.testers() {
		if strings.EqualFold(t.Name, name) || (t.Login != "" && strings.EqualFold(t.Login, name)) {
			reply(w, c.updated(), t, -1)
			return
		}
	}
	extErr(w, http.StatusNotFound, "тестировщик не найден")
}

// ---------- посещаемость ----------

func monthStart(now time.Time) string {
	l := now.In(metrics.Almaty)
	return time.Date(l.Year(), l.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
}

// GET /attendance?from=&to= — сводка за период (по умолчанию текущий месяц): кто отсутствует сейчас и показатели по людям.
func (s *extServer) attendanceSummary(w http.ResponseWriter, r *http.Request) {
	c, ok := s.load(w, r)
	if !ok {
		return
	}
	att, err := loadAttendance(r, s.pool)
	if err != nil {
		fail(w, s.log, "ext/attendance", err)
		return
	}
	now := time.Now()
	from, to := qDate(r, "from", monthStart(now)), qDate(r, "to", c.Today)
	people := c.attendanceByPerson(att, from, to)
	if q := r.URL.Query().Get("q"); q != "" {
		f := people[:0]
		for _, p := range people {
			if contains2(p.Name, q) || contains2(p.Login, q) {
				f = append(f, p)
			}
		}
		people = f
	}
	absent := []map[string]any{}
	for n, a := range c.Absent {
		absent = append(absent, map[string]any{"name": n, "login": c.Rep.Logins[n], "position": c.role(n), "absence": absenceView(a, qBool(r, "reasons"))})
	}
	sort.SliceStable(absent, func(i, j int) bool { return absent[i]["name"].(string) < absent[j]["name"].(string) })
	reply(w, c.updated(), map[string]any{"from": from, "to": to, "absent_today": absent, "people": people}, len(people))
}

type extAttEvent struct {
	Date           string `json:"date"`
	Time           string `json:"time"` // когда отправлено сообщение
	Name           string `json:"name"`
	Type           string `json:"type"` // late | early | mid | day | sick | vac | upd
	Minutes        int    `json:"minutes"`
	From           string `json:"from,omitempty"`
	To             string `json:"to,omitempty"`
	Reason         string `json:"reason"`
	ReasonCategory string `json:"reason_category"` // study | health | family | personal | other
	ApprovedBy     string `json:"approved_by"`
	MakeupDate     string `json:"makeup_date,omitempty"`
	Makeup         string `json:"makeup"`
}

func hm(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v/60/10) + strconv.Itoa(*v/60%10) + ":" + strconv.Itoa(*v%60/10) + strconv.Itoa(*v%60%10)
}

// GET /attendance/events?from=&to=&employee=&type= — отдельные записи посещаемости (с причинами).
func (s *extServer) attendanceEvents(w http.ResponseWriter, r *http.Request) {
	c, ok := s.load(w, r)
	if !ok {
		return
	}
	att, err := loadAttendance(r, s.pool)
	if err != nil {
		fail(w, s.log, "ext/attendance", err)
		return
	}
	from, to := qDate(r, "from", monthStart(time.Now())), qDate(r, "to", c.Today)
	emp, typ := r.URL.Query().Get("employee"), r.URL.Query().Get("type")
	out := []extAttEvent{}
	for _, e := range att.Ev {
		if e.Date < from || e.Date > to {
			continue
		}
		if emp != "" && !contains2(e.N, emp) && !strings.EqualFold(c.Rep.Logins[e.N], emp) {
			continue
		}
		if typ != "" && e.T != typ {
			continue
		}
		out = append(out, extAttEvent{Date: e.Date, Time: e.Post, Name: e.N, Type: e.T, Minutes: e.M, From: hm(e.A), To: hm(e.B), Reason: e.Rsn,
			ReasonCategory: e.C, ApprovedBy: e.Ag, MakeupDate: e.Od, Makeup: e.Otr})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date > out[j].Date
		}
		return out[i].Time > out[j].Time
	})
	out = limit(r, out)
	reply(w, c.updated(), out, len(out))
}

// GET /absences?days=14 — текущие и будущие отсутствия (отпуск, больничный, БС, «отпросился») на ближайшие дни.
func (s *extServer) absencesList(w http.ResponseWriter, r *http.Request) {
	c, ok := s.load(w, r)
	if !ok {
		return
	}
	today, _ := time.Parse("2006-01-02", c.Today)
	until := today.AddDate(0, 0, qInt(r, "days", 14)).Format("2006-01-02")
	type row struct {
		Name     string `json:"name"`
		Login    string `json:"login"`
		Position string `json:"position"`
		extAbsence
		Now bool `json:"now"`
	}
	out := []row{}
	for _, a := range c.Rep.Absences {
		if (a.To != nil && *a.To < c.Today) || a.From > until {
			continue
		}
		out = append(out, row{a.Name, c.Rep.Logins[a.Name], c.role(a.Name), absenceView(a, qBool(r, "reasons")), a.From <= c.Today})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Now != out[j].Now {
			return out[i].Now
		}
		return out[i].From < out[j].From
	})
	reply(w, c.updated(), out, len(out))
}

// ---------- списки ----------

func (s *extServer) listNeedTasks(w http.ResponseWriter, r *http.Request) {
	c, ok := s.load(w, r)
	if !ok {
		return
	}
	th, all := qInt(r, "threshold", extNeedDefault), qBool(r, "include_excluded")
	out := []extEmployeeLoad{}
	for _, e := range c.employeeLoads(th, extOverDefault) {
		if e.AbsentNow != nil || e.JiraInactive || e.Queue > th || (!all && (c.excluded(e.Name) || c.Leads[e.Name])) {
			continue
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Queue != out[j].Queue {
			return out[i].Queue < out[j].Queue
		}
		return out[i].Name < out[j].Name
	})
	reply(w, c.updated(), out, len(out))
}

func (s *extServer) listOverloaded(w http.ResponseWriter, r *http.Request) {
	c, ok := s.load(w, r)
	if !ok {
		return
	}
	th, all := qInt(r, "threshold", extOverDefault), qBool(r, "include_excluded")
	out := []extEmployeeLoad{}
	for _, e := range c.employeeLoads(extNeedDefault, th) {
		if e.Tasks.Backlog < th || (!all && (c.TL[e.Name] || c.pmLike(e.Name) || extQARe.MatchString(e.Position))) {
			continue
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Tasks.Backlog != out[j].Tasks.Backlog {
			return out[i].Tasks.Backlog > out[j].Tasks.Backlog
		}
		return out[i].Name < out[j].Name
	})
	reply(w, c.updated(), out, len(out))
}

// longList — общая часть списков из «Контроля спринта руководителей».
func (s *extServer) longList(w http.ResponseWriter, r *http.Request, pick func(w1, t1, b1 []extLongTask) []extLongTask) {
	c, ok := s.load(w, r)
	if !ok {
		return
	}
	reps, order, err := s.leadReports(r)
	if err != nil {
		fail(w, s.log, "ext/lists", err)
		return
	}
	w1, t1, b1 := longTasks(reportsList(reps, order))
	out := pick(w1, t1, b1)
	if p := r.URL.Query().Get("project"); p != "" {
		f := []extLongTask{}
		for _, x := range out {
			if strings.EqualFold(x.Project, p) {
				f = append(f, x)
			}
		}
		out = f
	}
	if a := r.URL.Query().Get("assignee"); a != "" {
		f := []extLongTask{}
		for _, x := range out {
			if contains2(x.Assignee, a) {
				f = append(f, x)
			}
		}
		out = f
	}
	out = limit(r, nonNilAny(out))
	reply(w, c.updated(), out, len(out))
}

func (s *extServer) listLongWork(w http.ResponseWriter, r *http.Request) {
	s.longList(w, r, func(a, _, _ []extLongTask) []extLongTask { return a })
}
func (s *extServer) listLongTest(w http.ResponseWriter, r *http.Request) {
	s.longList(w, r, func(_, t, _ []extLongTask) []extLongTask { return t })
}
func (s *extServer) listBlocked(w http.ResponseWriter, r *http.Request) {
	s.longList(w, r, func(_, _, b []extLongTask) []extLongTask { return b })
}

// bottlenecks — задачи в Blocked / Need Rework / «Требует уточнения» или без обновлений больше 30 дн. («Узкие места»).
func (s *extServer) listBottlenecks(w http.ResponseWriter, r *http.Request) {
	c, ok := s.load(w, r)
	if !ok {
		return
	}
	out := []metrics.Bottleneck{}
	p := r.URL.Query().Get("project")
	for _, b := range c.Rep.Bottlenecks {
		if p == "" || strings.EqualFold(projectOfKey(b.Key), p) {
			out = append(out, b)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Days > out[j].Days })
	out = limit(r, out)
	reply(w, c.updated(), out, len(out))
}

func (s *extServer) listLowSprints(w http.ResponseWriter, r *http.Request) {
	c, ok := s.load(w, r)
	if !ok {
		return
	}
	out := c.lowSprints()
	reply(w, c.updated(), out, len(out))
}

func (s *extServer) listRisks(w http.ResponseWriter, r *http.Request) {
	c, ok := s.load(w, r)
	if !ok {
		return
	}
	out := limit(r, c.projectRisks())
	reply(w, c.updated(), out, len(out))
}

func (s *extServer) listSpikes(w http.ResponseWriter, r *http.Request) {
	c, ok := s.load(w, r)
	if !ok {
		return
	}
	out := limit(r, c.creationSpikes())
	reply(w, c.updated(), out, len(out))
}

// ---------- дэшборд ----------

type extTestLoad struct {
	Project string `json:"project"`
	Name    string `json:"name"`
	Lead    string `json:"lead"`
	InTest  int    `json:"in_test"`
	Level   string `json:"level"` // "" | warn | bad
}

// GET /dashboards/sprints — сводка для экрана/дэшборда: ключевые числа и все важные сигналы одним ответом (как экран ТВ).
func (s *extServer) dashboardSprints(w http.ResponseWriter, r *http.Request) {
	c, ok := s.load(w, r)
	if !ok {
		return
	}
	sm := c.summary()
	top := func(n int) int { return qInt(r, "top", n) }
	need := []extEmployeeLoad{}
	over := []extEmployeeLoad{}
	for _, e := range c.employeeLoads(extNeedDefault, extOverDefault) {
		if e.NeedTasks && !c.Leads[e.Name] {
			need = append(need, e)
		}
		if e.Overloaded {
			over = append(over, e)
		}
	}
	sort.SliceStable(need, func(i, j int) bool {
		if need[i].Queue != need[j].Queue {
			return need[i].Queue < need[j].Queue
		}
		return need[i].Name < need[j].Name
	})
	sort.SliceStable(over, func(i, j int) bool { return over[i].Tasks.Backlog > over[j].Tasks.Backlog })

	tests := []extTestLoad{}
	for _, p := range c.Rep.Projects {
		if p.InTest > 0 {
			lvl := ""
			if p.InTest >= extTestHigh {
				lvl = "bad"
			} else if p.InTest >= extTestWarn {
				lvl = "warn"
			}
			tests = append(tests, extTestLoad{p.Key, p.Name, p.Lead, p.InTest, lvl})
		}
	}
	sort.SliceStable(tests, func(i, j int) bool { return tests[i].InTest > tests[j].InTest })

	absent := []map[string]any{}
	for n, a := range c.Absent {
		absent = append(absent, map[string]any{"name": n, "position": c.role(n), "absence": absenceView(a, false)})
	}
	sort.SliceStable(absent, func(i, j int) bool { return absent[i]["name"].(string) < absent[j]["name"].(string) })

	var inWork, testing, blocked []extLongTask
	if reps, order, err := s.leadReports(r); err == nil {
		inWork, testing, blocked = longTasks(reportsList(reps, order))
		a, b := len(inWork), len(testing)
		sm.LongInWork, sm.LongInTest = &a, &b
	} else {
		s.log.Warn("ext/dashboard: контроль руководителей", "err", err)
	}
	n := top(10)
	cut := func(a []extLongTask) []extLongTask {
		if len(a) > n {
			a = a[:n]
		}
		return nonNilAny(a)
	}
	cutE := func(a []extEmployeeLoad) []extEmployeeLoad {
		if len(a) > n {
			a = a[:n]
		}
		return nonNilAny(a)
	}
	risks, low, spikes := c.projectRisks(), c.lowSprints(), c.creationSpikes()
	if len(risks) > n {
		risks = risks[:n]
	}
	if len(spikes) > n {
		spikes = spikes[:n]
	}
	reply(w, c.updated(), map[string]any{
		"kpi": sm, "absent_today": absent, "need_tasks": cutE(need), "overloaded": cutE(over), "tests_by_project": tests,
		"low_sprints": low, "risks": risks, "creation_spikes": spikes,
		"long_in_work": cut(inWork), "long_in_test": cut(testing), "blocked": cut(blocked),
		"thresholds": map[string]int{"need_tasks_queue_max": extNeedDefault, "overloaded_backlog_min": extOverDefault, "long_in_work_days": extLongWork,
			"long_in_test_days": extLongTest, "creation_spike_per_day": extSpikeMin},
	}, -1)
}
