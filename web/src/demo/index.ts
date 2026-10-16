// Демо-режим фронтенда (сборка с VITE_DEMO=1): настоящее приложение без
// сервера. Запросы /api/* обслуживает DemoServer в памяти страницы,
// изображения — data: URL. Оглавление состояний — #/demo; состояние
// открывается ссылкой `?demo=<id>` (полная загрузка страницы: данные
// каждый раз из снимка, затем шаги состояния).
import type { Router } from 'vue-router'

import { api, type PreviewShape } from '@/api/client'

import DemoIndexView from './DemoIndexView.vue'
import { addImage, fileUrl, previewUrl } from './files'
import rawFixtures from './fixtures.json'
import { runState } from './runner'
import { DemoServer, type DemoAuth, type Fixtures } from './server'
import { demoStates, type DemoState } from './states'

const fixtures = rawFixtures as unknown as Fixtures

declare global {
  interface Window {
    /** демо-режим: состояния и сервер (для выгрузки и отладки) */
    __kinDemo?: { states: DemoState[]; server: DemoServer; fixtures: Fixtures }
  }
}

function isAuth(v: string | null): v is DemoAuth {
  return v === 'in' || v === 'guest' || v === 'setup'
}

export interface DemoStart {
  /** куда перейти до монтирования: адрес состояния или оглавление; null — по адресу страницы */
  start: string | null
  /** звать после монтирования: шаги состояния из `?demo=` */
  mounted: () => void
}

/** Включить демо до монтирования приложения. */
export async function startDemo(router: Router): Promise<DemoStart> {
  const states = demoStates(fixtures)
  const params = new URLSearchParams(location.search)
  const state = states.find(s => s.id === params.get('demo'))
  const auth = params.get('auth')
  const server = new DemoServer(fixtures, { auth: state?.auth ?? (isAuth(auth) ? auth : 'in'), prefs: state?.prefs, cardMode: state?.cardMode })

  await Promise.all(fixtures.files.map(f => addImage(f.sha256, `data:${f.mime};base64,${f.data}`)))
  api.fileUrl = (_d: string, sha: string) => fileUrl(sha)
  api.previewUrl = (_d: string, sha: string, crop: unknown, shape: PreviewShape = 'square', size = 64) =>
    previewUrl(sha, crop && typeof crop === 'object' ? crop : null, shape, size)

  let pending = 0
  const realFetch = globalThis.fetch.bind(globalThis)
  globalThis.fetch = async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const url = new URL(input instanceof Request ? input.url : String(input), location.href)
    if (url.origin !== location.origin || !(url.pathname.startsWith('/api/') || url.pathname === '/healthz')) {
      return realFetch(input, init)
    }
    pending++
    try {
      const raw = init?.body
      const body = raw instanceof FormData ? raw : typeof raw === 'string' ? (JSON.parse(raw) as unknown) : undefined
      await new Promise(r => setTimeout(r, 30)) // как по сети: экраны успевают показать «загрузку»
      const res = await server.handle((init?.method ?? 'GET').toUpperCase(), url, body)
      if (res.status === 204 || res.body === undefined) return new Response(null, { status: res.status })
      return new Response(JSON.stringify(res.body), { status: res.status, headers: { 'Content-Type': 'application/json' } })
    } finally {
      pending--
    }
  }

  router.addRoute({ path: '/demo', name: 'demo', component: DemoIndexView, meta: { guest: true } })
  window.__kinDemo = { states, server, fixtures }

  // состояние — со своего адреса; без адреса — оглавление
  const start = state ? state.path : !location.hash || location.hash === '#/' ? '/demo' : null
  const mounted = () => {
    const first = fixtures.files[0]
    void router.isReady().then(() => runState(state, {
      pending: () => pending,
      fault: (status, body) => (server.fault = { status, body }),
      file: () => {
        const bin = Uint8Array.from(atob(first?.data ?? ''), c => c.charCodeAt(0))
        return new File([bin], 'photo.png', { type: first?.mime ?? 'image/png' })
      },
    }))
  }
  return { start, mounted }
}
