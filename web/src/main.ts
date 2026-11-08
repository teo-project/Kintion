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
  const pinia = createPinia()
  guard(router, pinia)
  onUnauthorized(() => {
    useSessionStore(pinia).lost()
    const next = router.currentRoute.value.fullPath
    void router.push({ name: 'login', query: next && next !== '/' ? { next } : {} })
  })

  createApp(App).use(pinia).use(router).mount('#app')
}

void boot()
