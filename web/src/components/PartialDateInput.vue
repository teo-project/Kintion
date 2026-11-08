<script setup lang="ts">
// Ввод частичной даты (F-14, F-142; дизайн «Historical Date Picker»): поле
// ДД.ММ.ГГГГ — каждая часть необязательна; две цифры, «.», «/», пробел —
// переход к следующей части, Backspace в пустой — назад, ↑/↓ — изменить;
// «−» в годе меняет знак (год до начала эры), если отрицательные разрешены;
// 0 или 00 — «не указано». Кнопка календаря — выбор без прокрутки от
// сегодняшнего года: век → десятилетие → год, месяц, день (дни недели — по
// григорианскому календарю). Недавние — 3 последних введённых года (в
// браузере, на 4 часа). Пределы года — из эры даты в настройках дашборда
// (F-143: минимальный и максимальный год эры; минимальный ≥ 1 — без минуса)
// или свойствами allowNegative, yearMin, yearMax.
//
// Эра (F-143): поле с подсказками — только если настройки дашборда разрешают
// менять эру, иначе эра даты сохраняется. Пустое поле эры у новой даты — эра
// по умолчанию: её ставит сервер, в поле она — подсказкой; у записанной даты
// эра показывается как есть. Дата может состоять из одной эры. Проверка —
// ядром (validateDate): 29.02 без года можно, с невисокосным годом — нет.
import { PopoverAnchor, PopoverContent, PopoverPortal, PopoverRoot } from 'reka-ui'
import { computed, ref, useId, watch } from 'vue'

import { astro, daysIn, isEmptyDate, isEraOnly, validateDate, type PartialDate } from '@/core/dates'
import { fmt, t } from '@/i18n'
import { useDashboardEras } from '@/ui/eras'
import { recentYears, rememberYear } from '@/ui/recentYears'

import KinIcon from './KinIcon.vue'

/** Эры для ввода: менять ли, эра по умолчанию, подсказки (F-143). */
export interface EraInput {
  edit: boolean
  default: string
  list: readonly string[]
  /** минимальный и максимальный год эры (F-143) */
  years?: Readonly<Record<string, { min?: number | null; max?: number | null }>>
}

const props = withDefaults(defineProps<{
  modelValue: PartialDate | null
  label?: string
  eras?: EraInput
  /** можно ли год до начала эры (со знаком минус) */
  allowNegative?: boolean
  yearMin?: number
  yearMax?: number
}>(), { label: undefined, eras: undefined, allowNegative: true, yearMin: undefined, yearMax: undefined })
/** popup — выбор открыт/закрыт (он в портале): фокус в нём — не уход из поля */
const emit = defineEmits<{ 'update:modelValue': [PartialDate | null]; popup: [boolean] }>()

const INT_MAX = 2147483647

const day = ref('')
const month = ref('')
const year = ref('')
const era = ref('')
/** дата новая (пустая снаружи): пустую эру сервер заменит эрой по умолчанию */
const isNew = ref(true)
/** эру стёр сброс года при скрытом поле эры */
let eraCleared = false

// эры — из свойства, иначе — настройки текущего дашборда
const fromDashboard = useDashboardEras()
const eraIn = computed(() => props.eras ?? fromDashboard.value)
const editEra = computed(() => !!eraIn.value?.edit)
const defaultEra = computed(() => eraIn.value?.default ?? '')
const suggestions = computed(() => (eraIn.value?.list ?? []).filter(x => x !== ''))

const pad = (n: number | null | undefined) => (n != null ? String(n).padStart(2, '0') : '')

function sync(v: PartialDate | null) {
  day.value = pad(v?.d)
  month.value = pad(v?.m)
  year.value = v?.y != null ? String(v.y) : ''
  // эра — как есть: сервер ставит эру по умолчанию только новой дате
  era.value = v?.e ?? ''
  isNew.value = v == null
  eraCleared = false
}
sync(props.modelValue)

// пределы года
/** пределы года: заданные свойствами, иначе — эры даты из настроек дашборда
 * (F-143; пустая эра — эра по умолчанию, как и на сервере); min > max — обратный счёт */
