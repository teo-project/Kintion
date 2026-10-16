// Строка отношения в списке дашборда (F-109): слева — участники, справа —
// шаблон и подпись. Чистый TS: какие роли «ведущие» (организация), решается
// по ролям шаблона из пака, без знания конкретных сущностей.

import { roleLabel } from './forms'
import type { Registry } from './registry'
import { recordTitle, tplOf, type Ctx } from './resolver'
import type { RecordDTO, RoleDef, TemplateDef } from './types'

/** Сколько людей показать по именам; остальные — «ещё N». */
export const LIST_PEOPLE = 3

export interface RelationRow {
  /** участники ведущих ролей (организация) — первыми, без формы роли */
  leads: string[]
  /** первые люди с формой роли: «Иван Краник — муж» */
  people: { title: string; label: string }[]
  /** сколько ещё видимых участников не показано (F-07) */
  more: number
}

/**
 * Отношение — запись сущности-контейнера, у которой есть конец: у сущности
 * событий `period.end: null` (F-70). Признак — сущности, а не шаблона:
 * интервальное событие (поездка) остаётся событием. Остальные сущности
 * показываются в списке как раньше.
 */
export function isRelation(ctx: Ctx, tpl: TemplateDef | undefined): boolean {
  const e = tpl ? ctx.reg.entities.get(tpl.entity) : undefined
  if (!e?.container) return false
  return !(e.period && 'end' in e.period && e.period.end == null)
}

const peopleCache = new WeakMap<Registry, string | null>()

/**
 * Сущность «людей»: чаще всех допускается в ролях шаблонов отношений реестра
 * (человек есть почти в каждом, организация — в немногих); ничью разрешают
 * роли событий. Всё равно ничья — не различить, `null`.
 */
export function peopleEntity(ctx: Ctx): string | null {
  const cached = peopleCache.get(ctx.reg)
  if (cached !== undefined) return cached
  // [в ролях отношений, в ролях событий]
  const count = new Map<string, [number, number]>()
  for (const t of ctx.reg.templates.values()) {
    if (!ctx.reg.entities.get(t.definition.entity)?.container) continue
    const i = isRelation(ctx, t.definition) ? 0 : 1
    for (const r of t.definition.roles ?? []) {
      for (const e of new Set(r.entities ?? [])) {
        const c = count.get(e) ?? [0, 0]
        c[i]++
        count.set(e, c)
      }
    }
  }
  const cmp = (a: [number, number], b: [number, number]) => b[0] - a[0] || b[1] - a[1]
  const sorted = [...count.entries()].sort((a, b) => cmp(a[1], b[1]))
  const out = sorted[0] && (!sorted[1] || cmp(sorted[0][1], sorted[1][1]) !== 0) ? sorted[0][0] : null
  peopleCache.set(ctx.reg, out)
  return out
}

/**
 * Ведущие роли шаблона (F-109): роль с `entities`, где нет сущности «людей»
 * («Работодатель» — организация). Людей не различить (`null`) или их нет ни
 * в одной роли — ведущих нет: участники идут в порядке ролей шаблона.
 */
export function leadRoles(tpl: TemplateDef, people: string | null): Set<string> {
  const roles = tpl.roles ?? []
  if (!people || !roles.some(r => !r.entities?.length || r.entities.includes(people))) return new Set()
  return new Set(roles.filter(r => r.entities?.length && !r.entities.includes(people)).map(r => r.key))
}

function lowerFirst(s: string): string {
  return s ? s.charAt(0).toLocaleLowerCase() + s.slice(1) : s
}

/**
 * Участники отношения для строки списка; `null` — не отношение. Видимых
 * участников нет — пустая строка (раскладка отношения сохраняется).
 */
export function relationRow(ctx: Ctx, rec: RecordDTO, limit = LIST_PEOPLE): RelationRow | null {
  const tpl = tplOf(ctx, rec)
  if (!tpl || !isRelation(ctx, tpl)) return null
  const order = new Map((tpl.roles ?? []).map((r: RoleDef, i) => [r.key, i]))
  const lead = leadRoles(tpl, peopleEntity(ctx))
  // участники — только видимые: сервер не отдаёт невидимых, а запись нужна для подписи
  const parts = rec.participants
    .filter(p => ctx.records[p.entity_id])
    .map((p, i) => ({ p, i, rank: order.get(p.role) ?? Infinity }))
    .sort((a, b) => a.rank - b.rank || a.i - b.i)
    .map(x => x.p)
  const leads = parts.filter(p => lead.has(p.role)).map(p => recordTitle(ctx, ctx.records[p.entity_id]!))
  const others = parts.filter(p => !lead.has(p.role))
  const people = others.slice(0, limit).map(p => ({
    title: recordTitle(ctx, ctx.records[p.entity_id]!),
    label: lowerFirst(roleLabel(ctx, rec, p.role, p.entity_id)),
  }))
  return { leads, people, more: others.length - people.length }
}

/** Правая часть строки отношения: «Пара (Брак)»; подпись — только если отличается. */
export function templateWithTitle(templateName: string, title: string): string {
  return title && title !== templateName ? `${templateName} (${title})` : templateName
}
