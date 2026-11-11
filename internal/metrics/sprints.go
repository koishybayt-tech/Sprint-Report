package metrics

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Almaty — часовой пояс отчётов.
var Almaty = mustLoad("Asia/Almaty")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

type Sprint struct {
	ID    int64
	Name  string
	State string // ACTIVE | FUTURE | CLOSED
	Start *time.Time
	End   *time.Time
}

// Issue — задача открытого спринта с полями, нужными для расчётов.
type Issue struct {
	Key              string
	ProjectKey       string
	ProjectName      string
	StatusID         string
	StatusName       string
	Type             string
	Plan             string
	Assignee         string // displayName, "" — не назначен
	AssigneeLogin    string
	AssigneeInactive bool   // пользователь Jira деактивирован (уволен)
	Tester           string // displayName поля «Тестировщик» (cf 10902)
	TesterLogin      string
	Position         string // cf 11101
	Summary          string
	Priority         string
	Created          time.Time
	Updated          time.Time
	TimespentS       int
	Sprints          []Sprint // в порядке Jira
}

type OldestOpen struct {
	Key     string `json:"key"`
	AgeDays int    `json:"age_days"`
}

// ProjectSprint — строка вкладки «Статус спринтов» (api/openapi.yaml).
type ProjectSprint struct {
	Key         string      `json:"key"`
	Name        string      `json:"name"`
	Lead        string      `json:"lead"`
	Sprint      string      `json:"sprint"`
	Start       string      `json:"start,omitempty"`
	End         string      `json:"end,omitempty"`
	LeftDays    *int        `json:"left_days"`
	Total       int         `json:"total"`
	Todo        int         `json:"todo"`
	Wk          int         `json:"wk"`
	Cr          int         `json:"cr"`
	Rw          int         `json:"rw"`
	Ts          int         `json:"ts"`
	Dn          int         `json:"dn"`
	Bug         int         `json:"bug"`
	Carry       int         `json:"carry"`
	CarryOpen   int         `json:"carry_open"`
	Added       int         `json:"added"`
	Unplanned   int         `json:"unplanned"`
	Blocked     int         `json:"blocked"`
	Stale       int         `json:"stale"`
	Hours       float64     `json:"hours"`
	People      int         `json:"people"`
	ReadyDeploy int         `json:"ready_deploy"`
	InTest      int         `json:"in_test"`
	ReadyPct    int         `json:"ready_pct"`
	TimePct     int         `json:"time_pct"`
	OldestOpen  *OldestOpen `json:"oldest_open"`
	// CreatedDaily — задачи, созданные после старта спринта, по дням (поток новых и внеплановых задач); nil — у проекта нет спринта.
	CreatedDaily *CreatedDaily `json:"created_daily"`
}

// CreatedDaily — ряд по календарным дням (Алматы) от начала спринта: сколько задач, лежащих сейчас в спринте, создано в этот день.
type CreatedDaily struct {
	From      string `json:"from"`      // дата первого дня ряда (YYYY-MM-DD)
	Total     []int  `json:"total"`     // создано задач
	Unplanned []int  `json:"unplanned"` // из них внеплановых (поле «План» = «Внеплановая»)
}

// maxCreatedDays — длиннее ряд не строим (просроченный спринт): берём последние дни.
const maxCreatedDays = 62

// createdDaily собирает ряд от start до min(сегодня, конец спринта), но не короче последнего дня с созданной задачей.
func createdDaily(start, end string, today time.Time, created map[int][2]int, maxIdx int) *CreatedDaily {
	if start == "" {
		return nil
	}
	st := dateNoon(start)
	idx := func(t time.Time) int { return int(math.Round(t.Sub(st).Hours() / 24)) }
	last := idx(today)
	if end != "" {
		last = min(last, idx(dateNoon(end)))
	}
	last = max(last, maxIdx, 0)
	first := max(0, last-maxCreatedDays+1)
	cd := &CreatedDaily{From: st.AddDate(0, 0, first).Format("2006-01-02"), Total: make([]int, last-first+1), Unplanned: make([]int, last-first+1)}
	for i := first; i <= last; i++ {
		c := created[i]
		cd.Total[i-first], cd.Unplanned[i-first] = c[0], c[1]
	}
	return cd
}

var readyDeployRe = regexp.MustCompile(`Ready for deploy|In Staging`)

