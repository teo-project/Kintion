import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import App from './App.vue'
import { guard, routes } from './router'

/** Подмена fetch: ответы по «МЕТОД путь». */
function serve(map: Record<string, [number, unknown]>) {
  const fn = vi.fn(async (url: string, init?: RequestInit) => {
    const key = `${init?.method ?? 'GET'} ${url}`
    const [status, body] = map[key] ?? [404, { error: 'not_found' }]
    return new Response(status === 204 ? null : JSON.stringify(body), { status })
  })
  vi.stubGlobal('fetch', fn)
  return fn
}

async function render(path: string) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({ history: createMemoryHistory(), routes })
  guard(router, pinia)
  await router.push(path)
  await router.isReady()
  const wrapper = mount(App, { global: { plugins: [pinia, router] } })
  await flushPromises()
  return { wrapper, router }
}

const me = { account: { id: 'a1', login: 'owner', is_admin: true }, csrf: 'c1' }

afterEach(() => vi.unstubAllGlobals())

describe('App', () => {
  it('shows not found on unknown path', async () => {
    serve({ 'GET /api/me': [200, me], 'GET /api/dashboards': [200, []] })
    const { wrapper } = await render('/any/path')
    expect(wrapper.find('h1').text()).toBe('Страница не найдена')
  })

  it('sends to setup when there is no owner yet', async () => {
    serve({ 'GET /api/me': [401, { error: 'unauthorized' }], 'GET /api/setup': [200, { needed: true }] })
    const { router } = await render('/d/x')
    expect(router.currentRoute.value.name).toBe('setup')
  })

  it('sends to login with the return path', async () => {
    serve({ 'GET /api/me': [401, { error: 'unauthorized' }], 'GET /api/setup': [200, { needed: false }] })
    const { router } = await render('/d/x')
    expect(router.currentRoute.value.name).toBe('login')
    expect(router.currentRoute.value.query.next).toBe('/d/x')
  })

  it('logs in, sends CSRF on changes and opens the first dashboard', async () => {
    const fetch = serve({
      'GET /api/me': [401, { error: 'unauthorized' }],
      'GET /api/setup': [200, { needed: false }],
      'POST /api/login': [200, me],
      'GET /api/dashboards': [200, [{ id: 'd1', name: 'Мой', created_at: '' }]],
      'GET /api/dashboards/d1/schema': [200, { language: 'ru', entities: [], templates: [] }],
      'GET /api/dashboards/d1/groups': [200, []],
    })
    const { wrapper, router } = await render('/login')
    const inputs = wrapper.findAll('input')
    await inputs[0]!.setValue('owner')
    await inputs[1]!.setValue('secret-password')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    const login = fetch.mock.calls.find(([u]) => u === '/api/login')!
    expect(JSON.parse(String(login[1]!.body))).toEqual({ login: 'owner', password: 'secret-password' })
    expect(router.currentRoute.value.name).toBe('dashboard')
    expect(router.currentRoute.value.params.d).toBe('d1')
    expect(wrapper.find('.account-avatar').attributes('aria-label')).toBe('owner')
  })

  it('shows a login error from the language file', async () => {
    serve({
      'GET /api/me': [401, { error: 'unauthorized' }],
      'GET /api/setup': [200, { needed: false }],
      'POST /api/login': [401, { error: 'bad_credentials', message: 'bad' }],
    })
    const { wrapper } = await render('/login')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(wrapper.find('[role=alert]').text()).toBe('Неверный логин или пароль')
  })
})

describe('ThemeSwitcher', () => {
  it('switches theme on <html>', async () => {
    serve({ 'GET /api/me': [401, { error: 'unauthorized' }], 'GET /api/setup': [200, { needed: false }] })
    const { wrapper } = await render('/login')
    const btn = wrapper.find('button.theme-switcher')
    expect(btn.attributes('aria-label')).toBe('Тема: Как в системе')
    await btn.trigger('click')
    expect(document.documentElement.getAttribute('data-theme')).toBe('light')
    await btn.trigger('click')
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')
    expect(btn.attributes('title')).toBe('Тема: Тёмная')
    await btn.trigger('click')
    expect(document.documentElement.hasAttribute('data-theme')).toBe(false)
  })
})
