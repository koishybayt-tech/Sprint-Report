// Package tgbot — сообщения группы «Adata Group»: приём ботом и импорт экспорта Telegram Desktop,
// сопоставление автора с сотрудником, разбор tgparse и запись событий посещаемости.
package tgbot

import (
	"sort"
	"strings"
	"unicode"
)

// Person — сотрудник и все его известные признаки в Telegram.
type Person struct {
	EmployeeID int64
	FullName   string   // ФИО из справочника (как в Jira)
	FIOs       []string // как подписывается в сообщениях (tg_aliases.fio)
	Displays   []string // имена аккаунта Telegram (tg_aliases.tg_display)
	UserIDs    []int64
	Usernames  []string
}

// Directory — справочник для поиска автора сообщения (docs/telegram-parsing.md, п. 4).
type Directory struct {
	byUserID   map[int64]int64
	byUsername map[string]int64
	byDisplay  map[string][]int64
	names      []nameKey
}

type nameKey struct {
	words []string // нормализованные слова ФИО, по алфавиту
	id    int64
}

func NewDirectory(people []Person) *Directory {
	d := &Directory{byUserID: map[int64]int64{}, byUsername: map[string]int64{}, byDisplay: map[string][]int64{}}
	for _, p := range people {
		for _, id := range p.UserIDs {
			d.byUserID[id] = p.EmployeeID
		}
		for _, u := range p.Usernames {
			d.byUsername[strings.ToLower(strings.TrimPrefix(u, "@"))] = p.EmployeeID
		}
		for _, s := range p.Displays {
			k := normDisplay(s)
			if k != "" && !containsID(d.byDisplay[k], p.EmployeeID) {
				d.byDisplay[k] = append(d.byDisplay[k], p.EmployeeID)
			}
		}
		for _, f := range append([]string{p.FullName}, p.FIOs...) {
			if w := nameWords(f); len(w) >= 2 {
				d.names = append(d.names, nameKey{w, p.EmployeeID})
			}
		}
	}
	return d
}

// How — каким признаком найден автор.
type How string

const (
	ByUserID   How = "user_id"
	ByUsername How = "username"
	ByFIO      How = "fio"
	ByDisplay  How = "display"
	NotFound   How = ""
)

// Resolve ищет сотрудника: Telegram ID → @username → ФИО в первых строках сообщения → имя аккаунта (если однозначно).
// ФИО важнее имени аккаунта: аккаунт могут переименовать, а подпись в сообщении — нет.
func (d *Directory) Resolve(userID int64, username, display, text string) (int64, How) {
	if id, ok := d.byUserID[userID]; ok && userID != 0 {
		return id, ByUserID
	}
	if id, ok := d.byUsername[strings.ToLower(strings.TrimPrefix(username, "@"))]; ok && username != "" {
		return id, ByUsername
	}
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if i >= 2 { // ФИО пишут первой строкой, иногда после «Пропуск …»
			break
		}
		if id := d.byName(l); id != 0 {
			return id, ByFIO
		}
	}
	if ids := d.byDisplay[normDisplay(display)]; len(ids) == 1 {
		return ids[0], ByDisplay
	}
	return 0, NotFound
}

// byName: все слова ФИО сотрудника есть в строке (отчество и порядок слов не важны); подходит ровно один сотрудник.
func (d *Directory) byName(line string) int64 {
	w := nameWords(line)
	if len(w) < 2 || len(w) > 5 {
		return 0
	}
	set := map[string]bool{}
	for _, x := range w {
		set[x] = true
	}
	var found int64
	for _, k := range d.names {
		all := true
		for _, x := range k.words {
			if !set[x] {
				all = false
				break
			}
		}
		if !all {
			continue
		}
		if found != 0 && found != k.id {
			return 0 // неоднозначно
		}
		found = k.id
	}
	return found
}

// Казахские и редкие буквы → ближайшие русские: «Асанәлі» и «Асанали» — одно имя.
var foldRunes = map[rune]rune{'ә': 'а', 'і': 'и', 'ү': 'у', 'ұ': 'у', 'қ': 'к', 'ғ': 'г', 'ң': 'н', 'ө': 'о', 'һ': 'х', 'ё': 'е', 'й': 'и'}

func nameWords(s string) []string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r == 'ь' || r == 'ъ' {
			continue // «Абильмансур» и «Әбілмансур» — одно имя
		}
		if f, ok := foldRunes[r]; ok {
			r = f
		}
		switch {
		case unicode.IsLetter(r):
			b.WriteRune(r)
		default:
			b.WriteRune(' ')
		}
	}
	w := strings.Fields(b.String())
	sort.Strings(w)
	return w
}

// normDisplay: имя аккаунта без эмодзи и значков — «Алек» и «Алек🏴‍☠️» одно имя (люди украшают ник, а не меняют его).
func normDisplay(s string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), " ")
}

func containsID(xs []int64, id int64) bool {
	for _, x := range xs {
		if x == id {
			return true
		}
	}
	return false
}
