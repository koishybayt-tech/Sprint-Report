// Карточка сотрудника — расчёты из reference/employee/page_template.html (S, rmed, spark, bars), перенесены 1-в-1.
import { esc } from '~/utils/sprints'

export type EmpRow = {
  id: number; n: string; pos: string; tl: boolean; lead: boolean; login: string
  proj: [string, number][] | null; open: [number, number]; m: Record<string, (number | null)[]>; cyc: number | null
}
export type EMP = { stamp: string; mon: string[]; att_from: string; ppl: EmpRow[]; history: string | null; tasks: boolean }
export type Task = [string, string, string, string, string, string, number, number, number | null, number, string]
export type Alog = [string, string, number, string]
export type Card = { t: Task[]; alog: Alog[] }

export const IX = { done: 0, bug: 1, h: 2, carry: 3, plan: 4, planDone: 5, rw: 6, cyc: 7, rep: 8, qa: 9, late: 10, early: 11, mid: 12, day: 13, sick: 14, min: 15, lateStudy: 16 } as const
const MN = ['янв', 'фев', 'мар', 'апр', 'май', 'июн', 'июл', 'авг', 'сен', 'окт', 'ноя', 'дек']
export const mlab = (m: string) => MN[+m.slice(5, 7) - 1]
export const fmtD = (d?: string | null) => d ? d.slice(8, 10) + '.' + d.slice(5, 7) : '—'
export const pct = (a: number, b: number) => b ? Math.round(a * 100 / b) : null
export const med = (a: (number | null | undefined)[]) => { const s = a.filter((x): x is number => x != null && !isNaN(x)).sort((x, y) => x - y); if (!s.length) return null; const h = s.length >> 1; return s.length % 2 ? s[h] : (s[h - 1] + s[h]) / 2 }
export const hm = (m: number) => m >= 60 ? Math.floor(m / 60) + ' ч' + (m % 60 ? ' ' + (m % 60) + ' мин' : '') : m + ' мин'
export const cls = (v: number | null, good: number, bad: number, inv?: boolean) => v == null ? '' : inv ? (v >= bad ? 'hi' : v >= good ? 'mid' : '') : (v <= bad ? 'hi' : v <= good ? 'mid' : 'good')

export type Sum = {
  done: number; plan: number; pd: number; planPct: number | null; rw: number; rwPct: number | null; bug: number; carry: number; carryPct: number | null
  cyc: number | null; late: number; off: number; day: number; sick: number; mins: number; perM: number; r3: number; rPrev: number; l3: number; lPrev: number; nM: number; h: number; open: number
}

// Сводка по человеку (S в шаблоне); медиана цикла по задачам приходит с сервера (cyc).
export function summarize(E: EMP, p: EmpRow): Sum {
  const MONS = E.mon
  const sumM = (f: keyof typeof IX, months = MONS) => months.reduce((a, m) => a + (p.m[m] ? (p.m[m][IX[f]] as number || 0) : 0), 0)
  const last3 = MONS.slice(-4, -1).length ? MONS.slice(-4, -1) : MONS // три полных месяца до текущего
  const prev = MONS.slice(0, Math.max(1, MONS.length - 4)), np = Math.max(1, MONS.length - 4)
  const done = sumM('done'), plan = sumM('plan'), pd = sumM('planDone'), rw = sumM('rw'), bug = sumM('bug'), carry = sumM('carry')
  const cyc = p.cyc
  const late = sumM('late'), off = sumM('early') + sumM('mid'), day = sumM('day'), sick = sumM('sick'), mins = sumM('min')
  const actM = MONS.filter(m => p.m[m] && (p.m[m][0] || p.m[m][4] || p.m[m][10] || p.m[m][13]))
  const firstM = actM[0] || null, nM = firstM ? MONS.length - MONS.indexOf(firstM) : 0
  return {
    done, plan, pd, planPct: pct(pd, plan), rw, rwPct: pct(rw, done), bug, carry, carryPct: pct(carry, done), cyc, late, off, day, sick, mins,
    perM: nM ? done / nM : 0, r3: sumM('done', last3) / last3.length, rPrev: sumM('done', prev) / np,
    l3: sumM('late', last3) / last3.length, lPrev: sumM('late', prev) / np, nM, h: Math.round(sumM('h')), open: p.open[0] + p.open[1],
  }
}
export const active = (s: Sum) => !!(s.done || s.plan || s.late || s.off || s.day || s.sick || s.open)

