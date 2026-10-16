import { describe, expect, it } from 'vitest'

import { altSchema, ctxOf, ruSchema } from './__tests__/kit'
import type { Condition, GrammarElement } from './types'
import { evalAddress, evalCondition, evalElement, findSlot, parseAddress, rawValue, recordTitle, sameRaw, textValue } from './resolver'

const liza = {
  id: 'liza', template: 'kin.core.person', start: { y: 1985, m: 3, d: 12 }, note: 'по паспорту 1923',
  fields: {
    name: ['Лиза'], patronymic: ['Сергеевна'], gender: ['female'],
    surname: ['Василькова', 'Петрова', { value: 'Давыдко', note: 'девичья' }],
    phone: ['+7 900', '+7 901'],
    'custom.army_unit': ['в/ч 1234'],
  },
  custom: [{ key: 'army_unit', type: 'text' as const, name: 'Воинская часть', history: true }],
}

describe('адресация и подписи на kin.core', () => {
  const ctx = ctxOf(ruSchema(), [liza, { id: 'empty', template: 'kin.core.person' },
    { id: 'pair', template: 'kin.core.relationship', parts: [['partner', 'liza']], title: 'Наша пара' }])
  const rec = ctx.records.liza!
  const t = (a: string) => textValue(ctx, rec, a)

  it('подпись по правилу шаблона', () => {
    expect(recordTitle(ctx, rec)).toBe('Лиза Сергеевна Василькова')
  })
  it('история: индексы и части', () => {
    expect(t('surname')).toBe('Василькова')
    expect(t('surname[1]')).toBe('Петрова')
    expect(t('surname[-1]')).toBe('Давыдко')
    expect(t('surname[-1].note')).toBe('девичья')
    expect(t('surname[5]')).toBe('')
  })
  it('разрешённые значения и системные поля', () => {
    expect(t('@start.year')).toBe('1985')
    expect(t('@start')).toBe('12 марта 1985')
    expect(t('start[3]')).toBe('12 марта 1985')
    expect(t('@gender')).toBe('Женский')
    expect(t('gender')).toBe('female')
    expect(t('.note')).toBe('по паспорту 1923')
  })
  it('описание поля через :', () => {
    expect(t('surname:name')).toBe('Фамилия')
    expect(t('start:name')).toBe('Дата рождения')
    expect(t('custom.army_unit:name')).toBe('Воинская часть')
  })
  it('свободные поля — только по префиксу custom.', () => {
    expect(t('@custom.army_unit')).toBe('в/ч 1234')
    expect(t('army_unit')).toBe('')
    expect(t('@custom.missing')).toBe('')
  })
  it('multiple без индекса — все значения, с индексом — по порядку', () => {
    expect(t('phone')).toBe('+7 900, +7 901')
    expect(t('phone[1]')).toBe('+7 901')
  })
  it('запасная подпись — имя шаблона; ручная правка', () => {
    expect(recordTitle(ctx, ctx.records.empty!)).toBe('Человек')
    expect(recordTitle(ctx, ctx.records.pair!)).toBe('Наша пара')
    expect(evalAddress(ctx, ctx.records.pair!, '@partner').map(i => i.text)).toEqual(['Лиза Сергеевна Василькова'])
  })
  it('разбор адреса', () => {
    expect(parseAddress('@custom.x[-1].note:name')).toEqual(
      { resolved: true, custom: true, key: 'x', index: -1, parts: ['note'], prop: 'name' })
    expect(() => parseAddress('[1]')).toThrow()
    expect(() => parseAddress('a b')).toThrow()
  })
})

