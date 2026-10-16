import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { records, ruSchema } from '@/core/__tests__/kit'
import { buildRegistry } from '@/core/registry'

import CreateRelationDialog from './CreateRelationDialog.vue'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const schema = ruSchema()
const reg = buildRegistry(schema)
const people = records(schema, [
  { id: 'anna', template: 'kin.core.person', fields: { name: ['Анна'], surname: ['Василькова'], gender: ['female'] } },
  { id: 'ivan', template: 'kin.core.person', fields: { name: ['Иван'], surname: ['Краник'], gender: ['male'] } },
  { id: 'org', template: 'kin.core.organization', fields: { name: ['Завод'] } },
])

function serve() {
  const calls: { method: string; url: string; body: unknown }[] = []
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    const method = init?.method ?? 'GET'
    calls.push({ method, url, body: init?.body ? JSON.parse(String(init.body)) : undefined })
    if (url.includes('/records')) return Response.json({ items: Object.values(people), total: 3, related: {} })
    if (url.endsWith('/operations')) {
      return Response.json({ operation_id: 'o', ids: { new: 'w1' }, records: [], related: {}, errors: [] }, { status: 201 })
    }
    return Response.json({ error: 'not_found' }, { status: 404 })
  }))
  return calls
}

const button = (cls: string) => document.body.querySelector(`.create-relation ${cls}`) as HTMLButtonElement

describe('CreateRelationDialog', () => {
  it('Свадьба: без двух партнёров создать нельзя; одна пакетная операция', async () => {
    const calls = serve()
    const wedding = reg.templates.get('kin.core.wedding')!.definition
    const w = mount(CreateRelationDialog, {
      props: {
        open: true, dashboard: 'd1', reg, template: wedding,
        preset: { id: 'anna', title: 'Анна Василькова', template: reg.templates.get('kin.core.person')!.definition },
      },
      attachTo: document.body,
    })
    await flushPromises()
    expect(document.body.textContent).toContain('Анна Василькова')
    expect(button('.submit').disabled).toBe(true)

    // первая роль — «Вступающий в брак»: добавить Ивана
    button('.add').click()
    await flushPromises()
    await new Promise(r => setTimeout(r, 0))
    const results = [...document.body.querySelectorAll('.create-relation .results button')] as HTMLButtonElement[]
    const titles = results.map(b => b.textContent ?? '')
    expect(titles.some(x => x.includes('Завод'))).toBe(false) // организация в роль не годится
    expect(titles.some(x => x.includes('Анна'))).toBe(false) // уже выбрана в этой роли
    results.find(b => b.textContent?.includes('Иван'))!.click()
    await flushPromises()
    expect(button('.submit').disabled).toBe(false)
    expect(button('.add').disabled).toBe(true) // max 2

    button('.submit').click()
    await flushPromises()
    const op = calls.find(c => c.url.endsWith('/operations'))!
    expect(op.body).toEqual({
      records: [{ ref: 'new', template: 'kin.core.wedding' }],
      participants: [
        { container: 'new', entity: 'anna', role: 'partner' },
        { container: 'new', entity: 'ivan', role: 'partner' },
      ],
      strict: true,
    })
    expect(w.emitted('created')).toEqual([['w1']])
  })

  it('Пара: этап пишется значением поля этапа', async () => {
    const calls = serve()
    const pair = reg.templates.get('kin.core.relationship')!.definition
    mount(CreateRelationDialog, { props: { open: true, dashboard: 'd1', reg, template: pair }, attachTo: document.body })
    await flushPromises()
    const select = document.body.querySelector('.create-relation select') as HTMLSelectElement
    const options = [...select.options].map(o => o.textContent?.trim())
    expect(options).toContain('Брак')
    expect(options).not.toContain('') // этап default без названия не предлагается — только «—»
    expect(button('.submit').disabled).toBe(true)
    expect(calls.some(c => c.url.endsWith('/operations'))).toBe(false)
  })

  it('новый участник прямо в диалоге — вместе со связью одной операцией (F-87)', async () => {
    const calls = serve()
    const wedding = reg.templates.get('kin.core.wedding')!.definition
    mount(CreateRelationDialog, {
      props: {
        open: true, dashboard: 'd1', reg, template: wedding,
        preset: { id: 'anna', title: 'Анна Василькова', template: reg.templates.get('kin.core.person')!.definition },
      },
      attachTo: document.body,
    })
    await flushPromises()
    button('.new').click()
    await flushPromises()
    // пусто — не добавить (F-88)
    button('.add-new').click()
    await flushPromises()
    expect(document.body.querySelector('.new-participant [role=alert]')?.textContent).toContain('Заполните')
    const name = document.body.querySelector('.new-participant input[aria-label="Имя"]') as HTMLInputElement
    name.value = 'Пётр'
    name.dispatchEvent(new Event('input'))
    button('.add-new').click()
    await flushPromises()
    expect(document.body.querySelector('.new-participant')).toBeNull()
    expect(document.body.textContent).toContain('Пётр')
    expect(button('.submit').disabled).toBe(false)
    button('.submit').click()
    await flushPromises()
    const op = calls.find(c => c.url.endsWith('/operations'))!
    expect(op.body).toMatchObject({
      records: [{ ref: 'new', template: 'kin.core.wedding' }, { ref: 'p1', template: 'kin.core.person', fields: { name: [{ value: 'Пётр' }] } }],
      participants: [
        { container: 'new', entity: 'anna', role: 'partner' },
        { container: 'new', entity: 'p1', role: 'partner' },
      ],
    })
  })
})
