import { flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { resetGroups } from '@/ui/useGroups'
import { envelope, mountRecord, serve } from '@/views/__tests__/card'

afterEach(() => {
  vi.unstubAllGlobals()
  resetGroups()
  document.body.innerHTML = ''
})

describe('группы в карточке', () => {
  it('метки записи и назначение в несколько групп одним запросом с версией', async () => {
    const liza = envelope({ id: 'liza', template: 'kin.core.person', fields: { name: ['Лиза'] } }, { groups: 3 })
    liza.groups = ['g1']
    const calls = serve((m, u) => {
      if (u === '/api/dashboards/d1/groups') {
        return [200, [{ id: 'g1', name: 'Наша семья', count: 1, parents: [], children: ['g2'] }, { id: 'g2', name: 'Ивановы', count: 0, parents: ['g1'], children: [] }]]
      }
      if (m === 'GET' && u === '/api/records/liza') return [200, { record: liza, related: {} }]
      if (m === 'PUT' && u === '/api/records/liza/groups') return [200, { record: liza, related: {} }]
      return undefined
    })
    const w = await mountRecord('liza', true)
    expect(w.find('.groups-row .chip').text()).toBe('Наша семья')
    await w.find('.groups-row .kin-icon-btn').trigger('click')
    await flushPromises()
    ;(document.body.querySelector('.kin-popover .choose') as HTMLButtonElement).click()
    await flushPromises()
    const boxes = [...document.body.querySelectorAll('.tree [role=checkbox]')] as HTMLButtonElement[]
    expect(boxes.map(b => b.getAttribute('aria-checked'))).toEqual(['true', 'false'])
    boxes[1]!.click()
    await flushPromises()
    ;(document.body.querySelector('.kin-popover .save') as HTMLButtonElement).click()
    await flushPromises()
    expect(calls.find(c => c.method === 'PUT')!.body).toEqual({ if_rev: 3, groups: ['g1', 'g2'] })
  })

  it('новая группа создаётся прямо из выбора и сразу выбирается (F-122)', async () => {
    const liza = envelope({ id: 'liza', template: 'kin.core.person', fields: { name: ['Лиза'] } }, { groups: 1 })
    liza.groups = []
    let list: unknown[] = []
    const calls = serve((m, u, body) => {
      if (m === 'GET' && u === '/api/dashboards/d1/groups') return [200, list]
      if (m === 'POST' && u === '/api/dashboards/d1/groups') {
        list = [{ id: 'gn', name: (body as { name: string }).name, count: 0, parents: [], children: [], auto_surname: false, surnames: [] }]
        return [201, list[0]]
      }
      if (m === 'GET' && u === '/api/records/liza') return [200, { record: liza, related: {} }]
      if (m === 'PUT' && u === '/api/records/liza/groups') return [200, { record: liza, related: {} }]
      return undefined
    })
    const w = await mountRecord('liza', true)
    await w.find('.groups-row .kin-icon-btn').trigger('click')
    await flushPromises()
    ;(document.body.querySelector('.kin-popover .choose') as HTMLButtonElement).click()
    await flushPromises()
    const input = document.body.querySelector('input[name=new-group]') as HTMLInputElement
    input.value = 'Ивановы'
    input.dispatchEvent(new Event('input'))
    ;(document.body.querySelector('.new-group') as HTMLFormElement).dispatchEvent(new Event('submit'))
    await flushPromises()
    expect(calls.find(c => c.method === 'POST')!.body).toEqual({ name: 'Ивановы' })
    ;(document.body.querySelector('.kin-popover .save') as HTMLButtonElement).click()
    await flushPromises()
    expect(calls.find(c => c.method === 'PUT')!.body).toEqual({ if_rev: 1, groups: ['gn'] })
  })
})
