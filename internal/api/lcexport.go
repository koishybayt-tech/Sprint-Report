package api

import (
	"encoding/json"
	"fmt"
	"math"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
)

// Выгрузка таблицы «Контроля спринта руководителей» в Excel.
// Страница присылает ровно то, что видит пользователь после фильтров и сортировки (названия колонок и строки),
// сервер собирает .xlsx: лист с данными (шапка, закрепление, автофильтр, ссылки на задачи Jira) и лист «Параметры».

const (
	exportMaxRows  = 20000
	exportMaxCols  = 40
	exportMaxBytes = 16 << 20
	excelCellLimit = 32000 // в ячейке Excel — не больше 32767 символов
)

type exportReq struct {
	Title   string   `json:"title"`   // вкладка, например «Долго на тесте»
	Lead    string   `json:"lead"`    // руководитель
	Project string   `json:"project"` // «Все проекты» или проект
	Filters []string `json:"filters"` // включённые фильтры: «Исполнитель: Иванов»
	Updated string   `json:"updated"` // когда обновлены данные Jira (как на странице)
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
	KeyCol  *int     `json:"key_col"` // колонка с ключами задач — станет ссылками на Jira
}

var reExportKey = regexp.MustCompile(`^[A-Z][A-Z0-9_]*-\d+$`)

// POST /api/v1/lead-control/export — скачать таблицу в Excel (доступ как у «Контроля спринта»).
func (s *leadControlServer) exportXLSX(w http.ResponseWriter, r *http.Request) {
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "ожидается application/json"})
		return
	}
	var req exportReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, exportMaxBytes)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "некорректный JSON или слишком большой запрос"})
		return
	}
	f, err := buildLeadXLSX(req, s.jiraBase, time.Now())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	defer f.Close()
	name := exportFileName(req, time.Now())
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="lead-control.xlsx"; filename*=UTF-8''`+url.PathEscape(name))
	w.Header().Set("Cache-Control", "no-store")
	if err := f.Write(w); err != nil {
		s.log.Warn("lead-control export: запись ответа", "err", err)
		return
	}
	s.log.Info("lead-control export", "tab", req.Title, "lead", req.Lead, "rows", len(req.Rows))
}

var reBadFileChars = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]+`)

// exportFileName — «Контроль спринта - Долго на тесте - Руководитель - 2026-11-04.xlsx» без запрещённых в именах символов.
func exportFileName(req exportReq, now time.Time) string {
	parts := []string{"Контроль спринта"}
	for _, p := range []string{req.Title, req.Lead} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	parts = append(parts, now.Format("2006-01-02"))
	name := strings.Join(parts, " - ")
	name = strings.Join(strings.Fields(reBadFileChars.ReplaceAllString(name, " ")), " ")
	if utf8.RuneCountInString(name) > 120 {
		name = string([]rune(name)[:120])
	}
	return name + ".xlsx"
}

func clipCell(s string) string {
	if utf8.RuneCountInString(s) > excelCellLimit {
		return string([]rune(s)[:excelCellLimit])
	}
	return s
}

