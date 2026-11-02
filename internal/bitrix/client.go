// Package bitrix — клиент собственного API отпусков Bitrix:
// GET $BITRIX_VACATION_URL?id=<табельный ID>&api_key=<ключ>.
//
// Ключ идёт в query, поэтому всё, что может попасть в лог (ошибки, URL, тело ответа), проходит через Mask.
package bitrix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type Client struct {
	base string
	key  string
	http *http.Client
}

func New(vacationURL, apiKey string) (*Client, error) {
	if apiKey == "" {
		return nil, errors.New("BITRIX_API_KEY не задан")
	}
	if vacationURL == "" {
		return nil, errors.New("BITRIX_VACATION_URL не задан")
	}
	return &Client{base: vacationURL, key: apiKey, http: &http.Client{Timeout: 30 * time.Second}}, nil
}

var apiKeyRe = regexp.MustCompile(`(?i)(api_key=)[^&\s"']+`)

// Mask убирает ключ из строки: и по значению, и любой api_key=… в URL.
func (c *Client) Mask(s string) string {
	if c.key != "" {
		s = strings.ReplaceAll(s, c.key, "***")
		s = strings.ReplaceAll(s, url.QueryEscape(c.key), "***")
	}
	return apiKeyRe.ReplaceAllString(s, "${1}***")
}

// Response — сырой ответ API (тело уже без ключа).
type Response struct {
	Status      int
	ContentType string
	Body        []byte
}

// Fetch запрашивает отпуска сотрудника по табельному ID. Ошибки и тело ответа маскируются.
func (c *Client) Fetch(ctx context.Context, tabID string) (Response, error) {
	q := url.Values{"id": {tabID}, "api_key": {c.key}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"?"+q.Encode(), nil)
	if err != nil {
		return Response{}, errors.New(c.Mask(err.Error()))
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		// *url.Error содержит полный URL с ключом — отдаём только замаскированный текст.
		return Response{}, fmt.Errorf("bitrix id=%s: %s", tabID, c.Mask(err.Error()))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return Response{}, fmt.Errorf("bitrix id=%s: чтение ответа: %s", tabID, c.Mask(err.Error()))
	}
	r := Response{Status: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), Body: []byte(c.Mask(string(body)))}
	if resp.StatusCode != http.StatusOK {
		msg := string(r.Body)
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(r.Body, &e) == nil && e.Error != "" {
			msg = e.Error
		}
		return r, &APIError{TabID: tabID, Status: resp.StatusCode, Message: truncate(msg, 300)}
	}
	return r, nil
}

// APIError — ответ API не 200. Текст уже замаскирован.
type APIError struct {
	TabID   string
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("bitrix id=%s: HTTP %d: %s", e.TabID, e.Status, e.Message)
}

// NoVacations: API отвечает 404 «Not Found: отпуска для данного сотрудника не найдены», когда заявлений нет вовсе.
func NoVacations(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.Status == http.StatusNotFound && strings.HasPrefix(e.Message, "Not Found")
}

func truncate(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return string(r)
}
