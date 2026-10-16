<script setup lang="ts">
// Поле карточки: название и значения по типу (F-69: на карточке — только
// значения; у multiple — все, иначе — текущее). В режиме редактирования
// (F-85) строки, числа, места, контакты, даты, варианты и да/нет сразу
// открыты для ввода и сохраняются по одному при потере фокуса (F-86, F-45);
// ссылка — выбором записи, изображение — загрузкой. Режим «По месту»
// (F-108): у поля ручка правки (при наведении), двойной клик (по значению или
// названию: значение ссылки — само ссылка) или ручка
// открывают ввод только этого поля; Enter или уход из поля сохраняет, Esc
// отменяет — и поле снова в обычном виде.
import { computed, nextTick, reactive, ref, watch } from 'vue'

import { api } from '@/api/client'
import type { FieldView } from '@/core/card'
import { storedRaw, valueIn } from '@/core/resolver'
import type { ValueDTO } from '@/core/types'
import { t } from '@/i18n'
import { useRecord, type SaveResult } from '@/ui/record'
import { isBlank, isInvalid, normalize, sameValue } from '@/ui/values'

import EditCell from './EditCell.vue'
import FieldValue from './FieldValue.vue'
import HistoryPopover from './HistoryPopover.vue'
import KinIcon from '../KinIcon.vue'
import ImageField from './ImageField.vue'
import ValueEditor from './ValueEditor.vue'

const props = defineProps<{ field: FieldView; dashboard: string; startEditing?: boolean }>()

/** типы, которые правятся прямо в поле ввода (F-85) */
const INLINE = new Set(['text', 'number', 'place', 'contact', 'phone', 'email', 'url', 'partial_date', 'choice', 'bool'])

const s = useRecord()
const editing = ref(false)
/** «По месту» (F-108): это поле открыто для ввода */
const single = ref(false)
const row = ref<HTMLElement | null>(null)
const drafts = reactive<Record<string, unknown>>({})
const fresh = ref<unknown>(null)
const localError = ref('')

const raw = computed<ValueDTO[]>(() => s.rec.value?.fields[props.field.key]?.values ?? [])
const multiple = computed(() => !!props.field.def.multiple)
/** что правится на месте: все значения multiple или текущее (верхнее) */
const editable = computed(() => (multiple.value ? raw.value : raw.value.slice(0, 1)))
const shown = computed(() => {
  const ids = new Set(props.field.values.slice(0, multiple.value ? undefined : 1).map(v => v.id))
  return props.field.values.filter(v => ids.has(v.id)).map(v => ({ view: v, dto: raw.value.find(x => x.id === v.id) }))
})
/** режим редактирования карточки (или поле, открытое по месту) и поле ввода по типу */
const inline = computed(() => (s.editing.value || single.value) && INLINE.has(props.field.def.type))
/** ссылка правится формой с выбором записи — в режиме редактирования или по месту */
const canEdit = computed(() => (s.editing.value || s.inlineMode.value) && props.field.def.type === 'ref')
/** ручка правки по месту (F-108): поля ввода; ссылка — своей кнопкой, изображение — загрузкой */
const handle = computed(() => s.inlineMode.value && INLINE.has(props.field.def.type))
/** новое значение: у multiple — всегда можно добавить, иначе — пока значения нет */
const canAdd = computed(() => multiple.value || raw.value.length === 0)
const error = computed(() => localError.value || s.fieldErrors[props.field.key])

function open() {
  for (const k of Object.keys(drafts)) delete drafts[k]
  for (const v of editable.value) drafts[v.id] = storedRaw(v)
  fresh.value = null
  localError.value = ''
  editing.value = true
}

watch(() => props.startEditing, v => {
  if (v && canEdit.value) open()
  else if (v) void startSingle()
}, { immediate: true })

watch(() => s.editing.value, v => {
  if (!v) editing.value = false
})

watch(() => s.inlineMode.value, v => {
  if (!v) single.value = false
})

/** «По месту»: открыть ввод этого поля и поставить в него фокус */
async function startSingle() {
  if (!s.inlineMode.value) return
  if (canEdit.value) return open()
  if (!INLINE.has(props.field.def.type)) return
  single.value = true
  await nextTick()
  row.value?.querySelector<HTMLElement>('.cells input, .cells select, .cells textarea, .cells button')?.focus()
}

