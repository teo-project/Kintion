// Эры текущего дашборда для ввода даты (F-143): дашборд — из адреса (/d/:d/…),
// эры — из его загруженного реестра. Вне дашборда или до загрузки — нет.
import { getActivePinia } from 'pinia'
import { computed, inject, type ComputedRef } from 'vue'
import { routeLocationKey } from 'vue-router'

import type { EraInput } from '@/components/PartialDateInput.vue'
import { useDashboardsStore } from '@/stores/dashboards'

export function useDashboardEras(): ComputedRef<EraInput | undefined> {
  const route = inject(routeLocationKey, null)
  const store = getActivePinia() ? useDashboardsStore() : null
  return computed(() => {
    const d = route?.params.d
    const reg = typeof d === 'string' && store ? store.current(d) : undefined
    if (!reg) return undefined
    return { edit: reg.eras.edit, default: reg.eras.default, list: reg.eras.list, years: reg.eras.years }
  })
}