const bounds = computed(() => {
  const e = era.value.trim() || defaultEra.value
  const y = eraIn.value?.years?.[e]
  let lo = props.yearMin ?? y?.min ?? null
  let hi = props.yearMax ?? y?.max ?? null
  if (lo != null && hi != null && lo > hi) [lo, hi] = [hi, lo]
  return { lo: lo || null, hi: hi || null }
})
/** минус можно: разрешён полем и нижний предел не положительный */
const canNeg = computed(() => props.allowNegative && (bounds.value.lo == null || bounds.value.lo < 0))
const yMax = computed(() => (bounds.value.hi != null ? Math.max(Math.min(bounds.value.hi, INT_MAX), -INT_MAX - 1) : INT_MAX))
const yMin = computed(() => {
  const floor = canNeg.value ? -INT_MAX - 1 : 1
  return bounds.value.lo != null ? Math.min(Math.max(bounds.value.lo, floor), yMax.value) : floor
})
const inRange = (n: number) => n >= yMin.value && n <= yMax.value
/** можно набирать: не дальше предела своего знака (ниже положительного
 * yearMin — ещё набирается, ошибка — под полем) */
const typable = (n: number) => (n > 0 ? yMax.value > 0 && n <= yMax.value : yMin.value < 0 && n >= yMin.value)

function num(s: string): number | null {
  const n = parseInt(s, 10)
  return Number.isNaN(n) || n === 0 ? null : n
}
const yNum = computed(() => num(year.value) ?? 0)

/** Собранная дата; пустые части — null; ни одной части — null. */
const parsed = computed<PartialDate | null>(() => {
  const p: PartialDate = { y: num(year.value), m: num(month.value), d: num(day.value) }
  const e = era.value.trim()
  if (e !== '') p.e = e
  return isEmptyDate(p) ? null : p
})

const error = computed(() => {
  const y = yNum.value
  if (y && !inRange(y)) return fmt(t.dates.picker.outOfRange, { min: yMin.value, max: yMax.value })
  return parsed.value && validateDate(parsed.value) ? t.card.dateError : ''
})

/** поле эры: если её можно менять или дата — из одной эры */
const showEra = computed(() => editEra.value || isEraOnly(parsed.value))

function same(a: PartialDate | null | undefined, b: PartialDate | null): boolean {
  if (!a || !b) return !a && !b
  return (a.y ?? null) === (b.y ?? null) && (a.m ?? null) === (b.m ?? null) &&
    (a.d ?? null) === (b.d ?? null) && (a.e ?? '') === (b.e ?? '')
}

watch(() => props.modelValue, v => {
  if (!same(v, parsed.value)) sync(v)
})
watch(parsed, v => emit('update:modelValue', v))
// год стёрт при скрытом поле эры: эра уходит; новый год получит эру по умолчанию
watch(yNum, v => {
  if (editEra.value) return
  if (!v) {
    if (era.value !== '') {
      era.value = ''
      eraCleared = true
    }
  } else if (eraCleared) {
    era.value = defaultEra.value
    eraCleared = false
  }
})

// ——— поле ДД.ММ.ГГГГ ———
const dayEl = ref<HTMLInputElement | null>(null)
const monthEl = ref<HTMLInputElement | null>(null)
const yearEl = ref<HTMLInputElement | null>(null)
const focused = ref(false)

/** фокус в часть поля: всё выделено или (select = false) курсор в конце */
function focus(el: HTMLInputElement | null, select = true) {
  setTimeout(() => {
    el?.focus()
    if (select) el?.select()
    else el?.setSelectionRange(el.value.length, el.value.length)
  }, 0)
}

/** год вводили (а не только день или месяц) — уход из поля запомнит его */
let yearTyped = false

const digits = (v: string, n: number) => v.replace(/\D/g, '').slice(0, n)

/** день и месяц: автодополнение нулём (4 → 04), 2 цифры — следующая часть */
function onPart(e: Event, part: 'd' | 'm') {
  const el = e.target as HTMLInputElement
  const raw = el.value.replace(/\D/g, '')
  let v = digits(el.value, 2)
  if (v.length === 1 && +v > (part === 'd' ? 3 : 1)) v = '0' + v
  if (v === '00') v = ''
  el.value = v
  if (part === 'd') day.value = v
  else month.value = v
  if (v.length === 2 || raw === '00') focus(part === 'd' ? monthEl.value : yearEl.value)
}

