import { createPinia } from 'pinia'
import { createApp } from 'vue'

import { onUnauthorized } from './api/client'
import App from './App.vue'
import { guard, router } from './router'
import { useSessionStore } from './stores/session'
import { applyTheme, loadTheme } from './theme'
import './style.css'

applyTheme(loadTheme())

async function boot() {
  // демо-сборка (VITE_DEMO=1): API в памяти страницы, см. src/demo
  const demo = import.meta.env.VITE_DEMO === '1' ? await (await import('./demo')).startDemo(router) : null

  const pinia = createPinia()
  guard(router, pinia)
  onUnauthorized(() => {
    useSessionStore(pinia).lost()
    const next = router.currentRoute.value.fullPath
    void router.push({ name: 'login', query: next && next !== '/' ? { next } : {} })
  })

  if (demo?.start) await router.replace(demo.start)
  createApp(App).use(pinia).use(router).mount('#app')
  demo?.mounted()
}

void boot()
