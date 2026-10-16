import { flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { RecSpec } from '@/core/__tests__/kit'
import type { RecordDTO, TemplateDef } from '@/core/types'
import { messagesFor } from '@/i18n'
import { getDraft, newDraft } from '@/ui/drafts'

import { envelope, mountRecord, schema, serve, typeAndBlur } from './__tests__/card'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const liza: RecSpec = {
  id: 'liza', template: 'kin.core.person', note: 'старое',
  start: { m: 3, d: 12 },
  fields: { name: ['Лиза'], surname: ['Василькова'] },
}

describe('карточка: каркас', () => {
  it('подпись, даты из period, разделы и скрытые пустые поля за «Добавить»', async () => {
    serve((m, u) => (m === 'GET' && u === '/api/records/liza' ? [200, { record: envelope(liza), related: {} }] : undefined))
    const w = await mountRecord()
    expect(w.find('h1').text()).toContain('Лиза Василькова')
    const dates = w.findAll('.sys-row').map(r => r.find('.label').text())
    expect(dates).toEqual(['Дата рождения']) // пустая «Дата смерти» скрыта (F-99)
    expect(w.find('.sys-row .value').text()).toBe('12 марта')
    expect(w.findAll('.section-title').map(x => x.text())).toEqual(['Основные', 'Дополнительные'])
    const main = w.findAll('.field-row .label').map(x => x.text())
    expect(main).toContain('Фамилия')
    expect(main).not.toContain('Отчество') // show: filled и пусто
    expect(w.find('.add-field').exists()).toBe(false) // режим чтения (F-85)
    expect(w.find('.field-row input').exists()).toBe(false)
    await w.find('.mode').trigger('click')
    await flushPromises()
    expect(w.find('.field-row input').exists()).toBe(true) // поля сразу открыты для ввода
    await w.find('.add-field').trigger('click')
    await flushPromises()
    const add = [...document.body.querySelectorAll('.add-list button')].map(b => b.textContent?.trim())
    expect(add).toContain('Отчество')
    ;(document.body.querySelector('.add-list button') as HTMLButtonElement).click()
    await flushPromises()
    expect(w.findAll('.field-row .label').map(x => x.text())).toContain(add[0])

    // скрытая дата — через свою «Добавить» (F-99)
    await w.find('.add-date').trigger('click')
    await flushPromises()
    const addDate = [...document.body.querySelectorAll('.add-list button')].map(b => b.textContent?.trim())
    expect(addDate).toEqual(['Дата смерти', 'Умер']) // признак завершения (F-126) — за той же кнопкой
    ;(document.body.querySelectorAll('.add-list button')[0] as HTMLButtonElement).click()
    await flushPromises()
    expect(w.findAll('.sys-row').map(r => r.find('.label').text())).toEqual(['Дата рождения', 'Дата смерти'])
    // «Умер» — в шапке, не в «Основных»
    expect(w.findAll('.head-block .field-row .label').map(x => x.text())).not.toContain('Умер')
    await w.find('.add-date').trigger('click')
    await flushPromises()
    ;(document.body.querySelector('.add-list button') as HTMLButtonElement).click()
    await flushPromises()
    expect(w.findAll('.basics .field-row .label').map(x => x.text())).toContain('Умер')
  })

  it('409: поле обновляется до текущего значения и показывается сообщение', async () => {
    let current = envelope(liza)
    const calls = serve((m, u) => {
      if (m === 'GET' && u === '/api/records/liza') return [200, { record: current, related: {} }]
      if (m === 'PATCH' && u === '/api/records/liza') {
        current = envelope({ ...liza, note: 'чужое' }, { note: 1 })
        return [409, { error: 'conflict', field: 'note', message: 'x', current: 'чужое' }]
      }
      return undefined
    })
    const w = await mountRecord()
    expect(w.find('.note .text').text()).toContain('старое')
    await w.find('.mode').trigger('click')
    await typeAndBlur(w.find('.note textarea'), 'моё')
    const patch = calls.find(c => c.method === 'PATCH')!
    expect(patch.body).toEqual({ if_rev: 0, note: 'моё' })
    expect(w.find('.conflict').text()).toBe('Поле уже изменили в другом месте — показано текущее значение')
    expect((w.find('.note textarea').element as HTMLTextAreaElement).value).toBe('чужое') // поле обновлено
  })

  it('ошибка проверки поля — сообщение у поля, запись остаётся', async () => {
    serve((m, u) => {
      if (m === 'GET' && u === '/api/records/liza') return [200, { record: envelope(liza), related: {} }]
      if (m === 'PATCH' && u === '/api/records/liza') return [400, { error: 'validation', field: 'note', message: 'слишком длинно' }]
      return undefined
    })
    const w = await mountRecord('liza', true)
    await typeAndBlur(w.find('.note textarea'), 'моё')
    expect(w.find('.note [role=alert]').text()).toBe('слишком длинно')
    expect(w.find('.conflict').exists()).toBe(false)
  })
})

describe('карточка: корзина и изменения (T25)', () => {
  it('в корзину → восстановить; вкладка «Изменения» — журнал записи', async () => {
    let deleted = false
    const calls = serve((m, u) => {
      const rec = () => ({ record: { ...envelope(liza), deleted }, related: {} })
      if (m === 'GET' && u === '/api/records/liza') return [200, rec()]
      if (m === 'DELETE' && u === '/api/records/liza') {
        deleted = true
        return [200, rec()]
      }
      if (m === 'POST' && u === '/api/records/liza/restore') {
        deleted = false
        return [200, rec()]
      }
      if (u.startsWith('/api/dashboards/d1/groups')) return [200, []]
      if (u.startsWith('/api/dashboards/d1/audit')) {
        return [200, {
          operations: [{
            operation_id: 'o1', at: '2026-10-11T10:00:00Z', actor_id: 'a', actor_login: 'owner', source: 'ui',
            request_id: 'r', app_version: 'dev', schema_version: 1,
            entries: [{ id: 'e1', at: '2026-10-11T10:00:00Z', entity_id: 'liza', object: 'entity', object_id: 'liza', op: 'delete', before: null, after: null }],
          }],
          next: null, records: { liza: envelope(liza) }, related: {},
        }]
      }
      return undefined
    })
    const w = await mountRecord()
    await w.find('.to-trash').trigger('click')
    await flushPromises()
    expect(w.text()).toContain('Запись в корзине')
    expect(w.find('.to-trash').exists()).toBe(false)
    await w.find('.kin-notice--error button').trigger('click')
    await flushPromises()
    expect(calls.map(c => `${c.method} ${c.url}`)).toContain('POST /api/records/liza/restore')
    expect(w.find('.to-trash').exists()).toBe(true)

    const tabs = w.findAll('.kin-tab')
    await tabs[1]!.trigger('mousedown', { button: 0 })
    await flushPromises()
    expect(calls.some(c => c.url.startsWith('/api/dashboards/d1/audit?entity=liza'))).toBe(true)
    expect(w.find('[data-test="audit-op"]').text()).toContain('в корзину')
  })
})

describe('карточка: свои поля (T29, F-75)', () => {
  it('добавить поле → описание в custom_fields, значение под custom.<key>, правка описания', async () => {
    let current = envelope(liza)
    const calls = serve((m, u, body) => {
      if (m === 'GET' && u === '/api/records/liza') return [200, { record: current, related: {} }]
      if (m === 'PATCH' && u === '/api/records/liza') {
        const b = body as { custom_fields: RecordDTO['custom_fields'] }
        current = { ...current, custom_fields: b.custom_fields, revs: { ...current.revs, custom_fields: (current.revs?.custom_fields ?? 0) + 1 } }
        return [200, { record: current, related: {} }]
      }
      if (m === 'POST' && u === '/api/records/liza/fields/custom.army_unit/values') {
        const b = body as { value: string }
        current = { ...current, fields: { ...current.fields, 'custom.army_unit': { rev: 1, values: [{ id: 'cv', value: b.value, valid_from: null, valid_to: null, note: null, meta: {}, links: [] }] } } }
        return [200, { record: current, related: {} }]
      }
      return undefined
    })
    const w = await mountRecord('liza', true)
    await w.find('.add-custom').trigger('click')
    await flushPromises()
    const input = document.body.querySelector('input[name="custom-name"]') as HTMLInputElement
    input.value = 'Army unit'
    input.dispatchEvent(new Event('input'))
    ;(document.body.querySelector('[data-name="custom-history"]') as HTMLInputElement).click()
    await flushPromises()
    ;(document.body.querySelector('.custom-form form') as HTMLFormElement).dispatchEvent(new Event('submit'))
    await flushPromises()
    const patch = calls.find(c => c.method === 'PATCH')!
    expect(patch.body).toEqual({ if_rev: 0, custom_fields: [{ key: 'army_unit', type: 'text', name: 'Army unit', history: true }] })
    // новое поле — в разделе «Свои», сразу в правке
    expect(w.findAll('.section-title').map(x => x.text())).toContain(messagesFor('ru').sections.custom)
    const row = w.findAll('.field-row').find(r => r.find('.label').text() === 'Army unit')!
    await typeAndBlur(row.find('input'), 'в/ч 1234')
    expect(calls.some(c => c.method === 'POST' && (c.body as { value?: string }).value === 'в/ч 1234')).toBe(true)
    await w.find('.mode').trigger('click') // в режим чтения
    expect(w.text()).toContain('в/ч 1234')
  })
})

describe('карточка: общее своё поле шаблона (F-96)', () => {
  it('«Добавить всем записям шаблона» → PUT шаблона дашборда, поле у записи под shared.<key>', async () => {
    let current = envelope(liza)
    const tpl = schema.templates.find(x => x.id === current.template_id)!
    const calls = serve((m, u, body) => {
      if (m === 'GET' && u === '/api/records/liza') return [200, { record: current, related: {} }]
      if (m === 'PUT' && u === `/api/dashboards/d1/templates/${tpl.id}/custom_fields`) {
        const b = body as { custom_fields: NonNullable<TemplateDef['custom_fields']> }
        return [200, { ...tpl, rev: 1, definition: { ...tpl.definition, custom_fields: b.custom_fields } }]
      }
      if (m === 'POST' && u === '/api/records/liza/fields/shared.religion/values') {
        const b = body as { value: string }
        current = { ...current, fields: { ...current.fields, 'shared.religion': { rev: 1, values: [{ id: 'sv', value: b.value, valid_from: null, valid_to: null, note: null, meta: {}, links: [] }] } } }
        return [200, { record: current, related: {} }]
      }
      return undefined
    })
    const w = await mountRecord('liza', true)
    await w.find('.add-custom').trigger('click')
    await flushPromises()
    const input = document.body.querySelector('input[name="custom-name"]') as HTMLInputElement
    input.value = 'Religion'
    input.dispatchEvent(new Event('input'))
    ;(document.body.querySelector('[data-name="custom-to-all"]') as HTMLInputElement).click()
    await flushPromises()
    ;(document.body.querySelector('.custom-form form') as HTMLFormElement).dispatchEvent(new Event('submit'))
    await flushPromises()
    const put = calls.find(c => c.method === 'PUT')!
    expect(put.body).toEqual({ if_rev: 0, custom_fields: [{ key: 'religion', type: 'text', name: 'Religion' }] })
    expect(calls.some(c => c.method === 'PATCH')).toBe(false)
    const row = w.findAll('.field-row').find(r => r.find('.label').text() === 'Religion')!
    await typeAndBlur(row.find('input'), 'нет')
    expect(calls.some(c => c.method === 'POST' && c.url.endsWith('/fields/shared.religion/values'))).toBe(true)
    expect(schema.templates.find(x => x.id === tpl.id)!.definition.custom_fields).toBeUndefined()
  })
})

describe('новая запись — черновик до первого поля (F-88)', () => {
  it('ничего на сервере, пока поле не заполнено; первое значение и связь — одной операцией', async () => {
    const id = newDraft({
      dashboard: 'd1', template: 'kin.core.person', groups: ['g1'],
      base: { fields: { surname: [{ value: 'Иванова' }] } },
      with: {
        records: [{ ref: 'rel', template: 'kin.core.parent_child' }],
        participants: [{ container: 'rel', entity: 'liza', role: 'child' }, { container: 'rel', entity: 'new', role: 'parent' }],
      },
    })
    const calls = serve((m, u) => {
      if (m === 'POST' && u === '/api/dashboards/d1/operations') {
        return [201, { operation_id: 'o', ids: { new: 'p1', rel: 'r1' }, records: [], related: {}, errors: [] }]
      }
      return undefined
    })
    const w = await mountRecord(id)
    expect(calls.filter(c => c.url.startsWith('/api/records'))).toHaveLength(0)
    expect(w.find('.draft-hint').exists()).toBe(true)
    expect(w.find('.mode').exists()).toBe(false) // сразу в режиме редактирования
    const name = w.findAll('.field-row').find(r => r.find('.label').text() === 'Имя')!
    await typeAndBlur(name.find('input'), '')
    expect(calls.some(c => c.method === 'POST')).toBe(false) // пустое — ничего
    await typeAndBlur(name.find('input'), 'Сергей')
    const op = calls.find(c => c.method === 'POST')!
    expect(op.body).toEqual({
      strict: true,
      records: [
        { ref: 'new', template: 'kin.core.person', groups: ['g1'], fields: { surname: [{ value: 'Иванова' }], name: [{ value: 'Сергей' }] } },
        { ref: 'rel', template: 'kin.core.parent_child' },
      ],
      participants: [{ container: 'rel', entity: 'liza', role: 'child' }, { container: 'rel', entity: 'new', role: 'parent' }],
    })
    expect(getDraft(id)).toBeUndefined()
  })

  it('очищенное предзаполненное не уходит с первым значением', async () => {
    const id = newDraft({ dashboard: 'd1', template: 'kin.core.person', groups: [], base: { fields: { surname: [{ value: 'Иванова' }] } } })
    const calls = serve((m, u) => (m === 'POST' && u === '/api/dashboards/d1/operations'
      ? [201, { operation_id: 'o', ids: { new: 'p1' }, records: [], related: {}, errors: [] }]
      : undefined))
    const w = await mountRecord(id)
    const row = (label: string) => w.findAll('.field-row').find(r => r.find('.label').text() === label)!
    await typeAndBlur(row('Фамилия').find('input'), '')
    expect((row('Фамилия').find('input').element as HTMLInputElement).value).toBe('')
    await typeAndBlur(row('Имя').find('input'), 'Сергей')
    const op = calls.find(c => c.method === 'POST')!
    expect(op.body).toMatchObject({ records: [{ ref: 'new', fields: { name: [{ value: 'Сергей' }] } }] })
    expect((op.body as { records: { fields: object }[] }).records[0]!.fields).not.toHaveProperty('surname')
  })

  it('черновик потерян (обновили страницу) — сообщение, без запросов записи', async () => {
    const calls = serve(() => undefined)
    const w = await mountRecord('draft-gone')
    expect(w.text()).toContain(messagesFor('ru').card.draftLost)
    expect(calls.filter(c => c.url.startsWith('/api/records'))).toHaveLength(0)
  })
})
