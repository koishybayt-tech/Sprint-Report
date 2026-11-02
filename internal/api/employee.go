package api

import (
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adata/reports/internal/auth"
	"github.com/adata/reports/internal/db"
	"github.com/adata/reports/internal/metrics"
)

// Карточка сотрудника считается по ~23 тыс. задач года с историей — держим готовый расчёт минуту.
const employeesTTL = time.Minute

type employeesCache struct {
	mu   sync.Mutex
	at   time.Time
	emp  metrics.Employees
	att  metrics.Attendance
	hist *time.Time
}

func (c *employeesCache) get(r *http.Request, pool *pgxpool.Pool) (metrics.Employees, metrics.Attendance, *time.Time, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.at) < employeesTTL {
		return c.emp, c.att, c.hist, nil
	}
	ctx := r.Context()
	att, err := loadAttendance(r, pool)
	if err != nil {
		return metrics.Employees{}, metrics.Attendance{}, nil, err
	}
	issues, err := db.EmpIssues(ctx, pool)
	if err != nil {
		return metrics.Employees{}, metrics.Attendance{}, nil, err
	}
	people, err := db.EmpPeople(ctx, pool)
	if err != nil {
		return metrics.Employees{}, metrics.Attendance{}, nil, err
	}
	for i := range people {
		if p, ok := att.Ppl[people[i].Name]; ok {
			people[i].Pos, people[i].TL = p.Pos, p.TL
		}
	}
	hist, err := db.HistoryLoadedAt(ctx, pool)
	if err != nil {
		return metrics.Employees{}, metrics.Attendance{}, nil, err
	}
	c.emp = metrics.BuildEmployees(issues, att.Ev, people, time.Now())
	c.att, c.hist, c.at = att, hist, time.Now()
	return c.emp, c.att, c.hist, nil
}

type employeesResp struct {
	metrics.Employees
	History *time.Time `json:"history"` // когда загружена история Jira; null — ещё грузится
	Tasks   bool       `json:"tasks"`   // показывать ли задачи (HR — нет)
}

// visibleEmployees — кого видит пользователь (docs/security.md): admin, hr — всех (hr без задач);
// lead — людей своих проектов и себя; employee — только себя.
func visibleEmployees(r *http.Request, pool *pgxpool.Pool, emp metrics.Employees, att metrics.Attendance) (metrics.Employees, auth.User, error) {
	u, _ := auth.FromContext(r.Context())
	allowed := map[int64]bool{}
	switch u.Role {
	case auth.RoleAdmin, auth.RoleHR:
		for _, p := range emp.Ppl {
			allowed[p.ID] = true
		}
	case auth.RoleLead:
		self, err := selfName(r, pool, u)
		if err != nil {
			return metrics.Employees{}, u, err
		}
		names := metrics.LeadScope(&att, self)
		for _, p := range emp.Ppl {
			if names[p.Name] {
				allowed[p.ID] = true
			}
		}
	}
	if u.EmployeeID != nil {
		allowed[*u.EmployeeID] = true
	}
	return metrics.RestrictEmployees(emp, allowed, u.Role == auth.RoleHR), u, nil
}

// GET /api/v1/employees — список сотрудников с помесячными показателями года (EMP.ppl без задач и журнала).
func employeesList(pool *pgxpool.Pool, log *slog.Logger, cache *employeesCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		emp, att, hist, err := cache.get(r, pool)
		if err != nil {
			fail(w, log, "employees", err)
			return
		}
		vis, u, err := visibleEmployees(r, pool, emp, att)
		if err != nil {
			fail(w, log, "employees", err)
			return
		}
		writeJSON(w, http.StatusOK, employeesResp{Employees: vis, History: hist, Tasks: u.Role != auth.RoleHR})
	}
}

// GET /api/v1/employees/{id} — задачи и журнал посещаемости одного сотрудника.
// Причины отсутствий — только admin, hr и самому сотруднику.
func employeeCard(pool *pgxpool.Pool, log *slog.Logger, cache *employeesCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверный id"})
			return
		}
		emp, att, _, err := cache.get(r, pool)
		if err != nil {
			fail(w, log, "employee", err)
			return
		}
		vis, u, err := visibleEmployees(r, pool, emp, att)
		if err != nil {
			fail(w, log, "employee", err)
			return
		}
		card := vis.Cards[id]
		if card == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "сотрудник не найден или нет доступа"})
			return
		}
		self := u.EmployeeID != nil && *u.EmployeeID == id
		if u.Role == auth.RoleLead && !self {
			metrics.HideAlogReasons(card)
		}
		writeJSON(w, http.StatusOK, card)
	}
}
