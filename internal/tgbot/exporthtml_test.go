package tgbot

import (
	"os"
	"path/filepath"
	"testing"
)

// Разметка как у Telegram Desktop (HTML-экспорт), имена выдуманы.
const htmlPage1 = `<!DOCTYPE html><html><body><div class="page_wrap">
<div class="page_header"><div class="content"><div class="text bold">
Adata Group
</div></div></div>
<div class="history">
 <div class="message service" id="message-1"><div class="body details">1 October 2026</div></div>
 <div class="message default clearfix" id="message600886">
  <div class="pull_left userpic_wrap"><div class="userpic"><div class="initials">??</div></div></div>
  <div class="body">
   <div class="pull_right date details" title="01.10.2026 08:49:17 UTC+05:00">
08:49
   </div>
   <div class="from_name">
Assan
   </div>
   <div class="text">
Алмабаев Асанали<br>Пропуск 9:00-9:40<br>Отработка сегодня
   </div>
  </div>
 </div>
 <div class="message default clearfix joined" id="message600887">
  <div class="body">
   <div class="pull_right date details" title="01.10.2026 08:50:00 UTC+05:00">08:50</div>
   <div class="reply_to details">In reply to <a href="#go_to_message600886">this message</a></div>
   <div class="text">
upd
   </div>
  </div>
 </div>
 <div class="message default clearfix" id="message600888">
  <div class="body">
   <div class="pull_right date details" title="01.10.2026 09:00:00 UTC+05:00">09:00</div>
   <div class="from_name">Julian</div>
   <div class="media_wrap clearfix"><div class="media">фото</div></div>
  </div>
 </div>
 <div class="message service" id="message600889"><div class="body details">Temirlan added AdataGroupBot</div></div>
</div></div></body></html>`

const htmlPage2 = `<html><body><div class="history">
 <div class="message default clearfix joined" id="message600890">
  <div class="body">
   <div class="pull_right date details" title="02.10.2026 10:00:00 UTC+05:00">10:00</div>
   <div class="text">Пропуск 10-11 <a href="https://t.me/lead">@lead</a></div>
  </div>
 </div>
</div></body></html>`

func TestParseExportDir(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"messages.html": htmlPage1, "messages2.html": htmlPage2, "style.css": "x"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	e, err := ParseExportDir(dir, -459179610)
	if err != nil {
		t.Fatal(err)
	}
	if e.Name != "Adata Group" || len(e.Messages) != 3 || e.Skipped != 3 {
		t.Fatalf("name=%q messages=%d skipped=%d", e.Name, len(e.Messages), e.Skipped)
	}
	a, b, c := e.Messages[0], e.Messages[1], e.Messages[2]
	if a.MessageID != 600886 || a.FromName != "Assan" || a.Text != "Алмабаев Асанали\nПропуск 9:00-9:40\nОтработка сегодня" ||
		a.SentAt.Format("2006-01-02 15:04:05") != "2026-10-01 03:49:17" || a.ChatID != -459179610 || a.Source != "export" || a.FromID != 0 {
		t.Errorf("a=%+v", a)
	}
	// «joined» берёт автора предыдущего; текст ответа — без строки «In reply to»
	if b.FromName != "Assan" || b.Text != "upd" {
		t.Errorf("b=%+v", b)
	}
	// автор переходит и в следующий файл; ссылка — её текст
	if c.MessageID != 600890 || c.FromName != "Julian" || c.Text != "Пропуск 10-11 @lead" {
		t.Errorf("c=%+v", c)
	}
	if _, err := ParseExportDir(t.TempDir(), 1); err == nil {
		t.Error("ждали ошибку для папки без messages.html")
	}
}
