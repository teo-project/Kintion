// Показ журнала (F-08, F-16): запись журнала → строка «запись · действие» и
// изменения «было → стало» по полям. Подписи — из шаблона записи, если он
// активен (поля, роли, системные даты); иначе — названия колонок из языкового
// файла и сырые ключи (план T25). Знания о конкретном паке здесь нет.
import type { AuditEntryDTO, AuditOperationDTO, GroupDTO, RelatedDTO } from '@/api/types'
import { formatDate, type PartialDate } from '@/core/dates'
import { roleLabel } from '@/core/forms'
import { periodLabel, systemLabel } from '@/core/registry'
import {
  CUSTOM_PREFIX, isDateLike, recordTitle, recordTitleById, SHARED_PREFIX, STAGE, storedRaw, tplOf, valueText, type Ctx,
} from '@/core/resolver'
import type { FieldDef, RecordDTO, TemplateDef } from '@/core/types'
import { fmt, type Messages } from '@/i18n'

/** Одно изменение: подпись, было, стало (пустая строка — не было / не стало). */
export interface Change {
  label: string
  before: string
  after: string
}

export interface EntryView {
  id: string
  /** запись, к истории которой относится изменение (null — нет) */
  recordId: string | null
  /** подпись записи; пусто — запись очищена */
  record: string
  /** запись удалена окончательно: подпись — последнее название по журналу */
  purged: boolean
  /** что изменено: объект (таблица) */
  object: string
  op: string
  changes: Change[]
}

export interface OperationView {
  id: string
  at: string
  actor: string
  /** источник, версии приложения и схемы, request_id (F-16) — подсказкой */
  details: string
  entries: EntryView[]
}

type Snap = Record<string, unknown>

const ENTITY = 'entity'
const FIELD_ENTRY = 'field_entry'
const PARTICIPATION = 'participation'
const GROUP = 'grp'
const GROUP_LINK = 'grp_link'
const LABEL = 'entity_group'

/** Системные колонки записи и их ключи подписей (план §4). */
const ENTITY_KEYS: [string, string][] = [
  ['start_date', 'start'], ['start_link', 'start'], ['end_date', 'end'], ['end_link', 'end'],
  ['note', 'note'], ['title_manual', 'title'], ['sensitive', 'sensitive'], ['custom_fields', ''],
]

/** Части значения поля, кроме самого значения. */
const SORT_KEY = 'sort_key'
const VALUE_PARTS = ['valid_from', 'valid_to', 'note', 'meta', 'links', SORT_KEY] as const

/**
 * Перенос в ручном порядке (F-11): ключ порядка сам по себе ничего не
 * говорит — показывается, что перенесено и куда (выше или ниже).
 */
function moved(a: AuditCtx, label: string, what: string, before: unknown, after: unknown): Change {
  const up = String(after ?? '') < String(before ?? '')
  return { label, before: '', after: `${what} ${up ? '↑' : '↓'} ${up ? a.m.history.moveUp : a.m.history.moveDown}` }
}

/** Описание своего поля для журнала: название, тип, история, несколько значений. */
function customText(a: AuditCtx, v: unknown): string {
  if (!Array.isArray(v)) return ''
  const types = a.m.custom.types as Record<string, string | undefined>
  return v.map(x => {
    const f = x as FieldDef
    const props = [types[f.type] ?? f.type, f.history ? a.m.custom.history : '', f.multiple ? a.m.custom.multiple : '']
    return `${f.name} (${props.filter(Boolean).join(', ')})`
  }).join('; ')
}

function same(a: unknown, b: unknown): boolean {
  return JSON.stringify(a ?? null) === JSON.stringify(b ?? null)
}

function isEmpty(v: unknown): boolean {
  return v == null || v === '' || (Array.isArray(v) && v.length === 0)
    || (typeof v === 'object' && !Array.isArray(v) && Object.keys(v as object).length === 0)
}

function schemaLabel(m: Messages, table: string, col?: string): string {
  const t = (m.schema as Record<string, Record<string, string> | undefined>)[table]
  if (!t) return col ?? table
  return (col ? t[col] : t._table) ?? col ?? table
}

/** Контекст показа операции: ядро, слова, группы и шаблоны записей страницы. */
export interface AuditCtx {
  ctx: Ctx
  m: Messages
  groups: Map<string, GroupDTO>
  /** шаблон по id записи — из снимков записи на странице (для очищенных) */
  templates: Map<string, string>
  /** очищенные записи, восстановленные по журналу, — только для названия (F-94) */
  purged: RelatedDTO
}

export function auditCtx(ctx: Ctx, m: Messages, ops: AuditOperationDTO[], groups: GroupDTO[], purged: RelatedDTO = {}): AuditCtx {
  const templates = new Map<string, string>()
  for (const o of ops) {
    for (const e of o.entries) {
      if (e.object !== ENTITY || !e.entity_id) continue
      const tid = (e.after ?? e.before)?.template_id
      if (typeof tid === 'string') templates.set(e.entity_id, tid)
    }
  }
  return { ctx, m, groups: new Map(groups.map(g => [g.id, g])), templates, purged }
}

