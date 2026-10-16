// Быстрое добавление (+) (F-73, F-74, F-43): меню из ui.quick_add шаблонов
// дашборда, форма из quick_fields шаблона по умолчанию, предзаполнение из
// `new`, одна пакетная операция «новая запись + отношение». Порт quickAdd.ts
// прототипа format-v0; правила Д-19 (занятость роли), Д-20 (на сегодня),
// Д-21 (шаблон новой записи).

import { activeToday, currentStage } from './forms'
import { treeAs, treeLinkOf } from './tree/graph'
import type { Side } from './tree/suggest'
import { defaultTemplate, periodLabel, systemLabel, type Registry, type Words } from './registry'
import { evalElements, isFreeText, NOTE, rawValue, sameRaw, STAGE, tplOf, valueIn, type Ctx } from './resolver'
import type {
  BatchIn, FieldDef, GrammarElement, NewRecordIn, PartialDate, QuickAddItem, RecordDTO, RoleDef, TemplateDef,
} from './types'

export interface QuickAddOption {
  key: string
  template: TemplateDef
  item: QuickAddItem
  visible: boolean
  /** почему пункт скрыт — для отладки формата */
  reason: string
}

const GRAMMAR_KEYS = ['field', 'string', 'pattern', 'template-property', 'template']

function isGrammar(v: unknown): v is GrammarElement {
  return !!v && typeof v === 'object' && !Array.isArray(v) && GRAMMAR_KEYS.some(k => k in v)
}

/** `for`: { адрес: значение } — хранимое значение; `null` — значения нет (Д-19). */
function failsFor(ctx: Ctx, rec: RecordDTO, cond?: Record<string, unknown>): string | null {
  for (const [k, want] of Object.entries(cond ?? {})) {
    const have = rawValue(ctx, rec, k) ?? null
    if (want === null ? have !== null : !sameRaw(have, want)) return `for.${k}: ${JSON.stringify(want)} ≠ ${JSON.stringify(have)}`
  }
  return null
}

/**
 * Занята ли роль для `single` (Д-19): есть действующее сегодня отношение этого
 * шаблона, где запись — в роли `role`, а другой действующий участник в роли
 * `new_role` совпадает с константами `new` и, если указан, с этапом.
 */
function occupiedBy(ctx: Ctx, rec: RecordDTO, tpl: TemplateDef, item: QuickAddItem): RecordDTO | undefined {
  const consts = Object.entries(item.new ?? {}).filter(([, v]) => !isGrammar(v))
  for (const mine of rec.participates_in) {
    if (mine.role !== item.role) continue
    const rel = ctx.records[mine.container_id]
    if (!rel || tplOf(ctx, rel)?.id !== tpl.id || !activeToday(ctx, rel, mine)) continue
    if (item.stage && currentStage(ctx, rel)?.key !== item.stage) continue
    for (const p of rel.participants) {
      if (p.id === mine.id || p.role !== item.new_role || !activeToday(ctx, rel, p)) continue
      const other = ctx.records[p.entity_id]
      if (other && consts.every(([k, v]) => sameRaw(rawValue(ctx, other, k), v))) return other
    }
  }
  return undefined
}

/** Все пункты меню (+) для записи: видимые и скрытые с причиной. */
export function quickAddOptions(ctx: Ctx, rec: RecordDTO): QuickAddOption[] {
  const out: QuickAddOption[] = []
  const myEntity = tplOf(ctx, rec)?.entity ?? rec.entity
  for (const { definition: tpl } of ctx.reg.templates.values()) {
    ;(tpl.ui?.quick_add ?? []).forEach((item, i) => {
      const key = `${tpl.id}#${i}`
      const hide = (reason: string) => out.push({ key, template: tpl, item, visible: false, reason })
      const role = tpl.roles?.find(r => r.key === item.role)
      const newRole = tpl.roles?.find(r => r.key === item.new_role)
      if (!role || !newRole) return hide(`roles ${item.role}/${item.new_role}`)
      // пункт — только для сущностей роли `role` (Д-19)
      if (role.entities && !role.entities.includes(myEntity)) return hide(`entity ${myEntity}`)
      const forFail = failsFor(ctx, rec, item.for)
      if (forFail) return hide(forFail)
      const occ = item.single ? occupiedBy(ctx, rec, tpl, item) : undefined
      if (occ) return hide(`single: ${occ.id}`)
      out.push({ key, template: tpl, item, visible: true, reason: '' })
    })
  }
  return out
}

/** Меню (+): видимые пункты. */
export function quickAddMenu(ctx: Ctx, rec: RecordDTO): QuickAddOption[] {
  return quickAddOptions(ctx, rec).filter(o => o.visible)
}

// ---------- Форма ----------

export type QuickInput =
  | { kind: 'note'; addr: string; label: string }
  | { kind: 'start' | 'end'; addr: string; label: string }
  | { kind: 'field'; addr: string; label: string; def: FieldDef }

