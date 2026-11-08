<script setup lang="ts">
// Эры — раздел настроек дашборда (F-143), в стиле облаков быстрой формы (F-132)
// и полей карточки (F-86): без кнопок сохранения — переключатели «да / нет»
// справа и поля сохраняются по изменению (Enter или уход из поля; Esc —
// отмена); остров подсвечен — сохраняется, сохранено (затухает), ошибка.
// Запросы идут по одному; обычные шаги, сделанные во время запроса, уходят
// следующим одним набором; замена эры в датах («к уже созданным»,
// переименование) — отдельным запросом, по порядку.
// Список «Хронологический порядок» — эры из дат дашборда и эра по умолчанию
// (собирает сервер); у каждой — необязательные минимальный и максимальный
// год: минимальный больше максимального — счёт обратный.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

import { api, ApiError } from '@/api/client'
import { eraReversed, validEra, type PartialDate } from '@/core/dates'
import type { EraYears, ErasDTO, ErasSettings } from '@/core/types'
import { errorText, t } from '@/i18n'
import { useDashboardsStore } from '@/stores/dashboards'

import KinBool from './field/KinBool.vue'

const props = defineProps<{ dashboard: string }>()

const dashboards = useDashboardsStore()
/** показанные эры: шаги применяются сразу, ответ сервера — когда новых шагов нет */
const cur = ref<ErasDTO | null>(null)
const error = ref('')
type SaveState = 'saving' | 'saved' | 'error'
const state = ref<SaveState | undefined>()
let timer: ReturnType<typeof setTimeout> | undefined
onBeforeUnmount(() => clearTimeout(timer))

// ---------- очередь сохранения ----------

/** шаг сохранения: обычный (настройки целиком) или с заменой эры в датах */
interface Step { replace?: { from: string; to: string } }
const steps: Step[] = []
let busy = false

const reg = computed(() => dashboards.current(props.dashboard))
watch(reg, r => {
  if (r && !busy && !steps.length) cur.value = clone(r.eras)
}, { immediate: true })

// список эр мог измениться (новая эра в дате) — при открытии перечитываются
onMounted(async () => {
  const d = props.dashboard
  try {
    const eras = await api.eras(d)
    const base = dashboards.current(d)
    // ответ мог устареть: за время чтения сохранён шаг — не откатывать (F-45)
    if (base && !busy && !steps.length && (eras.rev ?? 0) >= (base.eras.rev ?? 0)) dashboards.setEras(d, base, eras, false)
  } catch {
    // показаны эры из загруженного реестра
  }
})

function clone(e: ErasDTO): ErasDTO {
  return { ...e, order: [...e.order], list: [...e.list], years: { ...(e.years ?? {}) }, today: e.today ? { ...e.today } : null }
}

function setState(v: SaveState | undefined) {
  clearTimeout(timer)
  state.value = v
  if (v === 'saved') timer = setTimeout(() => (state.value = undefined), 3000)
}

function settings(e: ErasDTO): ErasSettings {
  return { show: e.show, edit: e.edit, default: e.default, order: e.order, years: e.years ?? {}, today: e.today }
}

/** порядок — таким, каким его видит админ: новая эра остаётся наверху (F-143) */
function addOnTop(c: ErasDTO, e: string) {
  if (!c.list.includes(e)) c.list = [e, ...c.list]
  c.order = [...c.list]
}

function step(s: Step = {}) {
  steps.push(s)
  setState('saving')
  void flush()
}

async function flush() {
  if (busy) return
  busy = true
  const d = props.dashboard
  error.value = ''
  try {
    while (steps.length && cur.value && d === props.dashboard) {
      // замена — отдельно; подряд идущие обычные шаги — одним набором
      const first = steps[0]!
      const nextReplace = steps.findIndex(s => !!s.replace)
      steps.splice(0, first.replace ? 1 : nextReplace < 0 ? steps.length : nextReplace)
      const out = await api.putEras(d, { if_rev: cur.value.rev ?? 0, eras: settings(cur.value), ...(first.replace ? { replace: first.replace } : {}) })
      const base = dashboards.current(d)
      if (base) dashboards.setEras(d, base, out, !!first.replace)
      if (d !== props.dashboard) break
      // следующий запрос — с версией из ответа (F-45)
      cur.value.rev = out.rev
      if (!steps.length) cur.value = clone(out)
    }
    setState('saved')
  } catch (e) {
    steps.length = 0
    if (d === props.dashboard) {
      setState('error')
      if (e instanceof ApiError && e.status === 409) {
        // эры изменены в другом окне (F-45): шаги отброшены, показаны текущие
        error.value = t.dashboard.erasStale
        try {
          const eras = await api.eras(d)
          const base = dashboards.current(d)
          if (base) dashboards.setEras(d, base, eras, true)
        } catch {
          // останутся эры из реестра
        }
      } else error.value = errorText(e)
      const r = dashboards.current(d)
      if (r) cur.value = clone(r.eras)
    }
  } finally {
    busy = false
  }
}

