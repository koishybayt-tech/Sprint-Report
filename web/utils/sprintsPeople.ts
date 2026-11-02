// Логика вкладок по людям из reference/sprints/page_template.html (needFlag, switchInfo, donorsFor, LRES…), 1-в-1.
// Отсутствия (ABS): отпуска и БС из Bitrix, позже больничные и «отпросился» из Telegram.
import { type Model, type Row, UNASSIGNED, esc, fmt, jiraLinks } from './sprints'

// Пороги и галочки вкладок; общие, потому что needFlag/overFlag используются и в KPI, и в дашборде.
export type Ctl = {
  nTh: number; dTh: number; nAll: boolean // «Нужны задачи»: осталось ≤, брать у тех, у кого > , показывать PM/QA/тимлидов
  oTh: number; oAll: boolean // «Перегружены»: в бэклоге ≥, показывать ПМ и тимлидов
  sTh: number // «Переключаются»: застряло ≥
  tTh: number; tPl: boolean // «Тимлиды»: держит ≥, добавить руководителей проектов
  aH: number; aRisk: boolean // «Отсутствия»: на ближайшие N дней, только с задачами
}
export const defaultCtl = (): Ctl => ({ nTh: 2, dTh: 2, nAll: false, oTh: 8, oAll: false, sTh: 1, tTh: 3, tPl: false, aH: 14, aRisk: true })

export const ABT: Record<string, string> = { vac: 'Отпуск', bs: 'Отпуск б/с', sick: 'Больничный', off: 'Отпросился' }
export const OPENQ = 'status in (10100, 10011, 10000)'
export const addD = (d: string, k: number) => { const x = new Date(d + 'T12:00:00'); x.setDate(x.getDate() + k); return x.toISOString().slice(0, 10) }
export type Ab = Model['AB'][number]
export const absTxt = (a: Ab, SNAP: string) => a.ty === 'off'
  ? (a.fr > SNAP ? 'отпросился на ' + fmt(a.fr) : a.fr === SNAP ? 'отпросился сегодня' : 'отпросился ' + fmt(a.fr))
  : a.fr > SNAP ? `${ABT[a.ty].toLowerCase()} с ${fmt(a.fr)}` : ABT[a.ty] + (a.to ? ` до ${fmt(a.to)}` : ' · выход не указан')

type L = ReturnType<typeof jiraLinks>
export type Group = { lead: string; keys: string[]; bl: number; act: number; wk: number; cr: number; rw: number
  blStale: number; blAge: number; blUpd: number; stalledKeys: string[] }

