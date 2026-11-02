package tgbot

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Export — история чата из Telegram Desktop (⋮ → Export chat history → JSON).
type Export struct {
	Name     string
	Type     string // private_group | private_supergroup | public_supergroup | …
	ChatID   int64  // как в Bot API: -id для обычной группы, -100id для супергруппы
	Messages []Message
	Skipped  int // служебные сообщения и сообщения не от пользователей
}

type exportMsg struct {
	ID         int64           `json:"id"`
	Type       string          `json:"type"`
	DateUnix   string          `json:"date_unixtime"`
	EditedUnix string          `json:"edited_unixtime"`
	From       string          `json:"from"`
	FromID     string          `json:"from_id"`
	Text       json.RawMessage `json:"text"`
}

// ParseExport читает result.json. Время берётся из *_unixtime (поле date — в поясе компьютера, где делали экспорт).
func ParseExport(r io.Reader) (Export, error) {
	var f struct {
		Name     string            `json:"name"`
		Type     string            `json:"type"`
		ID       int64             `json:"id"`
		Messages []json.RawMessage `json:"messages"`
	}
	if err := json.NewDecoder(r).Decode(&f); err != nil {
		return Export{}, fmt.Errorf("это не экспорт Telegram в JSON: %w", err)
	}
	if f.ID == 0 || f.Messages == nil {
		return Export{}, fmt.Errorf("в файле нет id чата или сообщений — нужен экспорт одного чата (result.json)")
	}
	e := Export{Name: f.Name, Type: f.Type, ChatID: botChatID(f.Type, f.ID)}
	for _, raw := range f.Messages {
		var m exportMsg
		if err := json.Unmarshal(raw, &m); err != nil {
			return e, fmt.Errorf("сообщение: %w", err)
		}
		uid, isUser := strings.CutPrefix(m.FromID, "user")
		if m.Type != "message" || !isUser {
			e.Skipped++
			continue
		}
		fromID, _ := strconv.ParseInt(uid, 10, 64)
		sec, err := strconv.ParseInt(m.DateUnix, 10, 64)
		if err != nil {
			return e, fmt.Errorf("сообщение %d: нет date_unixtime", m.ID)
		}
		msg := Message{ChatID: e.ChatID, Source: "export", MessageID: m.ID, FromID: fromID, FromName: m.From,
			SentAt: time.Unix(sec, 0).UTC(), Text: flattenText(m.Text), Raw: raw}
		if s, err := strconv.ParseInt(m.EditedUnix, 10, 64); err == nil && s > 0 {
			t := time.Unix(s, 0).UTC()
			msg.EditedAt = &t
		}
		e.Messages = append(e.Messages, msg)
	}
	return e, nil
}

// botChatID: в экспорте id без знака; Bot API даёт -id обычной группе и -100… супергруппе.
func botChatID(typ string, id int64) int64 {
	if strings.Contains(typ, "supergroup") || strings.Contains(typ, "channel") {
		return -1_000_000_000_000 - id
	}
	return -id
}

// flattenText: text — строка или массив из строк и объектов {type, text} (ссылки, упоминания, жирный…).
func flattenText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []json.RawMessage
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var b strings.Builder
	for _, p := range parts {
		var str string
		if json.Unmarshal(p, &str) == nil {
			b.WriteString(str)
			continue
		}
		var ent struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(p, &ent) == nil {
			b.WriteString(ent.Text)
		}
	}
	return b.String()
}
