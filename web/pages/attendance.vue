<script setup lang="ts">
// Отчёт «Посещаемость» — повторяет reference/attendance/page_template.html; формулы разделов перенесены 1-в-1.
// Данные (ATT) — GET /api/v1/attendance; что видно, решает сервер по роли (docs/security.md).
import { type Col, esc } from '~/utils/sprints'

type Ev = { id: number; date: string; post: string; n: string; t: string; m: number; a?: number; b?: number; arr?: number; otr: string; rsn: string; ag: string; c: string; od?: string; txt?: string }
type Person = { pos: string; tl: boolean; jira: boolean; pr: [string, string, number, string, number, number, number, number, number][]; login: string }
type AbsApi = { name: string; type: 'vac' | 'bs' | 'sick'; from: string; to: string | null; source: string; note: string }
type ATT = { ev: Ev[]; ppl: Record<string, Person>; abs: AbsApi[]; abs_at: Record<string, string> | null; coll: string[]; lead: string[]; cpos: Record<string, string>; from: string; to: string; stamp: string }

const { apiBase, jiraBase } = useRuntimeConfig().public
const { me } = useMe()
const { data, error, pending } = await useFetch<ATT>(`${apiBase}/attendance`)

// ---------- помощники как в шаблоне ----------
const hm = (m?: number | null) => m == null ? '' : String(Math.floor(m / 60)).padStart(2, '0') + ':' + String(m % 60).padStart(2, '0')
const dur = (m: number) => !m ? '—' : m < 60 ? m + ' мин' : (Math.floor(m / 60) + ' ч' + (m % 60 ? ' ' + m % 60 + ' мин' : ''))
const durK = (m: number) => m < 60 ? m + ' мин' : (m / 60).toFixed(1).replace('.0', '').replace('.', ',') + ' ч'
const D = (s: string) => new Date(s + 'T00:00:00')
const iso = (d: Date) => d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0')
const WD = ['вс', 'пн', 'вт', 'ср', 'чт', 'пт', 'сб']
const fmt = (s: string) => { const d = D(s); return WD[d.getDay()] + ' ' + String(d.getDate()).padStart(2, '0') + '.' + String(d.getMonth() + 1).padStart(2, '0') }
const dm2 = (s?: string | null) => s ? String(D(s).getDate()).padStart(2, '0') + '.' + String(D(s).getMonth() + 1).padStart(2, '0') : ''
const addD = (d: string, k: number) => { const x = D(d); x.setDate(x.getDate() + k); return iso(x) }
const wdays = (f: string, t: string) => { let c = 0; for (let d = D(f); iso(d) <= t; d.setDate(d.getDate() + 1)) { const w = d.getDay(); if (w > 0 && w < 6) c++ } return c }
const TNAME: Record<string, string> = { late: 'Опоздание', early: 'Ушёл раньше', mid: 'Отлучка', day: 'Целый день', sick: 'Больничный', vac: 'Отпуск' }
const ANAME: Record<string, string> = { vac: 'Отпуск', sick: 'Больничный', bs: 'БС (без содержания)' }
const lvl = (m: number) => m > 120 ? 'l4' : m > 60 ? 'l3' : m > 30 ? 'l2' : m > 0 ? 'l1' : ''

// ---------- состояние (как в localStorage «att» шаблона — удобство конкретного зрителя) ----------
const saved = (() => { try { return JSON.parse(localStorage.getItem('att') || '{}') } catch { return {} } })()
const TAB = ref<'today' | 'stat'>(saved.TAB || 'today')
const P = ref<'today' | 'week' | 'prev' | 'all'>(saved.P || 'week')
const ST = ref<boolean>(!!saved.ST)
const Q = ref('')
const FR = ref<string>(saved.FR || '')
const FL = ref<string>(saved.FL || '')
const FP = ref<string>(saved.FP || '')
const AT = ref('')
const AUP = ref(true)
const OFUT = ref(true)
watch([TAB, P, ST, FR, FL, FP], () => { try { localStorage.setItem('att', JSON.stringify({ TAB: TAB.value, P: P.value, ST: ST.value, FR: FR.value, FL: FL.value, FP: FP.value })) } catch { /* приватный режим */ } })

