import type { GroupDTO } from '@/api/types'
import type { RecordDTO } from '@/core/types'

/** Связанные группы по всем путям (F-105): `down` — все вложенные в id
 * («Включает в себя» вглубь), иначе — все, куда id входит; сама id — тоже. */
function reach(groups: GroupDTO[], id: string, down: boolean): Set<string> {
  const byId = new Map(groups.map(g => [g.id, g]))
  const out = new Set([id])
  const queue = [id]
  while (queue.length) {
    const g = byId.get(queue.shift()!)
    for (const next of (down ? g?.children : g?.parents) ?? []) {
      if (!out.has(next)) {
        out.add(next)
        queue.push(next)
      }
    }
  }
  return out
}

/** Группа и все вложенные в неё — нельзя выбрать в «Входит в» (цикл). */
export function descendants(groups: GroupDTO[], id: string): Set<string> {
  return reach(groups, id, true)
}

/** Группа и все, куда она входит, — нельзя выбрать в «Включает в себя» (цикл). */
export function ancestors(groups: GroupDTO[], id: string): Set<string> {
  return reach(groups, id, false)
}

const RECENT = 4

/** Предложения групп (F-04): группы связанных записей, затем последние этого пользователя (F-81); без уже выбранных. */
export function suggestGroups(groups: GroupDTO[], related: string[], recent: string[], selected: string[]): GroupDTO[] {
  const byId = new Map(groups.map(g => [g.id, g]))
  const ids = [...new Set([...related, ...recent])].filter(id => byId.has(id) && !selected.includes(id))
  return ids.slice(0, RECENT).map(id => byId.get(id)!)
}

/** Группы связанных записей (F-04): участники, контейнеры и их участники. */
export function relatedGroups(rec: RecordDTO, records: Record<string, RecordDTO | undefined>): string[] {
  const ids = new Set<string>()
  for (const p of rec.participants) ids.add(p.entity_id)
  for (const p of rec.participates_in) {
    ids.add(p.container_id)
    for (const o of records[p.container_id]?.participants ?? []) ids.add(o.entity_id)
  }
  ids.delete(rec.id)
  const out = new Set<string>()
  for (const id of ids) for (const g of records[id]?.groups ?? []) out.add(g)
  return [...out]
}
