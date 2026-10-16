import { describe, expect, it } from 'vitest'

import { altSchema, ctxOf, ruSchema, type RecSpec } from './__tests__/kit'
import { cardView } from './card'
import { activeToday, roleLabel } from './forms'
import { buildDraft, draftBatch, existingBatch, existingSide, fitsExisting, quickAddMenu, quickInputs } from './quickAdd'
import { coreWords, messagesFor } from '@/i18n'
import { recordTitle } from './resolver'

const people: RecSpec[] = [
  { id: 'anna', template: 'kin.core.person', fields: { name: ['Анна'], surname: ['Василькова'], gender: ['female'] } },
  { id: 'ivan', template: 'kin.core.person', fields: { name: ['Иван'], surname: ['Краник', 'Краников'], gender: ['male'] } },
  { id: 'maria', template: 'kin.core.person', fields: { name: ['Мария'], gender: ['female'] } },
  { id: 'org', template: 'kin.core.organization', fields: { name: ['Завод'] } },
]

describe('формы ролей и карточка на kin.core', () => {
  const ctx = ctxOf(ruSchema(), [
    ...people,
    { id: 'w', template: 'kin.core.wedding', start: { y: 2021, m: 6 }, parts: [['partner', 'anna'], ['partner', 'ivan']] },
    { id: 'pc1', template: 'kin.core.parent_child', parts: [['parent', 'ivan'], ['child', 'anna']] },
    { id: 'pc2', template: 'kin.core.parent_child', parts: [['parent', 'maria'], ['child', 'anna']] },
    { id: 'job', template: 'kin.core.work', parts: [['employer', 'org'], ['subordinate', 'anna']] },
    { id: 'm', template: 'kin.core.relationship', fields: { stage: ['marriage', 'dating'] }, parts: [['partner', 'anna'], ['partner', 'ivan']] },
    { id: 'm0', template: 'kin.core.relationship', parts: [['partner', 'anna'], ['partner', 'ivan']] },
  ])
  const r = ctx.records

  it('подпись события', () => {
    expect(recordTitle(ctx, r.w!)).toBe('Свадьба · Анна Василькова, Иван Краник · 2021')
  })

  it('формы со стороны другого участника', () => {
    const anna = cardView(ctx, r.anna!)
    const by = (id: string) => anna.relations.find(x => x.record === id)!
    expect(by('pc1').others.map(o => o.label)).toEqual(['Папа'])
    expect(by('pc2').others.map(o => o.label)).toEqual(['Мама'])
    expect(by('pc1').myLabel).toBe('Дочь')
    expect(by('job').others.map(o => o.label)).toEqual(['Работодатель'])
    expect(cardView(ctx, r.org!).relations[0]!.others.map(o => o.label)).toEqual(['Подчинённая'])
    expect(cardView(ctx, r.ivan!).relations.find(x => x.record === 'pc1')!.others[0]!.label).toBe('Дочь')
  })

  it('этап: marriage — Муж / Жена, не выбран — формы default', () => {
    expect(roleLabel(ctx, r.m!, 'partner', 'ivan')).toBe('Муж')
    expect(roleLabel(ctx, r.m!, 'partner', 'anna')).toBe('Жена')
    expect(roleLabel(ctx, r.m0!, 'partner', 'ivan')).toBe('Партнёр')
    const anna = cardView(ctx, r.anna!)
    expect(anna.relations.find(x => x.record === 'm')!.stage).toBe('Брак')
    expect(cardView(ctx, r.m!).stage!.current).toEqual({ key: 'marriage', text: 'Брак' })
    expect(recordTitle(ctx, r.m!)).toBe('Брак')
    expect(recordTitle(ctx, r.m0!)).toBe('Пара')
  })

  it('вид связи «Родитель — ребёнок»: формы по варианту и полу, своё значение — формы роли (F-118)', () => {
    const c = ctxOf(ruSchema(), [
      ...people,
      { id: 'st', template: 'kin.core.parent_child', fields: { kind: ['step'] }, parts: [['parent', 'ivan'], ['child', 'anna']] },
      { id: 'ad', template: 'kin.core.parent_child', fields: { kind: ['adopted'] }, parts: [['parent', 'maria'], ['child', 'ivan']] },
      { id: 'fr', template: 'kin.core.parent_child', fields: { kind: [{ value: 'step', meta: { free: true } }] }, parts: [['parent', 'ivan'], ['child', 'maria']] },
    ])
    expect(roleLabel(c, c.records.st!, 'parent', 'ivan')).toBe('Отчим')
    expect(roleLabel(c, c.records.st!, 'child', 'anna')).toBe('Падчерица')
    expect(roleLabel(c, c.records.ad!, 'parent', 'maria')).toBe('Приёмная мать')
    expect(roleLabel(c, c.records.fr!, 'parent', 'ivan')).toBe('Папа')
  })

  it('мгновенное событие и действующее отношение (Д-20)', () => {
    expect(activeToday(ctx, r.w!)).toBe(false)
    expect(activeToday(ctx, r.m!)).toBe(true)
  })

  it('поле end_flag — в шапке у конца периода, а не в разделе (F-126)', () => {
    const card = cardView(ctx, r.anna!)
    expect(card.endFlag?.def.key).toBe('deceased')
    expect(card.sections.flatMap(sec => [...sec.fields, ...sec.addable]).some(f => f.def.key === 'deceased')).toBe(false)
  })

  it('пустая дата смерти скрыта за «Добавить», заполненная — на месте (F-99)', () => {
    expect(cardView(ctx, r.anna!).dates.map(d => [d.key, d.hidden])).toEqual([['start', false], ['end', true]])
    const dead = ctxOf(ruSchema(), [{ id: 'p', template: 'kin.core.person', fields: { name: ['П'] }, end: { y: 1990 } }])
    expect(dead.records.p && cardView(dead, dead.records.p).dates.map(d => d.hidden)).toEqual([false, false])
    // в быстрой форме конец не скрывается, если он в quick_fields
    const person = ctx.reg.templates.get('kin.core.person')!.definition
    const inputs = quickInputs(ctx.reg, coreWords(messagesFor('ru')), { ...person, quick_fields: ['end'] })
    expect(inputs.map(i => i.label)).toEqual(['Дата смерти'])
  })

  it('карточка: даты, разделы, роли', () => {
    const card = cardView(ctx, r.anna!)
    expect(card.dates.map(d => d.label)).toEqual(['Дата рождения', 'Дата смерти'])
    expect(card.labels.note).toBe('Примечание')
    const main = card.sections.find(s => s.key === 'main')!
    expect(main.name).toBe('Основные')
    expect(main.fields.find(f => f.key === 'gender')!.shown).toEqual(['Женский'])
    expect(main.addable.map(f => f.key)).toContain('patronymic')
    const wedding = cardView(ctx, r.w!)
    expect(wedding.dates.map(d => d.label)).toEqual(['Дата']) // у события конца нет
    expect(wedding.roles.find(x => x.role === 'partner')!.items.map(i => i.label)).toEqual(['Невеста', 'Жених'])
  })

  it('меню (+): после отца «Добавить мать» остаётся, «Добавить отца» скрыт', () => {
    const one = ctxOf(ruSchema(), [...people, { id: 'pc', template: 'kin.core.parent_child', parts: [['parent', 'ivan'], ['child', 'anna']] }])
    const names = quickAddMenu(one, one.records.anna!).map(o => o.item.name)
    expect(names).toContain('Добавить мать')
    expect(names).not.toContain('Добавить отца')
    // все родители и муж есть — в меню только ребёнок (брата / сестры в меню нет, F-43)
    expect(quickAddMenu(ctx, r.anna!).map(o => o.item.name)).toEqual(['Добавить ребёнка'])
    const fresh = quickAddMenu(ctx, r.maria!).map(o => o.item.name)
    expect(fresh).toEqual(expect.arrayContaining(['Добавить отца', 'Добавить мать', 'Добавить ребёнка', 'Добавить мужа']))
    expect(fresh).not.toContain('Добавить жену')
    expect(quickAddMenu(ctx, r.org!).map(o => o.item.name)).not.toContain('Добавить отца')
  })

  it('«Добавить ребёнка» — текущая фамилия родителя; отцу — фамилия при рождении', () => {
    const child = quickAddMenu(ctx, r.ivan!).find(o => o.item.name === 'Добавить ребёнка')!
    const draft = buildDraft(ctx, r.ivan!, child)
    expect(draft.values).toEqual({ surname: 'Краник' })
    expect(draft.inputs.map(i => i.label)).toEqual(['Имя', 'Фамилия', 'Пол', 'Примечание', 'Дата рождения'])
    const father = quickAddMenu(ctx, r.ivan!).find(o => o.item.name === 'Добавить отца')!
    const fd = buildDraft(ctx, r.ivan!, father)
    expect(fd.values).toEqual({ gender: 'male', surname: 'Краников' })
    expect(fd.hidden).toEqual({})
    fd.values.name = 'Пётр'
    fd.values.start = { y: 1950 }
    expect(draftBatch(r.ivan!, fd)).toEqual({
      records: [
        { ref: 'new', template: 'kin.core.person', fields: { gender: [{ value: 'male' }], name: [{ value: 'Пётр' }], surname: [{ value: 'Краников' }] }, start: { value: { y: 1950 } } },
        { ref: 'rel', template: 'kin.core.parent_child' },
      ],
      participants: [
        { container: 'rel', entity: 'ivan', role: 'child' },
        { container: 'rel', entity: 'new', role: 'parent' },
      ],
    })
  })
  it('«Существующий»: только отношение с выбранной записью; сторона в дереве — по роли (F-130)', () => {
    const menu = quickAddMenu(ctx, r.ivan!)
    const father = menu.find(o => o.item.name === 'Добавить отца')!
    const child = menu.find(o => o.item.name === 'Добавить ребёнка')!
    expect(existingSide(father)).toBe('up')
    expect(existingSide(child)).toBe('down')
    // «Добавить отца» — не женщина; без пола — можно (F-42)
    expect(fitsExisting(ctx, father, r.anna!)).toBe(false)
    expect(fitsExisting(ctx, father, r.org!)).toBe(true)
    // фамилия и прочие `new` — только для нового: другая фамилия подсказку не скрывает
    const withSurname = { ...father, item: { ...father.item, new: { ...father.item.new, surname: 'Другая' } } }
    expect(fitsExisting(ctx, withSurname, r.ivan!)).toBe(true)
    expect(fitsExisting(ctx, withSurname, r.anna!)).toBe(false)
    expect(existingBatch(r.ivan!, buildDraft(ctx, r.ivan!, father), 'maria')).toEqual({
      records: [{ ref: 'rel', template: 'kin.core.parent_child' }],
      participants: [
        { container: 'rel', entity: 'ivan', role: 'child' },
        { container: 'rel', entity: 'maria', role: 'parent' },
      ],
    })
  })
  it('«Добавить отца»: начало отношения — дата рождения ребёнка; нет даты — не заполняется (F-102)', () => {
    const born = ctxOf(ruSchema(), [...people, { id: 'kid', template: 'kin.core.person', start: { y: 2001, m: 3 }, fields: { name: ['Ося'] } }])
    const father = quickAddMenu(born, born.records.kid!).find(o => o.item.name === 'Добавить отца')!
    const d = buildDraft(born, born.records.kid!, father)
    expect(d.relation).toEqual({ start: { y: 2001, m: 3 } })
    expect(d.problems).toEqual([])
    d.values.name = 'Пётр'
    expect(draftBatch(born.records.kid!, d).records[1]).toEqual({ ref: 'rel', template: 'kin.core.parent_child', start: { value: { y: 2001, m: 3 } } })
    const fd = buildDraft(ctx, r.ivan!, quickAddMenu(ctx, r.ivan!).find(o => o.item.name === 'Добавить мать')!)
    expect(fd.relation).toEqual({})
  })

  it('relation: неизвестный ключ — в problems, не пишется', () => {
    const opt = quickAddMenu(ctx, r.ivan!).find(o => o.item.name === 'Добавить отца')!
    const item = { ...opt.item, relation: { nope: 'x', '.note': { field: 'name' } } }
    const d = buildDraft(ctx, r.ivan!, { ...opt, item })
    expect(d.relation).toEqual({ '.note': 'Иван' })
    expect(d.problems).toEqual(['relation: nope'])
    expect(draftBatch(r.ivan!, d).records[1]).toEqual({ ref: 'rel', template: 'kin.core.parent_child', note: 'Иван' })
  })
})

