// Универсальный резолвер: адресация значений (F-73, F-75) и грамматика
// подписи `title` (F-71). Порт resolver.ts прототипа format-v0 на конверт
// API. Ничего не знает о конкретных паках: всё берётся из шаблона записи.
//
// Адрес: `поле` — как хранится; `@поле` — разрешённое (текст по правилам
// шаблона); `поле[n]` — по истории (`[0]` текущее, `[-1]` самое первое);
// `.часть` — часть значения (`start.year`, `surname.note`), без поля — часть
// самой записи (`.note`); `custom.<ключ>` — свободное поле (F-75);
// `поле:свойство` — свойство описания поля, а не значения (`surname:name`).

import { compareDates, formatDate, isEmptyDate, type PartialDate } from './dates'
import { periodLabel, systemLabel, templateOf, type Registry, type Words } from './registry'
import type {
  Condition, ConditionArg, FieldDef, GrammarElement, ParticipantDTO, PeriodDTO, RecordDTO, RoleDef, StageDef, TemplateDef, ValueDTO,
} from './types'

/** Ключи системных полей записи (F-17, F-37, F-70, F-73). */
export const STAGE = 'stage'
export const NOTE = 'note'
export const CUSTOM_PREFIX = 'custom.'
/** Общие свои поля шаблона в дашборде (F-96): значения — `shared.<key>`. */
export const SHARED_PREFIX = 'shared.'
const START = 'start'
const END = 'end'

export interface Ctx {
  reg: Registry
  /** запись и связанные с ней (`related` ответа API) */
  records: Record<string, RecordDTO | undefined>
  words: Words
  /** сегодня (ISO): «действующее» и `single` считаются на сегодня (Д-20) */
  today: string
}

/** Шаблон записи; нет — запись неактивного или незагруженного шаблона. */
export function tplOf(ctx: Ctx, rec: RecordDTO): TemplateDef | undefined {
  return templateOf(ctx.reg, rec)
}

/** Значения поля записи в ручном порядке (сервер отдаёт их отсортированными). */
export function valuesOf(rec: RecordDTO, key: string): ValueDTO[] {
  return rec.fields[key]?.values ?? []
}

/** Описания свободных полей записи (F-75). */
export function customDef(rec: RecordDTO, key: string): FieldDef | undefined {
  return rec.custom_fields.find(f => f.key === key)
}

// ---------- Адрес ----------

export interface Address {
  resolved: boolean
  /** свободное поле: ключ без префикса `custom.` */
  custom: boolean
  /** общее своё поле шаблона (F-96): ключ без префикса `shared.` */
  shared?: boolean
  /** ключ поля; нет — часть самой записи (`.note`) */
  key?: string
  index?: number
  parts: string[]
  /** свойство описания поля (`:name`) */
  prop?: string
}

const ID = '[A-Za-z_][A-Za-z0-9_]*'
const ADDR_RE = new RegExp(
  `^(@)?(custom\\.|shared\\.)?(${ID})?(?:\\[(-?\\d+)\\])?((?:\\.${ID})*)(?::(${ID}))?$`,
)

export function parseAddress(s: string): Address {
  const m = ADDR_RE.exec(s.trim())
  if (!m) throw new Error(`address: ${s}`)
  const [, at, space, key, index, parts, prop] = m
  if (!key && (index != null || space)) throw new Error(`address: ${s}`)
  return {
    resolved: !!at,
    custom: space === CUSTOM_PREFIX,
    ...(space === SHARED_PREFIX ? { shared: true } : {}),
    key: key || undefined,
    index: index != null ? Number(index) : undefined,
    parts: parts ? parts.slice(1).split('.') : [],
    prop: prop || undefined,
  }
}

// ---------- Слоты: что стоит за ключом ----------

export type Slot =
  | { kind: 'field'; def: FieldDef; values: ValueDTO[] }
  | { kind: 'custom'; def: FieldDef; values: ValueDTO[] }
  | { kind: 'role'; def: RoleDef; parts: ParticipantDTO[] }
  | { kind: 'date'; key: 'start' | 'end'; period: PeriodDTO | null }
  | { kind: 'stage'; stages: StageDef[]; values: ValueDTO[] }
  | { kind: 'none' }