const A = computed(() => data.value)
const TODAY = computed(() => A.value?.stamp ? A.value.stamp.slice(0, 10) : (A.value?.to || ''))
const COLL = computed(() => new Set(A.value?.coll || []))
const LEADS = computed(() => new Set(A.value?.lead || []))
const ABS = computed(() => (A.value?.abs || []).map(a => [a.name, a.type, a.from, a.to, a.source, a.note] as [string, string, string, string | null, string, string]))
const ppl = (n: string) => A.value?.ppl[n]
const prAll = (n: string) => ppl(n)?.pr || []
const prOf = (n: string) => prAll(n).filter(x => x[2] > 0)
const roleOf = (n: string) => ppl(n)?.pos || 'Роль не указана'
const teamOf = (n: string) => { const m: Record<string, number> = {}; prAll(n).forEach(x => { if (x[3]) m[x[3]] = (m[x[3]] || 0) + (x[4] || 0) }); return Object.keys(m).sort((a, b) => m[b] - m[a]) }
const openOf = (n: string) => prOf(n).reduce((s, x) => s + x[2], 0)
const sumF = (n: string, i: number) => prAll(n).reduce((s, x) => s + ((x[i] as number) || 0), 0)
const F = (n: string) => (!Q.value || n.toLowerCase().includes(Q.value)) && (!FR.value || roleOf(n) === FR.value) && (!FL.value || teamOf(n).includes(FL.value)) && (!FP.value || prAll(n).some(x => x[0] === FP.value))
const absOn = (r: (string | null)[], d: string) => r[2]! <= d && (r[3] || '9999') >= d
const isStudy = (e: Ev) => e.c === 'study'
const endHit = (e: Ev) => prOf(e.n).filter(([, en]) => en && e.date >= addD(en, -2) && e.date <= en)
const noAg = (e: Ev) => !e.ag && !isStudy(e) && (e.t === 'day' || e.m >= 120) && e.t !== 'sick' && e.t !== 'vac'
const counts = (e: Ev) => ST.value || !isStudy(e)
const JB = jiraBase.replace(/\/$/, '') + '/'
const SQ: Record<string, string> = { bl: 'status = 10100', wk: 'status = 10011', rw: 'status = 10000', bk: 'status = "Blocked"', all: 'status in (10100, 10011, 10000, "Blocked")' }
const JQ = (n: string, st: string) => { const l = ppl(n)?.login; return l ? JB + 'issues/?jql=' + encodeURIComponent(`cf[10104] in openSprints() AND assignee = "${l}" AND ${st} ORDER BY status ASC, updated DESC`) : null }
const lnk = (n: string, k: string, v: number, cls = '') => { const u = JQ(n, SQ[k]); return v ? (u ? `<a class="nl ${cls}" href="${u}" target="_blank" rel="noopener" title="Открыть в Jira">${v}</a>` : `<span class="${cls}">${v}</span>`) : '<span class="sub">0</span>' }
const who = (n: string) => `<b>${esc(n)}</b>${LEADS.value.has(n) ? '<span class="tag">рук.</span>' : ''}<div class="sub">${esc([roleOf(n) === 'Роль не указана' ? '' : roleOf(n), teamOf(n)[0] ? 'рук. ' + teamOf(n)[0] : ''].filter(Boolean).join(' · '))}</div>`
const rsnCell = (r: string) => esc(r) || '<span class="sub">не указана</span>'

const range = computed<[string, string]>(() => {
  const t = D(TODAY.value), dow = (t.getDay() + 6) % 7, mon = new Date(t); mon.setDate(t.getDate() - dow)
  if (P.value === 'today') return [TODAY.value, TODAY.value]
  if (P.value === 'week') { const f = new Date(mon); f.setDate(mon.getDate() + 6); return [iso(mon), iso(f)] }
  if (P.value === 'prev') { const a = new Date(mon); a.setDate(mon.getDate() - 7); const b = new Date(mon); b.setDate(mon.getDate() - 1); return [iso(a), iso(b)] }
  return [A.value!.from, TODAY.value]
})
function days(a: string, b: string) { const out: string[] = []; const has = new Set(A.value!.ev.map(e => e.date)); for (let d = D(a); iso(d) <= b && iso(d) <= TODAY.value; d.setDate(d.getDate() + 1)) { const k = iso(d), w = d.getDay(); if ((w > 0 && w < 6) || has.has(k)) out.push(k) } return out }

// ---------- фильтры ----------
const srt = (a: (string | undefined)[]) => [...new Set(a)].filter(Boolean).sort((x, y) => x!.localeCompare(y!, 'ru')) as string[]
const roles = computed(() => srt(Object.keys(A.value?.ppl || {}).map(n => ppl(n)?.pos)))
const leadsOpt = computed(() => srt(Object.keys(A.value?.ppl || {}).flatMap(n => prAll(n).map(x => x[3]))))
const projOpt = computed(() => srt(Object.keys(A.value?.ppl || {}).flatMap(n => prAll(n).map(x => x[0]))))
const anyFilter = computed(() => !!(Q.value || FR.value || FL.value || FP.value))
function resetF() { Q.value = FR.value = FL.value = FP.value = '' }

// ---------- сегодня ----------
type TRow = { n: string; k: string; lbl: string; s: number; time: string; m: number; rsn: string; otr: string; ag: string; arr?: number; b?: number; post?: string; src: string }
const todayRows = computed<TRow[]>(() => {
  if (!A.value) return []
  const rows: TRow[] = A.value.ev.filter(e => e.date === TODAY.value && F(e.n)).map(e => ({
    n: e.n, k: isStudy(e) ? 'study' : e.t, lbl: isStudy(e) ? 'Учёба' : TNAME[e.t], s: e.a ?? (e.t === 'day' ? 540 : 0),
    time: e.a != null ? `${hm(e.a)}–${hm(e.b)}` : (e.t === 'day' || e.t === 'sick' || e.t === 'vac' ? 'весь день' : esc(e.txt || '')), m: e.t === 'day' ? 480 : e.m,
    rsn: e.rsn, otr: e.otr, ag: e.ag, arr: e.arr, b: e.b, post: e.post, src: 'Telegram',
  }))
  ABS.value.filter(r => absOn(r, TODAY.value) && F(r[0]) && !rows.some(x => x.n === r[0] && (x.k === 'sick' || x.k === 'vac'))).forEach(r => rows.push({
    n: r[0], k: r[1], lbl: ANAME[r[1]], s: -1, time: `весь день · ${dm2(r[2])}–${r[3] ? dm2(r[3]) : '?'}`, m: 480, rsn: r[5], otr: '', ag: '', src: r[4],
  }))
  return rows
})
const todayCount = computed(() => new Set(todayRows.value.map(r => r.n)).size)
const todayCols: Col<TRow>[] = [
  { h: 'Сотрудник', v: r => r.n, f: r => who(r.n) },
  { h: 'Тип', v: r => r.lbl, f: r => `<span class="pill ${r.k}">${esc(r.lbl)}</span>` },
  { h: 'С — по', v: r => r.s, tip: 'Время отсутствия из сообщения; для отпуска/больничного — даты', f: r => `<b>${r.time}</b>` },
  { h: 'Часов', v: r => r.m, num: true, f: r => r.s < 0 ? '—' : dur(r.m) },
  { h: 'Пришёл', v: r => r.arr ?? -1, tip: 'Время ответа «upd»', f: r => r.arr != null ? `<span class="${r.arr > (r.b ?? 0) + 10 ? 'hi' : ''}">${hm(r.arr)}</span>` : '<span class="sub">—</span>' },
  { h: 'Причина', v: r => r.rsn, cls: 'wrap', f: r => rsnCell(r.rsn) },
  { h: 'Отработка', v: r => r.otr, cls: 'wrap', f: r => esc(r.otr) || '<span class="sub">—</span>' },
  { h: 'Согласовано', v: r => r.ag, f: r => r.ag ? esc('@' + r.ag.split(',').join(', @')) : '<span class="sub">—</span>' },
  { h: 'Источник', v: r => r.src, f: r => `<span class="sub">${r.src}${r.post ? ' · ' + r.post : ''}</span>` },
]