export function peopleLogic(m: Model, ctl: Ctl, inS: (k: string) => boolean, allScope: () => boolean) {
  const { PEOPLE, PR, PBY, LEAD, roleOf, isTL, INACT, AB, SNAP } = m
  const sumRows = (rows: Row[], f: keyof Row) => rows.reduce((a, r) => a + (r[f] as number), 0)

  // need: очередь по ВСЕМ проектам
  const needInfo = (n: string) => { const rows = PEOPLE[n].rows; return sumRows(rows, 'bl') + sumRows(rows, 'wk') + sumRows(rows, 'rw') }
  // switch: группировка по руководителю проекта
  function switchInfo(n: string) {
    const G: Record<string, Group> = {}
    PEOPLE[n].rows.forEach(r => {
      const l = LEAD(r.k)
      const g = G[l] ??= { lead: l, keys: [], bl: 0, act: 0, wk: 0, cr: 0, rw: 0, blStale: 0, blAge: 0, blUpd: -1, stalledKeys: [] }
      g.keys.push(r.k); g.bl += r.bl; g.wk += r.wk; g.cr += r.cr; g.rw += r.rw; g.act += r.wk + r.cr + r.rw; g.blStale += r.blStale
      g.blAge = Math.max(g.blAge, r.blAge); if (r.bl && r.blUpd >= 0) g.blUpd = g.blUpd < 0 ? r.blUpd : Math.min(g.blUpd, r.blUpd)
    })
    const gs = Object.values(G), active = gs.filter(g => g.act > 0), stalled = gs.filter(g => g.bl > 0 && g.act === 0 && g.lead !== n)
    stalled.forEach(g => { g.stalledKeys = PEOPLE[n].rows.filter(r => LEAD(r.k) === g.lead && r.bl > 0).map(r => r.k) })
    return { active, stalled, flag: active.length > 0 && stalled.length > 0 }
  }
  // ---------- отсутствия ----------
  const TODAY = new Date(SNAP + 'T12:00:00')
  const days = (d: string) => Math.round((new Date(d + 'T12:00:00').getTime() - TODAY.getTime()) / 864e5)
  const absNow = (n: string) => AB.find(a => a.n === n && a.fr <= SNAP && (!a.to || a.to >= SNAP))
  const absSoon = (n: string, h: number) => AB.filter(a => a.n === n && a.fr > SNAP && a.fr <= addD(SNAP, h)).sort((x, y) => x.fr < y.fr ? -1 : 1)[0]
  type RK = { k: string; q: number; en: string; hit: boolean }
  type AbRow = Ab & { pos: string; now: boolean; pr: string[]; bl: number; wk: number; rw: number; cr: number; ts: number; dn: number; q: number
    rk: RK[]; ls: { l: string; keys: string[]; t: number }[]; endRisk: boolean; when: number }
  function absRows(H: number, onlyRisk: boolean): AbRow[] {
    const lim = addD(SNAP, H)
    return AB.filter(a => (!a.to || a.to >= SNAP) && a.fr <= lim).map(a => {
      const P = PEOPLE[a.n], rows = P ? P.rows.filter(r => inS(r.k)) : []
      if (!rows.length && !allScope()) return null
      const x: any = { ...a, pos: roleOf(a.n), now: a.fr <= SNAP, pr: rows.map(r => r.k) }
      ;(['bl', 'wk', 'rw', 'cr', 'ts', 'dn'] as const).forEach(f => { x[f] = sumRows(rows, f) }); x.q = x.bl + x.wk + x.rw
      x.rk = rows.filter(r => r.bl + r.wk + r.rw > 0).map(r => { const p = PBY[r.k]; return { k: r.k, q: r.bl + r.wk + r.rw, en: p.en, hit: p.en >= a.fr && (!a.to || addD(p.en, -2) <= a.to) } }).sort((u: RK, v: RK) => v.q - u.q)
      const LM: Record<string, { l: string; keys: string[]; t: number }> = {}
      x.rk.forEach((r: RK) => { const l = LEAD(r.k); const mm = LM[l] ??= { l, keys: [], t: 0 }; mm.keys.push(r.k); mm.t += r.q })
      x.ls = Object.values(LM).sort((u, v) => v.t - u.t); x.endRisk = x.rk.some((r: RK) => r.hit); x.when = x.now ? (a.to ? days(a.to) : 999) : days(a.fr)
      return x as AbRow
    }).filter((x): x is AbRow => !!x && (!onlyRisk || x.q > 0))
  }
  const absVisibleRisk = () => absRows(0, true).filter(x => x.now).length

  const EXCL = (n: string) => isTL(n) || /Project Manager|QA Engineer/.test(roleOf(n)) || n === UNASSIGNED
  function needFlag(n: string) {
    if (n === UNASSIGNED || absNow(n) || INACT.includes(n)) return false
    return needInfo(n) <= (ctl.nTh || 0) && (!EXCL(n) || ctl.nAll)
  }
  type Donor = { n: string; pos: string; bl: number; keys: string[]; same: boolean; pm: boolean }
  function donorsFor(n: string, keys: string[]) {
    const th = ctl.dTh || 0, me = roleOf(n), M: Record<string, Donor> = {}
    m.AX.forEach(r => {
      if (r.name === n || r.name === UNASSIGNED || !keys.includes(r.project_key) || !r.todo) return
      const pos = roleOf(r.name), same = me !== '—' && pos === me, pmRole = /Project Manager/.test(pos), ownProj = LEAD(r.project_key) === r.name
      if (!(same || pmRole || ownProj)) return
      const d = M[r.name] ??= { n: r.name, pos, bl: 0, keys: [], same, pm: !same }
      d.bl += r.todo; d.keys.push(r.project_key)
    })
    return Object.values(M).filter(d => d.bl > th).sort((a, b) => (+b.same - +a.same) || (b.bl - a.bl))
  }
  const needVisible = (n: string) => needFlag(n) && PEOPLE[n].rows.some(r => inS(r.k))

  const isPMlike = (n: string) => /Project Manager/.test(roleOf(n)) || PR.some(p => p.lead === n)
  const blTotal = (n: string) => sumRows(PEOPLE[n].rows, 'bl')
  function overFlag(n: string) {
    if (n === UNASSIGNED) return false
    const ex = isTL(n) || isPMlike(n) || /QA Engineer/.test(roleOf(n))
    return blTotal(n) >= (ctl.oTh || 1) && (!ex || ctl.oAll)
  }
  const overVisible = (n: string) => overFlag(n) && PEOPLE[n].rows.some(r => r.bl && inS(r.k))
  type Rec = { n: string; pos: string; q: number; keys: string[]; same: boolean }
  function receiversFor(n: string, keys: string[]) {
    const me = roleOf(n), pm = isPMlike(n), M: Record<string, Rec> = {}
    m.AX.forEach(r => {
      if (r.name === n || !keys.includes(r.project_key) || !PEOPLE[r.name] || !needFlag(r.name)) return
      const pos = roleOf(r.name); if (!(pm || (me !== '—' && pos === me))) return
      const x = M[r.name] ??= { n: r.name, pos, q: needInfo(r.name), keys: [], same: me !== '—' && pos === me }
      x.keys.push(r.project_key)
    })
    return Object.values(M).sort((a, b) => (+b.same - +a.same) || (a.q - b.q))
  }
  function switchVisible(n: string) {
    const s = switchInfo(n)
    return s.flag && s.stalled.reduce((a, g) => a + g.bl, 0) >= (ctl.sTh || 1) && s.stalled.some(g => g.stalledKeys.some(inS))
  }
  function leadsOf(n: string, keys: string[]) {
    const M: Record<string, { l: string; keys: string[]; t: number }> = {}
    PEOPLE[n].rows.filter(r => keys.includes(r.k)).forEach(r => { const l = LEAD(r.k); const x = M[l] ??= { l, keys: [], t: 0 }; x.keys.push(r.k); x.t += r.t })
    return Object.values(M).sort((a, b) => b.t - a.t)
  }

  return { sumRows, needInfo, switchInfo, absNow, absSoon, absRows, absVisibleRisk, days, EXCL, needFlag, donorsFor, needVisible, isPMlike, blTotal, overFlag,
    overVisible, receiversFor, switchVisible, leadsOf }
}
export type PeopleLogic = ReturnType<typeof peopleLogic>