function recordOf(a: AuditCtx, id: string | null | undefined): RecordDTO | undefined {
  return id ? a.ctx.records[id] : undefined
}

/** Шаблон записи: из конверта, иначе из снимка записи на странице; неактивный — нет. */
function templateFor(a: AuditCtx, id: string | null): TemplateDef | undefined {
  const rec = recordOf(a, id)
  if (rec) return tplOf(a.ctx, rec)
  const tid = id ? a.templates.get(id) : undefined
  return tid ? a.ctx.reg.byRowId.get(tid)?.definition : undefined
}

function titleOf(a: AuditCtx, id: unknown): string {
  if (typeof id !== 'string' || !id) return ''
  return recordTitleById(a.ctx, id) || purgedTitle(a, id) || a.m.audit.unknownRecord
}

/** Последнее название окончательно удалённой записи (F-94); нет — пусто. */
function purgedTitle(a: AuditCtx, id: string): string {
  const rec = a.purged[id]
  return rec ? recordTitle(a.ctx, rec) : ''
}

/** Текст простого значения снимка. */
function plain(a: AuditCtx, v: unknown): string {
  if (v == null || v === '') return ''
  if (typeof v === 'boolean') return v ? a.ctx.words.yes : a.ctx.words.no
  if (isDateLike(v)) return formatDate(v as PartialDate, a.ctx.words.dates)
  if (typeof v === 'object') return JSON.stringify(v)
  return String(v)
}

function entityChanges(a: AuditCtx, e: AuditEntryDTO): Change[] {
  const tpl = templateFor(a, e.entity_id)
  const entity = tpl?.entity ?? (e.after ?? e.before)?.entity as string | undefined
  const b: Snap = e.before ?? {}
  const af: Snap = e.after ?? {}
  const out: Change[] = []
  for (const [col, key] of ENTITY_KEYS) {
    if (same(b[col], af[col])) continue
    if (e.op === 'create' && isEmpty(af[col])) continue
    let label: string
    if (key === 'start' || key === 'end') label = (tpl && periodLabel(a.ctx.reg, tpl, key, a.ctx.words)) || schemaLabel(a.m, ENTITY, col)
    else if (key) label = systemLabel(a.ctx.reg, entity, key, a.ctx.words)
    else label = schemaLabel(a.m, ENTITY, col)
    const text = (v: unknown) => {
      if (col.endsWith('_link')) return titleOf(a, v)
      if (col === 'custom_fields') return customText(a, v)
      return plain(a, v)
    }
    out.push({ label, before: text(b[col]), after: text(af[col]) })
  }
  return out
}

/** Описание поля по ключу значения: шаблон, свободное поле, этап; null — неизвестно. */
function fieldDef(a: AuditCtx, recId: string | null, key: string): { label: string; def?: Pick<FieldDef, 'type' | 'options'> } {
  const tpl = templateFor(a, recId)
  if (key.startsWith(CUSTOM_PREFIX)) {
    const def = recordOf(a, recId)?.custom_fields.find(f => CUSTOM_PREFIX + f.key === key)
    return def ? { label: def.name, def } : { label: key }
  }
  if (!tpl) return { label: key }
  if (key.startsWith(SHARED_PREFIX)) {
    const def = tpl.custom_fields?.find(f => SHARED_PREFIX + f.key === key)
    return def ? { label: def.name, def } : { label: key }
  }
  if (key === STAGE) {
    const options = (tpl.stages ?? []).map(s => ({ key: s.key, name: s.name ?? s.key }))
    return { label: systemLabel(a.ctx.reg, tpl.entity, STAGE, a.ctx.words), def: { type: 'choice', options } }
  }
  const def = tpl.fields?.find(f => f.key === key)
  return def ? { label: def.name, def } : { label: key }
}

function valueChanges(a: AuditCtx, e: AuditEntryDTO): Change[] {
  const b = e.before
  const af = e.after
  const key = String((af ?? b)?.field ?? '')
  const { label, def } = fieldDef(a, e.entity_id, key)
  // своё значение выбора — по пометке в meta (F-104)
  const raw = (s: Snap) => storedRaw(s as Parameters<typeof storedRaw>[0])
  const val = (s: Snap | null) => (s ? valueText(a.ctx, def, raw(s)) : '')
  const out: Change[] = []
  if (!b || !af || !same(raw(b), raw(af))) out.push({ label, before: val(b), after: val(af) })
  if (b && af) {
    for (const p of VALUE_PARTS) {
      if (same(b[p], af[p])) continue
      if (p === SORT_KEY) {
        out.push(moved(a, `${label} · ${a.m.audit.order}`, valueText(a.ctx, def, raw(af)), b[p], af[p]))
        continue
      }
      const text = (v: unknown) => (p === 'links' ? linksText(a, v) : plain(a, v))
      out.push({ label: `${label} · ${schemaLabel(a.m, FIELD_ENTRY, p)}`, before: text(b[p]), after: text(af[p]) })
    }
  }
  return out
}

