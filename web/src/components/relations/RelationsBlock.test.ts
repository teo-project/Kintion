import { afterEach, describe, expect, it, vi } from 'vitest'

import { records, type RecSpec } from '@/core/__tests__/kit'
import { mountRecord, schema, serve } from '@/views/__tests__/card'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const specs: RecSpec[] = [
  { id: 'anna', template: 'kin.core.person', fields: { name: ['Анна'], surname: ['Василькова'], gender: ['female'] } },
  { id: 'ivan', template: 'kin.core.person', fields: { name: ['Иван'], surname: ['Краник'], gender: ['male'] } },
  { id: 'petr', template: 'kin.core.person', fields: { name: ['Пётр'], gender: ['male'] } },
  // без даты — в конце (Д-27)
  { id: 'z-nodate', template: 'kin.core.friend', parts: [['friend', 'anna'], ['friend', 'petr']] },
  { id: 'wedding', template: 'kin.core.wedding', start: { y: 2021, m: 6 }, parts: [['partner', 'anna'], ['partner', 'ivan']] },
  { id: 'pair', template: 'kin.core.relationship', start: { y: 2019 }, fields: { stage: ['marriage'] }, parts: [['partner', 'anna'], ['partner', 'ivan']] },
  // второй пак (TEO_KIN_PACKS=kin.core,kin.christian.orthodox) — так же по своим ролям
  { id: 'cm', template: 'kin.christian.orthodox.church_marriage', start: { y: 2022 }, parts: [['spouse', 'anna'], ['spouse', 'ivan']] },
  // закончилось — в «Прошлых», формы этапа те же («Муж», без «бывшего», F-74)
  { id: 'old', template: 'kin.core.relationship', start: { y: 2010 }, end: { y: 2015 }, fields: { stage: ['marriage'] }, parts: [['partner', 'anna'], ['partner', 'petr']] },
]