// Ресурсы по руководителям (LRES): доля человека делится между руководителями пропорционально его задачам.
// Не зависит от фильтра и порогов.
export type LresPerson = { n: string; f: number; t: number; keys: string[] }
export type LresProj = { k: string; fte: number; hc: number; only: number; shared: number; t: number; ppl: LresPerson[]; roles: Record<string, number>; share: number; isP: 1 }
export type LresLead = { l: string; fte: number; hc: number; only: number; shared: number; t: number; keys: string[]; ppl: LresPerson[]; roles: Record<string, number>; pj: Record<string, LresProj>; share: number; sub: LresProj[] }
export function lres(m: Model) {
  const M: Record<string, LresLead> = {}
  let N = 0
  Object.values(m.PEOPLE).forEach(P => {
    if (P.n === UNASSIGNED || m.INACT.includes(P.n)) return
    const rows = P.rows.filter(r => r.t > 0), T = rows.reduce((a, r) => a + r.t, 0)
    if (!T) return
    N++
    const by: Record<string, { t: number; keys: string[] }> = {}
    rows.forEach(r => { const l = m.LEAD(r.k); const b = by[l] ??= { t: 0, keys: [] }; b.t += r.t; b.keys.push(r.k) })
    const nl = Object.keys(by).length, np = rows.length
    Object.entries(by).forEach(([l, b]) => {
      const x = M[l] ??= { l, fte: 0, hc: 0, only: 0, shared: 0, t: 0, keys: [], ppl: [], roles: {}, pj: {}, share: 0, sub: [] }
      const f = b.t / T; x.fte += f; x.hc++; x.t += b.t; if (nl === 1) x.only++; else x.shared++
      b.keys.forEach(k => { if (!x.keys.includes(k)) x.keys.push(k) }); x.ppl.push({ n: P.n, f, t: b.t, keys: b.keys })
      const r = m.roleOf(P.n); x.roles[r] = (x.roles[r] || 0) + f
    })
    rows.forEach(rw => {
      const x = M[m.LEAD(rw.k)]
      const q = x.pj[rw.k] ??= { k: rw.k, fte: 0, hc: 0, only: 0, shared: 0, t: 0, ppl: [], roles: {}, share: 0, isP: 1 }
      const f = rw.t / T; q.fte += f; q.hc++; q.t += rw.t; if (np === 1) q.only++; else q.shared++
      q.ppl.push({ n: P.n, f, t: rw.t, keys: [rw.k] }); const r = m.roleOf(P.n); q.roles[r] = (q.roles[r] || 0) + f
    })
  })
  const L = Object.values(M)
  L.forEach(x => {
    x.share = N ? x.fte / N * 100 : 0; x.keys.sort(); x.ppl.sort((a, b) => b.f - a.f || b.t - a.t)
    x.sub = Object.values(x.pj).map(q => { q.share = N ? q.fte / N * 100 : 0; q.ppl.sort((a, b) => b.f - a.f || b.t - a.t); return q }).sort((a, b) => b.fte - a.fte)
  })
  return { N, L }
}