/** Уход из поля по месту: сохранено (или без изменений) и фокус ушёл из поля — обычный вид */
function onSettled(res: SaveResult) {
  if (!single.value || (res !== 'ok' && res !== 'cancel')) return
  setTimeout(() => {
    // фокус в другом вводе этого поля (multiple) — правка продолжается;
    // на ручке, истории и прочих кнопках строки — поле уже покинуто
    const active = document.activeElement
    const inCell = [...(row.value?.querySelectorAll('.cell') ?? [])].some(c => c.contains(active))
    if (!inCell) single.value = false
  })
}

function onEscape() {
  if (!single.value) return
  single.value = false
}

/** Значение поля ввода: изменить, очистить — удалить. Своё значение выбора — с пометкой (F-104). */
async function saveCell(v: ValueDTO | undefined, value: unknown): Promise<SaveResult> {
  const rec = s.rec.value
  if (!rec) return 'error'
  const key = props.field.key
  // черновик (F-88): первое непустое значение создаёт запись
  if (s.draft.value) {
    if (!isBlank(value)) return s.createWith(key, { fields: { [key]: [valueIn(value)] } })
    s.clearDraftValue(key) // очищено предзаполненное — с записью не уйдёт
    return 'ok'
  }
  if (!v) {
    if (isBlank(value)) return 'ok'
    return s.saveField(key, rev => api.addValue(rec.id, key, {
      if_rev: rev, ...valueIn(value), position: multiple.value ? 'bottom' : 'top',
    }))
  }
  if (isBlank(value)) return s.saveField(key, rev => api.deleteValue(rec.id, key, v.id, rev))
  // поле с историей и непустым значением: заменить или добавить новое (F-80)
  if (props.field.def.history && !multiple.value && !isBlank(v.value)) {
    const how = await s.historyChoice(props.field.name)
    if (!how) return 'cancel'
    if (how === 'add') return s.saveField(key, rev => api.addValue(rec.id, key, { if_rev: rev, ...valueIn(value), position: 'top' }))
  }
  return s.saveField(key, rev => api.patchValue(rec.id, key, v.id, { if_rev: rev, ...valueIn(value, v.meta) }))
}

/** Сохранить форму ссылки: правки значений, удаление очищенных, новое значение. */
async function save() {
  const rec = s.rec.value
  if (!rec) return
  const key = props.field.key
  const def = props.field.def
  localError.value = ''
  if ([...Object.values(drafts), fresh.value].some(v => isInvalid(def, v))) {
    localError.value = t.card.dateError
    return
  }
  for (const v of editable.value) {
    if (!(v.id in drafts) || sameValue(drafts[v.id], storedRaw(v))) continue
    const d = drafts[v.id]
    const ok = isBlank(d)
      ? await s.save(key, rev => api.deleteValue(rec.id, key, v.id, rev))
      : await s.save(key, rev => api.patchValue(rec.id, key, v.id, { if_rev: rev, ...valueIn(normalize(d), v.meta) }))
    if (!ok) return finish()
  }
  if (canAdd.value && !isBlank(fresh.value)) {
    const ok = await s.save(key, rev => api.addValue(rec.id, key, {
      if_rev: rev, ...valueIn(normalize(fresh.value)), position: multiple.value ? 'bottom' : 'top',
    }))
    if (!ok) return finish()
  }
  editing.value = false
}

/** Ошибка проверки — правка остаётся открытой; конфликт — поле уже показано текущим. */
function finish() {
  if (!s.fieldErrors[props.field.key]) editing.value = false
}

async function remove(v: ValueDTO) {
  const rec = s.rec.value
  if (!rec) return
  delete drafts[v.id]
  const ok = await s.save(props.field.key, rev => api.deleteValue(rec.id, props.field.key, v.id, rev))
  if (!ok) finish()
}
</script>