function onYear(e: Event) {
  const el = e.target as HTMLInputElement
  const raw = el.value.trim()
  const neg = /^[-−–]/.test(raw)
  const v = (neg ? '-' : '') + digits(raw, 10)
  const n = num(v)
  // минус запрещён или год вне пределов — ввод не принимается (смысл не меняем)
  if ((neg && !canNeg.value) || (n && !typable(n))) {
    el.value = year.value
    return
  }
  el.value = v
  year.value = v
  yearTyped = true
  if (n) cursorTo(n)
}

type Part = 'day' | 'month' | 'year'
const vals = { day, month, year }
const els = { day: dayEl, month: monthEl, year: yearEl }
const order: Part[] = ['day', 'month', 'year']

function onKey(e: KeyboardEvent, part: Part) {
  const el = e.target as HTMLInputElement
  const val = vals[part].value
  const i = order.indexOf(part)
  const prev = i > 0 ? els[order[i - 1]!].value : null
  const next = i < 2 ? els[order[i + 1]!].value : null
  if (part === 'year' && (e.key === '-' || e.key === '−')) {
    e.preventDefault()
    if (!canNeg.value) return
    const v = val.startsWith('-') ? val.slice(1) : '-' + val
    const n = num(v)
    if (n && !typable(n)) return
    year.value = v
    yearTyped = true
    if (n) cursorTo(n)
  } else if (e.key === 'Backspace' && val === '' && prev) {
    e.preventDefault()
    focus(prev)
  } else if (['.', '/', ' ', ','].includes(e.key) || (e.key === '-' && part !== 'year')) {
    e.preventDefault()
    if (!next) return
    if (val.length === 1) vals[part].value = '0' + val
    if (e.key === '-' && canNeg.value && part === 'month' && !year.value.startsWith('-')) {
      const n = num('-' + year.value)
      if (!(n && !typable(n))) year.value = '-' + year.value
      focus(next, false) // минус не выделен: следующая цифра его не заменит
      return
    }
    focus(next)
  } else if (e.key === 'ArrowRight' && el.selectionStart === val.length && next) {
    e.preventDefault()
    focus(next)
  } else if (e.key === 'ArrowLeft' && el.selectionStart === 0 && prev) {
    e.preventDefault()
    focus(prev)
  } else if (e.key === 'ArrowUp' || e.key === 'ArrowDown') {
    e.preventDefault()
    const dir = e.key === 'ArrowUp' ? 1 : -1
    if (part === 'year') {
      let n = val === '' || val === '-' ? (recent.value[0] ?? 1900) : (num(val) ?? 0) + dir
      if (n === 0) n = dir
      n = Math.max(yMin.value, Math.min(yMax.value, n)) || 1
      year.value = String(n)
      yearTyped = true
      cursorTo(n)
      return
    }
    const max = part === 'day' ? 31 : 12
    let n = val === '' ? 1 : +val + dir
    if (n > max) n = 1
    if (n < 1) n = max
    vals[part].value = pad(n)
  }
}

/** уход из поля: «0», «00», «-» — не указано; введённый год — в недавние */
function onBlur() {
  focused.value = false
  for (const p of order) if (/^-?0+$/.test(vals[p].value) || vals[p].value === '-') vals[p].value = ''
  if (yearTyped && yNum.value && inRange(yNum.value)) rememberYear(yNum.value)
  yearTyped = false
}

const hasAny = computed(() => !!(day.value || month.value || year.value || era.value))
function clearAll() {
  day.value = ''
  month.value = ''
  year.value = ''
  era.value = ''
  focus(dayEl.value)
}

// ——— выбор в календаре ———
type View = 'year' | 'month' | 'day'
const open = ref(false)
const view = ref<View>('year')
/** показ «Недавних» вместо сетки лет */
const showRecent = ref(false)
/** курсор сетки лет: знак, уровень (0 — годы, 1 — десятилетия, 2 — века…), начало */
const neg = ref(false)
const lvl = ref(1)
const base = ref(1900)

const recent = ref<number[]>(recentYears())
const recentList = computed(() => {
  const out: number[] = []
  for (const n of recent.value) if (n && inRange(n) && !out.includes(n)) out.push(n)
  return out.slice(0, 3)
})
/** ряды по 5 лет вокруг недавних (без повторов, без нулевого года) */
const recentRows = computed(() => {
  const rows: number[][] = []
  for (const y of [...recentList.value].sort((a, b) => a - b)) {
    if (rows.some(r => y >= r[0]! && y <= r[r.length - 1]!)) continue
    let st = y - 2
    const last = rows.at(-1)
    if (last && st <= last[last.length - 1]!) st = last[last.length - 1]! + 1
    if (st < yMin.value) st = yMin.value
    const row: number[] = []
    for (let v = st; row.length < 5 && v <= yMax.value; v++) if (v !== 0) row.push(v)
    rows.push(row)
  }
  return rows
})

