import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import { routes } from '@/router'

import { envelope, serve } from './__tests__/card'
import TrashView from './TrashView.vue'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const person = (id: string, name: string) => ({ ...envelope({ id, template: 'kin.core.person', fields: { name: [name] } }), deleted: true })

describe('корзина (T25)', () => {
  it('страницы, восстановление и очистка с подтверждением', async () => {
    let items = [person('a', 'Анна'), person('b', 'Борис')]
    const calls = serve((m, u) => {
      if (m === 'GET' && u.startsWith('/api/dashboards/d1/records')) {
        const off = Number(new URL(u, 'http://x').searchParams.get('offset') ?? 0)
        return [200, { items: off ? [person('c', 'Вера')] : items, total: off ? 3 : items.length + 1, related: {} }]
      }
      if (m === 'POST' && u === '/api/records/a/restore') {
        items = items.filter(r => r.id !== 'a')
        return [200, { record: items[0], related: {} }]
      }
      if (m === 'DELETE' && u === '/api/records/b/purge') {
        items = []
        return [200, {}]
      }
      return undefined
    })
    setActivePinia(createPinia())
    const router = createRouter({ history: createMemoryHistory(), routes })
    await router.push('/d/d1/trash')
    const w = mount(TrashView, { props: { d: 'd1' }, global: { plugins: [router] }, attachTo: document.body })
    await flushPromises()
    expect(w.findAll('[data-test="trash-row"]').map(r => r.find('.title').text())).toEqual(['Анна', 'Борис'])

    await w.findAll('button').find(b => b.text() === 'Показать ещё')!.trigger('click')
    await flushPromises()
    expect(calls.some(c => c.url.includes('offset=2'))).toBe(true)
    expect(w.findAll('[data-test="trash-row"]')).toHaveLength(3)

    await w.findAll('[data-test="trash-row"]')[0]!.findAll('button')[0]!.trigger('click')
    await flushPromises()
    expect(calls.map(c => `${c.method} ${c.url}`)).toContain('POST /api/records/a/restore')

    await w.findAll('[data-test="trash-row"]')[0]!.findAll('button')[1]!.trigger('click')
    await flushPromises()
    const confirm = [...document.body.querySelectorAll('[role="alertdialog"] button')].find(b => b.getAttribute('aria-label') === 'Удалить окончательно') as HTMLButtonElement
    confirm.click()
    await flushPromises()
    expect(calls.map(c => `${c.method} ${c.url}`)).toContain('DELETE /api/records/b/purge')
    expect(w.text()).toContain('Корзина пуста')
  })
})
