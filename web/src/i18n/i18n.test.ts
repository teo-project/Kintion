import { describe, expect, it } from 'vitest'

import en from '@i18n/en.json'
import ru from '@i18n/ru.json'

import { messagesFor } from '.'

function keys(v: unknown, prefix = ''): string[] {
  if (v === null || typeof v !== 'object') return [prefix]
  return Object.entries(v).flatMap(([k, sub]) => keys(sub, `${prefix}.${k}`))
}

describe('i18n', () => {
  it('has the same keys in every language', () => {
    expect(keys(en).sort()).toEqual(keys(ru).sort())
  })

  it('falls back to ru for unknown language', () => {
    expect(messagesFor('xx')).toBe(ru)
    expect(messagesFor('en')).toBe(en)
  })
})