<template>
  <div
    ref="row"
    class="field-row kin-hover-row"
    :class="{ 'field-row--inline': single }"
  >
    <span
      class="kin-muted label"
      @dblclick="startSingle"
    >{{ field.name }}</span>
    <div
      v-if="inline"
      class="values cells"
    >
      <template v-if="multiple">
        <EditCell
          v-for="v in editable"
          :key="v.id"
          :def="{ ...field.def, name: field.name }"
          :value="storedRaw(v)"
          :dashboard="dashboard"
          :save="x => saveCell(v, x)"
          @settled="onSettled"
          @escape="onEscape"
        />
        <EditCell
          key="new"
          :def="{ ...field.def, name: field.name }"
          :value="null"
          :dashboard="dashboard"
          :save="x => saveCell(undefined, x)"
          fresh
          @settled="onSettled"
          @escape="onEscape"
        />
      </template>
      <EditCell
        v-else
        key="one"
        :def="{ ...field.def, name: field.name }"
        :value="editable[0] ? storedRaw(editable[0]) : null"
        :dashboard="dashboard"
        :save="x => saveCell(editable[0], x)"
        @settled="onSettled"
        @escape="onEscape"
      />
    </div>
    <form
      v-else-if="editing"
      class="values editing"
      @submit.prevent="save"
      @keydown.esc="editing = false"
    >
      <div
        v-for="v in editable"
        :key="v.id"
        class="kin-row"
      >
        <ValueEditor
          v-model="drafts[v.id]"
          :def="field.def"
          :dashboard="dashboard"
          @submit="save"
        />
        <button
          class="kin-icon-btn"
          type="button"
          :aria-label="t.common.delete"
          :title="t.common.delete"
          @click="remove(v)"
        >
          <KinIcon name="close" />
        </button>
      </div>
      <div
        v-if="canAdd"
        class="kin-row"
      >
        <ValueEditor
          v-model="fresh"
          :def="field.def"
          :dashboard="dashboard"
          @submit="save"
        />
      </div>
      <p
        v-if="error"
        class="kin-error"
        role="alert"
      >
        {{ error }}
      </p>
      <div class="kin-actions">
        <button
          class="kin-ibtn"
          type="button"
          :aria-label="t.common.cancel"
          :title="t.common.cancel"
          @click="editing = false"
        >
          <KinIcon name="close" />
        </button>
        <button
          class="kin-ibtn kin-ibtn--primary"
          type="submit"
          :disabled="s.saving[field.key]"
          :aria-label="t.common.save"
          :title="t.common.save"
        >
          <KinIcon name="check" />
        </button>
      </div>
    </form>
    <template v-else>
      <span
        class="values"
        @dblclick="startSingle"
      >
        <FieldValue
          v-for="v in shown"
          :key="v.view.id"
          :def="field.def"
          :value="v.dto ?? { id: v.view.id, value: null, valid_from: null, valid_to: null, note: null, meta: null, links: null }"
          :text="v.view.text"
          :dashboard="dashboard"
        />
        <span
          v-if="field.empty"
          class="kin-muted"
        >{{ t.common.empty }}</span>
      </span>
    </template>
    <!-- инструменты — общие для чтения и ввода: история не пропадает, когда
         поле по месту закрывается от клика по ней (F-108) -->
    <span
      v-if="!editing"
      class="tools"
    >
      <template v-if="!inline">
        <ImageField
          v-if="field.def.type === 'image' && (s.editing.value || s.inlineMode.value)"
          :field="field"
          :dashboard="dashboard"
        />
        <button
          v-if="handle"
          class="kin-icon-btn kin-semi handle"
          type="button"
          :aria-label="`${t.common.edit}: ${field.name}`"
          :title="t.common.edit"
          @click="startSingle"
        >
          <KinIcon name="edit" />
        </button>
        <button
          v-if="canEdit"
          class="kin-icon-btn kin-semi"
          type="button"
          :aria-label="`${t.common.edit}: ${field.name}`"
          :title="t.common.edit"
          @click="open"
        >
          <KinIcon name="edit" />
        </button>
      </template>
      <HistoryPopover
        v-if="!field.empty && !s.draft.value"
        :field-key="field.key"
        :label="field.name"
        :def="field.def"
        :views="field.values"
        :dashboard="dashboard"
      />
      <slot name="tools" />
    </span>
    <p
      v-if="error && !editing"
      class="kin-error full"
      role="alert"
    >
      {{ error }}
    </p>
  </div>
</template>

<style scoped>
.field-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kin-space-2) 12px;
  align-items: baseline;
  min-height: 30px;
  padding: 4px 0;
}

.label {
  flex: none;
  width: 130px;
  font-size: var(--kin-font-tiny);
}

.values {
  display: flex;
  flex: 1;
  flex-wrap: wrap;
  gap: var(--kin-space-1) var(--kin-space-3);
  min-width: 0;
  font-weight: 600;
}

.cells {
  align-items: center;
}

.cells > :deep(.cell) {
  flex: 1 1 14rem;
}

.cells > :deep(.cell > .kin-input) {
  width: 100%;
}

.editing {
  flex-direction: column;
}

.tools {
  display: inline-flex;
  gap: 2px;
  align-self: center;
}

@media (max-width: 560px) {
  .label {
    width: 100%;
  }
}


.full {
  flex-basis: 100%;
}
</style>
