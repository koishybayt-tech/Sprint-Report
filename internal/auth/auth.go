// Package auth — пользователи, пароли (bcrypt), сессии в cookie и проверка ролей.
// Роли и права — docs/security.md.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const (
	CookieName = "adata_session"
	SessionTTL = 14 * 24 * time.Hour
	MinPassLen = 10
	bcryptCost = 12
)

// Роли (users.role).
const (
	RoleAdmin    = "admin"
	RoleLead     = "lead"
	RoleHR       = "hr"
	RoleEmployee = "employee"
)

var Roles = []string{RoleAdmin, RoleLead, RoleHR, RoleEmployee}

func ValidRole(r string) bool {
	for _, x := range Roles {
		if x == r {
			return true
		}
	}
	return false
}

// Модули шапки (users.modules). Админ видит все всегда; остальным модуль нужен и по роли (docs/security.md), и в списке.
const (
	ModSprints     = "sprints"
	ModLeadControl = "lead_control"
	ModAttendance  = "attendance"
	ModEmployees   = "employees"
	ModTV          = "tv" // экран для ТВ (/tv): никому не выдаётся сам, только админом вручную
)

// Modules — все модули; DefaultModules — что получает пользователь, если модули не указаны (ТВ — только явно).
var Modules = []string{ModSprints, ModLeadControl, ModAttendance, ModEmployees, ModTV}
var DefaultModules = []string{ModSprints, ModLeadControl, ModAttendance, ModEmployees}

func validModule(m string) bool {
	for _, x := range Modules {
		if x == m {
			return true
		}
	}
	return false
}

// NormalizeModules проверяет список модулей, убирает повторы и приводит к порядку Modules. nil → DefaultModules.
func NormalizeModules(in []string) ([]string, error) {
	if in == nil {
		return append([]string{}, DefaultModules...), nil
	}
	seen := map[string]bool{}
	for _, m := range in {
		if !validModule(m) {
			return nil, fmt.Errorf("неизвестный модуль %q", m)
		}
		seen[m] = true
	}
	out := []string{}
	for _, m := range Modules {
		if seen[m] {
			out = append(out, m)
		}
	}
	return out, nil
}

// HasModule: админ — всегда да; остальные — по списку modules.
func (u User) HasModule(m string) bool {
	if u.Role == RoleAdmin {
		return true
	}
	for _, x := range u.Modules {
		if x == m {
			return true
		}
	}
	return false
}

type User struct {
	ID            int64      `json:"id"`
	Login         string     `json:"login"`
	FullName      string     `json:"full_name"`
	Role          string     `json:"role"`
	EmployeeID    *int64     `json:"employee_id"`
	EmployeeLogin *string    `json:"employee_login"` // логин Jira связанного сотрудника
	Modules       []string   `json:"modules"`        // у админа всегда все
	HasProjects   bool       `json:"has_projects"`   // админ назначил пользователю проекты в «Контроле спринта»
	ManagerID     *int64     `json:"manager_id"`     // руководитель пользователя (users.manager_id)
	HasTeam       bool       `json:"has_team"`       // у пользователя есть активные подчинённые
	Active        bool       `json:"active"`
	CreatedAt     time.Time  `json:"created_at"`
	LastLoginAt   *time.Time `json:"last_login_at"`
}

var (
	ErrBadCredentials = errors.New("неверный логин или пароль")
	ErrNotFound       = errors.New("пользователь не найден")
	ErrLoginTaken     = errors.New("такой логин уже есть")
	ErrWeakPassword   = fmt.Errorf("пароль должен быть не короче %d символов", MinPassLen)
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// ---------- пароли ----------

func HashPassword(p string) (string, error) {
	if len([]rune(p)) < MinPassLen {
		return "", ErrWeakPassword
	}
	h, err := bcrypt.GenerateFromPassword([]byte(p), bcryptCost)
	return string(h), err
}

// GeneratePassword — случайный пароль из букв и цифр без похожих символов (0/O, 1/l/I).
func GeneratePassword() string {
	const alphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	out := make([]byte, len(b))
	for i, x := range b {
		out[i] = alphabet[int(x)%len(alphabet)]
	}
	return string(out)
}

// фиктивный хеш, чтобы проверка несуществующего логина занимала столько же времени, сколько настоящая
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), bcryptCost)

