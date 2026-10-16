// Сессия владельца (F-27, F-28): кто вошёл, нужна ли первичная настройка.
// CSRF-токен из ответов входа и /api/me передаётся клиенту API (Д-13).
import { defineStore } from 'pinia'
import { ref } from 'vue'

import { api, ApiError, setCsrf } from '@/api/client'
import type { AccountDTO, MeDTO } from '@/api/types'

export const useSessionStore = defineStore('session', () => {
  const account = ref<AccountDTO | null>(null)
  const checked = ref(false)
  const setupNeeded = ref(false)

  function apply(me: MeDTO | null) {
    account.value = me?.account ?? null
    setCsrf(me?.csrf ?? '')
  }

  /** Проверить сессию: /api/me; без сессии — нужна ли настройка. */
  async function check(): Promise<void> {
    try {
      apply(await api.me())
      setupNeeded.value = false
    } catch (e) {
      if (!(e instanceof ApiError) || e.status !== 401) throw e
      apply(null)
      setupNeeded.value = (await api.setupStatus()).needed
    }
    checked.value = true
  }

  async function setup(token: string, login: string, password: string) {
    apply(await api.setup(token, login, password))
    setupNeeded.value = false
    checked.value = true
  }

  async function login(login: string, password: string) {
    apply(await api.login(login, password))
    checked.value = true
  }

  async function logout() {
    try {
      await api.logout()
    } finally {
      apply(null)
    }
  }

  /** Сессия пропала (401 на защищённом методе). */
  function lost() {
    apply(null)
  }

  return { account, checked, setupNeeded, check, setup, login, logout, lost }
})
