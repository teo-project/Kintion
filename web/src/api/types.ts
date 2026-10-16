// Типы API (план §5, Д-11): руками, без генерации. Конверт записи и реестр —
// в web/src/core/types.ts (их использует ядро); здесь — ответы и тела
// запросов остальных методов. Расхождение с сервером ловят эталоны
// testdata/api/*.json (types.test.ts).

import type {
  Anchor, BatchIn, FieldDef, PartialDate, PeriodIn, RecordDTO, SchemaDTO,
} from '@/core/types'

export type { BatchIn, RecordDTO, SchemaDTO }

export interface AccountDTO {
  id: string
  login: string
  is_admin: boolean
}

/** Ответ /api/setup, /api/login, /api/me: аккаунт и CSRF-токен сессии (Д-13). */
export interface MeDTO {
  account: AccountDTO
  csrf: string
}

/** Правка поля с историей на карточке (F-80): заменить, добавить в историю, '' — спрашивать. */
export type HistoryEdit = '' | 'replace' | 'add'

/** Режим карточек (F-108): чтение, правка по месту, сразу форма. */
export type CardMode = 'read' | 'inline' | 'form'

/** Настройки пользователя на сервере (GET/PUT /api/me/prefs). */
export interface PrefsDTO {
  history_edit: HistoryEdit
  card_mode: CardMode
  /** траурная лента в дереве (F-119); нет — да */
  tree_ribbon?: boolean
  /** «Скрыть умерших» в дереве (F-119) */
  tree_hide_dead?: boolean
}

/** GET /api/dashboards/{d}/tree: отношения с tree_link и их участники. */
export interface TreeOut {
  relations: RecordDTO[]
  records: RecordDTO[]
}

export interface SetupStatusDTO {
  needed: boolean
}

export interface DashboardDTO {
  id: string
  name: string
  created_at: string
}

/** Связанные записи для подписей: замыкание на 2 шага (Д-12). */
export type RelatedDTO = Record<string, RecordDTO | undefined>

export interface RecordOut {
  record: RecordDTO
  related: RelatedDTO
}

export interface ListOut {
  items: RecordDTO[]
  total: number
  related: RelatedDTO
}

export interface ListQuery {
  entity?: string
  group?: string
  q?: string
  deleted?: boolean
  limit?: number
  offset?: number
}

/** Значение поля, не сохранённое при создании записи (F-15). */
export interface FieldErrorDTO {
  ref: string
  field: string
  message: string
}

export interface OperationOut {
  operation_id: string
  ids: Record<string, string>
  records: RecordDTO[]
  related: RelatedDTO
  errors: FieldErrorDTO[]
}

export interface GroupDTO {
  id: string
  name: string
  count: number
  /** «Входит в» — видимые группы, в порядке списка (F-105) */
  parents: string[]
  /** «Включает в себя» */
  children: string[]
  /** автодобавление записей по фамилии (F-122) */
  auto_surname: boolean
  surnames: string[]
}

/** Тело ответа с ошибкой: общая, проверки поля (`field`) или конфликт версии (`current`, F-45). */
export interface ErrorBody {
  error: string
  message?: string
  field?: string
  current?: unknown
}

// ---------- Тела запросов ----------

export interface LinkIn {
  target: string
  anchor: Anchor
}

/** Место значения: над before_id, под after_id, наверх или вниз. */
export interface PositionIn {
  before_id?: string
  after_id?: string
  position?: 'top' | 'bottom'
}

export interface AddValueIn extends PositionIn {
  if_rev: number
  value: unknown
  valid_from?: PartialDate | null
  valid_to?: PartialDate | null
  note?: string | null
  meta?: Record<string, unknown>
  links?: LinkIn[]
}

/** Изменить и/или переместить значение: переданы только меняемые ключи. */
export interface PatchValueIn extends PositionIn {
  if_rev: number
  value?: unknown
  valid_from?: PartialDate | null
  valid_to?: PartialDate | null
  note?: string | null
  meta?: Record<string, unknown> | null
  links?: LinkIn[] | null
}

/** Ровно одно системное поле за запрос (F-45). */
export interface SystemPatchIn {
  if_rev: number
  start?: PeriodIn | null
  end?: PeriodIn | null
  note?: string | null
  title_manual?: string | null
  sensitive?: boolean
  custom_fields?: FieldDef[]
}

/** Участник контейнера: в конец роли или на место (F-45 — версия роли). */
export interface AddParticipantIn extends PositionIn {
  if_rev: number
  entity: string
  role: string
  valid_from?: PartialDate | null
  valid_to?: PartialDate | null
  note?: string | null
}

export interface PatchParticipantIn extends PositionIn {
  if_rev: number
  valid_from?: PartialDate | null
  valid_to?: PartialDate | null
  note?: string | null
}

export interface GroupIn extends PositionIn {
  name?: string
  /** «Входит в» целиком (F-105); нет ключа — без изменений */
  parents?: string[]
  /** «Включает в себя» целиком */
  children?: string[]
  auto_surname?: boolean
  surnames?: string[]
}

/** Загруженный файл: значение поля image — его sha256 (F-29). */
export interface FileDTO {
  sha256: string
  mime: string
  size: number
  width: number | null
  height: number | null
  created_at: string
}

// ---------- Журнал (F-08, F-16) ----------

/** Одно изменение объекта: было → стало (снимки — как пишет сервер). */
export interface AuditEntryDTO {
  id: string
  at: string
  entity_id: string | null
  /** таблица объекта: entity, field_entry, participation, grp, entity_group, … */
  object: string
  object_id: string | null
  op: string
  before: Record<string, unknown> | null
  after: Record<string, unknown> | null
}

/** Операция: все записи журнала с общим operation_id. */
export interface AuditOperationDTO {
  operation_id: string
  at: string
  actor_id: string | null
  actor_login: string | null
  source: string
  request_id: string
  app_version: string
  schema_version: number
  entries: AuditEntryDTO[]
}

/** Страница журнала: новые операции сверху; `next` — курсор следующей страницы. */
export interface AuditOut {
  operations: AuditOperationDTO[]
  next: string | null
  /** упомянутые записи (корзина тоже; очищенных нет) */
  records: RelatedDTO
  related: RelatedDTO
  /** окончательно удалённые записи страницы по журналу — для их последнего названия (F-94) */
  purged?: RelatedDTO
}

export interface AuditQuery {
  entity?: string
  operation?: string
  /** автор: id аккаунта или `system` */
  actor?: string
  /** действие: create, update, delete, restore, purge, move */
  op?: string
  before?: string
  limit?: number
}