function cursorTo(y: number) {
  const a = Math.abs(y)
  neg.value = y < 0
  lvl.value = 0
  base.value = Math.floor(a / 10) * 10
  showRecent.value = false
}
function around(y: number, l: number) {
  const sz = 10 ** (l + 1)
  neg.value = y < 0
  lvl.value = l
  base.value = Math.floor(Math.abs(y) / sz) * sz
  showRecent.value = false
}

function toggleOpen() {
  if (open.value) {
    open.value = false
    return
  }
  recent.value = recentYears()
  const y = yNum.value
  if (y) cursorTo(y)
  else if (recentList.value.length) {
    around(recentList.value[0]!, 1)
    showRecent.value = true
  } else around(Math.min(Math.max(1900, yMin.value), yMax.value) || 1, 1)
  view.value = !y ? 'year' : !month.value ? 'month' : 'day'
  open.value = true
}

/** шаг сетки лет вперёд (+1) или назад во времени; через ноль — смена знака */
function step(dir: number) {
  const sz = 10 ** (lvl.value + 1)
  const mag = base.value + (neg.value ? -dir : dir) * sz
  if (mag < 0) {
    if (canNeg.value) {
      neg.value = !neg.value
      base.value = 0
    }
    return
  }
  if (mag > (neg.value ? -yMin.value : yMax.value)) return
  base.value = mag
}

const fmtYear = (n: number) => {
  const a = Math.abs(n)
  return (n < 0 ? '−' : '') + (a >= 10000 ? a.toLocaleString('ru-RU') : String(a))
}
const yLabel = (y: number) => fmt(t.dates.picker.year, { y: fmtYear(y) })
const monthName = (m: number) => t.dates.monthsNominative[m - 1] ?? ''

function pickYear(v: number) {
  year.value = String(v)
  cursorTo(v)
  rememberYear(v)
  view.value = month.value ? 'year' : 'month'
}

interface Cell { key: string; label: string; sub?: string; on: boolean; ring?: boolean; muted?: boolean; small?: boolean; pick?: () => void }