// ProjectSprints считает строки проектов 1-в-1 как reference/sprints/inc.js (PJ, PX)
// плюс производные из page_template.html (left, done, time). asOf — момент выгрузки данных.
//
// Как и в текущем дашборде, в проект попадают все задачи cf[10104] in openSprints() (только активные спринты);
// спринт проекта берётся по самой ранней по created задаче.
func ProjectSprints(issues []Issue, leads map[string]string, asOf time.Time) []ProjectSprint {
	iss := append([]Issue(nil), issues...)
	sort.SliceStable(iss, func(a, b int) bool {
		if !iss[a].Created.Equal(iss[b].Created) {
			return iss[a].Created.Before(iss[b].Created)
		}
		return iss[a].Key < iss[b].Key
	})
	dd := func(t time.Time) int { return int(math.Floor(asOf.Sub(t).Hours() / 24)) }

	type acc struct {
		p      ProjectSprint
		ppl    map[string]bool
		hours  float64
		oldAge int
		start  *time.Time
		// created — задачи, созданные после старта спринта: индекс дня от начала спринта → [всего, внеплановых]
		created map[int][2]int
		maxIdx  int
	}
	byKey := map[string]*acc{}
	var order []string
	for _, f := range iss {
		var active *Sprint
		for i := range f.Sprints {
			if f.Sprints[i].State == "ACTIVE" {
				active = &f.Sprints[i]
			}
		}
		a := byKey[f.ProjectKey]
		if a == nil {
			lead := leads[f.ProjectKey]
			if lead == "" {
				lead = "—"
			}
			a = &acc{p: ProjectSprint{Key: f.ProjectKey, Name: f.ProjectName, Lead: lead}, ppl: map[string]bool{}, oldAge: -1, created: map[int][2]int{}}
			if active != nil {
				a.p.Sprint = active.Name
				a.p.Start = dateOf(active.Start)
				a.p.End = dateOf(active.End)
			}
			byKey[f.ProjectKey] = a
			order = append(order, f.ProjectKey)
		}
		p := &a.p
		g := StatusGroup(f.StatusID)
		up, cr := dd(f.Updated), dd(f.Created)
		if g != Dn {
			if cr > a.oldAge {
				p.OldestOpen = &OldestOpen{Key: f.Key}
				a.oldAge = cr
			}
			if len(f.Sprints) > 1 {
				p.CarryOpen++
			}
		}
		p.Total++
		switch g {
		case Todo:
			p.Todo++
		case Wk:
			p.Wk++
		case Cr:
			p.Cr++
		case Rw:
			p.Rw++
		case Dn:
			p.Dn++
		default:
			p.Ts++
		}
		if f.Type == "Ошибка" {
			p.Bug++
		}
		if len(f.Sprints) > 1 {
			p.Carry++
		}
		if active != nil && active.Start != nil && f.Created.After(*active.Start) {
			p.Added++
			if p.Start != "" {
				i := int(math.Round(noon(f.Created.In(Almaty)).Sub(dateNoon(p.Start)).Hours() / 24))
				if i >= 0 {
					c := a.created[i]
					c[0]++
					if f.Plan == "Внеплановая" {
						c[1]++
					}
					a.created[i] = c
					a.maxIdx = max(a.maxIdx, i)
				}
			}
		}
		if f.Plan == "Внеплановая" {
			p.Unplanned++
		}
		if f.StatusName == "Blocked" {
			p.Blocked++
		}
		if g != Dn && up > 14 {
			p.Stale++
		}
		a.hours += float64(f.TimespentS) / 3600
		if readyDeployRe.MatchString(f.StatusName) {
			p.ReadyDeploy++
		}
		if f.StatusName == "Test" {
			p.InTest++
		}
		if f.Assignee != "" {
			a.ppl[f.Assignee] = true
		}
	}

	today := noon(asOf.In(Almaty))
	out := make([]ProjectSprint, 0, len(order))
	for _, k := range order {
		a := byKey[k]
		p := a.p
		p.Hours = math.Round(a.hours*10) / 10
		p.People = len(a.ppl)
		if p.OldestOpen != nil {
			p.OldestOpen.AgeDays = max(0, a.oldAge)
		}
		p.ReadyPct = pct(p.Dn, p.Total)
		if p.End != "" {
			left := int(math.Round(dateNoon(p.End).Sub(today).Hours() / 24))
			p.LeftDays = &left
		}
		p.TimePct = timePct(p.Start, p.End, today)
		p.CreatedDaily = createdDaily(p.Start, p.End, today, a.created, a.maxIdx)
		out = append(out, p)
	}
	return out
}

