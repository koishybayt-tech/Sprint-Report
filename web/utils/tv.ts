// Экран для ТВ (/tv): только важные сигналы с одной страницы. Данные — те же, что в «Спринтах» (дэшборды) и
// «Контроле спринта руководителей»; пороги и формулы совпадают с ними, чтобы цифры на экране не расходились с отчётами.
import { type Model, UNASSIGNED, pct } from './sprints'
import { type PeopleLogic, absTxt } from './sprintsPeople'

export const TV = {
  longWork: 5, // «Долго в работе»: от 5 дн. с момента, когда задачу взяли в работу
  longTest: 3, // «Долго на тесте»: от 3 дн.
  lag: 25, // проект отстаёт, если готовность меньше прошедшего времени спринта больше чем на 25 п.п. (как в дэшбордах)
  testWarn: 8, // задач в Test у проекта: от 8 — внимание, от 15 — тревога
  testHigh: 15,
  absentDays: 14, // отсутствия на ближайшие 14 дней (как в дэшбордах)
  // «Низкий % закрытия» спринта: учитываем спринты, где прошло не меньше 20% времени (в начале спринта 0% — норма);
  // тревога — готово меньше 30% или отставание от времени больше 25 п.п. (TV.lag), внимание — меньше 50% или отставание больше 10 п.п.
  minTime: 20, doneBad: 30, doneWarn: 50, lagWarn: 10,
}

// Строки «Контроля спринта руководителей» (GET /lead-control?lead=): только нужные экрану поля.
export type LcInWork = { key: string; project: string; status: string; assignee: string; days_in_status: number; days_since_start?: number | null; summary: string }
// Сколько дней задача в работе: от момента, когда её взяли («В работе»); нет истории — от последней смены статуса.
export const workDays = (x: Pick<LcInWork, 'days_in_status' | 'days_since_start'>) => x.days_since_start ?? x.days_in_status
export type LcTesting = { key: string; project: string; assignee: string; tester: string; days_in_test: number; summary: string }
export type LcBlocked = { key: string; project: string; assignee: string; days_blocked: number; block_state: string; summary: string }
export type LcReport = { in_work?: LcInWork[] | null; testing?: LcTesting[] | null; blocked?: LcBlocked[] | null }

// Один проект может быть у нескольких руководителей (например, назначен ПМу) — задача не должна считаться дважды.
export function dedupe<T extends { key: string }>(rows: T[]): T[] {
  const seen = new Set<string>()
  return rows.filter(r => !seen.has(r.key) && !!seen.add(r.key))
}

export type Who = { name: string; count: number; max: number; keys: string[] }
// Группировка по человеку: сколько «долгих» задач и самая долгая (дни). Пустое имя — noName.
export function groupLong<T extends { key: string }>(rows: T[], who: (r: T) => string, days: (r: T) => number, noName: string): Who[] {
  const M = new Map<string, Who>()
  for (const r of rows) {
    const n = who(r).trim() || noName
    const x = M.get(n) ?? { name: n, count: 0, max: 0, keys: [] }
    x.count++; x.max = Math.max(x.max, days(r)); x.keys.push(r.key)
    M.set(n, x)
  }
  return [...M.values()].sort((a, b) => b.count - a.count || b.max - a.max || a.name.localeCompare(b.name, 'ru'))
}

export type Reason = { text: string; sev: 1 | 2 | 3 }
export type Risk = { k: string; name: string; lead: string; done: number; time: number; score: number; reasons: Reason[] }
// Риски проектов — те же правила, что в дэшбордах «Риски по спринтам»: не закрыт, отстаёт, Blocked, нет следующего спринта.
export function projectRisks(m: Pick<Model, 'PR' | 'NSD'>): Risk[] {
  const out: Risk[] = []
  for (const p of m.PR) {
    const reasons: Reason[] = []
    if (p.left < 0) reasons.push({ text: `Не закрыт ${-p.left} дн.`, sev: 3 })
    if (p.time - p.done > TV.lag) reasons.push({ text: `Отстаёт: ${p.done}% из ${p.time}%`, sev: 2 })
    if (p.blk > 0) reasons.push({ text: `Blocked ${p.blk}`, sev: p.blk >= 3 ? 2 : 1 })
    const ns = m.NSD[p.k]
    if ((!ns || !ns.t) && p.left <= 3) reasons.push({ text: 'Нет след. спринта', sev: 1 })
    if (reasons.length) out.push({ k: p.k, name: p.n, lead: p.lead, done: p.done, time: p.time, reasons, score: reasons.reduce((a, r) => a + r.sev, 0) })
  }
  return out.sort((a, b) => b.score - a.score || b.reasons.length - a.reasons.length || a.k.localeCompare(b.k))
}

