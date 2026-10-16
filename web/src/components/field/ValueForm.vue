<script setup lang="ts">
// Форма одного значения поля в истории: само значение, даты значения,
// пометка, привязки к записям-событиям (start / end / none) и редакторы meta
// по `editor` из шаблона (F-11, F-12, F-17, F-69). Для этапа — выбор из
// этапов шаблона, подсказка `next` и своё название (F-37, F-54).
import { computed, reactive, ref } from 'vue'

import type { LinkIn } from '@/api/types'
import { validateDate, type PartialDate } from '@/core/dates'
import { anchorEntities } from '@/core/participants'
import { FREE_META, recordTitleById, storedRaw, valueIn } from '@/core/resolver'
import type { Anchor, FieldDef, MetaDef, StageDef, ValueDTO } from '@/core/types'
import { t } from '@/i18n'
import { useRecord } from '@/ui/record'
import { isBlank, isInvalid, normalize } from '@/ui/values'

import { api } from '@/api/client'
import { CROP_EDITOR, cropShapes, type Crops } from '@/ui/crop'

import CropEditor from '../CropEditor.vue'
import PartialDateInput from '../PartialDateInput.vue'
import RecordPicker from '../RecordPicker.vue'

import ValueEditor from './ValueEditor.vue'
import KinIcon from '../KinIcon.vue'

export interface ValuePayload {
  value: unknown
  valid_from: PartialDate | null
  valid_to: PartialDate | null
  note: string | null
  meta: Record<string, unknown>
  links: LinkIn[]
}

const props = defineProps<{
  def: Pick<FieldDef, 'type' | 'options' | 'free' | 'meta'> & { name?: string }
  initial: ValueDTO | null
  dashboard: string
  /** этапы шаблона: поле — системный этап */
  stages?: StageDef[]
  /** текущий этап — для подсказки `next` */
  currentStage?: string | null
  busy?: boolean
}>()
const emit = defineEmits<{ save: [ValuePayload]; cancel: [] }>()

const s = useRecord()
const anchors: Anchor[] = ['start', 'end', 'none']
/** пункт «своё название» в выборе этапа (ключи этапов — идентификаторы, `*` среди них не бывает) */
const FREE = '*'

const draft = reactive({
  // своё значение выбора — { free } по пометке в meta (F-104)
  value: (props.initial ? storedRaw(props.initial) : null) as unknown,
  from: props.initial?.valid_from ?? null as PartialDate | null,
  to: props.initial?.valid_to ?? null as PartialDate | null,
  note: props.initial?.note ?? '',
  meta: { ...(props.initial?.meta ?? {}) } as Record<string, unknown>,
  links: (props.initial?.links ?? []).map(l => ({ ...l })) as LinkIn[],
})
const picking = ref(false)
const error = ref('')

const namedStages = computed(() => (props.stages ?? []).filter(x => x.name))
/** своё название этапа: значение не из набора (F-54) */
const free = ref(!!props.stages && typeof draft.value === 'string' && draft.value !== '' && !namedStages.value.some(x => x.key === draft.value))
const nextStages = computed(() => {
  const cur = props.stages?.find(x => x.key === props.currentStage)
  return (cur?.next ?? []).map(k => namedStages.value.find(x => x.key === k)).filter((x): x is StageDef => !!x)
})

/** редакторы meta: объявленные в поле или в выбранном этапе (F-12, F-37) */
const metaDefs = computed<MetaDef[]>(() => {
  const defs = props.stages ? props.stages.find(x => x.key === draft.value)?.meta : props.def.meta
  return (defs ?? []).filter(m => m.editor === 'color' || m.editor === 'text'
    || (m.editor === CROP_EDITOR && props.def.type === 'image' && typeof draft.value === 'string' && !!draft.value))
})

function palette(m: MetaDef): string[] {
  return Array.isArray(m.palette) ? m.palette.filter((c): c is string => typeof c === 'string') : []
}

function setMeta(key: string, v: unknown) {
  if (isBlank(v)) delete draft.meta[key]
  else draft.meta[key] = v
}

function linkTitle(id: string): string {
  return s.ctx.value ? recordTitleById(s.ctx.value, id) : id
}

function addLink(id: string) {
  draft.links.push({ target: id, anchor: 'none' })
  picking.value = false
}

function onStage(e: Event) {
  const v = (e.target as HTMLSelectElement).value
  free.value = v === FREE
  draft.value = free.value || v === '' ? null : v
}

function submit() {
  error.value = ''
  const bad = [draft.from, draft.to].some(d => d && validateDate(d)) || isInvalid(props.def, draft.value)
  if (bad) {
    error.value = t.card.dateError
    return
  }
  if (isBlank(draft.value)) return
  // пометка своего значения (F-104) — из значения, а не из прежней meta
  const rest = { ...draft.meta }
  delete rest[FREE_META]
  const vin = valueIn(normalize(draft.value), rest)
  emit('save', {
    value: vin.value,
    valid_from: draft.from,
    valid_to: draft.to,
    note: draft.note.trim() || null,
    meta: vin.meta ?? rest,
    links: draft.links,
  })
}
</script>

