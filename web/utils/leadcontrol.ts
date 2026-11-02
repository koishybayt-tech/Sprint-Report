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

// Значение для сортировки: пустые — в конец при любом направлении (как в макете).
export function sortVal(v: unknown, field: string): number | string | null {
  if (v === null || v === undefined || v === '') return null
  if (field === 'priority') return PRI[String(v)] ?? 9
  return typeof v === 'number' ? v : String(v)
}
