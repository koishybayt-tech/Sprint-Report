<script setup lang="ts">
// Вкладки отчёта «Спринты» по людям: Дэшборды, Загрузка ресурсов, Ресурсы по руководителям, Нужны задачи,
// Перегружены, Переключаются, Тимлиды и бэклог. Колонки — из reference/sprints/page_template.html 1-в-1.
import { type Col, type Model, type P, UNASSIGNED, esc, fmt, jiraLinks, lvl, pct, sumOf } from '~/utils/sprints'
import { ABT, type Ab, type Ctl, type LresLead, type LresProj, OPENQ, type PeopleLogic, absTxt, lres } from '~/utils/sprintsPeople'

const props = defineProps<{
  tab: string; m: Model; L: ReturnType<typeof jiraLinks>; pl: PeopleLogic; ctl: Ctl
  scopedP: P[]; inS: (k: string) => boolean
}>()
const emit = defineEmits<{ lead: [string]; go: [string] }>()

const SEG = [['dn', 'Готово'], ['ts', 'Тестируется'], ['cr', 'Код ревью'], ['rw', 'Реворк'], ['wk', 'В работе'], ['bl', 'Бэклог']] as const
const pos = (p: string) => p === '—' ? '<span class="sub">не указана</span>' : esc(p)
const pin = (keys: string[]) => `project in (${keys.map(k => '"' + k + '"').join(', ')})`

// ---------- общие ячейки ----------
const leadChips = (n: string, ls: { l: string; keys: string[]; t: number }[]) => ls.map(x => `<a class="who wl" href="${props.L.jq([props.L.who(n), pin(x.keys)])}" target="_blank" rel="noopener" title="${esc(x.l)}: ${x.keys.join(', ')} — ${x.t} задач">${esc(x.l)} <b>${x.t}</b></a>`).join('')
const nmList = (arr: string[], max = 4) => arr.length ? `<div class="nm">${arr.slice(0, max).join('')}${arr.length > max ? `<span class="sub">+${arr.length - max}</span>` : ''}</div>` : '<span class="sub">—</span>'
const nameChip = (n: string, url: string, cls = 'mute', extra = '') => `<a class="pill ${cls}" href="${url}" target="_blank" rel="noopener" title="${esc(props.m.roleOf(n))}${extra ? ' · ' + esc(extra) : ''}">${esc(n)}${extra ? ' · ' + esc(extra) : ''}</a>`

// ---------- Загрузка ресурсов ----------
const pFlag = ref('')
const fPos = ref('')
const qPeople = ref('')
const allPos = computed(() => [...new Set(Object.keys(props.m.PEOPLE).map(props.m.roleOf))].sort())
type A = { n: string; pos: string; pr: string[]; t: number; bl: number; wk: number; cr: number; rw: number; ts: number; dn: number; bug: number
  open: number; need: boolean; over: boolean; sw: boolean; tl: boolean; ab: Ab | undefined }
const peopleRows = computed<A[]>(() => {
  const { pl, m } = props
  return Object.values(m.PEOPLE).map(Pp => {
    const rows = Pp.rows.filter(r => props.inS(r.k)); if (!rows.length) return null
    const a: any = { n: Pp.n, pos: m.roleOf(Pp.n), pr: rows.map(r => r.k) }
    ;(['t', 'bl', 'wk', 'cr', 'rw', 'ts', 'dn', 'bug'] as const).forEach(f => { a[f] = pl.sumRows(rows, f) })
    a.open = a.t - a.dn; a.need = pl.needFlag(Pp.n); a.over = pl.overFlag(Pp.n); a.sw = pl.switchInfo(Pp.n).flag; a.tl = m.isTL(Pp.n)
    a.ab = pl.absNow(Pp.n) || pl.absSoon(Pp.n, 7)
    return a as A
  }).filter((a): a is A => !!a).filter(a => (!fPos.value || a.pos === fPos.value) && (!qPeople.value || a.n.toLowerCase().includes(qPeople.value.toLowerCase())) &&
    (!pFlag.value || (pFlag.value === 'need' && a.need) || (pFlag.value === 'over' && a.over) || (pFlag.value === 'switch' && a.sw) || (pFlag.value === 'tl' && a.tl) || (pFlag.value === 'abs' && a.ab)))
})
const peopleSum = computed(() => { const r = peopleRows.value; return `<span><b>${r.length}</b> чел.</span><span><b>${sumOf(r, 't')}</b> задач</span>` + SEG.slice().reverse().map(([k, l]) => `<span>${l} <b>${sumOf(r, k)}</b></span>`).join('') })
const flagChips = (a: A) => (a.ab ? `<span class="pill ${a.ab.fr > props.m.SNAP ? 'warn' : 'bad'}" title="${esc(a.ab.src + (a.ab.note ? ' · ' + a.ab.note : ''))}">${esc(absTxt(a.ab, props.m.SNAP))}</span> ` : '') + (a.tl ? '<span class="pill mute">тимлид</span> ' : '') + (a.need ? '<span class="pill warn">нужны задачи</span> ' : '') + (a.over ? '<span class="pill bad">перегружен</span> ' : '') + (a.sw ? `<span class="pill bad has-tip" tabindex="0" data-tipn="${esc(a.n)}">тормозит проект ⓘ</span>` : '')
const peopleCols = computed<Col<A>[]>(() => {
  const { jq, nl, who, projQ, projList, ST } = props.L
  const pp = (a: A, st?: string) => jq([who(a.n), projQ(), st ?? ''])
  return [
    { h: 'Исполнитель', v: a => a.n, f: a => esc(a.n) + `<div class="flags">${flagChips(a)}</div>` },
    { h: 'Роль', v: a => a.pos, f: a => pos(a.pos) },
    { h: 'Всего', v: a => a.t, num: true, f: a => nl(a.t, pp(a)) },
    { h: 'Открыто', v: a => a.open, num: true, tip: 'Всё, кроме «Готово»', f: a => nl(a.open, pp(a, ST.open), lvl(a.open, 30, 45)) },
    { h: 'Бэклог', v: a => a.bl, num: true, tip: 'Статус «Сделать»', f: a => nl(a.bl, pp(a, ST.bl)) },
    { h: 'В работе', v: a => a.wk, num: true, f: a => nl(a.wk, pp(a, ST.wk)) },
    { h: 'Код ревью', v: a => a.cr, num: true, f: a => nl(a.cr, pp(a, ST.cr)) },
    { h: 'Реворк', v: a => a.rw, num: true, f: a => nl(a.rw, pp(a, ST.rw), a.rw ? 'mid' : '') },
    { h: 'Тестируется', v: a => a.ts, num: true, tip: 'Test, In Staging, Ready for deploy (и Production), In Production, Blocked, Требует уточнения', f: a => nl(a.ts, pp(a, ST.ts)) },
    { h: 'Готово', v: a => a.dn, num: true, f: a => nl(a.dn, pp(a, ST.dn)) },
    { h: 'Ошибок', v: a => a.bug, num: true, f: a => nl(a.bug, pp(a, 'issuetype = "Ошибка"')) },
    { h: 'Руководители (ПМ)', v: a => props.pl.leadsOf(a.n, a.pr).length, num: true, cls: 'wrap', tip: 'Руководители проектов, у которых работает сотрудник, и число его задач у каждого', f: a => { const ls = props.pl.leadsOf(a.n, a.pr); return `<div class="plist"><b class="pc" title="Руководителей">${ls.length}</b>${leadChips(a.n, ls)}</div>` } },
    { h: 'Проекты', v: a => a.pr.length, num: true, cls: 'wrap', f: a => projList(a.n, a.pr) },
  ]
})

