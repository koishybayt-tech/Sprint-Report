package tgbot

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// HTML-экспорт Telegram Desktop (формат по умолчанию): messages.html, messages2.html, …
// В нём нет Telegram ID авторов и id чата — автор ищется по ФИО в тексте и имени аккаунта, чат задаётся снаружи.
// Номера сообщений те же, что в JSON-экспорте того же аккаунта, поэтому источник — тоже 'export'.

var reMsgFile = regexp.MustCompile(`^messages(\d*)\.html$`)

// ParseExportDir читает все messages*.html папки экспорта по порядку.
func ParseExportDir(dir string, chatID int64) (Export, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Export{}, err
	}
	type file struct {
		n    int
		name string
	}
	var files []file
	for _, e := range entries {
		if m := reMsgFile.FindStringSubmatch(e.Name()); m != nil {
			n := 1
			if m[1] != "" {
				n, _ = strconv.Atoi(m[1])
			}
			files = append(files, file{n, e.Name()})
		}
	}
	if len(files) == 0 {
		return Export{}, fmt.Errorf("в %s нет messages.html — это папка экспорта Telegram Desktop?", dir)
	}
	sort.Slice(files, func(a, b int) bool { return files[a].n < files[b].n })
	out := Export{Type: "html", ChatID: chatID}
	last := "" // автор предыдущего сообщения: у «joined» имени нет, и цепочка продолжается в следующем файле
	for _, f := range files {
		fh, err := os.Open(filepath.Join(dir, f.name))
		if err != nil {
			return out, err
		}
		e, err := parseHTML(fh, chatID, &last)
		fh.Close()
		if err != nil {
			return out, fmt.Errorf("%s: %w", f.name, err)
		}
		if out.Name == "" {
			out.Name = e.Name
		}
		out.Messages = append(out.Messages, e.Messages...)
		out.Skipped += e.Skipped
	}
	return out, nil
}

func parseHTML(r io.Reader, chatID int64, last *string) (Export, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return Export{}, err
	}
	var e Export
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			cls := attr(n, "class")
			switch {
			case e.Name == "" && hasClass(cls, "page_header"):
				if t := findClass(n, "text"); t != nil {
					e.Name = strings.TrimSpace(textOf(t, false))
				}
			case hasClass(cls, "message") && strings.HasPrefix(attr(n, "id"), "message"):
				id, _ := strconv.ParseInt(strings.TrimPrefix(attr(n, "id"), "message"), 10, 64)
				if !hasClass(cls, "default") || id <= 0 {
					e.Skipped++ // служебные: «добавил», «закрепил», даты-разделители
					return
				}
				m, ok, err := htmlMessage(n, id, chatID, last)
				if err != nil {
					e.Skipped++
					return
				}
				if ok {
					e.Messages = append(e.Messages, m)
				} else {
					e.Skipped++
				}
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return e, nil
}

func htmlMessage(n *html.Node, id, chatID int64, last *string) (Message, bool, error) {
	body := findClass(n, "body")
	if body == nil {
		return Message{}, false, nil
	}
	var date, from, text string
	var hasText bool
	// Берём только прямых детей body: внутри ответов и пересылок бывают свои from_name/text.
	for c := body.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || c.Data != "div" {
			continue
		}
		cls := attr(c, "class")
		switch {
		case hasClass(cls, "date") && hasClass(cls, "details"):
			date = attr(c, "title")
		case hasClass(cls, "from_name"):
			from = strings.TrimSpace(textOf(c, false))
		case hasClass(cls, "text"):
			text, hasText = strings.TrimSpace(textOf(c, true)), true
		}
	}
	if from == "" {
		from = *last // «joined»: то же имя, что у предыдущего
	} else {
		*last = from
	}
	if !hasText || text == "" {
		return Message{}, false, nil // фото/файл без подписи
	}
	sent, err := time.Parse("02.01.2006 15:04:05 UTC-07:00", date)
	if err != nil {
		return Message{}, false, fmt.Errorf("сообщение %d: дата %q", id, date)
	}
	return Message{ChatID: chatID, Source: "export", MessageID: id, FromName: from, SentAt: sent.UTC(), Text: text,
		Raw: []byte(fmt.Sprintf(`{"format":"html","id":%d,"date":%q,"from":%q,"text":%q}`, id, date, from, text))}, true, nil
}

// textOf собирает текст; br → перевод строки (если lines).
func textOf(n *html.Node, lines bool) string {
	var b strings.Builder
	var rec func(*html.Node)
	rec = func(n *html.Node) {
		switch {
		case n.Type == html.TextNode:
			b.WriteString(n.Data)
		case n.Type == html.ElementNode && n.Data == "br" && lines:
			b.WriteString("\n")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			rec(c)
		}
	}
	rec(n)
	s := b.String()
	if !lines {
		return strings.Join(strings.Fields(s), " ")
	}
	// Переносы строк в тексте сообщения — это <br>; переносы исходника HTML бывают только по краям, их срежет TrimSpace.
	return s
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasClass(cls, c string) bool {
	for _, x := range strings.Fields(cls) {
		if x == c {
			return true
		}
	}
	return false
}

func findClass(n *html.Node, c string) *html.Node {
	for x := n.FirstChild; x != nil; x = x.NextSibling {
		if x.Type == html.ElementNode && hasClass(attr(x, "class"), c) {
			return x
		}
		if f := findClass(x, c); f != nil {
			return f
		}
	}
	return nil
}
