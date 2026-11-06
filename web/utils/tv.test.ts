import { describe, expect, it } from 'vitest'
import { TV, dedupe, daysLevel, groupLong, lowSprints, projectRisks, testLevel, workDays } from './tv'

describe('dedupe', () => {
  it('одна задача у нескольких руководителей считается один раз', () => {
    expect(dedupe([{ key: 'SUP-1' }, { key: 'SUP-2' }, { key: 'SUP-1' }]).map(x => x.key)).toEqual(['SUP-1', 'SUP-2'])
  })
})

describe('groupLong', () => {
  const rows = [
    { key: 'A-1', who: 'Иван', d: 9 }, { key: 'A-2', who: 'Иван', d: 5 }, { key: 'B-1', who: 'Пётр', d: 12 },
    { key: 'B-2', who: '', d: 6 }, { key: 'B-3', who: '  ', d: 7 },
  ]
  const g = groupLong(rows, r => r.who, r => r.d, 'не назначен')
  it('считает задачи и максимум дней, сортирует по количеству', () => {
    expect(g.map(x => [x.name, x.count, x.max])).toEqual([['Иван', 2, 9], ['не назначен', 2, 7], ['Пётр', 1, 12]])
  })
  it('собирает ключи задач', () => { expect(g[0].keys).toEqual(['A-1', 'A-2']) })
})

describe('lowSprints', () => {
  const P = (o: object) => ({ k: 'X', n: 'Проект', lead: 'Лид', sp: 'X 1', t: 10, done: 60, time: 60, left: 5, ...o }) as any
  const run = (ps: any[]) => lowSprints({ PR: ps } as any)
  it('здоровые и только начавшиеся спринты не показываем', () => {
    expect(run([P({ k: 'OK', done: 70, time: 60 }), P({ k: 'NEW', done: 0, time: 10 }), P({ k: 'EMPTY', t: 0, done: 0, time: 90 })])).toEqual([])
  })
  it('уровни: тревога — готово < 30% или отставание > 25 п.п., внимание — < 50% или отставание > 10 п.п.', () => {
    const r = run([P({ k: 'BADLOW', done: 20, time: 40 }), P({ k: 'BADLAG', done: 60, time: 90 }), P({ k: 'WARNLOW', done: 45, time: 50 }), P({ k: 'WARNLAG', done: 70, time: 85 })])
    expect(Object.fromEntries(r.map(x => [x.k, x.level]))).toEqual({ BADLOW: 'bad', BADLAG: 'bad', WARNLOW: 'warn', WARNLAG: 'warn' })
  })
  it('самые низкие сверху; при равенстве — у кого прошло больше времени', () => {
    const r = run([P({ k: 'B', done: 40, time: 60 }), P({ k: 'A', done: 10, time: 50 }), P({ k: 'C', done: 40, time: 90 })])
    expect(r.map(x => x.k)).toEqual(['A', 'C', 'B'])
  })
  it('граница минимального времени', () => {
    expect(run([P({ k: 'Z', done: 5, time: TV.minTime - 1 })])).toEqual([])
    expect(run([P({ k: 'Z', done: 5, time: TV.minTime })]).length).toBe(1)
  })
})

describe('workDays', () => {
  it('считаем от момента взятия в работу, а без истории — от последней смены статуса', () => {
    expect(workDays({ days_in_status: 28, days_since_start: 5 })).toBe(5)
    expect(workDays({ days_in_status: 28, days_since_start: null })).toBe(28)
    expect(workDays({ days_in_status: 7 })).toBe(7)
    expect(workDays({ days_in_status: 3, days_since_start: 0 })).toBe(0)
  })
})

describe('уровни', () => {
  it('Test по проекту: порог внимания и тревоги', () => {
    expect(testLevel(TV.testWarn - 1)).toBe('')
    expect(testLevel(TV.testWarn)).toBe('warn')
    expect(testLevel(TV.testHigh)).toBe('bad')
  })
  it('дни: warn и bad от порогов', () => {
    expect(daysLevel(4, 5, 10)).toBe('')
    expect(daysLevel(5, 5, 10)).toBe('warn')
    expect(daysLevel(10, 5, 10)).toBe('bad')
  })
})

describe('projectRisks', () => {
  const P = (o: object) => ({ k: 'X', n: 'Проект', lead: 'Лид', t: 10, dn: 5, blk: 0, left: 5, done: 50, time: 50, ...o }) as any
  const m = (ps: any[], nsd: any = {}) => ({ PR: ps, NSD: nsd }) as any
  it('здоровый проект не попадает в риски', () => {
    expect(projectRisks(m([P({ k: 'OK' })], { OK: { t: 3 } }))).toEqual([])
  })
  it('не закрытый спринт, отставание и Blocked складываются и сортируются по тяжести', () => {
    const r = projectRisks(m([
      P({ k: 'LOW', blk: 1 }),
      P({ k: 'BAD', left: -2, done: 20, time: 90, blk: 3 }),
    ], { LOW: { t: 1 }, BAD: { t: 1 } }))
    expect(r.map(x => x.k)).toEqual(['BAD', 'LOW'])
    expect(r[0].reasons.map(x => x.text)).toEqual(['Не закрыт 2 дн.', 'Отстаёт: 20% из 90%', 'Blocked 3'])
  })
  it('нет следующего спринта — риск только в последние 3 дня', () => {
    expect(projectRisks(m([P({ k: 'Z', left: 10 })]))).toEqual([])
    expect(projectRisks(m([P({ k: 'Z', left: 3 })]))[0].reasons[0].text).toBe('Нет след. спринта')
  })
})