// Подсказка «почему тормозит проект» (switchTip).
export function switchTip(n: string, pl: PeopleLogic, L: L) {
  const zad = (k: number) => { const a = k % 10, h = k % 100; return k + ' ' + (a === 1 && h !== 11 ? 'задача' : a >= 2 && a <= 4 && (h < 12 || h > 14) ? 'задачи' : 'задач') }
  const s = pl.switchInfo(n), tot = s.stalled.reduce((a, g) => a + g.bl, 0), W = L.who(n)
  const A = (v: number, parts: string[], txt?: string) => `<a class="tl-a" href="${L.jq([W, ...parts])}" target="_blank" rel="noopener">${txt || v}</a>`
  const pin = (keys: string[]) => `project in (${keys.map(k => '"' + k + '"').join(', ')})`
  const actKeys = s.active.flatMap(g => g.keys), stKeys = s.stalled.flatMap(g => g.stalledKeys)
  const act = s.active.map(g => {
    const k = pin(g.keys)
    const parts = [g.wk && `в работе ${A(g.wk, [k, L.ST.wk])}`, g.cr && `на ревью ${A(g.cr, [k, L.ST.cr])}`, g.rw && `в реворке ${A(g.rw, [k, L.ST.rw])}`].filter(Boolean).join(', ')
    return `<li><b>${esc(g.lead)}</b> <span>${g.keys.join(', ')}</span><br>${parts}</li>`
  }).join('')
  const st = s.stalled.map(g => {
    const k = pin(g.stalledKeys)
    return `<li><b>${esc(g.lead)}</b> <span>${g.stalledKeys.join(', ')}</span><br>в бэклоге ${A(g.bl, [k, L.ST.bl])}, в работу не взято ни одной` +
      (g.blStale ? `, без движения &gt;7 дн.: ${A(g.blStale, [k, L.ST.bl, 'updated <= -7d'])}` : '') +
      (g.blAge ? `<br>самая старая создана ${g.blAge} дн. назад` : '') +
      (g.blUpd >= 0 ? `, последнее изменение ${g.blUpd === 0 ? 'сегодня' : g.blUpd + ' дн. назад'}` : '') + '</li>'
  }).join('')
  const aSum = s.active.reduce((a, g) => a + g.act, 0)
  return `<div class="tt-h">Почему тормозит проект</div><p>Ведёт ${A(aSum, [pin(actKeys), 'status in (10011, 10008, 10000)'], zad(aSum))} у ${s.active.length === 1 ? 'одного руководителя' : s.active.length + ' руководителей'}, а ${A(tot, [pin(stKeys), L.ST.bl], zad(tot))} у ${s.stalled.length === 1 ? 'другого руководителя' : s.stalled.length + ' других руководителей'} лежат в бэклоге, и ни одна не взята в работу.</p><div class="tt-s">Работает на</div><ul>${act}</ul><div class="tt-s bad">Стоит у</div><ul>${st}</ul><div class="tt-f">Цифры открывают список задач в Jira</div>`
}
