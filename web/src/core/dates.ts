// Частичная дата (F-14, docs/concepts/dates.md): проверка, границы, точная дата,
// возраст, годовщина, ключ сортировки (Д-9), вид для показа.
// Год — целое со знаком без 0 (отрицательный — до начала эры), эра — необязательная
// надпись при годе или одна, без месяца и дня («мезозойская эра»); пусто — эра по
// умолчанию дашборда (её ставит сервер); порядок эр — настройки дашборда (F-143).
// Те же правила — в PostgreSQL (домен partial_date, функции pd_*) и Go
// (server/internal/partialdate); общие кейсы — testdata/partial_dates.json.
// Операторов «раньше / возможно раньше» нет (Q-07, В-13).

/** Частичная дата: каждая часть необязательна, хотя бы одна из y, m, d, e есть. ISO-даты — 'ГГГГ-ММ-ДД'. */
export interface PartialDate {
  y?: number | null
  m?: number | null
  d?: number | null
  /** эра; нет — эра по умолчанию (F-14), в показе — наша */
  e?: string | null
}

/**
 * Порядок эр дашборда (F-143): эры по порядку и эры с обратным счётом
 * (минимальный год больше максимального — больший год раньше). Можно передать
 * просто список.
 */
export interface EraOrder {
  order: readonly string[]
  reversed?: readonly string[]
}
export type EraOrderLike = EraOrder | readonly string[]

const eraOrderOf = (o: EraOrderLike = []): EraOrder => ('order' in o ? o : { order: o as readonly string[] })

/** Счёт в эре обратный: минимальный год больше максимального (15000 … 1). */
export const eraReversed = (y?: { min?: number | null; max?: number | null } | null): boolean =>
  y?.min != null && y.max != null && y.min > y.max

/** Порядок эр из настроек дашборда (F-143). */
export function eraOrder(s: { order: readonly string[]; years?: Record<string, { min?: number | null; max?: number | null }> }): EraOrder {
  return { order: s.order, reversed: Object.entries(s.years ?? {}).filter(([, y]) => eraReversed(y)).map(([e]) => e) }
}

export type DateError = 'empty' | 'year' | 'month' | 'day' | 'no_such_day' | 'era'

/** Слова вида даты — из языкового файла (F-76, раздел dates). */
export interface DateWords {
  monthsGenitive: string[]
  monthsNominative: string[]
  dayMonth: string
  monthYear: string
  dayOnly: string
  withYear: string
  dayWithYear: string
  /** год нашей эры: положительный и отрицательный ({y} — без знака) */
  yearPositive: string
  yearNegative: string
  /** положительный год нашей эры, когда эра показывается: «1985 н. э.» */
  yearOursPositive: string
  /** год до начала эры у даты без эры (пустая при непустой эре по умолчанию): «−500» */
  yearNoEraNegative: string
  /** год заданной эры ({era}) */
  yearEraPositive: string
  yearEraNegative: string
}

/** Как показывать дату: эра — по настройке дашборда (F-143). */
export interface DateView {
  showEra?: boolean
  /** пустая эра — «без эры», а не наша: эра по умолчанию дашборда непустая (F-14) */
  noEra?: boolean
}

export const MAX_ERA_LEN = 64
const INT32_MIN = -2147483648
const INT32_MAX = 2147483647
/** Пределы календарной даты (date PostgreSQL): вне них lo/hi/exact и годовщины не считаются. */
const MIN_CAL_YEAR = -4713
const MAX_CAL_YEAR = 5874897

const has = (v: number | null | undefined): v is number => v != null

/** Эра даты: пусто — наша. */
export const eraOf = (p?: PartialDate | null): string => p?.e ?? ''

export function isEmptyDate(p?: PartialDate | null): boolean {
  return !p || (!has(p.y) && !has(p.m) && !has(p.d) && !p.e)
}

/** Дата из одной эры, без года, месяца и дня («мезозойская эра»). */
export function isEraOnly(p?: PartialDate | null): boolean {
  return !!p && !!p.e && !has(p.y) && !has(p.m) && !has(p.d)
}