// ---------- статистика: по людям ----------
type PRow = { n: string; late: number; lateM: number; maxL: number; early: number; mid: number; half: number; day: number; study: number; after: number; sick: number; miss: number
  dm: Record<string, number>; ds: Record<string, number>; dx: Record<string, number>; dv: Record<string, string>; ad: Set<string>; noag: number; end: Record<string, [string, number]>
  avg: number; pos: string; tl: boolean; jira: boolean; endK: string[]; open: number; absD: number }
const stat = computed(() => {
  const [a, b] = range.value
  const ev = A.value!.ev.filter(e => e.date >= a && e.date <= b && F(e.n))
  const by: Record<string, PRow> = {}
  const mk = (n: string) => by[n] ??= { n, late: 0, lateM: 0, maxL: 0, early: 0, mid: 0, half: 0, day: 0, study: 0, after: 0, sick: 0, miss: 0, dm: {}, ds: {}, dx: {}, dv: {}, ad: new Set(), noag: 0, end: {}, avg: 0, pos: '', tl: false, jira: false, endK: [], open: 0, absD: 0 }
  const dd = days(a, b)
  ABS.value.filter(r => F(r[0]) && r[2] <= b && (r[3] || '9999') >= a).forEach(r => { const p = mk(r[0]); dd.forEach(d => { if (absOn(r, d)) { const w = D(d).getDay(); if (w > 0 && w < 6) p.ad.add(d); p.dv[d] = r[1] } }) })
  ev.forEach(e => {
    const p = mk(e.n); if (noAg(e)) p.noag++
    endHit(e).forEach(([k, en, o]) => { if (e.t !== 'sick' && e.t !== 'vac') p.end[k] = [en, o] })
    if (e.t === 'sick' || e.t === 'vac') { p.sick++; p.dx[e.date] = 1; p.ad.add(e.date); return }
    if (e.t === 'day') { if (isStudy(e) && !ST.value) { p.study += e.m; p.ds[e.date] = 1 } else { p.day++; p.dx[e.date] = 1 } return }
    if (!counts(e)) { p.study += e.m; p.ds[e.date] = 1; return }
    p.miss += e.m; p.dm[e.date] = (p.dm[e.date] || 0) + e.m
    if (e.t === 'late') { p.late++; p.lateM += e.m; p.maxL = Math.max(p.maxL, e.m); if (e.arr != null && e.arr > (e.b ?? 0) + 10) p.after++ }
    else if (e.t === 'early') p.early++; else p.mid++
  })
  Object.values(by).forEach(p => { p.half = Object.values(p.dm).filter(m => m >= 240).length; p.avg = p.late ? Math.round(p.lateM / p.late) : 0; const i = ppl(p.n); p.pos = i?.pos || ''; p.tl = !!i?.tl; p.jira = !!i?.jira; p.endK = Object.keys(p.end); p.open = (i?.pr || []).reduce((s, x) => s + x[2], 0); p.absD = p.ad.size })
  return { list: Object.values(by), ev, a, b, dd }
})
const perLabel = computed(() => { const { a, b } = stat.value; return a === b ? fmt(a) + ' · ниже всё за этот день' : fmt(a) + ' — ' + fmt(b < TODAY.value ? b : TODAY.value) + ' · ниже всё за этот период' })
const S = (f: (p: PRow) => number) => stat.value.list.reduce((s, p) => s + f(p), 0)
const kpis = computed(() => [
  [S(p => p.late), 'опозданий', 'warn'], [durK(S(p => p.lateM)), 'сумма опозданий', ''], [stat.value.list.filter(p => p.late >= 3).length, 'опаздывали 3+ раз', 'bad'],
  [S(p => p.half), 'полдня и больше', 'bad'], [S(p => p.day), 'целых дней', ''], [S(p => p.after), 'пришли позже заявленного', 'bad'],
  [stat.value.list.filter(p => p.endK.length).length, 'отпрашивались под конец спринта', 'bad'], [S(p => p.noag), 'от 2 ч без согласования', 'warn'], [durK(S(p => p.study)), 'учёба по расписанию', 'study'],
] as [string | number, string, string][])
const fmtStamp = (s?: string) => s ? s.replace('T', ' ').slice(5).replace(/^(\d\d)-(\d\d)/, '$2.$1') : '—'
const meta = computed(() => A.value && A.value.from ? `Сообщения ${fmt(A.value.from)} — ${fmt(A.value.to)} · обновлено ${fmtStamp(A.value.stamp)}` : '')
const peopleCols = computed<Col<PRow>[]>(() => {
  const dd = stat.value.dd
  return [
    { h: 'Сотрудник', v: p => p.n, f: p => `<b>${esc(p.n)}</b>${LEADS.value.has(p.n) ? '<span class="tag">рук.</span>' : p.tl ? '<span class="tag">TL</span>' : ''}<div class="sub">${esc(p.pos || (p.jira ? '' : 'нет в Jira'))}</div>` },
    { h: 'По дням', v: p => p.miss, tip: 'Каждая клетка — рабочий день периода', f: p => `<div class="grid">${dd.map(d => { const m = p.dm[d] || 0; const v = p.dv[d]; const cl = p.dx[d] || v === 'sick' ? 'x' : v ? 'v' : m ? lvl(m) : p.ds[d] ? 's' : ''; return `<i class="${cl}" title="${fmt(d)}: ${v ? ANAME[v] : p.dx[d] ? 'целый день / больничный' : m ? dur(m) : p.ds[d] ? 'учёба' : 'нет отметок'}"></i>` }).join('')}</div>` },
    { h: 'Опозданий', v: p => p.late, num: true, f: p => `<span class="${p.late >= 5 ? 'hi' : p.late >= 3 ? 'mid' : ''}">${p.late}</span>` },
    { h: 'Сумма', v: p => p.lateM, num: true, tip: 'Сумма опозданий', f: p => dur(p.lateM) },
    { h: 'В среднем', v: p => p.avg, num: true, f: p => p.late ? dur(p.avg) : '—' },
    { h: 'Макс.', v: p => p.maxL, num: true, f: p => `<span class="${p.maxL >= 120 ? 'hi' : p.maxL >= 60 ? 'mid' : ''}">${p.maxL ? dur(p.maxL) : '—'}</span>` },
    { h: 'Позже заявленного', v: p => p.after, num: true, tip: 'Ответ «upd» пришёл позже заявленного времени прихода больше чем на 10 минут', f: p => p.after ? `<span class="hi">${p.after}</span>` : '0' },
    { h: 'Ушёл раньше', v: p => p.early, num: true },
    { h: 'Отлучки', v: p => p.mid, num: true },
    { h: 'Полдня+', v: p => p.half, num: true, tip: 'Дни, где отметки в сумме от 4 часов', f: p => p.half ? `<span class="hi">${p.half}</span>` : '0' },
    { h: 'Целый день', v: p => p.day, num: true },
    { h: 'Всего пропущено', v: p => p.miss, num: true, f: p => dur(p.miss) },
    { h: 'Учёба', v: p => p.study, num: true, f: p => p.study ? `<span style="color:var(--study)">${dur(p.study)}</span>` : '—' },
    { h: 'Без согласования', v: p => p.noag, num: true, tip: 'Отсутствия от 2 часов (кроме учёбы), где не указано, кто согласовал', f: p => p.noag ? `<span class="mid">${p.noag}</span>` : '0' },
    { h: 'Под конец спринта', v: p => p.endK.length, tip: 'Отпрашивался в последние 3 дня спринта проекта, где у него есть незакрытые задачи', f: p => p.endK.length ? p.endK.map(k => `<span class="pill day" title="Спринт до ${fmt(p.end[k][0])}, незакрытых задач: ${p.end[k][1]}">${k} · ${p.end[k][1]}</span>`).join(' ') : '<span class="sub">—</span>' },
    { h: 'Открытых задач', v: p => p.open, num: true, tip: 'Бэклог + В работе + Реворк в открытых спринтах (Jira)', f: p => p.open ? String(p.open) : '<span class="sub">0</span>' },
    { h: 'Отпуск / больн. / БС, дн.', v: p => p.absD, num: true, tip: 'Рабочие дни периода в отпуске, на больничном или в БС (Bitrix + Telegram)', f: p => p.absD ? String(p.absD) : '<span class="sub">0</span>' },
  ]
})

