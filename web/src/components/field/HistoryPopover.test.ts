import { flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { RecSpec } from '@/core/__tests__/kit'
import type { RecordDTO, ValueDTO } from '@/core/types'
import { envelope, mountRecord, serve, typeAndBlur } from '@/views/__tests__/card'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

type Body = Record<string, unknown> & { if_rev: number }

/** Сервер значений в памяти: порядок по позициям, версия поля, 409 при чужой версии. */
function fakeServer(spec: RecSpec) {
  const rec: RecordDTO = envelope(spec)
  let seq = 0
  const out = () => [200, { record: structuredClone(rec), related: {} }] as [number, unknown]
  const calls = serve((m, u, b) => {
    if (m === 'GET' && u === `/api/records/${rec.id}`) return out()
    const hit = /^\/api\/records\/[^/]+\/fields\/([^/]+)\/values(?:\/([^?]+))?(?:\?if_rev=(\d+))?$/.exec(u)
    if (!hit) return undefined
    const key = decodeURIComponent(hit[1]!)
    const field = (rec.fields[key] ??= { rev: 0, values: [] })
    const body = (b ?? { if_rev: Number(hit[3]) }) as Body
    if (body.if_rev !== field.rev) return [409, { error: 'conflict', field: key }]
    const list = field.values
    const place = (v: ValueDTO) => {
      if (typeof body.before_id === 'string') list.splice(list.findIndex(x => x.id === body.before_id), 0, v)
      else if (typeof body.after_id === 'string') list.splice(list.findIndex(x => x.id === body.after_id) + 1, 0, v)
      else if (body.position === 'bottom') list.push(v)
      else list.unshift(v)
    }
    if (m === 'POST') {
      place({
        id: `e${++seq}`, value: body.value, valid_from: (body.valid_from as never) ?? null, valid_to: null,
        note: (body.note as string) ?? null, meta: (body.meta as never) ?? {}, links: (body.links as never) ?? [],
      })
    } else {
      const i = list.findIndex(x => x.id === hit[2])
      const [v] = list.splice(i, 1)
      if (m === 'PATCH') {
        Object.assign(v!, Object.fromEntries(['value', 'note', 'meta', 'links', 'valid_from', 'valid_to']
          .filter(k => k in body).map(k => [k, body[k]])))
        if (body.before_id || body.after_id || body.position) place(v!)
        else list.splice(i, 0, v!)
      }
    }
    field.rev++
    return out()
  })
  return { rec, calls }
}

const body = () => document.body

function popover(): HTMLElement {
  const el = body().querySelector<HTMLElement>('.history')
  if (!el) throw new Error('history popover is closed')
  return el
}

function items(): string[] {
  return [...popover().querySelectorAll('.item .text')].map(x => x.textContent ?? '')
}

async function click(el: Element | null | undefined) {
  if (!el) throw new Error('no element')
  ;(el as HTMLElement).click()
  await flushPromises()
}

async function fill(value: string, note = '') {
  const form = popover().querySelector('form')!
  const input = form.querySelector<HTMLInputElement>('input')!
  input.value = value
  input.dispatchEvent(new Event('input'))
  if (note) {
    const noteInput = [...form.querySelectorAll<HTMLInputElement>('.kin-label input')].at(-1)!
    noteInput.value = note
    noteInput.dispatchEvent(new Event('input'))
  }
  await flushPromises()
  form.dispatchEvent(new Event('submit'))
  await flushPromises()
}

describe('история значения', () => {
  it('сценарий «фамилии Лизы»: порядок e1, e4, e3, e2', async () => {
    const { rec, calls } = fakeServer({ id: 'liza', template: 'kin.core.person', fields: { name: ['Лиза'] } })
    const w = await mountRecord('liza', true)
    const surname = w.findAll('.field-row').find(r => r.find('.label').text() === 'Фамилия')!

    // 1. «Василькова» — в поле ввода пустого поля (режим редактирования)
    await typeAndBlur(surname.find('input'), 'Василькова')

    const row = () => w.findAll('.field-row').find(r => r.find('.label').text() === 'Фамилия')!
    await row().find('.history-trigger').trigger('click')
    await flushPromises()
    // 2. девичья «Давыдко» — вниз
    await click(popover().querySelector('.add-bottom'))
    await fill('Давыдко', 'девичья')
    // 3. «Краник» — между: над «Давыдко»
    await click(popover().querySelectorAll('.item')[1]!.querySelector('[aria-label="Вставить выше"]'))
    await fill('Краник')
    // 4. снова «Давыдко» — между Васильковой и Краник
    await click(popover().querySelectorAll('.item')[1]!.querySelector('[aria-label="Вставить выше"]'))
    await fill('Давыдко')

    expect(items()).toEqual(['Василькова', 'Давыдко', 'Краник', 'Давыдко'])
    expect(rec.fields.surname!.values.map(v => v.id)).toEqual(['e1', 'e4', 'e3', 'e2'])
    expect(rec.fields.surname!.values[3]!.note).toBe('девичья')
    const writes = calls.filter(c => c.method !== 'GET')
    expect(writes.map(c => [c.method, (c.body as Body).if_rev])).toEqual([['POST', 0], ['POST', 1], ['POST', 2], ['POST', 3]])
    expect(writes[2]!.body).toMatchObject({ before_id: 'e2' })
    expect(writes[3]!.body).toMatchObject({ before_id: 'e3' })
    expect(popover().querySelectorAll('.badge')).toHaveLength(1)
  })

  it('переместить, сделать основным, удалить', async () => {
    const { rec } = fakeServer({ id: 'liza', template: 'kin.core.person', fields: { surname: ['А', 'Б', 'В'] } })
    const w = await mountRecord()
    const row = () => w.findAll('.field-row').find(r => r.find('.label').text() === 'Фамилия')!
    await row().find('.history-trigger').trigger('click')
    await flushPromises()
    await click(popover().querySelectorAll('.item')[2]!.querySelector('.primary'))
    expect(items()).toEqual(['В', 'А', 'Б'])
    expect(row().find('.values').text()).toBe('В') // текущее — верхнее
    await click(popover().querySelectorAll('.item')[0]!.querySelector('[aria-label="Ниже"]'))
    expect(items()).toEqual(['А', 'В', 'Б'])
    await click(popover().querySelectorAll('.item')[2]!.querySelector('[aria-label="Удалить"]'))
    expect(items()).toEqual(['А', 'В'])
    expect(rec.fields.surname!.rev).toBe(6)
  })

  it('meta по editor: цвет из палитры шаблона', async () => {
    const { calls } = fakeServer({ id: 'liza', template: 'kin.core.person', fields: { name: ['Лиза'] } })
    const w = await mountRecord()
    const row = w.findAll('.field-row').find(r => r.find('.label').text() === 'Имя')!
    await row.find('.history-trigger').trigger('click')
    await flushPromises()
    await click(popover().querySelector('.edit'))
    const swatches = popover().querySelectorAll('.swatch')
    expect(swatches.length).toBeGreaterThan(0)
    await click(swatches[1])
    popover().querySelector('form')!.dispatchEvent(new Event('submit'))
    await flushPromises()
    const patch = calls.find(c => c.method === 'PATCH')!
    expect(patch.body).toMatchObject({ value: 'Лиза', meta: { bColor: swatches[1]!.getAttribute('aria-label') } })
  })

  it('этап: выбор из этапов шаблона, подсказка next, своё название', async () => {
    const { calls } = fakeServer({ id: 'pair', template: 'kin.core.relationship', fields: { stage: ['dating'] } })
    const w = await mountRecord('pair')
    const stageRow = w.find('.stage-row')
    expect(stageRow.text()).toContain('Отношения')
    await stageRow.find('.history-trigger').trigger('click')
    await flushPromises()
    await click(popover().querySelector('.add-top'))
    const hint = [...popover().querySelectorAll('.hint button')].map(b => b.textContent?.trim())
    expect(hint.length).toBeGreaterThan(0)
    await click(popover().querySelector('.hint button'))
    popover().querySelector('form')!.dispatchEvent(new Event('submit'))
    await flushPromises()
    const post = calls.find(c => c.method === 'POST')!
    expect(post.url).toBe('/api/records/pair/fields/stage/values')
    expect(post.body).toMatchObject({ position: 'top' })
    expect(w.find('.stage-row').text()).toContain(hint[0]!)

    // своё название — текстом (F-54)
    await click(popover().querySelector('.add-top'))
    const select = popover().querySelector<HTMLSelectElement>('form select')!
    select.value = '*'
    select.dispatchEvent(new Event('change'))
    await flushPromises()
    const free = popover().querySelector<HTMLInputElement>('form input')!
    free.value = 'Дружба'
    free.dispatchEvent(new Event('input'))
    popover().querySelector('form')!.dispatchEvent(new Event('submit'))
    await flushPromises()
    expect(calls.filter(c => c.method === 'POST').at(-1)!.body).toMatchObject({ value: 'Дружба' })
    expect(w.find('.stage-row').text()).toContain('Дружба')
  })
})