/** Астрономический год: −1 → 0 (года 0 у дат нет). */
export const astro = (y: number): number => (y < 0 ? y + 1 : y)

function isLeap(y: number): boolean {
  return y % 4 === 0 && (y % 100 !== 0 || y % 400 === 0)
}

/** Число дней в месяце; без года у февраля 29. Високосность — по астрономическому году. */
export function daysIn(y: number | null | undefined, m: number): number {
  if (m === 2) return !has(y) || isLeap(astro(y)) ? 29 : 28
  return [4, 6, 9, 11].includes(m) ? 30 : 31
}

const isInt = (v: number) => Number.isInteger(v)

/** Пробелы по краям эры и управляющие символы — как unicode.IsSpace / IsControl в Go. */
const ERA_EDGE_SPACE = /^[ \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]|[ \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]$/
const ERA_CONTROL = /[\u0000-\u001f\u007f-\u009f]/

/** Допустимый текст эры (непустой): до 64 символов, без пробелов по краям и управляющих символов. */
export function validEra(e: string): boolean {
  return e !== '' && [...e].length <= MAX_ERA_LEN && !ERA_CONTROL.test(e) && !ERA_EDGE_SPACE.test(e)
}

/**
 * Сравнение ключей по кодовым точкам — как побайтно в UTF-8 (Go, COLLATE "C"):
 * строки JS сравниваются по UTF-16, и эры вне BMP легли бы иначе.
 */
export function compareKeys(a: string, b: string): number {
  if (a === b) return 0
  const x = [...a]
  const y = [...b]
  for (let i = 0; i < Math.min(x.length, y.length); i++) {
    const cx = x[i]!.codePointAt(0)!
    const cy = y[i]!.codePointAt(0)!
    if (cx !== cy) return cx < cy ? -1 : 1
  }
  return x.length < y.length ? -1 : x.length > y.length ? 1 : 0
}

export function validateDate(p: PartialDate): DateError | null {
  if (isEmptyDate(p)) return 'empty'
  if (has(p.y) && (!isInt(p.y) || p.y === 0 || p.y < INT32_MIN || p.y > INT32_MAX)) return 'year'
  if (has(p.m) && (!isInt(p.m) || p.m < 1 || p.m > 12)) return 'month'
  if (has(p.d) && (!isInt(p.d) || p.d < 1 || p.d > 31)) return 'day'
  if (p.e != null && p.e !== '' && (!validEra(p.e) || (!has(p.y) && (has(p.m) || has(p.d))))) return 'era'
  if (has(p.m) && has(p.d) && p.d > daysIn(p.y, p.m)) return 'no_such_day'
  return null
}

const pad = (v: number, w = 2) => String(v).padStart(w, '0')
const iso = (y: number, m: number, d: number) => `${y < 0 ? '-' : ''}${pad(Math.abs(y), 4)}-${pad(m)}-${pad(d)}`

/** Календарная граница есть: год известен, наша эра, в пределах date. */
const calendar = (p?: PartialDate | null): p is PartialDate & { y: number } =>
  !!p && has(p.y) && eraOf(p) === '' && p.y >= MIN_CAL_YEAR && p.y <= MAX_CAL_YEAR

/** Нижняя граница (±ГГГГ-ММ-ДД); только при известном годе нашей эры. */
export function lo(p?: PartialDate | null): string | null {
  if (!calendar(p)) return null
  return iso(p.y, p.m ?? 1, p.d ?? 1)
}

/** Верхняя граница; только при известном годе нашей эры. */
export function hi(p?: PartialDate | null): string | null {
  if (!calendar(p)) return null
  const m = p.m ?? 12
  return iso(p.y, m, p.d ?? daysIn(p.y, m))
}

/** Точная дата, если известны все части (наша эра). */
export function exact(p?: PartialDate | null): string | null {
  if (!calendar(p) || !has(p.m) || !has(p.d)) return null
  return iso(p.y, p.m, p.d)
}

function parts(isoDate: string): [number, number, number] {
  const neg = isoDate.startsWith('-')
  const [y, m, d] = (neg ? isoDate.slice(1) : isoDate).split('-').map(Number)
  return [(neg ? -1 : 1) * (y ?? 0), m ?? 0, d ?? 0]
}