describe('меню и формы на синтетическом паке', () => {
  const ctx = ctxOf(altSchema(), [
    { id: 'luna', template: 'test.alt.being', fields: { given: ['Луна'], clan: ['Серебряные', 'Древние'], kind: ['moon'] } },
    { id: 'sol', template: 'test.alt.being', fields: { given: ['Сол'], kind: ['sun'] } },
    { id: 'b', template: 'test.alt.bond', fields: { stage: ['bound'] }, parts: [['ally', 'luna'], ['ally', 'sol']] },
    { id: 'b0', template: 'test.alt.bond', parts: [['ally', 'luna'], ['ally', 'sol']] },
    { id: 'l', template: 'test.alt.lineage', parts: [['source', 'sol'], ['offspring', 'luna']] },
  ])
  const r = ctx.records

  it('формы по своему признаку и этапу', () => {
    expect(roleLabel(ctx, r.b!, 'ally', 'sol')).toBe('Спутник')
    expect(roleLabel(ctx, r.b!, 'ally', 'luna')).toBe('Спутница')
    expect(roleLabel(ctx, r.b0!, 'ally', 'sol')).toBe('Союзник')
    const luna = cardView(ctx, r.luna!)
    expect(luna.relations.find(x => x.record === 'l')!.others.map(o => o.label)).toEqual(['Отец-светило'])
    expect(luna.relations.find(x => x.record === 'l')!.myLabel).toBe('Дочь')
    expect(luna.dates.map(d => d.label)).toEqual(['Появилось', 'Исчезло'])
    expect(cardView(ctx, r.b!).stage!.label).toBe('Ступень')
    expect(luna.labels.note).toBe('Заметка')
  })

  it('меню (+): for, single, этап, предзаполнение', () => {
    const names = quickAddMenu(ctx, r.luna!).map(o => o.item.name)
    // спутник-светило уже есть на этапе bound; исток-светило занят, исток-луна свободен
    expect(names).toEqual(['Добавить исток-луну', 'Добавить потомка'])
    expect(quickAddMenu(ctx, r.sol!).map(o => o.item.name)).toEqual(['Добавить исток-светило', 'Добавить исток-луну', 'Добавить потомка'])
    const moon = quickAddMenu(ctx, r.luna!).find(o => o.item.name === 'Добавить исток-луну')!
    const d = buildDraft(ctx, r.luna!, moon)
    expect(d.values).toEqual({ clan: 'Древние' })
    expect(d.hidden).toEqual({ kind: 'moon' })
    expect(d.inputs.map(i => i.label)).toEqual(['Данное имя', 'Род', 'Заметка', 'Появилось'])
  })
})

