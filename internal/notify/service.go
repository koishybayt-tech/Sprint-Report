package notify

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adata/reports/internal/leadcontrol"
)

// Service — уведомления руководителям. box == nil — не задан SECRETS_KEY: работает только колокольчик.
type Service struct {
	pool     *pgxpool.Pool
	box      *Box
	jiraBase string
	log      *slog.Logger
	// Rebuild пересчитывает отчёт руководителя (после «Игнорировать» из бота), задаётся вызывающим.
	Rebuild func(ctx context.Context, lead string) error
}

func New(pool *pgxpool.Pool, secretsKey, jiraBase string, log *slog.Logger) *Service {
	box, _ := NewBox(secretsKey)
	return &Service{pool: pool, box: box, jiraBase: strings.TrimRight(jiraBase, "/"), log: log}
}

const initMarker = "_init" // в lead_notify_state: состояние руководителя уже фиксировалось

// Process — после пересчёта: новые события по каждому руководителю → колокольчик и личный бот подписчиков.
// Первый расчёт по руководителю только фиксирует состояние, чтобы не прислать разом всё накопившееся.
func (s *Service) Process(ctx context.Context, reports map[string]leadcontrol.Report) error {
	for lead, rep := range reports {
		cur, ev := Current(rep)
		prev, seen, err := s.loadState(ctx, lead)
		if err != nil {
			return err
		}
		if err := s.saveState(ctx, lead, cur); err != nil {
			return err
		}
		if !seen {
			continue
		}
		events := dropUnseen(prev, Diff(prev, cur, ev))
		if len(events) == 0 {
			continue
		}
		if err := s.deliver(ctx, lead, events); err != nil {
			return err
		}
		s.log.Info("notify: события", "lead", lead, "count", len(events))
	}
	return nil
}

func (s *Service) loadState(ctx context.Context, lead string) (State, bool, error) {
	rows, err := s.pool.Query(ctx, `SELECT kind, issue_key FROM lead_notify_state WHERE lead = $1`, lead)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	st, seen := State{}, false
	for rows.Next() {
		var k, key string
		if err := rows.Scan(&k, &key); err != nil {
			return nil, false, err
		}
		if k == initMarker {
			seen = true
			continue
		}
		if st[k] == nil {
			st[k] = map[string]bool{}
		}
		st[k][key] = true
	}
	return st, seen, rows.Err()
}

