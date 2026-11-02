package tgbot

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// Справочник из seed/ так же, как его собирает `reports seed` (employees.csv + tg_aliases.csv).
// names: id → все ФИО сотрудника (дубли строк с одним логином — один человек, как в seed).
func seedDirectory(t *testing.T) (*Directory, map[int64][]string) {
	t.Helper()
	read := func(p string) []map[string]string {
		f, err := os.Open(p)
		if err != nil {
			t.Skipf("нет %s: %v", p, err)
		}
		defer f.Close()
		recs, err := csv.NewReader(f).ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		var out []map[string]string
		for _, r := range recs[1:] {
			m := map[string]string{}
			for i, h := range recs[0] {
				if i < len(r) {
					m[strings.TrimPrefix(h, "\uFEFF")] = strings.TrimSpace(r[i])
				}
			}
			out = append(out, m)
		}
		return out
	}
	byLogin, byName, names := map[string]*Person{}, map[string]*Person{}, map[int64][]string{}
	var people []*Person
	for i, e := range read("../../seed/employees.csv") {
		if p := byLogin[e["jira_login"]]; p != nil && e["jira_login"] != "" {
			names[p.EmployeeID] = append(names[p.EmployeeID], e["full_name"]) // дубль логина — тот же человек, как в seed
			continue
		}
		p := &Person{EmployeeID: int64(i + 1), FullName: e["full_name"]}
		people = append(people, p)
		names[p.EmployeeID] = []string{p.FullName}
		if e["jira_login"] != "" {
			byLogin[e["jira_login"]] = p
		}
		byName[e["full_name"]] = p
	}
	for _, a := range read("../../seed/tg_aliases.csv") {
		p := byLogin[a["jira_login"]]
		if a["jira_login"] == "" {
			p = byName[a["jira_name"]]
			if p == nil {
				p = byName[a["full_name"]]
			}
		}
		if p == nil {
			continue
		}
		p.Displays = append(p.Displays, a["tg_display_name"])
		if a["full_name"] != "" {
			p.FIOs = append(p.FIOs, a["full_name"])
		}
	}
	var ps []Person
	for _, p := range people {
		ps = append(ps, *p)
	}
	return NewDirectory(ps), names
}

// TestResolveCorpus: на размеченном корпусе автор находится верно и почти никогда — ошибочно.
func TestResolveCorpus(t *testing.T) {
	dir, names := seedDirectory(t)
	f, err := os.Open("../../testdata/telegram_corpus.jsonl")
	if err != nil {
		t.Skip("нет корпуса")
	}
	defer f.Close()
	var total, ok, wrong, missing int
	how := map[How]int{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var m struct{ From, Employee, Text string }
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatal(err)
		}
		if m.Employee == "" || m.Employee == "Ермухан Равиль" { // нет в справочнике сотрудников
			continue
		}
		total++
		id, h := dir.Resolve(0, "", m.From, m.Text)
		how[h]++
		switch {
		case id == 0:
			missing++
		case contains(names[id], m.Employee):
			ok++
		default:
			wrong++
			if wrong <= 5 {
				t.Logf("ошибка: from=%q → %q, ждали %q; текст %q", m.From, names[id][0], m.Employee, firstLine(m.Text))
			}
		}
	}
	t.Logf("корпус: %d сообщений; верно %.1f%%, не найдено %.1f%%, ошибок %.2f%%; по признаку %v",
		total, pct(ok, total), pct(missing, total), pct(wrong, total), how)
	if pct(ok, total) < 97 || pct(wrong, total) > 0.5 {
		t.Errorf("точность сопоставления ниже порога: верно %.1f%%, ошибок %.2f%%", pct(ok, total), pct(wrong, total))
	}
}

func TestResolveOrder(t *testing.T) {
	d := NewDirectory([]Person{
		{EmployeeID: 1, FullName: "Алмабаев Асанәлі", Displays: []string{"Assan"}, UserIDs: []int64{111}},
		{EmployeeID: 2, FullName: "Халел Санжар", Displays: []string{"Санжар"}},
		{EmployeeID: 3, FullName: "Нұрғали Санжар", Displays: []string{"Санжар"}, Usernames: []string{"snurgali"}},
		{EmployeeID: 4, FullName: "Муратбай Әбілмансур"},
	})
	cases := []struct {
		uid              int64
		user, disp, text string
		want             int64
		how              How
	}{
		{111, "", "кто угодно", "Халел Санжар\nПропуск 9-10", 1, ByUserID},
		{0, "@SNurgali", "", "", 3, ByUsername},
		{0, "", "Assan", "Санжар Халел\nПропуск 9:00-10:00", 2, ByFIO}, // ФИО важнее имени аккаунта, порядок слов не важен
		{0, "", "Assan", "Алмабаев Асанали Ерланович\nПропуск 9-10", 1, ByFIO},
		{0, "", "?", "Муратбай Абильмансур\nПропуск 9-10", 4, ByFIO}, // мягкий знак и казахские буквы не мешают
		{0, "", "Assan", "Пропуск 9-10", 1, ByDisplay},
		{0, "", "  ASSAN 🏴‍☠️", "upd", 1, ByDisplay},   // эмодзи и регистр в имени аккаунта не мешают
		{0, "", "Санжар", "Пропуск 9-10", 0, NotFound}, // два «Санжара» — не угадываем
	}
	for _, c := range cases {
		id, h := d.Resolve(c.uid, c.user, c.disp, c.text)
		if id != c.want || h != c.how {
			t.Errorf("Resolve(%d,%q,%q,%q) = %d,%s; ждали %d,%s", c.uid, c.user, c.disp, firstLine(c.text), id, h, c.want, c.how)
		}
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func firstLine(s string) string { l, _, _ := strings.Cut(s, "\n"); return l }

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b) * 100
}
