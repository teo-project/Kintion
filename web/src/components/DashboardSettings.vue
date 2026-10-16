<script setup lang="ts">
// Настройки дашборда (F-132): сверху «Настройки / Шаблоны / Доступ». «Настройки» —
// название; «Шаблоны» — остров на шаблон: поля быстрой формы (F-124) облаками плашек
// (включённые — в порядке формы, выключенные — ниже; клик переносит), переопределение
// `quick_fields` шаблона в дашборде, пак — по умолчанию. Обязательные поля пака
// (`required`) и роли с `min` ≥ 1 закреплены: убрать нельзя, поля — только порядок.
// Каждый шаг (перенос плашки, порядок, «По умолчанию») сохраняется сразу, как поле
// карточки (F-86): остров подсвечен — сохраняется, сохранено (затухает), ошибка.
// Шаги шаблона уходят по очереди с текущей версией шаблона (F-45); быстрые клики,
// сделанные во время сохранения, объединяются в один следующий запрос.
import { DialogClose, DialogContent, DialogOverlay, DialogPortal, DialogRoot, DialogTitle, TabsContent, TabsList, TabsRoot, TabsTrigger } from 'reka-ui'
import { computed, nextTick, onBeforeUnmount, ref, shallowRef, watch } from 'vue'

import { api, ApiError } from '@/api/client'
import { quickInputs } from '@/core/quickAdd'
import type { Registry } from '@/core/registry'
import type { SchemaTemplateDTO } from '@/core/types'
import { errorText, t } from '@/i18n'
import { useDashboardsStore } from '@/stores/dashboards'
import { makeCtx } from '@/ui/ctx'

import KinIcon from './KinIcon.vue'

const props = defineProps<{ dashboard: string; name: string }>()
const open = defineModel<boolean>('open', { required: true })

const dashboards = useDashboardsStore()
const reg = shallowRef<Registry | null>(null)
const title = ref('')
const error = ref('')
const busy = ref(false)
const tab = ref('settings')
/** выбранные адреса полей быстрой формы по шаблону (порядок = порядок в форме) */
const chosen = ref<Record<string, string[]>>({})

const templates = computed<SchemaTemplateDTO[]>(() => (reg.value ? [...reg.value.templates.values()].sort((a, b) => Number(b.default) - Number(a.default)) : []))

/** only — перечитать набор одного шаблона (после его сохранения), несохранённые правки остальных остаются */
function load(r: Registry, only?: string) {
  reg.value = r
  const fresh = Object.fromEntries([...r.templates.values()].map(x => [x.id, [...(x.definition.quick_fields ?? [])]]))
  // набор шаблона, который ещё сохраняется, — локальный (иначе его несохранённые шаги пропали бы)
  const keep = (id: string) => id in fresh && id !== only && !!queue.get(id)?.busy
  chosen.value = { ...fresh, ...Object.fromEntries(Object.entries(chosen.value).filter(([id]) => keep(id))) }
}

// реестр правят и другие экраны (общие свои поля в карточке): версия шаблона —
// текущая; набор изменившегося шаблона, который здесь не сохраняется, — с сервера
watch(() => dashboards.current(props.dashboard), r => {
  if (!r || !reg.value || r === reg.value) return
  const old = reg.value
  reg.value = r
  const next = { ...chosen.value }
  for (const x of r.byRowId.values()) {
    if (old.byRowId.get(x.id)?.rev !== x.rev && !queue.get(x.id)?.busy) next[x.id] = [...(x.definition.quick_fields ?? [])]
  }
  chosen.value = next
})

// другой дашборд (например, «назад» в браузере) — настройки прежнего закрываются,
// его набор и реестр не показываются (очередь доживает сама)
watch(() => props.dashboard, () => {
  open.value = false
  reg.value = null
  chosen.value = {}
})

watch(open, async v => {
  if (!v) return
  title.value = props.name
  tab.value = 'settings'
  error.value = ''
  const d = props.dashboard
  try {
    const r = await dashboards.registry(d)
    if (d === props.dashboard) load(r)
  } catch (e) {
    if (d === props.dashboard) error.value = errorText(e)
  }
})