const grid = computed(() => {
  const y = yNum.value
  const m = num(month.value) ?? 0
  const d = num(day.value) ?? 0
  const sg = neg.value ? '−' : ''
  const cells: Cell[] = []
  let cols = 4
  let title = ''
  let sub = ''
  let canUp = false
  let arrows = false
  let up = () => {}
  let note = ''
  let weekdays = false
  let size: 'm' | 'l' | 's' = 'm'
  if (view.value === 'year' && showRecent.value && recentRows.value.length) {
    cols = 5
    title = t.dates.picker.recent
    canUp = true
    up = () => around(recentList.value[0]!, 1)
    for (const row of recentRows.value) {
      for (const v of row) {
        cells.push({ key: `r${v}`, label: fmtYear(v), on: y === v, ring: recentList.value.includes(v), small: Math.abs(v) >= 1e5, pick: () => pickYear(v) })
      }
    }
  } else if (view.value === 'year') {
    arrows = true
    const k = lvl.value
    const sz = 10 ** k
    const B = base.value
    const lim = neg.value ? -yMin.value : yMax.value
    const lo = Math.max(B, 1)
    const hi = B + sz * 10 - 1
    title = neg.value ? `${sg}${fmtYear(hi)} … ${sg}${fmtYear(lo)}` : `${fmtYear(lo)} … ${fmtYear(hi)}`
    canUp = 10 ** (k + 1) * 10 <= lim * 10 && k < 9
    up = () => {
      const big = 10 ** (k + 2)
      lvl.value = k + 1
      base.value = Math.floor(B / big) * big
    }
    const sameSign = y && (y < 0) === neg.value
    for (let i = -1; i <= 10; i++) {
      const st = B + i * sz
      const muted = i < 0 || i > 9
      if (k === 0) {
        const v = neg.value ? -st : st
        if (st < 1 || st > lim || !inRange(v)) {
          cells.push({ key: `b${i}`, label: '', on: false })
          continue
        }
        cells.push({ key: `y${v}`, label: fmtYear(v), on: y === v, muted, small: st >= 1e5, pick: () => pickYear(v) })
      } else {
        // группа целиком вне пределов (в т. ч. ниже положительного yearMin)
        const ends = neg.value ? [-(st + sz - 1), -Math.max(st, 1)] : [Math.max(st, 1), st + sz - 1]
        if (st < 0 || st > lim || ends[1]! < yMin.value || ends[0]! > yMax.value) {
          cells.push({ key: `b${i}`, label: '', on: false })
          continue
        }
        const on = !!sameSign && Math.floor(Math.abs(y) / sz) * sz === st
        cells.push({
          key: `g${k}-${st}`, label: k === 1 ? fmt(t.dates.picker.decade, { y: sg + fmtYear(st) }) : sg + fmtYear(st),
          sub: k === 1 ? undefined : `… ${sg}${fmtYear(st + sz - 1)}`, on, muted, small: st >= 1e5,
          pick: () => {
            lvl.value = k - 1
            base.value = st
          },
        })
        if (k > 1) size = 'l'
      }
    }
    if (neg.value) cells.reverse()
  } else if (view.value === 'month') {
    title = y ? yLabel(y) : t.dates.picker.noYear
    t.dates.picker.monthsShort.forEach((n, i) => cells.push({
      key: `m${i}`, label: n, on: m === i + 1,
      pick: () => {
        month.value = pad(i + 1)
        view.value = day.value ? 'month' : 'day'
      },
    }))
  } else {
    cols = 7
    size = 's'
    const okMonth = m >= 1 && m <= 12
    if (okMonth && y && Math.abs(y) < 275000) {
      weekdays = true
      const dt = new Date(2000, m - 1, 1)
      dt.setFullYear(astro(y), m - 1, 1)
      for (let i = 0; i < (dt.getDay() + 6) % 7; i++) cells.push({ key: `l${i}`, label: '', on: false })
      title = `${monthName(m)} ${fmtYear(y)}`
      note = y < 1918 ? t.dates.picker.gregorian : ''
    } else {
      title = okMonth ? monthName(m) : t.dates.picker.noMonth
      sub = okMonth && !y ? t.dates.picker.yearUnknown : ''
    }
    const n = okMonth ? daysIn(y || null, m) : 31
    for (let i = 1; i <= n; i++) {
      cells.push({
        key: `d${i}`, label: String(i), on: d === i,
        pick: () => {
          day.value = pad(i)
          close()
        },
      })
    }
  }
  return { cells, cols, title, sub, canUp, arrows, up, note, weekdays, size }
})

const tabs = computed(() => {
  const y = yNum.value
  const m = num(month.value) ?? 0
  const d = num(day.value) ?? 0
  return [
    { k: 'year' as View, label: t.dates.partYear, value: y ? fmtYear(y) : '—', has: !!y },
    { k: 'month' as View, label: t.dates.partMonth, value: m >= 1 && m <= 12 ? monthName(m) : '—', has: !!m },
    { k: 'day' as View, label: t.dates.partDay, value: d ? String(d) : '—', has: !!d },
  ]
})
const inRecent = computed(() => view.value === 'year' && showRecent.value && recentRows.value.length > 0)
const clearLabel = computed(() => ({ year: t.dates.picker.clearYear, month: t.dates.picker.clearMonth, day: t.dates.picker.clearDay })[view.value])
function clearCurrent() {
  vals[view.value].value = ''
}
/** закрыть выбор и вернуть фокус в поле: правка по месту не считает это уходом */
function close() {
  const el = yearEl.value
  el?.focus()
  el?.setSelectionRange(el.value.length, el.value.length)
  open.value = false
}
watch(open, v => emit('popup', v))

/** Esc в поле при открытом выборе (фокус ещё на кнопке календаря) — закрыть выбор, а не правку */
function onAnchorEsc(e: KeyboardEvent) {
  if (!open.value) return
  e.stopPropagation()
  e.preventDefault()
  close()
}

function setSign(v: boolean) {
  neg.value = v
  base.value = 0
}

const listId = `eras-${useId()}`
</script>