// ---------- переключатели ----------

function setFlag(key: 'show' | 'edit', v: boolean | null) {
  // повторный клик по выбранному — без изменений: у настройки пустого нет
  if (!cur.value || v === null || cur.value[key] === v) return
  cur.value[key] = v
  step()
}

// ---------- переименование и замена ----------

function renameIn(list: string[], from: string, to: string): string[] {
  const has = list.includes(to)
  return list.flatMap(x => (x === from ? (has ? [] : [to]) : [x]))
}

/** Замена эры from на to: в датах (на сервере), в порядке, годах, эре по умолчанию и годе мира. */
function replaceEra(from: string, to: string) {
  const c = cur.value
  if (!c || from === to) return
  // порядок фиксируется таким, каким его видит админ
  c.order = renameIn(c.list, from, to)
  c.list = [...c.order]
  const years = { ...(c.years ?? {}) }
  if (from in years) {
    if (!(to in years)) years[to] = years[from]!
    delete years[from]
  }
  c.years = years
  if (c.default === from) c.default = to
  if (c.today && (c.today.e ?? '') === from) c.today = { ...c.today, e: to || undefined }
  step({ replace: { from, to } })
}

/** локальная ошибка ввода (не сохранено): неверная эра или год */
const inputError = ref('')

/** Переименование эры в списке — по изменению поля названия. */
function rename(from: string, ev: Event) {
  const el = ev.target as HTMLInputElement
  const to = el.value.trim()
  inputError.value = ''
  if (to === from) return
  if (to !== '' && !validEra(to)) {
    inputError.value = t.dashboard.eraBad
    el.value = from
    return
  }
  // пустое название при непустой эре по умолчанию — «без эры»: даты остаются
  // без эры, дата из одной эры очищается (F-143)
  replaceEra(from, to)
}

function revert(ev: Event, v: string) {
  const el = ev.target as HTMLInputElement
  el.value = v
  el.blur()
}

// ---------- эра по умолчанию ----------

const defaultInput = ref('')
watch(() => cur.value?.default, v => (defaultInput.value = v ?? ''), { immediate: true })
/** новая эра по умолчанию ждёт ответа «к уже созданным / только к новым» */
const askDefault = ref<string | null>(null)
const defaultBad = computed(() => defaultInput.value.trim() !== '' && !validEra(defaultInput.value.trim()))

function proposeDefault() {
  const c = cur.value
  const v = defaultInput.value.trim()
  if (!c || defaultBad.value || v === c.default) {
    askDefault.value = null
    return
  }
  // вопрос «к уже созданным / только к новым» — всегда, и при смене с пустой (F-143)
  askDefault.value = v
}

function applyDefault(existing: boolean) {
  const c = cur.value
  const v = askDefault.value
  askDefault.value = null
  if (!c || v === null) return
  if (existing) {
    replaceEra(c.default, v)
    return
  }
  c.default = v
  addOnTop(c, v)
  step()
}

function cancelDefault() {
  askDefault.value = null
  defaultInput.value = cur.value?.default ?? ''
}

// ---------- порядок ----------

function setOrder(list: string[]) {
  if (!cur.value) return
  cur.value.list = list
  cur.value.order = [...list]
  step()
}

const drag = ref<string | null>(null)

function onDrop(target: string) {
  const from = drag.value
  drag.value = null
  if (from === null || !cur.value || from === target) return
  const rest = cur.value.list.filter(x => x !== from)
  rest.splice(rest.indexOf(target), 0, from)
  setOrder(rest)
}

