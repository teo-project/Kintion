import { describe, expect, it } from 'vitest'

import { ctxOf, ruSchema } from './__tests__/kit'
import { cardView, sectionChoices } from './card'

// F-139: своё поле — в любом блоке карточки; свои блоки — после блоков шаблона.
describe('свои блоки карточки (F-139)', () => {
  const schema = ruSchema()
  const person = schema.templates.find(t => t.key === 'kin.core.person')!
  person.definition = {
    ...person.definition,
    custom_fields: [{ key: 'faith', type: 'text', name: 'Вера', section: 'fam' }],
    custom_sections: [{ key: 'fam', name: 'Семья' }],
  }
  const ctx = ctxOf(schema, [{
    id: 'p', template: 'kin.core.person',
    fields: { name: ['Лиза'], 'custom.a': ['1'], 'custom.b': ['2'], 'custom.c': ['3'], 'custom.d': ['4'] },
    custom: [
      { key: 'a', type: 'text', name: 'А', section: 'main' },
      { key: 'b', type: 'text', name: 'Б', section: 'war' },
      { key: 'c', type: 'text', name: 'В' },
      { key: 'd', type: 'text', name: 'Г', section: 'gone' },
    ],
  }])
  const rec = ctx.records.p!
  rec.custom_sections = [{ key: 'war', name: 'Война' }, { key: 'empty', name: 'Пусто' }]

  it('порядок блоков: шаблона, «Свои поля», общие свои, свои записи', () => {
    expect(sectionChoices(ctx, rec).map(s => s.key)).toEqual(['main', 'additional', 'custom', 'fam', 'war', 'empty'])
    expect(sectionChoices(ctx, rec).filter(s => s.own).map(s => s.key)).toEqual(['war', 'empty'])
  })

  it('поля по блокам; блока нет — «Свои поля»; пустой свой блок есть', () => {
    const card = cardView(ctx, rec)
    const by = Object.fromEntries(card.sections.map(s => [s.key, s.fields.map(f => f.name)]))
    expect(by.main).toContain('А')
    expect(by.war).toEqual(['Б'])
    expect(by.custom).toEqual(['В', 'Г'])
    expect(by.fam).toEqual(['Вера'])
    expect(by.empty).toEqual([])
    expect(card.sections.map(s => s.key)).toEqual(['main', 'additional', 'custom', 'fam', 'war', 'empty'])
    const war = card.sections.find(s => s.key === 'war')!
    expect(war.name).toBe('Война')
    expect(war.custom).toEqual({ def: { key: 'war', name: 'Война' }, shared: false })
    expect(card.sections.find(s => s.key === 'fam')!.custom?.shared).toBe(true)
    expect(card.sections.find(s => s.key === 'main')!.custom).toBeUndefined()
  })
})
