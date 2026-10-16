import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { serve } from '@/views/__tests__/card'

import TreeSelection from './TreeSelection.vue'

afterEach(() => vi.unstubAllGlobals())

describe('панель выделения дерева (F-131)', () => {
  it('число выделенных; «Добавить» и «Убрать» — одной массовой операцией; новая группа создаётся', async () => {
    const calls = serve((m, u) => {
      if (m === 'GET' && u === '/api/dashboards/d1/groups') return [200, [{ id: 'g1', name: 'Родня', count: 0 }]]
      if (m === 'POST' && u === '/api/dashboards/d1/groups') return [201, { id: 'g2', name: 'Новые', count: 0 }]
      if (m === 'POST' && u.endsWith('/records')) return [200, { changed: 2 }]
      return undefined
    })
    const w = mount(TreeSelection, { props: { d: 'd1', ids: ['a', 'b'] } })
    await flushPromises()
    expect(w.text()).toContain('Выделено: 2')
    await w.find('select').setValue('g1')
    await w.find('[data-testid="sel-add"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="sel-remove"]').trigger('click')
    await flushPromises()
    await w.find('select').setValue('+new')
    await w.find('input').setValue('Новые')
    await w.find('[data-testid="sel-add"]').trigger('click')
    await flushPromises()
    const posts = calls.filter(c => c.method === 'POST').map(c => [c.url, c.body])
    expect(posts).toEqual([
      ['/api/dashboards/d1/groups/g1/records', { add: ['a', 'b'] }],
      ['/api/dashboards/d1/groups/g1/records', { remove: ['a', 'b'] }],
      ['/api/dashboards/d1/groups', { name: 'Новые' }],
      ['/api/dashboards/d1/groups/g2/records', { add: ['a', 'b'] }],
    ])
    expect(w.emitted('changed')).toHaveLength(3)
    expect(w.text()).toContain('Готово: 2')
  })
})
