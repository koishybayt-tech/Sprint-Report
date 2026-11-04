<script setup lang="ts">
// «Проекты руководителей · контроль спринта» — по design/prototypes/lead-control.html и docs/lead-control.md.
// Все фильтры — в query-строке: lead, proj, tab, q, all, pr, miss, kind, who (исполнитель), tst (тестировщик).
import { type LcCol } from '~/components/LcTable.vue'
import { HOT, type LCTab, TABS, countBy, linkKeys, matchWho, personOf, sortRows, testerOf } from '~/utils/leadcontrol'
import { esc } from '~/utils/sprints'

type Project = { key: string; title: string }
type Lead = { lead: string; projects: Project[] }
type Totals = { project: string; blocked: number; blocked_wait: number; can_unblock: number; long_work: number; long_test: number; stalled_people: number; gaps_all: number; gaps_high: number }
type SprintRow = { project: string; sprint: string; total: number; snapshot: string } & Record<'done' | 'ready' | 'test' | 'review' | 'in_work' | 'rework' | 'blocked' | 'todo', number>
type Ignored = { project: string; key: string; summary: string; by: string; at: string }
type Report = { updated_at: string; lead: string; projects: Project[]; totals: Totals[]; sprints: SprintRow[]; blocked: any[]; in_work: any[]; people: any[]; gaps: any[]; testing: any[]; ignored: Ignored[] }

const { apiBase, jiraBase } = useRuntimeConfig().public
const route = useRoute()
const router = useRouter()
const qs = (k: string) => typeof route.query[k] === 'string' ? route.query[k] as string : ''
function setQuery(patch: Record<string, string | undefined>) {
  const q: Record<string, string> = {}
  for (const [k, v] of Object.entries({ ...route.query, ...patch })) if (typeof v === 'string' && v !== '') q[k] = v
  router.replace({ query: q })
}

const { data: leads, error: leadsErr } = await useFetch<Lead[]>(`${apiBase}/lead-control/leads`)
const lead = computed(() => leads.value?.some(l => l.lead === qs('lead')) ? qs('lead') : leads.value?.[0]?.lead || '')
const projects = computed(() => leads.value?.find(l => l.lead === lead.value)?.projects || [])
const proj = computed(() => projects.value.some(p => p.key === qs('proj')) ? qs('proj') : 'all')
const tab = computed<LCTab>(() => (TABS as string[]).includes(qs('tab')) ? qs('tab') as LCTab : 'blocked')

const { data: R, error, pending, refresh } = await useFetch<Report>(`${apiBase}/lead-control`, {
  query: computed(() => ({ lead: lead.value || undefined, project: proj.value === 'all' ? undefined : proj.value })),
  immediate: !!leads.value?.length, watch: [lead, proj],
})

