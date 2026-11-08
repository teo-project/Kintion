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
    // подсказка — одна на все шаблоны, без упоминания Alt + стрелок
    const hints = [...document.body.querySelectorAll('.settings .hint')].filter(h => h.textContent!.includes('Нажмите на плашку'))
    expect(hints).toHaveLength(1)
    expect(hints[0]!.textContent!.trim()).toBe('Нажмите на плашку, чтобы добавить или убрать; изменить порядок — перетаскиванием')
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

  it('эры (F-143): каждый шаг сохраняется сразу, без кнопок; смена эры по умолчанию — всегда с вопросом; переименование — заменой', async () => {
    setActivePinia(createPinia())
    schema.eras = { show: false, edit: false, default: '', order: [], today: null, rev: 0, list: ['', 'AC'] }
    try {
      const calls = serve((m, u, body) => {
        if (m === 'GET' && u === '/api/dashboards/d1/eras') return [200, schema.eras]
        if (m === 'PUT' && u === '/api/dashboards/d1/eras') {
          const b = body as { if_rev: number; eras: { order: string[] } & Record<string, unknown> }
          return [200, { ...b.eras, rev: b.if_rev + 1, list: b.eras.order.length ? b.eras.order : ['', 'AC'] }]
        }
        return undefined
      })
      const w = mount(DashboardSettings, { props: { dashboard: 'd1', name: 'Мой', open: false }, attachTo: document.body })
      await w.setProps({ open: true })
      await flushPromises()
      const box = document.body.querySelector('[data-testid=eras]') as HTMLElement
      const puts = () => calls.filter(c => c.method === 'PUT').map(c => c.body as Record<string, unknown>)
      const rows = () => [...box.querySelectorAll<HTMLInputElement>('.order input.name')].map(x => x.value || x.placeholder)
      expect(rows()).toEqual(['н. э.', 'AC'])
      // кнопок-галочек подтверждения нет
      expect(box.querySelectorAll('button[type=submit], form')).toHaveLength(0)
      const change = (el: HTMLInputElement, v: string) => {
        el.value = v
        el.dispatchEvent(new Event('input'))
        el.dispatchEvent(new Event('change'))
      }

      // показывать эру — переключатель «да / нет»
      const yes = (name: string) => [...box.querySelectorAll(`[data-name=${name}] button`)].find(b => b.textContent!.trim() === 'Да') as HTMLElement
      yes('era-show').click()
      await flushPromises()
      expect(puts()[0]).toEqual({ if_rev: 0, eras: { show: true, edit: false, default: '', order: [], years: {}, today: null } })
      expect(box.dataset.state).toBe('saved')

      // эра по умолчанию с нашей — тоже с вопросом; «к уже созданным» — замена в датах
      change(box.querySelector('input[name=era-default]') as HTMLInputElement, 'AC')
      await flushPromises()
      expect(puts()).toHaveLength(1)
      ;(box.querySelector('[data-testid=era-apply-existing]') as HTMLElement).click()
      await flushPromises()
      expect(puts()[1]).toEqual({ if_rev: 1, eras: { show: true, edit: false, default: 'AC', order: ['AC'], years: {}, today: null }, replace: { from: '', to: 'AC' } })
      expect(rows()).toEqual(['AC'])

      // переименование эры в списке — такая же замена, по изменению поля
      change(box.querySelector('.order input.name') as HTMLInputElement, 'ACC')
      await flushPromises()
      expect(puts()[2]).toEqual({ if_rev: 2, eras: { show: true, edit: false, default: 'ACC', order: ['ACC'], years: {}, today: null }, replace: { from: 'AC', to: 'ACC' } })

      // годы эры: минимальный больше максимального — обратный счёт
      change(box.querySelector('.order input[name=era-min]') as HTMLInputElement, '15000')
      await flushPromises()
      change(box.querySelector('.order input[name=era-max]') as HTMLInputElement, '1')
      await flushPromises()
      expect(puts()[4]).toEqual({ if_rev: 4, eras: { show: true, edit: false, default: 'ACC', order: ['ACC'], years: { ACC: { min: 15000, max: 1 } }, today: null } })
      expect(box.querySelector('.order .rev')).not.toBeNull()

      // эра по умолчанию с непустой — с вопросом «к уже созданным / только к новым»
      change(box.querySelector('input[name=era-default]') as HTMLInputElement, 'BC')
      await flushPromises()
      expect(puts()).toHaveLength(5)
      ;(box.querySelector('[data-testid=era-apply-new]') as HTMLElement).click()
      await flushPromises()
      expect(puts()[5]).toEqual({ if_rev: 5, eras: { show: true, edit: false, default: 'BC', order: ['BC', 'ACC'], years: { ACC: { min: 15000, max: 1 } }, today: null } })

      // пустое название при непустой эре по умолчанию — замена на «без эры», не отказ
      change(box.querySelector('.order li[data-era=ACC] input.name') as HTMLInputElement, '')
      await flushPromises()
      expect(puts()[6]).toEqual({ if_rev: 6, eras: { show: true, edit: false, default: 'BC', order: ['BC', ''], years: { '': { min: 15000, max: 1 } }, today: null }, replace: { from: 'ACC', to: '' } })
      expect(rows()).toEqual(['BC', 'без эры'])

      // текущий год мира — по изменению поля
      const y = box.querySelector('input[name=era-today-year]') as HTMLInputElement
      y.value = '305'
      y.dispatchEvent(new Event('input'))
      change(box.querySelector('input[name=era-today-era]') as HTMLInputElement, 'BC')
      await flushPromises()
      expect(puts()[7]).toEqual({ if_rev: 7, eras: { show: true, edit: false, default: 'BC', order: ['BC', ''], years: { '': { min: 15000, max: 1 } }, today: { y: 305, e: 'BC' } } })
    } finally {
      delete schema.eras
    }
  })

  it('эры: порядок — перестановкой (Alt + стрелки), сохраняется весь список', async () => {
    setActivePinia(createPinia())
    schema.eras = { show: false, edit: false, default: '', order: [], today: null, rev: 0, list: ['BC', '', 'AC'] }
    try {
      const calls = serve((m, u, body) => {
        if (m === 'PUT' && u === '/api/dashboards/d1/eras') {
          const b = body as { if_rev: number; eras: { order: string[] } & Record<string, unknown> }
          if (b.if_rev !== 0) return [409, { error: 'conflict', message: 'conflict' }]
          return [200, { ...b.eras, rev: 1, list: b.eras.order }]
        }
        return undefined
      })
      const w = mount(DashboardSettings, { props: { dashboard: 'd1', name: 'Мой', open: false }, attachTo: document.body })
      await w.setProps({ open: true })
      await flushPromises()
      const box = document.body.querySelector('[data-testid=eras]') as HTMLElement
      const handle = box.querySelectorAll('.order .handle')[0] as HTMLElement
      handle.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', altKey: true, bubbles: true }))
      await flushPromises()
      const put = calls.find(c => c.method === 'PUT')!.body as { eras: { order: string[] } }
      expect(put.eras.order).toEqual(['', 'BC', 'AC'])
      expect([...box.querySelectorAll<HTMLInputElement>('.order input.name')].map(x => x.value || x.placeholder)).toEqual(['н. э.', 'BC', 'AC'])
      // версия из ответа; другая — конфликт: шаги отброшены, ошибка
      ;(box.querySelectorAll('.order .handle')[0] as HTMLElement).dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', altKey: true, bubbles: true }))
      await flushPromises()
      expect((calls.filter(c => c.method === 'PUT')[1]!.body as { if_rev: number }).if_rev).toBe(1)
      expect(box.dataset.state).toBe('error')
      expect(box.querySelector('[role=alert]')!.textContent).toContain('Эры изменены')
    } finally {
      delete schema.eras
    }
  })
})

describe('настройки чужого дашборда (гость, F-03)', () => {
  it('только просмотр: поля и плашки недоступны, вкладки «Доступ» нет', async () => {
    setActivePinia(createPinia())
    const calls = serve(() => undefined)
    const w = mount(DashboardSettings, { props: { dashboard: 'd1', name: 'Чужой', open: false, readonly: true }, attachTo: document.body })
    await w.setProps({ open: true })
    await flushPromises()
    const tabs = [...document.body.querySelectorAll('[role=tab]')].map(x => x.textContent!.trim())
    expect(tabs).not.toContain('Доступ')
    expect((document.body.querySelector('input[name=dashboard-name]') as HTMLInputElement).matches(':disabled')).toBe(true)
    await tab('Шаблоны')
    const pill = document.body.querySelector('[data-testid="quick-kin.core.person"] .pill') as HTMLButtonElement
    expect(pill.matches(':disabled')).toBe(true)
    pill.click()
    await flushPromises()
    expect(calls.filter(c => c.method !== 'GET')).toEqual([])
  })
})