// ---------- коллектив ----------
const coll = computed(() => {
  const { list, ev, dd } = stat.value
  const CS = new Set([...COLL.value].filter(F)); const N = CS.size
  const late = list.filter(p => CS.has(p.n) && p.late > 0), late3 = late.filter(p => p.late >= 3)
  const pc = (x: number) => N ? Math.round(x / N * 100) : 0
  const wd = dd.filter(d => { const w = D(d).getDay(); return w > 0 && w < 6 })
  const hasD = new Set(A.value!.ev.map(e => e.date))
  const per = wd.map(d => ({ d, no: !hasD.has(d), c: new Set(ev.filter(e => e.date === d && e.t === 'late' && counts(e) && CS.has(e.n)).map(e => e.n)).size }))
  const okd = per.filter(x => !x.no); const avg = okd.length ? okd.reduce((s, x) => s + x.c, 0) / okd.length : 0
  const lm = late.reduce((s, p) => s + p.lateM, 0), lc = late.reduce((s, p) => s + p.late, 0)
  const lead = `Коллектив${CS.size < COLL.value.size ? ' (с учётом фильтров)' : ''} — ${N} сотрудников без руководящего состава (${LEADS.value.size} чел.). Считаются только опоздания${ST.value ? ', включая учёбу по расписанию' : ', без учёбы по расписанию'}.`
  const kp = [[pc(late.length) + '%', `опоздали хотя бы раз · ${late.length} из ${N}`, 'warn'], [pc(late3.length) + '%', `опаздывали 3+ раз · ${late3.length} чел.`, 'bad'],
    [pc(avg) + '%', `опаздывают в средний день · ${avg.toFixed(1).replace('.', ',')} чел.`, ''], [pc(N - late.length) + '%', `ни разу не опоздали · ${N - late.length} чел.`, 'ok'],
    [lc ? dur(Math.round(lm / lc)) : '—', 'среднее опоздание', '']] as [string, string, string][]
  const cmax = Math.max(1, ...per.map(x => x.c))
  const cday = per.length ? per.map(x => `<div class="wrow"><b>${fmt(x.d)}</b><div><div class="b" style="width:${x.c / cmax * 100}%"></div></div><span class="v">${x.no ? 'нет данных' : pc(x.c) + '% · ' + x.c + ' чел.'}</span></div>`).join('') : '<div class="sub">В периоде нет рабочих дней с данными</div>'
  const R: Record<string, { r: string; n: number; l: number }> = {}
  CS.forEach(n => { const r = (A.value!.cpos || {})[n] || 'Роль не указана'; const g = R[r] ??= { r, n: 0, l: 0 }; g.n++; if (late.some(p => p.n === n)) g.l++ })
  const crole = Object.values(R).sort((a, b) => b.l / b.n - a.l / a.n).map(g => `<div class="wrow"><b>${esc(g.r)}</b><div><div class="b" style="width:${g.l / g.n * 100}%"></div></div><span class="v">${Math.round(g.l / g.n * 100)}% · ${g.l} из ${g.n}</span></div>`).join('')
  return { lead, kp, cday, crole }
})

