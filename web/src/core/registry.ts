// Реестр дашборда из GET /api/dashboards/{d}/schema: сущности и действующие
// шаблоны (F-36, F-58). Фронтенд работает только по нему — ни одного id или
// ключа пака в коде (F-52, план §3 п.7).

import type { DateWords } from './dates'
import type { EntityDef, PeriodBoundDef, RecordDTO, SchemaDTO, SchemaTemplateDTO, TemplateDef } from './types'

export interface Registry {
  language: string
  entities: Map<string, EntityDef>
  /** по id шаблона в паке */
  templates: Map<string, SchemaTemplateDTO>
  /** по id строки dashboard_template */
  byRowId: Map<string, SchemaTemplateDTO>
}

export function buildRegistry(schema: SchemaDTO): Registry {
  const reg: Registry = {
    language: schema.language,
    entities: new Map(),
    templates: new Map(),
    byRowId: new Map(),
  }
  for (const e of schema.entities) reg.entities.set(e.id, e)
  for (const t of schema.templates) {
    reg.templates.set(t.key, t)
    reg.byRowId.set(t.id, t)
  }
  return reg
}

/** Шаблон записи: по строке шаблона дашборда, иначе по id шаблона. */
export function templateOf(reg: Registry, rec: Pick<RecordDTO, 'template' | 'template_id'>): TemplateDef | undefined {
  return (reg.byRowId.get(rec.template_id) ?? reg.templates.get(rec.template))?.definition
}

/** Шаблон по умолчанию для сущности (F-62): помеченный сервером или с id сущности. */
export function defaultTemplate(reg: Registry, entity: string): TemplateDef | undefined {
  for (const t of reg.templates.values()) if (t.entity === entity && t.default) return t.definition
  return reg.templates.get(entity)?.definition
}

/** Слова интерфейса, нужные ядру (F-76): из языкового файла приложения. */
export interface Words {
  dates: DateWords
  yes: string
  no: string
  /** запасные подписи системных полей — если в паке их нет */
  systemFields: Record<string, string | undefined>
  /** названия разделов карточки (main, additional, custom) */
  sections: Record<string, string | undefined>
}

export type SystemKey = 'start' | 'end'

/**
 * Подпись системной даты (F-70): шаблон может переопределить или скрыть,
 * иначе — сущность, иначе — языковой файл. `null` — поля нет (скрыто).
 */
export function periodLabel(reg: Registry, tpl: TemplateDef | undefined, key: SystemKey, words: Words): string | null {
  const declared = periodDeclared(reg, tpl, key)
  if (declared === undefined) return words.systemFields[key] ?? key
  if (declared === null) return null
  return typeof declared === 'string' ? declared : declared.name
}

/**
 * Пустая системная дата скрыта и добавляется через «Добавить» (F-99):
 * `{ "show": "filled" }` у начала или конца в period шаблона, иначе сущности.
 */
export function periodHiddenWhenEmpty(reg: Registry, tpl: TemplateDef | undefined, key: SystemKey): boolean {
  const declared = periodDeclared(reg, tpl, key)
  return typeof declared === 'object' && declared !== null && declared.show === 'filled'
}

/** Начало или конец из пака: `null` — скрыто, `undefined` — пак не задал. */
function periodDeclared(reg: Registry, tpl: TemplateDef | undefined, key: SystemKey): PeriodBoundDef | null | undefined {
  const own = tpl?.period
  if (own && key in own) return own[key] ?? null
  const ent = tpl ? reg.entities.get(tpl.entity)?.period : undefined
  if (ent && key in ent) return ent[key] ?? null
  return undefined
}

/**
 * Показывает ли раздел «Связи» других участников записи шаблона (F-93):
 * `links` шаблона, иначе сущности, по умолчанию — да.
 */
export function showsLinks(reg: Registry, tpl: TemplateDef | undefined): boolean {
  if (typeof tpl?.links === 'boolean') return tpl.links
  return (tpl ? reg.entities.get(tpl.entity)?.links : undefined) ?? true
}

/** Вес записей шаблона в «Связях» (F-103): число из пака, иначе — после всех с весом. */
export function linksRank(tpl: TemplateDef | undefined): number {
  const r = tpl?.links_rank
  return typeof r === 'number' && Number.isFinite(r) ? r : Infinity
}

/** Событие без конца — мгновенное: действует только в свою дату (F-70). */
export function isInstant(reg: Registry, tpl: TemplateDef | undefined): boolean {
  return periodDeclared(reg, tpl, 'end') === null
}

/**
 * Подпись прочих системных полей (stage, note, title, …): из файла сущности
 * пака (`system.<ключ>.name`), запасная — из языкового файла (F-76).
 */
export function systemLabel(reg: Registry, entity: string | undefined, key: string, words: Words): string {
  const fromPack = entity ? reg.entities.get(entity)?.system?.[key]?.name : undefined
  return fromPack ?? words.systemFields[key] ?? key
}