func (s *Service) saveState(ctx context.Context, lead string, cur State) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM lead_notify_state WHERE lead = $1`, lead); err != nil {
		return err
	}
	b := &pgx.Batch{}
	b.Queue(`INSERT INTO lead_notify_state (lead, kind, issue_key) VALUES ($1, $2, '')`, lead, initMarker)
	for kind, keys := range cur {
		for key := range keys {
			b.Queue(`INSERT INTO lead_notify_state (lead, kind, issue_key) VALUES ($1, $2, $3)`, lead, kind, key)
		}
	}
	if err := tx.SendBatch(ctx, b).Close(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type recipient struct {
	userID                                  int64
	onBlocked, onUnblock, onGap, onAssigned bool
	tokenEnc                                []byte
	chatID                                  *int64
}

func (r recipient) wants(kind string) bool {
	switch kind {
	case KindBlocked:
		return r.onBlocked
	case KindUnblock:
		return r.onUnblock
	case KindGap:
		return r.onGap
	case KindAssigned:
		return r.onAssigned
	}
	return true
}

// recipients — активные пользователи: admin/lead, связанные с сотрудником-руководителем, и те, кому назначены его проекты
// (lead_projects.user_id); без настроек — все события.
func (s *Service) recipients(ctx context.Context, lead string) ([]recipient, error) {
	rows, err := s.pool.Query(ctx, `SELECT u.id, coalesce(n.on_blocked, true), coalesce(n.on_unblock, true), coalesce(n.on_gap, true), coalesce(n.on_assigned, true),
			n.tg_token_enc, n.tg_chat_id
		FROM users u LEFT JOIN employees e ON e.id = u.employee_id LEFT JOIN notify_settings n ON n.user_id = u.id
		WHERE u.active AND ((e.full_name = $1 AND u.role IN ('admin', 'lead'))
			OR u.id IN (SELECT user_id FROM lead_projects WHERE lead = $1 AND user_id IS NOT NULL))`, lead)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []recipient
	for rows.Next() {
		var r recipient
		if err := rows.Scan(&r.userID, &r.onBlocked, &r.onUnblock, &r.onGap, &r.onAssigned, &r.tokenEnc, &r.chatID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Service) deliver(ctx context.Context, lead string, events []Event) error {
	rs, err := s.recipients(ctx, lead)
	if err != nil {
		return err
	}
	for _, r := range rs {
		var tg *TG
		if s.box != nil && r.tokenEnc != nil && r.chatID != nil {
			if tok, err := s.box.Open(r.tokenEnc); err == nil {
				tg = NewTG(tok)
			} else {
				s.log.Warn("notify: токен бота", "user", r.userID, "err", err)
			}
		}
		for _, e := range events {
			if !r.wants(e.Kind) {
				continue
			}
			url := s.jiraBase + "/browse/" + e.Key
			var id int64
			if err := s.pool.QueryRow(ctx, `INSERT INTO notifications (user_id, kind, lead, issue_key, text, url, create_url)
				VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`, r.userID, e.Kind, e.Lead, e.Key, e.Text, url, e.CreateURL).Scan(&id); err != nil {
				return err
			}
			if tg == nil {
				continue
			}
			btn := []Button{{Text: "Открыть в Jira", URL: url}}
			if e.Kind == KindGap {
				btn = nil
				if e.CreateURL != "" {
					btn = append(btn, Button{Text: "Создать продолжение", URL: e.CreateURL})
				}
				btn = append(btn, Button{Text: "Игнорировать", Data: "ign:" + strconv.FormatInt(id, 10)})
			}
			if err := tg.SendLinked(ctx, *r.chatID, e.Text, s.jiraBase, btn...); err != nil {
				s.log.Warn("notify: отправка в бота", "user", r.userID, "err", err)
			}
		}
	}
	return nil
}

// ---------- колокольчик ----------

type Notification struct {
	ID        int64     `json:"id"`
	Kind      string    `json:"kind"`
	Lead      string    `json:"lead"`
	IssueKey  string    `json:"issue_key"`
	Text      string    `json:"text"`
	URL       string    `json:"url"`
	CreateURL string    `json:"create_url"`
	CreatedAt time.Time `json:"created_at"`
	Read      bool      `json:"read"`
}

type List struct {
	Unread int            `json:"unread"`
	Items  []Notification `json:"items"`
}

func (s *Service) List(ctx context.Context, userID int64) (List, error) {
	l := List{Items: []Notification{}}
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`, userID).Scan(&l.Unread); err != nil {
		return l, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id, kind, lead, issue_key, text, url, create_url, created_at, read_at IS NOT NULL
		FROM notifications WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT 50`, userID)
	if err != nil {
		return l, err
	}
	defer rows.Close()
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.Kind, &n.Lead, &n.IssueKey, &n.Text, &n.URL, &n.CreateURL, &n.CreatedAt, &n.Read); err != nil {
			return l, err
		}
		l.Items = append(l.Items, n)
	}
	return l, rows.Err()
}

func (s *Service) MarkRead(ctx context.Context, userID int64, ids []int64, all bool) error {
	if all {
		_, err := s.pool.Exec(ctx, `UPDATE notifications SET read_at = now() WHERE user_id = $1 AND read_at IS NULL`, userID)
		return err
	}
	_, err := s.pool.Exec(ctx, `UPDATE notifications SET read_at = now() WHERE user_id = $1 AND id = ANY($2) AND read_at IS NULL`, userID, ids)
	return err
}

// Ignore — «продолжение не нужно»: задача скрывается из разрывов руководителя и больше не уведомляет.
func (s *Service) Ignore(ctx context.Context, lead, key string, userID int64) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO lead_gap_ignored (lead, issue_key, user_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, lead, key, userID)
	return err
}

func (s *Service) Unignore(ctx context.Context, lead, key string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM lead_gap_ignored WHERE lead = $1 AND issue_key = $2`, lead, key)
	return err
}

// ---------- настройки и личный бот ----------

