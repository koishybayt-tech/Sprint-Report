package bitrix

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Vacation — запись API отпусков, которую сервис учитывает.
type Vacation struct {
	ID     string // element_id заявления в Bitrix
	Kind   string // vacation | unpaid (значения absences.kind)
	Type   string // vocation_type как пришёл
	Status string
	From   time.Time
	To     time.Time
}

// Типы отпусков API → absences.kind. Больничные из этого API не берём.
var kinds = map[string]string{"Трудовой отпуск": "vacation", "Отпуск б/с": "unpaid"}

// Статус, при котором заявление пропускается.
const statusInterrupted = "Прерван"

// response — ответ vacation_api.php (проверено `reports bitrix probe`):
// {"employee_id":20980,"count":5,...,"items":[{"element_id":"356424","vocation_type":"Трудовой отпуск",
// "date_start":"19.12.2025","date_end":"19.12.2025","status":"Завершен",...}]}
type response struct {
	Items []struct {
		ElementID    string `json:"element_id"`
		VocationType string `json:"vocation_type"`
		DateStart    string `json:"date_start"`
		DateEnd      string `json:"date_end"`
		Status       string `json:"status"`
	} `json:"items"`
}

// Seen — что встретилось в ответе: тип → статус → число заявлений (для статистики синхронизации).
type Seen map[string]map[string]int

// Parse разбирает ответ API. Возвращает учитываемые отпуска, число пропущенных заявлений
// (другой тип, «Прерван», без дат) и все встретившиеся типы и статусы.
func Parse(body []byte) (vac []Vacation, skipped int, seen Seen, err error) {
	var r response
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, 0, nil, fmt.Errorf("ответ не в ожидаемом формате: %w", err)
	}
	seen = Seen{}
	for _, it := range r.Items {
		v := Vacation{ID: strings.TrimSpace(it.ElementID), Type: strings.TrimSpace(it.VocationType), Status: strings.TrimSpace(it.Status)}
		if seen[v.Type] == nil {
			seen[v.Type] = map[string]int{}
		}
		seen[v.Type][v.Status]++
		kind, ok := kinds[v.Type]
		if !ok || v.Status == statusInterrupted {
			skipped++
			continue
		}
		v.Kind = kind
		from, err1 := time.Parse("02.01.2006", strings.TrimSpace(it.DateStart))
		to, err2 := time.Parse("02.01.2006", strings.TrimSpace(it.DateEnd))
		if err1 != nil || err2 != nil {
			skipped++
			continue
		}
		v.From, v.To = from, to
		vac = append(vac, v)
	}
	return vac, skipped, seen, nil
}
