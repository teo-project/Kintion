<script setup lang="ts">
// Ввод значения по типу поля. v-model — хранимое значение (как в API):
// строка, число, частичная дата, да/нет, ключ варианта (F-54), id записи.
// null — значения нет. Выбор со своим значением (F-100) — комбобокс:
// вариант — ключом, своё — { free: текст } (пометка в meta, F-104;
// совпавшее с названием варианта — ключом). В API — через valueIn.
import {
  ComboboxAnchor, ComboboxContent, ComboboxInput, ComboboxItem, ComboboxPortal, ComboboxRoot, ComboboxTrigger,
  ComboboxViewport,
} from 'reka-ui'
import { computed, ref } from 'vue'

import { api } from '@/api/client'

import type { PartialDate } from '@/core/dates'
import { refAllowed } from '@/core/participants'
import { freeChoiceValue, isFreeText, recordTitleById, tplOf, type Ctx, type FreeText } from '@/core/resolver'
import type { FieldDef, RecordDTO } from '@/core/types'
import { t } from '@/i18n'
import { useRecordMaybe } from '@/ui/record'

import PartialDateInput from '../PartialDateInput.vue'
import RecordPicker from '../RecordPicker.vue'
import KinIcon from '../KinIcon.vue'
import KinBool from './KinBool.vue'
import ImagePickDialog from './ImagePickDialog.vue'

const props = defineProps<{
  def: Pick<FieldDef, 'type' | 'options' | 'free'> & { name?: string; entities?: unknown }
  modelValue: unknown
  dashboard: string
  /** пока идёт сохранение: «да/нет» и список не принимают новый выбор (иначе он потеряется) */
  disabled?: boolean
}>()
/** pick — значение выбрано одним действием (вариант из списка, «да/нет»):
 * сохранять сразу, не дожидаясь ухода из поля; popup — открыт/закрыт список
 * в портале (фокус уходит из поля, но поле не покинуто); cancel — Esc в поле
 * комбобокса (список может перехватить его до ячейки) */
const emit = defineEmits<{ 'update:modelValue': [unknown]; submit: []; pick: []; popup: [boolean]; cancel: [] }>()

const s = useRecordMaybe()
const picking = ref(false)

// type email/url не используются: проверка браузера молча не даёт отправить
// «example.org»; адрес проверяет сервер, клавиатуру подбирает inputmode.
const inputType = computed(() => {
  switch (props.def.type) {
    case 'phone': return 'tel'
    case 'number': return 'number'
  }
  return 'text'
})
const inputMode = computed(() => {
  switch (props.def.type) {
    case 'email': return 'email'
    case 'url': return 'url'
  }
  return undefined
})

const text = computed(() => (props.modelValue == null ? '' : String(props.modelValue)))

function onText(e: Event) {
  const v = (e.target as HTMLInputElement).value
  if (props.def.type === 'number') emit('update:modelValue', v.trim() === '' ? null : Number(v))
  else emit('update:modelValue', v === '' ? null : v)
}

function onChoice(e: Event) {
  const v = (e.target as HTMLSelectElement).value
  emit('update:modelValue', v === '' ? null : v)
}

const comboOpen = ref(false)

function onComboOpen(v: boolean) {
  comboOpen.value = v
  emit('popup', v)
}

/** текст в поле комбобокса: название варианта или своё значение (F-104) */
function choiceText(v: unknown): string {
  if (v == null) return ''
  if (isFreeText(v)) return v.free
  return props.def.options?.find(o => o.key === v)?.name ?? String(v)
}

function onFreeInput(e: Event) {
  emit('update:modelValue', freeChoiceValue(props.def, (e.target as HTMLInputElement).value))
}

function onFreePick(v: unknown) {
  if (typeof v !== 'string') return
  emit('update:modelValue', v)
  emit('pick')
}

/** Enter при закрытом списке — как уход из поля; открытый список выбирает вариант сам */
function onFreeEnter(e: KeyboardEvent) {
  if (comboOpen.value) return
  e.preventDefault()
  emit('submit')
}

/** сущности, допустимые для ссылки, если поле их перечисляет */
const refEntities = computed(() =>
  Array.isArray(props.def.entities) ? props.def.entities.filter((x): x is string => typeof x === 'string') : undefined)

/** цель ссылки: сущности поля и `contains` шаблона записи (F-82) */
function refAccept(cand: RecordDTO, ctx: Ctx): boolean {
  const holder = s?.rec.value && s.ctx.value ? tplOf(s.ctx.value, s.rec.value) : undefined
  return refAllowed(ctx.reg, holder, refEntities.value, tplOf(ctx, cand))
}

function refTitle(id: unknown): string {
  return typeof id === 'string' && s?.ctx.value ? recordTitleById(s.ctx.value, id) : ''
}

/** диалог загрузки изображения (F-107) */
const imagePick = ref(false)

/** изображение: загрузить файл, значение — его sha256 (F-29) */
async function upload(file: File) {
  emit('update:modelValue', (await api.uploadFile(props.dashboard, file)).sha256)
}

function pick(id: string) {
  emit('update:modelValue', id)
  picking.value = false
}
</script>