// ---------- Ресурсы по руководителям ----------
const LRES = computed(() => lres(props.m))
const lresSum = computed(() => {
  const { m } = props
  const multi = Object.values(m.PEOPLE).filter(Pp => Pp.n !== UNASSIGNED && !m.INACT.includes(Pp.n) && new Set(Pp.rows.filter(r => r.t > 0).map(r => m.LEAD(r.k))).size > 1).length
  return `Сотрудников с задачами в спринтах: <b>${LRES.value.N}</b> · руководителей: <b>${LRES.value.L.length}</b> · работают у нескольких руководителей: <b>${multi}</b>`
})
const f1 = (v: number) => (Math.round(v * 10) / 10).toLocaleString('ru')
const lresCols = computed<Col<LresLead>[]>(() => {
  const { jq, pl: plink, who } = props.L, m = props.m
  const maxShare = Math.max(1, ...LRES.value.L.map(x => x.share))
  return [
    { h: 'Руководитель / проект', v: x => x.l, f: x => `<b>${esc(x.l)}</b>`, sf: (q: LresProj) => `${plink(q.k, jq([`project = "${q.k}"`]))} <span class="sub">${esc(m.pname(q.k))}</span>` },
    { h: 'Проекты', v: x => x.keys.length, num: true, sf: (q: LresProj) => `<span class="sub">${esc(m.PBY[q.k]?.sp || '')}</span>`, f: x => `<div class="plist" style="min-width:170px;flex-wrap:wrap"><b class="pc">${x.keys.length}</b>${x.keys.map(k => plink(k, jq([`project = "${k}"`]))).join(' ')}</div>` },
    { h: 'Доля сотрудников', v: x => x.share, num: true, tip: 'Сколько процентов всех сотрудников с задачами занято у руководителя (в эквиваленте полных людей)', f: (x: LresLead | LresProj) => `<div style="display:flex;align-items:center;gap:8px;min-width:170px"><div class="bar" style="flex:1"><i style="width:${x.share / maxShare * 100}%;background:var(--accent)"></i></div><b>${f1(x.share)}%</b></div>` },
    { h: 'Людей, экв.', v: x => x.fte, num: true, tip: 'Сумма долей: сколько это полных людей', f: (x: LresLead | LresProj) => f1(x.fte) },
    { h: 'Задействовано', v: x => x.hc, num: true, tip: 'Сколько разных людей имеют задачи в его проектах' },
    { h: 'Только у него', v: x => x.only, num: true, tip: 'Работают только в его проектах; в строке проекта — только в этом проекте' },
    { h: 'Делит с другими', v: x => x.shared, num: true, tip: 'Работают ещё у других руководителей' },
    { h: 'По ролям, экв.', v: x => x.fte, num: true, cls: 'wrap', f: (x: LresLead | LresProj) => Object.entries(x.roles).sort((a, b) => b[1] - a[1]).map(([r, v]) => `<span class="pill mute" title="${esc(r)}">${esc(r === '—' ? 'роль не указана' : r)} ${f1(v)}</span>`).join(' ') },
    { h: 'Люди (доля у руководителя)', v: x => x.hc, num: true, cls: 'wrap', f: (x: LresLead | LresProj) => nmList(x.ppl.map(y => nameChip(y.n, jq([who(y.n), pin(y.keys)]), y.f >= 0.99 ? 'ok' : 'mute', y.f >= 0.99 ? '' : Math.round(y.f * 100) + '%')), 8) },
    { h: 'Задач', v: x => x.t, num: true },
  ]
})
const lresEx = { key: (x: LresLead) => x.l, sub: (x: LresLead) => x.sub }

