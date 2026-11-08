import { describe, expect, it } from 'vitest'

import cases from '@testdata/partial_dates.json'
import en from '@i18n/en.json'
import ru from '@i18n/ru.json'

import {
  ageAt,
  hiKey,
  loKey,
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
    expect(ageAt(c.date as PartialDate, c.at, 'reversed' in c && c.reversed === true)).toBe(c.age)
  })

  it.each(cases.anniversary)('anniversary $date from $from', (c) => {
    expect(nextAnniversary(c.date as PartialDate, c.from)).toBe(c.next)
  })

  it('sorts by Д-9', () => {
    const want = cases.sorted as PartialDate[]
    expect([...want].reverse().sort((a, b) => compareDates(a, b))).toEqual(want)
  })

  it('sorts by era order (F-143)', () => {
    const { eras, reversed, dates } = cases.sorted_eras
    const want = dates as PartialDate[]
    const order = { order: eras, reversed }
    expect([...want].reverse().sort((a, b) => compareDates(a, b, order))).toEqual(want)
  })

  it('era-only date spans its era in lo/hi keys', () => {
    const order = { order: ['BC', ''], reversed: ['BC'] }
    const era = { e: 'BC' }
    for (const d of [{ y: 15000, e: 'BC' }, { y: 1, m: 12, d: 31, e: 'BC' }]) {
      expect(loKey(era, order)! < loKey(d, order)!).toBe(true)
      expect(hiKey(era, order)! > hiKey(d, order)!).toBe(true)
    }
    // в эре с обратным счётом 500 раньше 5
    expect(hiKey({ y: 500, e: 'BC' }, order)! < loKey({ y: 5, e: 'BC' }, order)!).toBe(true)
    expect(hiKey(era, order)! < loKey({ y: 1 }, order)!).toBe(true)
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

  it('shows signed years and eras (F-14)', () => {
    expect(formatDate({ y: -500 }, ru.dates)).toBe('500 до н. э.')
    expect(formatDate({ y: -44, m: 3, d: 15 }, ru.dates)).toBe('15 марта 44 до н. э.')
    expect(formatDate({ y: -500 }, en.dates)).toBe('500 BC')
    expect(formatDate({ y: 300, e: 'AC' }, ru.dates, { showEra: true })).toBe('300 AC')
    expect(formatDate({ y: -500, e: 'AC' }, ru.dates, { showEra: true })).toBe('500 до AC')
    expect(formatDate({ y: -500, e: 'AC' }, en.dates, { showEra: true })).toBe('−500 AC')
    expect(formatDate({ y: -500, m: 3, e: 'AC' }, ru.dates, { showEra: true })).toBe('март 500 до AC')
    // эра скрыта настройкой — без пометки, отрицательный — «до н. э.»
    expect(formatDate({ y: -500, e: 'AC' }, ru.dates)).toBe('500 до н. э.')
    expect(formatDate({ y: -500, e: 'AC' }, en.dates)).toBe('500 BC')
    expect(formatDate({ y: 300, e: 'AC' }, ru.dates)).toBe('300')
  })

  it('shows our era only when eras are shown (F-14)', () => {
    expect(formatDate({ y: 1985 }, ru.dates, { showEra: true })).toBe('1985 н. э.')
    expect(formatDate({ y: -500 }, ru.dates, { showEra: true })).toBe('500 до н. э.')
    expect(formatDate({ y: 1985 }, en.dates, { showEra: true })).toBe('1985 AD')
    expect(formatDate({ y: -500 }, en.dates, { showEra: true })).toBe('500 BC')
    expect(formatDate({ y: 1985 }, en.dates)).toBe('1985')
  })

  it('shows a date without era without an era label when the default era is set (F-14)', () => {
    expect(formatDate({ y: 1985, m: 3 }, ru.dates, { showEra: true, noEra: true })).toBe('март 1985')
    expect(formatDate({ y: -500 }, ru.dates, { showEra: true, noEra: true })).toBe('−500')
    expect(formatDate({ y: 300, e: 'AC' }, ru.dates, { showEra: true, noEra: true })).toBe('300 AC')
    expect(formatDate({ y: -500 }, ru.dates, { noEra: true })).toBe('500 до н. э.')
  })

  it('shows an era-only date as the era (F-14)', () => {
    expect(formatDate({ e: 'мезозойская эра' }, ru.dates)).toBe('мезозойская эра')
    expect(formatDate({ e: 'мезозойская эра' }, ru.dates, { showEra: true })).toBe('мезозойская эра')
  })
})

describe('era keys', () => {
  it('compares moments across eras by era order', () => {
    const order = ['AC', '']
    expect(hiKey({ y: 300, e: 'AC' }, order)! < loKey({ y: -1000 }, order)!).toBe(true)
    expect(hiKey({ y: 300, e: 'AC' })! > loKey({ y: 3000 })!).toBe(true) // по умолчанию наша эра первой
    expect(loKey({ m: 3 })).toBeNull()
  })
})
