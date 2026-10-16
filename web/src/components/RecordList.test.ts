import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import { altSchema, ctxOf, ruSchema } from '@/core/__tests__/kit'
import { routes } from '@/router'
import { getDraft } from '@/ui/drafts'
import DashboardView from '@/views/DashboardView.vue'

import RecordList from './RecordList.vue'

const liza = {
  id: 'liza', template: 'kin.core.person',
  fields: { name: ['Лиза'], patronymic: ['Сергеевна'], surname: ['Василькова', 'Давыдко'], photo: [{ value: 'abc', meta: { crop: { x: 1, y: 2, w: 30 } } }] },
}

function router() {
  return createRouter({ history: createMemoryHistory(), routes })
}

describe('RecordList', () => {
  it('подписи — резолвером по правилу шаблона, фото — превью с обрезкой', () => {
    const ctx = ctxOf(ruSchema(), [liza, { id: 'org', template: 'kin.core.organization', title: 'ООО Ромашка' }])
    const w = mount(RecordList, {
      props: { dashboard: 'd1', items: [ctx.records.liza!, ctx.records.org!], ctx, emptyText: 'пусто' },
      global: { plugins: [router()] },
    })
    const titles = w.findAll('.title').map(x => x.text())
    expect(titles[0]).toBe('Лиза Сергеевна Василькова')
    expect(w.find('img').attributes('src')).toBe('/api/files/d1/abc/preview?shape=circle&size=64&x=1&y=2&w=30&h=30')
    expect(w.findAll('.placeholder')).toHaveLength(1)
    expect(w.find('a').attributes('href')).toBe('/d/d1/r/liza')
  })

  it('синтетический пак: подписи тоже из шаблона', () => {
    const schema = altSchema()
    const tpl = schema.templates.find(x => x.default && x.definition.title?.length)!
    const field = tpl.definition.fields!.find(f => f.type === 'text')!.key
    const ctx = ctxOf(schema, [{ id: 'x', template: tpl.key, fields: { [field]: ['Зета'] } }])
    const w = mount(RecordList, {
      props: { dashboard: 'd1', items: [ctx.records.x!], ctx, emptyText: 'пусто' },
      global: { plugins: [router()] },
    })
    expect(w.find('.title').text()).toContain('Зета')
  })

  it('отношение (F-109): участники слева, «ещё N», справа — шаблон и подпись', () => {
    const p = (id: string, name: string, gender: string) => ({ id, template: 'kin.core.person', fields: { name: [name], gender: [gender] } })
    const ctx = ctxOf(ruSchema(), [
      p('ivan', 'Иван', 'male'), p('anna', 'Анна', 'female'), p('petr', 'Пётр', 'male'), p('olga', 'Ольга', 'female'),
      { id: 'org', template: 'kin.core.organization', fields: { name: ['ООО Ромашка'] } },
      { id: 'm', template: 'kin.core.relationship', fields: { stage: ['marriage'] }, parts: [['partner', 'ivan'], ['partner', 'anna']] },
      { id: 'w', template: 'kin.core.work', parts: [['colleague', 'ivan'], ['colleague', 'anna'], ['employer', 'org'], ['colleague', 'petr'], ['colleague', 'olga']] },
    ])
    const w = mount(RecordList, {
      props: { dashboard: 'd1', items: [ctx.records.m!, ctx.records.w!, ctx.records.ivan!], ctx, emptyText: 'пусто' },
      global: { plugins: [router()] },
    })
    const rows = w.findAll('.row')
    expect(rows[0]!.findAll('.part').map(x => x.text())).toEqual(['Иван — муж', 'Анна — жена'])
    expect(rows[0]!.find('.more').exists()).toBe(false)
    expect(rows[0]!.find('.template').text()).toBe('Пара (Брак)')
    expect(rows[1]!.findAll('.part').map(x => x.text())).toEqual(['ООО Ромашка', 'Иван — коллега', 'Анна — коллега', 'Пётр — коллега'])
    expect(rows[1]!.find('.more').text()).toBe('ещё 1')
    expect(rows[1]!.find('.template').text()).toBe('Работа')
    // прочие сущности — как раньше
    expect(rows[2]!.find('.title').text()).toBe('Иван')
    expect(rows[2]!.find('.template').text()).toBe('Человек')
  })

  it('пустое состояние', () => {
    const ctx = ctxOf(ruSchema(), [])
    const w = mount(RecordList, {
      props: { dashboard: 'd1', items: [], ctx, emptyText: 'Записей нет' },
      global: { plugins: [router()] },
    })
    expect(w.text()).toBe('Записей нет')
    expect(w.find('ul').exists()).toBe(false)
  })
})

describe('DashboardView', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('вкладки из реестра; «Создать» открывает черновик карточки без запроса (F-88)', async () => {
    const schema = ruSchema()
    const calls: [string, string, unknown][] = []
    vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET'
      calls.push([method, url, init?.body ? JSON.parse(String(init.body)) : undefined])
      if (url.endsWith('/schema')) return Response.json(schema)
      if (url.endsWith('/groups')) return Response.json([])
      if (url === '/api/dashboards') return Response.json([{ id: 'd1', name: 'Мой', created_at: '' }])
      if (url.includes('/records')) return Response.json({ items: [], total: 0, related: {} })
      if (url.endsWith('/operations')) {
        return Response.json({ operation_id: 'o', ids: { r: 'new-id' }, records: [], related: {}, errors: [] }, { status: 201 })
      }
      return Response.json({ error: 'not_found' }, { status: 404 })
    }))
    setActivePinia(createPinia())
    const r = router()
    await r.push('/d/d1?e=kin.core.person')
    await r.isReady()
    const w = mount(DashboardView, { props: { d: 'd1' }, global: { plugins: [r] } })
    await flushPromises()
    const tabs = w.findAll('[role=tab]').map(x => x.text())
    expect(tabs).toEqual([...schema.entities.map(e => e.name)])
    const list = calls.find(([, u]) => u.includes('/records'))!
    expect(list[1]).toBe('/api/dashboards/d1/records?entity=kin.core.person&limit=50&offset=0')
    expect(w.text()).toContain('Записей нет')

    // у Человека один шаблон без обязательных участников
    await w.find('.create').trigger('click')
    await flushPromises()
    expect(calls.some(([, u]) => u.endsWith('/operations'))).toBe(false) // на сервере ничего
    const id = String(r.currentRoute.value.params.id)
    expect(r.currentRoute.value.name).toBe('record')
    expect(getDraft(id)).toEqual({ dashboard: 'd1', template: 'kin.core.person', groups: [] })
  })
})

describe('выделение в списке (F-131)', () => {
  it('галочки у строк только в режиме выделения; переключение — событием', async () => {
    const ctx = ctxOf(ruSchema(), [{ id: 'a', template: 'kin.core.person', fields: { name: ['Лиза'] } }])
    const base = { dashboard: 'd1', items: [ctx.records.a!], ctx, emptyText: '-' }
    expect(mount(RecordList, { props: base, global: { plugins: [router()] } }).find('.sel').exists()).toBe(false)
    const w = mount(RecordList, { props: { ...base, selectable: true, selected: new Set(['a']) }, global: { plugins: [router()] } })
    const box = w.find('.sel [role=checkbox]')
    expect(box.attributes('aria-checked')).toBe('true')
    await box.trigger('click')
    expect(w.emitted('toggle')).toEqual([['a', false]])
  })
})