// ---------- Отсутствия ----------
type AR = ReturnType<PeopleLogic['absRows']>[number]
const absRowsTab = computed(() => props.pl.absRows(props.ctl.aH || 0, props.ctl.aRisk))
const absSum = computed(() => { const r = absRowsTab.value; return `<span><b>${r.length}</b> отсутствий</span><span>Сейчас <b>${r.filter(a => a.now).length}</b></span><span>Скоро <b>${r.filter(a => !a.now).length}</b></span><span>Под конец спринта <b>${r.filter(a => a.endRisk).length}</b></span><span>Задач не закроется <b>${sumOf(r, 'q')}</b></span>` })
const absSrc = computed(() => {
  const at = props.m.ABSAT, f = (s?: string) => { if (!s) return ''; const d = new Date(s), p = (n: number) => String(n).padStart(2, '0'); return `${p(d.getDate())}.${p(d.getMonth() + 1)} ${p(d.getHours())}:${p(d.getMinutes())}` }
  return [at.bitrix ? `Bitrix обновлён ${f(at.bitrix)}` : 'Bitrix ещё не синхронизирован', at.telegram ? `Telegram прочитан ${f(at.telegram)}` : 'Telegram пока не подключён'].join(', ') + '.'
})
const absCols = computed<Col<AR>[]>(() => {
  const { jq, nl, who, projQ, ST } = props.L, m = props.m
  return [
    { h: 'Сотрудник', v: a => a.n, f: a => esc(a.n) + `<div class="sub">${a.pos === '—' ? 'роль не указана' : esc(a.pos)}</div>` },
    { h: 'Отсутствие', v: a => a.ty, f: a => `<span class="pill ${a.ty === 'sick' ? 'bad' : a.ty === 'off' ? 'mute' : 'warn'}">${ABT[a.ty]}</span>${a.endRisk ? ' <span class="pill bad" title="Спринт заканчивается во время отсутствия или в течение 2 дней после">конец спринта</span>' : ''}<div class="sub">${a.ty === 'off' ? fmt(a.fr) : fmt(a.fr) + ' – ' + (a.to ? fmt(a.to) : '?')}</div>` },
    { h: 'Когда', v: a => a.now ? -1 : a.when, num: true, cls: 'wrap', tip: 'Отсутствует сейчас или начнёт отсутствовать через N дней', f: a => a.ty === 'off' ? `<b class="${a.now ? 'hi' : 'mid'}">${a.now ? 'Сегодня' : fmt(a.fr)}</b> <span class="sub">${esc(a.note.replace('Отпросился: ', ''))}</span>` : a.now ? `<b class="hi">Сейчас</b> <span class="sub">${a.to ? (a.when === 0 ? 'последний день' : 'ещё ' + a.when + ' дн.') : 'дата выхода не указана'}</span>` : `<span class="mid">через ${a.when} дн.</span>` },
    { h: 'Не закроются', v: a => a.q, num: true, tip: 'Бэклог + В работе + Реворк по выбранным проектам', f: a => nl(a.q, jq([who(a.n), projQ(), OPENQ]), a.q ? 'hi' : '') },
    { h: 'Бэклог', v: a => a.bl, num: true, f: a => nl(a.bl, jq([who(a.n), projQ(), ST.bl])) },
    { h: 'В работе', v: a => a.wk, num: true, f: a => nl(a.wk, jq([who(a.n), projQ(), ST.wk])) },
    { h: 'Реворк', v: a => a.rw, num: true, f: a => nl(a.rw, jq([who(a.n), projQ(), ST.rw]), a.rw ? 'mid' : '') },
    { h: 'Ревью / тест', v: a => a.cr + a.ts, num: true, tip: 'Ждут других, но вопросы по ним некому будет закрыть', f: a => nl(a.cr + a.ts, jq([who(a.n), projQ(), 'status not in (10100, 10011, 10000, 10001)'])) },
    { h: 'Проекты под риском', v: a => a.rk.length, num: true, cls: 'wrap', tip: 'Проекты, где у сотрудника остались незакрытые задачи. Оранжевым — спринт заканчивается во время отсутствия или в течение 2 дней после него',
      f: a => a.rk.length ? `<div class="plist">${a.rk.map(r => `<a class="pl${r.hit ? ' wl same' : ''}" style="${r.hit ? 'border-color:var(--warn);background:var(--warn-soft)' : ''}" href="${jq([who(a.n), 'project = "' + r.k + '"', OPENQ])}" target="_blank" rel="noopener" title="${esc(m.pname(r.k))} · ${esc(m.LEAD(r.k))} · спринт до ${fmt(r.en)}${r.hit ? ' — спринт заканчивается во время отсутствия или сразу после' : ''}">${r.k} <b>${r.q}</b></a>`).join(' ')}</div>` : '<span class="sub">—</span>' },
    { h: 'Руководители (ПМ)', v: a => a.ls.length, num: true, cls: 'wrap', tip: 'Кому переназначать задачи', f: a => a.ls.length ? `<div class="plist">${a.ls.map(x => `<a class="who wl" href="${jq([who(a.n), pin(x.keys), OPENQ])}" target="_blank" rel="noopener" title="${esc(x.l)}: ${x.keys.join(', ')} — ${x.t} незакрытых задач">${esc(x.l)} <b>${x.t}</b></a>`).join('')}</div>` : '<span class="sub">—</span>' },
    { h: 'Источник', v: a => a.src, cls: 'wrap', f: a => `<span class="sub">${esc(a.src)}${a.note ? ' · ' + esc(a.note) : ''}</span>` },
  ]
})

// ---------- Нужны задачи ----------
type NA = { n: string; pos: string; tl: boolean; pr: string[]; ls: { l: string; keys: string[]; t: number }[]; donors: ReturnType<PeopleLogic['donorsFor']>
  bl: number; wk: number; cr: number; rw: number; ts: number; dn: number; q: number }
const needRows = computed<NA[]>(() => {
  const { pl, m } = props
  return Object.keys(m.PEOPLE).filter(pl.needVisible).map(n => {
    const rows = m.PEOPLE[n].rows, a: any = { n, pos: m.roleOf(n), tl: m.isTL(n), pr: rows.map(r => r.k) }
    a.ls = pl.leadsOf(n, a.pr); a.donors = pl.donorsFor(n, a.pr)
    ;(['bl', 'wk', 'cr', 'rw', 'ts', 'dn'] as const).forEach(f => { a[f] = pl.sumRows(rows, f) }); a.q = a.bl + a.wk + a.rw
    return a as NA
  })
})
const needSum = computed(() => { const r = needRows.value; return `<span><b>${r.length}</b> чел.</span><span>Без задач <b>${r.filter(a => a.q === 0).length}</b></span><span>Заканчиваются <b>${r.filter(a => a.q > 0).length}</b></span>` })
const needCols = computed<Col<NA>[]>(() => {
  const { jq, nl, who, projList, ST } = props.L
  return [
    { h: 'Сотрудник', v: a => a.n, f: a => esc(a.n) + (a.tl ? ' <span class="pill mute">тимлид</span>' : '') },
    { h: 'Роль', v: a => a.pos, f: a => pos(a.pos) },
    { h: 'Состояние', v: a => a.q, f: a => a.q === 0 ? '<span class="pill bad">Без задач</span>' : '<span class="pill warn">Заканчиваются</span>' },
    { h: 'Осталось', v: a => a.q, num: true, tip: 'Бэклог + В работе + Реворк по всем проектам', f: a => nl(a.q, jq([who(a.n), 'status in (10100, 10011, 10000)']), a.q === 0 ? 'hi' : 'mid') },
    { h: 'Бэклог', v: a => a.bl, num: true, f: a => nl(a.bl, jq([who(a.n), ST.bl])) },
    { h: 'В работе', v: a => a.wk, num: true, f: a => nl(a.wk, jq([who(a.n), ST.wk])) },
    { h: 'Ждут ревью/теста', v: a => a.cr + a.ts, num: true, f: a => nl(a.cr + a.ts, jq([who(a.n), 'status not in (10100, 10011, 10000, 10001)'])) },
    { h: 'Готово', v: a => a.dn, num: true, f: a => nl(a.dn, jq([who(a.n), ST.dn])) },
    { h: 'У кого взять задачи', v: a => a.donors.length, num: true, cls: 'wrap', tip: 'Коллеги из тех же проектов с той же ролью и ПМ (их задачи может взять любой), у которых в бэклоге общих проектов больше порога',
      f: a => a.donors.length ? `<div class="plist">${a.donors.slice(0, 8).map(d => `<a class="who wl${d.same ? ' same' : ''}" href="${jq([who(d.n), pin(d.keys), ST.bl])}" target="_blank" rel="noopener" title="${esc(d.n)} · ${esc(d.pos)} · в бэклоге ${d.bl} в ${d.keys.join(', ')}">${d.pm && !d.same ? '<i class="pmtag">ПМ</i>' : ''}${esc(d.n)} <b>${d.bl}</b></a>`).join('')}${a.donors.length > 8 ? `<span class="sub">+${a.donors.length - 8}</span>` : ''}</div>` : `<span class="sub">${a.pos === '—' ? 'роль не указана, ПМ с бэклогом нет' : 'нет подходящих'}</span>` },
    { h: 'Руководители (ПМ)', v: a => a.ls.length, num: true, cls: 'wrap', tip: 'Руководители проектов сотрудника — им делегировать задачи', f: a => `<div class="plist"><b class="pc">${a.ls.length}</b>${leadChips(a.n, a.ls)}</div>` },
    { h: 'Проекты', v: a => a.pr.length, num: true, cls: 'wrap', f: a => projList(a.n, a.pr) },
  ]
})