function move(e: string, by: -1 | 1) {
  if (!cur.value) return
  const list = [...cur.value.list]
  const i = list.indexOf(e)
  const j = i + by
  if (i < 0 || j < 0 || j >= list.length) return
  ;[list[i], list[j]] = [list[j]!, list[i]!]
  setOrder(list)
}

function onKey(ev: KeyboardEvent, e: string) {
  if (!ev.altKey || (ev.key !== 'ArrowUp' && ev.key !== 'ArrowDown')) return
  ev.preventDefault()
  move(e, ev.key === 'ArrowUp' ? -1 : 1)
}

// ---------- годы эры ----------

const yearsOf = (e: string): EraYears => cur.value?.years?.[e] ?? {}
const yearText = (v: number | null | undefined) => (v == null ? '' : String(v))

/** Минимальный или максимальный год эры — по изменению поля; пусто — не задан. */
function setYear(e: string, part: 'min' | 'max', ev: Event) {
  const c = cur.value
  const el = ev.target as HTMLInputElement
  const raw = el.value.trim().replace('−', '-')
  inputError.value = ''
  if (!c) return
  const n = raw === '' ? null : Number(raw)
  if (n !== null && (!/^-?\d{1,10}$/.test(raw) || n === 0 || n < -2147483648 || n > 2147483647)) {
    inputError.value = t.dashboard.eraYearBad
    el.value = yearText(yearsOf(e)[part])
    return
  }
  const prev = yearsOf(e)
  if ((prev[part] ?? null) === n) return
  const next: EraYears = { ...prev, [part]: n }
  if (next.min == null) delete next.min
  if (next.max == null) delete next.max
  const years = { ...(c.years ?? {}) }
  if (next.min == null && next.max == null) delete years[e]
  else years[e] = next
  c.years = years
  // порядок фиксируется таким, каким его видит админ
  c.order = [...c.list]
  step()
}

// ---------- текущий год мира ----------

const todayYear = ref('')
const todayBefore = ref(false)
const todayEra = ref('')
watch(() => cur.value?.today, v => {
  todayYear.value = v?.y != null ? String(Math.abs(v.y)) : ''
  todayBefore.value = v?.y != null && v.y < 0
  todayEra.value = v?.e ?? ''
}, { immediate: true })

const todayBad = computed(() => {
  const y = todayYear.value.trim()
  const e = todayEra.value.trim()
  const bad = y !== '' && (!/^\d{1,10}$/.test(y) || Number(y) === 0 || Number(y) > 2147483647)
  return bad || (e !== '' && !validEra(e)) || (y === '' && e !== '')
})

function saveToday() {
  const c = cur.value
  if (!c || todayBad.value) return
  const y = todayYear.value.trim()
  const e = todayEra.value.trim()
  const next: PartialDate | null = y === '' ? null : { y: (todayBefore.value ? -1 : 1) * Number(y), ...(e ? { e } : {}) }
  if (JSON.stringify(next) === JSON.stringify(c.today)) return
  c.today = next
  // порядок фиксируется таким, каким его видит админ; эра года мира в список
  // не добавляется — в нём только эры дат и эра по умолчанию (F-143)
  c.order = [...c.list]
  step()
}

function toggleBefore() {
  todayBefore.value = !todayBefore.value
  saveToday()
}

function clearToday() {
  todayYear.value = ''
  todayEra.value = ''
  todayBefore.value = false
  saveToday()
}

const suggestions = computed(() => (cur.value?.list ?? []).filter(x => x !== ''))
/** подпись пустой эры: «н. э.» при пустой эре по умолчанию, иначе «без эры» (F-143) */
const emptyLabel = computed(() => (cur.value?.default ? t.dashboard.eraNone : t.dashboard.eraOurs))
const eraLabel = (e: string) => (e === '' ? emptyLabel.value : e)
const listId = `eras-${props.dashboard}`
</script>

