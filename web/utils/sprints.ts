// Модель отчёта «Спринты» на клиенте: данные из /api/v1/sprints/report, приведённые к именам
// reference/sprints/page_template.html, чтобы формулы и колонки вкладок переносились 1-в-1.

export type ApiProject = {
  key: string; name: string; lead: string; sprint: string; start?: string; end?: string; left_days: number | null
  total: number; todo: number; wk: number; cr: number; rw: number; ts: number; dn: number; bug: number
  carry: number; carry_open: number; added: number; unplanned: number; blocked: number; stale: number
  hours: number; people: number; ready_deploy: number; in_test: number; ready_pct: number; time_pct: number
  oldest_open: { key: string; age_days: number } | null
}
type NameCount = { name: string; n: number }
export type ApiReport = {
  updated_at: string | null
  projects: ApiProject[]
  people: { name: string; project_key: string; t: number; todo: number; wk: number; cr: number; rw: number; ts: number; dn: number
    bug: number; todo_stale: number; act_days: number; any_days: number; todo_age: number; todo_upd: number }[]
  testers: { name: string; project_key: string; t: number; tst: number; dn: number }[]
  bottlenecks: { key: string; summary: string; status: string; assignee: string; days: number; priority: string }[]
  roles: Record<string, string>
  logins: Record<string, string>
  team_leads: string[]
  testing: Record<string, Record<string, { n: number; logins: string[] }>>
  next: Record<string, { next_id: number; next_name: string; next_start?: string; next_end?: string; next_total: number
    statuses: NameCount[]; by: NameCount[] }>
}

export type P = {
  k: string; n: string; lead: string; sp: string; st: string; en: string; t: number; bl: number; wk: number; cr: number
  rw: number; ts: number; dn: number; bug: number; carry: number; add: number; unpl: number; blk: number; stale: number
  h: number; ppl: number; rdy: number; tst: number; oKey: string | null; oAge: number; carryOpen: number
  left: number; done: number; time: number
}
export type Row = { k: string; t: number; bl: number; wk: number; cr: number; rw: number; ts: number; dn: number; bug: number
  blStale: number; act: number; any: number; blAge: number; blUpd: number }
export type NS = { id: number; name: string; st: string; en: string; t: number; ss: Record<string, number>; by: Record<string, number> }

export const UNASSIGNED = '— не назначен'

export const esc = (s: unknown) => String(s).replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]!))
export const fmt = (d?: string | null) => d ? d.slice(8, 10) + '.' + d.slice(5, 7) : '—'
export const pct = (a: number, b: number) => b ? Math.round(a / b * 100) : 0
export const lvl = (v: number, w: number, b: number) => v >= b ? 'hi' : v >= w ? 'mid' : ''
export const sumOf = <T>(arr: T[], f: keyof T) => arr.reduce((a, x) => a + (x[f] as unknown as number), 0)

export function buildModel(r: ApiReport) {
  const PR: P[] = r.projects.map(p => ({
    k: p.key, n: p.name, lead: p.lead, sp: p.sprint, st: p.start ?? '', en: p.end ?? '', t: p.total, bl: p.todo, wk: p.wk,
    cr: p.cr, rw: p.rw, ts: p.ts, dn: p.dn, bug: p.bug, carry: p.carry, add: p.added, unpl: p.unplanned, blk: p.blocked,
    stale: p.stale, h: p.hours, ppl: p.people, rdy: p.ready_deploy, tst: p.in_test,
    oKey: p.oldest_open?.key ?? null, oAge: p.oldest_open?.age_days ?? 0, carryOpen: p.carry_open,
    left: p.left_days ?? Number.NaN, done: p.ready_pct, time: p.time_pct,
  }))
  const PBY = Object.fromEntries(PR.map(p => [p.k, p]))
  const PEOPLE: Record<string, { n: string; rows: Row[] }> = {}
  r.people.forEach(a => {
    const p = PEOPLE[a.name] ??= { n: a.name, rows: [] }
    p.rows.push({ k: a.project_key, t: a.t, bl: a.todo, wk: a.wk, cr: a.cr, rw: a.rw, ts: a.ts, dn: a.dn, bug: a.bug,
      blStale: a.todo_stale, act: a.act_days, any: a.any_days, blAge: a.todo_age, blUpd: a.todo_upd })
  })
  const NSD: Record<string, NS> = Object.fromEntries(Object.entries(r.next).map(([k, n]) => [k, {
    id: n.next_id, name: n.next_name, st: n.next_start ?? '', en: n.next_end ?? '', t: n.next_total,
    ss: Object.fromEntries(n.statuses.map(x => [x.name, x.n])), by: Object.fromEntries(n.by.map(x => [x.name, x.n])),
  }]))
  return {
    PR, PBY, PEOPLE, NSD,
    QX: r.testers, X: r.bottlenecks, U: r.logins, POS: r.roles, TL: r.team_leads, TST: r.testing,
    LEAD: (k: string) => PBY[k]?.lead || '—',
    pname: (k: string) => PBY[k]?.n || k,
    roleOf: (n: string) => r.roles[n] || '—',
    isTL: (n: string) => r.team_leads.includes(n),
  }
}
export type Model = ReturnType<typeof buildModel>

// Ссылки в Jira — как jq/who/nl/pl в шаблоне.
export function jiraLinks(JB: string, m: Model, scope: () => string[]) {
  const BASE = 'cf[10104] in openSprints()'
  const allScope = () => scope().length === m.PR.length
  const who = (n: string, f = 'assignee') => n === UNASSIGNED ? `${f} is EMPTY` : `${f} = "${m.U[n] || n}"`
  const projQ = (keys?: string[]) => { keys = keys || scope(); return keys.length === m.PR.length && allScope() ? '' : `project in (${keys.map(k => '"' + k + '"').join(', ')})` }
  const jq = (parts: string[]) => JB + 'issues/?jql=' + encodeURIComponent([BASE, ...parts.filter(Boolean)].join(' AND ') + ' ORDER BY status ASC, updated DESC')
  const jqs = (id: number, extra?: string[]) => JB + 'issues/?jql=' + encodeURIComponent(['cf[10104] = ' + id, ...(extra || []).filter(Boolean)].join(' AND ') + ' ORDER BY status ASC, updated DESC')
  const nl = (v: number, url: string, cls = '') => v ? `<a class="nl ${cls}" href="${url}" target="_blank" rel="noopener" title="Открыть в Jira">${v}</a>` : '<span class="sub">0</span>'
  const pl = (k: string, url: string) => `<a class="pl" href="${url}" target="_blank" rel="noopener" title="${esc(m.pname(k))} · ${esc(m.LEAD(k))}">${k}</a>`
  const projList = (n: string, keys: string[], field = 'assignee') => `<div class="plist"><b class="pc" title="Проектов">${keys.length}</b>${keys.map(k => pl(k, jq([who(n, field), 'project = "' + k + '"']))).join(' ')}</div>`
  const ST = { bl: 'status = 10100', wk: 'status = 10011', cr: 'status = 10008', rw: 'status = 10000', dn: 'status = 10001',
    ts: 'status not in (10100, 10011, 10008, 10000, 10001)', open: 'status != 10001' }
  return { JB, who, projQ, jq, jqs, nl, pl, projList, ST }
}

// Колонка сортируемой таблицы (как в table() шаблона): f возвращает HTML, данные внутри экранируются esc().
export type Col<T = any> = {
  h: string
  v: (r: T) => string | number
  f?: (r: T) => string
  num?: boolean
  tip?: string
  cls?: string
}