/**
 * Порядок поиска ключа (Д-18): поле шаблона → роль → системные start/end →
 * системный stage. Свободные поля — только по префиксу `custom.` (F-75).
 */
export function findSlot(ctx: Ctx, rec: RecordDTO, a: Pick<Address, 'key' | 'custom' | 'shared'>): Slot {
  const key = a.key
  if (!key) return { kind: 'none' }
  if (a.shared) {
    const def = tplOf(ctx, rec)?.custom_fields?.find(f => f.key === key)
    return def ? { kind: 'custom', def, values: valuesOf(rec, SHARED_PREFIX + key) } : { kind: 'none' }
  }
  if (a.custom) {
    const def = customDef(rec, key)
    return def ? { kind: 'custom', def, values: valuesOf(rec, CUSTOM_PREFIX + key) } : { kind: 'none' }
  }
  const tpl = tplOf(ctx, rec)
  const def = tpl?.fields?.find(f => f.key === key)
  if (def) return { kind: 'field', def, values: valuesOf(rec, key) }
  const role = tpl?.roles?.find(r => r.key === key)
  if (role) return { kind: 'role', def: role, parts: rec.participants.filter(p => p.role === key) }
  if (key === START || key === END) return { kind: 'date', key, period: rec[key] }
  if (key === STAGE && tpl?.stages?.length) return { kind: 'stage', stages: tpl.stages, values: valuesOf(rec, STAGE) }
  return { kind: 'none' }
}

/** Одна «единица» результата адреса: хранимое значение и его текст. */
export interface Item {
  raw: unknown
  text: string
}

export function isDateLike(v: unknown): v is PartialDate {
  return !!v && typeof v === 'object' && !Array.isArray(v) && ('y' in v || 'm' in v || 'd' in v)
}

function isEmpty(v: unknown): boolean {
  if (v == null || v === '') return true
  if (Array.isArray(v)) return v.length === 0
  if (isDateLike(v)) return isEmptyDate(v)
  return false
}

/** Выбор по индексу; без истории любой индекс ведёт к единственному значению (F-73). */
function pickIndex<T>(list: T[], index: number | undefined, single: boolean): T[] {
  if (index == null) return list
  if (single) return list.slice(0, 1)
  const v = list.at(index)
  return v === undefined ? [] : [v]
}

function datePart(d: PartialDate | null | undefined, part: string): number | undefined {
  if (!d) return undefined
  if (part === 'year') return d.y ?? undefined
  if (part === 'month') return d.m ?? undefined
  if (part === 'day') return d.d ?? undefined
  return undefined
}

/** Пометка своего значения выбора в meta значения (F-104). */
export const FREE_META = 'free'

/**
 * Своё значение выбора (F-100, F-104): текст с пометкой `meta.free` — не ключ
 * варианта, даже если с ним совпадает. Так его видят вычислитель (`rawValue`,
 * условия) и редакторы.
 */
export interface FreeText { free: string }

export function isFreeText(v: unknown): v is FreeText {
  return !!v && typeof v === 'object' && !Array.isArray(v) && typeof (v as FreeText).free === 'string' && Object.keys(v).length === 1
}

/** Хранимое значение с учётом пометки: своё — FreeText, прочее — как есть. */
export function storedRaw(v: { value: unknown; meta?: Record<string, unknown> | null }): unknown {
  return v.meta?.[FREE_META] === true && typeof v.value === 'string' ? { free: v.value } : v.value
}

/**
 * Значение для API (F-104): своё — текст и пометка в meta (прочие ключи
 * прежней meta сохраняются); вариант и остальное — как есть, без meta.
 */
export function valueIn(v: unknown, meta?: Record<string, unknown> | null): { value: unknown; meta?: Record<string, unknown> } {
  return isFreeText(v) ? { value: v.free, meta: { ...(meta ?? {}), [FREE_META]: true } } : { value: v }
}

