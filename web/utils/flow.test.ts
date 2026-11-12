import { describe, expect, it } from 'vitest'
import { barPct, flowOf, SPIKE } from './flow'

describe('всплески', () => {
  it('день с числом задач от порога SPIKE помечается hot', () => {
    const f = flowOf({ from: '2026-10-26', total: [SPIKE - 1, SPIKE, SPIKE + 5], unplanned: [0, 0, 0] })!
    expect(f.days.map(d => d.hot)).toEqual([false, true, true])
  })
})

describe('flowOf', () => {
  // 26.10.2026 — понедельник
  const cd = { from: '2026-10-26', total: [1, 0, 3, 0, 1, 0, 0, 2], unplanned: [0, 0, 2, 0, 1, 0, 0, 0] }
  const f = flowOf(cd)!
  it('даты по дням от начала ряда, выходные отмечены', () => {
    expect(f.days.map(d => d.date)).toEqual(['2026-10-26', '2026-10-27', '2026-10-28', '2026-10-29', '2026-10-30', '2026-10-31', '2026-11-01', '2026-11-02'])
    expect(f.days.map(d => d.weekend)).toEqual([false, false, false, false, false, true, true, false])
  })
  it('итоги, пик и масштаб', () => {
    expect(f.total).toBe(7)
    expect(f.unplanned).toBe(3)
    expect(f.max).toBe(3)
    expect(f.peak?.date).toBe('2026-10-28')
  })
  it('при равных пиках берётся более поздний день', () => {
    expect(flowOf({ from: '2026-10-26', total: [2, 0, 2], unplanned: [0, 0, 0] })!.peak?.date).toBe('2026-10-28')
  })
  it('переход через конец месяца и года', () => {
    expect(flowOf({ from: '2026-12-30', total: [1, 1, 1], unplanned: [0, 0, 0] })!.days.map(d => d.date)).toEqual(['2026-12-30', '2026-12-31', '2027-01-01'])
  })
  it('внеплановых не больше созданных; нет внеплановых в ответе — нули', () => {
    expect(flowOf({ from: '2026-10-26', total: [1], unplanned: [5] })!.days[0].u).toBe(1)
    expect(flowOf({ from: '2026-10-26', total: [2, 1], unplanned: [] as number[] })!.unplanned).toBe(0)
  })
  it('нет ряда — нет графика; ряд без задач — пик пустой', () => {
    expect(flowOf(null)).toBeNull()
    expect(flowOf({ from: '', total: [1], unplanned: [0] })).toBeNull()
    expect(flowOf({ from: '2026-10-26', total: [], unplanned: [] })).toBeNull()
    const z = flowOf({ from: '2026-10-26', total: [0, 0], unplanned: [0, 0] })!
    expect(z.total).toBe(0)
    expect(z.peak).toBeNull()
  })
})

describe('barPct', () => {
  it('доля от максимума с одним знаком', () => {
    expect(barPct(3, 3)).toBe(100)
    expect(barPct(1, 3)).toBe(33.3)
    expect(barPct(5, 0)).toBe(0)
  })
})
