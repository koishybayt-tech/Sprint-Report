// Package leadcontrol — «Проекты руководителей · контроль спринта» (docs/lead-control.md):
// что мешает открытым спринтам руководителя прямо сейчас. Правила — из описания, спорные места — в его разделе «Решения».
package leadcontrol

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Change — переход из истории задачи. Статус — названия; исполнитель и автор — логины Jira.
type Change struct {
	At     time.Time
	Field  string // status | assignee
	From   string
	To     string
	Author string
}

// Link — связь задачи (issuelinks).
type Link struct {
	Type      string // Relate, Blocks, ...
	Direction string // outward | inward
	Key       string
}

type Sprint struct {
	Name  string
	State string // ACTIVE | FUTURE | CLOSED
}

// Issue — задача Jira со всем, что нужно модулю.
type Issue struct {
	Key, Project, Summary, Type string
	Status                      string // название статуса
	Assignee                    string // отображаемое имя ("" — не назначена)
	AssigneeKey                 string // ключ исполнителя в Jira (JIRAUSER…): так исполнитель записан в истории
	Tester                      string // «Тестировщик» (customfield_10902), отображаемое имя
	Position                    string // «Позиция разработчика» (customfield_11101)
	Created                     time.Time
	Sprints                     []Sprint
	InOpenSprint                bool
	InFutureSprint              bool
	Changes                     []Change // по времени
	Links                       []Link
	ProjectID, TypeID           string // id проекта и типа задачи в Jira — для формы «Создать продолжение»
}

// LeadProject — проект руководителя (lead_projects).
type LeadProject struct {
	Key      string `json:"key"`
	Title    string `json:"title"`
	Assigned bool   `json:"-"` // проект назначен пользователю сервиса (lead_projects.user_id)
}

// Input — всё для расчёта по одному руководителю.
type Input struct {
	Now      time.Time
	Lead     string
	Projects []LeadProject
	// LeadOf — руководитель любого проекта: из lead_projects, иначе лид проекта в Jira (для «Другие проекты (руководитель)»).
	LeadOf map[string]string
	Issues map[string]*Issue
	Names  map[string]string // логин → отображаемое имя
	// KeyNames — ключ пользователя Jira (JIRAUSER…) → имя: для «было»/«стало» в смене исполнителя.
	KeyNames map[string]string
	Roles    map[string]string // отображаемое имя → должность человека (самая частая по задачам или из справочника)
	// Ignored — «Игнорировать» в разрывах: продолжение не нужно (ключ задачи → кто и когда скрыл).
	Ignored  map[string]Ignore
	JiraBase string // для ссылок «Создать продолжение»
}

// Ignore — задача, скрытая из «Разрывов».
type Ignore struct {
	By string
	At time.Time
}

// ---------- статусы ----------

const (
	stTodo    = "Сделать"
	stWork    = "В работе"
	stReview  = "Code review"
	stRework  = "Need Rework"
	stTest    = "Test"
	stBlocked = "Blocked"
)

// rank — насколько далеко задача по процессу: 0 — не начата, 1 — в работе/на доработке, 2 — ревью,
// 3 — тест, 4 — готова к выкладке, 5 — готово. Blocked — -1 (вне шкалы).
func rank(status string) int {
	switch status {
	case stTodo, "To Do", "Open", "Backlog", "Бэклог", "Открыт", "Требует уточнения", "Selected For Development":
		return 0
	case stWork, stRework:
		return 1
	case stReview:
		return 2
	case stTest, "Тестирование":
		return 3
	case "Ready for deploy", "Ready for deploy Production", "In Staging":
		return 4
	case "Готово", "Done", "In Production", "Закрыт", "Closed", "Resolved":
		return 5
	case stBlocked:
		return -1
	}
	return 3 // прочие статусы Jira в этом процессе — разновидности тестирования (CLAUDE.md: «прочее = тестирование»)
}

// atTest — на тесте или дальше.
func atTest(status string) bool { return rank(status) >= 3 }