/**
 * Значение выбора со своим значением (F-100) по введённому тексту: совпал с
 * названием варианта (без учёта регистра и пробелов по краям) — ключ
 * варианта, иначе — своё значение (F-104); пусто — null.
 */
export function freeChoiceValue(def: Pick<FieldDef, 'options'>, text: string): string | FreeText | null {
  const s = text.trim().toLocaleLowerCase()
  if (!s) return null
  return def.options?.find(o => o.name.trim().toLocaleLowerCase() === s)?.key ?? { free: text }
}

/** Текст хранимого значения по типу поля (то, что делает `@`). */
export function valueText(ctx: Ctx, def: Pick<FieldDef, 'type' | 'options'> | undefined, raw: unknown, depth = 0): string {
  if (isEmpty(raw)) return ''
  if (isFreeText(raw)) return raw.free // своё значение — текстом, не вариантом (F-104)
  switch (def?.type) {
    case 'choice':
      return def.options?.find(o => o.key === raw)?.name ?? String(raw)
    case 'ref':
      return recordTitleById(ctx, String(raw), depth)
    case 'bool':
      return raw ? ctx.words.yes : ctx.words.no
  }
  return storedText(ctx, raw)
}

/** Текст значения как хранится (без `@`). */
function storedText(ctx: Ctx, raw: unknown): string {
  if (isEmpty(raw)) return ''
  if (isFreeText(raw)) return raw.free
  if (isDateLike(raw)) return formatDate(raw, ctx.words.dates)
  if (typeof raw === 'object') return JSON.stringify(raw)
  return String(raw)
}

/** Часть значения поля: пометка, часть даты, период значения, ключ meta. */
function valuePart(v: ValueDTO, parts: string[]): unknown {
  const [p, ...rest] = parts
  let raw: unknown
  if (p === NOTE) raw = v.note
  else if (isDateLike(v.value) && datePart(v.value, p!) !== undefined) raw = datePart(v.value, p!)
  else if (p === 'from') raw = v.valid_from
  else if (p === 'to') raw = v.valid_to
  else raw = v.meta?.[p!]
  for (const q of rest) raw = raw && typeof raw === 'object' ? (raw as Record<string, unknown>)[q] : undefined
  return raw
}

/** Описание поля за слотом — для `поле:свойство` (F-75). */
function describe(ctx: Ctx, rec: RecordDTO, slot: Slot): Record<string, unknown> | undefined {
  const tpl = tplOf(ctx, rec)
  switch (slot.kind) {
    case 'field':
    case 'custom':
    case 'role':
      return slot.def
    case 'date':
      return { key: slot.key, name: periodLabel(ctx.reg, tpl, slot.key, ctx.words) }
    case 'stage':
      return { key: STAGE, name: systemLabel(ctx.reg, tpl?.entity ?? rec.entity, STAGE, ctx.words) }
  }
  return undefined
}

const MAX_DEPTH = 6

