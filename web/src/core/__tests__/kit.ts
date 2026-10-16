// Тестовый набор ядра: реестр из файлов паков (как его отдаёт /schema) и
// конверты записей. Только для тестов.

import { coreWords, messagesFor } from '@/i18n'

import type { PartialDate } from '../dates'
import { buildRegistry } from '../registry'
import type { Ctx } from '../resolver'
import type { EntityDef, ParticipantDTO, RecordDTO, SchemaDTO, TemplateDef, ValueDTO } from '../types'

export const ruFiles = import.meta.glob('../../../../pack-storage/ru/**/*.json', { eager: true, import: 'default' })
const altFiles = import.meta.glob('../../../../testdata/packs/xx/**/*.json', { eager: true, import: 'default' })

/** Реестр из файлов паков: шаблон по умолчанию — с id сущности (F-62). */
function schemaOf(files: Record<string, unknown>, language: string): SchemaDTO {
  const schema: SchemaDTO = { language, entities: [], templates: [] }
  const packOf = new Map<string, string>()
  for (const json of Object.values(files)) {
    const j = json as Record<string, unknown>
    if (Array.isArray(j.files)) {
      packOf.set(String(j.id), String(j.version))
    } else if (typeof j.entity === 'string') {
      const t = j as unknown as TemplateDef
      schema.templates.push({
        id: `row-${t.id}`, key: t.id, entity: t.entity, pack: String(t.source), pack_version: '',
        default: t.id === t.entity, definition: t,
      })
    } else {
      schema.entities.push(j as unknown as EntityDef)
    }
  }
  for (const t of schema.templates) t.pack_version = packOf.get(t.pack) ?? ''
  return schema
}

export const ruSchema = (): SchemaDTO => schemaOf(ruFiles, 'ru')
export const altSchema = (): SchemaDTO => schemaOf(altFiles, 'xx')

export type Val = unknown | { value: unknown; note?: string; meta?: Record<string, unknown>; valid_from?: PartialDate }

export interface RecSpec {
  id: string
  template: string
  fields?: Record<string, Val[]>
  parts?: [role: string, entity: string][]
  start?: PartialDate
  end?: PartialDate
  note?: string
  title?: string
  custom?: RecordDTO['custom_fields']
}

let seq = 0
function value(v: Val): ValueDTO {
  seq++
  const o = v && typeof v === 'object' && 'value' in v ? (v as { value: unknown; note?: string; meta?: Record<string, unknown>; valid_from?: PartialDate }) : { value: v }
  return {
    id: `v${seq}`, value: o.value, valid_from: o.valid_from ?? null, valid_to: null,
    note: o.note ?? null, meta: o.meta ?? {}, links: [],
  }
}

/** Конверты записей; participates_in собирается по participants всех записей. */
export function records(schema: SchemaDTO, specs: RecSpec[]): Record<string, RecordDTO> {
  const out: Record<string, RecordDTO> = {}
  for (const s of specs) {
    const t = schema.templates.find(x => x.key === s.template)
    if (!t) throw new Error(`no template ${s.template}`)
    const fields: RecordDTO['fields'] = {}
    for (const [k, vals] of Object.entries(s.fields ?? {})) fields[k] = { rev: vals.length, values: vals.map(value) }
    const participants: ParticipantDTO[] = (s.parts ?? []).map(([role, entity], i) => ({
      id: `${s.id}-p${i}`, entity_id: entity, role, valid_from: null, valid_to: null, note: null,
    }))
    out[s.id] = {
      id: s.id, entity: t.entity, template: t.key, template_id: t.id,
      start: s.start ? { value: s.start, link: null } : null, end: s.end ? { value: s.end, link: null } : null,
      note: s.note ?? null, title_manual: s.title ?? null, sensitive: false, groups: [],
      custom_fields: s.custom ?? [], fields, participants, participates_in: [], deleted: false,
    }
  }
  for (const c of Object.values(out)) {
    for (const p of c.participants) {
      out[p.entity_id]?.participates_in.push({
        id: p.id, container_id: c.id, role: p.role, valid_from: null, valid_to: null, note: null,
      })
    }
  }
  return out
}

export function ctxOf(schema: SchemaDTO, specs: RecSpec[], today = '2026-10-11'): Ctx {
  return { reg: buildRegistry(schema), records: records(schema, specs), words: coreWords(messagesFor('ru')), today }
}
