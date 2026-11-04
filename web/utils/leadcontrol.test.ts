import { describe, expect, it } from 'vitest'
import { HOT, NONE, countBy, linkKeys, matchWho, personOf, sortRows, sortVal, testerOf } from './leadcontrol'

describe('linkKeys', () => {
  const J = 'https://jira.adata.kz/'
  it('ключи задач в тексте — ссылки на Jira, остальное экранируется', () => {
    expect(linkKeys('AC-11 Бэк — Test с 31.10; <b>', J)).toBe(
      '<a href="https://jira.adata.kz/browse/AC-11" target="_blank" rel="noopener">AC-11</a> Бэк — Test с 31.10; &lt;b&gt;')
  })
  it('несколько ключей и проекты с цифрами', () => {
    expect(linkKeys('взял ADTC-5, завершил A1B-12', J)).toContain('browse/A1B-12')
    expect((linkKeys('взял ADTC-5, завершил A1B-12', J).match(/<a /g) || []).length).toBe(2)
  })
  it('пусто — прочерк', () => {
    expect(linkKeys('', J)).toBe('—')
    expect(linkKeys(null, J)).toBe('—')
  })
})

describe('sortVal и подсветка', () => {
  it('приоритеты разрывов сортируются по важности, пустые — null', () => {
    expect(sortVal('Будущий спринт', 'priority')).toBeLessThan(sortVal('Низкий', 'priority') as number)
    expect(sortVal(null, 'days_in_status')).toBeNull()
    expect(sortVal('', 'tester')).toBeNull()
  })
  it('пороги подсветки как в макете', () => {
    expect([4, 5, 10].map(d => HOT.work({ days_in_status: d }))).toEqual(['', 'warm', 'hot'])
    expect([2, 3, 7].map(d => HOT.test({ days_in_test: d }))).toEqual(['', 'warm', 'hot'])
    expect(HOT.gaps({ priority: 'Будущий спринт' })).toBe('hot')
    expect(HOT.blocked({ not_started: 'да' })).toBe('hot')
  })
})

describe('фильтры по человеку', () => {
  const rows = [
    { key: 'A-1', assignee: 'Петров', tester: 'Тестов', d: 5 }, { key: 'A-2', assignee: 'Иванов', tester: '', d: 9 },
    { key: 'A-3', assignee: '', tester: 'Тестов', d: 1 }, { key: 'A-4', assignee: 'Петров', tester: ' ', d: 7 },
  ]
  it('исполнитель берётся из assignee, на вкладке «Сотрудники» — из person', () => {
    expect(personOf('test', rows[0])).toBe('Петров')
    expect(personOf('people', { person: 'Сидоров', assignee: 'не он' })).toBe('Сидоров')
    expect(testerOf(rows[3])).toBe('')
  })
  it('matchWho: пусто — все, NONE — без значения, иначе точное совпадение', () => {
    expect(matchWho('Петров', '')).toBe(true)
    expect(matchWho('', NONE)).toBe(true)
    expect(matchWho('Петров', NONE)).toBe(false)
    expect(matchWho('Петров', 'Петров')).toBe(true)
    expect(matchWho('Иванов', 'Петров')).toBe(false)
  })
  it('countBy: счётчики, по алфавиту, «не назначен» последним', () => {
    const o = countBy(rows, r => personOf('test', r), 'не назначен')
    expect(o.map(x => [x.label, x.n])).toEqual([['Иванов', 1], ['Петров', 2], ['не назначен', 1]])
    expect(o[2].value).toBe(NONE)
  })
})

describe('sortRows', () => {
  const rows = [{ k: 'a', d: 2 }, { k: 'b', d: null }, { k: 'c', d: 9 }]
  it('по убыванию, пустые всегда в конце', () => {
    expect(sortRows(rows, ['d', -1]).map(r => r.k)).toEqual(['c', 'a', 'b'])
    expect(sortRows(rows, ['d', 1]).map(r => r.k)).toEqual(['a', 'c', 'b'])
  })
  it('исходный массив не меняется', () => {
    sortRows(rows, ['d', -1])
    expect(rows.map(r => r.k)).toEqual(['a', 'b', 'c'])
  })
})
