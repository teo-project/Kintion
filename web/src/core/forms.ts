// Формы ролей (F-39), этапы (F-37, F-74) и «действующее на сегодня» (Д-20).
// Порт части resolver.ts прототипа format-v0. Поправки: `by` в формах этапов
// только явно — наследования от формы роли нет (F-74); событие с
// `period.end: null` — мгновенное (F-70); этап на дату не считается (Q-03).

import { lo, hi, isEmptyDate, type PartialDate } from './dates'
import { isInstant } from './registry'
import { isFreeText, rawValue, STAGE, storedRaw, tplOf, valuesOf, type Ctx } from './resolver'
import type { FieldDef, Forms, OptionDef, RecordDTO, StageDef } from './types'

/** Зарезервированный этап «не выбран» (F-37). */
export const DEFAULT_STAGE = 'default'

/**
 * Форма по особенности участника: `by` — адрес на записи участника, берётся
 * текущее хранимое значение; нет `by` или значения — `default`.
 */
export function pickForm(ctx: Ctx, forms: Forms, participant: RecordDTO | undefined): string {
  if (!forms.by || !participant) return forms.default
  const feature = rawValue(ctx, participant, forms.by)
  if (feature == null) return forms.default
  return forms[String(feature)] ?? forms.default
}

/** Текущий этап: верхнее значение поля этапа, если это ключ этапа шаблона. */
export function currentStage(ctx: Ctx, rel: RecordDTO): StageDef | undefined {
  const stages = tplOf(ctx, rel)?.stages
  if (!stages?.length) return undefined
  const top = valuesOf(rel, STAGE)[0]
  return top ? stages.find(s => s.key === top.value && s.key !== DEFAULT_STAGE) : undefined
}

/** Текущий вариант поля-выбора: верхнее хранимое значение, если это ключ варианта (своё — не вариант, F-104; у `multiple` — нет). */
export function currentOption(ctx: Ctx, rec: RecordDTO, field: FieldDef): OptionDef | undefined {
  if (field.multiple) return undefined // у multiple текущего нет — порядок только для показа
  const top = valuesOf(rec, field.key)[0]
  const raw = top ? storedRaw(top) : undefined
  if (raw == null || isFreeText(raw)) return undefined
  return field.options?.find(o => o.key === raw)
}

/**
 * Формы роли (F-37, F-118): текущий этап → вариант текущего значения
 * поля-выбора (поля по порядку шаблона) → этап `default` → формы самой роли.
 */
export function formsFor(ctx: Ctx, rel: RecordDTO, roleKey: string): Forms | undefined {
  const tpl = tplOf(ctx, rel)
  const role = tpl?.roles?.find(r => r.key === roleKey)
  if (!role) return undefined
  const fromStage = currentStage(ctx, rel)?.forms?.[roleKey]
  if (fromStage) return fromStage
  for (const f of tpl?.fields ?? []) {
    if (f.type !== 'choice') continue
    const forms = currentOption(ctx, rel, f)?.forms?.[roleKey]
    if (forms) return forms
  }
  return tpl?.stages?.find(s => s.key === DEFAULT_STAGE)?.forms?.[roleKey] ?? role.forms
}

/** Подпись участника в роли записи-контейнера — со стороны другого участника (F-39). */
export function roleLabel(ctx: Ctx, rel: RecordDTO, roleKey: string, participantId: string): string {
  const forms = formsFor(ctx, rel, roleKey)
  return forms ? pickForm(ctx, forms, ctx.records[participantId]) : roleKey
}

/** Идёт ли период на дату today: начался и не закончился (нет конца — идёт). */
function within(today: string, from: PartialDate | null | undefined, to: PartialDate | null | undefined): boolean {
  const start = isEmptyDate(from) ? null : lo(from)
  if (start != null && start > today) return false
  if (isEmptyDate(to)) return true
  const end = lo(to)
  return end != null && end > today
}

/**
 * Действует ли запись сегодня (Д-20) и, если задано, участие в ней.
 * Мгновенное событие действует только в свою дату.
 */
export function activeToday(
  ctx: Ctx, rec: RecordDTO, part?: { valid_from: PartialDate | null; valid_to: PartialDate | null },
): boolean {
  const today = ctx.today
  if (part && !within(today, part.valid_from, part.valid_to)) return false
  if (flagEnded(ctx, rec)) return false
  if (isInstant(ctx.reg, tplOf(ctx, rec))) {
    const d = rec.start?.value
    const l = lo(d)
    const h = hi(d)
    return l != null && h != null && l <= today && today <= h
  }
  return within(today, rec.start?.value, rec.end?.value)
}

/**
 * Запись завершена (F-126): заполнен `end` или истинно поле, которое шаблон
 * указал как признак завершения (`end_flag`). Ключа нет — только по `end`.
 */
export function isEnded(ctx: Ctx, rec: RecordDTO): boolean {
  if (!isEmptyDate(rec.end?.value) || rec.end?.link) return true
  return flagEnded(ctx, rec)
}

/** Истинно ли поле `end_flag` шаблона записи (F-126). */
export function flagEnded(ctx: Ctx, rec: RecordDTO): boolean {
  const flag = tplOf(ctx, rec)?.end_flag
  return typeof flag === 'string' && flag !== '' && rawValue(ctx, rec, flag) === true
}