// buildLeadXLSX собирает книгу: лист «Данные» и лист «Параметры».
func buildLeadXLSX(req exportReq, jiraBase string, now time.Time) (*excelize.File, error) {
	if len(req.Columns) == 0 || len(req.Columns) > exportMaxCols {
		return nil, fmt.Errorf("колонок должно быть от 1 до %d", exportMaxCols)
	}
	if len(req.Rows) > exportMaxRows {
		return nil, fmt.Errorf("слишком много строк (максимум %d) — сузьте фильтры", exportMaxRows)
	}
	if req.KeyCol != nil && (*req.KeyCol < 0 || *req.KeyCol >= len(req.Columns)) {
		return nil, fmt.Errorf("неверная колонка с ключами задач")
	}
	f := excelize.NewFile()
	const data, params = "Данные", "Параметры"
	if err := f.SetSheetName("Sheet1", data); err != nil {
		return nil, err
	}
	head, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#E8EEF4"}},
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true}, Border: []excelize.Border{{Type: "bottom", Color: "#9AA8B6", Style: 1}}})
	if err != nil {
		return nil, err
	}
	body, err := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{Vertical: "top", WrapText: true}})
	if err != nil {
		return nil, err
	}
	link, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Color: "#0563C1", Underline: "single"}, Alignment: &excelize.Alignment{Vertical: "top"}})
	if err != nil {
		return nil, err
	}

	widths := make([]float64, len(req.Columns))
	for c, label := range req.Columns {
		cell, _ := excelize.CoordinatesToCellName(c+1, 1)
		if err := f.SetCellStr(data, cell, clipCell(label)); err != nil {
			return nil, err
		}
		widths[c] = float64(utf8.RuneCountInString(label)) + 3
	}
	base := strings.TrimRight(jiraBase, "/")
	for ri, row := range req.Rows {
		for c := 0; c < len(req.Columns) && c < len(row); c++ {
			cell, _ := excelize.CoordinatesToCellName(c+1, ri+2)
			text := ""
			switch v := row[c].(type) {
			case nil:
				continue
			case float64:
				if v == math.Trunc(v) && math.Abs(v) < 1e15 {
					err = f.SetCellInt(data, cell, int64(v))
				} else {
					err = f.SetCellFloat(data, cell, v, -1, 64)
				}
				text = fmt.Sprint(v)
			case bool:
				err = f.SetCellBool(data, cell, v)
				text = fmt.Sprint(v)
			case string:
				// SetCellStr кладёт именно текст: «=1+1» не станет формулой
				text = clipCell(v)
				err = f.SetCellStr(data, cell, text)
			default:
				text = clipCell(fmt.Sprint(v))
				err = f.SetCellStr(data, cell, text)
			}
			if err != nil {
				return nil, err
			}
			if n := float64(utf8.RuneCountInString(text)) + 2; n > widths[c] {
				widths[c] = n
			}
			if req.KeyCol != nil && c == *req.KeyCol && reExportKey.MatchString(text) {
				if err := f.SetCellHyperLink(data, cell, base+"/browse/"+text, "External"); err != nil {
					return nil, err
				}
				if err := f.SetCellStyle(data, cell, cell, link); err != nil {
					return nil, err
				}
			}
		}
	}
	last, _ := excelize.CoordinatesToCellName(len(req.Columns), len(req.Rows)+1)
	if err := f.SetCellStyle(data, "A1", fmt.Sprintf("%s1", strings.TrimRight(last, "0123456789")), head); err != nil {
		return nil, err
	}
	if len(req.Rows) > 0 {
		firstBody, _ := excelize.CoordinatesToCellName(1, 2)
		if err := f.SetCellStyle(data, firstBody, last, body); err != nil {
			return nil, err
		}
		// ссылки на задачи перекрыли стиль выше — вернуть им стиль ссылки
		if req.KeyCol != nil {
			for ri, row := range req.Rows {
				if *req.KeyCol < len(row) {
					if s, ok := row[*req.KeyCol].(string); ok && reExportKey.MatchString(s) {
						cell, _ := excelize.CoordinatesToCellName(*req.KeyCol+1, ri+2)
						if err := f.SetCellStyle(data, cell, cell, link); err != nil {
							return nil, err
						}
					}
				}
			}
		}
	}
	for c, wd := range widths {
		col, _ := excelize.ColumnNumberToName(c + 1)
		if err := f.SetColWidth(data, col, col, math.Min(math.Max(wd, 8), 60)); err != nil {
			return nil, err
		}
	}
	if err := f.SetPanes(data, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return nil, err
	}
	if err := f.AutoFilter(data, "A1:"+last, nil); err != nil {
		return nil, err
	}

	// лист «Параметры»: что выгружено и с какими фильтрами
	if _, err := f.NewSheet(params); err != nil {
		return nil, err
	}
	filters := "нет (все строки вкладки)"
	if len(req.Filters) > 0 {
		filters = strings.Join(req.Filters, "; ")
	}
	meta := [][2]string{
		{"Выгрузка", "Контроль спринта руководителей · " + req.Title},
		{"Руководитель", req.Lead},
		{"Проект", req.Project},
		{"Фильтры", filters},
		{"Строк", fmt.Sprint(len(req.Rows))},
		{"Данные Jira обновлены", req.Updated},
		{"Выгружено", now.Format("02.01.2006 15:04")},
	}
	bold, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Alignment: &excelize.Alignment{Vertical: "top"}})
	wrap, _ := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{Vertical: "top", WrapText: true}})
	for i, m := range meta {
		a, _ := excelize.CoordinatesToCellName(1, i+1)
		b, _ := excelize.CoordinatesToCellName(2, i+1)
		_ = f.SetCellStr(params, a, m[0])
		_ = f.SetCellStr(params, b, clipCell(m[1]))
		_ = f.SetCellStyle(params, a, a, bold)
		_ = f.SetCellStyle(params, b, b, wrap)
	}
	_ = f.SetColWidth(params, "A", "A", 26)
	_ = f.SetColWidth(params, "B", "B", 80)
	f.SetActiveSheet(0)
	return f, nil
}
