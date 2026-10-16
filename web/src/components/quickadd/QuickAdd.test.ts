import { flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { records, type RecSpec } from '@/core/__tests__/kit'
import type { RecordDTO } from '@/core/types'
import { mountRecord, schema, serve } from '@/views/__tests__/card'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const liza: RecSpec = {
  id: 'liza', template: 'kin.core.person',
  fields: { name: ['Лиза'], surname: ['Василькова', 'Давыдко'], gender: ['female'] },
}
const father: RecSpec = { id: 'f1', template: 'kin.core.person', fields: { name: ['Сергей'], surname: ['Давыдко'], gender: ['male'] } }

function state(specs: RecSpec[]): { record: RecordDTO; related: Record<string, RecordDTO> } {
  const all = records(schema, specs)
  const record = { ...all.liza!, revs: { start: 0, end: 0, note: 0, title: 0, sensitive: 0, groups: 0, custom_fields: 0 } }
  return { record, related: Object.fromEntries(Object.entries(all).filter(([k]) => k !== 'liza')) }
}

const menuItems = () => [...document.body.querySelectorAll('.kin-popover .item')] as HTMLButtonElement[]

describe('быстрое добавление (+)', () => {
  it('«Добавить отца»: фамилия при рождении, пол предзаполнен, одна операция; после — пункт скрыт', async () => {
    let current = state([liza])
    const calls = serve((m, u) => {
      if (m === 'GET' && u === '/api/records/liza') return [200, current]
      if (m === 'POST' && u === '/api/dashboards/d1/operations') {
        current = state([liza, father, { id: 'pc', template: 'kin.core.parent_child', parts: [['parent', 'f1'], ['child', 'liza']] }])
        return [201, { operation_id: 'op', ids: { new: 'f1', rel: 'pc' }, records: [], related: {}, errors: [] }]
      }
      return undefined
    })
    const w = await mountRecord()
    await w.find('.quick-add').trigger('click')
    await flushPromises()
    const items = menuItems()
    expect(items.filter(b => !b.disabled).map(b => b.textContent?.trim())).toEqual(
      expect.arrayContaining(['Добавить отца', 'Добавить мать', 'Добавить ребёнка']),
    )
    expect(items.map(b => b.textContent?.trim())).not.toContain('Добавить брата') // F-43: в паке нет пункта
    items.find(b => b.textContent?.trim() === 'Добавить отца')!.click()
    await flushPromises()

    const form = document.body.querySelector('.quick-form')!
    const surname = form.querySelector('input[aria-label="Фамилия"]') as HTMLInputElement
    expect(surname.value).toBe('Давыдко')
    expect((form.querySelector('[aria-label="Пол"]') as HTMLSelectElement).value).toBe('male') // пол предзаполнен пунктом, его можно поправить (F-124)
    const name = form.querySelector('input[aria-label="Имя"]') as HTMLInputElement
    name.value = 'Сергей'
    name.dispatchEvent(new Event('input'))
    ;(form.querySelector('.submit') as HTMLButtonElement).click()
    await flushPromises()

    const ops = calls.filter(c => c.url.endsWith('/operations'))
    expect(ops).toHaveLength(1)
    expect(ops[0]!.body).toEqual({
      records: [
        { ref: 'new', template: 'kin.core.person', fields: { gender: [{ value: 'male' }], name: [{ value: 'Сергей' }], surname: [{ value: 'Давыдко' }] } },
        { ref: 'rel', template: 'kin.core.parent_child' },
      ],
      participants: [
        { container: 'rel', entity: 'liza', role: 'child' },
        { container: 'rel', entity: 'new', role: 'parent' },
      ],
    })

    await w.find('.quick-add').trigger('click')
    await flushPromises()
    const after = menuItems()
    expect(after.find(b => b.textContent?.trim() === 'Добавить отца')!.disabled).toBe(true)
    expect(after.find(b => b.textContent?.trim() === 'Добавить мать')!.disabled).toBe(false)
  })

  it('«Существующий»: поиск по ФИО, выбор — только отношение с выбранным (F-130)', async () => {
    const current = state([liza])
    const fRec = records(schema, [father]).f1!
    const calls = serve((m, u) => {
      if (m === 'GET' && u === '/api/records/liza') return [200, current]
      if (m === 'GET' && u.startsWith('/api/dashboards/d1/tree')) return [200, { relations: [], records: [] }]
      if (m === 'GET' && u.startsWith('/api/dashboards/d1/records')) return [200, { items: [fRec, current.record], total: 2, related: {} }]
      if (m === 'POST' && u === '/api/dashboards/d1/operations') return [201, { operation_id: 'op', ids: { rel: 'pc' }, records: [], related: {}, errors: [] }]
      return undefined
    })
    const w = await mountRecord()
    await w.find('.quick-add').trigger('click')
    await flushPromises()
    menuItems().find(b => b.textContent?.trim() === 'Добавить отца')!.click()
    await flushPromises()
    const form = document.body.querySelector('.quick-form')!
    ;(form.querySelector('[data-testid="mode-existing"]') as HTMLInputElement).click()
    await flushPromises()
    expect(form.querySelector('input[aria-label="Имя"]')).toBeNull()
    const submit = form.querySelector('.submit') as HTMLButtonElement
    expect(submit.disabled).toBe(true)
    vi.useFakeTimers()
    const q = form.querySelector('[data-testid="existing-search"]') as HTMLInputElement
    q.value = 'Серг'
    q.dispatchEvent(new Event('input'))
    await vi.advanceTimersByTimeAsync(300)
    vi.useRealTimers()
    await flushPromises()
    expect(calls.some(c => c.url.includes('/records?') && decodeURIComponent(c.url).includes('q=серг'))).toBe(true)
    const radios = [...form.querySelectorAll('[data-testid="candidate"]')] as HTMLButtonElement[]
    expect(radios.map(r => r.dataset.id)).toEqual(['f1']) // сама запись не предлагается
    radios[0]!.click()
    await flushPromises()
    expect(radios[0]!.classList.contains('on')).toBe(true) // выбранная подсвечена
    submit.click()
    await flushPromises()
    const ops = calls.filter(c => c.url.endsWith('/operations'))
    expect(ops.map(c => c.body)).toEqual([{
      records: [{ ref: 'rel', template: 'kin.core.parent_child' }],
      participants: [
        { container: 'rel', entity: 'liza', role: 'child' },
        { container: 'rel', entity: 'f1', role: 'parent' },
      ],
    }])
  })
})

describe('«Существующий»: поиск без фильтра по полу пункта (F-130)', () => {
  it('«Добавить мать» находит и мужчину, и человека без пола; пол в операции не пишется', async () => {
    const current = state([liza])
    const man = records(schema, [father]).f1!
    const nobody = records(schema, [{ id: 'n1', template: 'kin.core.person', fields: { name: ['Серго'] } }]).n1!
    const calls = serve((m, u) => {
      if (m === 'GET' && u === '/api/records/liza') return [200, current]
      if (m === 'GET' && u.startsWith('/api/dashboards/d1/tree')) return [200, { relations: [], records: [] }]
      if (m === 'GET' && u.startsWith('/api/dashboards/d1/records')) return [200, { items: [man, nobody], total: 2, related: {} }]
      if (m === 'POST' && u === '/api/dashboards/d1/operations') return [201, { operation_id: 'op', ids: { rel: 'pc' }, records: [], related: {}, errors: [] }]
      return undefined
    })
    const w = await mountRecord()
    await w.find('.quick-add').trigger('click')
    await flushPromises()
    menuItems().find(b => b.textContent?.trim() === 'Добавить мать')!.click()
    await flushPromises()
    const form = document.body.querySelector('.quick-form')!
    ;(form.querySelector('[data-testid="mode-existing"]') as HTMLButtonElement).click()
    await flushPromises()
    vi.useFakeTimers()
    const q = form.querySelector('[data-testid="existing-search"]') as HTMLInputElement
    q.value = 'Серг'
    q.dispatchEvent(new Event('input'))
    await vi.advanceTimersByTimeAsync(300)
    vi.useRealTimers()
    await flushPromises()
    const ids = [...form.querySelectorAll('[data-testid="candidate"]')].map(b => (b as HTMLElement).dataset.id)
    expect(ids).toEqual(['f1', 'n1'])
    ;(form.querySelector('[data-id="f1"]') as HTMLButtonElement).click()
    await flushPromises()
    ;(form.querySelector('.submit') as HTMLButtonElement).click()
    await flushPromises()
    const op = calls.find(c => c.url.endsWith('/operations'))!.body as { records: { ref: string }[] }
    expect(op.records.map(r => r.ref)).toEqual(['rel']) // новой записи (с полом) нет
  })
})
