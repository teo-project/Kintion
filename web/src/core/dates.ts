// Частичная дата (F-14, docs/concepts/dates.md): проверка, границы, точная дата,
// возраст, годовщина, ключ сортировки (Д-9), вид для показа.
// Те же правила — в PostgreSQL (домен partial_date, функции pd_*) и Go
// (server/internal/partialdate); общие кейсы — testdata/partial_dates.json.
// Операторов «раньше / возможно раньше» нет (Q-07, В-13).

/** Частичная дата: каждая часть необязательна, хотя бы одна есть. ISO-даты — 'ГГГГ-ММ-ДД'. */
export interface PartialDate {
  y?: number | null
  m?: number | null
  d?: number | null
}

export type DateError = 'empty' | 'year' | 'month' | 'day' | 'no_such_day'

/** Слова вида даты — из языкового файла (F-76, раздел dates). */
export interface DateWords {
  monthsGenitive: string[]
  monthsNominative: string[]
  dayMonth: string
  monthYear: string
  dayOnly: string
  withYear: string
  dayWithYear: string
}

const has = (v: number | null | undefined): v is number => v != null

export function isEmptyDate(p?: PartialDate | null): boolean {
  return !p || (!has(p.y) && !has(p.m) && !has(p.d))
}

function isLeap(y: number): boolean {
  return y % 4 === 0 && (y % 100 !== 0 || y % 400 === 0)
}

/** Число дней в месяце; без года у февраля 29. */
export function daysIn(y: number | null | undefined, m: number): number {
  if (m === 2) return !has(y) || isLeap(y) ? 29 : 28
  return [4, 6, 9, 11].includes(m) ? 30 : 31
}

const isInt = (v: number) => Number.isInteger(v)

export function validateDate(p: PartialDate): DateError | null {
  if (isEmptyDate(p)) return 'empty'
  if (has(p.y) && (!isInt(p.y) || p.y < 1 || p.y > 9999)) return 'year'
  if (has(p.m) && (!isInt(p.m) || p.m < 1 || p.m > 12)) return 'month'
  if (has(p.d) && (!isInt(p.d) || p.d < 1 || p.d > 31)) return 'day'
  if (has(p.m) && has(p.d) && p.d > daysIn(p.y, p.m)) return 'no_such_day'
  return null
}

const pad = (v: number, w = 2) => String(v).padStart(w, '0')
const iso = (y: number, m: number, d: number) => `${pad(y, 4)}-${pad(m)}-${pad(d)}`

/** Нижняя граница; только при известном годе. */
export function lo(p?: PartialDate | null): string | null {
  if (!p || !has(p.y)) return null
  return iso(p.y, p.m ?? 1, p.d ?? 1)
}

/** Верхняя граница; только при известном годе. */
export function hi(p?: PartialDate | null): string | null {
  if (!p || !has(p.y)) return null
  const m = p.m ?? 12
  return iso(p.y, m, p.d ?? daysIn(p.y, m))
}

/** Точная дата, если известны все части. */
export function exact(p?: PartialDate | null): string | null {
  if (!p || !has(p.y) || !has(p.m) || !has(p.d)) return null
  return iso(p.y, p.m, p.d)
}

function parts(isoDate: string): [number, number, number] {
  const [y, m, d] = isoDate.split('-').map(Number)
  return [y ?? 0, m ?? 0, d ?? 0]
}

/**
 * Полных лет на дату at (ISO), считая от верхней границы (не меньше стольких).
 * Без года и до нижней границы — null. 29.02 в невисокосный год — 28.02.
 */
export function ageAt(p: PartialDate, at: string): number | null {
  const l = lo(p)
  const h = hi(p)
  if (l == null || h == null || at < l) return null
  const [ay, am, ad] = parts(at)
  const [hy, hm, hdRaw] = parts(h)
  const hd = Math.min(hdRaw, daysIn(ay, hm))
  let age = ay - hy
  if (am < hm || (am === hm && ad < hd)) age--
  return Math.max(age, 0)
}

/**
 * Ближайшая годовщина не раньше from (ISO, включительно); нужны месяц и день.
 * Год известен — не раньше самой даты. 29.02 в невисокосный год — 28.02.
 */
export function nextAnniversary(p: PartialDate, from: string): string | null {
  if (!has(p.m) || !has(p.d)) return null
  for (let y = Math.max(parts(from)[0], p.y ?? 0); ; y++) {
    const c = iso(y, p.m, Math.min(p.d, daysIn(y, p.m)))
    if (c >= from) return c
  }
}

/** Ключ сортировки (Д-9): датированные — '0' + lo + hi, без года — '1' + ММДД границ (после). */
export function sortKey(p: PartialDate): string {
  const l = lo(p)
  const h = hi(p)
  if (l != null && h != null) return '0' + l.replaceAll('-', '') + h.replaceAll('-', '')
  const hm = p.m ?? 12
  return '1' + pad(p.m ?? 1) + pad(p.d ?? 1) + pad(hm) + pad(p.d ?? daysIn(null, hm))
}

export function compareDates(a: PartialDate, b: PartialDate): number {
  const ka = sortKey(a)
  const kb = sortKey(b)
  return ka < kb ? -1 : ka > kb ? 1 : 0
}

function fill(pattern: string, values: Record<string, string | number>): string {
  return pattern.replace(/\{(\w+)\}/g, (_, k: string) => String(values[k] ?? ''))
}

/** Вид для показа по словам языкового файла: «12 марта 1985», «март 1985», «12-е число». */
export function formatDate(p: PartialDate | null | undefined, w: DateWords): string {
  if (!p || isEmptyDate(p)) return ''
  const { y, m, d } = p
  let base: string
  if (has(m) && has(d)) base = fill(w.dayMonth, { d, month: w.monthsGenitive[m - 1] ?? '' })
  else if (has(m)) {
    const month = w.monthsNominative[m - 1] ?? ''
    return has(y) ? fill(w.monthYear, { month, y }) : month
  } else if (has(d)) {
    const day = fill(w.dayOnly, { d })
    return has(y) ? fill(w.dayWithYear, { date: day, y }) : day
  } else return String(y)
  return has(y) ? fill(w.withYear, { date: base, y }) : base
}

/** Сегодня по местному времени, ISO. */
export function todayISO(now = new Date()): string {
  return iso(now.getFullYear(), now.getMonth() + 1, now.getDate())
}