export interface QuickDraft {
  option: QuickAddOption
  /** сущности роли new_role; несколько — выбор в форме (Д-21) */
  entities: string[]
  entity: string
  /** шаблон новой записи — по умолчанию для выбранной сущности (F-62) */
  newTemplate: TemplateDef
  inputs: QuickInput[]
  /** значения формы по адресам quick_fields (даты — PartialDate) */
  values: Record<string, unknown>
  /** предзаполнено из `new`, но в форме нет — пишется как есть (F-74: пол не правится) */
  hidden: Record<string, unknown>
  /** предзаполнение отношения из `relation` (F-102): пустое не пишется */
  relation: Record<string, unknown>
  problems: string[]
}

function newRoleOf(option: QuickAddOption): RoleDef {
  const r = option.template.roles?.find(x => x.key === option.item.new_role)
  if (!r) throw new Error(`role ${option.item.new_role}`)
  return r
}

/** Поля быстрой формы шаблона: `quick_fields` (F-73); неизвестные адреса — в problems. */
export function quickInputs(reg: Registry, words: Words, ctpl: TemplateDef, problems: string[] = []): QuickInput[] {
  const inputs: QuickInput[] = []
  for (const addr of ctpl.quick_fields ?? []) {
    if (addr === '.' + NOTE) {
      inputs.push({ kind: 'note', addr, label: systemLabel(reg, ctpl.entity, NOTE, words) })
    } else if (addr === 'start' || addr === 'end') {
      const label = periodLabel(reg, ctpl, addr, words)
      if (label !== null) inputs.push({ kind: addr, addr, label })
    } else {
      const def = ctpl.fields?.find(f => f.key === addr)
      if (def) inputs.push({ kind: 'field', addr, label: def.name, def })
      else problems.push(`quick_fields: ${addr}`)
    }
  }
  return inputs
}

/**
 * Новая запись из быстрой формы: значения по адресам quick_fields (и
 * скрытые); null — ничего не заполнено (пустых записей не бывает, F-88).
 */
export function quickRecord(ref: string, tpl: TemplateDef, inputs: QuickInput[], values: Record<string, unknown>, hidden: Record<string, unknown> = {}): NewRecordIn | null {
  const rec: NewRecordIn = { ref, template: tpl.id, fields: {} }
  let filled = false
  const put = (key: string, val: unknown, visible: boolean) => {
    if (val == null || (typeof val === 'string' && val.trim() === '')) return
    if (key === '.' + NOTE) rec.note = String(val).trim()
    else if (key === 'start' || key === 'end') {
      if (!isDate(val) || (val.y == null && val.m == null && val.d == null)) return
      rec[key] = { value: val }
    } else rec.fields![key] = [valueIn(normalizeText(val))]
    if (visible) filled = true
  }
  for (const [k, v] of Object.entries(hidden)) put(k, v, false)
  for (const inp of inputs) put(inp.addr, values[inp.addr], true)
  return filled ? rec : null
}

/**
 * Предзаполнение `new` / `relation`: константы как есть, элементы грамматики —
 * от записи, у которой нажали (F-74, F-102). Пустое пропускается.
 */
function prefill(ctx: Ctx, rec: RecordDTO, spec: Record<string, unknown> | undefined): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const [k, v] of Object.entries(spec ?? {})) {
    let val: unknown = v
    if (isGrammar(v)) {
      // `{ field }` без @ — хранимое значение (тип сохраняется); прочее — текст по грамматике.
      // Фамилия не склоняется и не автоматизируется (F-74).
      val = v.field != null && !v.field.startsWith('@') && Object.keys(v).length === 1
        ? rawValue(ctx, rec, v.field)
        : evalElements(ctx, rec, v)
    }
    if (val == null || val === '') continue
    out[k] = val
  }
  return out
}

/** Ключ `relation` (F-102): поле шаблона отношения, его `start` / `end` или `.note`. */
function relationKeyOk(ctx: Ctx, tpl: TemplateDef, key: string): boolean {
  if (key === '.' + NOTE) return true
  if (key === 'start' || key === 'end') return periodLabel(ctx.reg, tpl, key, ctx.words) !== null
  return !!tpl.fields?.some(f => f.key === key)
}