/** Все адреса, которые можно поставить в быструю форму шаблона, с подписями. */
function candidates(tpl: SchemaTemplateDTO): { addr: string; label: string }[] {
  if (!reg.value) return []
  const addrs = ['.note', 'start', 'end', ...(tpl.definition.fields ?? []).map(f => f.key)]
  const ctx = makeCtx(reg.value, [])
  return quickInputs(reg.value, ctx.words, { ...tpl.definition, quick_fields: addrs }).map(i => ({ addr: i.addr, label: i.label }))
}

/** Закреплённые поля: обязательные по паку (`required`); снять с формы нельзя. */
function pinnedAddrs(tpl: SchemaTemplateDTO): string[] {
  return (tpl.definition.fields ?? []).filter(f => f.required === true).map(f => f.key)
}

/** Роли с `min` ≥ 1 (родитель в «Родитель — ребёнок»): всегда в форме, адреса у них нет. */
function pinnedRoles(tpl: SchemaTemplateDTO): { key: string; label: string }[] {
  return (tpl.definition.roles ?? []).filter(r => (r.min ?? 0) >= 1).map(r => ({ key: r.key, label: r.forms.default }))
}

/** Включённые в порядке формы; закреплённые, которых в наборе нет, — в начале. */
function inForm(tpl: SchemaTemplateDTO) {
  const byAddr = new Map(candidates(tpl).map(c => [c.addr, c]))
  const sel = chosen.value[tpl.id] ?? []
  const pinned = pinnedAddrs(tpl)
  const addrs = [...pinned.filter(a => !sel.includes(a)), ...sel]
  return addrs.flatMap(a => (byAddr.has(a) ? [{ ...byAddr.get(a)!, pinned: pinned.includes(a) }] : []))
}

function outForm(tpl: SchemaTemplateDTO) {
  const on = new Set(inForm(tpl).map(x => x.addr))
  return candidates(tpl).filter(c => !on.has(c.addr))
}

/** Текущий набор вместе с закреплёнными. */
function current(tpl: SchemaTemplateDTO): string[] {
  return inForm(tpl).map(x => x.addr)
}

function setChosen(tpl: SchemaTemplateDTO, list: string[]) {
  if (resetting(tpl)) return
  chosen.value = { ...chosen.value, [tpl.id]: list }
  // набор для запроса — сейчас, по реестру, где сделан шаг
  void saveQuick(tpl.id, false, current(tpl))
}

/** Клик по плашке переносит её между облаками; закреплённую — нет. */
function toggle(tpl: SchemaTemplateDTO, addr: string) {
  if (pinnedAddrs(tpl).includes(addr)) return
  const cur = current(tpl)
  setChosen(tpl, cur.includes(addr) ? cur.filter(a => a !== addr) : [...cur, addr])
}

function move(tpl: SchemaTemplateDTO, addr: string, by: -1 | 1) {
  const sel = current(tpl)
  const i = sel.indexOf(addr)
  const j = i + by
  if (i < 0 || j < 0 || j >= sel.length) return
  ;[sel[i], sel[j]] = [sel[j]!, sel[i]!]
  setChosen(tpl, sel)
}

