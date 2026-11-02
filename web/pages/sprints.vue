<script setup lang="ts">
// Отчёт «Спринты» — повторяет reference/sprints/page_template.html. Колонки вкладок перенесены оттуда 1-в-1.
// Вкладки по людям и «Отсутствия» — в components/SprintsPeople.vue.
import { type ApiReport, type Col, type P, UNASSIGNED, buildModel, esc, fmt, jiraLinks, lvl, pct, sumOf } from '~/utils/sprints'
import { defaultCtl, peopleLogic, switchTip } from '~/utils/sprintsPeople'

const { apiBase, jiraBase } = useRuntimeConfig().public
const route = useRoute()
const router = useRouter()
// Источник — живой отчёт или сохранённый снепшот (?snap=ГГГГ-ММ-ДД): снепшот — замороженная копия того же ответа.
type SnapInfo = { taken_for: string; history_end: string; note: string; created_by: string; created_at: string }
const snap = computed(() => typeof route.query.snap === 'string' ? route.query.snap : '')
const { data: snaps, refresh: refreshSnaps } = await useFetch<SnapInfo[]>(`${apiBase}/snapshots`)
const { data: raw, error, pending, refresh } = await useFetch<any>(
  () => snap.value ? `${apiBase}/snapshots/${snap.value}` : `${apiBase}/sprints/report`, { watch: [snap] })
const data = computed<ApiReport | null>(() => (snap.value ? raw.value?.snapshot : raw.value) ?? null)
const snapInfo = computed<SnapInfo | null>(() => snap.value ? raw.value?.info ?? null : null)

// ---------- снепшоты: сохранить (на сегодня), открыть, удалить — admin и lead ----------
const { me } = useMe()
const canSnap = computed(() => canSeeSnapshots(me.value))
const snapNote = ref('')
const snapBusy = ref(false)
const snapMsg = ref('')
const dmy = (d: string) => d.slice(8, 10) + '.' + d.slice(5, 7) + '.' + d.slice(0, 4)
function openSnap(d: string) { router.replace({ query: { ...route.query, snap: d || undefined } }) }
async function saveSnap() {
  snapBusy.value = true; snapMsg.value = ''
  try {
    const r = await $fetch<{ date: string }>(`${apiBase}/snapshots`, { method: 'POST', body: { note: snapNote.value.trim() } })
    snapNote.value = ''
    await refreshSnaps()
    snapMsg.value = `Снепшот на ${dmy(r.date)} сохранён`
  } catch (e: any) {
    snapMsg.value = e?.data?.error || (e?.statusCode ? `Ошибка ${e.statusCode}` : 'Сервис недоступен')
  } finally { snapBusy.value = false }
}
async function deleteSnap() {
  const d = snap.value
  if (!d || !confirm(`Удалить снепшот на ${dmy(d)}? Восстановить его будет нельзя.`)) return
  snapBusy.value = true; snapMsg.value = ''
  try {
    await $fetch(`${apiBase}/snapshots/${d}`, { method: 'DELETE' })
    await refreshSnaps()
    openSnap('')
    snapMsg.value = `Снепшот на ${dmy(d)} удалён`
  } catch (e: any) {
    snapMsg.value = e?.data?.error || (e?.statusCode ? `Ошибка ${e.statusCode}` : 'Сервис недоступен')
  } finally { snapBusy.value = false }
}

const m = computed(() => data.value?.updated_at ? buildModel(data.value) : null)

// ---------- фильтр по руководителю и вкладка (в query-строке) ----------
const lead = ref(String(route.query.lead ?? ''))
const TABS = [
  ['dash', 'Дэшборды'], ['sprint', 'Статус спринтов'], ['people', 'Загрузка ресурсов'], ['lres', 'Ресурсы по руководителям'], ['abs', 'Отсутствия'],
  ['need', 'Нужны задачи'], ['over', 'Перегружены'], ['switch', 'Переключаются между проектами'], ['tl', 'Тимлиды и бэклог'],
  ['qa', 'Загрузка QA'], ['block', 'Узкие места'], ['stab', 'Стабильность'], ['qual', 'Качество'],
  ['rel', 'Готовность к релизу'], ['next', 'Следующий спринт'], ['dq', 'Качество данных'],
] as const
const PEOPLE_TABS = ['dash', 'people', 'lres', 'abs', 'need', 'over', 'switch', 'tl']
const tab = ref(String(route.query.tab ?? 'dash'))
watch([lead, tab], ([l, t]) => router.replace({ query: { ...route.query, lead: l || undefined, tab: t === 'dash' ? undefined : t } }))