// ---------- Перегружены ----------
type OA = { n: string; pos: string; tl: boolean; pm: boolean; pr: string[]; bl: number; wk: number; rw: number; blStale: number; blAge: number
  bk: { k: string; bl: number }[]; ls: { l: string; keys: string[]; t: number }[]; rec: ReturnType<PeopleLogic['receiversFor']> }
const overRows = computed<OA[]>(() => {
  const { pl, m } = props
  return Object.keys(m.PEOPLE).filter(pl.overVisible).map(n => {
    const rows = m.PEOPLE[n].rows, a: any = { n, pos: m.roleOf(n), tl: m.isTL(n), pm: pl.isPMlike(n), pr: rows.map(r => r.k) }
    ;(['bl', 'wk', 'rw', 'blStale'] as const).forEach(f => { a[f] = pl.sumRows(rows, f) }); a.blAge = Math.max(0, ...rows.filter(r => r.bl).map(r => r.blAge))
    a.bk = rows.filter(r => r.bl).sort((x, y) => y.bl - x.bl); a.ls = pl.leadsOf(n, a.pr); a.rec = pl.receiversFor(n, a.bk.map((r: { k: string }) => r.k))
    return a as OA
  })
})
const overSum = computed(() => { const r = overRows.value; return `<span><b>${r.length}</b> чел.</span><span>Задач в их бэклоге <b>${sumOf(r, 'bl')}</b></span><span>Есть кому отдать <b>${r.filter(a => a.rec.length).length}</b></span>` })
const overCols = computed<Col<OA>[]>(() => {
  const { jq, nl, who, ST } = props.L, m = props.m
  return [
    { h: 'Сотрудник', v: a => a.n, f: a => esc(a.n) + `<div class="flags">${a.tl ? '<span class="pill mute">тимлид</span> ' : ''}${a.pm ? '<span class="pill mute">ПМ</span>' : ''}</div>` },
    { h: 'Роль', v: a => a.pos, f: a => pos(a.pos) },
    { h: 'Бэклог', v: a => a.bl, num: true, f: a => nl(a.bl, jq([who(a.n), ST.bl]), a.bl >= 15 ? 'hi' : 'mid') },
    { h: 'Без движения >7 дн.', v: a => a.blStale, num: true, f: a => a.blStale ? nl(a.blStale, jq([who(a.n), ST.bl, 'updated <= -7d'])) : '<span class="sub">0</span>' },
    { h: 'Самая старая, дн.', v: a => a.blAge, num: true, f: a => `<span class="${lvl(a.blAge, 30, 90)}">${a.blAge}</span>` },
    { h: 'В работе', v: a => a.wk, num: true, f: a => nl(a.wk, jq([who(a.n), ST.wk])) },
    { h: 'Реворк', v: a => a.rw, num: true, f: a => nl(a.rw, jq([who(a.n), ST.rw]), a.rw ? 'mid' : '') },
    { h: 'Кому отдать', v: a => a.rec.length, num: true, cls: 'wrap', tip: 'Сотрудники тех же проектов, у которых заканчиваются задачи. Число — сколько задач у них осталось',
      f: a => a.rec.length ? `<div class="plist">${a.rec.slice(0, 8).map(x => `<a class="who wl${x.same ? ' same' : ''}" href="${jq([who(a.n), pin(x.keys), ST.bl])}" target="_blank" rel="noopener" title="${esc(x.n)} · ${esc(x.pos)} · осталось задач: ${x.q}. Ссылка открывает бэклог перегруженного в общих проектах: ${x.keys.join(', ')}">${esc(x.n)} <b>${x.q}</b></a>`).join('')}${a.rec.length > 8 ? `<span class="sub">+${a.rec.length - 8}</span>` : ''}</div>` : '<span class="sub">свободных в этих проектах нет</span>' },
    { h: 'Руководители (ПМ)', v: a => a.ls.length, num: true, cls: 'wrap', f: a => `<div class="plist"><b class="pc">${a.ls.length}</b>${leadChips(a.n, a.ls)}</div>` },
    { h: 'Проекты с бэклогом', v: a => a.bk.length, num: true, cls: 'wrap', f: a => `<div class="plist"><b class="pc">${a.bk.length}</b>${a.bk.map(r => `<a class="pl" href="${jq([who(a.n), 'project = "' + r.k + '"', ST.bl])}" target="_blank" rel="noopener" title="${esc(m.pname(r.k))} · ${esc(m.LEAD(r.k))}">${r.k} <b>${r.bl}</b></a>`).join(' ')}</div>` },
  ]
})

