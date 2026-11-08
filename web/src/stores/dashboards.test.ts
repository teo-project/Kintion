import { flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { schema, serve } from '@/views/__tests__/card'

import { useDashboardsStore } from './dashboards'

afterEach(() => vi.unstubAllGlobals())

describe('общий реестр дашборда', () => {
  it('правка шаблона — в общий реестр; устаревший ответ не откатывает её', async () => {
    setActivePinia(createPinia())
    serve(() => undefined)
    const s = useDashboardsStore()
    const first = await s.registry('d1')
    const person = schema.templates.find(x => x.key === 'kin.core.person')!
    const quick = (r: ReturnType<typeof s.current>) => r!.templates.get('kin.core.person')!.definition.quick_fields
    // правка от «чужого» снимка ложится на текущий реестр
    s.replaceTemplate('d1', first, { ...person, rev: 2, definition: { ...person.definition, quick_fields: ['name'] } })
    expect(quick(s.current('d1'))).toEqual(['name'])
    // ответ со старой версией не затирает
    s.replaceTemplate('d1', first, { ...person, rev: 1, definition: { ...person.definition, quick_fields: ['surname'] } })
    expect(quick(s.current('d1'))).toEqual(['name'])
    // перечитанный /schema (rev 0) не откатывает сохранённое позже
    s.forget('d1')
    await s.registry('d1')
    await flushPromises()
    expect(quick(s.current('d1'))).toEqual(['name'])
    expect(quick(await s.registry('d1'))).toEqual(['name'])
  })
})

describe('чужие дашборды (F-03)', () => {
  it('isGuest — по признаку guest из списка', async () => {
    setActivePinia(createPinia())
    serve((m, u) => (m === 'GET' && u === '/api/dashboards'
      ? [200, [{ id: 'mine', name: 'Мой', created_at: '' }, { id: 'his', name: 'Его', created_at: '', guest: true, owner_nick: 'papa' }]]
      : undefined))
    const s = useDashboardsStore()
    await s.load()
    expect(s.isGuest('mine')).toBe(false)
    expect(s.isGuest('his')).toBe(true)
    expect(s.isGuest('unknown')).toBe(false)
  })
})
