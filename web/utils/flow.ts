// «Динамика созданных задач в спринте»: ряд из /sprints/report (created_daily) → дни для графика и итоги.
// Ряд приходит с сервера: от даты from по календарным дням (Алматы), total — создано, unplanned — из них внеплановых.

export type CreatedDaily = { from: string; total: number[]; unplanned: number[] }
export type FlowDay = { date: string; n: number; u: number; weekend: boolean }
export type Flow = {
  days: FlowDay[]
  total: number // создано всего
  unplanned: number // из них внеплановых
  peak: FlowDay | null // самый «шумный» день (при равенстве — более поздний: свежий поток важнее)
  max: number // высота самой высокой колонки — для масштаба графика
}

// Дата + k дней без часовых поясов (YYYY-MM-DD, берём полдень UTC).
const addDays = (d: string, k: number) => {
  const x = new Date(`${d}T12:00:00Z`)
  x.setUTCDate(x.getUTCDate() + k)
  return x
}

export function flowOf(cd: CreatedDaily | null | undefined): Flow | null {
  if (!cd || !cd.from || !cd.total?.length) return null
  const days: FlowDay[] = cd.total.map((n, i) => {
    const x = addDays(cd.from, i)
    const wd = x.getUTCDay()
    return { date: x.toISOString().slice(0, 10), n, u: Math.min(n, cd.unplanned?.[i] ?? 0), weekend: wd === 0 || wd === 6 }
  })
  const total = days.reduce((a, d) => a + d.n, 0)
  const unplanned = days.reduce((a, d) => a + d.u, 0)
  let peak: FlowDay | null = null
  for (const d of days) if (d.n > 0 && (!peak || d.n >= peak.n)) peak = d
  return { days, total, unplanned, peak, max: Math.max(0, ...days.map(d => d.n)) }
}

// Высота части колонки в процентах от самой высокой (для стека «плановые + внеплановые»).
export const barPct = (v: number, max: number) => (max > 0 ? Math.round((v / max) * 1000) / 10 : 0)