// ---------- Переключаются ----------
type SA = { n: string; pos: string; tl: boolean; active: ReturnType<PeopleLogic['switchInfo']>['active']; stalled: ReturnType<PeopleLogic['switchInfo']>['stalled']; stBl: number; stStale: number; stAge: number }
const switchRows = computed<SA[]>(() => {
  const { pl, m } = props
  return Object.keys(m.PEOPLE).filter(pl.switchVisible).map(n => {
    const s = pl.switchInfo(n)
    return { n, pos: m.roleOf(n), tl: m.isTL(n), active: s.active, stalled: s.stalled, stBl: sumOf(s.stalled, 'bl'), stStale: sumOf(s.stalled, 'blStale'), stAge: Math.max(...s.stalled.map(g => g.blAge)) }
  })
})
const switchSum = computed(() => `<span><b>${switchRows.value.length}</b> чел.</span><span>Задач застряло <b>${sumOf(switchRows.value, 'stBl')}</b></span>`)
const grpCell = (g: SA['active'][number], n: string, kind: 'act' | 'st') => {
  const { jq, nl, who, ST } = props.L
  return `<div class="grp"><span class="who">${esc(g.lead)}</span> ` + (kind === 'act'
    ? `<span class="sub">в работе</span> ${nl(g.act, jq([who(n), pin(g.keys), 'status in (10011, 10008, 10000)']))}`
    : `<span class="sub">в бэклоге</span> ${nl(g.bl, jq([who(n), pin(g.stalledKeys), ST.bl]), 'hi')}`) + ` <span class="sub">${(kind === 'act' ? g.keys : g.stalledKeys).join(', ')}</span></div>`
}
const switchCols: Col<SA>[] = [
  { h: 'Сотрудник', v: a => a.n, f: a => esc(a.n) + (a.tl ? ' <span class="pill mute">тимлид</span>' : '') + `<div class="flags"><span class="pill bad has-tip" tabindex="0" data-tipn="${esc(a.n)}">почему ⓘ</span></div>` },
  { h: 'Роль', v: a => a.pos, f: a => pos(a.pos) },
  { h: 'Работает на', v: a => a.active.map(g => g.lead).join(), cls: 'wrap', f: a => a.active.map(g => grpCell(g, a.n, 'act')).join('') },
  { h: 'Стоит у', v: a => a.stalled.map(g => g.lead).join(), cls: 'wrap', f: a => a.stalled.map(g => grpCell(g, a.n, 'st')).join('') },
  { h: 'Застряло в бэклоге', v: a => a.stBl, num: true, f: a => `<span class="${lvl(a.stBl, 3, 8)}">${a.stBl}</span>` },
  { h: 'Из них без движения >7 дн.', v: a => a.stStale, num: true },
  { h: 'Самая старая, дн.', v: a => a.stAge, num: true, tip: 'Сколько дней назад создана самая старая задача в бэклоге', f: a => `<span class="${lvl(a.stAge, 30, 90)}">${a.stAge}</span>` },
]

// ---------- Тимлиды и бэклог ----------
type TA = { n: string; inGroup: boolean; isPL: boolean; pos: string; bl: number; blStale: number; blAge: number; act: number; blKeys: string[]; myProj: string[]; idle: string[]; team: number; flag: boolean }
const tlRows = computed<TA[]>(() => {
  const { pl, m, ctl } = props
  const names = new Set(m.TL); if (ctl.tPl) m.PR.forEach(p => names.add(p.lead))
  return [...names].map(n => {
    const rows = (m.PEOPLE[n] ? m.PEOPLE[n].rows : []).filter(r => props.inS(r.k))
    const myProj = m.PR.filter(p => p.lead === n && props.inS(p.k)).map(p => p.k)
    const team = new Set<string>(); m.AX.forEach(r => { if (myProj.includes(r.project_key) && r.name !== n) team.add(r.name) })
    const idle = [...team].filter(x => m.PEOPLE[x] && pl.needFlag(x))
    const a: TA = { n, inGroup: m.isTL(n), isPL: m.PR.some(p => p.lead === n), pos: m.roleOf(n), bl: pl.sumRows(rows, 'bl'), blStale: pl.sumRows(rows, 'blStale'),
      blAge: Math.max(0, ...rows.filter(r => r.bl).map(r => r.blAge)), act: pl.sumRows(rows, 'wk') + pl.sumRows(rows, 'cr') + pl.sumRows(rows, 'rw'),
      blKeys: rows.filter(r => r.bl).map(r => r.k), myProj, idle, team: team.size, flag: false }
    a.flag = a.bl >= (ctl.tTh || 1)
    return a
  }).filter(a => a.bl > 0 || a.myProj.length)
})
const tlSum = computed(() => `<span><b>${tlRows.value.filter(a => a.flag).length}</b> держат бэклог</span><span>Задач на тимлидах в бэклоге <b>${sumOf(tlRows.value, 'bl')}</b></span>`)
const tlCols = computed<Col<TA>[]>(() => {
  const { jq, nl, who, projQ, pl: plink, ST } = props.L
  return [
    { h: 'Тимлид', v: a => a.n, f: a => esc(a.n) + `<div class="flags">${a.inGroup ? '<span class="pill mute">adata-team-leads</span> ' : ''}${a.isPL ? '<span class="pill mute">рук. проекта</span>' : ''}</div>` },
    { h: 'Состояние', v: a => a.flag ? 1 : 0, num: true, f: a => a.flag ? '<span class="pill bad">Держит бэклог</span>' : a.bl ? '<span class="pill warn">Немного</span>' : '<span class="pill ok">Распределено</span>' },
    { h: 'Бэклог на себе', v: a => a.bl, num: true, f: a => nl(a.bl, jq([who(a.n), projQ(), ST.bl]), a.flag ? 'hi' : '') },
    { h: 'Без движения >7 дн.', v: a => a.blStale, num: true },
    { h: 'Самая старая, дн.', v: a => a.blAge, num: true, f: a => a.blAge ? `<span class="${lvl(a.blAge, 30, 90)}">${a.blAge}</span>` : '<span class="sub">—</span>' },
    { h: 'Где держит', v: a => a.blKeys.join(' '), cls: 'wrap', f: a => a.blKeys.map(k => plink(k, jq([who(a.n), 'project = "' + k + '"', ST.bl]))).join(' ') || '<span class="sub">—</span>' },
    { h: 'Сам в работе', v: a => a.act, num: true, tip: 'В работе + Код ревью + Реворк' },
    { h: 'Без задач в его проектах', v: a => a.idle.length, num: true, tip: 'Сотрудники проектов, где он руководитель, у которых заканчиваются задачи', f: a => a.idle.length ? `<span class="mid" title="${esc(a.idle.join(', '))}">${a.idle.length}</span> <span class="sub">из ${a.team}</span>` : (a.myProj.length ? `<span class="sub">0 из ${a.team}</span>` : '<span class="sub">—</span>') },
  ]
})