// ---------- пользователи ----------

const userCols = `id, login, full_name, role, employee_id, active, created_at, last_login_at,
	(SELECT jira_login FROM employees e WHERE e.id = employee_id), modules,
	EXISTS (SELECT 1 FROM lead_projects lp WHERE lp.user_id = users.id), manager_id,
	EXISTS (SELECT 1 FROM users sub WHERE sub.manager_id = users.id AND sub.active)`

func scanUser(r pgx.Row) (User, error) {
	var u User
	err := r.Scan(&u.ID, &u.Login, &u.FullName, &u.Role, &u.EmployeeID, &u.Active, &u.CreatedAt, &u.LastLoginAt, &u.EmployeeLogin, &u.Modules, &u.HasProjects, &u.ManagerID, &u.HasTeam)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	u.fixModules()
	return u, err
}

// fixModules: у админа в ответе все модули, у остальных — список из базы (не nil).
func (u *User) fixModules() {
	if u.Role == RoleAdmin {
		u.Modules = append([]string{}, Modules...)
	} else if u.Modules == nil {
		u.Modules = []string{}
	}
}

// Authenticate проверяет логин и пароль активного пользователя.
func (s *Store) Authenticate(ctx context.Context, login, password string) (User, error) {
	var hash string
	var u User
	err := s.pool.QueryRow(ctx, `SELECT `+userCols+`, password_hash FROM users WHERE lower(login) = lower($1) AND active`,
		strings.TrimSpace(login)).Scan(&u.ID, &u.Login, &u.FullName, &u.Role, &u.EmployeeID, &u.Active, &u.CreatedAt, &u.LastLoginAt, &u.EmployeeLogin, &u.Modules, &u.HasProjects, &u.ManagerID, &u.HasTeam, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return User{}, ErrBadCredentials
	}
	if err != nil {
		return User{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return User{}, ErrBadCredentials
	}
	u.fixModules()
	_, err = s.pool.Exec(ctx, `UPDATE users SET last_login_at = now() WHERE id = $1`, u.ID)
	return u, err
}

func (s *Store) List(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+userCols+` FROM users ORDER BY active DESC, lower(full_name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) Get(ctx context.Context, id int64) (User, error) {
	return scanUser(s.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id))
}

// Create добавляет пользователя. employeeLogin — логин сотрудника в Jira для связи с карточкой (необязателен).
// modules — доступные модули шапки; nil — все.
// managerID — руководитель пользователя (nil — без руководителя).
func (s *Store) Create(ctx context.Context, login, fullName, role, password, employeeLogin string, modules []string, managerID *int64) (User, error) {
	login, fullName = strings.TrimSpace(login), strings.TrimSpace(fullName)
	if login == "" || fullName == "" {
		return User{}, errors.New("укажите логин и имя")
	}
	if !ValidRole(role) {
		return User{}, fmt.Errorf("неизвестная роль %q", role)
	}
	mods, err := NormalizeModules(modules)
	if err != nil {
		return User{}, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	if managerID != nil {
		if err := s.checkManager(ctx, 0, *managerID); err != nil {
			return User{}, err
		}
	}
	u, err := scanUser(s.pool.QueryRow(ctx, `INSERT INTO users (login, full_name, role, password_hash, employee_id, modules, manager_id)
		VALUES ($1, $2, $3, $4, (SELECT id FROM employees WHERE jira_login = nullif($5, '')), $6, $7)
		RETURNING `+userCols, login, fullName, role, hash, strings.TrimSpace(employeeLogin), mods, managerID))
	if err != nil && strings.Contains(err.Error(), "users_login_key") {
		return User{}, ErrLoginTaken
	}
	return u, err
}

// UserPatch — что меняется у пользователя; nil — не трогать. EmployeeLogin "" — отвязать от сотрудника.
type UserPatch struct {
	Login         *string
	FullName      *string
	Role          *string
	Active        *bool
	EmployeeLogin *string
	Modules       *[]string // nil — не трогать
	ManagerID     *int64    // nil — не трогать; 0 — убрать руководителя
}

// Ошибки назначения руководителя — тексты начинаются с «руководитель» (API отвечает 400).
var errManagerMissing = errors.New("руководитель не найден или заблокирован")
var errManagerSelf = errors.New("руководитель не может быть самим пользователем")
var errManagerCycle = errors.New("руководитель не может быть подчинённым этого пользователя")

// Subordinates — все активные подчинённые пользователя, в том числе подчинённые подчинённых.
func (s *Store) Subordinates(ctx context.Context, id int64) ([]int64, error) {
	return s.descendants(ctx, id, true)
}

// descendants обходит дерево руководитель → подчинённые; глубина ограничена на случай испорченных данных.
func (s *Store) descendants(ctx context.Context, id int64, onlyActive bool) ([]int64, error) {
	rows, err := s.pool.Query(ctx, `WITH RECURSIVE t(id, depth) AS (
		SELECT id, 1 FROM users WHERE manager_id = $1 AND (active OR NOT $2::boolean)
		UNION
		SELECT u.id, t.depth + 1 FROM users u JOIN t ON u.manager_id = t.id WHERE (u.active OR NOT $2::boolean) AND t.depth < 20
	) SELECT DISTINCT id FROM t`, id, onlyActive)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if out == nil {
		out = []int64{}
	}
	return out, err
}

// checkManager: руководитель существует и активен, это не сам пользователь и не его подчинённый (иначе получится цикл).
// userID == 0 — пользователь ещё не создан.
func (s *Store) checkManager(ctx context.Context, userID, managerID int64) error {
	var ok bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1 AND active)`, managerID).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return errManagerMissing
	}
	if userID == 0 {
		return nil
	}
	if managerID == userID {
		return errManagerSelf
	}
	subs, err := s.descendants(ctx, userID, false)
	if err != nil {
		return err
	}
	for _, x := range subs {
		if x == managerID {
			return errManagerCycle
		}
	}
	return nil
}