<template>
  <fieldset
    class="pdate"
    :aria-invalid="error ? 'true' : undefined"
  >
    <legend
      v-if="label"
      class="kin-label"
    >
      {{ label }}
    </legend>
    <div class="parts">
      <PopoverRoot v-model:open="open">
        <PopoverAnchor
          class="field"
          :class="{ focused: focused || open, invalid: !!error }"
          @keydown.esc="onAnchorEsc"
        >
          <input
            ref="dayEl"
            class="seg day"
            :value="day"
            inputmode="numeric"
            :placeholder="t.dates.picker.dd"
            :aria-label="t.dates.partDay"
            :aria-invalid="error ? 'true' : undefined"
            @input="e => onPart(e, 'd')"
            @keydown="e => onKey(e, 'day')"
            @focus="focused = true"
            @blur="onBlur"
          >
          <span class="dot">.</span>
          <input
            ref="monthEl"
            class="seg month"
            :value="month"
            inputmode="numeric"
            :placeholder="t.dates.picker.mm"
            :aria-label="t.dates.partMonth"
            :aria-invalid="error ? 'true' : undefined"
            @input="e => onPart(e, 'm')"
            @keydown="e => onKey(e, 'month')"
            @focus="focused = true"
            @blur="onBlur"
          >
          <span class="dot">.</span>
          <input
            ref="yearEl"
            class="seg year"
            :value="year"
            inputmode="numeric"
            :placeholder="t.dates.picker.yyyy"
            :aria-label="t.dates.partYear"
            :aria-invalid="error ? 'true' : undefined"
            :style="{ width: Math.max(4, year.length) + 1.6 + 'ch' }"
            @input="onYear"
            @keydown="e => onKey(e, 'year')"
            @focus="focused = true"
            @blur="onBlur"
          >
          <span class="grow" />
          <button
            v-if="hasAny"
            class="kin-icon-btn clear"
            type="button"
            :aria-label="t.dates.picker.clear"
            :title="t.dates.picker.clear"
            @keydown.enter.stop
            @click="clearAll"
          >
            <KinIcon name="close" />
          </button>
          <button
            class="kin-icon-btn cal"
            type="button"
            :aria-label="t.dates.picker.open"
            :title="t.dates.picker.open"
            :aria-expanded="open"
            @keydown.enter.stop
            @click="toggleOpen"
          >
            <KinIcon name="calendar" />
          </button>
        </PopoverAnchor>
        <!-- в портале — иначе диалог с прокруткой его обрежет; правка по месту
             (EditCell) знает о нём по popup -->
        <PopoverPortal>
          <PopoverContent
            class="hdp"
            side="bottom"
            align="start"
            :side-offset="6"
            :collision-padding="8"
            @open-auto-focus.prevent
            @keydown.enter.stop
            @keydown.esc.stop.prevent="close"
          >
            <div class="tabs">
              <button
                v-for="tb in tabs"
                :key="tb.k"
                type="button"
                class="tab"
                :class="{ on: view === tb.k, has: tb.has }"
                :aria-pressed="view === tb.k"
                @click="view = tb.k"
              >
                <span class="tab-label">{{ tb.label }}</span>
                <span class="tab-value">{{ tb.value }}</span>
              </button>
            </div>
            <div
              v-if="view === 'year' && !inRecent && canNeg"
              class="sign"
            >
              <button
                type="button"
                :class="{ on: neg }"
                :aria-pressed="neg"
                :title="t.dates.partBeforeHint"
                @click="setSign(true)"
              >
                −
              </button>
              <button
                type="button"
                :class="{ on: !neg }"
                :aria-pressed="!neg"
                @click="setSign(false)"
              >
                +
              </button>
            </div>
            <div class="head">
              <button
                v-if="grid.arrows"
                class="kin-icon-btn"
                type="button"
                :aria-label="t.dates.picker.prev"
                :title="t.dates.picker.prev"
                @click="step(-1)"
              >
                <KinIcon name="prev" />
              </button>
              <button
                type="button"
                class="head-title"
                :class="{ up: grid.canUp }"
                :disabled="!grid.canUp"
                :title="grid.canUp ? t.dates.picker.up : undefined"
                @click="grid.up()"
              >
                <span>{{ grid.title }}</span>
                <span
                  v-if="grid.sub"
                  class="head-sub"
                >{{ grid.sub }}</span>
                <KinIcon
                  v-if="grid.canUp"
                  name="chevronUp"
                />
              </button>
              <button
                v-if="grid.arrows"
                class="kin-icon-btn"
                type="button"
                :aria-label="t.dates.picker.next"
                :title="t.dates.picker.next"
                @click="step(1)"
              >
                <KinIcon name="next" />
              </button>
            </div>
            <div
              v-if="grid.weekdays"
              class="weekdays"
            >
              <span
                v-for="w in t.dates.picker.weekdays"
                :key="w"
              >{{ w }}</span>
            </div>
            <div
              class="grid"
              :class="`size-${grid.size}`"
              :style="{ gridTemplateColumns: `repeat(${grid.cols}, minmax(0, 1fr))` }"
            >
              <template
                v-for="c in grid.cells"
                :key="c.key"
              >
                <button
                  v-if="c.pick"
                  type="button"
                  class="cell"
                  :class="{ on: c.on, ring: c.ring, muted: c.muted, small: c.small }"
                  :aria-pressed="c.on"
                  @click="c.pick"
                >
                  <span>{{ c.label }}</span>
                  <span
                    v-if="c.sub"
                    class="cell-sub"
                  >{{ c.sub }}</span>
                </button>
                <span v-else />
              </template>
            </div>
            <p
              v-if="grid.note"
              class="note"
            >
              {{ grid.note }}
            </p>
            <div class="foot">
              <button
                type="button"
                class="kin-btn kin-btn--small ghost"
                @click="clearCurrent"
              >
                {{ clearLabel }}
              </button>
              <button
                v-if="view === 'year' && !inRecent && recentRows.length"
                type="button"
                class="kin-btn kin-btn--small ghost recent"
                @click="showRecent = true"
              >
                {{ t.dates.picker.recent }}
              </button>
              <span class="grow" />
              <button
                type="button"
                class="kin-btn kin-btn--small kin-btn--primary"
                @click="close"
              >
                {{ t.dates.picker.done }}
              </button>
            </div>
          </PopoverContent>
        </PopoverPortal>
      </PopoverRoot>
      <template v-if="showEra">
        <input
          v-model="era"
          class="kin-input part era"
          maxlength="64"
          :list="listId"
          :placeholder="(isNew && defaultEra) || t.dates.partEra"
          :title="t.dates.partEra"
          :aria-label="t.dates.partEra"
          :aria-invalid="error ? 'true' : undefined"
        >
        <datalist :id="listId">
          <option
            v-for="x in suggestions"
            :key="x"
            :value="x"
          />
        </datalist>
      </template>
    </div>
    <p
      v-if="error"
      class="kin-error"
      role="alert"
    >
      {{ error }}
    </p>
  </fieldset>