// ---------- Дэшборды ----------
const dashNeed = computed(() => {
  const { pl, m } = props
  const needN = Object.keys(m.PEOPLE).filter(pl.needVisible), need0 = needN.filter(n => pl.needInfo(n) === 0).length
  const nr = needN.filter(n => !m.PR.some(p => p.lead === n)).map(n => {
    const rows = m.PEOPLE[n].rows
    return { n, pos: m.roleOf(n), q: pl.needInfo(n), cr: pl.sumRows(rows, 'cr'), ts: pl.sumRows(rows, 'ts'),
      pr: [...new Set(rows.filter(r => r.bl + r.wk + r.rw + r.cr + r.ts > 0 || props.inS(r.k)).map(r => r.k))] }
  }).sort((a, b) => a.q - b.q || a.cr + a.ts - (b.cr + b.ts) || a.n.localeCompare(b.n, 'ru'))
  return { nr, need0 }
})
const dnSub = computed(() => { const { nr, need0 } = dashNeed.value; return nr.length ? `${nr.length} чел. с ≤ ${props.ctl.nTh} задачами (бэклог + в работе + реворк), из них ${need0} совсем без задач. Руководители проектов не показаны. Первые 10 — самые незагруженные.` : 'Сейчас всем хватает задач.' })
const dnTable = computed(() => {
  const { jq, nl, who, pl: plink } = props.L
  return '<thead><tr><th>Сотрудник</th><th>Роль</th><th class=n>Осталось</th><th class=n>Ревью/тест</th><th>Проекты</th></tr></thead><tbody>' +
    (dashNeed.value.nr.slice(0, 10).map(a => `<tr><td>${esc(a.n)}${(x => x ? ` <span class="pill warn" title="Скоро отсутствует">${esc(absTxt(x, props.m.SNAP))}</span>` : '')(props.pl.absSoon(a.n, 14))}</td><td class="sub">${esc(a.pos)}</td><td class=n>${a.q === 0 ? '<span class="pill bad">0 · нет задач</span>' : `<span class="pill warn">${a.q}</span>`}</td><td class=n>${nl(a.cr + a.ts, jq([who(a.n), 'status not in (10100, 10011, 10000, 10001)']))}</td><td>${a.pr.slice(0, 3).map(k => plink(k, jq([who(a.n), 'project = "' + k + '"']))).join(' ')}${a.pr.length > 3 ? ` <span class="sub">+${a.pr.length - 3}</span>` : ''}</td></tr>`).join('') || '<tr><td colspan=5 class="sub empty">Никого нет</td></tr>') + '</tbody>'
})
const dashAbs = computed(() => props.pl.absRows(14, false).sort((a, b) => (+b.now - +a.now) || (a.fr < b.fr ? -1 : 1)))
const daSub = computed(() => { const ar = dashAbs.value; return ar.length ? `${ar.filter(a => a.now).length} отсутствуют сейчас, ${ar.filter(a => !a.now).length} — в ближайшие 14 дней. Отпуска — Bitrix, больничные и «отпросился» — Telegram.` : 'Отсутствий нет.' })
const daTable = computed(() => {
  const { jq, nl, who } = props.L, ar = dashAbs.value
  return '<thead><tr><th>Сотрудник</th><th>Отсутствие</th><th class=n>Открытых задач</th><th>Риск</th></tr></thead><tbody>' +
    (ar.slice(0, 12).map(a => `<tr><td>${esc(a.n)}<div class="sub">${esc(a.pos)}</div></td><td><span class="pill ${a.now ? 'bad' : 'warn'}">${esc(absTxt(a, props.m.SNAP))}</span></td><td class=n>${nl(a.q, jq([who(a.n), OPENQ]))}</td><td>${a.endRisk ? '<span class="pill bad">конец спринта</span>' : a.q ? '<span class="pill warn">задачи висят</span>' : '<span class="sub">—</span>'}</td></tr>`).join('') || '<tr><td colspan=4 class="sub empty">Никого нет</td></tr>') +
    '</tbody>' + (ar.length > 12 ? `<tfoot><tr><td colspan=4 class="sub">ещё ${ar.length - 12} — на вкладке «Отсутствия»</td></tr></tfoot>` : '')
})
type LR = { l: string; ps: P[]; keys: string[]; T: number; dn: number; done: number; tm: number; stuck: P[]; lagP: P[]; noNext: P[]; blk: number; need: string[]; over: string[]; sw: string[]; abs: AR[]; risk: number }
const leadRows = computed<LR[]>(() => {
  const { pl, m } = props, NSD = m.NSD
  const L: Record<string, P[]> = {}; props.scopedP.forEach(p => (L[p.lead] ??= []).push(p))
  const ab = pl.absRows(14, true)
  return Object.entries(L).map(([l, ps]) => {
    const keys = ps.map(p => p.k), T = sumOf(ps, 't'), dn = sumOf(ps, 'dn')
    const tm = Math.round(ps.reduce((a, p) => a + p.time * p.t, 0) / Math.max(1, T))
    const stuck = ps.filter(p => p.left < 0), lagP = ps.filter(p => p.time - p.done > 25), noNext = ps.filter(p => (!NSD[p.k] || !NSD[p.k].t) && p.left <= 3)
    const inK = (n: string) => m.PEOPLE[n].rows.filter(r => keys.includes(r.k))
    const need = Object.keys(m.PEOPLE).filter(n => pl.needFlag(n) && inK(n).length && n !== l).sort((a, b) => pl.needInfo(a) - pl.needInfo(b))
    const over = Object.keys(m.PEOPLE).filter(n => pl.overFlag(n) && inK(n).some(r => r.bl))
    const sw = Object.keys(m.PEOPLE).filter(n => { const s = pl.switchInfo(n); return s.flag && s.stalled.some(g => g.lead === l) })
    const abs = ab.filter(x => x.rk.some(r => keys.includes(r.k)))
    const risk = stuck.length * 3 + lagP.length * 2 + noNext.length + sumOf(ps, 'blk') / 5 + abs.filter(x => x.endRisk).length * 2 + sw.length
    return { l, ps, keys, T, dn, done: pct(dn, T), tm, stuck, lagP, noNext, blk: sumOf(ps, 'blk'), need, over, sw, abs, risk }
  })
})
const dleadCols = computed<Col<LR>[]>(() => {
  const { jq, nl, who, pl: plink, ST } = props.L, pl = props.pl
  return [
    { h: 'Руководитель', v: x => x.l, f: x => `<button class="lbtn" data-lead="${esc(x.l)}">${esc(x.l)}</button><div class="sub">${x.ps.length} ${x.ps.length === 1 ? 'проект' : x.ps.length < 5 ? 'проекта' : 'проектов'}</div>` },
    { h: 'Проекты', v: x => x.ps.length, f: x => `<div class="dlpr">${x.keys.map(k => plink(k, jq([`project = "${k}"`]))).join('')}</div>` },
    { h: 'Задач', v: x => x.T, num: true, f: x => nl(x.T, jq([pin(x.keys)])) },
    { h: 'Готовность / время', v: x => x.done - x.tm, num: true, tip: 'Доля задач в «Готово» против прошедшего времени спринтов (взвешено по задачам). Красным — отставание больше 25 п.п.', f: x => `<div style="display:flex;align-items:center;gap:8px;min-width:150px"><div class="bar" style="flex:1;position:relative"><i style="width:${x.done}%;background:var(--s-dn)"></i><span style="position:absolute;left:${x.tm}%;top:-3px;bottom:-3px;width:2px;background:var(--fg)" title="Прошло времени ${x.tm}%"></span></div><b class="${x.tm - x.done > 25 ? 'hi' : ''}">${x.done}%</b><span class="sub">/ ${x.tm}%</span></div>` },
    { h: 'Риски по спринтам', v: x => x.stuck.length * 3 + x.lagP.length * 2 + x.noNext.length, num: true, cls: 'wrap', f: x => [
      x.stuck.length ? `<span class="pill bad" title="${x.stuck.map(p => p.k).join(', ')}">не закрыто: ${x.stuck.map(p => p.k).join(', ')}</span>` : '',
      x.lagP.length ? `<span class="pill bad" title="Готовность отстаёт от времени больше 25 п.п.">отстают: ${x.lagP.map(p => p.k).join(', ')}</span>` : '',
      x.noNext.length ? `<span class="pill warn">нет след. спринта: ${x.noNext.map(p => p.k).join(', ')}</span>` : '',
      x.blk ? `<a class="pill bad" href="${jq([pin(x.keys), 'status = "Blocked"'])}" target="_blank" rel="noopener">Blocked ${x.blk}</a>` : '',
    ].filter(Boolean).join(' ') || '<span class="pill ok">в норме</span>' },
    { h: 'Нужны задачи', v: x => x.need.length, num: true, cls: 'wrap', f: x => nmList(x.need.map(n => nameChip(n, jq([who(n)]), pl.needInfo(n) === 0 ? 'bad' : 'warn', pl.needInfo(n) === 0 ? '0' : String(pl.needInfo(n))))) },
    { h: 'Отсутствуют', v: x => x.abs.length, num: true, cls: 'wrap', f: x => nmList(x.abs.map(a => nameChip(a.n, jq([who(a.n), OPENQ, pin(x.keys)]), a.endRisk || a.now ? 'bad' : 'warn', absTxt(a, props.m.SNAP)))) },
    { h: 'Перегружены', v: x => x.over.length, num: true, cls: 'wrap', f: x => nmList(x.over.map(n => nameChip(n, jq([who(n), ST.bl]), 'bad', pl.blTotal(n) + ' в бэклоге'))) },
    { h: 'Тормозят его проекты', v: x => x.sw.length, num: true, cls: 'wrap', f: x => nmList(x.sw.map(n => nameChip(n, jq([who(n), ST.bl, pin(x.keys)]), 'bad'))) },
  ]
})
function onDashClick(e: MouseEvent) {
  const t = e.target as HTMLElement
  const b = t.closest<HTMLElement>('[data-lead]'); if (b) { emit('lead', b.dataset.lead!); return }
  const g = t.closest<HTMLElement>('[data-go]'); if (g) { emit('go', g.dataset.go!); scrollTo({ top: 0, behavior: 'smooth' }) }
}
</script>

