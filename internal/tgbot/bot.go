package tgbot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Bot — приём сообщений группы длинным опросом getUpdates (локально и на сервере без публичного вебхука).
// Токен входит в URL запроса, поэтому все ошибки проходят через mask.
type Bot struct {
	token string
	http  *http.Client
	chats map[int64]bool // группы, сообщения которых обрабатываем
	store *Store
	log   *slog.Logger
}

func NewBot(token string, chatID int64, store *Store, log *slog.Logger) (*Bot, error) {
	if token == "" {
		return nil, errors.New("TG_BOT_TOKEN не задан")
	}
	if chatID == 0 {
		return nil, errors.New("TG_CHAT_ID не задан")
	}
	return &Bot{token: token, http: &http.Client{Timeout: 70 * time.Second}, chats: map[int64]bool{chatID: true}, store: store, log: log}, nil
}

func (b *Bot) mask(s string) string { return strings.ReplaceAll(s, b.token, "***") }

type tgUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

type tgMessage struct {
	MessageID int64  `json:"message_id"`
	From      tgUser `json:"from"`
	Chat      struct {
		ID    int64  `json:"id"`
		Title string `json:"title"`
	} `json:"chat"`
	Date            int64  `json:"date"`
	EditDate        int64  `json:"edit_date"`
	Text            string `json:"text"`
	Caption         string `json:"caption"`
	MigrateToChatID int64  `json:"migrate_to_chat_id"`
}

type update struct {
	UpdateID      int64           `json:"update_id"`
	Message       json.RawMessage `json:"message"`
	EditedMessage json.RawMessage `json:"edited_message"`
}

func (b *Bot) getUpdates(ctx context.Context, offset int64) ([]update, error) {
	q := url.Values{"timeout": {"50"}, "allowed_updates": {`["message","edited_message"]`}}
	if offset != 0 {
		q.Set("offset", strconv.FormatInt(offset, 10))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.telegram.org/bot"+b.token+"/getUpdates?"+q.Encode(), nil)
	if err != nil {
		return nil, errors.New(b.mask(err.Error()))
	}
	resp, err := b.http.Do(req)
	if err != nil {
		return nil, errors.New(b.mask(err.Error()))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	var r struct {
		OK          bool     `json:"ok"`
		ErrorCode   int      `json:"error_code"`
		Description string   `json:"description"`
		Result      []update `json:"result"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("telegram: HTTP %d, ответ не JSON", resp.StatusCode)
	}
	if !r.OK {
		return nil, fmt.Errorf("telegram: %d %s", r.ErrorCode, b.mask(r.Description))
	}
	return r.Result, nil
}

// Run читает обновления, пока не отменён ctx. Обновления подтверждаются (offset) только после записи в БД,
// поэтому при сбое они придут снова (Telegram хранит их 24 часа), а повторная обработка безопасна.
func (b *Bot) Run(ctx context.Context) {
	b.log.Info("tg bot: приём сообщений запущен")
	var offset int64
	backoff := time.Second
	reload := time.NewTicker(time.Hour)
	defer reload.Stop()
	for ctx.Err() == nil {
		select {
		case <-reload.C:
			if err := b.store.Reload(ctx); err != nil {
				b.log.Warn("tg bot: справочник авторов", "err", err)
			}
		default:
		}
		ups, err := b.getUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			b.log.Warn("tg bot: getUpdates", "err", err.Error(), "retry_in", backoff.String())
			select {
			case <-ctx.Done():
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, 5*time.Minute)
			continue
		}
		failed := false
		for _, u := range ups {
			if err := b.handle(ctx, u); err != nil {
				b.log.Error("tg bot: сообщение не записано, повторим", "update", u.UpdateID, "err", err, "retry_in", backoff.String())
				failed = true
				break // offset не двигаем — обновление придёт снова
			}
			offset = u.UpdateID + 1
		}
		if failed {
			// Без паузы getUpdates сразу вернёт то же обновление, и повторы пойдут без остановки.
			select {
			case <-ctx.Done():
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, 5*time.Minute)
		} else {
			backoff = time.Second
		}
	}
	b.log.Info("tg bot: остановка")
}

func (b *Bot) handle(ctx context.Context, u update) error {
	raw := u.Message
	if len(raw) == 0 {
		raw = u.EditedMessage
	}
	if len(raw) == 0 {
		return nil
	}
	var m tgMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil // не сообщение — пропускаем
	}
	if !b.chats[m.Chat.ID] {
		return nil // личка боту и чужие чаты не храним
	}
	if m.MigrateToChatID != 0 {
		// Группу превратили в супергруппу (например, при назначении админа) — дальше сообщения придут с нового id.
		b.chats[m.MigrateToChatID] = true
		b.log.Warn("tg bot: группа стала супергруппой — обновите TG_CHAT_ID в .env", "old", m.Chat.ID, "new", m.MigrateToChatID)
		return nil
	}
	text := m.Text
	if text == "" {
		text = m.Caption
	}
	if m.From.IsBot || text == "" {
		return nil
	}
	msg := Message{ChatID: m.Chat.ID, Source: "bot", MessageID: m.MessageID, FromID: m.From.ID,
		FromName: strings.TrimSpace(m.From.FirstName + " " + m.From.LastName), Username: m.From.Username,
		SentAt: time.Unix(m.Date, 0).UTC(), Text: text, Raw: raw}
	if m.EditDate != 0 {
		t := time.Unix(m.EditDate, 0).UTC()
		msg.EditedAt = &t
	}
	out, err := b.store.Ingest(ctx, msg)
	if err != nil {
		return err
	}
	b.log.Info("tg bot: сообщение", "message_id", m.MessageID, "edited", m.EditDate != 0, "type", out.Type,
		"status", out.Status, "author_found", out.EmployeeID != 0, "by", out.How, "events", out.Events)
	return nil
}