describe('«Связи» (F-89, F-93)', () => {
  it('строка — форма роли другого, подпись и «через», своя роль не пишется; порядок F-103 и Д-27; прошлые отдельно', async () => {
    const all = records(schema, specs)
    const anna = { ...all.anna!, revs: { start: 0, end: 0, note: 0, title: 0, sensitive: 0, groups: 0, custom_fields: 0 } }
    const related = Object.fromEntries(Object.entries(all).filter(([k]) => k !== 'anna'))
    serve((m, u) => (m === 'GET' && u === '/api/records/anna' ? [200, { record: anna, related }] : undefined))
    const w = await mountRecord('anna')
    expect(w.find('.relations h2').text()).toBe('Связи')
    const current = w.findAll('.relations .list.current .relation')
    // F-93: сначала люди через отношения, затем событие — без людей;
    // отношения по весу (F-103): церковный брак (10) — вместе с парой, выше друга
    expect(current.map(r => r.text().replace(/\s+/g, ' '))).toEqual([
      'Муж: Иван Краник (через Брак)',
      'Муж: Иван Краник (через Церковный брак)',
      'Друг: Пётр (через Дружба)',
      'Свадьба · Анна Василькова, Иван Краник · 2021',
    ])
    expect(w.find('.relations [data-relation="wedding"]').text()).not.toContain('Жених')
    expect(current[0]!.find('.other').attributes('href')).toBe('/d/d1/r/ivan')
    expect(current[0]!.find('.via-link').attributes('href')).toBe('/d/d1/r/pair')
    expect(w.text()).not.toContain('Жена') // своя роль не пишется
    const past = w.findAll('.relations .list.past .relation')
    expect(past.map(r => r.text().replace(/\s+/g, ' '))).toEqual(['Муж: Пётр (через Брак)'])
  })

  it('несколько участников: одинаковая форма — одной строкой, разные — разными', async () => {
    const all = records(schema, [
      ...specs.slice(0, 3),
      { id: 'maria', template: 'kin.core.person', fields: { name: ['Мария'], gender: ['female'] } },
      { id: 'k', template: 'kin.core.commune', parts: [['member', 'anna'], ['member', 'ivan'], ['member', 'petr'], ['member', 'maria']] },
    ])
    const anna = { ...all.anna!, revs: { start: 0, end: 0, note: 0, title: 0, sensitive: 0, groups: 0, custom_fields: 0 } }
    const related = Object.fromEntries(Object.entries(all).filter(([k]) => k !== 'anna'))
    serve((m, u) => (m === 'GET' && u === '/api/records/anna' ? [200, { record: anna, related }] : undefined))
    const w = await mountRecord('anna')
    const lines = w.findAll('.relations .relation').map(r => r.text().replace(/\s+/g, ' '))
    expect(lines.length).toBeGreaterThan(0)
    for (const l of lines) expect(l).toMatch(/\(через /)
    const people = lines.join(' | ')
    expect(people).toContain('Иван Краник, Пётр')
  })

  it('прямые ссылки: событие и поля-ссылки после людей, по дате начала; links шаблона выключает людей', async () => {
    const sch = schema
    const friend = sch.templates.find(x => x.key === 'kin.core.friend')!
    const custom = [
      { key: 'godfather', type: 'ref' as const, name: 'Крёстный' },
      { key: 'school', type: 'ref' as const, name: 'Школа' },
      { key: 'alma', type: 'ref' as const, name: 'Альма-матер' },
    ]
    const all = records(sch, [
      ...specs.slice(0, 3),
      { id: 'sch', template: 'kin.core.organization', fields: { name: ['Школа № 5'] }, start: { y: 1990 } },
      { id: 'pair', template: 'kin.core.relationship', parts: [['partner', 'anna'], ['partner', 'ivan']] },
      { id: 'wedding', template: 'kin.core.wedding', start: { y: 2021, m: 6 }, parts: [['partner', 'anna'], ['partner', 'ivan']] },
      { id: 'fr', template: 'kin.core.friend', start: { y: 2000 }, parts: [['friend', 'anna'], ['friend', 'petr']] },
      { id: 'anna2', template: 'kin.core.person', fields: { name: ['Анна'], 'custom.godfather': ['petr'], 'custom.school': ['sch'], 'custom.alma': ['sch'] }, custom },
    ])
    // ссылки — у отдельной записи без связей с Петром, чтобы не путать группы
    for (const k of ['pair', 'wedding', 'fr']) all[k]!.participants.forEach(p => { if (p.entity_id === 'anna') p.entity_id = 'anna2' })
    all.anna2!.participates_in = ['pair', 'wedding', 'fr'].map(c => ({ container_id: c, ...all[c]!.participants.find(p => p.entity_id === 'anna2')! }))
    friend.definition = { ...friend.definition, links: false }
    try {
      const me = { ...all.anna2!, revs: { start: 0, end: 0, note: 0, title: 0, sensitive: 0, groups: 0, custom_fields: 0 } }
      const related = Object.fromEntries(Object.entries(all).filter(([k]) => k !== 'anna2'))
      serve((m, u) => (m === 'GET' && u === '/api/records/anna2' ? [200, { record: me, related }] : undefined))
      const w = await mountRecord('anna2')
      const lines = w.findAll('.relations .list.current .relation').map(r => r.text().replace(/\s+/g, ' '))
      expect(lines).toEqual([
        'Партнёр: Иван Краник (через Пара)',
        'Школа: Школа № 5',
        'Альма-матер: Школа № 5', // та же запись в другом поле — своя строка
        'Дружба',
        'Свадьба · Анна, Иван Краник · 2021',
        'Крёстный: Пётр',
      ])
      expect(w.find('.relations .relation.link [href="/d/d1/r/petr"]').exists()).toBe(true)
    } finally {
      delete (friend.definition as { links?: boolean }).links
    }
  })
})