describe('прошлая связь (F-98)', () => {
  const pair = (id: string, end?: RecSpec['end']): RecSpec => ({
    id, template: 'kin.core.relationship', start: { y: 2000 }, end, parts: [['partner', 'anna'], ['partner', 'ivan']],
  })
  const pastOf = (specs: RecSpec[], today: string, patch?: (c: ReturnType<typeof ctxOf>) => void) => {
    const ctx = ctxOf(ruSchema(), [...people, ...specs], today)
    patch?.(ctx)
    return Object.fromEntries(cardView(ctx, ctx.records.anna!).relations.map(r => [r.record, r.past]))
  }

  it('конец в будущем — текущая; частичная дата наступила после верхней границы', () => {
    const specs = [pair('y2025', { y: 2025 }), pair('m2026', { y: 2026, m: 10 }), pair('d', { y: 2026, m: 10, d: 11 }), pair('none')]
    expect(pastOf(specs, '2025-12-31')).toEqual({ y2025: false, m2026: false, d: false, none: false })
    expect(pastOf(specs, '2026-01-01')).toEqual({ y2025: true, m2026: false, d: false, none: false })
    expect(pastOf(specs, '2026-10-12')).toEqual({ y2025: true, m2026: false, d: true, none: false })
    expect(pastOf(specs, '2026-11-01')).toEqual({ y2025: true, m2026: true, d: true, none: false })
  })

  it('конец без известной даты (нет года) — связь текущая', () => {
    expect(pastOf([pair('noyear', { m: 5 })], '2026-10-12')).toEqual({ noyear: false })
  })

  it('дата «по» моего участия: будущая — текущая, наступившая — прошлая', () => {
    const setTo = (y: number) => (ctx: ReturnType<typeof ctxOf>) => {
      const p = ctx.records.anna!.participates_in.find(x => x.container_id === 'none')!
      p.valid_to = { y }
    }
    expect(pastOf([pair('none')], '2026-10-12', setTo(2027))).toEqual({ none: false })
    expect(pastOf([pair('none')], '2026-10-12', setTo(2025))).toEqual({ none: true })
  })
})

