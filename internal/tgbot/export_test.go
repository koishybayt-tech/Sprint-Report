package tgbot

import (
	"strings"
	"testing"
)

func TestParseExport(t *testing.T) {
	// Структура как у Telegram Desktop; ФИО и id выдуманы.
	js := `{"name":"Adata Group","type":"private_group","id":459179610,"messages":[
	 {"id":10,"type":"service","date":"2026-11-02T09:00:00","date_unixtime":"1793592000","actor":"Temirlan","action":"invite_members"},
	 {"id":11,"type":"message","date":"2026-11-02T09:05:00","date_unixtime":"1793592300","from":"Assan","from_id":"user111",
	  "text":"Алмабаев Асанали\nПропуск 9:00-9:20","text_entities":[]},
	 {"id":12,"type":"message","date_unixtime":"1793592400","edited_unixtime":"1793592500","from":"Julian","from_id":"user222",
	  "text":["Пропуск 10-11 ",{"type":"mention","text":"@lead"}," согласовано"]},
	 {"id":13,"type":"message","date_unixtime":"1793592600","from":"Новости","from_id":"channel333","text":"репост"}
	]}`
	e, err := ParseExport(strings.NewReader(js))
	if err != nil {
		t.Fatal(err)
	}
	if e.ChatID != -459179610 || len(e.Messages) != 2 || e.Skipped != 2 {
		t.Fatalf("chat=%d messages=%d skipped=%d", e.ChatID, len(e.Messages), e.Skipped)
	}
	a, b := e.Messages[0], e.Messages[1]
	if a.FromID != 111 || a.MessageID != 11 || a.Source != "export" || a.SentAt.Unix() != 1793592300 || a.EditedAt != nil {
		t.Errorf("a=%+v", a)
	}
	if b.Text != "Пропуск 10-11 @lead согласовано" || b.EditedAt == nil || b.EditedAt.Unix() != 1793592500 {
		t.Errorf("b=%+v", b)
	}
	if botChatID("private_supergroup", 1234567890) != -1001234567890 {
		t.Errorf("supergroup id: %d", botChatID("private_supergroup", 1234567890))
	}
	if _, err := ParseExport(strings.NewReader(`{"messages":[]}`)); err == nil {
		t.Error("ждали ошибку без id чата")
	}
}