<template>
  <form
    class="value-form"
    @submit.prevent="submit"
  >
    <template v-if="stages">
      <select
        class="kin-input"
        :value="free ? FREE : (draft.value ?? '')"
        :aria-label="def.name"
        @change="onStage"
      >
        <option value="">
          {{ t.common.empty }}
        </option>
        <option
          v-for="st in namedStages"
          :key="st.key"
          :value="st.key"
        >
          {{ st.name }}
        </option>
        <option :value="FREE">
          {{ t.history.stageFree }}
        </option>
      </select>
      <input
        v-if="free"
        v-model="draft.value"
        class="kin-input"
        :aria-label="t.history.stageFree"
      >
      <p
        v-if="nextStages.length"
        class="kin-row kin-muted hint"
      >
        {{ t.history.stageNext }}:
        <button
          v-for="st in nextStages"
          :key="st.key"
          class="kin-btn kin-btn--small"
          type="button"
          @click="free = false; draft.value = st.key"
        >
          {{ st.name }}
        </button>
      </p>
    </template>
    <ValueEditor
      v-else
      v-model="draft.value"
      :def="def"
      :dashboard="dashboard"
    />

    <div class="kin-row dates">
      <PartialDateInput
        v-model="draft.from"
        :label="t.history.from"
      />
      <PartialDateInput
        v-model="draft.to"
        :label="t.history.to"
      />
    </div>

    <label class="kin-label">
      {{ t.history.note }}
      <input
        v-model="draft.note"
        class="kin-input"
      >
    </label>

    <div
      v-for="m in metaDefs"
      :key="m.key"
      class="kin-label"
    >
      {{ m.name }}
      <div
        v-if="m.editor === 'color'"
        class="kin-row"
        role="radiogroup"
        :aria-label="m.name"
      >
        <button
          v-for="c in palette(m)"
          :key="c"
          type="button"
          class="swatch"
          role="radio"
          :aria-checked="draft.meta[m.key] === c"
          :aria-label="c"
          :title="c"
          :style="{ background: c }"
          @click="setMeta(m.key, c)"
        />
        <button
          type="button"
          class="kin-btn kin-btn--small"
          role="radio"
          :aria-checked="draft.meta[m.key] == null"
          @click="setMeta(m.key, null)"
        >
          {{ t.history.noColor }}
        </button>
      </div>
      <CropEditor
        v-else-if="m.editor === CROP_EDITOR"
        :key="String(draft.value)"
        :model-value="(draft.meta[m.key] as Crops | undefined) ?? null"
        :src="api.fileUrl(dashboard, String(draft.value))"
        :shapes="cropShapes(m)"
        @update:model-value="v => setMeta(m.key, v)"
      />
      <input
        v-else
        class="kin-input"
        :value="draft.meta[m.key] ?? ''"
        @input="e => setMeta(m.key, (e.target as HTMLInputElement).value)"
      >
    </div>

    <div class="kin-label">
      {{ t.history.links }}
      <ul class="links">
        <li
          v-for="(l, i) in draft.links"
          :key="i"
          class="kin-row"
        >
          <span>{{ linkTitle(l.target) }}</span>
          <select
            v-model="l.anchor"
            class="kin-input"
            :aria-label="t.card.link"
          >
            <option
              v-for="a in anchors"
              :key="a"
              :value="a"
            >
              {{ t.history.anchor[a] }}
            </option>
          </select>
          <button
            class="kin-icon-btn"
            type="button"
            :aria-label="t.common.delete"
            :title="t.common.delete"
            @click="draft.links.splice(i, 1)"
          >
            <KinIcon name="close" />
          </button>
        </li>
      </ul>
      <div>
        <button
          class="kin-ibtn"
          type="button"
          :aria-label="t.history.addLink"
          :title="t.history.addLink"
          @click="picking = !picking"
        >
          <KinIcon name="link" />
        </button>
      </div>
      <RecordPicker
        v-if="picking && s.reg.value"
        :dashboard="dashboard"
        :reg="s.reg.value"
        :exclude="s.rec.value ? [s.rec.value.id] : []"
        :entities="anchorEntities(s.reg.value)"
        @pick="addLink"
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
        @click="emit('cancel')"
      >
        <KinIcon name="close" />
      </button>
      <button
        class="kin-ibtn kin-ibtn--primary"
        type="submit"
        :disabled="busy"
        :aria-label="t.common.save"
        :title="t.common.save"
      >
        <KinIcon name="check" />
      </button>
    </div>
  </form>
</template>

<style scoped>
.value-form {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin: 6px 0;
  padding: 12px;
  background: var(--kin-color-bg);
  border: 1px solid var(--kin-color-border);
  border-radius: 12px;
}

.dates {
  align-items: flex-start;
}

.links {
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-1);
  margin: 0;
  padding: 0;
  list-style: none;
}

.swatch {
  width: 1.5rem;
  height: 1.5rem;
  border: 2px solid var(--kin-color-border);
  border-radius: 50%;
  cursor: pointer;
}

.swatch[aria-checked='true'] {
  border-color: var(--kin-color-text);
}

.hint {
  margin: 0;
  font-size: var(--kin-font-tiny);
}
</style>
