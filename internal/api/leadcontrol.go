package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adata/reports/internal/api/lcapi"
	"github.com/adata/reports/internal/auth"
	"github.com/adata/reports/internal/db"
	"github.com/adata/reports/internal/leadcontrol"
	"github.com/adata/reports/internal/notify"
)

// canLead — может ли пользователь управлять разрывами руководителя (admin — любого, lead — своими).
func (s *leadControlServer) canLead(w http.ResponseWriter, r *http.Request, lead string) bool {
	leads, _, err := s.visibleLeads(r)
	if err != nil {
		fail(w, s.log, "lead-control", err)
		return false
	}
	if !containsStr(leads, lead) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "руководитель видит только свои проекты"})
		return false
	}
	return true
}

// POST /api/v1/lead-control/gaps/ignore {lead, key} — «Игнорировать»: скрыть из разрывов и не уведомлять.
func (s *leadControlServer) IgnoreLeadControlGap(w http.ResponseWriter, r *http.Request) {
	var body lcapi.LeadControlGapRef
	if !readJSON(w, r, &body) {
		return
	}
	if body.Key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "нет ключа задачи"})
		return
	}
	if !s.canLead(w, r, body.Lead) {
		return
	}
	u, _ := auth.FromContext(r.Context())
	if err := s.notify.Ignore(r.Context(), body.Lead, body.Key, u.ID); err != nil {
		fail(w, s.log, "lead-control ignore", err)
		return
	}
	if err := db.RebuildLead(r.Context(), s.pool, body.Lead, s.jiraBase); err != nil {
		fail(w, s.log, "lead-control ignore", err)
		return
	}
	s.log.Info("lead-control: скрыто из разрывов", "lead", body.Lead, "key", body.Key, "by", u.Login)
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /api/v1/lead-control/gaps/ignore?lead=&key= — вернуть в разрывы.
func (s *leadControlServer) UnignoreLeadControlGap(w http.ResponseWriter, r *http.Request, params lcapi.UnignoreLeadControlGapParams) {
	if !s.canLead(w, r, params.Lead) {
		return
	}
	if err := s.notify.Unignore(r.Context(), params.Lead, params.Key); err != nil {
		fail(w, s.log, "lead-control unignore", err)
		return
	}
	if err := db.RebuildLead(r.Context(), s.pool, params.Lead, s.jiraBase); err != nil {
		fail(w, s.log, "lead-control unignore", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// leadControlServer реализует сгенерированный lcapi.ServerInterface (api/openapi.yaml, тег lead-control).
// Доступ — admin, lead и пользователи с назначенными проектами (requireLeadAccess в роутере); не-admin видит только свои проекты.
type leadControlServer struct {
	pool     *pgxpool.Pool
	log      *slog.Logger
	jiraBase string
	notify   *notify.Service
}

var _ lcapi.ServerInterface = (*leadControlServer)(nil)

// visibleLeads — руководители, доступные пользователю: admin — все из справочника, остальные — только свои:
// он сам (по сотруднику) и те, под кем ему назначены проекты. Свои идут первыми: «по умолчанию видит себя».
func (s *leadControlServer) visibleLeads(r *http.Request) ([]string, map[string][]leadcontrol.LeadProject, error) {
	leads, by, err := db.LeadControlLeads(r.Context(), s.pool)
	if err != nil {
		return nil, nil, err
	}
	u, _ := auth.FromContext(r.Context())
	self, err := selfName(r, s.pool, u)
	if err != nil {
		return nil, nil, err
	}
	own := []string{}
	if _, ok := by[self]; ok && self != "" {
		own = append(own, self)
	}
	assigned, err := db.UserLeads(r.Context(), s.pool, u.ID)
	if err != nil {
		return nil, nil, err
	}
	for _, l := range assigned {
		if _, ok := by[l]; ok && !containsStr(own, l) {
			own = append(own, l)
		}
	}
	// Подчинённые (в том числе подчинённые подчинённых) и их проекты — после своих.
	subs, err := auth.NewStore(s.pool).Subordinates(r.Context(), u.ID)
	if err != nil {
		return nil, nil, err
	}
	team, err := db.LeadsOfUsers(r.Context(), s.pool, subs)
	if err != nil {
		return nil, nil, err
	}
	for _, l := range team {
		if _, ok := by[l]; ok && !containsStr(own, l) {
			own = append(own, l)
		}
	}
	if u.Role != auth.RoleAdmin {
		return own, by, nil
	}
	out := append([]string{}, own...)
	for _, l := range leads {
		if !containsStr(own, l) {
			out = append(out, l)
		}
	}
	return out, by, nil
}

// GET /api/v1/lead-control/leads
func (s *leadControlServer) GetLeadControlLeads(w http.ResponseWriter, r *http.Request) {
	leads, by, err := s.visibleLeads(r)
	if err != nil {
		fail(w, s.log, "lead-control leads", err)
		return
	}
	out := make([]lcapi.LeadControlLead, 0, len(leads))
	for _, l := range leads {
		ps := make([]lcapi.LeadControlProject, 0, len(by[l]))
		for _, p := range by[l] {
			ps = append(ps, lcapi.LeadControlProject{Key: p.Key, Title: p.Title})
		}
		out = append(out, lcapi.LeadControlLead{Lead: l, Projects: ps})
	}
	writeJSON(w, http.StatusOK, out)
}

// GET /api/v1/lead-control?lead=&project= — готовый расчёт (пишет worker); если его ещё нет — считается сейчас.
func (s *leadControlServer) GetLeadControl(w http.ResponseWriter, r *http.Request, params lcapi.GetLeadControlParams) {
	leads, by, err := s.visibleLeads(r)
	if err != nil {
		fail(w, s.log, "lead-control", err)
		return
	}
	if len(leads) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "вас нет в справочнике руководителей — попросите администратора добавить ваши проекты в «Настройках»"})
		return
	}
	lead := leads[0]
	if params.Lead != nil && *params.Lead != "" {
		lead = *params.Lead
	}
	if !containsStr(leads, lead) {
		if _, known := by[lead]; known {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "руководитель видит только свои проекты"})
		} else {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "руководителя нет в справочнике"})
		}
		return
	}
	var rep leadcontrol.Report
	raw, _, err := db.LeadControl(r.Context(), s.pool, lead)
	if err != nil {
		fail(w, s.log, "lead-control", err)
		return
	}
	if raw != nil {
		err = json.Unmarshal(raw, &rep)
	} else {
		var reps map[string]leadcontrol.Report
		if reps, err = db.BuildLeadControl(r.Context(), s.pool, []string{lead}, by, time.Now(), s.jiraBase); err == nil {
			rep = reps[lead]
			err = db.SaveLeadControl(r.Context(), s.pool, lead, rep)
		}
	}
	if err != nil {
		fail(w, s.log, "lead-control", err)
		return
	}
	// Проекты — из справочника (могли поменять после расчёта); фильтр по проекту — на сервере.
	rep.Projects = by[lead]
	if params.Project != nil && *params.Project != "" && *params.Project != "all" {
		p := *params.Project
		if !hasProject(by[lead], p) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "такого проекта у руководителя нет"})
			return
		}
		rep = filterProject(rep, p)
	}
	writeJSON(w, http.StatusOK, rep)
}