var ErrNoEmployee = errors.New("сотрудника с таким логином Jira нет в справочнике")

// Update меняет логин, имя, роль, активность и связь с сотрудником. Деактивация закрывает все сессии пользователя.
// Нельзя оставить систему без активного администратора.
func (s *Store) Update(ctx context.Context, id int64, p UserPatch) (User, error) {
	if p.Role != nil && !ValidRole(*p.Role) {
		return User{}, fmt.Errorf("неизвестная роль %q", *p.Role)
	}
	if p.Login != nil {
		if l := strings.TrimSpace(*p.Login); l == "" {
			return User{}, errors.New("укажите логин")
		}
	}
	if p.FullName != nil && strings.TrimSpace(*p.FullName) == "" {
		return User{}, errors.New("укажите имя")
	}
	var mods []string // nil → не менять (coalesce)
	if p.Modules != nil {
		var err error
		if mods, err = NormalizeModules(*p.Modules); err != nil {
			return User{}, err
		}
	}
	if p.ManagerID != nil && *p.ManagerID > 0 {
		if err := s.checkManager(ctx, id, *p.ManagerID); err != nil {
			return User{}, err
		}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)
	if p.EmployeeLogin != nil && strings.TrimSpace(*p.EmployeeLogin) != "" {
		var ok bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM employees WHERE jira_login = $1)`, strings.TrimSpace(*p.EmployeeLogin)).Scan(&ok); err != nil {
			return User{}, err
		}
		if !ok {
			return User{}, ErrNoEmployee
		}
	}
	u, err := scanUser(tx.QueryRow(ctx, `UPDATE users SET login = coalesce(nullif(trim($2), ''), login),
		full_name = coalesce(nullif(trim($3), ''), full_name), role = coalesce($4, role), active = coalesce($5, active),
		employee_id = CASE WHEN $6::text IS NULL THEN employee_id ELSE (SELECT id FROM employees WHERE jira_login = nullif(trim($6), '')) END,
		modules = coalesce($7::text[], modules),
		manager_id = CASE WHEN $8::bigint IS NULL THEN manager_id WHEN $8::bigint = 0 THEN NULL ELSE $8::bigint END,
		updated_at = now() WHERE id = $1 RETURNING `+userCols,
		id, p.Login, p.FullName, p.Role, p.Active, p.EmployeeLogin, mods, p.ManagerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil && strings.Contains(err.Error(), "users_login_key") {
		return User{}, ErrLoginTaken
	}
	if err != nil {
		return User{}, err
	}
	var admins int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE role = 'admin' AND active`).Scan(&admins); err != nil {
		return User{}, err
	}
	if admins == 0 {
		return User{}, errors.New("должен остаться хотя бы один активный администратор")
	}
	if !u.Active {
		if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, id); err != nil {
			return User{}, err
		}
	}
	return u, tx.Commit(ctx)
}

