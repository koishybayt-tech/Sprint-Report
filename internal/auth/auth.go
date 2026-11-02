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

type User struct {
	ID          int64      `json:"id"`
	Login       string     `json:"login"`
	FullName    string     `json:"full_name"`
	Role        string     `json:"role"`
	EmployeeID  *int64     `json:"employee_id"`
	Active      bool       `json:"active"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at"`
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

const userCols = `id, login, full_name, role, employee_id, active, created_at, last_login_at`

func scanUser(r pgx.Row) (User, error) {
	var u User
	err := r.Scan(&u.ID, &u.Login, &u.FullName, &u.Role, &u.EmployeeID, &u.Active, &u.CreatedAt, &u.LastLoginAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

// Authenticate проверяет логин и пароль активного пользователя.
func (s *Store) Authenticate(ctx context.Context, login, password string) (User, error) {
	var hash string
	var u User
	err := s.pool.QueryRow(ctx, `SELECT `+userCols+`, password_hash FROM users WHERE lower(login) = lower($1) AND active`,
		strings.TrimSpace(login)).Scan(&u.ID, &u.Login, &u.FullName, &u.Role, &u.EmployeeID, &u.Active, &u.CreatedAt, &u.LastLoginAt, &hash)
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
func (s *Store) Create(ctx context.Context, login, fullName, role, password, employeeLogin string) (User, error) {
	login, fullName = strings.TrimSpace(login), strings.TrimSpace(fullName)
	if login == "" || fullName == "" {
		return User{}, errors.New("укажите логин и имя")
	}
	if !ValidRole(role) {
		return User{}, fmt.Errorf("неизвестная роль %q", role)
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	u, err := scanUser(s.pool.QueryRow(ctx, `INSERT INTO users (login, full_name, role, password_hash, employee_id)
		VALUES ($1, $2, $3, $4, (SELECT id FROM employees WHERE jira_login = nullif($5, '')))
		RETURNING `+userCols, login, fullName, role, hash, strings.TrimSpace(employeeLogin)))
	if err != nil && strings.Contains(err.Error(), "users_login_key") {
		return User{}, ErrLoginTaken
	}
	return u, err
}

// Update меняет имя, роль и активность. Деактивация закрывает все сессии пользователя.
// Нельзя оставить систему без активного администратора.
func (s *Store) Update(ctx context.Context, id int64, fullName *string, role *string, active *bool) (User, error) {
	if role != nil && !ValidRole(*role) {
		return User{}, fmt.Errorf("неизвестная роль %q", *role)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)
	u, err := scanUser(tx.QueryRow(ctx, `UPDATE users SET full_name = coalesce(nullif(trim($2), ''), full_name),
		role = coalesce($3, role), active = coalesce($4, active), updated_at = now() WHERE id = $1 RETURNING `+userCols,
		id, fullName, role, active))
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
		RETURNING u.id, u.login, u.full_name, u.role, u.employee_id, u.active, u.created_at, u.last_login_at`, tokenHash(token)))
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