const PR = computed(() => m.value?.PR ?? [])
const leads = computed(() => [...new Set(PR.value.map(p => p.lead))].sort((a, b) => a.localeCompare(b, 'ru'))
  .map(l => ({ l, n: PR.value.filter(p => p.lead === l).length })))
const scopedP = computed(() => lead.value ? PR.value.filter(p => p.lead === lead.value) : PR.value)
const scope = computed(() => new Set(scopedP.value.map(p => p.k)))
const inS = (k: string) => scope.value.has(k)
const L = computed(() => jiraLinks(jiraBase.replace(/\/$/, '') + '/', m.value!, () => [...scope.value]))

const updated = computed(() => {
  const u = data.value?.updated_at
  if (!u) return ''
  const d = new Date(u), p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getDate())}.${p(d.getMonth() + 1)}.${d.getFullYear()} ${p(d.getHours())}:${p(d.getMinutes())}`
})

// ---------- пороги вкладок по людям и их логика ----------
const ctl = useState('sprints-ctl', defaultCtl)
const pl = computed(() => peopleLogic(m.value!, ctl.value, inS, () => scope.value.size === PR.value.length))

// ---------- KPI ----------
const kpis = computed(() => {
  const P = scopedP.value, T = sumOf(P, 't'), stuck = P.filter(p => p.left < 0).length, NSD = m.value!.NSD
  const people = Object.keys(m.value!.PEOPLE), need = people.filter(pl.value.needVisible).length, sw = people.filter(pl.value.switchVisible).length
  return [
    [T, 'задач в спринтах', '', 'sprint'], [P.length, 'проектов', '', 'sprint'], [pct(sumOf(P, 'dn'), T) + '%', 'готово', '', 'rel'],
    [stuck, 'спринтов не закрыто', stuck ? 'bad' : '', 'sprint'],
    [need, 'сотрудникам нужны задачи', need ? 'warn' : '', 'need'], [people.filter(pl.value.overVisible).length, 'перегружены бэклогом', 'bad', 'over'],
    [sw, 'тормозят чужой проект', sw ? 'bad' : '', 'switch'],
    [pl.value.absVisibleRisk(), 'отсутствуют, а задачи висят', pl.value.absVisibleRisk() ? 'bad' : '', 'abs'],
    [P.filter(p => !NSD[p.k] || !NSD[p.k].t).length, 'нет следующего спринта (или пустой)', P.some(p => (!NSD[p.k] || !NSD[p.k].t) && p.left <= 3) ? 'bad' : '', 'next'],
    [sumOf(P, 'blk'), 'в Blocked', 'bad', 'block'], [sumOf(P, 'bl'), 'в бэклоге', '', 'people'],
  ] as [string | number, string, string, string][]
})

// ---------- подсказка «почему тормозит проект» (как .tt в шаблоне) ----------
const tip = reactive({ html: '', x: 0, y: 0, show: false })
let tipEl: HTMLElement | null = null
let tipT: ReturnType<typeof setTimeout> | undefined
const ttRef = ref<HTMLElement | null>(null)
async function tipShow(el: HTMLElement) {
  clearTimeout(tipT)
  if (tipEl === el && tip.show) return
  tipEl = el; tip.html = switchTip(el.dataset.tipn!, pl.value, L.value); tip.show = true
  await nextTick()
  const r = el.getBoundingClientRect(), w = ttRef.value?.offsetWidth ?? 380, h = ttRef.value?.offsetHeight ?? 200
  tip.x = Math.min(Math.max(8, r.left), innerWidth - w - 8)
  tip.y = r.bottom + 4 + h > innerHeight - 8 ? Math.max(8, r.top - h - 4) : r.bottom + 4
}
function tipHide() { clearTimeout(tipT); tip.show = false; tipEl = null }
const onOver = (e: MouseEvent) => { const t = e.target as HTMLElement; const el = t.closest<HTMLElement>('.has-tip'); if (el) tipShow(el); else if (ttRef.value?.contains(t)) clearTimeout(tipT) }
const onOut = (e: MouseEvent) => {
  const t = e.target as HTMLElement, from = t.closest('.has-tip') || (ttRef.value?.contains(t) ? ttRef.value : null)
  if (!from) return
  const to = e.relatedTarget as Node | null
  if (to && (ttRef.value?.contains(to) || tipEl?.contains(to))) return
  clearTimeout(tipT); tipT = setTimeout(tipHide, 350)
}
const onClickDoc = (e: MouseEvent) => {
  const t = e.target as HTMLElement
  if (ttRef.value?.contains(t)) return
  const el = t.closest<HTMLElement>('.has-tip')
  if (el) { tipEl === el && tip.show ? tipHide() : tipShow(el) } else tipHide()
}
const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') tipHide() }
const onScroll = (e: Event) => { if (!ttRef.value?.contains(e.target as Node)) tipHide() }
onMounted(() => {
  document.addEventListener('mouseover', onOver); document.addEventListener('mouseout', onOut)
  document.addEventListener('click', onClickDoc); document.addEventListener('keydown', onKey); addEventListener('scroll', onScroll, true)
})
onBeforeUnmount(() => {
  document.removeEventListener('mouseover', onOver); document.removeEventListener('mouseout', onOut)
  document.removeEventListener('click', onClickDoc); document.removeEventListener('keydown', onKey); removeEventListener('scroll', onScroll, true)
})

// ---------- общие ячейки ----------
const projCell = (p: P) => `<b>${esc(p.k)}</b> <span class="sub">${esc(p.n)}</span>`
const leadCell = (p: P) => `<span class="sub">${esc(p.lead)}</span>`
const pq = (k: string) => `project = "${k}"`

// ---------- 1 статус спринтов ----------
const SEG = [['dn', 'Готово'], ['ts', 'Тестируется'], ['cr', 'Код ревью'], ['rw', 'Реворк'], ['wk', 'В работе'], ['bl', 'Бэклог']] as const
const sprintCols = computed<Col<P>[]>(() => {
  const { jq, nl, ST, JB } = L.value, NSD = m.value!.NSD
  return [
    { h: 'Проект', v: p => p.k, f: projCell },
    { h: 'Руководитель', v: p => p.lead, f: leadCell },
    { h: 'Спринт', v: p => p.sp, f: p => esc(p.sp) },
    { h: 'Даты', v: p => p.en, f: p => `${fmt(p.st)} – ${fmt(p.en)}` },
    { h: 'Срок', v: p => p.left, num: true, f: p => Number.isNaN(p.left) ? '<span class="sub">—</span>' : p.left < 0 ? `<span class="pill bad">Не закрыт · ${-p.left} дн.</span>` : p.left <= 3 ? `<span class="pill warn">${p.left} дн.</span>` : `<span class="pill mute">${p.left} дн.</span>` },
    { h: 'Следующий спринт', v: p => NSD[p.k] ? (NSD[p.k].t ? 2 : 1) : 0, cls: 'wrap', tip: 'Есть ли в Jira следующий спринт (номер +1 к текущему) и сколько в нём задач. Подробнее — вкладка «Следующий спринт»',
      f: p => { const n = NSD[p.k]; if (!n) return `<span class="pill ${p.left <= 3 ? 'bad' : 'mute'}">не создан</span>`; return (n.t ? `<span class="pill ok">есть</span> <a class="nl" href="${JB}issues/?jql=${encodeURIComponent('cf[10104] = ' + n.id)}" target="_blank" rel="noopener" title="${esc(n.name)}">${n.t} задач</a>` : `<span class="pill ${p.left <= 3 ? 'bad' : 'warn'}">пустой</span>`) + `<div class="sub">${esc(n.name)}</div>` } },
    { h: 'Задач', v: p => p.t, num: true, f: p => nl(p.t, jq([pq(p.k)])) },
    { h: 'Прогресс', v: p => p.done, f: p => `<div class="bar" title="${SEG.map(([k, l]) => l + ' ' + p[k]).join(' · ')}">${SEG.map(([k]) => `<i style="width:${p[k] / p.t * 100}%;background:var(--s-${k})"></i>`).join('')}</div>` },
    { h: 'Готово', v: p => p.done, num: true, f: p => p.done + '%' },
    { h: 'Самая старая задача', v: p => p.oAge, num: true, tip: 'Самая давно созданная незакрытая задача спринта: сколько дней назад создана', f: p => p.oKey ? `<a class="key" href="${JB}browse/${p.oKey}" target="_blank" rel="noopener">${p.oKey}</a> <span class="${lvl(p.oAge, 30, 90)}">${p.oAge} дн.</span>` : '<span class="sub">—</span>' },
    { h: 'Переходят из спринта в спринт', v: p => p.carryOpen, num: true, tip: 'Незакрытые задачи, которые были минимум в 2 спринтах (перенесены из закрытого спринта)', f: p => nl(p.carryOpen, jq([pq(p.k), 'cf[10104] in closedSprints()', ST.open]), lvl(pct(p.carryOpen, p.t), 20, 40)) + (p.carryOpen ? ` <span class="sub">${pct(p.carryOpen, p.t)}%</span>` : '') },
    { h: 'Люди', v: p => p.ppl, num: true },
  ]
})

// ---------- QA ----------
type Q = { n: string; t: number; tst: number; dn: number; pr: string[] }
const qaRows = computed(() => { const M: Record<string, Q> = {}; m.value!.QX.forEach(r => { if (!inS(r.project_key)) return; const q = M[r.name] ??= { n: r.name, t: 0, tst: 0, dn: 0, pr: [] }; q.t += r.t; q.tst += r.tst; q.dn += r.dn; q.pr.push(r.project_key) }); return Object.values(M) })
const qaCols = computed<Col<Q>[]>(() => {
  const { jq, nl, who, projQ, projList, ST } = L.value
  return [
    { h: 'Тестировщик', v: q => q.n, f: q => esc(q.n) + (q.pr.length > 20 ? ' <span class="pill warn">похоже на значение по умолчанию</span>' : '') },
    { h: 'Задач', v: q => q.t, num: true, f: q => nl(q.t, jq([who(q.n, 'cf[10902]'), projQ()])) },
    { h: 'Сейчас в Test', v: q => q.tst, num: true, f: q => nl(q.tst, jq([who(q.n, 'cf[10902]'), projQ(), 'status = "Test"']), lvl(q.tst, 20, 40)) },
    { h: 'Готово', v: q => q.dn, num: true, f: q => nl(q.dn, jq([who(q.n, 'cf[10902]'), projQ(), ST.dn])) },
    { h: 'Проекты', v: q => q.pr.length, num: true, cls: 'wrap', f: q => projList(q.n, q.pr, 'cf[10902]') },
  ]
})

// ---------- узкие места ----------
type XB = { k: string; s: string; st: string; a: string; d: number; pr: string; p: string }
const XP = computed<XB[]>(() => m.value!.X.map(x => ({ k: x.key, s: x.summary, st: x.status, a: x.assignee, d: x.days, pr: x.priority, p: x.key.split('-')[0] })))
const blockSt = computed(() => [...new Set(XP.value.map(x => x.st))])
const fSt = ref('')
const qBlock = ref('')
const blockRows = computed(() => XP.value.filter(x => inS(x.p) && (!fSt.value || x.st === fSt.value) && (!qBlock.value || (x.k + ' ' + x.s + ' ' + x.a).toLowerCase().includes(qBlock.value.toLowerCase()))))
const stPill = (s: string) => `<span class="pill ${s === 'Blocked' ? 'bad' : s === 'Need Rework' || s === 'Требует уточнения' ? 'warn' : 'mute'}">${esc(s)}</span>`
const blockCols = computed<Col<XB>[]>(() => [
  { h: 'Ключ', v: x => x.k, f: x => `<a class="key" href="${L.value.JB}browse/${x.k}" target="_blank" rel="noopener">${x.k}</a>` },
  { h: 'Задача', v: x => x.s, cls: 'wrap' },
  { h: 'Статус', v: x => x.st, f: x => stPill(x.st) },
  { h: 'Исполнитель', v: x => x.a },
  { h: 'Руководитель', v: x => m.value!.LEAD(x.p), f: x => `<span class="sub">${esc(m.value!.LEAD(x.p))}</span>` },
  { h: 'Без обновлений, дн.', v: x => x.d, num: true, f: x => `<span class="${lvl(x.d, 14, 30)}">${x.d}</span>` },
  { h: 'Приоритет', v: x => x.pr, f: x => /высокий|Неотложный/.test(x.pr) ? `<span class="pill bad">${esc(x.pr)}</span>` : `<span class="sub">${esc(x.pr)}</span>` },
])

// ---------- стабильность / качество / релиз ----------
const stabCols: Col<P>[] = [
  { h: 'Проект', v: p => p.k, f: projCell }, { h: 'Руководитель', v: p => p.lead, f: leadCell },
  { h: 'Задач', v: p => p.t, num: true },
  { h: 'Перенесено', v: p => p.carry, num: true },
  { h: '% перенесено', v: p => pct(p.carry, p.t), num: true, f: p => `<span class="${lvl(pct(p.carry, p.t), 40, 60)}">${pct(p.carry, p.t)}%</span>` },
  { h: 'Добавлено после старта', v: p => p.add, num: true },
  { h: '% добавлено', v: p => pct(p.add, p.t), num: true, f: p => `<span class="${lvl(pct(p.add, p.t), 40, 70)}">${pct(p.add, p.t)}%</span>` },
  { h: 'Внеплановые', v: p => p.unpl, num: true },
  { h: 'Без обновлений >14 дн.', v: p => p.stale, num: true, f: p => `<span class="${lvl(p.stale, 3, 10)}">${p.stale}</span>` },
]
const qualCols = computed<Col<P>[]>(() => {
  const { jq, nl, ST } = L.value
  return [
    { h: 'Проект', v: p => p.k, f: projCell }, { h: 'Руководитель', v: p => p.lead, f: leadCell },
    { h: 'Задач', v: p => p.t, num: true },
    { h: 'Ошибок', v: p => p.bug, num: true, f: p => nl(p.bug, jq([pq(p.k), 'issuetype = "Ошибка"'])) },
    { h: 'Доля ошибок', v: p => pct(p.bug, p.t), num: true, f: p => `<span class="${lvl(pct(p.bug, p.t), 25, 40)}">${pct(p.bug, p.t)}%</span>` },
    { h: 'Need Rework', v: p => p.rw, num: true, f: p => nl(p.rw, jq([pq(p.k), ST.rw]), 'mid') },
    { h: 'Blocked', v: p => p.blk, num: true, f: p => nl(p.blk, jq([pq(p.k), 'status = "Blocked"']), 'hi') },
  ]
})
const tstKind = (n: string) => { if (!n) return 'qa'; const r = m.value!.roleOf(n); return /QA|тест|test/i.test(r) ? 'qa' : /Project Manager|менеджер/i.test(r) ? 'pm' : 'ot' }
const tstSplit = (k: string) => {
  const o = { qa: { n: 0, who: [] as string[] }, pm: { n: 0, who: [] as string[] }, ot: { n: 0, who: [] as string[] } }
  Object.entries(m.value!.TST[k] || {}).forEach(([n, v]) => { const g = o[tstKind(n)]; g.n += v.n; if (!n) g.who.push(''); else v.logins.forEach(l => g.who.push(l)) })
  return o
}
const tstJq = (k: string, who: string[]) => {
  const l = who.filter(Boolean).map(x => '"' + x + '"'), un = who.includes('')
  const a = l.length && un ? `(cf[10902] in (${l.join(', ')}) OR cf[10902] is EMPTY)` : l.length ? `cf[10902] in (${l.join(', ')})` : un ? 'cf[10902] is EMPTY' : ''
  return L.value.jq([pq(k), 'status = "Test"', a])
}
const relFin = (p: P) => pct(p.dn + p.rdy + p.tst + p.cr, p.t)
const relCols = computed<Col<P>[]>(() => {
  const { jq, nl, ST } = L.value
  return [
    { h: 'Проект', v: p => p.k, f: projCell }, { h: 'Руководитель', v: p => p.lead, f: leadCell },
    { h: 'Задач', v: p => p.t, num: true, f: p => nl(p.t, jq([pq(p.k)])) },
    { h: 'Готовность спринта', v: p => p.done, num: true, tip: 'Доля задач спринта в статусе «Готово». Светлая часть полосы — задачи на финише (Code review, Test, Ready for deploy / Staging). Красным — если готовность отстаёт от прошедшего времени спринта больше чем на 25 п.п.',
      f: p => { const fin = relFin(p), lag = p.time - p.done; return `<div style="display:flex;align-items:center;gap:8px;min-width:170px"><div class="bar" style="flex:1" title="Готово ${p.done}% · на финише ещё ${fin - p.done}% · прошло времени ${p.time}%"><i style="width:${p.done}%;background:var(--s-dn)"></i><i style="width:${fin - p.done}%;background:var(--s-dn);opacity:.35"></i></div><b class="${lag > 25 ? 'hi' : lag > 10 ? 'mid' : ''}" style="min-width:3.2em;text-align:right">${p.done}%</b></div>` + (fin > p.done ? `<span class="sub">с финишем ${fin}%</span>` : '') } },
    { h: 'Code review', v: p => p.cr, num: true, f: p => nl(p.cr, jq([pq(p.k), ST.cr])) },
    { h: 'На тесте у тестера', v: p => tstSplit(p.k).qa.n, num: true, tip: 'Задачи в статусе Test, поле «Тестировщик» — QA Engineer или пустое', f: p => { const x = tstSplit(p.k).qa; return nl(x.n, tstJq(p.k, x.who)) } },
    { h: 'На тесте у PM', v: p => tstSplit(p.k).pm.n, num: true, tip: 'Задачи в статусе Test, поле «Тестировщик» — Project Manager', f: p => { const x = tstSplit(p.k).pm; return nl(x.n, tstJq(p.k, x.who), 'mid') } },
    { h: 'На тесте у тимлида / остальных', v: p => tstSplit(p.k).ot.n, num: true, tip: 'Задачи в статусе Test, поле «Тестировщик» — тимлид или другой сотрудник (не QA и не PM)', f: p => { const x = tstSplit(p.k).ot; return nl(x.n, tstJq(p.k, x.who), x.n ? 'hi' : '') } },
    { h: 'Ready for deploy / Staging', v: p => p.rdy, num: true, f: p => p.rdy ? `<b>${p.rdy}</b>` : '0' },
    { h: 'Готово', v: p => p.dn, num: true },
  ]
})

// ---------- следующий спринт ----------
type NR = P & { ns: ReturnType<typeof buildModel>['NSD'][string] | undefined; carryN: number; idle: string[]; state: string; risk: boolean }
const nextRows = computed<NR[]>(() => scopedP.value.map(p => {
  const n = m.value!.NSD[p.k], carryN = p.t - p.dn
  const ppl = Object.values(m.value!.PEOPLE).filter(Pp => Pp.n !== UNASSIGNED && Pp.rows.some(r => r.k === p.k && r.t > 0)).map(Pp => Pp.n)
  const idle = n ? ppl.filter(x => !n.by[x]) : []
  const state = !n ? 'none' : !n.t ? 'empty' : 'ok'
  return { ...p, ns: n, carryN, idle, state, risk: state !== 'ok' && p.left <= 3 }
}))
const nsPill = (x: NR) => x.state === 'none' ? `<span class="pill ${x.risk ? 'bad' : 'mute'}">не создан</span>` : x.state === 'empty' ? `<span class="pill ${x.risk ? 'bad' : 'warn'}">пустой</span>` : '<span class="pill ok">готов</span>'
const nextCols = computed<Col<NR>[]>(() => {
  const { jq, jqs, nl, who } = L.value
  return [
    { h: 'Проект', v: p => p.k, f: projCell }, { h: 'Руководитель', v: p => p.lead, f: leadCell },
    { h: 'Текущий до', v: p => p.left, num: true, tip: 'Дата окончания текущего спринта; красным — уже прошла', f: p => `${fmt(p.en)} <span class="${p.left < 0 ? 'hi' : p.left <= 3 ? 'mid' : 'sub'}">${p.left < 0 ? 'просрочен' : p.left === 0 ? 'сегодня' : 'ещё ' + p.left + ' дн.'}</span>` },
    { h: 'Перейдёт', v: p => p.carryN, num: true, tip: 'Незакрытые задачи текущего спринта — перейдут в следующий, если не закрыть', f: p => nl(p.carryN, jq([pq(p.k), 'status != 10001']), p.left <= 3 && p.carryN ? 'mid' : '') },
    { h: 'Следующий спринт', v: p => p.state === 'ok' ? 2 : p.state === 'empty' ? 1 : 0, cls: 'wrap', f: p => nsPill(p) + (p.ns ? ` <span>${esc(p.ns.name)}</span>` + (p.ns.st ? ` <span class="sub">${fmt(p.ns.st)}–${fmt(p.ns.en)}</span>` : ' <span class="sub">даты не заданы</span>') : '') },
    { h: 'Запланировано', v: p => p.ns ? p.ns.t : -1, num: true, tip: 'Задач уже в следующем спринте', f: p => p.ns ? (p.ns.t ? `<a class="nl" href="${jqs(p.ns.id)}" target="_blank" rel="noopener">${p.ns.t}</a>` : '<span class="sub">0</span>') : '<span class="sub">—</span>' },
    { h: 'Статусы', v: p => p.ns ? p.ns.t : 0, cls: 'wrap', f: p => p.ns && p.ns.t ? Object.entries(p.ns.ss).map(([s, c]) => `<span class="sub">${esc(s)} ${c}</span>`).join(' · ') : '<span class="sub">—</span>' },
    { h: 'Исполнители в следующем', v: p => p.ns ? Object.keys(p.ns.by).length : 0, num: true, cls: 'wrap', f: p => p.ns && p.ns.t ? `<div class="plist">${Object.entries(p.ns.by).sort((a, b) => b[1] - a[1]).map(([n, c]) => `<a class="who wl" href="${jqs(p.ns!.id, [who(n)])}" target="_blank" rel="noopener">${esc(n)} <b>${c}</b></a>`).join('')}</div>` : '<span class="sub">—</span>' },
    { h: 'Без задач в следующем', v: p => p.idle.length, num: true, cls: 'wrap', tip: 'Работают в текущем спринте проекта, а в следующем задач нет', f: p => p.ns ? (p.idle.length ? `<div class="plist"><b class="pc">${p.idle.length}</b>${p.idle.map(n => `<span class="who">${esc(n)}</span>`).join('')}</div>` : '<span class="sub">все с задачами</span>') : '<span class="sub">—</span>' },
  ]
})
const nsSum = computed(() => {
  const r = nextRows.value, riskNone = r.filter(x => x.risk && x.state === 'none').length
  return `<span>Следующий создан: <b>${r.filter(x => x.ns).length}</b> из ${r.length}</span><span>Не создан, а текущий кончается ≤3 дн. или просрочен: <b class="${riskNone ? 'hi' : ''}">${riskNone}</b></span><span>Пустых: <b>${r.filter(x => x.state === 'empty').length}</b></span><span>Перейдёт задач: <b>${sumOf(r, 'carryN')}</b></span><span>Людей без задач в следующем: <b>${r.reduce((s, x) => s + x.idle.length, 0)}</b></span>`
})

// ---------- качество данных (тексты карточек — из текущего дашборда) ----------
const notes = computed(() => {
  const P = scopedP.value, T = sumOf(P, 't'), stuck = P.filter(p => p.left < 0)
  return [
    ['Оценки времени', '0 из ' + T, 'Ни у одной задачи нет первоначальной оценки. План/факт и burndown по часам построить нельзя.'],
    ['Срок исполнения (duedate)', 'не заполнен', 'Без сроков нельзя посчитать просрочки по задачам.'],
    ['Списание времени', Math.round(sumOf(P, 'h')).toLocaleString('ru') + ' ч', 'Время списывают не во всех проектах; для отчёта по трудозатратам данных мало.'],
    ['Незакрытые спринты', stuck.length + '', stuck.length ? 'Даты прошли, но спринты активны: ' + stuck.map(p => p.k).join(', ') + '.' : 'В выбранных проектах таких нет.'],
    ['Поле «Тестировщик»', 'значение по умолчанию', 'У одного тестировщика задачи почти во всех проектах. Отчёт по QA точен, только если тестировщика назначают вручную.'],
    ['Группа тимлидов', m.value!.TL.length + ' чел.', 'Состав взят из JQL membersOf("adata-team-leads"): видны только те, у кого были задачи за 180 дней.'],
  ]
})
</script>

<template>
  <div>
    <div class="stick">
      <header>
        <h1>Спринты Adata</h1>
        <span class="meta">{{ updated ? `Данные Jira на ${updated}` : '' }}</span>
      </header>
      <div class="gbar snapbar">
        <label class="gf"><span>Снепшот</span>
          <select :value="snap" aria-label="Снепшот" @change="openSnap(($event.target as HTMLSelectElement).value)">
            <option value="">Сейчас (живые данные)</option>
            <option v-for="s in snaps || []" :key="s.taken_for" :value="s.taken_for">{{ dmy(s.taken_for) }}{{ s.note ? ' — ' + s.note : '' }}</option>
          </select>
        </label>
        <template v-if="canSnap">
          <template v-if="!snap">
            <input v-model="snapNote" type="search" maxlength="200" placeholder="Комментарий к снепшоту" aria-label="Комментарий к снепшоту">
            <button type="button" class="btn primary" :disabled="snapBusy || !data?.updated_at" @click="saveSnap">{{ snapBusy ? 'Сохраняю…' : 'Сохранить снепшот' }}</button>
          </template>
          <button v-else type="button" class="btn danger" :disabled="snapBusy" @click="deleteSnap">Удалить снепшот</button>
        </template>
        <button v-if="!snap" type="button" class="btn" :disabled="pending" @click="refresh()">Обновить</button>
        <span v-if="snapMsg" class="sub" role="status">{{ snapMsg }}</span>
      </div>
      <p v-if="snapInfo" class="snap-banner">Снепшот на {{ dmy(snapInfo.taken_for) }} — замороженная копия отчёта{{ updated ? `, данные Jira на ${updated}` : '' }}.
        Сохранил {{ snapInfo.created_by || 'администратор' }}{{ snapInfo.note ? ' · ' + snapInfo.note : '' }}.
        <button type="button" class="linkbtn" @click="openSnap('')">Вернуться к живым данным</button></p>
      <div class="gbar">
        <label class="gf"><span>Руководитель проекта</span>
          <select v-model="lead">
            <option value="">Все руководители</option>
            <option v-for="o in leads" :key="o.l" :value="o.l">{{ o.l }} ({{ o.n }})</option>
          </select>
        </label>
        <span v-if="lead" class="scope">В фильтре: {{ scopedP.length }} проектов · {{ sumOf(scopedP, 't') }} задач</span>
      </div>
      <nav role="tablist">
        <button v-for="[id, title] in TABS" :key="id" role="tab" :aria-selected="tab === id" @click="tab = id">{{ title }}</button>
      </nav>
    </div>

    <p v-if="pending" class="sub">Загружаю…</p>
    <div v-else-if="error" class="notice">Не удалось получить данные: {{ error.statusCode ? `HTTP ${error.statusCode}` : error.message }}</div>
    <div v-else-if="snap && !m" class="notice">
      Этот снепшот сохранён в старом формате (до того, как снепшоты стали копией «Спринтов»), отчёта в нём нет.
      Удалите его и сохраните заново — новый снепшот повторит все вкладки.
    </div>
    <div v-else-if="!m" class="notice">
      Данных Jira ещё нет. Запустите синхронизацию: <code>reports sync jira --full</code> (worker делает это сам, если задан JIRA_PAT).
    </div>
    <template v-else>
      <div v-if="tab !== 'dash'" class="kpis">
        <button v-for="[v, l, c, go] in kpis" :key="l" class="kpi" :class="c" style="font:inherit;color:inherit;text-align:left;cursor:pointer" @click="tab = go"><b>{{ v }}</b><span>{{ l }}</span></button>
      </div>

      <SprintsPeople v-if="PEOPLE_TABS.includes(tab)" :tab="tab" :m="m" :L="L" :pl="pl" :ctl="ctl" :scoped-p="scopedP" :in-s="inS"
                     @lead="l => { lead = l }" @go="t => { tab = t }" />

      <section v-else-if="tab === 'sprint'">
        <h2>Статус спринтов</h2>
        <p class="lead">Прогресс каждого активного спринта по статусам. Спринт, чья дата окончания прошла, но он не закрыт в Jira, отмечен «Не закрыт».</p>
        <div class="legend">
          <span v-for="[k, l] in SEG" :key="k"><i :style="{ background: `var(--s-${k})` }" />{{ l }}</span>
        </div>
        <SortTable :cols="sprintCols" :rows="scopedP" :init="[4, 'asc']" />
      </section>

      <section v-else-if="tab === 'qa'">
        <h2>Загрузка QA</h2>
        <p class="lead">По полю «Тестировщик». Если у одного тестировщика задачи почти во всех проектах — похоже, это значение поля по умолчанию, а не реальное назначение.</p>
        <SortTable :cols="qaCols" :rows="qaRows" :init="[2, 'desc']" />
      </section>

      <section v-else-if="tab === 'block'">
        <h2>Узкие места</h2>
        <p class="lead">Незакрытые задачи в статусах Blocked, Need Rework, «Требует уточнения», а также любые задачи без обновлений больше 30 дней.</p>
        <div class="tools">
          <input v-model="qBlock" type="search" placeholder="Поиск по ключу, тексту или исполнителю">
          <select v-model="fSt" aria-label="Статус">
            <option value="">Все статусы</option>
            <option v-for="s in blockSt" :key="s" :value="s">{{ s }}</option>
          </select>
        </div>
        <SortTable :cols="blockCols" :rows="blockRows" :init="[5, 'desc']" />
      </section>

      <section v-else-if="tab === 'stab'">
        <h2>Стабильность спринтов</h2>
        <p class="lead">Сколько задач перенесено из прошлых спринтов, сколько создано после старта текущего и сколько помечено внеплановыми.</p>
        <SortTable :cols="stabCols" :rows="scopedP" :init="[4, 'desc']" />
      </section>

      <section v-else-if="tab === 'qual'">
        <h2>Качество</h2>
        <p class="lead">Доля задач типа «Ошибка» в спринте и задачи, вернувшиеся на доработку.</p>
        <SortTable :cols="qualCols" :rows="scopedP" :init="[4, 'desc']" />
      </section>

      <section v-else-if="tab === 'rel'">
        <h2>Готовность к релизу</h2>
        <p class="lead">Процент готовности спринта (доля задач в «Готово») и задачи на финише: Code review, Test, Ready for deploy / In Staging. Задачи в Test разделены по полю «Тестировщик»: QA (и пустое поле), PM, тимлид / остальные.</p>
        <SortTable :cols="relCols" :rows="scopedP" :init="[3, 'desc']" />
      </section>

      <section v-else-if="tab === 'next'">
        <h2>Следующий спринт</h2>
        <p class="lead">Готов ли следующий спринт к старту. Следующий ищется по названию текущего с номером +1 (например, «Адата Комплаенс 2026-19» → «2026-20»). «Перейдёт» — незакрытые задачи текущего спринта. «Без задач в следующем» — кто работает в текущем спринте проекта, но в следующем у него задач нет.</p>
        <div class="psum" v-html="nsSum" />
        <SortTable :cols="nextCols" :rows="nextRows" :init="[4, 'asc']" />
      </section>

      <section v-else-if="tab === 'dq'">
        <h2>Качество данных в Jira</h2>
        <p class="lead">Чего не хватает, чтобы строить отчёты по трудозатратам и срокам.</p>
        <div class="notes">
          <div v-for="[h, b, p] in notes" :key="h" class="note"><h3>{{ h }}</h3><b class="big">{{ b }}</b><p>{{ p }}</p></div>
        </div>
      </section>

      <footer>Данные выгружены из jira.adata.kz через REST API и обновляются автоматически. Ссылки открывают актуальные задачи в Jira.</footer>
    </template>
    <div v-show="tip.show" ref="ttRef" class="tt" role="tooltip" :style="{ left: tip.x + 'px', top: tip.y + 'px' }" v-html="tip.html" />
  </div>
</template>
