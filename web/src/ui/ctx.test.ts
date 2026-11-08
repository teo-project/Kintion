import { describe, expect, it } from 'vitest'

import { ruSchema } from '@/core/__tests__/kit'
import { buildRegistry } from '@/core/registry'

import { makeCtx } from './ctx'

describe('контекст экранов (F-143)', () => {
  it('эры дашборда: порядок, показ и год мира вместо настоящей даты', () => {
    const reg = buildRegistry({ ...ruSchema(), eras: { show: true, edit: true, default: 'AC', order: ['AC', ''], years: { AC: { min: 1, max: 400 }, BC: { min: 15000, max: 1 } }, today: { y: 300, e: 'AC' }, list: ['BC', 'AC', ''] } })
    const ctx = makeCtx(reg, [])
    // порядок для сравнения — заданный (эры вне него — сверху по тексту, как на сервере)
    // эры с обратным счётом — минимальный год больше максимального
    expect(ctx.eras).toEqual({ order: ['AC', ''], reversed: ['BC'] })
    expect(ctx.today).toEqual({ y: 300, e: 'AC' })
    expect(ctx.dateView).toEqual({ showEra: true, noEra: true })
  })

  it('год мира с пустой эрой — как есть (эру по умолчанию новому ставит сервер; пустая — как у дат)', () => {
    const reg = buildRegistry({ ...ruSchema(), eras: { show: false, edit: false, default: 'AC', order: [], today: { y: 300 }, list: ['AC'] } })
    expect(makeCtx(reg, []).today).toEqual({ y: 300 })
  })

  it('без настроек — настоящая дата, порядок по умолчанию', () => {
    const ctx = makeCtx(buildRegistry(ruSchema()), [])
    expect(ctx.eras).toEqual({ order: [], reversed: [] })
    expect(ctx.today.y).toBe(new Date().getFullYear())
    expect(ctx.dateView).toEqual({ showEra: false, noEra: false })
  })
})