// ---------- отпуска, больничные и БС ----------
type ARow = { n: string; t: string; f: string; to: string | null; src: string; note: string; s: number; wd: number | null }
const absRows = computed<ARow[]>(() => {
  const [a, b] = range.value, lim = addD(TODAY.value, 30)
  return ABS.value.filter(r => F(r[0])).filter(r => (!AT.value || r[1] === AT.value) && ((r[2] <= b && (r[3] || '9999') >= a) || (AUP.value && r[2] > TODAY.value && r[2] <= lim)))
    .map(r => { const to = r[3]; const s = to && to < TODAY.value ? 2 : r[2] > TODAY.value ? 1 : 0; return { n: r[0], t: r[1], f: r[2], to, src: r[4], note: r[5], s, wd: to ? wdays(r[2], to) : null } })
})
const akpis = computed(() => {
  const base = ABS.value.filter(r => F(r[0])), lim = addD(TODAY.value, 30)
  const now = (t: string) => new Set(base.filter(r => r[1] === t && absOn(r, TODAY.value)).map(r => r[0])).size
  const up = base.filter(r => r[2] > TODAY.value && r[2] <= lim)
  return [[now('vac'), 'сейчас в отпуске', 'ok'], [now('sick'), 'сейчас на больничном', 'bad'], [now('bs'), 'сейчас в БС', ''], [new Set(up.map(r => r[0])).size, 'уходят в ближайшие 30 дней', 'warn']] as [number, string, string][]
})
const aLead = computed(() => `Отпуска и БС — из Bitrix (срез ${fmtStamp(A.value?.abs_at?.bitrix?.slice(0, 16))}); больничные — из группы Telegram. Показаны записи за выбранный период${AUP.value ? ' и предстоящие 30 дней' : ''}.`)
const absCols: Col<ARow>[] = [
  { h: 'Сотрудник', v: r => r.n, f: r => who(r.n) },
  { h: 'Тип', v: r => r.t, f: r => `<span class="pill ${r.t}">${ANAME[r.t]}</span>` },
  { h: 'С', v: r => r.f, f: r => fmt(r.f) },
  { h: 'По', v: r => r.to || '9999', f: r => r.to ? fmt(r.to) : '<span class="sub">не указано</span>' },
  { h: 'Статус', v: r => r.s + r.f, f: r => r.s === 0 ? '<span class="pill day">сейчас</span>' : r.s === 1 ? `<span class="pill late">через ${Math.round((D(r.f).getTime() - D(TODAY.value).getTime()) / 864e5)} дн.</span>` : '<span class="sub">прошёл</span>' },
  { h: 'Раб. дней', v: r => r.wd ?? 0, num: true, f: r => r.wd != null ? String(r.wd) : '—' },
  { h: 'Открытых задач', v: r => openOf(r.n), num: true, tip: 'Бэклог + В работе + Реворк в открытых спринтах — кому передать на время отсутствия', f: r => openOf(r.n) ? lnk(r.n, 'all', openOf(r.n), r.s < 2 ? 'mid' : '') : '<span class="sub">0</span>' },
  { h: 'Источник', v: r => r.src, f: r => `<span class="sub">${esc(r.src)}</span>` },
  { h: 'Примечание', v: r => r.note, cls: 'wrap', f: r => esc(r.note) },
]

// ---------- по дням недели, согласующие ----------
const wdHtml = computed(() => {
  const W = [1, 2, 3, 4, 5].map(i => ({ i, c: 0, m: 0 }))
  stat.value.ev.filter(e => e.t === 'late' && counts(e)).forEach(e => { const w = D(e.date).getDay(); if (w >= 1 && w <= 5) { W[w - 1].c++; W[w - 1].m += e.m } })
  const wmax = Math.max(1, ...W.map(w => w.c))
  return W.map(w => `<div class="wrow"><b>${WD[w.i]}</b><div><div class="b" style="width:${w.c / wmax * 100}%"></div></div><span class="v">${w.c} · ${durK(w.m)}</span></div>`).join('')
})
type AgRow = { a: string; c: number; m: number; ps: Set<string> }
const agRows = computed<AgRow[]>(() => {
  const AG: Record<string, AgRow> = {}
  stat.value.ev.filter(e => e.ag).forEach(e => e.ag.split(',').forEach(a => { const g = AG[a] ??= { a, c: 0, m: 0, ps: new Set() }; g.c++; g.m += e.t === 'day' ? 480 : e.m; g.ps.add(e.n) }))
  return Object.values(AG)
})
const agCols: Col<AgRow>[] = [
  { h: 'Согласовал', v: g => g.a, f: g => '@' + esc(g.a) },
  { h: 'Отсутствий', v: g => g.c, num: true },
  { h: 'Часов', v: g => g.m, num: true, f: g => durK(g.m) },
  { h: 'Сотрудников', v: g => g.ps.size, num: true, f: g => `<span title="${esc([...g.ps].join(', '))}">${g.ps.size}</span>` },
]

