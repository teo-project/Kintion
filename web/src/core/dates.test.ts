import { describe, expect, it } from 'vitest'

import cases from '@testdata/partial_dates.json'
import en from '@i18n/en.json'
import ru from '@i18n/ru.json'

import {
  ageAt,
  compareDates,
  exact,
  formatDate,
  hi,
  lo,
  nextAnniversary,
  sortKey,
  validateDate,
  type PartialDate,
} from './dates'

// Общие кейсы testdata/partial_dates.json — те же, что в Go- и SQL-тестах.
describe('partial date (shared cases)', () => {
  it.each(cases.valid)('valid $date', (c) => {
    const d = c.date as PartialDate
    expect(validateDate(d)).toBeNull()
    expect(lo(d)).toBe(c.lo)
    expect(hi(d)).toBe(c.hi)
    expect(exact(d)).toBe(c.exact)
    expect(sortKey(d)).toBe(c.sort_key)
  })

  it.each(cases.invalid)('invalid $why', (c) => {
    expect(validateDate(c.date as PartialDate)).not.toBeNull()
  })

  it.each(cases.age)('age $date at $at', (c) => {
    expect(ageAt(c.date as PartialDate, c.at)).toBe(c.age)
  })

  it.each(cases.anniversary)('anniversary $date from $from', (c) => {
    expect(nextAnniversary(c.date as PartialDate, c.from)).toBe(c.next)
  })

  it('sorts by Д-9', () => {
    const want = cases.sorted as PartialDate[]
    expect([...want].reverse().sort(compareDates)).toEqual(want)
  })
})

describe('formatDate', () => {
  it('uses words from the language file', () => {
    expect(formatDate({ y: 1985, m: 3, d: 12 }, ru.dates)).toBe('12 марта 1985')
    expect(formatDate({ m: 3, d: 12 }, ru.dates)).toBe('12 марта')
    expect(formatDate({ y: 1985, m: 3 }, ru.dates)).toBe('март 1985')
    expect(formatDate({ d: 12 }, ru.dates)).toBe('12-е число')
    expect(formatDate({ y: 1985 }, ru.dates)).toBe('1985')
    expect(formatDate({ y: 1985, m: 3, d: 12 }, en.dates)).toBe('12 March 1985')
    expect(formatDate(null, ru.dates)).toBe('')
  })
})