/** Alt + ← / → двигает плашку в форме (для клавиатуры). */
function onKey(e: KeyboardEvent, tpl: SchemaTemplateDTO, addr: string) {
  if (!e.altKey || (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight')) return
  e.preventDefault()
  const el = e.currentTarget as HTMLElement | null
  move(tpl, addr, e.key === 'ArrowLeft' ? -1 : 1)
  void nextTick(() => el?.focus())
}

/** перетаскиваемая плашка: шаблон и адрес */
const drag = ref<{ tpl: string; addr: string } | null>(null)

function onDragStart(e: DragEvent, tpl: SchemaTemplateDTO, addr: string) {
  drag.value = { tpl: tpl.id, addr }
  e.dataTransfer?.setData('text/plain', addr)
  if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move'
}

/** Бросок на плашку включённых — вставить перед ней; на облако — в конец; на «не в форме» — убрать. */
function onDrop(tpl: SchemaTemplateDTO, target: string | null, into: boolean) {
  const d = drag.value
  drag.value = null
  if (!d || d.tpl !== tpl.id || d.addr === target) return
  if (!into) {
    if (!pinnedAddrs(tpl).includes(d.addr)) setChosen(tpl, current(tpl).filter(a => a !== d.addr))
    return
  }
  const rest = current(tpl).filter(a => a !== d.addr)
  const at = target ? rest.indexOf(target) : -1
  rest.splice(at < 0 ? rest.length : at, 0, d.addr)
  setChosen(tpl, rest)
}

async function run(fn: () => Promise<unknown>) {
  error.value = ''
  busy.value = true
  try {
    await fn()
  } catch (e) {
    error.value = errorText(e)
  } finally {
    busy.value = false
  }
}

async function renameDashboard() {
  const n = title.value.trim()
  if (!n || n === props.name) return
  await run(() => dashboards.rename(props.dashboard, n))
}

/** итог сохранения набора шаблона (F-86): сохраняется / сохранено (затухает) / ошибка */
type QuickState = 'saving' | 'saved' | 'error'
const quickState = ref<Record<string, QuickState | undefined>>({})
/** по шаблону: идёт запрос; есть несохранённый шаг; следующий шаг — «По умолчанию» */
const queue = new Map<string, { busy: boolean; pending: boolean; reset: boolean; list: string[] }>()
const timers = new Map<string, ReturnType<typeof setTimeout>>()
onBeforeUnmount(() => timers.forEach(clearTimeout))

/** длительность затухания подсветки «сохранено», мс (как у поля, F-86) */
const SAVED_MS = 3000

function setState(id: string, v: QuickState | undefined) {
  clearTimeout(timers.get(id))
  quickState.value = { ...quickState.value, [id]: v }
  if (v === 'saved') timers.set(id, setTimeout(() => (quickState.value = { ...quickState.value, [id]: undefined }), SAVED_MS))
}

/** чьё сейчас сообщение об ошибке (шаблон): успех другого шаблона его не стирает */
let errorOf = ''

function showError(id: string, msg: string) {
  errorOf = msg ? id : ''
  error.value = msg
}

/** Пока «По умолчанию» шаблона не сохранено или шаблон перечитывается после
 * конфликта, его плашки не трогаем: набор известен только из ответа. */
const resetBusy = ref<Record<string, boolean | undefined>>({})

function resetting(tpl: SchemaTemplateDTO): boolean {
  return !!resetBusy.value[tpl.id]
}

function lock(id: string, on: boolean) {
  resetBusy.value = { ...resetBusy.value, [id]: on }
}

/**
 * Сохранить набор шаблона: запросы по шаблону идут по одному, каждый — с
 * версией из последнего ответа; шаги, сделанные во время запроса, уходят
 * следующим запросом одним набором (последнее состояние).
 */
async function saveQuick(id: string, reset = false, list: string[] = []) {
  const q = queue.get(id) ?? { busy: false, pending: false, reset: false, list: [] }
  queue.set(id, q)
  q.pending = true
  if (!reset) {
    q.list = list
    q.reset = false
  }
  if (reset) {
    q.reset = true
    lock(id, true)
  }
  setState(id, 'saving')
  if (q.busy) return
  q.busy = true
  // общее сообщение — об ошибке последнего шаблона; чужая очередь его не стирает
  if (errorOf === id) showError(id, '')
  // очередь привязана к дашборду, где начата: настройки остаются смонтированными
  // и при переключении дашборда, ответ не должен попасть в чужой реестр
  const d = props.dashboard
  const here = () => d === props.dashboard
  try {
    while (q.pending) {
      q.pending = false
      // реестр дашборда очереди — из общего (на экране его может ещё не быть)
      const src = () => dashboards.current(d) ?? (here() ? reg.value : null)
      const tpl = src()?.byRowId.get(id)
      if (!tpl) return
      const doReset = q.reset
      q.reset = false
      try {
        const dto = await api.putQuickFields(d, id, { if_rev: tpl.rev ?? 0, quick_fields: doReset ? [] : q.list })
        const base = src()
        if (base) {
          const next = dashboards.replaceTemplate(d, base, dto)
          if (here() && reg.value) reg.value = next
        }
        // набор с сервера — только когда новых шагов нет (иначе они бы пропали)
        if (!q.pending) chosen.value = { ...chosen.value, [id]: [...(dto.definition.quick_fields ?? [])] }
      } catch (e) {
        if (e instanceof ApiError && e.status === 409) {
          // конфликт (F-45): шаги отброшены, шаблон перечитан и показан текущий
          // набор; пока читается — плашки заперты
          q.pending = false
          q.reset = false
          lock(id, true)
          try {
            dashboards.forget(d)
            const r = await dashboards.registry(d)
            if (here()) load(r, id)
          } finally {
            lock(id, false)
          }
          if (here()) showError(id, t.dashboard.quickStale)
          setState(id, 'error')
          return
        }
        // другая ошибка (сеть, сервер): показанный набор остаётся, как введённое в
        // поле (F-86); шаги, сделанные во время запроса, уходят следующим запросом,
        // иначе — повторит следующий шаг
        if (here()) showError(id, errorText(e))
        if (!q.pending) {
          setState(id, 'error')
          return
        }
      } finally {
        if (doReset) lock(id, false)
      }
    }
    if (here() && errorOf === id) showError(id, '')
    setState(id, 'saved')
  } catch (e) {
    if (here()) showError(id, errorText(e))
    setState(id, 'error')
  } finally {
    q.busy = false
  }
}
</script>

<template>
  <DialogRoot v-model:open="open">
    <DialogPortal>
      <DialogOverlay class="kin-overlay" />
      <DialogContent
        class="kin-dialog settings"
        :aria-describedby="undefined"
      >
        <DialogTitle>{{ t.dashboard.settings }}</DialogTitle>
        <TabsRoot
          v-model="tab"
          class="tabs-root"
        >
          <TabsList
            class="kin-seg"
            :aria-label="t.dashboard.settings"
          >
            <TabsTrigger
              class="seg-item"
              value="settings"
            >
              {{ t.dashboard.tabSettings }}
            </TabsTrigger>
            <TabsTrigger
              class="seg-item"
              value="templates"
            >
              {{ t.dashboard.settingsTemplates }}
            </TabsTrigger>
            <TabsTrigger
              class="seg-item"
              value="access"
            >
              {{ t.dashboard.tabAccess }}
            </TabsTrigger>
          </TabsList>
          <div class="body">
            <TabsContent
              value="settings"
              class="pane"
            >
              <section class="kin-island pane-island">
                <h3 class="kin-section-title">
                  {{ t.dashboard.settingsDashboard }}
                </h3>
                <form
                  class="kin-row"
                  @submit.prevent="renameDashboard"
                >
                  <input
                    v-model="title"
                    class="kin-input grow"
                    name="dashboard-name"
                    :aria-label="t.dashboard.newDashboardName"
                    required
                  >
                  <button
                    class="kin-ibtn kin-ibtn--primary"
                    type="submit"
                    :disabled="busy"
                    :aria-label="t.common.save"
                    :title="t.common.save"
                  >
                    <KinIcon name="check" />
                  </button>
                </form>
              </section>
            </TabsContent>
            <TabsContent
              value="templates"
              class="pane"
            >
              <section
                v-for="tpl in templates"
                :key="tpl.id"
                class="kin-island pane-island quick"
                :class="quickState[tpl.id] && `quick--${quickState[tpl.id]}`"
                :data-testid="`quick-${tpl.key}`"
                :data-state="quickState[tpl.id] ?? 'idle'"
              >
                <div class="kin-row">
                  <h3 class="kin-section-title grow">
                    {{ tpl.definition.name }}
                  </h3>
                  <button
                    class="kin-btn kin-btn--small"
                    type="button"
                    :disabled="resetting(tpl)"
                    @click="saveQuick(tpl.id, true)"
                  >
                    {{ t.dashboard.quickReset }}
                  </button>
                </div>
                <p class="kin-muted hint">
                  {{ t.dashboard.quickHint }}
                </p>
                <div class="kin-muted cloud-label">
                  {{ t.dashboard.quickOn }}
                </div>
                <ul
                  class="cloud cloud-on"
                  :aria-label="t.dashboard.quickOn"
                  @dragover.prevent
                  @drop.prevent="onDrop(tpl, null, true)"
                >
                  <li
                    v-for="r in pinnedRoles(tpl)"
                    :key="'role:' + r.key"
                  >
                    <span
                      class="pill pinned"
                      :title="t.dashboard.quickPinned"
                    >
                      <KinIcon name="lock" />{{ r.label }}
                    </span>
                  </li>
                  <li
                    v-for="r in inForm(tpl)"
                    :key="r.addr"
                    @dragover.prevent
                    @drop.prevent.stop="onDrop(tpl, r.addr, true)"
                  >
                    <button
                      class="pill"
                      :class="{ pinned: r.pinned }"
                      type="button"
                      draggable="true"
                      aria-pressed="true"
                      :aria-disabled="r.pinned || undefined"
                      :title="r.pinned ? t.dashboard.quickPinned : t.dashboard.quickHint"
                      @click="toggle(tpl, r.addr)"
                      @keydown="e => onKey(e, tpl, r.addr)"
                      @dragstart="e => onDragStart(e, tpl, r.addr)"
                      @dragend="drag = null"
                    >
                      <KinIcon
                        v-if="r.pinned"
                        name="lock"
                      />{{ r.label }}
                    </button>
                  </li>
                </ul>
                <div class="kin-muted cloud-label">
                  {{ t.dashboard.quickOff }}
                </div>
                <ul
                  class="cloud cloud-off"
                  :aria-label="t.dashboard.quickOff"
                  @dragover.prevent
                  @drop.prevent="onDrop(tpl, null, false)"
                >
                  <li
                    v-for="r in outForm(tpl)"
                    :key="r.addr"
                  >
                    <button
                      class="pill pill-off"
                      type="button"
                      draggable="true"
                      aria-pressed="false"
                      @click="toggle(tpl, r.addr)"
                      @dragstart="e => onDragStart(e, tpl, r.addr)"
                      @dragend="drag = null"
                    >
                      {{ r.label }}
                    </button>
                  </li>
                </ul>
              </section>
            </TabsContent>
            <TabsContent
              value="access"
              class="pane"
            >
              <section class="kin-island pane-island">
                <p class="kin-muted soon">
                  {{ t.dashboard.accessSoon }}
                </p>
              </section>
            </TabsContent>
          </div>
        </TabsRoot>
        <p
          v-if="error"
          class="kin-error"
          role="alert"
        >
          {{ error }}
        </p>
        <DialogClose
          class="kin-icon-btn kin-dialog-x"
          :aria-label="t.common.close"
          :title="t.common.close"
        >
          <KinIcon name="close" />
        </DialogClose>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<style scoped>
/* окно скруглено со всех сторон: прокручивается внутреннее поле, а не само окно */
.settings {
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-3);
  width: min(640px, calc(100vw - 2 * var(--kin-space-3)));
  overflow: hidden;
}

.tabs-root {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--kin-space-3);
  min-height: 0;
}

