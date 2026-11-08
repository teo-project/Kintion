import { describe, expect, it } from 'vitest'

import type { AuditEntryDTO, AuditOperationDTO } from '@/api/types'
import { ctxOf, ruSchema } from '@/core/__tests__/kit'
import { messagesFor } from '@/i18n'

import { auditCtx, entryView, operationView } from './audit'

const m = messagesFor('ru')

function entry(e: Partial<AuditEntryDTO>): AuditEntryDTO {
  return { id: 'e', at: '2026-10-11T00:00:00Z', entity_id: null, object: 'entity', object_id: null, op: 'update', before: null, after: null, ...e }
}

function op(entries: AuditEntryDTO[]): AuditOperationDTO {
  return {
    operation_id: 'op', at: '2026-10-11T00:00:00Z', actor_id: 'a', actor_login: 'owner', source: 'ui',
    request_id: 'r', app_version: 'dev', schema_version: 1, entries,
  }
}

describe('журнал: было → стало', () => {
  const ctx = ctxOf(ruSchema(), [
    { id: 'anna', template: 'kin.core.person', fields: { name: ['Анна'], surname: ['Василькова'], gender: ['female'] } },
    { id: 'ivan', template: 'kin.core.person', fields: { name: ['Иван'], gender: ['male'] } },
    { id: 'm', template: 'kin.core.relationship', fields: { stage: ['marriage'] }, parts: [['partner', 'anna'], ['partner', 'ivan']] },
  ])
  const a = auditCtx(ctx, m, [], [{ id: 'g1', name: 'Семья', count: 1, parents: [], children: ['g2'], auto_surname: false, surnames: [] }, { id: 'g2', name: 'Петровы', count: 0, parents: ['g1'], children: [], auto_surname: false, surnames: [] }])

  it('эры дашборда (F-143): настройки; каждая замена эры — своей записью с числом дат; эра в значениях видна всегда', () => {
    const v = entryView(a, entry({
      object: 'dashboard', before: { eras: { show: false, edit: false, default: 'ACC', order: [], today: null } },
      after: { eras: { show: false, edit: false, default: 'AC', order: ['AC', ''], today: { y: -5, e: 'AC' } } },
    }))
    expect(v.changes).toEqual([
      { label: 'Эра по умолчанию', before: 'ACC', after: 'AC' },
      { label: 'Хронологический порядок', before: '', after: 'AC → без эры' },
      { label: 'Текущий год мира', before: '', after: '5 до AC' },
    ])
    const r = entryView(a, entry({ object: 'dashboard', after: { replace: { from: 'ACC', to: 'AC', changed: 3, default_before: 'ACC', default_after: 'AC' } } }))
    expect(r.changes).toEqual([{ label: 'Эра в датах (изменено дат: 3)', before: 'ACC', after: 'AC' }])
    // пустая эра: при пустой эре по умолчанию — «н. э.», иначе — «без эры»
    const o = entryView(a, entry({ object: 'dashboard', after: { replace: { from: '', to: 'AC', changed: 2, default_before: '', default_after: 'AC' } } }))
    expect(o.changes).toEqual([{ label: 'Эра в датах (изменено дат: 2)', before: 'н. э.', after: 'AC' }])
    const n = entryView(a, entry({ object: 'dashboard', after: { replace: { from: 'BC', to: '', changed: 1, default_before: 'AC', default_after: 'AC' } } }))
    expect(n.changes).toEqual([{ label: 'Эра в датах (изменено дат: 1)', before: 'BC', after: 'без эры' }])
    const d = entryView(a, entry({ entity_id: 'anna', object: 'entity', before: { start_date: { y: 300, e: 'ACC' } }, after: { start_date: { y: 300, e: 'AC' } } }))
    expect(d.changes.map(c => [c.before, c.after])).toEqual([['300 ACC', '300 AC']])
  })

  it('вложенность группы (F-105): «Петровы · Входит в» → «Семья»', () => {
    const v = entryView(a, entry({ object: 'grp_link', op: 'create', after: { parent_id: 'g1', child_id: 'g2' } }))
    expect(v.changes).toEqual([{ label: 'Петровы · Входит в', before: '', after: 'Семья' }])
  })

  it('значение поля — с подписью из шаблона', () => {
    const v = entryView(a, entry({
      entity_id: 'anna', object: 'field_entry',
      before: { field: 'surname', value: 'Иванова', sort_key: 'a' },
      after: { field: 'surname', value: 'Василькова', sort_key: 'a' },
    }))
    expect(v.record).toBe('Анна Василькова')
    expect(v.changes).toEqual([{ label: 'Фамилия', before: 'Иванова', after: 'Василькова' }])
  })

  it('своё значение выбора → вариант с тем же текстом — изменение значения (F-104)', () => {
    const custom = [{ key: 'mood', type: 'choice' as const, name: 'Настроение', free: true, options: [{ key: 'fun', name: 'Весело' }] }]
    const c2 = ctxOf(ruSchema(), [{ id: 'p', template: 'kin.core.person', custom, fields: { name: ['Пётр'] } }])
    const v = entryView(auditCtx(c2, m, [], []), entry({
      entity_id: 'p', object: 'field_entry',
      before: { field: 'custom.mood', value: 'fun', meta: { free: true }, sort_key: 'a' },
      after: { field: 'custom.mood', value: 'fun', meta: {}, sort_key: 'a' },
    }))
    expect(v.changes).toContainEqual({ label: 'Настроение', before: 'fun', after: 'Весело' })
  })

  it('choice и этап — словами пака, перенос — «порядок»', () => {
    const v = entryView(a, entry({
      entity_id: 'm', object: 'field_entry', op: 'update',
      before: { field: 'stage', value: 'marriage', sort_key: 'b' },
      after: { field: 'stage', value: 'marriage', sort_key: 'a' },
    }))
    expect(v.changes).toEqual([{ label: 'Этап · порядок', before: '', after: 'Брак ↑ Выше' }])
    const c = entryView(a, entry({ entity_id: 'm', object: 'field_entry', op: 'create', after: { field: 'stage', value: 'marriage' } }))
    expect(c.changes[0]).toEqual({ label: 'Этап', before: '', after: 'Брак' })
  })

  it('участие — форма роли со стороны другого участника', () => {
    const v = entryView(a, entry({
      entity_id: 'm', object: 'participation', op: 'create',
      after: { container_id: 'm', entity_id: 'ivan', role: 'partner' },
    }))
    expect(v.changes).toEqual([{ label: 'Муж', before: '', after: 'Иван' }])
    expect(v.op).toBe('создание')
  })

  it('системные поля записи и метка группы', () => {
    const v = entryView(a, entry({
      entity_id: 'anna', object: 'entity', op: 'update',
      before: { template_id: 'row-kin.core.person', start_date: null, note: null, sensitive: false },
      after: { template_id: 'row-kin.core.person', start_date: { y: null, m: 3, d: 12 }, note: null, sensitive: true },
    }))
    expect(v.changes.map(c => c.label)).toEqual(['Дата рождения', 'Чувствительное'])
    expect(v.changes[0]!.after).toBe('12 марта')
    const g = entryView(a, entry({ entity_id: 'anna', object: 'entity_group', op: 'create', after: { entity_id: 'anna', group_id: 'g1' } }))
    expect(g.changes).toEqual([{ label: 'Группы', before: '', after: 'Семья' }])
  })

  it('свои поля: видны тип, история и несколько значений', () => {
    const def = { key: 'x', type: 'text', name: 'Часть' }
    const v = entryView(a, entry({
      entity_id: 'anna', object: 'entity',
      before: { custom_fields: [def] },
      after: { custom_fields: [{ ...def, history: true }] },
    }))
    expect(v.changes).toEqual([{ label: 'Свободные поля', before: 'Часть (Текст)', after: 'Часть (Текст, С историей значений)' }])
  })

  it('очищенная запись: шаблон из снимка на странице, иначе сырые ключи', () => {
    const create = entry({ entity_id: 'gone', object: 'entity', op: 'create', after: { template_id: 'row-kin.core.person' } })
    const val = entry({ id: 'e2', entity_id: 'gone', object: 'field_entry', op: 'create', after: { field: 'surname', value: 'Х' } })
    const withSnap = auditCtx(ctx, m, [op([create, val])], [])
    const v = operationView(withSnap, op([create, val]))
    expect(v.actor).toBe('owner')
    expect(v.details).toContain('ui')
    expect(v.details).toContain('dev')
    expect(v.entries[1]!.record).toBe(m.audit.unknownRecord)
    expect(v.entries[1]!.changes[0]!.label).toBe('Фамилия')
    const raw = entryView(a, val)
    expect(raw.changes[0]!.label).toBe('surname')
  })

  it('очищенная запись — последнее название по журналу (F-94), подписи полей по-прежнему', () => {
    const gone = ctxOf(ruSchema(), [{ id: 'gone', template: 'kin.core.person', fields: { name: ['Лиза'], surname: ['Х'] } }]).records.gone!
    const val = entry({ id: 'e3', entity_id: 'gone', object: 'field_entry', op: 'purge', before: { field: 'surname', value: 'Х' } })
    const withPurged = auditCtx(ctx, m, [op([val])], [], { gone })
    const v = entryView(withPurged, val)
    expect(v.record).toBe('Лиза Х')
    expect(v.purged).toBe(true)
    expect(v.changes[0]!.label).toBe('surname')
    expect(entryView(a, entry({ entity_id: 'anna' })).purged).toBe(false)
  })
})