<template>
  <section
    v-if="cur"
    class="kin-island pane-island eras"
    :class="state && `eras--${state}`"
    data-testid="eras"
    :data-state="state ?? 'idle'"
  >
    <h3 class="kin-section-title">
      {{ t.dashboard.eras }}
    </h3>
    <p class="kin-muted hint">
      {{ t.dashboard.erasHint }}
    </p>
    <datalist :id="listId">
      <option
        v-for="x in suggestions"
        :key="x"
        :value="x"
      />
    </datalist>

    <div class="setting">
      <span class="grow">{{ t.dashboard.eraShow }}</span>
      <KinBool
        :model-value="cur.show"
        :aria-label="t.dashboard.eraShow"
        data-name="era-show"
        @update:model-value="v => setFlag('show', v)"
      />
    </div>
    <div class="setting">
      <span class="grow">{{ t.dashboard.eraEdit }}</span>
      <KinBool
        :model-value="cur.edit"
        :aria-label="t.dashboard.eraEdit"
        data-name="era-edit"
        @update:model-value="v => setFlag('edit', v)"
      />
    </div>

    <label class="setting">
      <span class="grow">{{ t.dashboard.eraDefault }}</span>
      <input
        v-model="defaultInput"
        class="kin-input default"
        name="era-default"
        maxlength="64"
        :list="listId"
        :placeholder="t.dashboard.eraOurs"
        :title="t.dashboard.eraDefaultHint"
        :aria-invalid="defaultBad ? 'true' : undefined"
        @change="proposeDefault"
        @keydown.enter.prevent="proposeDefault"
        @keydown.esc="cancelDefault"
      >
    </label>
    <div
      v-if="askDefault !== null"
      class="ask"
      role="group"
      :aria-label="t.dashboard.eraApply"
    >
      <p class="ask-text">
        {{ t.dashboard.eraApply }}
      </p>
      <div class="kin-row">
        <button
          class="kin-btn kin-btn--small kin-btn--primary"
          type="button"
          data-testid="era-apply-existing"
          @click="applyDefault(true)"
        >
          {{ t.dashboard.eraApplyExisting }}
        </button>
        <button
          class="kin-btn kin-btn--small"
          type="button"
          data-testid="era-apply-new"
          @click="applyDefault(false)"
        >
          {{ t.dashboard.eraApplyNew }}
        </button>
        <button
          class="kin-btn kin-btn--small"
          type="button"
          @click="cancelDefault"
        >
          {{ t.common.cancel }}
        </button>
      </div>
    </div>

    <h4 class="subtitle">
      {{ t.dashboard.eraOrder }}
    </h4>
    <p class="kin-muted hint">
      {{ t.dashboard.eraOrderHint }}
    </p>
    <ol
      class="order"
      :aria-label="t.dashboard.eraOrder"
    >
      <li
        v-for="e in cur.list"
        :key="'era:' + e"
        class="row"
        :class="{ dragging: drag === e }"
        :data-era="e"
        @dragover.prevent
        @drop.prevent="onDrop(e)"
      >
        <span
          class="handle"
          draggable="true"
          tabindex="0"
          role="button"
          :aria-label="`${t.dashboard.eraOrderHint}: ${eraLabel(e)}`"
          @dragstart="ev => { drag = e; ev.dataTransfer?.setData('text/plain', e) }"
          @dragend="drag = null"
          @keydown="ev => onKey(ev, e)"
        >⋮⋮</span>
        <input
          class="kin-input name grow"
          :class="{ ours: e === '' }"
          :value="e"
          maxlength="64"
          :placeholder="emptyLabel"
          :aria-label="t.dashboard.eraRenameTitle"
          :title="t.dashboard.eraRenameTitle"
          @change="ev => rename(e, ev)"
          @keydown.enter.prevent="ev => (ev.target as HTMLInputElement).blur()"
          @keydown.esc="ev => revert(ev, e)"
        >
        <input
          class="kin-input year"
          :value="yearText(yearsOf(e).min)"
          name="era-min"
          inputmode="numeric"
          maxlength="11"
          :placeholder="t.dashboard.eraMin"
          :aria-label="`${t.dashboard.eraMin}: ${eraLabel(e)}`"
          :title="t.dashboard.eraYearsHint"
          @change="ev => setYear(e, 'min', ev)"
          @keydown.enter.prevent="ev => (ev.target as HTMLInputElement).blur()"
          @keydown.esc="ev => revert(ev, yearText(yearsOf(e).min))"
        >
        <span class="kin-muted dash">…</span>
        <input
          class="kin-input year"
          :value="yearText(yearsOf(e).max)"
          name="era-max"
          inputmode="numeric"
          maxlength="11"
          :placeholder="t.dashboard.eraMax"
          :aria-label="`${t.dashboard.eraMax}: ${eraLabel(e)}`"
          :title="t.dashboard.eraYearsHint"
          @change="ev => setYear(e, 'max', ev)"
          @keydown.enter.prevent="ev => (ev.target as HTMLInputElement).blur()"
          @keydown.esc="ev => revert(ev, yearText(yearsOf(e).max))"
        >
        <span
          v-if="eraReversed(yearsOf(e))"
          class="kin-muted rev"
          :title="t.dashboard.eraReversed"
        >↓</span>
      </li>
    </ol>
    <p class="kin-muted hint">
      {{ t.dashboard.eraYearsHint }}
    </p>

    <h4 class="subtitle">
      {{ t.dashboard.eraToday }}
    </h4>
    <div class="kin-row">
      <button
        type="button"
        class="kin-btn kin-btn--small before"
        :class="{ on: todayBefore }"
        :aria-pressed="todayBefore ? 'true' : 'false'"
        :title="t.dates.partBeforeHint"
        @click="toggleBefore"
      >
        {{ t.dates.partBefore }}
      </button>
      <input
        v-model="todayYear"
        class="kin-input year"
        name="era-today-year"
        inputmode="numeric"
        maxlength="10"
        :placeholder="t.dates.partYear"
        :aria-label="t.dates.partYear"
        :title="t.dashboard.eraTodayHint"
        :aria-invalid="todayBad ? 'true' : undefined"
        @change="saveToday"
        @keydown.enter.prevent="saveToday"
      >
      <input
        v-model="todayEra"
        class="kin-input grow"
        name="era-today-era"
        maxlength="64"
        :list="listId"
        :placeholder="cur.default || t.dashboard.eraOurs"
        :aria-label="t.dates.partEra"
        :aria-invalid="todayBad ? 'true' : undefined"
        @change="saveToday"
        @keydown.enter.prevent="saveToday"
      >
      <button
        class="kin-btn kin-btn--small"
        type="button"
        :disabled="!cur.today"
        @click="clearToday"
      >
        {{ t.dashboard.eraTodayClear }}
      </button>
    </div>
    <p class="kin-muted hint">
      {{ t.dashboard.eraTodayHint }}
    </p>
    <p
      v-if="error || defaultBad || inputError"
      class="kin-error"
      role="alert"
    >
      {{ error || inputError || t.dashboard.eraBad }}
    </p>
  </section>
