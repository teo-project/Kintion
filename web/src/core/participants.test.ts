import { describe, expect, it } from 'vitest'

import { altSchema, ctxOf, ruSchema } from './__tests__/kit'
import { anchorEntities, canJoin, endEvents, limits, refAllowed } from './participants'
import { tplOf } from './resolver'

describe('участники: пределы, вложенность, события окончания', () => {
  const ctx = ctxOf(ruSchema(), [
    { id: 'anna', template: 'kin.core.person' },
    { id: 'org', template: 'kin.core.organization' },
    { id: 'reg', template: 'kin.core.registration' },
    { id: 'birth', template: 'kin.core.birth' },
    { id: 'm', template: 'kin.core.relationship', fields: { stage: ['marriage'] } },
    { id: 'm0', template: 'kin.core.relationship' },
  ])
  const tpl = (id: string) => ctx.reg.templates.get(id)!.definition
  const wedding = tpl('kin.core.wedding')

  it('min / max ролей (Д-22): без двух партнёров создать нельзя', () => {
    const none = limits(wedding, [])
    expect(none.complete).toBe(false)
    expect(none.roles.find(r => r.role.key === 'partner')!.missing).toBe(2)
    const one = limits(wedding, ['partner'])
    expect(one.complete).toBe(false)
    const two = limits(wedding, ['partner', 'partner'])
    expect(two.complete).toBe(true)
    const partner = two.roles.find(r => r.role.key === 'partner')!
    expect(partner.canAdd).toBe(false) // max 2
    expect(partner.canRemove).toBe(false) // min 2
    expect(two.roles.find(r => r.role.key === 'guest')!.canAdd).toBe(true) // max null
  })

  it('роль: сущности из entities; контейнер не участник; поле-ссылка — по contains (F-82)', () => {
    const partner = wedding.roles!.find(r => r.key === 'partner')!
    const r = ctx.records
    expect(canJoin(ctx, wedding, partner, r.anna!)).toBe(true)
    expect(canJoin(ctx, wedding, partner, r.org!)).toBe(false)
    expect(canJoin(ctx, wedding, partner, r.anna!, 'anna')).toBe(false)
    expect(canJoin(ctx, wedding, { key: 'x', forms: { default: 'x' } }, r.reg!)).toBe(false) // вложенность — не участием
    expect(refAllowed(ctx.reg, wedding, undefined, tplOf(ctx, r.reg!))).toBe(true)
    expect(refAllowed(ctx.reg, wedding, undefined, tplOf(ctx, r.birth!))).toBe(false)
    expect(refAllowed(ctx.reg, wedding, undefined, tplOf(ctx, r.anna!))).toBe(true) // не контейнер — без contains
    expect(refAllowed(ctx.reg, wedding, ['kin.core.organization'], tplOf(ctx, r.anna!))).toBe(false)
    expect(refAllowed(ctx.reg, tpl('kin.core.birth'), undefined, tplOf(ctx, r.birth!))).toBe(true) // нет contains — любые
  })

  it('привязки — к сущностям с anchor (F-83)', () => {
    expect(anchorEntities(ctx.reg)).toEqual(['kin.core.event'])
  })

  it('end_events: этапа, иначе шаблона; нет — без ограничений', () => {
    expect(endEvents(ctx, ctx.records.m!)).toEqual(['kin.core.divorce', 'kin.core.death'])
    expect(endEvents(ctx, ctx.records.m0!)).toBeUndefined()
  })

  it('синтетический пак: пределы из его шаблонов', () => {
    const alt = ctxOf(altSchema(), [])
    for (const { definition } of alt.reg.templates.values()) {
      const l = limits(definition, [])
      const need = (definition.roles ?? []).some(r => (r.min ?? 0) > 0) || (definition.participants?.min ?? 0) > 0
      expect(l.complete).toBe(!need)
    }
  })
})