/** Вычислить адрес на записи: список единиц (пусто — значения нет). */
export function evalAddress(ctx: Ctx, rec: RecordDTO, addr: string | Address, depth = 0): Item[] {
  const a = typeof addr === 'string' ? parseAddress(addr) : addr
  if (depth > MAX_DEPTH) return []
  const out: Item[] = []
  const push = (raw: unknown, text: string) => {
    if (!isEmpty(raw) && text !== '') out.push({ raw, text })
  }

  // Части самой записи: `.note`, `.id`.
  if (!a.key) {
    const [p] = a.parts
    if (a.prop) {
      const d: Record<string, unknown> = { key: p, name: p ? systemLabel(ctx.reg, rec.entity, p, ctx.words) : undefined }
      const v = d[a.prop]
      if (typeof v === 'string') push(v, v)
      return out
    }
    if (p === NOTE) push(rec.note, rec.note ?? '')
    else if (p === 'id') push(rec.id, rec.id)
    return out
  }

  const slot = findSlot(ctx, rec, a)
  if (a.prop) {
    const v = describe(ctx, rec, slot)?.[a.prop]
    if (typeof v === 'string' || typeof v === 'number') push(v, String(v))
    return out
  }

  switch (slot.kind) {
    case 'none':
      return out
    case 'field':
    case 'custom': {
      const { def } = slot
      // Без индекса: обычное поле → верхнее значение; multiple → все, текущего нет
      // (F-11, FINDINGS №9). Без истории (и не multiple) любой индекс → единственное.
      let vals = slot.values
      if (a.index == null && !def.multiple) vals = vals.slice(0, 1)
      vals = pickIndex(vals, a.index, !def.history && !def.multiple)
      for (const v of vals) {
        if (a.parts.length === 0) {
          const raw = storedRaw(v)
          push(raw, a.resolved ? valueText(ctx, def, raw, depth) : storedText(ctx, raw))
        } else {
          const raw = valuePart(v, a.parts)
          push(raw, storedText(ctx, raw))
        }
      }
      return out
    }
    case 'date': {
      const p = slot.period
      if (!p) return out
      // start / end — без истории: любой индекс → единственное значение.
      if (a.parts.length === 0) {
        push(p.value, formatDate(p.value, ctx.words.dates))
        return out
      }
      const [part] = a.parts
      if (part === 'link') {
        push(p.link, p.link ? recordTitleById(ctx, p.link, depth) : '')
        return out
      }
      const n = datePart(p.value, part!)
      push(n, n != null ? String(n) : '')
      return out
    }
    case 'stage': {
      const vals = a.index == null ? slot.values.slice(0, 1) : pickIndex(slot.values, a.index, false)
      for (const v of vals) {
        if (a.parts.length === 0) {
          // Своё значение не из набора хранится текстом (F-54) — как есть.
          const st = slot.stages.find(s => s.key === v.value)
          push(v.value, a.resolved ? (st?.name ?? String(v.value)) : String(v.value))
        } else {
          const raw = valuePart({ ...v, value: null }, a.parts)
          push(raw, storedText(ctx, raw))
        }
      }
      return out
    }
    case 'role': {
      for (const p of pickIndex(slot.parts, a.index, false)) {
        if (a.parts.length === 0) {
          push(p.entity_id, a.resolved ? recordTitleById(ctx, p.entity_id, depth) : p.entity_id)
        } else if (a.parts[0] === NOTE) {
          push(p.note, p.note ?? '')
        } else {
          // `роль.адрес` — адрес на записи участника (FINDINGS №15)
          const target = ctx.records[p.entity_id]
          if (target) {
            const inner: Address = { ...parseAddress(a.parts.join('.')), resolved: a.resolved }
            out.push(...evalAddress(ctx, target, inner, depth + 1))
          }
        }
      }
      return out
    }
  }
}

export function hasValue(ctx: Ctx, rec: RecordDTO, addr: string): boolean {
  return evalAddress(ctx, rec, addr).length > 0
}

/** Хранимое значение (первое) — для `new`, `for`, `by`. */
export function rawValue(ctx: Ctx, rec: RecordDTO, addr: string): unknown {
  return evalAddress(ctx, rec, addr)[0]?.raw
}

export function textValue(ctx: Ctx, rec: RecordDTO, addr: string, depth = 0): string {
  return evalAddress(ctx, rec, addr, depth).map(i => i.text).join(', ')
}

// ---------- Грамматика title (F-71) ----------

export function recordTitleById(ctx: Ctx, id: string, depth = 0): string {
  const r = ctx.records[id]
  return r ? recordTitle(ctx, r, depth + 1) : ''
}

/** Есть ли в правиле подписи ручная правка (`freeedit`). */
export function hasFreeEdit(tpl: TemplateDef | undefined): boolean {
  return !!tpl?.title?.some(e => e.freeedit)
}

/**
 * Подпись записи по правилу шаблона. Ручная правка (`freeedit`) показывается
 * вместо остального правила; пустой результат — запасная подпись по имени
 * шаблона.
 */