const cmp3 = (a: number[], b: number[]) => {
  for (let i = 0; i < 3; i++) if (a[i] !== b[i]) return a[i]! < b[i]! ? -1 : 1
  return 0
}

/**
 * Полных лет на дату at той же эры, считая от верхней границы (не меньше
 * стольких); у at без месяца или дня — его нижняя граница. Без года, в другой
 * эре и до нижней границы — null. 29.02 в невисокосный год — 28.02. reversed —
 * счёт в эре обратный (F-143): больший год раньше.
 */
export function ageAt(p: PartialDate, at: PartialDate, reversed = false): number | null {
  if (!has(p.y) || !has(at.y) || eraOf(p) !== eraOf(at)) return null
  // год по ходу времени: в эре с обратным счётом — со знаком минус
  const fy = (y: number) => (reversed ? -astro(y) : astro(y))
  const am = at.m ?? 1
  const ad = at.d ?? 1
  if (cmp3([fy(at.y), am, ad], [fy(p.y), p.m ?? 1, p.d ?? 1]) < 0) return null
  const hm = p.m ?? 12
  const hd = Math.min(p.d ?? daysIn(p.y, hm), daysIn(at.y, hm))
  let age = fy(at.y) - fy(p.y)
  if (am < hm || (am === hm && ad < hd)) age--
  return Math.max(age, 0)
}

/**
 * Ближайшая годовщина не раньше from (ISO, включительно); нужны месяц и день,
 * наша эра. Год известен — не раньше самой даты. 29.02 в невисокосный год — 28.02.
 */
export function nextAnniversary(p: PartialDate, from: string): string | null {
  if (!has(p.m) || !has(p.d) || eraOf(p) !== '') return null
  const [fy] = parts(from)
  if (Math.max(fy, p.y ?? fy) < MIN_CAL_YEAR) return null // вне пределов date — как в SQL
  for (let y = Math.max(fy, p.y ?? fy); y <= MAX_CAL_YEAR; y = y === -1 ? 1 : y + 1) {
    const c = iso(y, p.m, Math.min(p.d, daysIn(y, p.m)))
    if (cmp3(parts(c), parts(from)) >= 0) return c
  }
  return null
}

/**
 * Место эры в ключе (F-143): эры не из списка order — первыми (новая — наверх),
 * между собой по тексту, пустая (наша) раньше всех; из списка — по месту в нём.
 */
export function eraKey(e: string, order: readonly string[] = []): string {
  const i = order.indexOf(e)
  return i < 0 ? '0' + e + '\u0001' : '1' + pad(i, 5)
}

/** Год в ключе: y + 2^31 (10 цифр); в эре с обратным счётом — 2^31 − y (больший год раньше). */
const yearKey = (y: number, reversed = false) => pad(reversed ? 2147483648 - y : y + 2147483648, 10)

const reversedIn = (e: string, o: EraOrder) => !!o.reversed?.includes(e)

/** Счёт в эре e обратный по порядку эр дашборда. */
export const isReversedEra = (e: string, order: EraOrderLike = []): boolean => reversedIn(e, eraOrderOf(order))

/**
 * Ключ сортировки (Д-9): датированные — '0' + место эры + год (yearKey) + ММДД
 * нижней и верхней границы; дата из одной эры — '0' + место эры (раньше всех
 * дат эры); без года — '1' + ММДД границ (после датированных).
 */
export function sortKey(p: PartialDate, order: EraOrderLike = []): string {
  const o = eraOrderOf(order)
  if (isEraOnly(p)) return '0' + eraKey(eraOf(p), o.order)
  const hm = p.m ?? 12
  const md = pad(p.m ?? 1) + pad(p.d ?? 1) + pad(hm) + pad(p.d ?? daysIn(p.y, hm))
  if (!has(p.y)) return '1' + md
  const e = eraOf(p)
  return '0' + eraKey(e, o.order) + yearKey(p.y, reversedIn(e, o)) + md
}

