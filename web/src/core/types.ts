// Типы формата паков v0 (docs/pack-format.md) и конверта записи API (план §5).
// Чистый TS: без Vue, DOM и ввода-вывода. Порт types.ts прототипа format-v0,
// данные записей — в форме конверта API, а не хранилища прототипа.

import type { PartialDate } from './dates'

export type { PartialDate }

// ---------- Формат паков ----------

/** Подпись системного поля в файле сущности (F-70, F-76): `system.stage.name`. */
export interface SystemFieldDef {
  name?: string
}

/**
 * Начало или конец в period: строка-подпись или объект с `show` (F-99) —
 * `filled`: пустое скрыто и добавляется через «Добавить», как у полей.
 */
export type PeriodBoundDef = string | { name: string; show?: 'always' | 'filled' }

/** Подписи `start` / `end`; `null` — поля нет (событие без конца — мгновенное, F-70). */
export interface PeriodDef {
  start?: PeriodBoundDef | null
  end?: PeriodBoundDef | null
}

export interface EntityDef {
  format?: string
  source?: string
  id: string
  name?: string
  participant?: boolean
  container?: boolean
  /** к записям сущности привязываются start / end и значения (F-83) */
  anchor?: boolean
  /** раздел «Связи» показывает других участников записей сущности (F-93); нет ключа — true */
  links?: boolean
  period?: PeriodDef
  stages?: string
  system?: Record<string, SystemFieldDef | undefined>
}

/** Набор форм роли: `{ by?: "<адрес признака>", <значение>: "…", default: "…" }` (F-39). */
export interface Forms {
  by?: string
  default: string
  [formKey: string]: string | undefined
}

export interface MetaDef {
  key: string
  name: string
  editor?: string
  [k: string]: unknown
}

export type FieldType =
  | 'text' | 'number' | 'partial_date' | 'bool' | 'choice' | 'ref' | 'image'
  | 'place' | 'phone' | 'email' | 'url' | 'contact'

export interface FieldDef {
  key: string
  type: FieldType
  name: string
  history?: boolean
  section?: string
  multiple?: boolean
  show?: 'always' | 'filled'
  sensitive?: boolean
  options?: OptionDef[]
  /** у выбора: можно вписать своё значение — хранится текстом (F-100) */
  free?: boolean
  default?: unknown
  meta?: MetaDef[]
  /** ссылка на запись: допустимые сущности цели; нет — любые */
  entities?: string[]
  [k: string]: unknown
}

/** Вариант выбора: вид линии в дереве и формы ролей, как у этапа (F-118). */
export interface OptionDef {
  key: string
  name: string
  tree_line?: 'dashed'
  forms?: Record<string, Forms | undefined>
}

/** Участие шаблона в дереве (F-111, F-118); `true` — объект по умолчанию. */
export interface TreeLinkDef {
  /** couple — рядом, siblings — узел над участниками, descent — первая роль выше второй */
  as?: 'couple' | 'siblings' | 'descent'
  /** поле-выбор, вариант которого задаёт вид линии */
  line_by?: string
  /** только в этих этапах */
  stages?: string[]
}

export interface RoleDef {
  key: string
  entities?: string[]
  min?: number
  max?: number | null
  forms: Forms
  [k: string]: unknown
}

export interface StageDef {
  key: string
  name?: string
  next?: string[]
  /** формы по ролям; `by` пишется явно (F-74) */
  forms?: Record<string, Forms | undefined>
  meta?: MetaDef[]
  end_events?: string[]
}

/** Элемент грамматики `title` / `new` / `data` (F-71). */
export interface GrammarElement {
  freeedit?: boolean
  field?: string
  'template-property'?: string
  string?: string
  pattern?: string
  template?: string
  data?: Record<string, GrammarElement | GrammarElement[]>
  if?: Condition
  not?: Condition
}

/**
 * Условие `if` / `not` (F-84): строка — «значение есть»; объект — сравнение
 * (`eq`, `ne`, `gt`, `lt`), функция-значение (`count`) или сочетание
 * (`all`, `any`). Строковый аргумент — всегда адрес (как хранится, без `@`);
 * строковая константа — `{ "const": "female" }`.
 */
export type Condition = string | ConditionObject

/** Аргумент: строка — адрес поля; `{ const }` — константа; число, true / false, null — как есть. */
export type ConditionArg = string | number | boolean | null | { const: unknown } | ConditionObject

export interface ConditionObject {
  eq?: [ConditionArg, ConditionArg]
  ne?: [ConditionArg, ConditionArg]
  gt?: [ConditionArg, ConditionArg]
  lt?: [ConditionArg, ConditionArg]
  count?: string
  all?: Condition[]
  any?: Condition[]
}

export interface QuickAddItem {
  name: string
  for?: Record<string, unknown>
  role: string
  new_role: string
  new?: Record<string, unknown>
  /** предзаполнение создаваемого отношения (F-102): поля, `start` / `end`, `.note` — как `new` */
  relation?: Record<string, unknown>
  stage?: string
  single?: boolean
}