type TelegramState struct {
	Available  bool   `json:"available"`
	Configured bool   `json:"configured"`
	Connected  bool   `json:"connected"`
	Bot        string `json:"bot"`
	Link       string `json:"link"`
}

type Settings struct {
	OnBlocked  bool          `json:"on_blocked"`
	OnUnblock  bool          `json:"on_unblock"`
	OnGap      bool          `json:"on_gap"`
	OnAssigned bool          `json:"on_assigned"`
	Telegram   TelegramState `json:"telegram"`
}

func (s *Service) Settings(ctx context.Context, userID int64) (Settings, error) {
	st := Settings{OnBlocked: true, OnUnblock: true, OnGap: true, OnAssigned: true, Telegram: TelegramState{Available: s.box != nil}}
	var tok []byte
	var bot, code *string
	var chat *int64
	err := s.pool.QueryRow(ctx, `SELECT on_blocked, on_unblock, on_gap, on_assigned, tg_token_enc, tg_bot, tg_chat_id, tg_link_code
		FROM notify_settings WHERE user_id = $1`, userID).Scan(&st.OnBlocked, &st.OnUnblock, &st.OnGap, &st.OnAssigned, &tok, &bot, &chat, &code)
	if errors.Is(err, pgx.ErrNoRows) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	st.Telegram.Configured = tok != nil
	st.Telegram.Connected = chat != nil
	if bot != nil {
		st.Telegram.Bot = *bot
		if code != nil && chat == nil {
			st.Telegram.Link = "https://t.me/" + *bot + "?start=" + *code
		}
	}
	return st, nil
}

func (s *Service) SaveFlags(ctx context.Context, userID int64, blocked, unblock, gap, assigned *bool) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO notify_settings (user_id, on_blocked, on_unblock, on_gap, on_assigned)
		VALUES ($1, coalesce($2, true), coalesce($3, true), coalesce($4, true), coalesce($5, true))
		ON CONFLICT (user_id) DO UPDATE SET on_blocked = coalesce($2, notify_settings.on_blocked),
		  on_unblock = coalesce($3, notify_settings.on_unblock), on_gap = coalesce($4, notify_settings.on_gap),
		  on_assigned = coalesce($5, notify_settings.on_assigned), updated_at = now()`,
		userID, blocked, unblock, gap, assigned)
	return err
}

var ErrBadToken = errors.New("Telegram не принял токен — проверьте, что скопировали его из @BotFather целиком")

// Connect проверяет токен (getMe), сохраняет его зашифрованным и выдаёт одноразовую ссылку привязки t.me/<бот>?start=<код>.
func (s *Service) Connect(ctx context.Context, userID int64, token string) error {
	if s.box == nil {
		return ErrNoKey
	}
	token = strings.TrimSpace(token)
	bot, err := NewTG(token).Me(ctx)
	if err != nil || bot == "" {
		s.log.Warn("notify: проверка токена бота", "user", userID, "err", err)
		return ErrBadToken
	}
	enc, err := s.box.Seal(token)
	if err != nil {
		return err
	}
	c := make([]byte, 8)
	if _, err := rand.Read(c); err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO notify_settings (user_id, tg_token_enc, tg_bot, tg_chat_id, tg_link_code, tg_offset)
		VALUES ($1, $2, $3, NULL, $4, 0)
		ON CONFLICT (user_id) DO UPDATE SET tg_token_enc = $2, tg_bot = $3, tg_chat_id = NULL, tg_link_code = $4, tg_offset = 0, updated_at = now()`,
		userID, enc, bot, hex.EncodeToString(c))
	return err
}