export function compareDates(a: PartialDate, b: PartialDate, order: EraOrderLike = []): number {
  return compareKeys(sortKey(a, order), sortKey(b, order))
}

/**
 * Ключи нижней и верхней границы для сравнения моментов разных эр (место эры,
 * год, месяц, день); дата из одной эры — от начала до конца эры; null — без года.
 */
export function loKey(p: PartialDate | null | undefined, order: EraOrderLike = []): string | null {
  const o = eraOrderOf(order)
  if (isEraOnly(p)) return eraKey(eraOf(p), o.order)
  if (!p || !has(p.y)) return null
  const e = eraOf(p)
  return eraKey(e, o.order) + yearKey(p.y, reversedIn(e, o)) + pad(p.m ?? 1) + pad(p.d ?? 1)
}

export function hiKey(p: PartialDate | null | undefined, order: EraOrderLike = []): string | null {
  const o = eraOrderOf(order)
  // '~' — после любых цифр года
  if (isEraOnly(p)) return eraKey(eraOf(p), o.order) + '~'
  if (!p || !has(p.y)) return null
  const m = p.m ?? 12
  const e = eraOf(p)
  return eraKey(e, o.order) + yearKey(p.y, reversedIn(e, o)) + pad(m) + pad(p.d ?? daysIn(p.y, m))
}

function fill(pattern: string, values: Record<string, string | number>): string {
  return pattern.replace(/\{(\w+)\}/g, (_, k: string) => String(values[k] ?? ''))
}

/**
 * Год для показа (F-14): эра скрыта — «1985», «500 до н. э.» (у любой эры);
 * показана — у нашей «1985 н. э.», «500 до н. э.», у заданной — «300 AC»,
 * «500 до AC» / «−500 AC»; у даты без эры (пустая при непустой эре по
 * умолчанию) — без надписи эры: «1985», «−500».
 */
export function formatYear(y: number, e: string, w: DateWords, view: DateView = {}): string {
  const abs = Math.abs(y)
  if (e === '' && view.showEra && view.noEra) return y < 0 ? fill(w.yearNoEraNegative, { y: abs }) : fill(w.yearPositive, { y: abs })
  if (e === '' || !view.showEra) return fill(y < 0 ? w.yearNegative : view.showEra ? w.yearOursPositive : w.yearPositive, { y: abs })
  return fill(y < 0 ? w.yearEraNegative : w.yearEraPositive, { y: abs, era: e })
}

/** Вид для показа по словам языкового файла: «12 марта 1985», «март 1985», «12-е число». */
export function formatDate(p: PartialDate | null | undefined, w: DateWords, view: DateView = {}): string {
  if (!p || isEmptyDate(p)) return ''
  // дата из одной эры — её название, даже если эры не показываются
  if (isEraOnly(p)) return eraOf(p)
  const { m, d } = p
  const y = has(p.y) ? formatYear(p.y, eraOf(p), w, view) : null
  let base: string
  if (has(m) && has(d)) base = fill(w.dayMonth, { d, month: w.monthsGenitive[m - 1] ?? '' })
  else if (has(m)) {
    const month = w.monthsNominative[m - 1] ?? ''
    return y != null ? fill(w.monthYear, { month, y }) : month
  } else if (has(d)) {
    const day = fill(w.dayOnly, { d })
    return y != null ? fill(w.dayWithYear, { date: day, y }) : day
  } else return y ?? ''
  return y != null ? fill(w.withYear, { date: base, y }) : base
}

/** Сегодня по местному времени, ISO. */
export function todayISO(now = new Date()): string {
  return iso(now.getFullYear(), now.getMonth() + 1, now.getDate())
}

/** Сегодня по местному времени — частичная дата нашей эры. */
export function todayDate(now = new Date()): PartialDate {
  return { y: now.getFullYear(), m: now.getMonth() + 1, d: now.getDate() }
}

/** Частичная дата из ISO ('ГГГГ-ММ-ДД', год до н. э. — со знаком минус). */
export function fromISO(isoDate: string): PartialDate {
  const [y, m, d] = parts(isoDate)
  return { y, m, d }
}