// ---------- шапка, плитки, вывод ----------
const updated = computed(() => R.value ? new Date(R.value.updated_at).toLocaleString('ru-RU', { timeZone: 'Asia/Almaty', day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : '—')
const T = computed(() => R.value?.totals.find(t => t.project === proj.value))
const TILES: { tab: LCTab; l: string; f: keyof Totals; s: string }[] = [
  { tab: 'blocked', l: 'Заблокированы', f: 'blocked', s: 'в статусе Blocked' },
  { tab: 'work', l: 'Долго в работе', f: 'long_work', s: '≥ 5 дн. в текущем статусе' },
  { tab: 'people', l: 'Тормозят задачи', f: 'stalled_people', s: 'сотрудники: здесь стоит, у других руководителей двигается' },
  { tab: 'gaps', l: 'Дизайн или бэк готов, а продолжения нет', f: 'gaps_all', s: 'текущий спринт: нет задачи на фронт или бэк' },
  { tab: 'test', l: 'Долго на тесте', f: 'long_test', s: '≥ 3 дн. в статусе Test' },
]
// По клику на плитку «разрывов» её фильтры сбрасываются (как в макете).
function openTab(t: LCTab) { setQuery({ tab: t, who: undefined, tst: undefined, ...(t === 'gaps' ? { q: undefined, pr: undefined, miss: undefined, kind: undefined } : {}) }) }

// ---------- таблицы ----------
const J = jiraBase
const key = (f: string) => (r: any) => r[f] ? `<a href="${J.replace(/\/$/, '')}/browse/${esc(r[f])}" target="_blank" rel="noopener">${esc(r[f])}</a>` : '—'
const links = (f: string) => (r: any) => linkKeys(r[f], J)
const flag = (cls: string, v: string) => `<span class="${cls}">${esc(v)}</span>`
const COLS: Record<LCTab, LcCol[]> = {
  blocked: [
    { f: 'key', label: 'Задача', cls: 'k', html: key('key') }, { f: 'assignee', label: 'Исполнитель' }, { f: 'role', label: 'Роль' },
    { f: 'days_blocked', label: 'Дн. в Blocked', cls: 'num hd' },
    { f: 'block_state', label: 'Что с блоком', cls: 'w', html: r => r.block_state.startsWith('Ждёт') ? linkKeys(r.block_state, J) : flag(r.block_state.startsWith('Можно') ? 'flag-ok' : 'flag-warn', r.block_state) },
    { f: 'waiting_key', label: 'Ждёт задачу', cls: 'k', html: key('waiting_key') }, { f: 'waiting_role', label: 'Её роль' },
    { f: 'waiting_status', label: 'Её статус' }, { f: 'waiting_assignee', label: 'Её исполнитель' },
    { f: 'not_started', label: 'Не начата', html: r => r.not_started === 'да' ? flag('flag-bad', 'да') : esc(r.not_started || '—') },
    { f: 'blockers', label: 'Блокирующие задачи (статус, когда ушла на тест)', cls: 'x', html: links('blockers') },
    { f: 'summary', label: 'Название', cls: 'w' }, { f: 'related', label: 'Прочие связанные', cls: 'x', html: links('related') },
  ],
  work: [
    { f: 'key', label: 'Задача', cls: 'k', html: key('key') }, { f: 'status', label: 'Статус' }, { f: 'assignee', label: 'Исполнитель' },
    { f: 'role', label: 'Роль' }, { f: 'days_in_status', label: 'Дн. в статусе', cls: 'num hd' }, { f: 'days_since_start', label: 'Дн. с начала', cls: 'num' },
    { f: 'total_days_in_status', label: 'Всего в статусе', cls: 'num' }, { f: 'reworks', label: 'Возвратов', cls: 'num' },
    { f: 'sprints', label: 'Спринтов', cls: 'num' }, { f: 'summary', label: 'Название', cls: 'w' },
  ],
  people: [
    { f: 'person', label: 'Сотрудник' }, { f: 'role', label: 'Роль' },
    { f: 'signal', label: 'Сигнал', html: r => r.signal ? flag(r.signal === 'Тормозит' ? 'flag-bad' : 'flag-warn', r.signal) : '—' },
    { f: 'my_stale', label: 'Стоят ≥ 3 дн.', cls: 'num hd' }, { f: 'my_open', label: 'Открытых здесь', cls: 'num' },
    { f: 'my_actions_7d', label: 'Его действия здесь, 7 дн.', cls: 'num' }, { f: 'my_last_action_days', label: 'Посл. действие здесь, дн. назад', cls: 'num' },
    { f: 'other_started_7d', label: 'Взял у других, 7 дн.', cls: 'num' }, { f: 'other_finished_7d', label: 'Завершил у других, 7 дн.', cls: 'num' },
    { f: 'my_tasks', label: 'Задачи здесь (дн. без движения)', cls: 'x', html: links('my_tasks') },
    { f: 'other_projects', label: 'Другие проекты (руководитель) — действий', cls: 'x' },
    { f: 'other_tasks', label: 'Что делал у других', cls: 'x', html: links('other_tasks') },
  ],
  gaps: [
    { f: 'key', label: 'Задача', cls: 'w', html: r => `${key('key')(r)}<div class="sub">${esc(r.summary || '')}</div>` },
    { f: 'priority', label: 'Приоритет', html: r => r.priority === 'Низкий' ? esc(r.priority) : flag(r.priority === 'Проверить' ? 'flag-warn' : 'flag-bad', r.priority) },
    { f: 'kind', label: 'Тип · статус', html: r => `${esc(r.kind || '—')}<div class="sub">${esc(r.status || '')}</div>` },
    { f: 'assignee', label: 'Исполнитель · спринт', html: r => `${esc(r.assignee || '—')}<div class="sub">${esc(r.sprint || '')}</div>` },
    { f: 'missing', label: 'Чего нет' },
    { f: 'problem', label: 'Проблема', cls: 'w', html: links('problem') }, { f: 'front', label: 'Фронт', cls: 'x', html: links('front') },
    { f: 'back', label: 'Бэк', cls: 'x', html: links('back') },
    { f: 'create_url', label: 'Действия', html: r => `<span class="lc-act">${r.create_url ? `<a href="${esc(r.create_url)}" target="_blank" rel="noopener">Создать продолжение</a>` : ''}<button type="button" data-ignore="${esc(r.key)}" title="Продолжение не нужно: скрыть из списка и больше не уведомлять">Игнорировать</button></span>` },
  ],
  test: [
    { f: 'key', label: 'Задача', cls: 'k', html: key('key') }, { f: 'assignee', label: 'Исполнитель' }, { f: 'role', label: 'Роль' },
    { f: 'days_in_test', label: 'Дн. на тесте', cls: 'num hd' }, { f: 'tester', label: 'Тестировщик' }, { f: 'days_since_start', label: 'Дн. с начала', cls: 'num' },
    { f: 'sprints', label: 'Спринтов', cls: 'num' }, { f: 'is_bug', label: 'Баг' }, { f: 'summary', label: 'Название', cls: 'w' },
  ],
}
const SRC: Record<LCTab, keyof Report> = { blocked: 'blocked', work: 'in_work', people: 'people', gaps: 'gaps', test: 'testing' }
const DEF: Record<LCTab, [string, number]> = { blocked: ['days_blocked', -1], work: ['days_in_status', -1], test: ['days_in_test', -1], people: ['my_stale', -1], gaps: ['priority', 1] }
const sorts = reactive<Record<string, [string, number]>>({})
const sortOf = (t: LCTab) => sorts[t] || DEF[t]

const Q = computed(() => qs('q').toLowerCase())
const ALL = computed(() => qs('all') === '1')
// Строки вкладки с общими фильтрами (без фильтров по человеку).
const baseRowsOf = (t: LCTab) => {
  const rows = (R.value?.[SRC[t]] as any[] | undefined) || []
  const keep = (r: any) => {
    switch (t) {
      case 'work': return ALL.value || r.days_in_status >= 5
      case 'test': return ALL.value || r.days_in_test >= 3
      case 'people': return ALL.value || !!r.signal
      case 'gaps': return (!qs('pr') || r.priority === qs('pr')) && (!qs('kind') || r.kind === qs('kind')) && (!qs('miss') || r.missing === qs('miss'))
    }
    return true
  }
  return rows.filter(r => keep(r) && (!Q.value || COLS[t].some(c => String(r[c.f] ?? '').toLowerCase().includes(Q.value))))
}
const baseRows = computed(() => baseRowsOf(tab.value))
// Исполнитель/сотрудник и тестировщик: значение из адреса, если оно есть среди строк вкладки.
const WHO_LABEL = computed(() => tab.value === 'people' ? 'Сотрудник' : 'Исполнитель')
const NONE_LABEL = computed(() => tab.value === 'people' ? 'не указан' : 'не назначен')
const hasTester = computed(() => tab.value === 'test')
const validWho = computed(() => countBy(baseRows.value, r => personOf(tab.value, r), '').some(o => o.value === qs('who')) ? qs('who') : '')
const validTst = computed(() => hasTester.value && countBy(baseRows.value, testerOf, '').some(o => o.value === qs('tst')) ? qs('tst') : '')
const whoOpts = computed(() => countBy(baseRows.value.filter(r => matchWho(testerOf(r), validTst.value) || !hasTester.value), r => personOf(tab.value, r), NONE_LABEL.value))
const tstOpts = computed(() => countBy(baseRows.value.filter(r => matchWho(personOf(tab.value, r), validWho.value)), testerOf, 'не указан'))
const rows = computed(() => baseRows.value.filter(r => matchWho(personOf(tab.value, r), validWho.value) && (!hasTester.value || matchWho(testerOf(r), validTst.value))))
const rowKey = (r: any) => tab.value === 'people' ? `${r.lead},${r.person}` : `${r.lead},${r.key}`
const NOTE: Record<LCTab, string[]> = {
  blocked: ['Кто кого ждёт: фронт ждёт связанные задачи дизайна и бэка, бэк (и ИИ) ждёт только дизайн, дизайн никого не ждёт. Задачи берутся по связям в Jira. «Ждёт задачу» — первая блокирующая задача, ещё не дошедшая до теста (сначала бэк, потом дизайн); красным — она даже не начата. Если все блокирующие задачи уже на тесте или дальше — «Можно снимать блок» с датой, когда последняя из них ушла на тест.'],
  work: ['«В работе», Code review и Need Rework. Дни в статусе — с последнего перехода в него по истории Jira; «с начала» — с момента, когда задачу взяли в работу. Красным — от 10 дн., оранжевым — от 5.'],
  people: ['Считаются действия самого сотрудника по истории Jira: «взял» — перевёл задачу в «В работе», «завершил» — перевёл в Code review, Test или дальше. «Тормозит» — у него есть задачи этого руководителя в «Сделать», «В работе» или Need Rework без смены статуса ≥ 3 дн., а за последние 7 дней он брал и завершал задачи других руководителей чаще, чем делал что-то по задачам этого руководителя. Тестировщики и ПМ не помечаются.'],
  gaps: ['Только задачи текущего спринта. Плитка показывает число строк этой таблицы; по клику на плитку фильтры сбрасываются. Приоритет: «Высокий» — нет задачи на фронт; «Проверить» — найдена только похожая задача или бэк готов без фронта; «Низкий» — дизайн без бэка, дизайн-задачи без разработки, исправления ошибок.',
    'Пара ищется по связи Relate, по одинаковому названию после «Дизайн -» / «Фронт -» / «БЭК -» и по похожему названию («похоже N%» — проверить вручную). Готовый фронт без бэка проблемой не считается.'],
  test: ['Тестировщик — из поля «Тестировщик» в Jira (в части задач там стоит значение по умолчанию). Дни с последнего перехода в Test. Красным — от 7 дн., оранжевым — от 3.'],
}
const TITLE: Record<LCTab, string> = {
  blocked: 'Заблокированные задачи', work: 'Задачи, которые долго в работе', people: 'Сотрудники: задачи руководителя стоят, а у другого руководителя двигаются',
  gaps: 'Дизайн или бэк готов, а продолжения нет', test: 'Задачи, которые долго на тесте',
}
const ALL_LABEL: Partial<Record<LCTab, string>> = { work: 'показать все, не только ≥ 5 дн.', people: 'показать всех, не только с сигналом', test: 'показать все, не только ≥ 3 дн.' }

// ---------- полоса спринтов ----------
const SC: [keyof SprintRow, string, string][] = [
  ['done', 'Готово', 'var(--ok)'], ['ready', 'Ready for deploy', 'var(--accent)'], ['test', 'Test', 'var(--s-ts)'], ['review', 'Code review', 'var(--s-cr)'],
  ['in_work', 'В работе', 'var(--s-wk)'], ['rework', 'Need Rework', 'var(--warn)'], ['blocked', 'Blocked', 'var(--bad)'], ['todo', 'Сделать', 'var(--s-bl)'],
]
// «Игнорировать» / «Вернуть» — пересчёт отчёта руководителя на сервере и перезагрузка.
const actMsg = ref('')
async function ignore(key: string) {
  actMsg.value = `Скрываю ${key}… (пересчёт занимает несколько секунд)`
  try {
    await $fetch(`${apiBase}/lead-control/gaps/ignore`, { method: 'POST', body: { lead: lead.value, key } })
    actMsg.value = `${key} скрыта из списка`
    await refresh()
  } catch (e: any) { actMsg.value = e?.data?.error || 'Не получилось скрыть' }
}
async function unignore(key: string) {
  actMsg.value = `Возвращаю ${key}…`
  try {
    await $fetch(`${apiBase}/lead-control/gaps/ignore`, { method: 'DELETE', query: { lead: lead.value, key } })
    actMsg.value = `${key} возвращена`
    await refresh()
  } catch (e: any) { actMsg.value = e?.data?.error || 'Не получилось вернуть' }
}
function onTableClick(e: MouseEvent) {
  const k = (e.target as HTMLElement).closest<HTMLElement>('[data-ignore]')?.dataset.ignore
  if (k) ignore(k)
}
// ---------- выгрузка в Excel: то, что на экране (фильтры и сортировка), сервер собирает .xlsx ----------
const GAPS_EXPORT = [
  { f: 'key', label: 'Задача' }, { f: 'summary', label: 'Название' }, { f: 'priority', label: 'Приоритет' }, { f: 'kind', label: 'Тип' },
  { f: 'status', label: 'Статус' }, { f: 'assignee', label: 'Исполнитель' }, { f: 'sprint', label: 'Спринт' }, { f: 'missing', label: 'Чего нет' },
  { f: 'problem', label: 'Проблема' }, { f: 'front', label: 'Фронт' }, { f: 'back', label: 'Бэк' }, { f: 'create_url', label: 'Создать продолжение (ссылка)' },
]
const exportCols = (t: LCTab) => t === 'gaps' ? GAPS_EXPORT : COLS[t].map(c => ({ f: c.f, label: c.label }))
const activeFilters = computed(() => {
  const out: string[] = []
  if (qs('q')) out.push(`Поиск: ${qs('q')}`)
  if (tab.value === 'work' && !ALL.value) out.push('только ≥ 5 дн. в статусе')
  if (tab.value === 'test' && !ALL.value) out.push('только ≥ 3 дн. на тесте')
  if (tab.value === 'people' && !ALL.value) out.push('только с сигналом')
  if (tab.value === 'gaps') for (const [k, l] of [['pr', 'Приоритет'], ['miss', 'Чего нет'], ['kind', 'Тип']] as const) if (qs(k)) out.push(`${l}: ${qs(k)}`)
  const label = (opts: { value: string; label: string }[], v: string) => opts.find(o => o.value === v)?.label ?? v
  if (validWho.value) out.push(`${WHO_LABEL.value}: ${label(whoOpts.value, validWho.value)}`)
  if (validTst.value) out.push(`Тестировщик: ${label(tstOpts.value, validTst.value)}`)
  return out
})
const exporting = ref(false)
async function exportExcel() {
  exporting.value = true; actMsg.value = ''
  try {
    const cols = exportCols(tab.value)
    const sorted = sortRows(rows.value, sortOf(tab.value))
    const keyCol = cols.findIndex(c => c.f === 'key')
    const res = await $fetch.raw<Blob>(`${apiBase}/lead-control/export`, {
      method: 'POST', responseType: 'blob',
      body: {
        title: TITLE[tab.value], lead: lead.value, project: proj.value === 'all' ? 'Все проекты' : (projects.value.find(p => p.key === proj.value)?.title || proj.value) + ` (${proj.value})`,
        filters: activeFilters.value, updated: updated.value, columns: cols.map(c => c.label), key_col: keyCol >= 0 ? keyCol : null,
        rows: sorted.map(r => cols.map(c => r[c.f] ?? null)),
      },
    })
    const cd = res.headers.get('content-disposition') || ''
    const star = /filename\*=UTF-8''([^;]+)/i.exec(cd)
    const name = star ? decodeURIComponent(star[1]) : 'lead-control.xlsx'
    const href = URL.createObjectURL(res._data as Blob)
    const a = Object.assign(document.createElement('a'), { href, download: name })
    document.body.appendChild(a); a.click(); a.remove()
    setTimeout(() => URL.revokeObjectURL(href), 10000)
    actMsg.value = `Файл «${name}» скачан (${sorted.length} строк)`
  } catch (e: any) {
    actMsg.value = e?.data?.error || (e?.statusCode ? `Не удалось выгрузить (HTTP ${e.statusCode})` : 'Не удалось выгрузить')
  } finally { exporting.value = false }
}
const showIgnored = ref(false)
const fmtAt = (s: string) => new Date(s).toLocaleString('ru-RU', { timeZone: 'Asia/Almaty', day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
useHead({ title: 'Контроль спринта — Отчёты Adata' })
</script>

<template>
  <div class="lc">
    <header class="lc-head">
      <div>
        <h1>Проекты руководителей · контроль спринта</h1>
        <p class="meta">Данные из Jira обновлены: <b>{{ updated }}</b> (Алматы)</p>
        <p v-if="R && T" class="lc-take">
          <template v-if="T.blocked"><b>{{ T.blocked_wait }}</b> из <b>{{ T.blocked }}</b> заблокированных задач ждут задачу, которая ещё не начата; с <b>{{ T.can_unblock }}</b> блок можно снимать. </template>
          Сотрудников, у которых здесь задачи стоят, а у других руководителей двигаются: <b>{{ T.stalled_people }}</b>.
        </p>
      </div>
    </header>

    <div v-if="leadsErr" class="notice">Не удалось получить список руководителей: {{ leadsErr.statusCode ? `HTTP ${leadsErr.statusCode}` : leadsErr.message }}</div>
    <div v-else-if="leads && !leads.length" class="notice">Вас нет в справочнике руководителей. Администратор добавляет руководителей и их проекты в «Настройках».</div>
    <template v-else-if="leads">
      <div class="lc-bar" role="group" aria-label="Руководитель">
        <span class="lc-lbl">Руководитель</span>
        <button v-for="l in leads" :key="l.lead" type="button" class="lc-chip" :aria-pressed="l.lead === lead" @click="setQuery({ lead: l.lead, proj: undefined, who: undefined, tst: undefined })">{{ l.lead }}</button>
      </div>
      <div class="lc-bar" role="group" aria-label="Проект">
        <span class="lc-lbl">Проект</span>
        <button type="button" class="lc-chip" :aria-pressed="proj === 'all'" @click="setQuery({ proj: undefined })">Все проекты</button>
        <button v-for="p in projects" :key="p.key" type="button" class="lc-chip" :aria-pressed="proj === p.key" @click="setQuery({ proj: p.key })">{{ p.title }} ({{ p.key }})</button>
      </div>

      <div class="lc-tiles" role="tablist">
        <button v-for="t in TILES" :key="t.tab" type="button" role="tab" class="lc-tile" :aria-selected="tab === t.tab" @click="openTab(t.tab)">
          <span class="l">{{ t.l }}</span><span class="n">{{ T ? T[t.f] : '—' }}</span><span class="s">{{ t.s }}</span>
        </button>
      </div>

      <div v-if="error" class="notice">{{ (error.data as { error?: string } | undefined)?.error || `Не удалось загрузить: ${error.statusCode ? 'HTTP ' + error.statusCode : error.message}` }}</div>
      <section v-else class="lc-card">
        <h2>{{ TITLE[tab] }}</h2>
        <p v-for="(n, i) in NOTE[tab]" :key="i" class="lc-note">{{ n }}</p>
        <div class="lc-tools">
          <template v-if="tab === 'gaps'">
            <label>Приоритет <select :value="qs('pr')" @change="setQuery({ pr: ($event.target as HTMLSelectElement).value })"><option value="">все</option><option>Будущий спринт</option><option>Высокий</option><option>Проверить</option><option>Низкий</option></select></label>
            <label>Чего нет <select :value="qs('miss')" @change="setQuery({ miss: ($event.target as HTMLSelectElement).value })"><option value="">всё</option><option>Нет фронта и бэка</option><option>Нет фронта</option><option>Нет бэка</option></select></label>
            <label>Тип <select :value="qs('kind')" @change="setQuery({ kind: ($event.target as HTMLSelectElement).value })"><option value="">все</option><option>Дизайн</option><option>Бэк</option></select></label>
          </template>
          <label v-if="ALL_LABEL[tab]"><input type="checkbox" :checked="ALL" @change="setQuery({ all: ($event.target as HTMLInputElement).checked ? '1' : undefined })"> {{ ALL_LABEL[tab] }}</label>
          <label>{{ WHO_LABEL }}
            <select :value="validWho" :aria-label="WHO_LABEL" @change="setQuery({ who: ($event.target as HTMLSelectElement).value })">
              <option value="">все</option>
              <option v-for="o in whoOpts" :key="o.value" :value="o.value">{{ o.label }} ({{ o.n }})</option>
            </select>
          </label>
          <label v-if="hasTester">Тестировщик
            <select :value="validTst" aria-label="Тестировщик" @change="setQuery({ tst: ($event.target as HTMLSelectElement).value })">
              <option value="">все</option>
              <option v-for="o in tstOpts" :key="o.value" :value="o.value">{{ o.label }} ({{ o.n }})</option>
            </select>
          </label>
          <input type="search" :value="qs('q')" placeholder="Поиск" aria-label="Поиск" @input="setQuery({ q: ($event.target as HTMLInputElement).value })">
          <span class="sub">{{ rows.length }} строк</span>
          <button type="button" class="btn sm" :disabled="exporting || !rows.length" title="Скачать таблицу в Excel — с текущими фильтрами и сортировкой" @click="exportExcel">{{ exporting ? 'Готовлю файл…' : 'Скачать в Excel' }}</button>
          <span v-if="actMsg" class="sub" role="status">{{ actMsg }}</span>
        </div>
        <p v-if="pending && !R" class="lc-empty">Загрузка…</p>
        <LcTable v-else :cols="COLS[tab]" @click="onTableClick" :rows="rows" :sort="sortOf(tab)" :row-class="HOT[tab] as (r: any) => string" :row-key="rowKey" :fit="tab === 'gaps'"
                 empty="Нет задач по этому фильтру." @sort="s => (sorts[tab] = s)" />
        <div v-if="tab === 'gaps' && R?.ignored?.length" class="lc-ignored">
          <button type="button" class="linkbtn" @click="showIgnored = !showIgnored">{{ showIgnored ? 'Скрыть' : 'Показать' }} скрытые («продолжение не нужно»): {{ R.ignored.length }}</button>
          <table v-if="showIgnored" class="lc-t">
            <thead><tr><th>Задача</th><th>Название</th><th>Скрыл</th><th>Когда</th><th /></tr></thead>
            <tbody>
              <tr v-for="x in R.ignored" :key="x.key">
                <td class="k"><a :href="`${jiraBase.replace(/\/$/, '')}/browse/${x.key}`" target="_blank" rel="noopener">{{ x.key }}</a></td>
                <td class="w">{{ x.summary }}</td><td>{{ x.by || '—' }}</td><td>{{ fmtAt(x.at) }}</td>
                <td><button type="button" class="linkbtn" @click="unignore(x.key)">Вернуть</button></td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section v-if="R" class="lc-card">
        <h2>Открытые спринты</h2>
        <p class="lc-note">Распределение задач текущего спринта по статусам.</p>
        <div class="lc-sp">
          <div v-for="s in R.sprints" :key="s.project + s.sprint" class="row">
            <div class="nm"><span>{{ s.project }} · {{ s.sprint }}</span><span>{{ s.total }} задач</span></div>
            <div class="st"><i v-for="c in SC" v-show="s[c[0]]" :key="c[0]" :style="{ width: (s[c[0]] as number) / s.total * 100 + '%', background: c[2] }" :title="`${c[1]}: ${s[c[0]]}`" /></div>
          </div>
          <p v-if="!R.sprints.length" class="lc-empty">Открытых спринтов нет.</p>
        </div>
        <div class="lc-lg"><span v-for="c in SC" :key="c[0]"><i :style="{ background: c[2] }" />{{ c[1] }}</span></div>
      </section>
    </template>
  </div>
</template>