func (s *Service) Disconnect(ctx context.Context, userID int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE notify_settings SET tg_token_enc = NULL, tg_bot = NULL, tg_chat_id = NULL, tg_link_code = NULL,
		tg_offset = 0, updated_at = now() WHERE user_id = $1`, userID)
	return err
}

var ErrNotConnected = errors.New("бот ещё не привязан: откройте ссылку привязки и нажмите «Старт»")

// TestResult — куда дошло тестовое уведомление.
type TestResult struct {
	Bell     bool   `json:"bell"`
	Telegram string `json:"telegram"` // sent | not_connected | unavailable | error
	Error    string `json:"error,omitempty"`
}

// testSamples — как выглядят события каждого вида (текст как у настоящих, с пометкой «тест»).
var testSamples = map[string]string{
	KindBlocked:  "🔒 ТЕСТ-1 попала в блок · Иванов Иван · Так будет выглядеть уведомление о задаче в Blocked",
	KindUnblock:  "🔓 ТЕСТ-2 можно вытащить из блока: можно снимать блок · с сегодня · Всё, что задача ждала, уже на тесте",
	KindGap:      "➡️ ТЕСТ-3 завершилась без продолжения (нет фронта, приоритет «Высокий»): Дизайн готов: задачи на фронт нет · Пример задачи",
	KindAssigned: "👤 ТЕСТ-4 назначена на вас (было: Иванов Иван, назначил: Петров Пётр) · Так будет выглядеть уведомление, когда задачу вернули вам",
}

// Test — пример события выбранного вида (blocked | unblock | gap | assigned, пусто — общий): в колокольчик всегда,
// в личный бот — если он привязан, с теми же кнопками, что у настоящего события.
func (s *Service) Test(ctx context.Context, userID int64, kind string) (TestResult, error) {
	text, ok := testSamples[kind]
	if !ok {
		kind, text = "", "✅ Тестовое уведомление «Контроля спринта руководителей». Сюда будут приходить события по вашим проектам."
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO notifications (user_id, kind, text, url) VALUES ($1, 'test', $2, $3)`,
		userID, "Тест · "+text, s.jiraBase); err != nil {
		return TestResult{}, err
	}
	res := TestResult{Bell: true}
	tg, chat, err := s.bot(ctx, userID)
	switch {
	case errors.Is(err, ErrNoKey):
		res.Telegram = "unavailable"
		return res, nil
	case errors.Is(err, ErrNotConnected):
		res.Telegram = "not_connected"
		return res, nil
	case err != nil:
		res.Telegram, res.Error = "error", err.Error()
		return res, nil
	}
	var btn []Button
	switch kind {
	case KindBlocked, KindUnblock, KindAssigned:
		btn = []Button{{Text: "Открыть в Jira", URL: s.jiraBase}}
	case KindGap:
		btn = []Button{{Text: "Создать продолжение", URL: s.jiraBase + "/secure/CreateIssueDetails!init.jspa"}, {Text: "Игнорировать", Data: "test"}}
	}
	if err := tg.SendLinked(ctx, chat, "🧪 Тест\n"+text, s.jiraBase, btn...); err != nil {
		res.Telegram, res.Error = "error", err.Error()
		return res, nil
	}
	res.Telegram = "sent"
	return res, nil
}

func (s *Service) bot(ctx context.Context, userID int64) (*TG, int64, error) {
	if s.box == nil {
		return nil, 0, ErrNoKey
	}
	var tok []byte
	var chat *int64
	err := s.pool.QueryRow(ctx, `SELECT tg_token_enc, tg_chat_id FROM notify_settings WHERE user_id = $1`, userID).Scan(&tok, &chat)
	if errors.Is(err, pgx.ErrNoRows) || tok == nil || chat == nil {
		return nil, 0, ErrNotConnected
	}
	if err != nil {
		return nil, 0, err
	}
	t, err := s.box.Open(tok)
	if err != nil {
		return nil, 0, err
	}
	return NewTG(t), *chat, nil
}

// ---------- опрос личных ботов ----------