func containsStr(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func hasProject(ps []leadcontrol.LeadProject, key string) bool {
	for _, p := range ps {
		if p.Key == key {
			return true
		}
	}
	return false
}

// filterProject оставляет строки одного проекта (итоги — «все проекты» и этот проект).
func filterProject(r leadcontrol.Report, p string) leadcontrol.Report {
	keep := func(project string) bool { return project == p }
	out := r
	out.Totals, out.Sprints, out.Blocked, out.InWork, out.People, out.Gaps, out.Testing, out.Ignored, out.Assigned =
		[]leadcontrol.Totals{}, []leadcontrol.SprintRow{}, []leadcontrol.BlockedRow{}, []leadcontrol.InWorkRow{},
		[]leadcontrol.PeopleRow{}, []leadcontrol.GapRow{}, []leadcontrol.TestingRow{}, []leadcontrol.IgnoredRow{}, []leadcontrol.AssignedRow{}
	for _, x := range r.Assigned {
		if keep(x.Project) {
			out.Assigned = append(out.Assigned, x)
		}
	}
	for _, x := range r.Ignored {
		if keep(x.Project) {
			out.Ignored = append(out.Ignored, x)
		}
	}
	for _, x := range r.Totals {
		if x.Project == "all" || keep(x.Project) {
			out.Totals = append(out.Totals, x)
		}
	}
	for _, x := range r.Sprints {
		if keep(x.Project) {
			out.Sprints = append(out.Sprints, x)
		}
	}
	for _, x := range r.Blocked {
		if keep(x.Project) {
			out.Blocked = append(out.Blocked, x)
		}
	}
	for _, x := range r.InWork {
		if keep(x.Project) {
			out.InWork = append(out.InWork, x)
		}
	}
	for _, x := range r.People {
		if containsStr(strings.Fields(x.Projects), p) {
			out.People = append(out.People, x)
		}
	}
	for _, x := range r.Gaps {
		if keep(x.Project) {
			out.Gaps = append(out.Gaps, x)
		}
	}
	for _, x := range r.Testing {
		if keep(x.Project) {
			out.Testing = append(out.Testing, x)
		}
	}
	return out
}