export interface TemplateDef {
  format?: string
  source?: string
  id: string
  entity: string
  name: string
  title?: GrammarElement[]
  quick_fields?: string[]
  fields?: FieldDef[]
  roles?: RoleDef[]
  participants?: { min?: number; max?: number | null }
  stages?: StageDef[]
  end_events?: string[]
  contains?: string[]
  effects?: Record<string, unknown>[]
  period?: PeriodDef
  /** переопределяет `links` сущности (F-93) */
  links?: boolean
  /** вес в «Связях» (F-103): меньше — выше; нет — после всех с весом */
  links_rank?: number
  /** общие свои поля шаблона в дашборде (F-96), значения — `shared.<key>` */
  custom_fields?: FieldDef[]
  ui?: { quick_add?: QuickAddItem[] }
  tree_link?: boolean | TreeLinkDef
  /** подпись узла в дереве (F-116); нет — `title` */
  tree_title?: GrammarElement[]
  /** ключ поля «да/нет» — признак завершения записи (F-126); `end` тоже завершает */
  end_flag?: string
  /** автор пака может добавлять свои свойства (template-property) */
  [extra: string]: unknown
}

// ---------- API: реестр дашборда (GET /api/dashboards/{d}/schema) ----------

export interface SchemaTemplateDTO {
  /** строка dashboard_template */
  id: string
  /** id шаблона в паке */
  key: string
  entity: string
  pack: string
  pack_version: string
  /** шаблон по умолчанию для сущности (F-62) */
  default: boolean
  /** версия правок шаблона в дашборде (общие свои поля, F-96, F-45) */
  rev?: number
  definition: TemplateDef
}

export interface SchemaDTO {
  language: string
  entities: EntityDef[]
  templates: SchemaTemplateDTO[]
}

// ---------- API: конверт записи ----------

/** Системная дата записи (F-70): частичная дата и привязка к записи-событию. */
export interface PeriodDTO {
  value: PartialDate | null
  link: string | null
}

export type Anchor = 'start' | 'end' | 'none'

export interface LinkDTO {
  target: string
  anchor: Anchor
}

/** Значение поля (F-11): значение по типу поля, период, пометка, meta, привязки. */
export interface ValueDTO {
  id: string
  /** строка, число, частичная дата, да/нет, id записи (`null` — цель ссылки не видна) */
  value: unknown
  valid_from: PartialDate | null
  valid_to: PartialDate | null
  note: string | null
  meta: Record<string, unknown> | null
  links: LinkDTO[] | null
}

/** Поле записи: версия и значения в ручном порядке, верхнее — текущее. */
export interface FieldDTO {
  rev: number
  values: ValueDTO[]
}

export interface ParticipantDTO {
  id: string
  entity_id: string
  role: string
  valid_from: PartialDate | null
  valid_to: PartialDate | null
  note: string | null
}

export interface ParticipationDTO {
  id: string
  container_id: string
  role: string
  valid_from: PartialDate | null
  valid_to: PartialDate | null
  note: string | null
}

export interface RecordDTO {
  id: string
  entity: string
  /** id шаблона в паке */
  template: string
  /** строка dashboard_template */
  template_id: string
  start: PeriodDTO | null
  end: PeriodDTO | null
  note: string | null
  title_manual: string | null
  sensitive: boolean
  groups: string[]
  /** описания свободных полей (F-75): та же форма, что поле шаблона; значения — `custom.<key>` */
  custom_fields: FieldDef[]
  fields: Record<string, FieldDTO | undefined>
  revs?: Record<string, number>
  participants: ParticipantDTO[]
  participates_in: ParticipationDTO[]
  deleted: boolean
}

// ---------- API: пакетная операция (POST /api/dashboards/{d}/operations) ----------

export interface ValueIn {
  value: unknown
  valid_from?: PartialDate | null
  valid_to?: PartialDate | null
  note?: string | null
  meta?: Record<string, unknown>
  links?: { target: string; anchor: Anchor }[]
}

export interface PeriodIn {
  value: PartialDate | null
  link?: string | null
}

export interface NewRecordIn {
  /** временный id: на него ссылаются участники и привязки пакета */
  ref: string
  entity?: string
  template?: string
  start?: PeriodIn
  end?: PeriodIn
  note?: string
  fields?: Record<string, ValueIn[]>
  /** метки групп новой записи (F-04) */
  groups?: string[]
}

export interface NewParticipantIn {
  container: string
  entity: string
  role: string
  valid_from?: PartialDate | null
  valid_to?: PartialDate | null
  note?: string | null
}

export interface BatchIn {
  records: NewRecordIn[]
  participants: NewParticipantIn[]
  /** ошибка значения отменяет весь пакет (черновик карточки, F-88) */
  strict?: boolean
}