// timePct — доля прошедшего времени спринта, 0…100 (page_template.html: p.time).
// Даты начала/конца там разбираются как полночь UTC, «сегодня» — полдень по Алматы; повторяем это.
func timePct(start, end string, today time.Time) int {
	if start == "" || end == "" {
		return 0
	}
	st, _ := time.Parse("2006-01-02", start)
	en, _ := time.Parse("2006-01-02", end)
	length := math.Max(1, en.Sub(st).Hours()/24)
	f := today.Sub(st).Hours() / 24 / length
	return int(math.Round(math.Min(1, math.Max(0, f)) * 100))
}

func pct(a, b int) int {
	if b == 0 {
		return 0
	}
	return int(math.Round(float64(a) / float64(b) * 100))
}

func dateOf(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.In(Almaty).Format("2006-01-02")
}

func noon(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 12, 0, 0, 0, Almaty)
}

func dateNoon(d string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02", d, Almaty)
	return noon(t)
}

// NextSprint — следующий спринт проекта (reference/sprints/ns_fetch.js).
type NextSprint struct {
	ProjectKey string      `json:"project_key"`
	Current    string      `json:"current"`
	NextID     *int64      `json:"next_id"`
	NextName   *string     `json:"next_name"`
	NextStart  string      `json:"next_start,omitempty"`
	NextEnd    string      `json:"next_end,omitempty"`
	NextTotal  int         `json:"next_total"`
	Statuses   []NameCount `json:"statuses"` // задачи следующего спринта по статусам
	By         []NameCount `json:"by"`       // и по исполнителям
}

var (
	digitsRe   = regexp.MustCompile(`[\d.]+`)
	sepRe      = regexp.MustCompile(`[,\s]+`)
	numRe      = regexp.MustCompile(`\d+`)
	yearRe     = regexp.MustCompile(`^20\d\d$`)
	hasDigitRe = regexp.MustCompile(`\d`)
	backlogRe  = regexp.MustCompile(`(?i)backlog|бэклог`)
)

func sprintTpl(s string) string {
	return strings.TrimSpace(sepRe.ReplaceAllString(digitsRe.ReplaceAllString(s, "#"), " "))
}

func sprintNum(s string) int {
	for _, m := range numRe.FindAllString(s, -1) {
		if !yearRe.MatchString(m) {
			n, _ := strconv.Atoi(m)
			return n
		}
	}
	return 0
}

// PickNext выбирает следующий спринт среди будущих спринтов досок проекта, как ns_fetch.js:
// сначала с тем же шаблоном имени, затем с тем же префиксом, затем любой с датой начала; меньший номер.
func PickNext(current string, futures []Sprint) *Sprint {
	seen := map[int64]bool{}
	var fut []Sprint
	for _, f := range futures {
		if !seen[f.ID] {
			seen[f.ID] = true
			fut = append(fut, f)
		}
	}
	// В ns_fetch.js кандидаты лежат в объекте по id, а JS перечисляет числовые ключи по возрастанию.
	sort.Slice(fut, func(a, b int) bool { return fut[a].ID < fut[b].ID })
	filter := func(ok func(Sprint) bool) []Sprint {
		var r []Sprint
		for _, f := range fut {
			if ok(f) {
				r = append(r, f)
			}
		}
		return r
	}
	tc := sprintTpl(current)
	c := filter(func(f Sprint) bool { return sprintTpl(f.Name) == tc })
	if len(c) == 0 {
		prefix := strings.TrimSpace(strings.SplitN(tc, "#", 2)[0])
		c = filter(func(f Sprint) bool {
			return strings.HasPrefix(sprintTpl(f.Name), prefix) && hasDigitRe.MatchString(f.Name) && !backlogRe.MatchString(f.Name)
		})
	}
	if len(c) == 0 {
		c = filter(func(f Sprint) bool { return f.Start != nil && !backlogRe.MatchString(f.Name) })
	}
	if len(c) == 0 {
		return nil
	}
	sort.SliceStable(c, func(a, b int) bool { return sprintNum(c[a].Name) < sprintNum(c[b].Name) })
	return &c[0]
}
