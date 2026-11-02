package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adata/reports/internal/auth"
	"github.com/adata/reports/internal/db"
	"github.com/adata/reports/internal/metrics"
)

// Срез дня — 18:00: до этого времени сегодняшний день в ряду ещё не полный, история — по вчера.
const snapCutHour = 18

var errNoJira = errors.New("Jira ещё не синхронизирована")

// buildSnapshot считает снепшот на сейчас по данным в БД (sprint_port.js).
func buildSnapshot(r *http.Request, pool *pgxpool.Pool) (metrics.Snapshot, error) {
	ctx := r.Context()
	rep, err := loadReport(r, pool)
	if err != nil {
		return metrics.Snapshot{}, err
	}
	if rep == nil {
		return metrics.Snapshot{}, errNoJira
	}
	names, err := db.JiraNames(ctx, pool)
	if err != nil {
		return metrics.Snapshot{}, err
	}
	issues, err := db.SnapIssues(ctx, pool, names)
	if err != nil {
		return metrics.Snapshot{}, err
	}
	off, err := db.DaysOff(ctx, pool)
	if err != nil {
		return metrics.Snapshot{}, err
	}
	staffPos, err := db.StaffPositions(ctx, pool)
	if err != nil {
		return metrics.Snapshot{}, err
	}
	now := time.Now().In(metrics.Almaty)
	since := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, metrics.Almaty)
	abs, err := db.Absences(ctx, pool, since)
	if err != nil {
		return metrics.Snapshot{}, err
	}
	tg, err := db.TelegramEvents(ctx, pool, since)
	if err != nil {
		return metrics.Snapshot{}, err
	}
	abs = append(abs, metrics.TelegramAbsences(tg, since)...)

	nameOf := func(login, full string) string {
		if n, ok := names[login]; ok && login != "" {
			return n
		}
		return full
	}
	absDays := map[string][]string{}
	for _, a := range abs {
		n := nameOf(a.Login, a.FullName)
		to := a.From
		if a.To != nil {
			to = *a.To
		}
		for d := a.From; !d.After(to); d = d.AddDate(0, 0, 1) {
			absDays[n] = append(absDays[n], d.Format("2006-01-02"))
		}
	}
	var staff []string
	pos := map[string]string{}
	for n, p := range rep.Roles {
		pos[n] = p
	}
	for login, p := range staffPos {
		n := nameOf(login, login)
		staff = append(staff, n)
		if pos[n] == "" {
			pos[n] = p
		}
	}
	today := now.Format("2006-01-02")
	histEnd := today
	if now.Hour() < snapCutHour {
		histEnd = now.AddDate(0, 0, -1).Format("2006-01-02")
	}
	return metrics.BuildSnapshot(metrics.SnapInput{
		Today: today, HistEnd: histEnd, Issues: issues, Projects: rep.Projects,
		Staff: staff, Pos: pos, AbsDays: absDays, AbsNow: rep.Absences, Holidays: off,
	}), nil
}

type snapshotResp struct {
	Info     *db.SnapshotInfo `json:"info"` // null — живой расчёт, не сохранён
	Snapshot json.RawMessage  `json:"snapshot"`
}

// GET /api/v1/snapshots — сохранённые снепшоты.
func snapshotsList(pool *pgxpool.Pool, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := db.ListSnapshots(r.Context(), pool)
		if err != nil {
			fail(w, log, "snapshots", err)
			return
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// GET /api/v1/snapshots/live — расчёт на сейчас, без сохранения.
func snapshotLive(pool *pgxpool.Pool, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, err := buildSnapshot(r, pool)
		if errors.Is(err, errNoJira) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		if err != nil {
			fail(w, log, "snapshot live", err)
			return
		}
		b, _ := json.Marshal(s)
		writeJSON(w, http.StatusOK, snapshotResp{Snapshot: b})
	}
}

var reDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// GET /api/v1/snapshots/{date} — сохранённый снепшот: читается payload как есть, без пересчёта.
func snapshotGet(pool *pgxpool.Pool, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d := chi.URLParam(r, "date")
		if !reDate.MatchString(d) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "дата — ГГГГ-ММ-ДД"})
			return
		}
		b, info, err := db.GetSnapshot(r.Context(), pool, d)
		if err != nil {
			fail(w, log, "snapshot", err)
			return
		}
		if info == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "снепшота на эту дату нет"})
			return
		}
		writeJSON(w, http.StatusOK, snapshotResp{Info: info, Snapshot: b})
	}
}

// POST /api/v1/snapshots {"note": "..."} — сформировать снепшот на сегодня (кнопка на странице).
// Один на дату: повторный — 409, сохранённый не меняется.
func snapshotCreate(pool *pgxpool.Pool, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Note string `json:"note"`
		}
		if r.ContentLength != 0 {
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверный запрос"})
				return
			}
		}
		s, err := buildSnapshot(r, pool)
		if errors.Is(err, errNoJira) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		if err != nil {
			fail(w, log, "snapshot create", err)
			return
		}
		u, _ := auth.FromContext(r.Context())
		err = db.SaveSnapshot(r.Context(), pool, s.Today, s.HistEnd, s, body.Note, u.EmployeeID)
		if errors.Is(err, db.ErrSnapshotExists) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "снепшот на сегодня уже сформирован", "date": s.Today})
			return
		}
		if err != nil {
			fail(w, log, "snapshot create", err)
			return
		}
		log.Info("snapshot: сформирован", "date", s.Today, "user", u.Login)
		writeJSON(w, http.StatusCreated, map[string]string{"date": s.Today})
	}
}