describe('те же механизмы на синтетическом паке', () => {
  const ctx = ctxOf(altSchema(), [
    {
      id: 'luna', template: 'test.alt.being', start: { y: 1001 }, note: 'заметка',
      fields: { given: ['Луна'], clan: ['Серебряные', { value: 'Тёмные', note: 'по рождению' }], kind: ['moon'] },
      custom: [{ key: 'x', type: 'text' as const, name: 'Икс' }],
    },
    { id: 'nameless', template: 'test.alt.being' },
    { id: 'u', template: 'test.alt.union_rite', parts: [['ally', 'luna']] },
  ])
  const rec = ctx.records.luna!
  const t = (a: string) => textValue(ctx, rec, a)

  it('подпись, история, части, описание', () => {
    expect(recordTitle(ctx, rec)).toBe('Серебряные, Луна')
    expect(t('clan[-1]')).toBe('Тёмные')
    expect(t('clan[-1].note')).toBe('по рождению')
    expect(t('@kind')).toBe('Лунный')
    expect(t('@start.year')).toBe('1001')
    expect(t('.note')).toBe('заметка')
    expect(t('given:name')).toBe('Данное имя')
    expect(t('start:name')).toBe('Появилось')
    expect(t('@custom.x')).toBe('')
    expect(recordTitle(ctx, ctx.records.nameless!)).toBe('Существо')
    expect(recordTitle(ctx, ctx.records.u!)).toBe('Обряд союза · Серебряные, Луна')
  })
})

describe('условия if / not: строка или объект (F-84)', () => {
  const ctx = ctxOf(ruSchema(), [
    liza,
    { id: 'one', template: 'kin.core.person', fields: { surname: ['Петрова'], gender: ['female'] } },
    { id: 'same', template: 'kin.core.person', fields: { surname: ['Петрова', 'Иванова', 'Петрова'] } },
    { id: 'old', template: 'kin.core.person', start: { y: 1950 }, fields: { name: ['Анна'] } },
  ])
  const rec = (id: string) => ctx.records[id]!
  // пример из docs/pack-format.md: девичья фамилия — только если первая отличается от текущей
  const maiden: GrammarElement = {
    pattern: ' ({maiden})',
    data: { maiden: { field: 'surname[-1]' } },
    if: { ne: ['surname[0]', 'surname[-1]'] },
  }

  it('девичья фамилия: есть отличие — в скобках; одна или совпадает — пусто', () => {
    expect(evalElement(ctx, rec('liza'), maiden)).toBe(' (Давыдко)')
    expect(evalElement(ctx, rec('one'), maiden)).toBe('')
    expect(evalElement(ctx, rec('same'), maiden)).toBe('')
    expect(evalElement(ctx, rec('old'), maiden)).toBe('') // фамилии нет совсем
  })
  it('строка — наличие значения, как раньше', () => {
    expect(evalElement(ctx, rec('liza'), { string: 'x', if: 'surname' })).toBe('x')
    expect(evalElement(ctx, rec('old'), { string: 'x', if: '@surname' })).toBe('')
    expect(evalElement(ctx, rec('old'), { string: 'x', not: 'surname' })).toBe('x')
  })
  it('eq / ne: строка — всегда адрес, строковая константа — { const }', () => {
    expect(evalCondition(ctx, rec('liza'), { eq: ['gender', { const: 'female' }] })).toBe(true)
    expect(evalCondition(ctx, rec('liza'), { ne: ['gender', { const: 'female' }] })).toBe(false)
    expect(evalCondition(ctx, rec('liza'), { eq: ['gender', 'female'] })).toBe(false) // 'female' — адрес, значения нет
    expect(evalCondition(ctx, rec('old'), { eq: ['gender', null] })).toBe(true)
    expect(evalCondition(ctx, rec('same'), { eq: ['surname[0]', 'surname[-1]'] })).toBe(true)
  })
  it('gt / lt — числа и даты; несравнимое — ложь', () => {
    expect(evalCondition(ctx, rec('old'), { lt: ['start', { const: { y: 1960 } }] })).toBe(true)
    expect(evalCondition(ctx, rec('old'), { gt: ['start', { const: { y: 1960 } }] })).toBe(false)
    expect(evalCondition(ctx, rec('liza'), { gt: [{ count: 'surname' }, 2] })).toBe(true)
    expect(evalCondition(ctx, rec('liza'), { lt: [{ count: 'phone' }, 2] })).toBe(false)
    expect(evalCondition(ctx, rec('liza'), { gt: ['name', { const: 'А' }] })).toBe(false)
    expect(evalCondition(ctx, rec('liza'), { gt: [{ count: 'surname' }, 1] })).toBe(true)
  })
  it('count — вся история; all / any', () => {
    expect(evalCondition(ctx, rec('liza'), { count: 'surname' })).toBe(true)
    expect(evalCondition(ctx, rec('old'), { count: 'surname' })).toBe(false)
    expect(evalCondition(ctx, rec('liza'), { eq: [{ count: 'surname' }, 3] })).toBe(true)
    const female: Condition = { all: [{ eq: ['gender', { const: 'female' }] }, { ne: ['surname[0]', 'surname[-1]'] }] }
    expect(evalCondition(ctx, rec('liza'), female)).toBe(true)
    expect(evalCondition(ctx, rec('one'), female)).toBe(false)
    expect(evalCondition(ctx, rec('one'), { any: ['patronymic', { eq: ['gender', { const: 'female' }] }] })).toBe(true)
    expect(evalCondition(ctx, rec('old'), { any: ['patronymic', { eq: ['gender', { const: 'female' }] }] })).toBe(false)
  })
})

