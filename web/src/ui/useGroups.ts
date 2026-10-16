// Группы дашборда (F-02) — общий список на сеанс: карточка, выбор групп,
// список записей и экран групп видят одно и то же; после изменений —
// reload().
import { ref, type Ref } from 'vue'

import { api } from '@/api/client'
import type { GroupDTO } from '@/api/types'

const cache = new Map<string, { list: Ref<GroupDTO[]>; loading: Promise<void> | null }>()

export function useGroups(d: string) {
  let entry = cache.get(d)
  if (!entry) {
    entry = { list: ref<GroupDTO[]>([]), loading: null }
    cache.set(d, entry)
  }
  const e = entry
  async function reload(): Promise<void> {
    e.list.value = await api.groups(d)
  }
  if (!e.loading) {
    e.loading = reload().catch(() => {
      e.loading = null // повторить при следующем обращении
    })
  }
  return { groups: e.list, reload, ready: e.loading }
}

/** Только для тестов: забыть закешированные группы. */
export function resetGroups(): void {
  cache.clear()
}
