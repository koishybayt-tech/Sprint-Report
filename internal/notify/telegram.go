package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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

func (t *TG) Send(ctx context.Context, chatID int64, text string, buttons ...Button) error {
	p := map[string]any{"chat_id": chatID, "text": text, "disable_web_page_preview": true}
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
	return t.call(ctx, "sendMessage", p, nil)
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