export function buildDraft(ctx: Ctx, rec: RecordDTO, option: QuickAddOption, entity?: string): QuickDraft {
  const problems: string[] = []
  const entities = newRoleOf(option).entities ?? []
  const chosen = entity ?? entities[0]
  if (!chosen || !entities.includes(chosen)) throw new Error(`entity ${chosen}`)
  const ctpl = defaultTemplate(ctx.reg, chosen)
  if (!ctpl) throw new Error(`default template for ${chosen}`)

  const inputs = quickInputs(ctx.reg, ctx.words, ctpl, problems)

  const inForm = new Set(inputs.map(i => i.addr))
  const values: Record<string, unknown> = {}
  const hidden: Record<string, unknown> = {}
  for (const [k, val] of Object.entries(prefill(ctx, rec, option.item.new))) {
    if (inForm.has(k)) values[k] = val
    else hidden[k] = val
  }
  const relation: Record<string, unknown> = {}
  for (const [k, val] of Object.entries(prefill(ctx, rec, option.item.relation))) {
    if (relationKeyOk(ctx, option.template, k)) relation[k] = val
    else problems.push(`relation: ${k}`)
  }
  return { option, entities, entity: chosen, newTemplate: ctpl, inputs, values, hidden, relation, problems }
}

/** Строка и своё значение выбора — без крайних пробелов. */
function normalizeText(v: unknown): unknown {
  if (typeof v === 'string') return v.trim()
  return isFreeText(v) ? { free: v.free.trim() } : v
}

function isDate(v: unknown): v is PartialDate {
  return !!v && typeof v === 'object' && ('y' in v || 'm' in v || 'd' in v)
}

/** Значение по ключу предзаполнения / формы в новую запись пакета; пустое не пишется. */
function putValue(rec: NewRecordIn, key: string, val: unknown) {
  if (val == null || val === '') return
  if (key === '.' + NOTE) rec.note = String(val)
  else if (key === 'start' || key === 'end') {
    if (isDate(val) && (val.y != null || val.m != null || val.d != null)) rec[key] = { value: val }
  } else (rec.fields ??= {})[key] = [valueIn(val)]
}

/** Ссылки пакета на новую запись и на отношение. */
export const NEW_REF = 'new'
export const REL_REF = 'rel'

/** Пакет операции «новая запись + отношение» (POST /api/dashboards/{d}/operations). */
export function draftBatch(rec: RecordDTO, draft: QuickDraft, groups: string[] = []): BatchIn {
  const person: NewRecordIn = { ref: NEW_REF, template: draft.newTemplate.id, fields: {} }
  if (groups.length) person.groups = groups
  const put = (key: string, val: unknown) => putValue(person, key, val)
  for (const [k, v] of Object.entries(draft.hidden)) put(k, v)
  for (const inp of draft.inputs) put(inp.addr, draft.values[inp.addr])

  return relationBatch(rec, draft, NEW_REF, [person])
}

/** Отношение пункта (+) между записью и other (ссылка пакета или id): предзаполнение `relation`, этап. */
function relationBatch(rec: RecordDTO, draft: QuickDraft, other: string, before: NewRecordIn[]): BatchIn {
  const { template: tpl, item } = draft.option
  const relation: NewRecordIn = { ref: REL_REF, template: tpl.id }
  for (const [k, v] of Object.entries(draft.relation)) putValue(relation, k, v)
  if (item.stage) relation.fields = { ...relation.fields, [STAGE]: [{ value: item.stage }] }
  return {
    records: [...before, relation],
    participants: [
      { container: REL_REF, entity: rec.id, role: item.role },
      { container: REL_REF, entity: other, role: item.new_role },
    ],
  }
}

/** «Существующий» (F-130): только отношение с выбранной записью, новой записи нет. */
export function existingBatch(rec: RecordDTO, draft: QuickDraft, other: string): BatchIn {
  return relationBatch(rec, draft, other, [])
}

/**
 * Подходит ли существующая запись под пункт (+) (F-130): подсказки учитывают
 * пол пункта — значение `new` поля-выбора (пол у «Добавить мать») не должно
 * противоречить значению записи; пустое не противоречит — недостающее не
 * придумываем (F-42). Остальные `new` (фамилия и т.п.) — только для нового.
 */
export function fitsExisting(ctx: Ctx, option: QuickAddOption, other: RecordDTO): boolean {
  const fields = tplOf(ctx, other)?.fields ?? []
  return Object.entries(option.item.new ?? {}).every(([k, v]) => {
    if (isGrammar(v) || fields.find(f => f.key === k)?.type !== 'choice') return true
    const have = rawValue(ctx, other, k)
    return have == null || sameRaw(have, v)
  })
}

/**
 * Где встанет выбранный в дереве относительно записи (F-130): по роли новой
 * записи в отношении с `tree_link` — ребёнок ниже, родитель выше, пара и
 * братья рядом; отношения вне дерева — null (подсказок по раскладке нет).
 */
export function existingSide(option: QuickAddOption): Side | null {
  const tpl = option.template
  const link = treeLinkOf(tpl)
  if (!link) return null
  const as = treeAs(tpl, link)
  if (as !== 'descent') return 'same'
  const roles = tpl.roles ?? []
  if (roles[1]?.key === option.item.new_role) return 'down'
  if (roles[0]?.key === option.item.new_role) return 'up'
  return null
}
