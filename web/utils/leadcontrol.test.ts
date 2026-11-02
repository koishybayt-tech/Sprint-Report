import { describe, expect, it } from 'vitest'
import { HOT, linkKeys, sortVal } from './leadcontrol'

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
