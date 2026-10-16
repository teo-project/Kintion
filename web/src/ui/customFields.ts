// Свободные поля записи (F-61, F-75): описание — в custom_fields записи (та же
// форма, что поле шаблона), значения — под ключом `custom.<key>`. Ключ
// создаётся из названия: латиница, цифры и `_` (как требует сервер);
// названия без латиницы получают ключ `f_<n>`.
// Типы (F-96): «выбор» — варианты задаются при создании, «изображение» —
// загрузка с одной свободной обрезкой, ссылка — с ограничением сущностью;
// «контакта» нет. «Выбор или своё» (F-100) — тип choice с флагом free.
import type { FieldDef, FieldType, MetaDef } from '@/core/types'
import { CROP_EDITOR } from './crop'

/** Типы, которые можно выбрать для своего поля. */
export const CUSTOM_TYPES: FieldType[] = ['text', 'number', 'partial_date', 'bool', 'choice', 'ref', 'image', 'phone', 'email', 'url', 'place']

/** Вид своего поля в форме: тип или «выбор или своё» (choice + free, F-100). */
export type CustomKind = FieldType | 'choice_free'
export const CUSTOM_KINDS: CustomKind[] = ['text', 'number', 'partial_date', 'bool', 'choice', 'choice_free', 'ref', 'image', 'phone', 'email', 'url', 'place']

/** Вид описания поля. */
export function kindOf(def: Pick<FieldDef, 'type' | 'free'>): CustomKind {
  return def.type === 'choice' && def.free ? 'choice_free' : def.type
}

/** Тип и флаг free по виду. */
export function fromKind(kind: CustomKind): { type: FieldType; free: boolean } {
  return kind === 'choice_free' ? { type: 'choice', free: true } : { type: kind, free: false }
}

/** meta обрезки своего изображения: одна свободная (место «карточка», F-96). */
export function freeCropMeta(name: string): MetaDef {
  return { key: 'crop', name, editor: CROP_EDITOR, places: ['card'] }
}

const MAX_KEY = 64

/** Ключ нового поля по названию, не совпадающий с имеющимися. */
export function customKey(name: string, taken: string[]): string {
  const base = name.toLowerCase().normalize('NFKD').replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '').slice(0, MAX_KEY - 4)
  const used = new Set(taken)
  if (base && !used.has(base)) return base
  const stem = base || 'f'
  for (let n = 1; ; n++) {
    const k = `${stem}_${n}`
    if (!used.has(k)) return k
  }
}

export interface CustomDraft {
  name: string
  type: FieldType
  /** у выбора: можно вписать своё (F-100) */
  free?: boolean
  history: boolean
  multiple: boolean
  /** варианты выбора — по строке на вариант */
  options?: string
  /** ссылка: допустимая сущность цели; '' — любая */
  entity?: string
  /** подпись meta обрезки изображения */
  cropName?: string
}

/**
 * Варианты выбора из строк. Ключ прежнего варианта сохраняется: по тому же
 * названию, иначе по месту строки (переименование — тот же ключ, значения не
 * теряют подпись); новый — свободный ключ из названия.
 */
export function choiceOptions(text: string, prev: FieldDef['options'] = []): { key: string; name: string }[] {
  const names: string[] = []
  for (const line of text.split('\n')) {
    const name = line.trim()
    if (name && !names.includes(name)) names.push(name)
  }
  const keys: (string | undefined)[] = names.map(n => prev.find(o => o.name === n)?.key)
  const used = new Set(keys.filter(Boolean))
  names.forEach((_, i) => {
    const old = prev[i]
    if (!keys[i] && old && !used.has(old.key) && !names.includes(old.name)) {
      keys[i] = old.key
      used.add(old.key)
    }
  })
  const taken = prev.map(o => o.key)
  const out: { key: string; name: string }[] = []
  names.forEach((name, i) => {
    out.push({ key: keys[i] ?? customKey(name, [...taken, ...out.map(o => o.key)]), name })
  })
  return out
}

/** Описание поля из черновика: только заданные свойства (history/multiple — если включены). */
export function customDef(key: string, d: CustomDraft, prev?: FieldDef): FieldDef {
  const def: FieldDef = { ...(prev ?? {}), key, type: d.type, name: d.name.trim() }
  if (d.history) def.history = true
  else delete def.history
  if (d.multiple) def.multiple = true
  else delete def.multiple
  if (d.type === 'choice') def.options = choiceOptions(d.options ?? '', prev?.options)
  else delete def.options
  if (d.type === 'choice' && d.free) def.free = true
  else delete def.free
  // ограничение ссылки: не трогали — прежний список (их может быть несколько)
  const kept = prev?.type === 'ref' && prev.entities?.length && d.entity === prev.entities[0] ? prev.entities : null
  if (d.type === 'ref' && kept) def.entities = kept
  else if (d.type === 'ref' && d.entity) def.entities = [d.entity]
  else delete def.entities
  if (d.type === 'image') def.meta = prev?.meta?.length ? prev.meta : [freeCropMeta(d.cropName ?? 'crop')]
  else delete def.meta
  return def
}

/** Список описаний после добавления, замены (по ключу) или удаления (def = null). */
export function withCustom(list: FieldDef[], key: string, def: FieldDef | null): FieldDef[] {
  const i = list.findIndex(f => f.key === key)
  if (def === null) return list.filter(f => f.key !== key)
  if (i < 0) return [...list, def]
  return list.map(f => (f.key === key ? def : f))
}
