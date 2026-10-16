import type { Pinia } from 'pinia'
import { createRouter, createWebHashHistory, createWebHistory, type Router, type RouteRecordRaw } from 'vue-router'

import { useSessionStore } from './stores/session'
import AccountView from './views/AccountView.vue'
import AuditView from './views/AuditView.vue'
import DashboardView from './views/DashboardView.vue'
import GroupsView from './views/GroupsView.vue'
import HomeView from './views/HomeView.vue'
import LoginView from './views/LoginView.vue'
import NotFoundView from './views/NotFoundView.vue'
import RecordView from './views/RecordView.vue'
import SetupView from './views/SetupView.vue'
import TrashView from './views/TrashView.vue'

declare module 'vue-router' {
  interface RouteMeta {
    /** маршрут без сессии: настройка и вход */
    guest?: boolean
  }
}

export const routes: RouteRecordRaw[] = [
  { path: '/', name: 'home', component: HomeView },
  { path: '/setup', name: 'setup', component: SetupView, meta: { guest: true, bare: true } },
  { path: '/login', name: 'login', component: LoginView, meta: { guest: true, bare: true } },
  { path: '/account', name: 'account', component: AccountView },
  { path: '/d/:d', name: 'dashboard', component: DashboardView, props: true },
  { path: '/d/:d/groups', name: 'groups', component: GroupsView, props: true },
  { path: '/d/:d/audit', name: 'audit', component: AuditView, props: true },
  { path: '/d/:d/trash', name: 'trash', component: TrashView, props: true },
  { path: '/d/:d/r/:id', name: 'record', component: RecordView, props: true },
  { path: '/:pathMatch(.*)*', name: 'not-found', component: NotFoundView, meta: { guest: true } },
]

/**
 * Охрана маршрутов: без сессии — на настройку (если владельца ещё нет) или
 * на вход; с сессией страницы настройки и входа не нужны.
 */
export function guard(router: Router, pinia: Pinia): void {
  router.beforeEach(async to => {
    const session = useSessionStore(pinia)
    if (!session.checked) {
      try {
        await session.check()
      } catch {
        // сервер недоступен: экран входа покажет ошибку при попытке
      }
    }
    const signedIn = !!session.account
    if (to.name === 'setup') {
      if (signedIn) return { name: 'home' }
      if (session.checked && !session.setupNeeded) return { name: 'login' }
      return true
    }
    if (to.name === 'login') {
      if (signedIn) return { name: 'home' }
      if (session.setupNeeded) return { name: 'setup' }
      return true
    }
    if (to.meta.guest || signedIn) return true
    if (session.setupNeeded) return { name: 'setup' }
    return { name: 'login', query: to.fullPath === '/' ? {} : { next: to.fullPath } }
  })
}

export const router = createRouter({
  // демо-сборка открывается с любого статического сервера: адрес — в #
  history: import.meta.env.VITE_DEMO === '1' ? createWebHashHistory() : createWebHistory(),
  routes,
})