export function recordTitle(ctx: Ctx, rec: RecordDTO, depth = 0): string {
  const tpl = tplOf(ctx, rec)
  if (!tpl) return ctx.reg.entities.get(rec.entity)?.name ?? ''
  if (hasFreeEdit(tpl) && rec.title_manual) return rec.title_manual
  const s = depth > MAX_DEPTH ? '' : evalElements(ctx, rec, tpl.title ?? [], depth)
  return s.trim() === '' ? tpl.name : s
}

export function evalElements(ctx: Ctx, rec: RecordDTO, els: GrammarElement[] | GrammarElement, depth = 0): string {
  const list = Array.isArray(els) ? els : [els]
  return list.map(e => evalElement(ctx, rec, e, depth)).join('')
}

/** `if` / `not` (F-84): строка — наличие значения, объект — условие. */
function condOk(ctx: Ctx, rec: RecordDTO, e: GrammarElement): boolean {
  if (e.if != null && !evalCondition(ctx, rec, e.if)) return false
  if (e.not != null && evalCondition(ctx, rec, e.not)) return false
  return true
}

/** Условие (F-84): строка — «значение есть» (ключ без `@`), объект — см. Condition. */
export function evalCondition(ctx: Ctx, rec: RecordDTO, c: Condition): boolean {
  if (typeof c === 'string') return hasValue(ctx, rec, c.replace(/^@/, ''))
  if (!c || typeof c !== 'object') return false
  const pair = (args: unknown): [unknown, unknown] | null =>
    Array.isArray(args) && args.length === 2 ? [condArg(ctx, rec, args[0] as ConditionArg), condArg(ctx, rec, args[1] as ConditionArg)] : null
  let p: [unknown, unknown] | null
  if (c.eq !== undefined) return (p = pair(c.eq)) !== null && sameRaw(p[0], p[1])
  if (c.ne !== undefined) return (p = pair(c.ne)) !== null && !sameRaw(p[0], p[1])
  if (c.gt !== undefined) return (p = pair(c.gt)) !== null && (order(p[0], p[1]) ?? 0) > 0
  if (c.lt !== undefined) return (p = pair(c.lt)) !== null && (order(p[0], p[1]) ?? 0) < 0
  if (c.count !== undefined) return countValues(ctx, rec, c.count) > 0
  if (Array.isArray(c.all)) return c.all.every(x => evalCondition(ctx, rec, x))
  if (Array.isArray(c.any)) return c.any.some(x => evalCondition(ctx, rec, x))
  return false
}

/**
 * Значение аргумента (F-84): строка — всегда адрес (хранимое значение,
 * первое), `{ const }` — константа (строка, дата и т. п.), `{ count }` —
 * число значений; числа, true / false, null — константы как есть.
 */
function condArg(ctx: Ctx, rec: RecordDTO, a: ConditionArg): unknown {
  if (typeof a === 'string') return rawValue(ctx, rec, a.replace(/^@/, '')) ?? null
  if (a && typeof a === 'object') {
    if ('const' in a) return a.const ?? null
    if (a.count !== undefined) return countValues(ctx, rec, a.count)
    return evalCondition(ctx, rec, a)
  }
  return a ?? null
}

/** `count`: сколько значений у поля (вся история), участников роли, дат. */
function countValues(ctx: Ctx, rec: RecordDTO, addr: string): number {
  let a: Address
  try {
    a = parseAddress(addr.replace(/^@/, ''))
  } catch {
    return 0
  }
  if (a.key && a.index == null && a.parts.length === 0) {
    const slot = findSlot(ctx, rec, a)
    switch (slot.kind) {
      case 'field':
      case 'custom':
      case 'stage':
        return slot.values.length
      case 'role':
        return slot.parts.length
    }
  }
  return evalAddress(ctx, rec, { ...a, resolved: false }).length
}

/**
 * Равенство хранимых значений по содержимому, без учёта порядка ключей
 * (даты, своё значение выбора — объекты).
 */
