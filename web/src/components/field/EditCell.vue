<script setup lang="ts">
// Поле ввода в режиме редактирования карточки (F-85, F-86): значение
// сразу открыто для ввода, сохраняется при потере фокуса; Esc — отмена
// изменений. Контур — по состоянию (токены темы): изменено в фокусе —
// жёлтый; сохранено — салатовый, затухает за 3 с; не сохранилось — красный
// и тряска полсекунды, держится до следующего фокуса (конфликт версии,
// F-45, — поле уже обновлено до текущего). Что и как сохранять — решает
// вызывающий (`save`).
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'

import type { PartialDate } from '@/core/dates'
import type { FieldDef } from '@/core/types'
import { t } from '@/i18n'
import type { SaveResult } from '@/ui/record'
import { isInvalid, normalize, sameValue } from '@/ui/values'

import PartialDateInput from '../PartialDateInput.vue'
import ValueEditor from './ValueEditor.vue'

const props = defineProps<{
  def: Pick<FieldDef, 'type' | 'options' | 'free'> & { name?: string; entities?: unknown }
  /** сохранённое значение */
  value: unknown
  dashboard: string
  save: (v: unknown) => Promise<SaveResult>
  /** многострочный текст (примечание) */
  multiline?: boolean
  /** после сохранения очистить ввод (новое значение multiple) */
  fresh?: boolean
}>()
/** settled — уход из поля обработан (итог сохранения; без изменений — ok);
 * escape — Esc, изменения отменены (правка по месту закрывается, F-108) */
const emit = defineEmits<{ settled: [SaveResult]; escape: [] }>()

const draft = ref<unknown>(props.value ?? null)
const focused = ref(false)
const busy = ref(false)
const localError = ref('')
const root = ref<HTMLElement | null>(null)
/** итог последнего сохранения: салатовый (затухает) или красный (до фокуса) */
const status = ref<'idle' | 'saved' | 'error'>('idle')
let timer: ReturnType<typeof setTimeout> | undefined
onBeforeUnmount(() => clearTimeout(timer))

/** длительность затухания салатового контура, мс */
const SAVED_MS = 3000

function setStatus(v: 'idle' | 'saved' | 'error') {
  clearTimeout(timer)
  status.value = v
  if (v === 'saved') timer = setTimeout(() => (status.value = 'idle'), SAVED_MS)
}

const changed = computed(() => !sameValue(draft.value, props.value))

watch(() => props.value, v => {
  if (!focused.value && !busy.value) draft.value = v ?? null
})

function onFocusIn() {
  if (!focused.value && status.value === 'error') setStatus('idle')
  focused.value = true
  localError.value = ''
}

/** открыт список поля в портале (вариант «выбора или своего»): фокус уходит в
 * список, но поле не покинуто — сохранение по выбору или по закрытию списка */
const popup = ref(false)

async function onFocusOut(e: FocusEvent) {
  if (e.relatedTarget instanceof Node && root.value?.contains(e.relatedTarget)) return
  if (popup.value) return
  await settle()
}

/** Значение выбрано одним действием (вариант из списка, «да/нет») — сохраняем
 * сразу, не дожидаясь ухода из поля: после выбора фокус может уже быть вне
 * поля, и потери фокуса больше не будет. */
async function onPick() {
  await nextTick()
  await settle()
}

/** Список закрыт без выбора, а фокус уже вне поля — это уход из поля. */
async function onPopup(open: boolean) {
  popup.value = open
  if (open) return
  await nextTick()
  if (focused.value && !root.value?.contains(document.activeElement)) await settle()
}

async function settle() {
  focused.value = false
  if (busy.value) return
  if (!changed.value) {
    emit('settled', 'ok')
    return
  }
  if (isInvalid(props.def, draft.value)) {
    localError.value = t.card.dateError
    setStatus('error')
    emit('settled', 'error')
    return
  }
  busy.value = true
  let res: SaveResult
  try {
    res = await props.save(normalize(draft.value))
  } finally {
    busy.value = false
  }
  if (res === 'ok' && props.fresh) draft.value = null
  else if (res === 'conflict' || res === 'cancel' || res === 'ok') draft.value = props.value ?? null
  setStatus(res === 'ok' ? 'saved' : res === 'cancel' ? 'idle' : 'error')
  emit('settled', res)
}

/** Esc — вернуть сохранённое значение. */
function revert() {
  draft.value = props.value ?? null
  localError.value = ''
  setStatus('idle')
  emit('escape')
}

/** Enter в однострочном поле — как уход из поля. */
function commit() {
  const el = document.activeElement
  if (el instanceof HTMLElement && root.value?.contains(el)) el.blur()
}

const text = computed(() => (draft.value == null ? '' : String(draft.value)))
</script>

<template>
  <div
    ref="root"
    class="cell"
    :class="{
      'cell--dirty': focused && changed,
      'cell--saved': status === 'saved' && !(focused && changed),
      'cell--error': status === 'error',
    }"
    :data-state="focused && changed ? 'dirty' : status"
    @focusin="onFocusIn"
    @focusout="onFocusOut"
    @keydown.esc.stop="revert"
  >
    <PartialDateInput
      v-if="def.type === 'partial_date'"
      :model-value="(draft as PartialDate | null)"
      :label="undefined"
      @update:model-value="v => (draft = v)"
      @keydown.enter.prevent="commit"
    />
    <textarea
      v-else-if="multiline"
      class="kin-input"
      rows="3"
      :value="text"
      :aria-label="def.name"
      @input="e => (draft = (e.target as HTMLTextAreaElement).value || null)"
    />
    <ValueEditor
      v-else
      v-model="draft"
      :def="def"
      :dashboard="dashboard"
      :disabled="busy"
      @pick="onPick"
      @popup="onPopup"
      @cancel="revert"
      @submit="commit"
    />
    <p
      v-if="localError"
      class="kin-error"
      role="alert"
    >
      {{ localError }}
    </p>
  </div>
</template>

<style scoped>
/* состояния F-86 — рамка поля ввода и фон *-soft (F-90) */
.cell {
  display: inline-flex;
  flex-direction: column;
  gap: var(--kin-space-1);
  border-radius: var(--kin-radius);
}

.cell--dirty :deep(.kin-input) {
  background-color: var(--kin-color-dirty-soft);
  border-color: var(--kin-color-dirty);
}

.cell--saved :deep(.kin-input) {
  animation: cell-saved 3s ease-in forwards;
}

.cell--error {
  animation: cell-shake 0.5s ease-in-out;
}

.cell--error :deep(.kin-input) {
  background-color: var(--kin-color-danger-soft);
  border-color: var(--kin-color-danger);
}

@keyframes cell-saved {
  from {
    background-color: var(--kin-color-saved-soft);
    border-color: var(--kin-color-saved);
  }

  to {
    background-color: var(--kin-color-surface-sunken);
    border-color: var(--kin-color-border-strong);
  }
}


@keyframes cell-shake {
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
  .cell--error {
    animation: none;
  }
}
</style>