describe('общие свои поля шаблона (F-96)', () => {
  it('адрес shared.<key> — значение записи по описанию из шаблона дашборда', () => {
    const ctx = ctxOf(ruSchema(), [{ id: 'a', template: 'kin.core.person', fields: { 'shared.religion': ['нет'] } }])
    const rec = ctx.records.a!
    const tpl = ctx.reg.byRowId.get(rec.template_id)!.definition
    tpl.custom_fields = [{ key: 'religion', type: 'text', name: 'Религия' }]
    const addr = parseAddress('@shared.religion')
    expect(addr).toMatchObject({ resolved: true, shared: true, custom: false, key: 'religion' })
    const slot = findSlot(ctx, rec, addr)
    expect(slot.kind).toBe('custom')
    expect(slot.kind === 'custom' && slot.values[0]!.value).toBe('нет')
    expect(findSlot(ctx, rec, parseAddress('@shared.other')).kind).toBe('none')
  })
})

describe('своё значение выбора помечено (F-104)', () => {
  it('помеченный текст, равный ключу варианта, — не вариант: подпись, rawValue, условия', () => {
    const custom = [{ key: 'mood', type: 'choice' as const, name: 'Настроение', free: true, options: [{ key: 'fun', name: 'Весело' }] }]
    const ctx = ctxOf(ruSchema(), [
      { id: 'own', template: 'kin.core.person', custom, fields: { 'custom.mood': [{ value: 'fun', meta: { free: true } }] } },
      { id: 'opt', template: 'kin.core.person', custom, fields: { 'custom.mood': ['fun'] } },
    ])
    const { own, opt } = ctx.records
    expect(textValue(ctx, own!, '@custom.mood')).toBe('fun')
    expect(textValue(ctx, opt!, '@custom.mood')).toBe('Весело')
    expect(textValue(ctx, own!, 'custom.mood')).toBe('fun')
    expect(rawValue(ctx, own!, 'custom.mood')).toEqual({ free: 'fun' })
    expect(rawValue(ctx, opt!, 'custom.mood')).toBe('fun')
    const isFun: Condition = { eq: ['custom.mood', { const: 'fun' }] }
    expect(evalCondition(ctx, own!, isFun)).toBe(false)
    expect(evalCondition(ctx, opt!, isFun)).toBe(true)
    expect(evalCondition(ctx, own!, 'custom.mood')).toBe(true)
    // сравнение по содержимому (quick_add: for, single)
    expect(sameRaw(rawValue(ctx, own!, 'custom.mood'), { free: 'fun' })).toBe(true)
    expect(sameRaw(rawValue(ctx, own!, 'custom.mood'), 'fun')).toBe(false)
    expect(sameRaw({ y: 2001, m: 3 }, { m: 3, y: 2001 })).toBe(true)
  })
})

describe('подпись организации с правовой формой (F-133)', () => {
  const ctx = ctxOf(ruSchema(), [
    { id: 'a', template: 'kin.core.organization', fields: { name: ['Ромашка'], legal_form: ['ooo'] } },
    { id: 'b', template: 'kin.core.organization', fields: { name: ['Ромашка'] } },
    { id: 'c', template: 'kin.core.organization', fields: { name: ['Ромашка'], legal_form: [{ value: 'ЖСК', meta: { free: true } }] } },
  ])
  it('форма + название в «ёлочках»; без формы — название', () => {
    expect(recordTitle(ctx, ctx.records.a!)).toBe('ООО «Ромашка»')
    expect(recordTitle(ctx, ctx.records.b!)).toBe('Ромашка')
    expect(recordTitle(ctx, ctx.records.c!)).toBe('ЖСК «Ромашка»')
  })
})
