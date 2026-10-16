// Участники контейнера по ролям шаблона (F-10, F-39): пределы min / max
// (Д-22), цели полей-ссылок по `contains` (F-82), привязки к записям с
// `anchor` (F-83) и события окончания по `end_events` (F-57, Д-28). Чистый TS: всё — из шаблона, без знания конкретных паков.

import { currentStage } from './forms'
import type { Registry } from './registry'
import { tplOf, type Ctx } from './resolver'
import type { RecordDTO, RoleDef, TemplateDef } from './types'

/** Ключ версии участников роли (F-45): `participants:<роль>`. */
export function participantsRev(role: string): string {
  return `participants:${role}`
}

export interface RoleLimit {
  role: RoleDef
  count: number
  /** можно добавить ещё одного: не превышен max роли и шаблона */
  canAdd: boolean
  /** можно убрать одного: не станет меньше min роли и шаблона */
  canRemove: boolean
  /** сколько ещё нужно до min роли */
  missing: number
}

export interface Limits {
  roles: RoleLimit[]
  total: number
  /** сколько ещё нужно до `participants.min` шаблона сверх ролей */
  missingTotal: number
  /** все min выполнены — запись можно создать (Д-22) */
  complete: boolean
}

/** Пределы по числу участников в ролях (считаются только видимые, F-07). */
export function limits(tpl: TemplateDef, roles: string[]): Limits {
  const byRole = new Map<string, number>()
  for (const r of roles) byRole.set(r, (byRole.get(r) ?? 0) + 1)
  const total = roles.length
  const tMin = tpl.participants?.min ?? 0
  const tMax = tpl.participants?.max ?? null
  const out: RoleLimit[] = (tpl.roles ?? []).map(role => {
    const count = byRole.get(role.key) ?? 0
    const min = role.min ?? 0
    const max = role.max ?? null
    return {
      role, count,
      canAdd: (max == null || count < max) && (tMax == null || total < tMax),
      canRemove: count > min && total > tMin,
      missing: Math.max(0, min - count),
    }
  })
  const missingRoles = out.reduce((n, r) => n + r.missing, 0)
  const missingTotal = Math.max(0, tMin - total - missingRoles)
  return { roles: out, total, missingTotal, complete: missingRoles === 0 && missingTotal === 0 }
}

/**
 * Может ли запись этого шаблона стоять в роли: сущность из `entities` роли,
 * с `participant` и не контейнер — вложенность задаётся полем-ссылкой (F-82).
 */
export function fitsRole(reg: Registry, role: RoleDef, tpl: TemplateDef | undefined): boolean {
  const e = tpl ? reg.entities.get(tpl.entity) : undefined
  if (!tpl || e?.participant === false || e?.container) return false
  return !role.entities?.length || role.entities.includes(tpl.entity)
}

/** Кандидат в участники роли: подходит по роли, не сама запись. */
export function canJoin(ctx: Ctx, _container: TemplateDef, role: RoleDef, cand: RecordDTO, self?: string): boolean {
  if (cand.id === self) return false
  return fitsRole(ctx.reg, role, tplOf(ctx, cand))
}

/**
 * Цель поля-ссылки (F-82): сущность из `entities` поля (нет — любая), а
 * запись-контейнер — только шаблона из `contains` шаблона записи (нет ключа — любого).
 */
export function refAllowed(
  reg: Registry, holder: TemplateDef | undefined, entities: string[] | undefined, target: TemplateDef | undefined,
): boolean {
  if (!target) return false
  if (entities?.length && !entities.includes(target.entity)) return false
  if (!reg.entities.get(target.entity)?.container || !holder?.contains) return true
  return holder.contains.includes(target.id)
}

/** Сущности, к записям которых привязываются даты и значения (F-83: `"anchor": true`). */
export function anchorEntities(reg: Registry): string[] {
  return [...reg.entities.values()].filter(e => e.anchor).map(e => e.id)
}

/**
 * Шаблоны, которыми может закончиться запись (F-57, Д-28): `end_events`
 * текущего этапа, иначе шаблона; `undefined` — ограничений нет.
 */
export function endEvents(ctx: Ctx, rec: RecordDTO): string[] | undefined {
  return currentStage(ctx, rec)?.end_events ?? tplOf(ctx, rec)?.end_events
}