// Медиана роли (rmed): для «задачных» показателей — только люди с закрытыми задачами.
export function roleMedian(E: EMP, sums: Map<number, Sum>, p: EmpRow, f: keyof Sum) {
  const r = p.pos || '—'
  return med(E.ppl.filter(x => (x.pos || '—') === r).filter(x => sums.get(x.id)!.done > 0 || f === 'late' || f === 'perM').map(x => sums.get(x.id)![f] as number | null))
}
export const roleSize = (E: EMP, p: EmpRow) => E.ppl.filter(x => (x.pos || '—') === (p.pos || '—')).length

export const spark = (vals: number[], w = 90, h = 22, col = 'var(--c-done)') => {
  const mx = Math.max(1, ...vals), bw = w / vals.length
  return `<svg class="spark" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}" aria-hidden="true">${vals.map((v, i) => `<rect x="${i * bw + 1}" y="${h - (v / mx) * (h - 2)}" width="${bw - 2}" height="${(v / mx) * (h - 2)}" fill="${col}" opacity="${i === vals.length - 1 ? 1 : 0.55}"></rect>`).join('')}</svg>`
}

// Столбики по месяцам (+ линия процента) — bars() шаблона, одна шкала на график.
export function bars(MONS: string[], W: number, series: { l: string; c: string; v: number[] }[], line: { c: string; v: (number | null)[] } | null, label: string) {
  W = Math.max(320, W)
  const H = 196, L = 30, R = line ? 34 : 8, T = 18, B = 24, iw = W - L - R, ih = H - T - B, n = MONS.length, bw = iw / n
  const stack = MONS.map((_, i) => series.reduce((a, s) => a + s.v[i], 0)), mx = Math.max(1, ...stack)
  const step = mx <= 5 ? 1 : mx <= 10 ? 2 : mx <= 25 ? 5 : mx <= 50 ? 10 : mx <= 100 ? 20 : 50, top = Math.ceil(mx / step) * step
  let g = ''
  for (let v = 0; v <= top; v += step) { const y = T + ih - v / top * ih; g += `<line x1="${L}" x2="${W - R}" y1="${y}" y2="${y}" stroke="var(--grid)"/><text x="${L - 6}" y="${y + 4}" text-anchor="end">${v}</text>` }
  let b = ''
  MONS.forEach((m, i) => {
    let acc = 0
    series.forEach(s => { const v = s.v[i]; if (v) { const h = v / top * ih; b += `<rect x="${L + i * bw + bw * 0.18}" y="${T + ih - (acc + v) / top * ih}" width="${bw * 0.64}" height="${h}" fill="${s.c}"><title>${mlab(m)}: ${s.l} ${v}</title></rect>`; acc += v } })
    b += `<text x="${L + i * bw + bw / 2}" y="${H - 6}" text-anchor="middle">${mlab(m)}</text>`
    if (stack[i]) b += `<text x="${L + i * bw + bw / 2}" y="${T + ih - stack[i] / top * ih - 4}" text-anchor="middle" style="fill:var(--fg)">${stack[i]}</text>`
  })
  let l = ''
  if (line) {
    const pts = line.v.map((v, i) => v == null ? null : [L + i * bw + bw / 2, T + ih - v / 100 * ih] as [number, number])
    for (let v = 0; v <= 100; v += 50) { const y = T + ih - v / 100 * ih; l += `<text x="${W - R + 6}" y="${y + 4}">${v}%</text>` }
    let d = ''
    pts.forEach((p, i) => { if (!p) return; d += (d && pts[i - 1] ? 'L' : 'M') + p[0].toFixed(1) + ' ' + p[1].toFixed(1) })
    l += `<path d="${d}" fill="none" stroke="${line.c}" stroke-width="2"/>` + pts.map((p, i) => p ? `<circle cx="${p[0]}" cy="${p[1]}" r="3.5" fill="${line.c}"><title>${mlab(MONS[i])}: ${line.v[i]}%</title></circle>` : '').join('')
  }
  return `<svg width="100%" viewBox="0 0 ${W} ${H}" role="img" aria-label="${esc(label)}">${g}${b}${l}</svg>`
}

export const empMeta = (E: EMP) => {
  const s = E.stamp, y = s.slice(0, 4)
  return `Данные на ${s.slice(8, 10)}.${s.slice(5, 7)}.${y} ${s.slice(11, 16)} · Jira с 01.01.${y}` + (E.att_from ? ` · Adata Group с ${fmtD(E.att_from)}` : '')
}