export function sameRaw(a: unknown, b: unknown): boolean {
  if (isEmpty(a) && isEmpty(b)) return true
  return JSON.stringify(sorted(a ?? null)) === JSON.stringify(sorted(b ?? null))
}

/** Объект с ключами по порядку — для сравнения через JSON. */
function sorted(v: unknown): unknown {
  if (Array.isArray(v)) return v.map(sorted)
  if (!v || typeof v !== 'object') return v
  const o = v as Record<string, unknown>
  return Object.fromEntries(Object.keys(o).sort().map(k => [k, sorted(o[k])]))
}

/** Порядок для gt / lt: числа и частичные даты; прочее — несравнимо (undefined). */
function order(a: unknown, b: unknown): number | undefined {
  if (typeof a === 'number' && typeof b === 'number') return a - b
  if (isDateLike(a) && isDateLike(b) && !isEmptyDate(a) && !isEmptyDate(b)) return compareDates(a, b)
  return undefined
}

export function evalElement(ctx: Ctx, rec: RecordDTO, e: GrammarElement, depth = 0): string {
  if (!condOk(ctx, rec, e)) return ''
  if (e.freeedit) return '' // обрабатывается в recordTitle
  if (e.field != null) return textValue(ctx, rec, e.field, depth)
  if (e['template-property'] != null) return templateProperty(ctx, rec, e['template-property'], depth)
  if (e.string != null) return e.string
  if (e.pattern != null) return evalPattern(ctx, rec, e.pattern, e.data ?? {}, depth)
  // Образцы из файлов пака не определены (pack-format.md) — заглушка, как в прототипе.
  if (e.template != null) return `⟨${e.template}⟩`
  return ''
}

/** `template-property`: свойство шаблона; `@свойство` разрешает формы `{ by, … }` по данным записи. */
export function templateProperty(ctx: Ctx, rec: RecordDTO, name: string, depth = 0): string {
  const resolved = name.startsWith('@')
  const key = resolved ? name.slice(1) : name
  const tpl = tplOf(ctx, rec)
  const v = tpl?.[key]
  if (v == null) return ''
  if (typeof v === 'string' || typeof v === 'number') return String(v)
  // «правило» — массив элементов грамматики (допущение прототипа)
  if (Array.isArray(v)) return evalElements(ctx, rec, v as GrammarElement[], depth)
  if (typeof v === 'object' && 'default' in v) {
    const forms = v as { by?: string; default: string; [k: string]: string | undefined }
    if (!resolved || !forms.by) return forms.default
    const feature = rawValue(ctx, rec, forms.by)
    return (feature != null ? forms[String(feature)] : undefined) ?? forms.default
  }
  if (typeof v === 'object') return evalElement(ctx, rec, v as GrammarElement, depth)
  return ''
}

/** `pattern`: `{место}` — ключ `data` или адрес; `[...]` выпадает, если пусто хоть одно место внутри. */
export function evalPattern(
  ctx: Ctx, rec: RecordDTO, pattern: string, data: Record<string, GrammarElement | GrammarElement[]>, depth = 0,
): string {
  const place = (name: string): string => {
    const d = data[name]
    return d !== undefined ? evalElements(ctx, rec, d, depth) : textValue(ctx, rec, name, depth)
  }
  const fill = (chunk: string): { text: string; anyEmpty: boolean } => {
    let anyEmpty = false
    const text = chunk.replace(/\{([^{}]+)\}/g, (_, name: string) => {
      const v = place(name)
      if (!v) anyEmpty = true
      return v
    })
    return { text, anyEmpty }
  }
  // Без вложенности и экранирования — их в спецификации нет.
  let out = ''
  const re = /\[([^[\]]*)\]/g
  let last = 0
  let m: RegExpExecArray | null
  while ((m = re.exec(pattern))) {
    out += fill(pattern.slice(last, m.index)).text
    const f = fill(m[1] ?? '')
    if (!f.anyEmpty) out += f.text
    last = m.index + m[0].length
  }
  return out + fill(pattern.slice(last)).text
}
