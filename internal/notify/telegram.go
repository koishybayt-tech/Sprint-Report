package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// TG — клиент Bot API для личного бота руководителя. Токен входит в URL, поэтому ошибки проходят через mask.
type TG struct {
	token string
	base  string
	http  *http.Client
}

func NewTG(token string) *TG {
	return &TG{token: strings.TrimSpace(token), base: "https://api.telegram.org", http: &http.Client{Timeout: 20 * time.Second}}
}

func (t *TG) mask(s string) string {
	if t.token == "" {
		return s
	}
	return strings.ReplaceAll(s, t.token, "***")
}

func (t *TG) call(ctx context.Context, method string, params any, out any) error {
	body, _ := json.Marshal(params)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.base+"/bot"+t.token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return errors.New(t.mask(err.Error()))
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.http.Do(req)
	if err != nil {
		return errors.New(t.mask(err.Error()))
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var r struct {
		OK          bool            `json:"ok"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return fmt.Errorf("telegram %s: HTTP %d, ответ не JSON", method, resp.StatusCode)
	}
	if !r.OK {
		return fmt.Errorf("telegram %s: %s", method, t.mask(r.Description))
	}
	if out != nil {
		return json.Unmarshal(r.Result, out)
	}
	return nil
}

// Me — @username бота (проверка токена).
func (t *TG) Me(ctx context.Context) (string, error) {
	var u struct {
		Username string `json:"username"`
	}
	err := t.call(ctx, "getMe", map[string]any{}, &u)
	return u.Username, err
}

// Button — кнопка под сообщением: ссылка (URL) или действие в боте (Data).
type Button struct {
	Text string
	URL  string
	Data string
}

// Send — обычный текст без разметки.
func (t *TG) Send(ctx context.Context, chatID int64, text string, buttons ...Button) error {
	_, err := t.send(ctx, chatID, text, false, buttons)
	return err
}

// SendID — то же, но возвращает id отправленного сообщения (чтобы потом при необходимости удалить).
func (t *TG) SendID(ctx context.Context, chatID int64, text string) (int64, error) {
	return t.send(ctx, chatID, text, false, nil)
}

// Delete удаляет сообщение бота (Telegram позволяет в течение 48 часов).
func (t *TG) Delete(ctx context.Context, chatID, messageID int64) error {
	return t.call(ctx, "deleteMessage", map[string]any{"chat_id": chatID, "message_id": messageID}, nil)
}

// SendLinked — то же, но номера задач Jira (AC-123) в тексте становятся ссылками на jiraBase/browse/KEY.
func (t *TG) SendLinked(ctx context.Context, chatID int64, text, jiraBase string, buttons ...Button) error {
	_, err := t.send(ctx, chatID, LinkKeys(text, jiraBase), true, buttons)
	return err
}

var reIssueKey = regexp.MustCompile(`\b[A-Z][A-Z0-9_]*-\d+\b`)

// LinkKeys экранирует текст для Telegram (parse_mode HTML) и оборачивает ключи задач в ссылки на Jira.
func LinkKeys(text, jiraBase string) string {
	base := strings.TrimRight(jiraBase, "/")
	return reIssueKey.ReplaceAllStringFunc(html.EscapeString(text), func(key string) string {
		return `<a href="` + html.EscapeString(base+"/browse/"+key) + `">` + key + `</a>`
	})
}

func (t *TG) send(ctx context.Context, chatID int64, text string, asHTML bool, buttons []Button) (int64, error) {
	p := map[string]any{"chat_id": chatID, "text": text, "disable_web_page_preview": true}
	if asHTML {
		p["parse_mode"] = "HTML"
	}
	if len(buttons) > 0 {
		row := make([]map[string]string, 0, len(buttons))
		for _, b := range buttons {
			if b.URL != "" {
				row = append(row, map[string]string{"text": b.Text, "url": b.URL})
			} else {
				row = append(row, map[string]string{"text": b.Text, "callback_data": b.Data})
			}
		}
		p["reply_markup"] = map[string]any{"inline_keyboard": [][]map[string]string{row}}
	}
	var sent struct {
		MessageID int64 `json:"message_id"`
	}
	err := t.call(ctx, "sendMessage", p, &sent)
	return sent.MessageID, err
}

// Update — то, что нужно от личного бота: /start <код> и нажатия кнопок.
type Update struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		Chat struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		} `json:"chat"`
		Text string `json:"text"`
	} `json:"message"`
	Callback *struct {
		ID      string `json:"id"`
		Data    string `json:"data"`
		Message *struct {
			Chat struct {
				ID int64 `json:"id"`
			} `json:"chat"`
		} `json:"message"`
	} `json:"callback_query"`
}

// Updates — новые события бота без ожидания (опрос раз в несколько секунд для всех ботов).
func (t *TG) Updates(ctx context.Context, offset int64) ([]Update, error) {
	var us []Update
	err := t.call(ctx, "getUpdates", map[string]any{"offset": offset, "timeout": 0, "allowed_updates": []string{"message", "callback_query"}}, &us)
	return us, err
}

func (t *TG) Answer(ctx context.Context, callbackID, text string) error {
	return t.call(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": callbackID, "text": text}, nil)
}