<template>
  <section v-if="tab === 'dash'" @click="onDashClick">
    <h2>Дэшборды</h2>
    <h3 class="dh">Сводка по руководителям проектов</h3>
    <p class="lead">Одна строка — один руководитель: его проекты, готовность спринтов против прошедшего времени, риски и люди, на которых стоит обратить внимание. Нажмите на имя руководителя, чтобы отфильтровать весь отчёт по его проектам.</p>
    <SortTable :cols="dleadCols" :rows="leadRows" :init="[4, 'desc']" />
    <div class="dtops">
      <div class="dtop">
        <h3><span>Кому критически нужны задачи</span><button data-go="need">все →</button></h3>
        <p class="sub" style="margin:0 0 6px">{{ dnSub }}</p>
        <div class="tbl"><table v-html="dnTable" /></div>
      </div>
      <div class="dtop">
        <h3><span>Кто отсутствует (сейчас и в ближайшие 14 дней)</span><button data-go="abs">все →</button></h3>
        <p class="sub" style="margin:0 0 6px">{{ daSub }}</p>
        <div class="tbl"><table v-html="daTable" /></div>
      </div>
    </div>
  </section>

  <section v-else-if="tab === 'people'">
    <h2>Загрузка ресурсов</h2>
    <p class="lead">Задачи исполнителей в выбранных проектах. «Тестируется» — все статусы, кроме Сделать, В работе, Code review, Need Rework и Готово. Нажмите на число или код проекта, чтобы открыть эти задачи в Jira.</p>
    <div class="tools">
      <select v-model="pFlag" aria-label="Показать">
        <option value="">Все сотрудники</option><option value="need">Нужны задачи</option><option value="over">Перегружены бэклогом</option>
        <option value="switch">Переключаются между руководителями</option><option value="tl">Тимлиды</option><option value="abs">Отпуск / больничный</option>
      </select>
      <select v-model="fPos" aria-label="Роль">
        <option value="">Все роли</option><option v-for="p in allPos" :key="p">{{ p }}</option>
      </select>
      <input v-model="qPeople" type="search" placeholder="Поиск по имени">
    </div>
    <div class="psum" v-html="peopleSum" />
    <SortTable :cols="peopleCols" :rows="peopleRows" :init="[3, 'desc']" />
  </section>

  <section v-else-if="tab === 'lres'">
    <h2>Ресурсы по руководителям</h2>
    <p class="lead">Какую долю всех сотрудников занимает каждый руководитель в текущих спринтах. Если человек работает в проектах нескольких руководителей, он делится между ними пропорционально числу своих задач в спринте (например, 6 задач у одного и 2 у другого — это 0,75 и 0,25 человека). Уволенные и задачи без исполнителя не учитываются. Нажмите на строку руководителя, чтобы увидеть, сколько ресурсов занимает каждый его проект.</p>
    <div class="psum" v-html="lresSum" />
    <SortTable :cols="lresCols" :rows="LRES.L" :init="[2, 'desc']" :ex="lresEx" />
  </section>

  <section v-else-if="tab === 'abs'">
    <h2>Отпуска, больничные и отгулы: задачи, которые никто не закроет</h2>
    <p class="lead">Сотрудник в отпуске или на больничном, а на нём висят задачи спринта в бэклоге, в работе или в реворке. Они не закроются, пока его нет, и план спринта не выполнится, если их не переназначить. Отпуска (трудовой и без содержания) — из Bitrix, больничные и «отпросился» (от 2 часов за день) — из Telegram-группы «Adata Group». Метка «конец спринта» — отсутствие попадает на последние 2 дня спринта, где у сотрудника есть незакрытые задачи. {{ absSrc }}</p>
    <div class="tools">
      <label class="gf"><span>Показывать отсутствия на ближайшие, дн.</span><input v-model.number="ctl.aH" type="number" min="0" max="60"></label>
      <label class="chk"><input v-model="ctl.aRisk" type="checkbox"> Только с задачами</label>
    </div>
    <div class="psum" v-html="absSum" />
    <SortTable :cols="absCols" :rows="absRowsTab" :init="[2, 'asc']" />
  </section>

  <section v-else-if="tab === 'need'">
    <h2>Сотрудники, у которых заканчиваются задачи</h2>
    <p class="lead">Считается всё, что сотрудник ещё может взять в работу во всех проектах: Бэклог + В работе + Реворк. Задачи на ревью и в тестировании не считаются — они ждут других. Тимлиду этих людей нужно делегировать задачи. В колонке «У кого взять задачи» — коллеги по тем же проектам с бэклогом больше порога: только с той же ролью (фронт у фронта, бэк у бэка и т.д.) и ПМ, задачи которых может взять любой. ПМ — сотрудник с ролью Project Manager или руководитель проекта (учитывается бэклог только в его собственных проектах). Если роль сотрудника в Jira не указана, показываются только ПМ. Число — их бэклог в общих проектах, нажмите, чтобы открыть эти задачи.</p>
    <div class="tools">
      <label class="gf"><span>Осталось задач не больше</span><input v-model.number="ctl.nTh" type="number" min="0" max="20"></label>
      <label class="gf"><span>Брать у тех, у кого в бэклоге больше</span><input v-model.number="ctl.dTh" type="number" min="0" max="50"></label>
      <label class="chk"><input v-model="ctl.nAll" type="checkbox"> Показывать PM, QA и тимлидов</label>
    </div>
    <div class="psum" v-html="needSum" />
    <SortTable :cols="needCols" :rows="needRows" :init="[3, 'asc']" />
  </section>

  <section v-else-if="tab === 'over'">
    <h2>Сотрудники с перегрузом в бэклоге</h2>
    <p class="lead">У кого в статусе «Сделать» скопилось слишком много задач спринта по всем проектам. Эти задачи стоит раздать. В колонке «Кому отдать» — сотрудники из тех же проектов, у которых заканчиваются задачи (по правилам вкладки «Нужны задачи»), с той же ролью. Если перегружен ПМ или руководитель проекта, предлагаются все, у кого заканчиваются задачи, — задачу ПМ может взять любой.</p>
    <div class="tools">
      <label class="gf"><span>В бэклоге не меньше</span><input v-model.number="ctl.oTh" type="number" min="1" max="100"></label>
      <label class="chk"><input v-model="ctl.oAll" type="checkbox"> Показывать ПМ и тимлидов</label>
    </div>
    <div class="psum" v-html="overSum" />
    <SortTable :cols="overCols" :rows="overRows" :init="[2, 'desc']" />
  </section>

  <section v-else-if="tab === 'switch'">
    <h2>Сотрудники, которые тормозят проект</h2>
    <p class="lead">Сотрудник работает над задачами одного руководителя, а задачи другого руководителя у него лежат в бэклоге и не двигаются (нет ни одной задачи в работе, ревью или реворке). Из-за этого проект второго руководителя стоит.</p>
    <div class="tools">
      <label class="gf"><span>Застряло в бэклоге не меньше</span><input v-model.number="ctl.sTh" type="number" min="1" max="50"></label>
    </div>
    <div class="psum" v-html="switchSum" />
    <SortTable :cols="switchCols" :rows="switchRows" :init="[4, 'desc']" />
  </section>

  <section v-else-if="tab === 'tl'">
    <h2>Тимлиды, которые держат бэклог на себе</h2>
    <p class="lead">Тимлиды из группы <code>adata-team-leads</code>: сколько задач спринта в статусе «Сделать» назначено на них самих. Такие задачи не распределены — команда не видит весь объём работы. Рядом — сколько сотрудников в проектах тимлида остаются без задач.</p>
    <div class="tools">
      <label class="gf"><span>Держит в бэклоге не меньше</span><input v-model.number="ctl.tTh" type="number" min="1" max="50"></label>
      <label class="chk"><input v-model="ctl.tPl" type="checkbox"> Добавить руководителей проектов не из группы</label>
    </div>
    <div class="psum" v-html="tlSum" />
    <SortTable :cols="tlCols" :rows="tlRows" :init="[2, 'desc']" />
  </section>
</template>
