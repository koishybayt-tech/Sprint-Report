package api

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

var testNow = time.Date(2026, 11, 4, 10, 30, 0, 0, time.FixedZone("ALA", 5*3600))

func reopen(t *testing.T, req exportReq) *excelize.File {
	t.Helper()
	f, err := buildLeadXLSX(req, "https://jira.example/", testNow)
	if err != nil {
		t.Fatalf("buildLeadXLSX: %v", err)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	g, err := excelize.OpenReader(&buf)
	if err != nil {
		t.Fatalf("файл не открывается: %v", err)
	}
	t.Cleanup(func() { g.Close() })
	return g
}

func TestBuildLeadXLSX(t *testing.T) {
	k := 0
	g := reopen(t, exportReq{
		Title: "Долго на тесте", Lead: "Иванов Иван", Project: "Все проекты", Filters: []string{"Исполнитель: Петров", "Тестировщик: Тестов"}, Updated: "04.11.2026 10:00",
		Columns: []string{"Задача", "Исполнитель", "Дн. на тесте", "Название"}, KeyCol: &k,
		Rows: [][]any{
			{"AC-12", "Петров", float64(7), "Проверить вход"},
			{"EDO-5", "Петров", float64(3), "=1+1"}, // формула-инъекция должна остаться текстом
			{"AC-13", nil, float64(2.5), "без исполнителя"},
		},
	})
	if got := g.GetSheetList(); len(got) != 2 || got[0] != "Данные" || got[1] != "Параметры" {
		t.Fatalf("листы: %v", got)
	}
	rows, err := g.GetRows("Данные")
	if err != nil || len(rows) != 4 {
		t.Fatalf("строки: %v %v", len(rows), err)
	}
	if strings.Join(rows[0], "|") != "Задача|Исполнитель|Дн. на тесте|Название" {
		t.Errorf("шапка: %v", rows[0])
	}
	if rows[1][0] != "AC-12" || rows[1][2] != "7" || rows[3][2] != "2.5" {
		t.Errorf("значения: %v", rows[1:])
	}
	// число — число, а не текст
	if tp, _ := g.GetCellType("Данные", "C2"); tp != excelize.CellTypeUnset && tp != excelize.CellTypeNumber {
		t.Errorf("C2 — тип %v, ждали число", tp)
	}
	// «=1+1» — текст, не формула
	if fm, _ := g.GetCellFormula("Данные", "D3"); fm != "" {
		t.Errorf("текст превратился в формулу: %q", fm)
	}
	if v, _ := g.GetCellValue("Данные", "D3"); v != "=1+1" {
		t.Errorf("D3 = %q", v)
	}
	// ключи задач — ссылки на Jira
	if ok, target, _ := g.GetCellHyperLink("Данные", "A2"); !ok || target != "https://jira.example/browse/AC-12" {
		t.Errorf("ссылка A2: %v %q", ok, target)
	}
	if ok, _, _ := g.GetCellHyperLink("Данные", "B2"); ok {
		t.Error("ссылка только у колонки с ключами")
	}
	// автофильтр и «Параметры»
	hasFilter := false
	for _, d := range g.GetDefinedName() {
		if d.Name == "_xlnm._FilterDatabase" && strings.Contains(d.RefersTo, "Данные") {
			hasFilter = true
		}
	}
	if !hasFilter {
		t.Error("нет автофильтра на листе данных")
	}
	meta, _ := g.GetRows("Параметры")
	text := ""
	for _, r := range meta {
		text += strings.Join(r, "|") + "\n"
	}
	for _, want := range []string{"Долго на тесте", "Иванов Иван", "Исполнитель: Петров; Тестировщик: Тестов", "Строк|3", "04.11.2026 10:30"} {
		if !strings.Contains(text, want) {
			t.Errorf("в «Параметрах» нет %q:\n%s", want, text)
		}
	}
}

func TestBuildLeadXLSXNoFilters(t *testing.T) {
	g := reopen(t, exportReq{Title: "Заблокированные задачи", Columns: []string{"Задача"}, Rows: [][]any{}})
	meta, _ := g.GetRows("Параметры")
	found := false
	for _, r := range meta {
		if len(r) == 2 && r[0] == "Фильтры" && strings.HasPrefix(r[1], "нет") {
			found = true
		}
	}
	if !found {
		t.Errorf("без фильтров должно быть «нет»: %v", meta)
	}
}

func TestBuildLeadXLSXLimits(t *testing.T) {
	cases := map[string]exportReq{
		"нет колонок":      {Columns: nil},
		"много колонок":    {Columns: make([]string, exportMaxCols+1)},
		"много строк":      {Columns: []string{"a"}, Rows: make([][]any, exportMaxRows+1)},
		"неверный key_col": {Columns: []string{"a"}, KeyCol: func() *int { n := 3; return &n }()},
	}
	for name, req := range cases {
		if f, err := buildLeadXLSX(req, "", testNow); err == nil {
			f.Close()
			t.Errorf("%s: ждали ошибку", name)
		}
	}
	// длинный текст обрезается до лимита Excel
	g := reopen(t, exportReq{Columns: []string{"a"}, Rows: [][]any{{strings.Repeat("я", 40000)}}})
	if v, _ := g.GetCellValue("Данные", "A2"); len([]rune(v)) != excelCellLimit {
		t.Errorf("длина %d, ждали %d", len([]rune(v)), excelCellLimit)
	}
}

func TestExportFileName(t *testing.T) {
	got := exportFileName(exportReq{Title: `Долго на тесте/"x"`, Lead: "Иванов: Иван"}, testNow)
	if got != "Контроль спринта - Долго на тесте x - Иванов Иван - 2026-11-04.xlsx" {
		t.Errorf("имя файла: %q", got)
	}
	if n := exportFileName(exportReq{Title: strings.Repeat("я", 300)}, testNow); len([]rune(n)) > 125 {
		t.Errorf("слишком длинное имя: %d", len([]rune(n)))
	}
}