</template>

<style scoped>
.pdate {
  margin: 0;
  padding: 0;
  border: none;
}

.parts {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kin-space-1);
}

.field {
  display: flex;
  align-items: center;
  gap: 2px;
  box-sizing: border-box;
  max-width: 100%;
  height: var(--kin-control-height);
  padding: 0 4px 0 12px;
  background: var(--kin-color-bg);
  border: 1px solid var(--kin-color-border);
  border-radius: var(--kin-radius);
}

.field.focused {
  border-color: var(--kin-color-accent);
}

.field.invalid {
  border-color: var(--kin-color-danger);
}

.seg {
  padding: 0;
  font: inherit;
  font-size: 15px;
  font-variant-numeric: tabular-nums;
  color: var(--kin-color-text);
  text-align: center;
  caret-color: var(--kin-color-accent);
  background: transparent;
  border: none;
  outline: none;
}

.seg::placeholder {
  color: var(--kin-color-text-muted);
  opacity: 0.7;
}

.day {
  width: 2.4ch;
}

.month {
  width: 2.6ch;
}

.dot {
  color: var(--kin-color-text-muted);
}

.grow {
  flex: 1;
  min-width: var(--kin-space-2);
}

.field .kin-icon-btn {
  width: 30px;
  height: 30px;
}

.clear {
  color: var(--kin-color-text-muted);
}

.era {
  width: 6rem;
}
</style>

<style>
/* всплывающий выбор — в портале, вне scoped */
.hdp {
  z-index: 60;
  display: flex;
  flex-direction: column;
  gap: 12px;
  box-sizing: border-box;
  width: min(320px, calc(100vw - 16px));
  padding: 12px;
  color: var(--kin-color-text);
  background: var(--kin-color-surface-raised);
  border-radius: var(--kin-radius-island);
  box-shadow: var(--kin-shadow);
}

.hdp button {
  font: inherit;
}