describe('длинные значения в журнале (F-141)', () => {
  it('начало длинного значения с многоточием, короткое — как есть', async () => {
    const { clipText, CLIP } = await import('./audit')
    expect(clipText('коротко')).toBe('коротко')
    const long = 'а'.repeat(CLIP + 50)
    expect(clipText(long)).toBe(`${'а'.repeat(CLIP)}…`)
    expect(clipText('1\n2\n3\n4\n5')).toBe('1\n2\n3…')
  })
})

describe('свои блоки в журнале (F-139)', () => {
  it('блок поля — по названию из того же снимка; блоки записи — по названиям', () => {
    const ctx = ctxOf(ruSchema(), [])
    const a = auditCtx(ctx, m, [], [])
    const v = entryView(a, entry({
      object: 'entity',
      before: { custom_fields: [{ key: 'u', type: 'text', name: 'U', section: 'w' }], custom_sections: [{ key: 'w', name: 'Война' }] },
      after: { custom_fields: [{ key: 'u', type: 'text', name: 'U', section: 'additional' }], custom_sections: [{ key: 'w', name: 'Служба' }] },
    }))
    const fields = v.changes.find(c => c.before.includes('U'))!
    expect(fields.before).toContain('Война')
    expect(fields.after).toContain(m.sections.additional)
    expect(v.changes.find(c => c.before === 'Война')!.after).toBe('Служба')
  })
})