export type Level = 'bad' | 'warn' | ''
export const testLevel = (n: number): Level => n >= TV.testHigh ? 'bad' : n >= TV.testWarn ? 'warn' : ''
export const daysLevel = (d: number, warn: number, bad: number): Level => d >= bad ? 'bad' : d >= warn ? 'warn' : ''

export type LowSprint = { k: string; name: string; lead: string; sprint: string; done: number; time: number; left: number; level: Level }
// Спринты с низким процентом закрытия (доля задач в «Готово»): самые низкие сверху; здоровые и только начавшиеся не показываем.
export function lowSprints(m: Pick<Model, 'PR'>): LowSprint[] {
  const out: LowSprint[] = []
  for (const p of m.PR) {
    if (!p.t || p.time < TV.minTime) continue
    const lag = p.time - p.done
    const level: Level = p.done < TV.doneBad || lag > TV.lag ? 'bad' : p.done < TV.doneWarn || lag > TV.lagWarn ? 'warn' : ''
    if (level) out.push({ k: p.k, name: p.n, lead: p.lead, sprint: p.sp, done: p.done, time: p.time, left: p.left, level })
  }
  return out.sort((a, b) => a.done - b.done || b.time - a.time || a.k.localeCompare(b.k))
}

export function buildTv(m: Model, pl: PeopleLogic, lc: LcReport[]) {
  const inWork = dedupe(lc.flatMap(r => r.in_work ?? [])).filter(x => workDays(x) >= TV.longWork).sort((a, b) => workDays(b) - workDays(a))
  const testing = dedupe(lc.flatMap(r => r.testing ?? [])).filter(x => x.days_in_test >= TV.longTest).sort((a, b) => b.days_in_test - a.days_in_test)
  const blocked = dedupe(lc.flatMap(r => r.blocked ?? [])).sort((a, b) => b.days_blocked - a.days_blocked)

  const absent = pl.absRows(TV.absentDays, false)
    .sort((a, b) => (+b.now - +a.now) || (a.fr < b.fr ? -1 : 1))
    .map(a => ({ n: a.n, pos: a.pos, text: absTxt(a, m.SNAP), now: a.now, q: a.q, endRisk: a.endRisk }))

  const need = Object.keys(m.PEOPLE).filter(pl.needVisible).filter(n => !m.PR.some(p => p.lead === n))
    .map(n => ({ n, pos: m.roleOf(n), q: pl.needInfo(n) }))
    .sort((a, b) => a.q - b.q || a.n.localeCompare(b.n, 'ru'))

  const tests = m.PR.filter(p => p.tst > 0).sort((a, b) => b.tst - a.tst || a.k.localeCompare(b.k))
    .map(p => ({ k: p.k, name: p.n, lead: p.lead, tst: p.tst, level: testLevel(p.tst) }))

  const total = m.PR.reduce((a, p) => a + p.t, 0), done = m.PR.reduce((a, p) => a + p.dn, 0)
  const risks = projectRisks(m)
  const kpi = {
    tasks: total, projects: m.PR.length, done: pct(done, total),
    stuck: m.PR.filter(p => p.left < 0).length,
    absentNow: absent.filter(a => a.now).length,
    need: need.length,
    blocked: m.PR.reduce((a, p) => a + p.blk, 0),
    longWork: inWork.length, longTest: testing.length,
  }
  return {
    kpi, absent, need, tests, risks, blocked, lowSprints: lowSprints(m),
    slowTests: testing, slowTesters: groupLong(testing, x => x.tester, x => x.days_in_test, 'без тестировщика'),
    slowWork: inWork, slowDevs: groupLong(inWork, x => x.assignee, workDays, UNASSIGNED),
  }
}
export type Tv = ReturnType<typeof buildTv>
