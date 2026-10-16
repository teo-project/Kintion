<script setup lang="ts">
// Своё поле записи (F-61, F-75, F-96): добавить или изменить описание во
// всплывающей форме (F-69). Поле записи — всё описание custom_fields одним
// запросом с версией записи; общее поле шаблона («добавить всем записям
// этого шаблона») — список шаблона в дашборде с версией шаблона (F-45).
// Тип поля со значениями не меняется и такое поле не убирается — сервер тоже
// отказывает.
import { PopoverContent, PopoverPortal, PopoverRoot, PopoverTrigger } from 'reka-ui'
import { computed, reactive, ref } from 'vue'

import { api } from '@/api/client'
import { CUSTOM_PREFIX, FREE_META, SHARED_PREFIX } from '@/core/resolver'
import type { FieldDef } from '@/core/types'
import { fmt, t } from '@/i18n'
import { CUSTOM_KINDS, customDef, customKey, fromKind, kindOf, withCustom, type CustomDraft, type CustomKind } from '@/ui/customFields'
import { SHARED_REV, useRecord } from '@/ui/record'
import KinCheckbox from '../KinCheckbox.vue'
import KinIcon from '../KinIcon.vue'

const props = defineProps<{
  /** описание поля; нет — добавление нового */
  def?: FieldDef
  /** общее поле шаблона в дашборде (F-96) */
  shared?: boolean
}>()

const emit = defineEmits<{ added: [key: string] }>()

/** ключ версии описаний свободных полей */
const REV = 'custom_fields'

const s = useRecord()
const open = ref(false)
const draft = reactive<Required<CustomDraft>>({
  name: '', type: 'text', free: false, history: false, multiple: false, options: '', entity: '', cropName: t.photo.crop,
})
/** новое поле — всем записям шаблона (F-96), по умолчанию — только этой */
const toAll = ref(false)

const typeNames = t.custom.types as Record<string, string | undefined>
/** вид поля в списке типов: «выбор или своё» — choice с free (F-100) */
const kind = computed<CustomKind>({
  get: () => kindOf(draft),
  set: v => Object.assign(draft, fromKind(v)),
})
const prefix = computed(() => (props.shared ? SHARED_PREFIX : CUSTOM_PREFIX))
const values = computed(() => (props.def ? s.rec.value?.fields[prefix.value + props.def.key]?.values ?? [] : []))
const hasValues = computed(() => values.value.length > 0)
/** свои значения выбора — по пометке в meta (F-104) */
const hasFree = computed(() => values.value.some(v => v.meta?.[FREE_META] === true))
/**
 * Виды поля со значениями и общего поля (F-104): тип не меняется, кроме
 * «выбор» → «выбор или своё» (всегда) и обратно (пока своих значений нет;
 * у общего поля значения других записей проверяет сервер).
 */
const kinds = computed<CustomKind[]>(() => {
  if (!props.def || (!props.shared && !hasValues.value)) return CUSTOM_KINDS
  const k = kindOf(props.def)
  if (k === 'choice' || (k === 'choice_free' && !hasFree.value)) return ['choice', 'choice_free']
  return [k]
})
const typeHint = computed(() => {
  if (kinds.value.length > 1) return undefined
  if (props.def && kindOf(props.def) === 'choice_free' && hasFree.value) return t.custom.freeLocked
  return props.shared ? t.custom.sharedType : undefined
})
const tplDef = computed(() => {
  const row = s.rec.value?.template_id
  return row ? s.reg.value?.byRowId.get(row)?.definition : undefined
})
/** сущности установленных паков — ограничение ссылки (F-96) */
const entities = computed(() => [...(s.reg.value?.entities.values() ?? [])].map(e => ({ id: e.id, name: e.name ?? e.id })))
const sharedMode = computed(() => props.shared || (!props.def && toAll.value))
const errKey = computed(() => (sharedMode.value ? SHARED_REV : REV))
const sharedTitle = computed(() => fmt(t.custom.shared, { name: tplDef.value?.name ?? '' }))

function onOpen(v: boolean) {
  open.value = v
  if (!v) return
  s.fieldErrors[REV] = undefined
  s.fieldErrors[SHARED_REV] = undefined
  draft.name = props.def?.name ?? ''
  draft.type = props.def?.type ?? 'text'
  draft.free = !!props.def?.free
  draft.history = !!props.def?.history
  draft.multiple = !!props.def?.multiple
  draft.options = (props.def?.options ?? []).map(o => o.name).join('\n')
  draft.entity = props.def?.entities?.[0] ?? ''
  toAll.value = false
}

const canSave = computed(() => !!draft.name.trim() && (draft.type !== 'choice' || !!draft.options.trim()))