// RunBots раз в every забирает события личных ботов: /start <код> — привязка чата, кнопка «Игнорировать».
func (s *Service) RunBots(ctx context.Context, every time.Duration) {
	if s.box == nil {
		s.log.Warn("notify: личные Telegram-боты выключены", "reason", "нет SECRETS_KEY")
		return
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if err := s.pollOnce(ctx); err != nil && ctx.Err() == nil {
			s.log.Warn("notify: опрос ботов", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

type botRow struct {
	userID int64
	tok    []byte
	chat   *int64
	code   *string
	offset int64
}

func (s *Service) pollOnce(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `SELECT user_id, tg_token_enc, tg_chat_id, tg_link_code, tg_offset FROM notify_settings WHERE tg_token_enc IS NOT NULL`)
	if err != nil {
		return err
	}
	var bots []botRow
	for rows.Next() {
		var b botRow
		if err := rows.Scan(&b.userID, &b.tok, &b.chat, &b.code, &b.offset); err != nil {
			rows.Close()
			return err
		}
		bots = append(bots, b)
	}
	rows.Close()
	for _, b := range bots {
		tok, err := s.box.Open(b.tok)
		if err != nil {
			continue
		}
		if err := s.pollBot(ctx, NewTG(tok), b); err != nil {
			s.log.Warn("notify: бот пользователя", "user", b.userID, "err", err)
		}
	}
	return nil
}

func (s *Service) pollBot(ctx context.Context, tg *TG, b botRow) error {
	us, err := tg.Updates(ctx, b.offset)
	if err != nil {
		return err
	}
	for _, u := range us {
		b.offset = u.UpdateID + 1
		switch {
		case u.Message != nil && strings.HasPrefix(u.Message.Text, "/start"):
			arg := strings.TrimSpace(strings.TrimPrefix(u.Message.Text, "/start"))
			if b.code != nil && arg == *b.code && u.Message.Chat.Type == "private" {
				chat := u.Message.Chat.ID
				if _, err := s.pool.Exec(ctx, `UPDATE notify_settings SET tg_chat_id = $2, tg_link_code = NULL, updated_at = now() WHERE user_id = $1`,
					b.userID, chat); err != nil {
					return err
				}
				b.chat, b.code = &chat, nil
				_ = tg.Send(ctx, chat, "✅ Бот привязан. Сюда будут приходить уведомления «Контроля спринта руководителей»; какие именно — настраивается в «Отчётах Adata» → «Настройки».")
			} else if b.chat == nil || *b.chat != u.Message.Chat.ID {
				_ = tg.Send(ctx, u.Message.Chat.ID, "Этот бот присылает уведомления одному руководителю. Привязка — по ссылке из «Отчётов Adata» → «Настройки».")
			}
		case u.Callback != nil:
			s.handleCallback(ctx, tg, b, u)
		}
	}
	if len(us) > 0 {
		_, err = s.pool.Exec(ctx, `UPDATE notify_settings SET tg_offset = $2 WHERE user_id = $1`, b.userID, b.offset)
	}
	return err
}

func (s *Service) handleCallback(ctx context.Context, tg *TG, b botRow, u Update) {
	cb := u.Callback
	if cb.Data == "test" {
		_ = tg.Answer(ctx, cb.ID, "Это тест: в настоящем уведомлении задача скроется из «Разрывов»")
		return
	}
	if b.chat == nil || cb.Message == nil || cb.Message.Chat.ID != *b.chat || !strings.HasPrefix(cb.Data, "ign:") {
		_ = tg.Answer(ctx, cb.ID, "Недоступно")
		return
	}
	id, _ := strconv.ParseInt(strings.TrimPrefix(cb.Data, "ign:"), 10, 64)
	var lead, key string
	err := s.pool.QueryRow(ctx, `SELECT lead, issue_key FROM notifications WHERE id = $1 AND user_id = $2 AND kind = 'gap'`, id, b.userID).Scan(&lead, &key)
	if err != nil {
		_ = tg.Answer(ctx, cb.ID, "Уведомление не найдено")
		return
	}
	if err := s.Ignore(ctx, lead, key, b.userID); err != nil {
		_ = tg.Answer(ctx, cb.ID, "Не получилось, попробуйте в «Отчётах Adata»")
		return
	}
	_, _ = s.pool.Exec(ctx, `UPDATE notifications SET read_at = coalesce(read_at, now()) WHERE id = $1`, id)
	_ = tg.Answer(ctx, cb.ID, key+" скрыта из разрывов")
	_ = tg.SendLinked(ctx, *b.chat, fmt.Sprintf("🙈 %s скрыта из «Дизайн или бэк готов, а продолжения нет» и больше не будет присылаться. Вернуть — на странице «Контроль спринта».", key), s.jiraBase)
	if s.Rebuild != nil {
		if err := s.Rebuild(ctx, lead); err != nil {
			s.log.Warn("notify: пересчёт после «Игнорировать»", "lead", lead, "err", err)
		}
	}
}
