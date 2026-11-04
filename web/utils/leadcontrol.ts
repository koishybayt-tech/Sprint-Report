// «Контроль спринта руководителей» — подсветка и ссылки как в design/prototypes/lead-control.html.
import { esc } from './sprints'

export type LCTab = 'blocked' | 'work' | 'people' | 'gaps' | 'test'
export const TABS: LCTab[] = ['blocked', 'work', 'people', 'gaps', 'test']

// Порядок приоритетов разрывов для сортировки (PRI в макете).
export const PRI: Record<string, number> = { 'Будущий спринт': 0, 'Высокий': 1, 'Проверить': 2, 'Низкий': 3 }

// Подсветка строк (HOT в макете): hot — красным, warm — оранжевым.
export const HOT = {
  blocked: (r: { not_started: string }) => r.not_started === 'да' ? 'hot' : '',
  work: (r: { days_in_status: number }) => r.days_in_status >= 10 ? 'hot' : r.days_in_status >= 5 ? 'warm' : '',
  test: (r: { days_in_test: number }) => r.days_in_test >= 7 ? 'hot' : r.days_in_test >= 3 ? 'warm' : '',
  people: (r: { signal: string }) => r.signal === 'Тормозит' ? 'hot' : r.signal ? 'warm' : '',
  gaps: (r: { priority: string }) => r.priority === 'Высокий' || r.priority === 'Будущий спринт' ? 'hot' : r.priority === 'Проверить' ? 'warm' : '',
}

// Ключи задач Jira в любом тексте — ссылки на jira.adata.kz/browse/<KEY>; остальное экранируется.
const reKey = /\b[A-Z][A-Z0-9]+-\d+\b/g
export function linkKeys(text: unknown, jiraBase: string): string {
  const s = text == null || text === '' ? '' : String(text)
  if (!s) return '—'
  const base = jiraBase.replace(/\/$/, '') + '/browse/'
  let out = '', last = 0
  for (const m of s.matchAll(reKey)) {
    out += esc(s.slice(last, m.index)) + `<a href="${base}${m[0]}" target="_blank" rel="noopener">${m[0]}</a>`
    last = m.index! + m[0].length
  }
  return out + esc(s.slice(last))
}

// Фильтры по человеку: «Исполнитель» (на вкладке «Сотрудники» — «Сотрудник») и «Тестировщик» (вкладка «Долго на тесте»).
export const NONE = '—' // значение фильтра для задач без исполнителя / тестировщика
export const personOf = (tab: LCTab, r: any): string => String((tab === 'people' ? r.person : r.assignee) ?? '').trim()
export const testerOf = (r: any): string => String(r.tester ?? '').trim()
export const matchWho = (value: string, want: string) => !want || (want === NONE ? value === '' : value === want)

export type Opt = { value: string; label: string; n: number }
// Варианты выпадающего списка со счётчиками: по алфавиту, «не назначен» — последним.
export function countBy(rows: any[], get: (r: any) => string, noneLabel: string): Opt[] {
  const m = new Map<string, number>()
  for (const r of rows) { const v = get(r) || NONE; m.set(v, (m.get(v) ?? 0) + 1) }
  return [...m.entries()].map(([value, n]) => ({ value, n, label: value === NONE ? noneLabel : value }))
    .sort((a, b) => (+(a.value === NONE) - +(b.value === NONE)) || a.label.localeCompare(b.label, 'ru'))
}

// Значение для сортировки: пустые — в конец при любом направлении (как в макете).
// Строки в порядке таблицы: сортировка по клику на заголовок, пустые значения всегда в конце.
export function sortRows<T extends Record<string, any>>(rows: T[], [f, d]: [string, number]): T[] {
  return [...rows].sort((a, b) => {
    const x = sortVal(a[f], f), y = sortVal(b[f], f)
    if (x === y) return 0
    if (x === null) return 1
    if (y === null) return -1
    return (typeof x === 'number' && typeof y === 'number' ? x - y : String(x).localeCompare(String(y), 'ru')) * d
  })
}

export function sortVal(v: unknown, field: string): number | string | null {
  if (v === null || v === undefined || v === '') return null
  if (field === 'priority') return PRI[String(v)] ?? 9
  return typeof v === 'number' ? v : String(v)
}
