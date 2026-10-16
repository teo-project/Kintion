// Черновики новых записей (F-88): до первого сохранённого непустого поля
// запись живёт только в интерфейсе — шаблон, группы, связь, из которой её
// создают, и предзаполненные значения. Первое сохранение создаёт запись,
// это значение (и связь) одной пакетной операцией; ушли, ничего не
// заполнив, — на сервере ничего не появляется.
import type { BatchIn, NewRecordIn, RecordDTO } from '@/core/types'

/** Временный id новой записи в пакете черновика (на него ссылаются участия `with`). */
export const DRAFT_REF = 'new'
const PREFIX = 'draft-'

/** Предзаполненное (быстрое добавление): показывается и сохраняется вместе с первым полем. */
export type DraftBase = Pick<NewRecordIn, 'fields' | 'note' | 'start' | 'end'>

export interface DraftSpec {
  dashboard: string
  /** id шаблона в паке */
  template: string
  groups: string[]
  base?: DraftBase
  /** записи и участия, создаваемые вместе с записью (связь, из которой создаём) */
  with?: BatchIn
}

const drafts = new Map<string, DraftSpec>()
let seq = 0

export function isDraftId(id: string): boolean {
  return id.startsWith(PREFIX)
}

/** Завести черновик; id — для адреса карточки `/d/:d/r/:id`. */
export function newDraft(spec: DraftSpec): string {
  const id = `${PREFIX}${Date.now().toString(36)}${(++seq).toString(36)}`
  drafts.set(id, spec)
  return id
}

export function getDraft(id: string): DraftSpec | undefined {
  return drafts.get(id)
}

export function dropDraft(id: string): void {
  drafts.delete(id)
}

/** Запись черновика в форме конверта — для карточки (на сервере её нет). */
export function draftRecord(id: string, spec: DraftSpec, entity: string, templateRow: string): RecordDTO {
  const fields: RecordDTO['fields'] = {}
  for (const [k, vals] of Object.entries(spec.base?.fields ?? {})) {
    fields[k] = {
      rev: 0,
      values: vals.map((v, i) => ({
        id: `${id}-${k}-${i}`, value: v.value, valid_from: v.valid_from ?? null, valid_to: v.valid_to ?? null,
        note: v.note ?? null, meta: v.meta ?? null, links: v.links ?? null,
      })),
    }
  }
  const period = (p: NewRecordIn['start']) => (p ? { value: p.value, link: p.link ?? null } : null)
  return {
    id, entity, template: spec.template, template_id: templateRow,
    start: period(spec.base?.start), end: period(spec.base?.end), note: spec.base?.note ?? null,
    title_manual: null, sensitive: false, groups: [...spec.groups], custom_fields: [], fields, revs: {},
    participants: [], participates_in: [], deleted: false,
  }
}

/** Пакет «запись + первое значение (+ связь)»: part — то, что сохраняют сейчас. */
export function draftCreateBatch(spec: DraftSpec, part: DraftBase): BatchIn {
  const rec: NewRecordIn = { ...spec.base, ...part, ref: DRAFT_REF, template: spec.template }
  const fields = { ...(spec.base?.fields ?? {}), ...(part.fields ?? {}) }
  if (Object.keys(fields).length) rec.fields = fields
  else delete rec.fields
  if (spec.groups.length) rec.groups = spec.groups
  return {
    records: [rec, ...(spec.with?.records ?? [])],
    participants: spec.with?.participants ?? [],
    strict: true,
  }
}
