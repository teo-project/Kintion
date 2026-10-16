import { describe, expect, it } from 'vitest'

import { freeChoiceValue, storedRaw, valueIn } from '@/core/resolver'

import { choiceOptions, CUSTOM_TYPES, customDef, customKey, fromKind, kindOf, withCustom } from './customFields'

describe('свободные поля', () => {
  it('ключ из названия: латиница — как есть, иначе f_n; без повторов', () => {
    expect(customKey('Army unit', [])).toBe('army_unit')
    expect(customKey('Army unit', ['army_unit'])).toBe('army_unit_1')
    expect(customKey('Воинская часть', [])).toBe('f_1')
    expect(customKey('Воинская часть', ['f_1'])).toBe('f_2')
    expect(customKey('Café №5', [])).toMatch(/^[a-z0-9_]+$/)
  })

  it('описание и список', () => {
    const d = customDef('x', { name: ' Часть ', type: 'text', history: true, multiple: false })
    expect(d).toEqual({ key: 'x', type: 'text', name: 'Часть', history: true })
    const list = withCustom([], 'x', d)
    expect(withCustom(list, 'x', { ...d, name: 'Ч' })).toEqual([{ ...d, name: 'Ч' }])
    expect(withCustom(list, 'x', null)).toEqual([])
    expect(customDef('x', { name: 'Ч', type: 'text', history: false, multiple: true }, d)).toEqual({ key: 'x', type: 'text', name: 'Ч', multiple: true })
  })

  it('типы F-96: выбор с вариантами, изображение со свободной обрезкой, ссылка с сущностью; контакта нет', () => {
    expect(CUSTOM_TYPES).toContain('choice')
    expect(CUSTOM_TYPES).toContain('image')
    expect(CUSTOM_TYPES).not.toContain('contact')
    const base = { history: false, multiple: false }
    const c = customDef('faith', { ...base, name: 'Вера', type: 'choice', options: 'Да\n\n Нет \nДа' })
    expect(c.options).toEqual([{ key: 'f_1', name: 'Да' }, { key: 'f_2', name: 'Нет' }])
    // прежние ключи сохраняются, новые не совпадают с ними
    expect(choiceOptions('Нет\nМожет быть', c.options)).toEqual([{ key: 'f_2', name: 'Нет' }, { key: 'f_3', name: 'Может быть' }])
    const img = customDef('pic', { ...base, name: 'Картинка', type: 'image', cropName: 'Обрезка' })
    expect(img.meta).toEqual([{ key: 'crop', name: 'Обрезка', editor: 'crop', places: ['card'] }])
    const ref = customDef('who', { ...base, name: 'Кто', type: 'ref', entity: 'p.e' })
    expect(ref.entities).toEqual(['p.e'])
    // переименование варианта — тот же ключ (значения не теряют подпись)
    expect(choiceOptions('Да\nНет больше', c.options)).toEqual([{ key: 'f_1', name: 'Да' }, { key: 'f_2', name: 'Нет больше' }])
    // несколько сущностей в описании сохраняются, пока ограничение не меняли
    const multi = { ...ref, entities: ['p.e', 'q.e'] }
    expect(customDef('who', { ...base, name: 'Кто-то', type: 'ref', entity: 'p.e' }, multi).entities).toEqual(['p.e', 'q.e'])
    expect(customDef('who', { ...base, name: 'Кто-то', type: 'ref', entity: 'q.e' }, multi).entities).toEqual(['q.e'])
    // смена типа убирает чужие настройки
    expect(customDef('who', { ...base, name: 'Кто', type: 'text', entity: 'p.e' }, ref)).toEqual({ key: 'who', type: 'text', name: 'Кто' })
    expect(customDef('who', { ...base, name: 'Кто', type: 'ref' }, ref).entities).toBeUndefined()
  })
})

describe('выбор со своим значением (F-100)', () => {
  const draft = { name: 'Настроение', type: 'choice' as const, history: false, multiple: false, options: 'Весело\nГрустно' }

  it('«выбор или своё» — choice с free', () => {
    expect(fromKind('choice_free')).toEqual({ type: 'choice', free: true })
    expect(kindOf({ type: 'choice', free: true })).toBe('choice_free')
    expect(kindOf({ type: 'choice' })).toBe('choice')
    const def = customDef('mood', { ...draft, free: true })
    expect(def.free).toBe(true)
    expect(customDef('mood', { ...draft, free: false }, def).free).toBeUndefined()
    expect(customDef('mood', { ...draft, type: 'text', free: true }).free).toBeUndefined()
  })

  it('своё, совпавшее с названием варианта, — ключ варианта', () => {
    const def = customDef('mood', { ...draft, free: true })
    const fun = def.options![0]!.key
    expect(freeChoiceValue(def, ' весело ')).toBe(fun)
    expect(freeChoiceValue(def, 'Спокойно')).toEqual({ free: 'Спокойно' })
    expect(freeChoiceValue(def, '  ')).toBeNull()
  })

  it('вписан текст, равный ключу варианта, — своё значение с пометкой (F-104)', () => {
    const def = customDef('mood', { ...draft, free: true })
    const fun = def.options![0]!.key
    const typed = freeChoiceValue(def, fun)
    expect(typed).toEqual({ free: fun })
    expect(valueIn(typed, { color: 'red' })).toEqual({ value: fun, meta: { color: 'red', free: true } })
    expect(valueIn(fun)).toEqual({ value: fun })
    expect(storedRaw({ value: fun, meta: { free: true } })).toEqual({ free: fun })
    expect(storedRaw({ value: fun, meta: null })).toBe(fun)
  })
})