async function save(remove = false) {
  const rec = s.rec.value
  if (!rec || (!remove && !canSave.value)) return
  if (sharedMode.value) {
    const list = tplDef.value?.custom_fields ?? []
    const key = props.def?.key ?? customKey(draft.name, list.map(f => f.key))
    const ok = await s.saveShared(withCustom(list, key, remove ? null : customDef(key, draft, props.def)))
    if (!ok) return
    open.value = false
    if (!props.def) emit('added', SHARED_PREFIX + key)
    return
  }
  const list = rec.custom_fields
  const key = props.def?.key ?? customKey(draft.name, list.map(f => f.key))
  const next = withCustom(list, key, remove ? null : customDef(key, draft, props.def))
  const ok = await s.save(REV, rev => api.patchRecord(rec.id, { if_rev: rev, custom_fields: next }))
  if (!ok) return
  open.value = false
  if (!props.def) emit('added', CUSTOM_PREFIX + key)
}
</script>

<template>
  <PopoverRoot
    :open="open"
    @update:open="onOpen"
  >
    <PopoverTrigger
      v-if="def"
      class="kin-icon-btn kin-semi"
      :aria-label="fmt(t.custom.editTitle, { name: def.name })"
      :title="fmt(t.custom.editTitle, { name: def.name })"
    >
      <KinIcon name="settings" />
    </PopoverTrigger>
    <PopoverTrigger
      v-else
      class="kin-ibtn kin-ibtn--dashed add-custom"
      :aria-label="t.custom.add"
      :title="t.custom.add"
    >
      <KinIcon name="plus" />
    </PopoverTrigger>
    <PopoverPortal>
      <PopoverContent
        class="kin-popover custom-form"
        side="bottom"
        align="start"
        :side-offset="4"
      >
        <form
          class="form"
          @submit.prevent="save()"
        >
          <strong>{{ def ? fmt(t.custom.editTitle, { name: def.name }) : t.custom.addTitle }}</strong>
          <span
            v-if="shared"
            class="kin-muted"
          >{{ sharedTitle }}</span>
          <label class="kin-label">
            {{ t.custom.name }}
            <input
              v-model="draft.name"
              class="kin-input"
              name="custom-name"
              required
              maxlength="200"
            >
          </label>
          <label class="kin-label">
            {{ t.custom.type }}
            <select
              v-model="kind"
              class="kin-input"
              name="custom-type"
              :disabled="kinds.length < 2"
              :title="typeHint"
            >
              <option
                v-for="ty in kinds"
                :key="ty"
                :value="ty"
              >
                {{ typeNames[ty] ?? ty }}
              </option>
            </select>
          </label>
          <label
            v-if="draft.type === 'choice'"
            class="kin-label"
          >
            {{ t.custom.options }}
            <textarea
              v-model="draft.options"
              class="kin-input"
              name="custom-options"
              rows="4"
              required
            />
          </label>
          <label
            v-if="draft.type === 'ref'"
            class="kin-label"
          >
            {{ t.custom.entity }}
            <select
              v-model="draft.entity"
              class="kin-input"
              name="custom-entity"
            >
              <option value="">
                {{ t.custom.anyEntity }}
              </option>
              <option
                v-for="e in entities"
                :key="e.id"
                :value="e.id"
              >
                {{ e.name }}
              </option>
            </select>
          </label>
          <KinCheckbox
            v-model="draft.history"
            name="custom-history"
          >
            {{ t.custom.history }}
          </KinCheckbox>
          <KinCheckbox
            v-model="draft.multiple"
            name="custom-multiple"
          >
            {{ t.custom.multiple }}
          </KinCheckbox>
          <KinCheckbox
            v-if="!def"
            v-model="toAll"
            name="custom-to-all"
          >
            {{ t.custom.toAll }}
          </KinCheckbox>
          <p
            v-if="s.fieldErrors[errKey]"
            class="kin-error"
            role="alert"
          >
            {{ s.fieldErrors[errKey] }}
          </p>
          <div class="kin-actions">
            <button
              v-if="def"
              class="kin-ibtn kin-ibtn--danger"
              type="button"
              :disabled="s.saving[errKey] || hasValues"
              :title="hasValues ? t.custom.removeHint : t.custom.remove"
              :aria-label="t.custom.remove"
              @click="save(true)"
            >
              <KinIcon name="trash" />
            </button>
            <button
              class="kin-ibtn kin-ibtn--primary"
              type="submit"
              :disabled="s.saving[errKey] || !canSave"
              :aria-label="t.common.save"
              :title="t.common.save"
            >
              <KinIcon name="check" />
            </button>
          </div>
        </form>
      </PopoverContent>
    </PopoverPortal>
  </PopoverRoot>
</template>

<style scoped>
.form {
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-2);
  min-width: 16rem;
}
</style>
