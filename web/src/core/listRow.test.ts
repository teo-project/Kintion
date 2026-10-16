import { describe, expect, it } from 'vitest'

import { ctxOf, ruSchema, type RecSpec } from './__tests__/kit'
import { leadRoles, peopleEntity, relationRow, templateWithTitle } from './listRow'
import { buildRegistry } from './registry'
import type { Ctx } from './resolver'
import type { SchemaDTO, SchemaTemplateDTO, TemplateDef } from './types'

const people: RecSpec[] = [
  { id: 'ivan', template: 'kin.core.person', fields: { name: ['Иван'], surname: ['Краник'], gender: ['male'] } },
  { id: 'anna', template: 'kin.core.person', fields: { name: ['Анна'], surname: ['Василькова'], gender: ['female'] } },
  { id: 'petr', template: 'kin.core.person', fields: { name: ['Пётр'], gender: ['male'] } },
  { id: 'olga', template: 'kin.core.person', fields: { name: ['Ольга'], gender: ['female'] } },
  { id: 'max', template: 'kin.core.person', fields: { name: ['Максим'], gender: ['male'] } },
  { id: 'org', template: 'kin.core.organization', fields: { name: ['ООО Ромашка'] } },
]

describe('relationRow (F-109)', () => {
  it('пара в браке: люди с формами ролей по полу', () => {
    const ctx = ctxOf(ruSchema(), [...people, {
      id: 'm', template: 'kin.core.relationship', fields: { stage: ['marriage'] },
      parts: [['partner', 'ivan'], ['partner', 'anna']],
    }])
    expect(relationRow(ctx, ctx.records.m!)).toEqual({
      leads: [],
      people: [{ title: 'Иван Краник', label: 'муж' }, { title: 'Анна Василькова', label: 'жена' }],
      more: 0,
    })
  })

  it('работа: организация первой, затем первые 3 человека и «ещё N» — по видимым', () => {
    const ctx = ctxOf(ruSchema(), [...people, {
      id: 'w', template: 'kin.core.work',
      parts: [['subordinate', 'anna'], ['colleague', 'ivan'], ['employer', 'org'], ['colleague', 'petr'],
        ['manager', 'olga'], ['colleague', 'max'], ['colleague', 'ghost']],
    }])
    const row = relationRow(ctx, ctx.records.w!)!
    expect(row.leads).toEqual(['ООО Ромашка'])
    // по порядку ролей шаблона: коллеги, руководитель, подчинённые
    expect(row.people.map(p => `${p.title} — ${p.label}`)).toEqual(['Иван Краник — коллега', 'Пётр — коллега', 'Максим — коллега'])
    // ghost нет среди видимых записей — в «ещё» не считается (F-07)
    expect(row.more).toBe(2)
  })

  it('не отношение: событие (и интервальное), человек — null', () => {
    const schema = ruSchema()
    // интервальное событие: шаблон задаёт конец, но сущность — событие
    const birth = schema.templates.find(x => x.key === 'kin.core.birth')!
    birth.definition = { ...birth.definition, period: { start: 'С', end: 'По' } }
    const ctx = ctxOf(schema, [...people, { id: 'b', template: 'kin.core.birth', parts: [['child', 'ivan']] }])
    expect(relationRow(ctx, ctx.records.b!)).toBeNull()
    expect(relationRow(ctx, ctx.records.ivan!)).toBeNull()
  })

  it('отношение без видимых участников — пустая строка, раскладка отношения', () => {
    const ctx = ctxOf(ruSchema(), [{ id: 'e', template: 'kin.core.relationship', parts: [['partner', 'ghost']] }])
    expect(relationRow(ctx, ctx.records.e!)).toEqual({ leads: [], people: [], more: 0 })
  })

  it('люди — сущность, чаще всех допустимая в ролях отношений реестра; события — только при ничьей', () => {
    expect(peopleEntity(ctxOf(ruSchema(), []))).toBe('kin.core.person')
    const f = { default: 'r' }
    const tpl = (id: string, entity: string, ents: string[][]): SchemaTemplateDTO => ({
      id, key: id, entity, pack: 'a', pack_version: '1', default: false,
      definition: { id, entity, name: id, roles: ents.map((e, i) => ({ key: `r${i}`, entities: e, forms: f })) },
    })
    const schema = (events: SchemaTemplateDTO[], rel: string[][]): SchemaDTO => ({
      language: 'xx',
      entities: [
        { id: 'a.rel', container: true, period: { start: 'С', end: 'По' } },
        { id: 'a.ev', container: true, period: { start: 'Дата', end: null } },
      ],
      templates: [tpl('rel', 'a.rel', rel), ...events],
    })
    const ctx = (s: SchemaDTO): Ctx => ({ ...ctxOf(ruSchema(), []), reg: buildRegistry(s) })
    // поровну в отношениях — решают события; события не перевешивают отношения
    expect(peopleEntity(ctx(schema([tpl('e1', 'a.ev', [['a.human']])], [['a.human'], ['a.club']])))).toBe('a.human')
    const clubEvents = ['e1', 'e2', 'e3'].map(id => tpl(id, 'a.ev', [['a.club']]))
    expect(peopleEntity(ctx(schema(clubEvents, [['a.human'], ['a.human'], ['a.club']])))).toBe('a.human')
    expect(peopleEntity(ctx(schema([], [['a.human'], ['a.club']])))).toBeNull()
  })

  it('ведущие роли — роли без сущности людей, без знания паков', () => {
    const tpl = (roles: TemplateDef['roles']): TemplateDef => ({ id: 't', entity: 'x', name: 'T', roles })
    const f = { default: 'r' }
    // организация ведущая, даже если людей в роли — один, а организаций — сколько угодно
    expect([...leadRoles(tpl([
      { key: 'm', entities: ['a.human'], max: 1, forms: f }, { key: 'club', entities: ['a.club'], max: null, forms: f },
    ]), 'a.human')]).toEqual(['club'])
    // роль «любая сущность» — люди; без людей или без сущности людей — ведущих нет
    expect([...leadRoles(tpl([{ key: 'any', forms: f }, { key: 'club', entities: ['a.club'], forms: f }]), 'a.human')]).toEqual(['club'])
    expect(leadRoles(tpl([{ key: 'c', entities: ['a.club'], forms: f }, { key: 'd', entities: ['a.club'], forms: f }]), 'a.human').size).toBe(0)
    expect(leadRoles(tpl([{ key: 'c', entities: ['a.club'], forms: f }]), null).size).toBe(0)
    // людей по реестру не различить — ведущих нет, по числу участников роли не гадаем
    expect(leadRoles(tpl([
      { key: 'm', entities: ['a.human'], max: 1, forms: f }, { key: 'club', entities: ['a.club'], max: null, forms: f },
    ]), null).size).toBe(0)
  })

  it('справа — шаблон, подпись в скобках, если отличается', () => {
    expect(templateWithTitle('Пара', 'Брак')).toBe('Пара (Брак)')
    expect(templateWithTitle('Пара', 'Пара')).toBe('Пара')
  })
})