.body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
}

.pane {
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-3);
}

.pane-island {
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-2);
  background: var(--kin-color-bg);
  box-shadow: none;
}

/* состояния сохранения набора (F-86): сохраняется — жёлтый, сохранено — салатовый (затухает), ошибка — красный и тряска */
.quick {
  border: 1px solid transparent;
}

.quick--saving {
  border-color: var(--kin-color-dirty);
}

.quick--saved {
  animation: quick-saved 3s ease-in forwards;
}

.quick--error {
  border-color: var(--kin-color-danger);
  animation: quick-shake 0.5s ease-in-out;
}

@keyframes quick-saved {
  from {
    border-color: var(--kin-color-saved);
  }

  to {
    border-color: transparent;
  }
}

@keyframes quick-shake {
  0%,
  100% {
    transform: translateX(0);
  }

  20%,
  60% {
    transform: translateX(-4px);
  }

  40%,
  80% {
    transform: translateX(4px);
  }
}

@media (prefers-reduced-motion: reduce) {
  .quick--error {
    animation: none;
  }
}

.pane-island h3 {
  margin: 0;
}

.hint,
.soon {
  margin: 0;
  font-size: var(--kin-font-tiny);
}

.cloud-label {
  font-size: var(--kin-font-tiny);
}

.cloud {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kin-space-2);
  min-height: 34px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.pill {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  padding: 5px 12px;
  color: var(--kin-color-accent-text);
  background: var(--kin-color-accent-soft);
  border: 1px solid transparent;
  border-radius: 999px;
  font: inherit;
  font-size: var(--kin-font-small);
  font-weight: 600;
  cursor: pointer;
}

.pill:hover {
  border-color: var(--kin-color-accent);
}

.pill:focus-visible {
  outline: 2px solid var(--kin-color-focus);
  outline-offset: 2px;
}

.pill.pinned {
  color: var(--kin-color-text-muted);
  cursor: default;
}

.pill.pinned:hover {
  border-color: transparent;
}

.pill-off {
  color: var(--kin-color-text-muted);
  background: none;
  border-color: var(--kin-color-border-strong);
}

.grow {
  flex: 1;
  min-width: 0;
}
</style>
