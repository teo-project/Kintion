// Дашборды владельца (F-01) и реестр каждого из них (/schema): сущности и
// действующие шаблоны. Интерфейс работает только по реестру (F-52).
import { defineStore } from 'pinia'
import { ref, shallowRef } from 'vue'

import { api } from '@/api/client'
import type { DashboardDTO } from '@/api/types'
import { buildRegistry, type Registry } from '@/core/registry'
import type { SchemaTemplateDTO } from '@/core/types'

const lastKey = 'kintion.dashboard'

export function rememberDashboard(id: string): void {
  try {
    globalThis.localStorage?.setItem(lastKey, id)
  } catch {
    // хранилище недоступно — помнить нечего
  }
}

function lastDashboard(): string | null {
  try {
    return globalThis.localStorage?.getItem(lastKey) ?? null
  } catch {
    return null
  }
}

/** Перечитанный реестр, где шаблоны, сохранённые позже начала чтения (rev
 * больше), взяты из текущего: ответ /schema не откатывает свежую правку. */
function fresher(next: Registry, cur: Registry | undefined): Registry {
  if (!cur) return next
  const newer = [...cur.byRowId.values()].filter(x => {
    const n = next.byRowId.get(x.id)
    return !!n && (x.rev ?? 0) > (n.rev ?? 0)
  })
  if (!newer.length) return next
  const out: Registry = { ...next, templates: new Map(next.templates), byRowId: new Map(next.byRowId) }
  for (const x of newer) {
    out.templates.set(x.key, x)
    out.byRowId.set(x.id, x)
  }
  return out
}

export const useDashboardsStore = defineStore('dashboards', () => {
  const list = ref<DashboardDTO[]>([])
  const loaded = ref(false)
  const registries = new Map<string, Promise<Registry>>()
  /** Загруженные реестры — общий источник для всех экранов: правка шаблона
   * (быстрая форма, общие свои поля) сразу видна везде, без перезагрузки. */
  const loadedRegs = shallowRef(new Map<string, Registry>())

  function put(d: string, r: Registry) {
    const m = new Map(loadedRegs.value)
    m.set(d, r)
    loadedRegs.value = m
  }

  // одна загрузка на всех; созданные здесь дашборды не теряются, если ответ
  // запроса, начатого до создания, придёт позже
  let pending: Promise<DashboardDTO[]> | null = null
  const created = new Map<string, DashboardDTO>()

  function load(force = false): Promise<DashboardDTO[]> {
    if (loaded.value && !force) return Promise.resolve(list.value)
    if (pending) return pending
    const p = api.dashboards().then((res) => {
      const ids = new Set(res.map(d => d.id))
      list.value = [...res, ...[...created.values()].filter(d => !ids.has(d.id))]
      loaded.value = true
      return list.value
    }).finally(() => { pending = null })
    pending = p
    return p
  }

  async function create(title: string): Promise<DashboardDTO> {
    const d = await api.createDashboard(title)
    created.set(d.id, d)
    list.value = [...list.value, d]
    return d
  }

  async function rename(id: string, name: string): Promise<void> {
    const d = await api.renameDashboard(id, name)
    list.value = list.value.map(x => (x.id === id ? d : x))
  }

  /** Дашборд, с которого начать: последний открытый, иначе первый. */
  function initial(): DashboardDTO | undefined {
    const last = lastDashboard()
    return list.value.find(d => d.id === last) ?? list.value[0]
  }

  /** Реестр дашборда (кешируется на сеанс: паки неизменны после старта сервера). */
  function registry(d: string): Promise<Registry> {
    let r = registries.get(d)
    if (!r) {
      const p: Promise<Registry> = api.schema(d).then(buildRegistry).then(x => {
        // ответ сброшенной (forget) загрузки не затирает более новый реестр
        // реестр уже заменён правкой (replaceTemplate) — ждущему отдать не старее него
        if (registries.get(d) !== p) return fresher(x, loadedRegs.value.get(d))
        const merged = fresher(x, loadedRegs.value.get(d))
        if (merged !== x) registries.set(d, Promise.resolve(merged))
        put(d, merged)
        return merged
      })
      p.catch(() => { if (registries.get(d) === p) registries.delete(d) })
      registries.set(d, p)
      r = p
    }
    return r
  }

  /** Загруженный реестр дашборда (реактивно) — экраны следят за ним. */
  function current(d: string): Registry | undefined {
    return loadedRegs.value.get(d)
  }

  /**
   * Обновлённый шаблон (общие свои поля F-96, быстрая форма F-124) — в общий
   * реестр. Основа — текущий реестр в кеше, а не снимок вызывающего: иначе
   * правка одного экрана откатывала бы правку другого. Ответ со старой версией
   * шаблона (rev) не затирает более новый.
   */
  function replaceTemplate(d: string, reg: Registry, dto: SchemaTemplateDTO): Registry {
    const base = loadedRegs.value.get(d) ?? reg
    const old = base.byRowId.get(dto.id)
    if (old && (old.rev ?? 0) > (dto.rev ?? 0)) return base
    const next: Registry = { ...base, templates: new Map(base.templates), byRowId: new Map(base.byRowId) }
    next.templates.set(dto.key, dto)
    next.byRowId.set(dto.id, dto)
    registries.set(d, Promise.resolve(next))
    put(d, next)
    return next
  }

  /** Сбросить кеш реестра дашборда (конфликт версии шаблона): следующий registry() перечитает. */
  function forget(d: string) {
    registries.delete(d)
  }

  return { list, loaded, load, create, rename, initial, registry, current, replaceTemplate, forget }
})