// ---------- план отработок ----------
type ORow = { n: string; od: string; items: Ev[]; m: number; otr: string[]; ag: string; time: string }
const otT = (s: string) => { const m = (s || '').match(/(\d{1,2})[:.](\d{2})\s*[-–—]\s*(\d{1,2})[:.](\d{2})/); if (m) return m[1].padStart(2, '0') + ':' + m[2] + '–' + m[3].padStart(2, '0') + ':' + m[4]; const s1 = (s || '').match(/(?:с|после)\s*(\d{1,2})[:.](\d{2})/i); return s1 ? 'с ' + s1[1].padStart(2, '0') + ':' + s1[2] : '' }
const oRows = computed<ORow[]>(() => {
  const OT = A.value!.ev.filter(e => e.od && e.od >= addD(TODAY.value, -7) && F(e.n) && (!OFUT.value || e.od >= TODAY.value))
  const g: Record<string, ORow & { agS: Set<string> }> = {}
  OT.forEach(e => { const k = e.n + '|' + e.od; const x = g[k] ??= { n: e.n, od: e.od!, items: [], m: 0, otr: [], ag: '', agS: new Set(), time: '' }; x.items.push(e); x.m += e.t === 'day' ? 480 : e.m; if (e.otr && !x.otr.includes(e.otr)) x.otr.push(e.otr); (e.ag || '').split(',').filter(Boolean).forEach(a => x.agS.add(a)); if (!x.time) x.time = otT(e.otr) })
  return Object.values(g).map(x => ({ ...x, ag: [...x.agS].join(',') }))
})
const oSum = computed(() => {
  const up = oRows.value.filter(e => e.od >= TODAY.value), zero = up.filter(e => !openOf(e.n)), few = up.filter(e => openOf(e.n) > 0 && openOf(e.n) <= 2)
  return up.length ? `Впереди отработок: <b>${up.length}</b> · без задач: <b class="${zero.length ? 'hi' : ''}">${zero.length}</b>${zero.length ? ' (' + [...new Set(zero.map(e => esc(e.n)))].join(', ') + ')' : ''} · 1–2 задачи: <b class="${few.length ? 'mid' : ''}">${few.length}</b>. Отработка обычно вне рабочего времени, тимлида рядом может не быть — задачи нужно выдать заранее.` : 'Предстоящих отработок с датой нет.'
})
const oCols = computed<Col<ORow>[]>(() => [
  { h: 'Дата отработки', v: e => (e.od >= TODAY.value ? '0' : '1') + (e.od >= TODAY.value ? e.od : String(99999999 - +e.od.replace(/-/g, ''))), f: e => `<span class="${e.od === TODAY.value ? 'mid' : e.od < TODAY.value ? 'sub' : ''}">${fmt(e.od)}${e.od === TODAY.value ? ' · сегодня' : e.od < TODAY.value ? ' · прошла' : ''}</span>` },
  { h: 'Время', v: e => e.time || '~', tip: 'Время отработки, если сотрудник его написал. Обычно после 18:00 или в субботу', f: e => e.time ? `<b>${e.time}</b>` : `<span class="sub">${D(e.od).getDay() === 6 ? 'суббота, ' : ''}не указано</span>` },
  { h: 'Сотрудник', v: e => e.n, f: e => esc(e.n) },
  { h: 'Роль', v: e => roleOf(e.n), f: e => roleOf(e.n) === 'Роль не указана' ? '<span class="sub">не указана</span>' : esc(roleOf(e.n)) },
  { h: 'За что', v: e => e.items[0].date, cls: 'wrap', f: e => e.items.map(x => `${fmt(x.date)} · ${isStudy(x) ? 'учёба' : TNAME[x.t].toLowerCase()} ${x.a != null ? hm(x.a) + '–' + hm(x.b) : ''}`).join('<br>') },
  { h: 'Часов', v: e => e.m, num: true, f: e => dur(e.m) },
  { h: 'Как написано', v: e => e.otr.join('; '), cls: 'wrap', f: e => esc(e.otr.join('; ')) },
  { h: 'Согласовано', v: e => e.ag, f: e => e.ag ? esc('@' + e.ag.split(',').join(', @')) : '<span class="sub">—</span>' },
  { h: 'Задач всего', v: e => openOf(e.n), num: true, tip: 'Бэклог + В работе + Реворк + Blocked в открытых спринтах (срез Jira). 0 — на отработке ему нечего делать', f: e => { const o = openOf(e.n); return o ? lnk(e.n, 'all', o, o <= 2 ? 'mid' : '') : '<span class="pill day">нет задач</span>' } },
  { h: 'Бэклог', v: e => sumF(e.n, 5), num: true, f: e => lnk(e.n, 'bl', sumF(e.n, 5)) },
  { h: 'В работе', v: e => sumF(e.n, 6), num: true, f: e => lnk(e.n, 'wk', sumF(e.n, 6)) },
  { h: 'Реворк', v: e => sumF(e.n, 7), num: true, f: e => lnk(e.n, 'rw', sumF(e.n, 7), 'mid') },
  { h: 'Blocked', v: e => sumF(e.n, 8), num: true, tip: 'Заблокированные задачи: на отработке их обычно не сделать без разблокировки', f: e => lnk(e.n, 'bk', sumF(e.n, 8), 'hi') },
  { h: 'Проекты', v: e => prOf(e.n).length, cls: 'wrap', f: e => prOf(e.n).length ? prOf(e.n).map(([k, , o]) => `<span class="sub">${k} ${o}</span>`).join(' · ') : '<span class="sub">—</span>' },
])

