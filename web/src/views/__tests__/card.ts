// Набор для тестов карточки: схема ru, конверты с версиями, подмена fetch.
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import { records, ruSchema, type RecSpec } from '@/core/__tests__/kit'
import type { RecordDTO } from '@/core/types'
import { routes } from '@/router'
import { resetPrefs } from '@/ui/prefs'

import RecordView from '../RecordView.vue'

export const schema = ruSchema()

/** Конверт записи, как его отдаёт сервер: с версиями системных полей. */
export function envelope(spec: RecSpec, revs: Record<string, number> = {}): RecordDTO {
  const rec = records(schema, [spec])[spec.id]!
  rec.revs = { start: 0, end: 0, note: 0, title: 0, sensitive: 0, groups: 0, custom_fields: 0, ...revs }
  return rec
}

export type Handler = (method: string, url: string, body: unknown) => [number, unknown] | undefined

/** Подмена fetch: схема дашборда и обработчик остального; журнал вызовов. */
export function serve(handler: Handler) {
  const calls: { method: string; url: string; body: unknown }[] = []
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    const method = init?.method ?? 'GET'
    const body = typeof init?.body === 'string' ? JSON.parse(init.body) : init?.body
    calls.push({ method, url, body })
    if (url.endsWith('/schema')) return Response.json(schema)
    const [status, out] = handler(method, url, body) ?? [404, { error: 'not_found' }]
    return Response.json(out, { status })
  }))
  return calls
}

/** Карточка записи; edit — сразу в режиме редактирования (F-85). */
export async function mountRecord(id = 'liza', edit = false) {
  setActivePinia(createPinia())
  resetPrefs()
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(`/d/d1/r/${id}`)
  const w = mount(RecordView, { props: { d: 'd1', id }, global: { plugins: [router] }, attachTo: document.body })
  await flushPromises()
  if (edit) {
    await w.find('.mode').trigger('click')
    await flushPromises()
  }
  return w
}

/** Ввести значение в поле ввода режима редактирования и уйти из поля (F-86). */
export async function typeAndBlur(el: { setValue: (v: string) => Promise<void>; trigger: (e: string, o?: object) => Promise<void> }, v: string) {
  await el.trigger('focusin')
  await el.setValue(v)
  await el.trigger('focusout')
  await flushPromises()
}