</template>

<style scoped>
.pane-island {
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-2);
  background: var(--kin-color-bg);
  box-shadow: none;
}

.pane-island h3 {
  margin: 0;
}

.eras {
  border: 1px solid transparent;
}

.eras--saving {
  border-color: var(--kin-color-dirty);
}

.eras--saved {
  animation: eras-saved 3s ease-in forwards;
}

.eras--error {
  border-color: var(--kin-color-danger);
}

@keyframes eras-saved {
  from {
    border-color: var(--kin-color-saved);
  }

  to {
    border-color: transparent;
  }
}

.hint {
  margin: 0;
  font-size: var(--kin-font-tiny);
}

/* заголовок части раздела — как заголовок, а не подсказка */
.subtitle {
  margin: var(--kin-space-2) 0 0;
  font-size: inherit;
  font-weight: 600;
  color: var(--kin-color-text);
}

/* строка настройки: подпись слева, переключатель или поле справа */
.setting {
  display: flex;
  gap: var(--kin-space-2);
  align-items: center;
}

.default {
  width: 12rem;
}

.ask {
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-1);
}

.ask-text {
  margin: 0;
}

.order {
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-1);
  margin: 0;
  padding: 0;
  list-style: none;
}

.row {
  display: flex;
  gap: var(--kin-space-2);
  align-items: center;
  padding: 4px 8px;
  border: 1px solid var(--kin-color-border-strong);
  border-radius: 8px;
}

.row.dragging {
  opacity: 0.5;
}

.handle {
  color: var(--kin-color-text-muted);
  cursor: grab;
  user-select: none;
}

.handle:focus-visible {
  outline: 2px solid var(--kin-color-focus);
  outline-offset: 2px;
}

.name.ours::placeholder {
  color: var(--kin-color-text-muted);
}

.year {
  width: 6rem;
}

.before {
  opacity: 0.55;
}

.before.on {
  opacity: 1;
  font-weight: 600;
}

.grow {
  flex: 1;
  min-width: 0;
}
</style>