// ---------- журнал ----------
const jCols: Col<Ev>[] = [
  { h: 'Дата', v: e => e.date + e.post, f: e => `${fmt(e.date)} <span class="sub">${e.post}</span>` },
  { h: 'Сотрудник', v: e => e.n, f: e => esc(e.n) },
  { h: 'Тип', v: e => e.t, f: e => `<span class="pill ${isStudy(e) ? 'study' : e.t}">${isStudy(e) ? 'Учёба' : TNAME[e.t]}</span>` + (endHit(e).length && e.t !== 'sick' && e.t !== 'vac' ? ` <span class="pill day" title="${endHit(e).map(([k, en, o]) => k + ': спринт до ' + fmt(en) + ', незакрытых ' + o).join('; ')}">конец спринта</span>` : '') + (noAg(e) ? ' <span class="pill late">без согласования</span>' : '') },
  { h: 'Время', v: e => e.a ?? 0, f: e => e.a != null ? `${hm(e.a)}–${hm(e.b)}` : `<span class="sub">${esc(e.txt || (e.t === 'day' ? 'весь день' : ''))}</span>` },
  { h: 'Минут', v: e => e.m, num: true, f: e => e.t === 'sick' || e.t === 'vac' ? '—' : dur(e.m) },
  { h: 'Пришёл', v: e => e.arr ?? -1, tip: 'Время ответа «upd»', f: e => e.arr != null ? `<span class="${e.arr > (e.b ?? 0) + 10 ? 'hi' : ''}">${hm(e.arr)}</span>` : '<span class="sub">—</span>' },
  { h: 'Причина', v: e => e.rsn, cls: 'wrap', f: e => rsnCell(e.rsn) },
  { h: 'Отработка', v: e => e.otr, cls: 'wrap', f: e => esc(e.otr) || '<span class="sub">—</span>' },
  { h: 'Согласовано', v: e => e.ag, f: e => e.ag ? esc('@' + e.ag.split(',').join(', @')) : '<span class="sub">—</span>' },
]

// Что видно этой роли (сервер уже отфильтровал; подпись — чтобы было понятно, почему людей меньше).
const scopeNote = computed(() => me.value?.role === 'lead' ? 'Вы видите сотрудников своих проектов; причины отсутствий других людей скрыты.' : me.value?.role === 'employee' ? 'Вы видите только свою посещаемость.' : '')
</script>