<template>
  <PartialDateInput
    v-if="def.type === 'partial_date'"
    :model-value="(modelValue as PartialDate | null)"
    @update:model-value="v => emit('update:modelValue', v)"
  />
  <ComboboxRoot
    v-else-if="def.type === 'choice' && def.free"
    :open="comboOpen"
    class="combo"
    :disabled="disabled"
    :model-value="(modelValue as string | FreeText | null) ?? undefined"
    open-on-click
    @update:open="onComboOpen"
    @update:model-value="onFreePick"
  >
    <div class="combo-row">
      <ComboboxAnchor class="combo-anchor">
        <ComboboxInput
          class="kin-input"
          :display-value="choiceText"
          :aria-label="def.name"
          :placeholder="t.custom.freePlaceholder"
          @input="onFreeInput"
          @keydown.enter="onFreeEnter"
          @keydown.esc="emit('cancel')"
        />
      </ComboboxAnchor>
      <!-- кнопка — вне якоря: список равняется по ширине самого поля ввода -->
      <ComboboxTrigger
        class="kin-icon-btn combo-trigger"
        :aria-label="t.custom.freeOptions"
        :title="t.custom.freeOptions"
      >
        <KinIcon name="more" />
      </ComboboxTrigger>
    </div>
    <ComboboxPortal>
      <ComboboxContent
        class="kin-popover combo-list"
        position="popper"
        align="start"
        :side-offset="4"
      >
        <ComboboxViewport>
          <ComboboxItem
            v-for="o in def.options ?? []"
            :key="o.key"
            class="combo-item"
            :value="o.key"
            :text-value="o.name"
          >
            {{ o.name }}
          </ComboboxItem>
        </ComboboxViewport>
      </ComboboxContent>
    </ComboboxPortal>
  </ComboboxRoot>
  <select
    v-else-if="def.type === 'choice'"
    class="kin-input"
    :value="modelValue ?? ''"
    :aria-label="def.name"
    @change="onChoice"
  >
    <option value="">
      {{ t.common.empty }}
    </option>
    <option
      v-for="o in def.options ?? []"
      :key="o.key"
      :value="o.key"
    >
      {{ o.name }}
    </option>
  </select>
  <KinBool
    v-else-if="def.type === 'bool'"
    :model-value="typeof modelValue === 'boolean' ? modelValue : null"
    :aria-label="def.name"
    :disabled="disabled"
    @update:model-value="v => { emit('update:modelValue', v); emit('pick') }"
  />
  <div
    v-else-if="def.type === 'ref'"
    class="ref"
  >
    <div class="kin-row">
      <span>{{ refTitle(modelValue) || t.common.empty }}</span>
      <button
        class="kin-ibtn"
        type="button"
        :aria-label="t.card.refPick"
        :title="t.card.refPick"
        @click="picking = !picking"
      >
        <KinIcon name="search" />
      </button>
    </div>
    <RecordPicker
      v-if="picking && s?.reg.value"
      :dashboard="dashboard"
      :reg="s.reg.value"
      :entities="refEntities"
      :accept="refAccept"
      :exclude="s?.rec.value ? [s.rec.value.id] : []"
      @pick="pick"
    />
  </div>
  <div
    v-else-if="def.type === 'image'"
    class="kin-row"
  >
    <img
      v-if="typeof modelValue === 'string' && modelValue"
      class="thumb"
      :src="api.previewUrl(dashboard, modelValue, null, 'square', 64)"
      alt=""
    >
    <button
      class="kin-btn kin-btn--small upload"
      type="button"
      @click="imagePick = true"
    >
      <KinIcon name="upload" />
      {{ t.photo.upload }}
    </button>
    <ImagePickDialog
      v-model:open="imagePick"
      :upload="upload"
    />
  </div>
  <input
    v-else
    class="kin-input"
    :type="inputType"
    :inputmode="inputMode"
    :value="text"
    :aria-label="def.name"
    :step="def.type === 'number' ? 'any' : undefined"
    @input="onText"
    @keydown.enter.prevent="emit('submit')"
  >
</template>

<style scoped>
/* корень Reka не получает scoped-атрибут — раскладка строки на своём div */
.combo-row {
  display: flex;
  width: 100%;
  align-items: center;
  gap: var(--kin-space-1);
}

.combo-anchor {
  display: flex;
  flex: 1;
  min-width: 0;
}

.combo-anchor .kin-input {
  flex: 1;
  min-width: 0;
}

/* список — в портале: scoped-атрибут достаётся обёртке Reka, а не самому списку — нужен :deep */
/* ширина ровно по полю ввода (якорь — только поле), длинные варианты переносятся */
:deep(.combo-list) {
  box-sizing: border-box;
  width: var(--reka-combobox-trigger-width);
  min-width: var(--reka-combobox-trigger-width);
  max-width: var(--reka-combobox-trigger-width);
  padding: var(--kin-space-1);
  max-height: 16rem;
  overflow-y: auto;
}

.combo-item {
  padding: var(--kin-space-1) var(--kin-space-2);
  overflow-wrap: anywhere;
  border-radius: var(--kin-radius);
  cursor: pointer;
}

.combo-item[data-highlighted] {
  background: var(--kin-color-accent-soft);
}

.thumb {
  width: 3rem;
  height: 3rem;
  border-radius: var(--kin-radius);
}

.ref {
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-2);
}
</style>