// SetPassword меняет пароль и закрывает все сессии пользователя, кроме keepToken (текущей, при смене своего пароля).
func (s *Store) SetPassword(ctx context.Context, id int64, password, keepToken string) error {
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1`, id, hash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	_, err = s.pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1 AND token_hash <> $2`, id, tokenHash(keepToken))
	return err
}

// CheckPassword — проверка текущего пароля перед сменой.
func (s *Store) CheckPassword(ctx context.Context, id int64, password string) error {
	var hash string
	if err := s.pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, id).Scan(&hash); err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return ErrBadCredentials
	}
	return nil
}

// ---------- сессии ----------

func tokenHash(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// NewSession создаёт сессию и возвращает токен для cookie. Попутно чистит просроченные.
func (s *Store) NewSession(ctx context.Context, userID int64) (string, time.Time, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	exp := time.Now().Add(SessionTTL)
	if _, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`); err != nil {
		return "", time.Time{}, err
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`, tokenHash(token), userID, exp)
	return token, exp, err
}

// UserBySession — активный пользователь по токену; продлевает отметку last_seen не чаще раза в минуту.
func (s *Store) UserBySession(ctx context.Context, token string) (User, error) {
	if token == "" {
		return User{}, ErrNotFound
	}
	u, err := scanUser(s.pool.QueryRow(ctx, `UPDATE sessions s SET last_seen_at = now()
		FROM users u WHERE s.token_hash = $1 AND s.expires_at > now() AND u.id = s.user_id AND u.active
		RETURNING u.id, u.login, u.full_name, u.role, u.employee_id, u.active, u.created_at, u.last_login_at,
		  (SELECT jira_login FROM employees e WHERE e.id = u.employee_id), u.modules,
		  EXISTS (SELECT 1 FROM lead_projects lp WHERE lp.user_id = u.id), u.manager_id,
		  EXISTS (SELECT 1 FROM users sub WHERE sub.manager_id = u.id AND sub.active)`, tokenHash(token)))
	return u, err
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash(token))
	return err
}

// ---------- cookie и контекст ----------

// SetCookie ставит cookie сессии. Secure — если запрос пришёл по HTTPS (в том числе через Caddy).
func SetCookie(w http.ResponseWriter, r *http.Request, token string, exp time.Time) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: token, Path: "/", Expires: exp, MaxAge: int(time.Until(exp).Seconds()),
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: isHTTPS(r)})
}

func ClearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: isHTTPS(r)})
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func Token(r *http.Request) string {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

type ctxKey struct{}

func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

func FromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
	return u, ok
}
