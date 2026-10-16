import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import { routes } from '@/router'
import { serve } from '@/views/__tests__/card'
import AuditList from './AuditList.vue'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

describe('журнал: фильтры (T28)', () => {
  it('автор и действие уходят в запрос; в истории записи фильтров нет', async () => {
    setActivePinia(createPinia())
    const calls = serve((m, u) => {
      if (u.startsWith('/api/dashboards/d1/audit')) return [200, { operations: [], next: null, records: {}, related: {}, purged: {} }]
      if (u === '/api/dashboards/d1/groups') return [200, []]
      return undefined
    })
    const router = createRouter({ history: createMemoryHistory(), routes })
    const w = mount(AuditList, { props: { dashboard: 'd1' }, global: { plugins: [router] }, attachTo: document.body })
    await flushPromises()
    await w.find('select[name="audit-op"]').setValue('purge')
    await flushPromises()
    await w.find('select[name="audit-actor"]').setValue('system')
    await flushPromises()
    const last = calls.filter(c => c.url.startsWith('/api/dashboards/d1/audit')).at(-1)!
    expect(last.url).toContain('op=purge')
    expect(last.url).toContain('actor=system')
    expect(w.find('.record-filter').exists()).toBe(true)

    const h = mount(AuditList, { props: { dashboard: 'd1', entity: 'anna' }, global: { plugins: [router] } })
    await flushPromises()
    expect(h.find('.filters').exists()).toBe(false)
  })
})
