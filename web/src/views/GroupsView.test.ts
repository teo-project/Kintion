import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import type { GroupDTO } from '@/api/types'
import { routes } from '@/router'
import { resetGroups } from '@/ui/useGroups'

import GroupsView from './GroupsView.vue'

afterEach(() => {
  vi.unstubAllGlobals()
  resetGroups()
  document.body.innerHTML = ''
})

describe('экран групп', () => {
  it('«Петровы» в «Семье» и в «Гостях»: плоский список, «Входит в» и «Включает в себя» (F-105)', async () => {
    const list: GroupDTO[] = []
    const calls: { method: string; url: string; body: unknown }[] = []
    vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET'
      const body = init?.body ? JSON.parse(String(init.body)) as { name: string; parents?: string[] } : undefined
      calls.push({ method, url, body })
      if (method === 'POST' && body) {
        const g: GroupDTO = { id: `g${list.length + 1}`, name: body.name, count: 0, parents: body.parents ?? [], children: [], auto_surname: false, surnames: [] }
        for (const p of g.parents) list.find(x => x.id === p)!.children.push(g.id)
        list.push(g)
        return Response.json(g, { status: 201 })
      }
      return Response.json(list)
    }))
    setActivePinia(createPinia())
    const router = createRouter({ history: createMemoryHistory(), routes })
    await router.push('/d/d1/groups')
    const w = mount(GroupsView, { props: { d: 'd1' }, global: { plugins: [router] }, attachTo: document.body })
    await flushPromises()
    expect(w.text()).toContain('Групп пока нет')

    for (const n of ['Семья', 'Гости со стороны невесты']) {
      await w.find('.create input').setValue(n)
      await w.find('.create').trigger('submit')
      await flushPromises()
    }
    // «Входит в» при создании — обе группы
    await w.find('.create input').setValue('Петровы')
    await w.find('.create-parents .choose').trigger('click')
    await flushPromises()
    const boxes = document.querySelectorAll<HTMLButtonElement>('.kin-popover [role=checkbox]')
    expect(boxes).toHaveLength(2)
    for (const b of boxes) {
      b.click()
      await flushPromises()
    }
    await w.find('.create').trigger('submit')
    await flushPromises()

    expect(calls.filter(c => c.method === 'POST').map(c => c.body)).toEqual([
      { name: 'Семья' },
      { name: 'Гости со стороны невесты' },
      { name: 'Петровы', parents: ['g1', 'g2'] },
    ])
    // каждая группа — один раз, без отступов; подписи «Входит в» и «Включает в себя»
    const rows = w.findAll('.tree li')
    expect(rows.map(r => r.find('a').text())).toEqual(['Семья', 'Гости со стороны невесты', 'Петровы'])
    expect(rows[0]!.attributes('style')).toBeUndefined()
    expect(rows[0]!.find('.children-count').text()).toBe('Включает в себя: 1')
    expect(rows[0]!.find('.parents-count').exists()).toBe(false)
    expect(rows[2]!.find('.parents-count').text()).toBe('Входит в: 2')
    expect(rows[2]!.find('a').attributes('href')).toBe('/d/d1?g=g3')
  })
})