<template>
  <div class="att">
    <header>
      <h1>Посещаемость Adata</h1>
      <span class="meta">{{ meta }}</span>
    </header>
    <p class="lead">Опоздания, уходы раньше, отлучки и пропуски из группы Telegram «Adata Group». График 9:00–18:00, пн–пт. Учитывается только то, что сотрудник сам написал в группу.</p>
    <p v-if="scopeNote" class="sub">{{ scopeNote }}</p>

    <p v-if="pending" class="sub">Загружаю…</p>
    <div v-else-if="error" class="notice">Не удалось получить данные: {{ error.statusCode ? `HTTP ${error.statusCode}` : error.message }}</div>
    <div v-else-if="!A || !A.ev.length" class="notice">Сообщений о посещаемости пока нет.</div>
    <template v-else>
      <nav class="atabs" role="tablist" aria-label="Разделы">
        <button type="button" role="tab" :aria-selected="TAB === 'today'" @click="TAB = 'today'">Сегодня<span class="cnt">{{ todayCount }}</span></button>
        <button type="button" role="tab" :aria-selected="TAB === 'stat'" @click="TAB = 'stat'">Статистика за период</button>
      </nav>
      <div class="tools stick">
        <input v-model.trim="Q" type="search" placeholder="Поиск ФИО" aria-label="Поиск по ФИО" @input="Q = Q.toLowerCase()">
        <select v-model="FR" aria-label="Позиция"><option value="">Все позиции</option><option v-for="v in roles" :key="v" :value="v">{{ v }}</option></select>
        <select v-model="FL" aria-label="Руководитель"><option value="">Все руководители</option><option v-for="v in leadsOpt" :key="v" :value="v">{{ v }}</option></select>
        <select v-model="FP" aria-label="Проект"><option value="">Все проекты</option><option v-for="v in projOpt" :key="v" :value="v">{{ v }}</option></select>
        <button v-if="anyFilter" type="button" class="btn" @click="resetF">Сбросить</button>
      </div>

      <div v-if="TAB === 'today'">
        <section>
          <h2>Сегодня, {{ fmt(TODAY) }} · {{ todayCount }} чел.</h2>
          <p class="lead">Кто отпросился сегодня и на какое время, плюс кто сегодня в отпуске, на больничном или в БС. Не зависит от выбранного периода, фильтры применяются.</p>
          <SortTable :cols="todayCols" :rows="todayRows" :init="[2, 'asc']" empty="Сегодня отметок нет" />
        </section>
      </div>

      <div v-else>
        <section style="margin-top:12px">
          <div class="tools" style="border:0;padding:0">
            <div class="seg" role="group" aria-label="Период">
              <button v-for="[p, l] in [['today', 'Сегодня'], ['week', 'Эта неделя'], ['prev', 'Прошлая неделя'], ['all', 'Весь период']]" :key="p" type="button" :aria-pressed="P === p" @click="P = p as typeof P">{{ l }}</button>
            </div>
            <span class="meta">{{ perLabel }}</span>
            <label><input v-model="ST" type="checkbox"> Считать учёбу по расписанию опозданием</label>
          </div>
        </section>
        <div class="kpis"><div v-for="[v, l, c] in kpis" :key="l" class="kpi" :class="c"><b>{{ v }}</b><span>{{ l }}</span></div></div>

        <section v-if="COLL.size">
          <h2>Доля опаздывающих в коллективе</h2>
          <p class="lead">{{ coll.lead }}</p>
          <div class="kpis" style="margin-block:0"><div v-for="[v, l, c] in coll.kp" :key="l" class="kpi" :class="c"><b>{{ v }}</b><span>{{ l }}</span></div></div>
          <div class="two">
            <div class="acard cday"><h3 class="ch">По дням: % коллектива с опозданием</h3><div v-html="coll.cday" /></div>
            <div class="acard crole"><h3 class="ch">По ролям: % опоздавших хотя бы раз</h3><div v-html="coll.crole" /></div>
          </div>
        </section>

        <section>
          <h2>По сотрудникам</h2>
          <div class="legend">
            <span><span class="grid"><i /></span>без отметок</span>
            <span><span class="grid"><i class="l1" /><i class="l2" /><i class="l3" /><i class="l4" /></span>до 30 мин · до 1 ч · до 2 ч · больше 2 ч</span>
            <span><span class="grid"><i class="s" /></span>учёба по расписанию</span>
            <span><span class="grid"><i class="x" /></span>целый день / больничный</span>
            <span><span class="grid"><i class="v" /></span>отпуск / БС</span>
          </div>
          <SortTable :cols="peopleCols" :rows="stat.list" :init="[2, 'desc']" empty="За этот период сообщений нет" />
        </section>

        <section>
          <h2>Отпуска, больничные и БС</h2>
          <p class="lead">{{ aLead }}</p>
          <div class="kpis" style="margin-block:0"><div v-for="[v, l, c] in akpis" :key="l" class="kpi" :class="c"><b>{{ v }}</b><span>{{ l }}</span></div></div>
          <div class="tools" style="border:0;padding:0">
            <div class="seg" role="group" aria-label="Тип">
              <button v-for="[t, l] in [['', 'Все'], ['vac', 'Отпуск'], ['sick', 'Больничный'], ['bs', 'БС']]" :key="t" type="button" :aria-pressed="AT === t" @click="AT = t">{{ l }}</button>
            </div>
            <label><input v-model="AUP" type="checkbox"> + предстоящие 30 дней</label>
          </div>
          <SortTable :cols="absCols" :rows="absRows" :init="[4, 'asc']" empty="Отпусков и больничных в периоде нет" />
        </section>

        <div class="two">
          <section>
            <h2>Опоздания по дням недели</h2>
            <p class="lead">Сколько раз опаздывали и на сколько в сумме, за выбранный период.</p>
            <div class="acard" v-html="wdHtml" />
          </section>
          <section>
            <h2>Кто согласовывает</h2>
            <p class="lead">Отсутствия, где в сообщении указан согласовавший.</p>
            <SortTable :cols="agCols" :rows="agRows" :init="[1, 'desc']" empty="За этот период согласований нет" />
          </section>
        </div>

        <section>
          <h2>План отработок</h2>
          <p class="lead">Отработки с конкретной датой из сообщений: что обещано на сегодня и вперёд. Выполнение не проверяется — это список для контроля.</p>
          <div class="tools" style="border:0;padding:0"><label><input v-model="OFUT" type="checkbox"> Только предстоящие</label></div>
          <p class="lead" v-html="oSum" />
          <SortTable :cols="oCols" :rows="oRows" :init="[0, 'asc']" empty="Отработок с датой нет" />
        </section>

        <section>
          <h2>Журнал сообщений</h2>
          <SortTable :cols="jCols" :rows="stat.ev" :init="[0, 'desc']" empty="За этот период сообщений нет" />
        </section>

        <section>
          <h2>Как считается</h2>
          <div class="notes">
            <div class="note"><h3>Опоздание</h3><p>Пропуск, который начинается в 9:00. Минуты — от 9:00 до заявленного времени прихода. «Позже заявленного» — сотрудник ответил «upd» о приходе позже заявленного больше чем на 10 минут.</p></div>
            <div class="note"><h3>Полдня и больше</h3><p>Отметки за день в сумме от 4 часов. Целый день — пропуск без времени или с 9:00 до 18:00.</p></div>
            <div class="note"><h3>Под конец спринта</h3><p>Отметка попадает на последние 3 дня спринта проекта, где у сотрудника есть задачи в бэклоге, в работе или в реворке (по текущему срезу Jira).</p></div>
            <div class="note"><h3>Учёба</h3><p>Причина «университет» или отработка «по расписанию». По умолчанию это согласованный график и в опоздания не входит.</p></div>
            <div class="note"><h3>Чего здесь нет</h3><p>Кто опоздал и не написал в группу — не виден. Факта прихода по СКУД пока нет. Отработки показаны текстом, их выполнение не проверяется.</p></div>
          </div>
        </section>
      </div>
    </template>
  </div>
</template>