.hdp .tabs {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 4px;
}

.hdp .tab {
  display: flex;
  flex-direction: column;
  gap: 2px;
  align-items: flex-start;
  min-width: 0;
  padding: 6px 10px;
  text-align: left;
  cursor: pointer;
  background: transparent;
  border: 1px solid var(--kin-color-border);
  border-radius: var(--kin-radius);
}

.hdp .tab.on {
  background: color-mix(in srgb, var(--kin-color-accent) 10%, transparent);
  border-color: var(--kin-color-accent);
}

.hdp .tab-label {
  font-size: 11px;
  color: var(--kin-color-text-muted);
}

.hdp .tab-value {
  max-width: 100%;
  overflow: hidden;
  font-size: 14px;
  color: var(--kin-color-text-muted);
  white-space: nowrap;
  text-overflow: ellipsis;
}

.hdp .tab.has .tab-value {
  color: var(--kin-color-text);
}

.hdp .sign {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  overflow: hidden;
  border: 1px solid var(--kin-color-border);
  border-radius: var(--kin-radius);
}

.hdp .sign button {
  height: 28px;
  font-size: 15px;
  color: var(--kin-color-text);
  cursor: pointer;
  background: transparent;
  border: none;
}

.hdp .sign button.on {
  color: var(--kin-color-accent);
  background: color-mix(in srgb, var(--kin-color-accent) 14%, transparent);
  box-shadow: inset 0 0 0 1px var(--kin-color-accent);
}

.hdp .head {
  display: flex;
  gap: 4px;
  align-items: center;
}

.hdp .head .kin-icon-btn {
  width: 30px;
  height: 30px;
}

.hdp .head-title {
  display: flex;
  flex: 1;
  gap: 6px;
  align-items: center;
  justify-content: center;
  height: 30px;
  font-size: 14px;
  font-weight: 500;
  color: var(--kin-color-text);
  cursor: default;
  background: transparent;
  border: none;
  border-radius: var(--kin-radius);
}

.hdp .head-title.up {
  cursor: pointer;
}

.hdp .head-title:disabled {
  opacity: 1;
}

.hdp .head-title .kin-icon,
.hdp .head-sub {
  font-size: 12px;
  font-weight: 400;
  color: var(--kin-color-text-muted);
}

.hdp .head-title .kin-icon {
  width: 12px;
  height: 12px;
}

.hdp .weekdays {
  display: grid;
  grid-template-columns: repeat(7, minmax(0, 1fr));
  gap: 2px;
  font-size: 11px;
  color: var(--kin-color-text-muted);
  text-align: center;
}

.hdp .grid {
  display: grid;
  gap: 4px;
}

.hdp .grid.size-s {
  gap: 2px;
}

.hdp .cell {
  display: flex;
  flex-direction: column;
  gap: 1px;
  align-items: center;
  justify-content: center;
  height: 44px;
  padding: 0;
  font-size: 14px;
  font-variant-numeric: tabular-nums;
  color: var(--kin-color-text);
  cursor: pointer;
  background: transparent;
  border: 1px solid transparent;
  border-radius: var(--kin-radius);
}

.hdp .size-l .cell {
  height: 48px;
}

.hdp .size-s .cell {
  height: 34px;
}

.hdp .cell:hover,
.hdp .tab:hover,
.hdp .head-title.up:hover,
.hdp .sign button:hover {
  background: color-mix(in srgb, var(--kin-color-accent) 12%, transparent);
}

.hdp .cell.muted {
  color: var(--kin-color-text-muted);
}

.hdp .cell.ring {
  border-color: var(--kin-color-border);
}

.hdp .cell.on {
  color: var(--kin-color-accent);
  background: color-mix(in srgb, var(--kin-color-accent) 14%, transparent);
  border-color: var(--kin-color-accent);
}

.hdp .cell.small {
  font-size: 11px;
}

.hdp .cell-sub {
  font-size: 10px;
  color: var(--kin-color-text-muted);
}

.hdp .note {
  margin: 0;
  font-size: 12px;
  color: var(--kin-color-text-muted);
}

.hdp .foot {
  display: flex;
  gap: 8px;
  align-items: center;
}

.hdp .foot .grow {
  flex: 1;
}

.hdp .ghost {
  background: transparent;
  border-color: transparent;
}

.hdp .recent {
  color: var(--kin-color-text-muted);
}
</style>
