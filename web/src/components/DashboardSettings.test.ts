import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import { schema, serve } from '@/views/__tests__/card'

import DashboardSettings from './DashboardSettings.vue'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

/** вкладка настроек (Reka Tabs переключает по mousedown) */
async function tab(name: string) {
  const el = [...document.body.querySelectorAll('[role=tab]')].find(x => x.textContent!.trim() === name) as HTMLElement
  el.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, button: 0 }))
  el.focus()
  el.click()
  await flushPromises()
}

describe('настройки дашборда (F-132)', () => {
  it('поля быстрой формы шаблона: каждый шаг сохраняется сразу, по очереди, с версией; возврат к умолчанию', async () => {
    setActivePinia(createPinia())
    const person = schema.templates.find(x => x.key === 'kin.core.person')!
    const calls = serve((m, u, body) => {
      if (m === 'PUT' && u.endsWith('/quick_fields')) {
        const b = body as { if_rev: number; quick_fields: string[] }
        const qf = b.quick_fields.length ? b.quick_fields : person.definition.quick_fields
        return [200, { ...person, rev: b.if_rev + 1, definition: { ...person.definition, quick_fields: qf } }]
      }
      if (m === 'PATCH' && u === '/api/dashboards/d1') return [200, { id: 'd1', name: (body as { name: string }).name, created_at: '' }]
      return undefined
    })
    const w = mount(DashboardSettings, { props: { dashboard: 'd1', name: 'Мой', open: false }, attachTo: document.body })
    await w.setProps({ open: true })
    await flushPromises()
    // название — на вкладке «Настройки», поля — на «Шаблоны»
    expect(document.body.querySelector('input[name=dashboard-name]')).not.toBeNull()
    await tab('Шаблоны')
    const card = document.body.querySelector('[data-testid="quick-kin.core.person"]') as HTMLElement
    const on = () => [...card.querySelectorAll('.cloud-on .pill')].map(l => l.textContent!.trim())
    const off = () => [...card.querySelectorAll('.cloud-off .pill')].map(l => l.textContent!.trim())
    const puts = () => calls.filter(c => c.method === 'PUT').map(c => c.body)
    expect(on()).toEqual(['Имя', 'Фамилия', 'Пол', 'Примечание', 'Дата рождения'])
    expect(card.querySelector('button[aria-label^="Сохранить"]')).toBeNull()
    // убрать «Фамилию» (клик переносит в нижнее облако) — сохраняется сразу
    const pill = (name: string) => [...card.querySelectorAll('.pill')].find(x => x.textContent!.trim() === name) as HTMLButtonElement
    pill('Фамилия').click()
    await nextTick()
    expect(card.dataset.state).toBe('saving')
    await flushPromises()
    expect(on()).toEqual(['Имя', 'Пол', 'Примечание', 'Дата рождения'])
    expect(off()).toContain('Фамилия')
    expect(puts()).toEqual([{ if_rev: 0, quick_fields: ['name', 'gender', '.note', 'start'] }])
    expect(card.dataset.state).toBe('saved')
    // поднять «Пол» (Alt + ←) — следующий запрос с версией из ответа
    pill('Пол').dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowLeft', altKey: true, bubbles: true }))
    await flushPromises()
    expect(puts()[1]).toEqual({ if_rev: 1, quick_fields: ['gender', 'name', '.note', 'start'] })
    // быстрые клики во время запроса: не теряются, уходят следующим запросом одним набором
    pill('Примечание').click()
    pill('Фамилия').click()
    pill('Дата рождения').click()
    await flushPromises()
    expect(puts().slice(2)).toEqual([
      { if_rev: 2, quick_fields: ['gender', 'name', 'start'] },
      { if_rev: 3, quick_fields: ['gender', 'name', 'surname'] },
    ])
    expect(on()).toEqual(['Пол', 'Имя', 'Фамилия'])
    // «По умолчанию» — сразу: пустой набор, форма — из ответа (пак)
    ;([...card.querySelectorAll('button')].find(b => b.textContent!.trim() === 'По умолчанию') as HTMLButtonElement).click()
    await flushPromises()
    expect(puts()[4]).toEqual({ if_rev: 4, quick_fields: [] })
    expect(on()).toEqual(['Имя', 'Фамилия', 'Пол', 'Примечание', 'Дата рождения'])
    // название
    await tab('Настройки')
    const input = document.body.querySelector('input[name=dashboard-name]') as HTMLInputElement
    input.value = 'Семья'
    input.dispatchEvent(new Event('input'))
    ;(input.closest('form') as HTMLFormElement).dispatchEvent(new Event('submit'))
    await flushPromises()
    expect(calls.find(c => c.method === 'PATCH')!.body).toEqual({ name: 'Семья' })
  })
})

describe('конфликт версии набора (F-45)', () => {
  it('409: шаблон перечитан, показан текущий набор и ошибка', async () => {
    setActivePinia(createPinia())
    const calls = serve((m, u) => {
      if (m === 'PUT' && u.endsWith('/quick_fields')) return [409, { error: 'conflict', message: 'conflict' }]
      return undefined
    })
    const w = mount(DashboardSettings, { props: { dashboard: 'd1', name: 'Мой', open: false }, attachTo: document.body })
    await w.setProps({ open: true })
    await flushPromises()
    await tab('Шаблоны')
    const card = document.body.querySelector('[data-testid="quick-kin.core.person"]') as HTMLElement
    const pill = (name: string) => [...card.querySelectorAll('.pill')].find(x => x.textContent!.trim() === name) as HTMLButtonElement
    pill('Фамилия').click()
    await flushPromises()
    expect(card.dataset.state).toBe('error')
    expect(document.body.querySelector('[role=alert]')!.textContent).toContain('Шаблон изменён')
    expect([...card.querySelectorAll('.cloud-on .pill')].map(l => l.textContent!.trim())).toContain('Фамилия')
    expect(calls.filter(c => c.method === 'GET' && c.url.endsWith('/schema')).length).toBe(2)
  })
})

describe('закреплённые плашки (F-132)', () => {
  it('обязательное поле пака и роль с min ≥ 1 не снимаются; порядок поля меняется', async () => {
    setActivePinia(createPinia())
    const person = schema.templates.find(x => x.key === 'kin.core.person')!
    const surname = person.definition.fields!.find(f => f.key === 'surname')!
    surname.required = true
    try {
      serve(() => undefined)
      const w = mount(DashboardSettings, { props: { dashboard: 'd1', name: 'Мой', open: false }, attachTo: document.body })
      await w.setProps({ open: true })
      await flushPromises()
      await tab('Шаблоны')
      const card = document.body.querySelector('[data-testid="quick-kin.core.person"]') as HTMLElement
      const pill = (name: string) => [...card.querySelectorAll('.pill')].find(x => x.textContent!.trim() === name) as HTMLButtonElement
      pill('Фамилия').click()
      await flushPromises()
      expect([...card.querySelectorAll('.cloud-on .pill')].map(x => x.textContent!.trim())).toContain('Фамилия')
      expect(pill('Фамилия').getAttribute('aria-disabled')).toBe('true')
      // роль с min ≥ 1 — закреплённая плашка без кнопки
      const rel = document.body.querySelector('[data-testid="quick-kin.core.parent_child"]') as HTMLElement
      expect(rel.querySelectorAll('.cloud-on span.pill.pinned').length).toBeGreaterThan(0)
    } finally {
      delete surname.required
    }
  })
})