// open — задача не закрыта (не «Готово» и аналоги).
func open(status string) bool { return rank(status) < 5 }

// ---------- роль задачи ----------

const (
	roleDesign = "Дизайн"
	roleFront  = "Фронт"
	roleBack   = "Бэк"
	roleAI     = "ИИ"
)

// Префикс названия: «Дизайн -», «Фронт -», «БЭК -»/«Бэк -», «ИИ -». Разделитель — дефис, тире, точка или двоеточие
// (в данных встречается «БЭК. …», «Фронт. …»).
var rePrefix = regexp.MustCompile(`(?i)^\s*(дизайн|фронт|бэк|ии)\s*[-–—.:]\s*`)

// Role — роль задачи: по префиксу названия, иначе по должности в задаче (customfield_11101).
func Role(is *Issue) string {
	if m := rePrefix.FindStringSubmatch(is.Summary); m != nil {
		switch strings.ToLower(m[1]) {
		case "дизайн":
			return roleDesign
		case "фронт":
			return roleFront
		case "бэк":
			return roleBack
		case "ии":
			return roleAI
		}
	}
	p := strings.ToLower(is.Position)
	switch {
	case strings.Contains(p, "design"):
		return roleDesign
	case strings.Contains(p, "front"):
		return roleFront
	case strings.Contains(p, "ai engineer"):
		return roleAI
	case strings.Contains(p, "backend"), strings.Contains(p, "data engineer"):
		return roleBack
	case strings.Contains(p, "qa"):
		return "QA"
	case strings.Contains(p, "manager"):
		return "ПМ"
	}
	return ""
}

// kind — роль для правил «кто кого ждёт» и поиска пары: ИИ считается бэком.
func kind(role string) string {
	if role == roleAI {
		return roleBack
	}
	return role
}