describe('порядок «Связей» по весу (F-103)', () => {
  const rel = (id: string, template: string, other: string, start?: RecSpec['start'], end?: RecSpec['end']): RecSpec => ({
    id, template, start, end, parts: template === 'kin.core.parent_child' ? [['parent', other], ['child', 'anna']] : [[template === 'kin.core.work' ? 'employer' : 'partner', other], [template === 'kin.core.work' ? 'subordinate' : 'partner', 'anna']],
  })

  it('по весу, при равном — по дате начала, без даты — в конце; без веса — после всех; прямые ссылки — ниже', () => {
    const ctx = ctxOf(ruSchema(), [
      ...people,
      { id: 'b', template: 'kin.core.birth', start: { y: 1990 }, parts: [['child', 'anna']] },
      rel('job', 'kin.core.work', 'org', { y: 2010 }),
      rel('pair2', 'kin.core.relationship', 'ivan'),
      rel('pair1', 'kin.core.relationship', 'ivan', { y: 2015 }),
      rel('pair0', 'kin.core.relationship', 'ivan', { y: 2012 }),
      rel('parent', 'kin.core.parent_child', 'maria', { y: 1990 }),
      rel('oldpair', 'kin.core.relationship', 'ivan', { y: 2001 }, { y: 2005 }),
      rel('oldjob', 'kin.core.work', 'org', { y: 2000 }, { y: 2003 }),
    ])
    const tpl = ctx.reg.templates.get('kin.core.relationship')!.definition
    const view = () => cardView(ctx, ctx.records.anna!).relations
    const order = view().map(r => r.record)
    expect(order).toEqual(['parent', 'oldpair', 'pair0', 'pair1', 'pair2', 'oldjob', 'job', 'b'])
    expect(view().filter(r => r.past).map(r => r.record)).toEqual(['oldpair', 'oldjob'])
    // без веса — после всех с весом
    delete tpl.links_rank
    expect(view().map(r => r.record)).toEqual(['parent', 'oldjob', 'job', 'oldpair', 'pair0', 'pair1', 'pair2', 'b'])
  })
})
