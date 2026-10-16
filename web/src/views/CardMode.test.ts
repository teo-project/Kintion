import { flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { CardMode } from '@/api/types'
import type { RecSpec } from '@/core/__tests__/kit'

import { envelope, mountRecord, serve } from './__tests__/card'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const liza: RecSpec = { id: 'liza', template: 'kin.core.person', fields: { name: ['Лиза'], surname: ['Василькова'] } }

/** Карточка в режиме mode; writable — есть право на правку (в 0.1 — запись не в корзине). */
async function card(mode: CardMode, writable: boolean) {
  const calls = serve((m, u, b) => {
    if (m === 'GET' && u === '/api/me/prefs') return [200, { history_edit: 'replace', card_mode: mode }]
    if (m === 'GET' && u === '/api/records/liza') return [200, { record: { ...envelope(liza), deleted: !writable }, related: {} }]
    if (m === 'POST' && u === '/api/records/liza/restore') return [200, { record: envelope(liza), related: {} }]
    if (m === 'PATCH' && u.startsWith('/api/records/liza/fields/surname/values/')) {
      const rec = envelope({ ...liza, fields: { ...liza.fields, surname: [(b as { value: string }).value] } }, { surname: 1 })
      return [200, { record: rec, related: {} }]
    }
    return undefined
  })
  const w = await mountRecord('liza')
  return { w, calls }
}

const row = (w: Awaited<ReturnType<typeof card>>['w'], name: string) =>
  w.findAll('.field-row').find(r => r.find('.label').text() === name)!

describe('режим карточек (F-108)', () => {
  const modes: CardMode[] = ['read', 'inline', 'form']

  for (const mode of modes) {
    it(`${mode}, без права на правку — только чтение: без ручек, кнопки правки и добавления полей`, async () => {
      const { w } = await card(mode, false)
      expect(w.find('h1').text()).toContain('Лиза')
      expect(w.find('.mode').exists()).toBe(false)
      expect(w.find('.field-row input').exists()).toBe(false)
      expect(w.find('.handle').exists()).toBe(false)
      expect(w.find('.add-field').exists()).toBe(false)
      expect(w.find('.add-custom').exists()).toBe(false)
      await row(w, 'Фамилия').find('.values').trigger('dblclick')
      expect(row(w, 'Фамилия').find('input').exists()).toBe(false)
    })
  }

  it('read, есть право — чтение; кнопка переключает в форму', async () => {
    const { w } = await card('read', true)
    expect(w.find('.field-row input').exists()).toBe(false)
    expect(w.find('.handle').exists()).toBe(false)
    expect(w.find('.add-field').exists()).toBe(false)
    await row(w, 'Фамилия').find('.values').trigger('dblclick')
    expect(row(w, 'Фамилия').find('input').exists()).toBe(false)
    await w.find('.mode').trigger('click')
    expect(row(w, 'Фамилия').find('input').exists()).toBe(true)
  })

  it('form, есть право — карточка сразу в правке', async () => {
    const { w } = await card('form', true)
    expect(w.find('.mode').attributes('aria-pressed')).toBe('true')
    expect(row(w, 'Фамилия').find('input').exists()).toBe(true)
    expect(w.find('.add-field').exists()).toBe(true)
  })

  it('inline, есть право — ручки и кнопки добавления; двойной клик → ввод, Enter сохраняет, Esc отменяет', async () => {
    const { w, calls } = await card('inline', true)
    expect(w.find('.mode').attributes('aria-pressed')).toBe('false')
    expect(row(w, 'Фамилия').find('.handle').exists()).toBe(true)
    expect(w.find('.add-field').exists()).toBe(true)
    expect(w.find('.add-custom').exists()).toBe(true)
    expect(row(w, 'Имя').find('input').exists()).toBe(false)

    // Esc — отмена, поле снова в обычном виде, ничего не сохранено
    await row(w, 'Фамилия').find('.values').trigger('dblclick')
    await flushPromises()
    let input = row(w, 'Фамилия').find('input')
    expect(input.exists()).toBe(true)
    expect(row(w, 'Имя').find('input').exists()).toBe(false) // открыто только это поле
    await input.trigger('focusin')
    await input.setValue('Иванова')
    await input.trigger('keydown', { key: 'Escape' })
    await flushPromises()
    expect(row(w, 'Фамилия').find('input').exists()).toBe(false)
    expect(row(w, 'Фамилия').text()).toContain('Василькова')
    expect(calls.some(c => c.method === 'PATCH')).toBe(false)

    // ручка → ввод; Enter — сохранение, поле снова в обычном виде
    await row(w, 'Фамилия').find('.handle').trigger('click')
    await flushPromises()
    input = row(w, 'Фамилия').find('input')
    expect(document.activeElement).toBe(input.element)
    await input.trigger('focusin')
    await input.setValue('Иванова')
    await input.trigger('keydown', { key: 'Enter' })
    await flushPromises()
    await new Promise(r => setTimeout(r))
    await flushPromises()
    expect(calls.find(c => c.method === 'PATCH')!.body).toMatchObject({ value: 'Иванова' })
    expect(row(w, 'Фамилия').find('input').exists()).toBe(false)
    expect(row(w, 'Фамилия').text()).toContain('Иванова')
  })

  it('form: запись восстановлена из корзины — снова форма', async () => {
    const { w } = await card('form', false)
    expect(w.find('.mode').exists()).toBe(false)
    await w.find('.restore').trigger('click')
    await flushPromises()
    expect(w.find('.mode').attributes('aria-pressed')).toBe('true')
    expect(row(w, 'Фамилия').find('input').exists()).toBe(true)
  })

  it('inline: фокус ушёл на кнопку той же строки — поле снова в обычном виде', async () => {
    const { w } = await card('inline', true)
    await row(w, 'Фамилия').find('.label').trigger('dblclick')
    await flushPromises()
    const input = row(w, 'Фамилия').find('input')
    expect(input.exists()).toBe(true)
    await input.trigger('focusin')
    const history = row(w, 'Фамилия').find('.history-trigger')
    ;(history.element as HTMLElement).focus()
    await input.trigger('focusout', { relatedTarget: history.element })
    await history.trigger('click') // история открыта тем же кликом
    await flushPromises()
    await new Promise(r => setTimeout(r))
    await flushPromises()
    expect(row(w, 'Фамилия').find('input').exists()).toBe(false)
    expect(document.body.querySelector('.kin-popover.history')).not.toBeNull()
  })
})
