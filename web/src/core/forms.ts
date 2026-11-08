// Формы ролей (F-39), этапы (F-37, F-74) и «действующее на сегодня» (Д-20).
// Порт части resolver.ts прототипа format-v0. Поправки: `by` в формах этапов
// только явно — наследования от формы роли нет (F-74); событие с
// `period.end: null` — мгновенное (F-70); этап на дату не считается (Q-03).

import { compareKeys, hiKey, isEmptyDate, loKey, type PartialDate } from './dates'
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

/**
 * Идёт ли период «сегодня»: начался и не закончился (нет конца — идёт).
 * Сравнение — по месту эры и году (F-143); «сегодня» без месяца и дня (год
 * мира) — промежуток: начался — не позже его конца, закончился — не позже начала.
 */
function within(ctx: Ctx, from: PartialDate | null | undefined, to: PartialDate | null | undefined): boolean {
  const tLo = loKey(ctx.today, ctx.eras) ?? ''
  const tHi = hiKey(ctx.today, ctx.eras) ?? ''
  const start = isEmptyDate(from) ? null : loKey(from, ctx.eras)
  if (start != null && compareKeys(start, tHi) > 0) return false
  if (isEmptyDate(to)) return true
  const end = loKey(to, ctx.eras)
  return end != null && compareKeys(end, tLo) > 0
}

/**
 * Действует ли запись сегодня (Д-20) и, если задано, участие в ней.
 * Мгновенное событие действует только в свою дату.
 */
export function activeToday(
  ctx: Ctx, rec: RecordDTO, part?: { valid_from: PartialDate | null; valid_to: PartialDate | null },
): boolean {
  if (part && !within(ctx, part.valid_from, part.valid_to)) return false
  if (flagEnded(ctx, rec)) return false
  if (isInstant(ctx.reg, tplOf(ctx, rec))) {
    const d = rec.start?.value
    const l = loKey(d, ctx.eras)
    const h = hiKey(d, ctx.eras)
    const tLo = loKey(ctx.today, ctx.eras)
    const tHi = hiKey(ctx.today, ctx.eras)
    return l != null && h != null && tLo != null && tHi != null && compareKeys(l, tHi) <= 0 && compareKeys(tLo, h) <= 0
  }
  return within(ctx, rec.start?.value, rec.end?.value)
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