function linksText(a: AuditCtx, v: unknown): string {
  if (!Array.isArray(v)) return ''
  return v.map(l => titleOf(a, (l as { target?: unknown }).target)).join(', ')
}

function participationChanges(a: AuditCtx, e: AuditEntryDTO): Change[] {
  const s = (e.after ?? e.before) as Snap
  const container = recordOf(a, s.container_id as string)
  const member = String(s.entity_id ?? '')
  const role = container ? roleLabel(a.ctx, container, String(s.role), member) : String(s.role)
  const who = titleOf(a, member)
  const out: Change[] = []
  if (!e.before || !e.after) {
    out.push({ label: role, before: e.before ? who : '', after: e.after ? who : '' })
    return out
  }
  for (const p of ['valid_from', 'valid_to', 'note', SORT_KEY]) {
    if (same(e.before[p], e.after[p])) continue
    if (p === SORT_KEY) {
      out.push(moved(a, `${role} · ${a.m.audit.order}`, who, e.before[p], e.after[p]))
      continue
    }
    out.push({ label: `${role} · ${who} · ${schemaLabel(a.m, PARTICIPATION, p)}`, before: plain(a, e.before[p]), after: plain(a, e.after[p]) })
  }
  return out
}

function groupName(a: AuditCtx, id: unknown, snap?: Snap | null): string {
  if (snap && typeof snap.name === 'string') return snap.name
  return (typeof id === 'string' && a.groups.get(id)?.name) || ''
}

function groupChanges(a: AuditCtx, e: AuditEntryDTO): Change[] {
  const b = e.before
  const af = e.after
  const out: Change[] = []
  if (!b || !af || b.name !== af.name) {
    out.push({ label: a.m.schema.grp.name, before: b ? String(b.name ?? '') : '', after: af ? String(af.name ?? '') : '' })
  }
  return out
}

/** Вложенность группы (F-105): «<группа> · Входит в» — было/стало. */
function groupLinkChanges(a: AuditCtx, e: AuditEntryDTO): Change[] {
  const s = (e.after ?? e.before) as Snap
  const child = groupName(a, s.child_id) || String(s.child_id ?? '')
  const parent = groupName(a, s.parent_id) || String(s.parent_id ?? '')
  return [{ label: `${child} · ${schemaLabel(a.m, GROUP_LINK, 'parent_id')}`, before: e.before ? parent : '', after: e.after ? parent : '' }]
}

function labelChanges(a: AuditCtx, e: AuditEntryDTO): Change[] {
  const s = (e.after ?? e.before) as Snap
  const name = groupName(a, s.group_id) || String(s.group_id ?? '')
  return [{ label: systemLabel(a.ctx.reg, templateFor(a, e.entity_id)?.entity, 'groups', a.ctx.words), before: e.before ? name : '', after: e.after ? name : '' }]
}

/** Строка журнала для показа. */
export function entryView(a: AuditCtx, e: AuditEntryDTO): EntryView {
  let changes: Change[]
  switch (e.object) {
    case ENTITY: changes = entityChanges(a, e); break
    case FIELD_ENTRY: changes = valueChanges(a, e); break
    case PARTICIPATION: changes = participationChanges(a, e); break
    case GROUP: changes = groupChanges(a, e); break
    case GROUP_LINK: changes = groupLinkChanges(a, e); break
    case LABEL: changes = labelChanges(a, e); break
    default: changes = []
  }
  const ops = a.m.audit.ops as Record<string, string | undefined>
  return {
    id: e.id,
    recordId: e.entity_id,
    record: e.entity_id ? titleOf(a, e.entity_id) : '',
    purged: !!e.entity_id && !recordOf(a, e.entity_id),
    object: schemaLabel(a.m, e.object),
    op: ops[e.op] ?? e.op,
    changes,
  }
}

export function operationView(a: AuditCtx, o: AuditOperationDTO): OperationView {
  return {
    id: o.operation_id,
    at: o.at,
    actor: o.actor_login ? fmt(a.m.audit.by, { login: o.actor_login }) : a.m.audit.system,
    details: [
      [a.m.schema.audit_log.source, o.source],
      [a.m.schema.audit_log.app_version, o.app_version],
      [a.m.schema.audit_log.schema_version, String(o.schema_version)],
      [a.m.schema.audit_log.request_id, o.request_id],
    ].map(([k, v]) => `${k}: ${v}`).join('\n'),
    entries: o.entries.map(e => entryView(a, e)),
  }
}
