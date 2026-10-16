// Проверки значения перед сохранением (на клиенте — только очевидное;
// правила шаблона проверяет сервер, F-15).
import { isEmptyDate, validateDate, type PartialDate } from '@/core/dates'
import { isDateLike, isFreeText } from '@/core/resolver'
import type { FieldDef } from '@/core/types'

/** Значения нет: пусто, пустая строка, пустая дата. */
export function isBlank(v: unknown): boolean {
  if (v == null) return true
  if (typeof v === 'string') return v.trim() === ''
  if (isFreeText(v)) return v.free.trim() === ''
  if (isDateLike(v)) return isEmptyDate(v)
  return false
}

/** Значение заведомо неверно: не число, несуществующая дата. */
export function isInvalid(def: Pick<FieldDef, 'type'>, v: unknown): boolean {
  if (v == null) return false
  if (def.type === 'number') return typeof v !== 'number' || !Number.isFinite(v)
  if (def.type === 'partial_date') return validateDate(v as PartialDate) !== null
  return false
}

/** Значение для API: строки (и своё значение выбора) — без крайних пробелов. */
export function normalize(v: unknown): unknown {
  if (isFreeText(v)) return { free: v.free.trim() }
  return typeof v === 'string' ? v.trim() : v
}

export function sameValue(a: unknown, b: unknown): boolean {
  return JSON.stringify(normalize(a) ?? null) === JSON.stringify(normalize(b) ?? null)
}
