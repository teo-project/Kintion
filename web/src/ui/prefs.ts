// Настройки пользователя на сервере: запомненный выбор правки поля с историей
// (F-80) и режим карточек (F-108). Загружаются один раз за сеанс страницы;
// меняются в аккаунте.
import { ref } from 'vue'

import { api } from '@/api/client'
import type { CardMode, HistoryEdit, PrefsDTO } from '@/api/types'

const prefs = ref<PrefsDTO | null>(null)
let loading: Promise<PrefsDTO> | null = null

export async function loadPrefs(): Promise<PrefsDTO> {
  if (prefs.value) return prefs.value
  loading ??= api.prefs().then(p => (prefs.value = p)).finally(() => (loading = null))
  return loading
}

/** Запомненный выбор (F-80); '' — спрашивать. Сервер недоступен — спрашивать. */
export async function historyEditPref(): Promise<HistoryEdit> {
  try {
    return (await loadPrefs()).history_edit
  } catch {
    return ''
  }
}

const DEFAULTS: PrefsDTO = { history_edit: '', card_mode: 'read' }

export async function setHistoryEditPref(v: HistoryEdit): Promise<void> {
  prefs.value = await api.setPrefs({ ...DEFAULTS, ...prefs.value, history_edit: v })
}

/** Режим карточек (F-108); сервер недоступен — «Чтение». */
export async function cardModePref(): Promise<CardMode> {
  try {
    return (await loadPrefs()).card_mode ?? 'read'
  } catch {
    return 'read'
  }
}

export async function setCardModePref(v: CardMode): Promise<void> {
  prefs.value = await api.setPrefs({ ...DEFAULTS, ...prefs.value, card_mode: v })
}

/** Настройки ракурса «Дерево» (F-119): лента по умолчанию включена. */
export async function treePrefs(): Promise<{ ribbon: boolean; hideDead: boolean }> {
  try {
    const p = await loadPrefs()
    return { ribbon: p.tree_ribbon ?? true, hideDead: p.tree_hide_dead ?? false }
  } catch {
    return { ribbon: true, hideDead: false }
  }
}

export async function setTreePrefs(v: { ribbon: boolean; hideDead: boolean }): Promise<void> {
  prefs.value = await api.setPrefs({ ...DEFAULTS, ...prefs.value, tree_ribbon: v.ribbon, tree_hide_dead: v.hideDead })
}

export function usePrefs() {
  return { prefs, loadPrefs, setHistoryEditPref, setCardModePref }
}

/** Только для тестов. */
export function resetPrefs(): void {
  prefs.value = null
  loading = null
}
