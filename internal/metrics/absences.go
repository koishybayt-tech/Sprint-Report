package metrics

import "time"

// AbsenceRow — запись absences с сотрудником (из БД).
type AbsenceRow struct {
	Login    string // jira_login сотрудника
	FullName string
	Kind     string // vacation | unpaid | sick
	From     time.Time
	To       *time.Time
	Source   string // bitrix | telegram | manual
	Note     string
}

// Absence — отсутствие в формате ABS из reference/absences/build_abs.js: [имя, тип, с, по, источник, примечание].
// Имя — как у исполнителя в Jira, чтобы совпадать со строками «Загрузки ресурсов».
type Absence struct {
	Name   string  `json:"name"`
	Type   string  `json:"type"` // vac | bs | sick | off
	From   string  `json:"from"`
	To     *string `json:"to"`
	Source string  `json:"source"` // Bitrix | Telegram
	Note   string  `json:"note"`
}

var absType = map[string]string{"vacation": "vac", "unpaid": "bs", "sick": "sick"}
var absNote = map[string]string{"vacation": "Трудовой отпуск", "unpaid": "Отпуск без содержания"}
var absSource = map[string]string{"bitrix": "Bitrix", "telegram": "Telegram", "manual": "Вручную"}

// AttachAbsences добавляет в отчёт отсутствия (ABS) и время их обновления по источникам (ABSAT).
// Имя берётся из Jira по логину сотрудника; если у него нет задач в спринтах — ФИО из справочника.
func AttachAbsences(r *SprintReport, rows []AbsenceRow, updated map[string]time.Time) {
	byLogin := map[string]string{}
	for name, login := range r.Logins {
		if login != "" {
			byLogin[login] = name
		}
	}
	r.Absences = []Absence{}
	for _, a := range rows {
		name := byLogin[a.Login]
		if name == "" {
			name = a.FullName
		}
		t, ok := absType[a.Kind]
		if !ok {
			continue
		}
		note := a.Note
		if n, ok := absNote[a.Kind]; ok && a.Source == "bitrix" {
			note = n
		}
		x := Absence{Name: name, Type: t, From: a.From.Format("2006-01-02"), Source: absSource[a.Source], Note: note}
		if a.To != nil {
			s := a.To.Format("2006-01-02")
			x.To = &s
		}
		r.Absences = append(r.Absences, x)
	}
	r.AbsencesAt = updated
}