// titleCore — название без префикса роли, в нижнем регистре, без лишних пробелов и знаков.
func titleCore(s string) string {
	s = rePrefix.ReplaceAllString(s, "")
	s = strings.ToLower(s)
	s = reNonWord.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

var reNonWord = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// similarity — коэффициент Дайса по множествам слов от 3 букв (0..1).
func similarity(a, b string) float64 {
	wa, wb := words(titleCore(a)), words(titleCore(b))
	if len(wa) == 0 || len(wb) == 0 {
		return 0
	}
	common := 0
	for w := range wa {
		if wb[w] {
			common++
		}
	}
	return 2 * float64(common) / float64(len(wa)+len(wb))
}

func words(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(s) {
		if len([]rune(w)) >= 3 {
			m[w] = true
		}
	}
	return m
}

// ---------- история ----------

// days — полных дней от t до now.
func days(now, t time.Time) int {
	if t.After(now) {
		return 0
	}
	return int(math.Floor(now.Sub(t).Hours() / 24))
}

// statusChanges — переходы статуса задачи по времени.
func statusChanges(is *Issue) []Change {
	var out []Change
	for _, c := range is.Changes {
		if c.Field == "status" {
			out = append(out, c)
		}
	}
	return out
}

// CanonStatus — русское имя статуса, как у самих задач. В истории Jira (changelog) «В работе», «Сделать» и «Готово»
// записаны по-английски (In Progress, To Do, Done): без приведения переход в «В работе» не находится и дни считаются
// от создания задачи. Остальные статусы в истории и у задач называются одинаково.
func CanonStatus(s string) string {
	switch s {
	case "In Progress":
		return stWork
	case "To Do":
		return stTodo
	case "Done":
		return "Готово"
	case "Need clarification":
		return "Требует уточнения"
	}
	return s
}

// CanonChange приводит имена статусов в записи истории к русским (остальные поля не трогает).
func CanonChange(c Change) Change {
	if c.Field == "status" {
		c.From, c.To = CanonStatus(c.From), CanonStatus(c.To)
	}
	return c
}

// takenAt — когда задачу взяли в работу: первый переход в «В работе» после последнего возврата в «Сделать»
// (nil — переходов в «В работе» не было, например нет истории). Круги ревью и доработки момент не сдвигают.
func takenAt(is *Issue) *time.Time {
	var t *time.Time
	for _, c := range statusChanges(is) {
		switch {
		case c.To == stWork && t == nil:
			at := c.At
			t = &at
		case c.To == stTodo:
			t = nil
		}
	}
	return t
}

// lastEnter — когда задача последний раз перешла в статус; без такого перехода — создание задачи
// (задача создана сразу в этом статусе).
func lastEnter(is *Issue, status string) time.Time {
	t := is.Created
	for _, c := range statusChanges(is) {
		if c.To == status {
			t = c.At
		}
	}
	return t
}

// firstEnter — первый переход в статус (nil — не было).
func firstEnter(is *Issue, status string) *time.Time {
	for _, c := range statusChanges(is) {
		if c.To == status {
			t := c.At
			return &t
		}
	}
	return nil
}

// lastStatusChange — последняя смена статуса (без смен — создание).
func lastStatusChange(is *Issue) time.Time {
	t := is.Created
	for _, c := range statusChanges(is) {
		t = c.At
	}
	return t
}

// totalIn — сколько всего дней задача провела в статусе (все заходы; текущий — до now).
func totalIn(is *Issue, status string, now time.Time) int {
	ch := statusChanges(is)
	cur := is.Status
	if len(ch) > 0 {
		cur = ch[0].From // статус при создании
	}
	since := is.Created
	var total time.Duration
	for _, c := range ch {
		if cur == status {
			total += c.At.Sub(since)
		}
		cur, since = c.To, c.At
	}
	if cur == status && now.After(since) {
		total += now.Sub(since)
	}
	return int(math.Floor(total.Hours() / 24))
}

// testSince — когда задача ушла на тест: последний переход из статуса до теста в тест или дальше (nil — не было).
func testSince(is *Issue) *time.Time {
	var t *time.Time
	for _, c := range statusChanges(is) {
		if !atTest(c.From) && atTest(c.To) {
			at := c.At
			t = &at
		}
	}
	return t
}

func countTo(is *Issue, status string) int {
	n := 0
	for _, c := range statusChanges(is) {
		if c.To == status {
			n++
		}
	}
	return n
}

// ---------- общее ----------

func (in *Input) leadProject(key string) bool {
	for _, p := range in.Projects {
		if p.Key == key {
			return true
		}
	}
	return false
}

// sprintIssues — задачи открытых спринтов проектов руководителя (без эпиков), по ключу.
func (in *Input) sprintIssues() []*Issue {
	var out []*Issue
	for _, is := range in.Issues {
		if is.InOpenSprint && is.Type != "Epic" && in.leadProject(is.Project) {
			out = append(out, is)
		}
	}
	sort.Slice(out, func(i, j int) bool { return keyLess(out[i].Key, out[j].Key) })
	return out
}

// keyLess — порядок ключей Jira: проект, затем номер.
func keyLess(a, b string) bool {
	pa, na, _ := strings.Cut(a, "-")
	pb, nb, _ := strings.Cut(b, "-")
	if pa != pb {
		return pa < pb
	}
	if len(na) != len(nb) {
		return len(na) < len(nb)
	}
	return na < nb
}

func activeSprint(is *Issue) string {
	for _, s := range is.Sprints {
		if s.State == "ACTIVE" {
			return s.Name
		}
	}
	return ""
}

func ddmm(t time.Time) string { return t.In(almaty).Format("02.01") }

var almaty = func() *time.Location {
	if l, err := time.LoadLocation("Asia/Almaty"); err == nil {
		return l
	}
	return time.FixedZone("Asia/Almaty", 5*3600)
}()

func yesNo(b bool) string {
	if b {
		return "да"
	}
	return "нет"
}

func (in *Input) name(login string) string {
	if n, ok := in.Names[login]; ok {
		return n
	}
	return login
}
