<script setup lang="ts">
// История значения поля во всплывающей форме (F-11, F-69): значения в
// ручном порядке (верхнее — текущее), добавить сверху / снизу / между,
// переместить, «сделать основным», правка дат, пометки, привязок и meta.
// Каждое действие — отдельный запрос с версией поля (F-45).
import { PopoverContent, PopoverPortal, PopoverRoot, PopoverTrigger } from 'reka-ui'
import { computed, ref, watch } from 'vue'

import { api } from '@/api/client'
import type { PositionIn, RecordOut } from '@/api/types'
import type { ValueView } from '@/core/card'
import type { FieldDef, StageDef, ValueDTO } from '@/core/types'
import { t } from '@/i18n'
import { useRecord } from '@/ui/record'

import ValueForm, { type ValuePayload } from './ValueForm.vue'
import KinIcon from '../KinIcon.vue'

const props = defineProps<{
  /** ключ значения в API: поле, `custom.<ключ>` или этап */
  fieldKey: string
  label: string
  def: Pick<FieldDef, 'type' | 'options' | 'free' | 'meta' | 'multiple'> & { name?: string }
  views: ValueView[]
  dashboard: string
  /** этапы шаблона, если это поле этапа (F-37) */
  stages?: StageDef[]
}>()

const s = useRecord()
const open = ref(false)
/** что открыто в форме: id значения (правка) или место нового значения */
const form = ref<{ edit: string } | { add: PositionIn } | null>(null)

watch(open, v => {
  if (!v) form.value = null
})

const values = computed<ValueDTO[]>(() => s.rec.value?.fields[props.fieldKey]?.values ?? [])
const rows = computed(() => values.value.map((dto, i) => ({
  dto, i, view: props.views.find(v => v.id === dto.id),
})))
const current = computed(() => (props.def.multiple ? null : values.value[0]?.id ?? null))
const currentStage = computed(() => (props.stages && typeof values.value[0]?.value === 'string' ? values.value[0].value : null))
const busy = computed(() => !!s.saving[props.fieldKey])
const error = computed(() => s.fieldErrors[props.fieldKey])

async function run(fn: (id: string, rev: number) => Promise<RecordOut>): Promise<boolean> {
  const rec = s.rec.value
  if (!rec) return false
  const ok = await s.save(props.fieldKey, rev => fn(rec.id, rev))
  if (ok || !error.value) form.value = null
  return ok
}

function move(v: ValueDTO, pos: PositionIn) {
  void run((id, rev) => api.patchValue(id, props.fieldKey, v.id, { if_rev: rev, ...pos }))
}

function remove(v: ValueDTO) {
  void run((id, rev) => api.deleteValue(id, props.fieldKey, v.id, rev))
}

function save(p: ValuePayload) {
  const f = form.value
  if (!f) return
  if ('edit' in f) {
    void run((id, rev) => api.patchValue(id, props.fieldKey, f.edit, { if_rev: rev, ...p }))
  } else {
    void run((id, rev) => api.addValue(id, props.fieldKey, { if_rev: rev, ...p, ...f.add }))
  }
}

function period(v: ValueView | undefined): string {
  return v ? [v.from, v.to].filter(Boolean).join(' — ') : ''
}

const formKey = computed(() => (form.value ? JSON.stringify(form.value) : ''))
const editing = computed(() => {
  const f = form.value
  return f && 'edit' in f ? values.value.find(v => v.id === f.edit) ?? null : null
})
function addAt(pos: PositionIn) {
  form.value = { add: pos }
}
function isAddAt(pos: PositionIn): boolean {
  return !!form.value && 'add' in form.value && JSON.stringify(form.value.add) === JSON.stringify(pos)
}
</script>

<template>
  <PopoverRoot v-model:open="open">
    <PopoverTrigger
      class="kin-icon-btn kin-semi history-trigger"
      :aria-label="`${t.history.open}: ${label}`"
      :title="t.history.open"
    >
      <KinIcon name="history" />
    </PopoverTrigger>
    <PopoverPortal>
      <PopoverContent
        class="kin-popover history"
        side="bottom"
        align="start"
        :side-offset="4"
      >
        <h3 class="head">
          {{ label }}
        </h3>
        <div class="kin-row">
          <button
            class="kin-btn kin-btn--small add-top"
            type="button"
            @click="addAt({ position: 'top' })"
          >
            <KinIcon name="plus" />
            {{ t.history.addTop }}
          </button>
          <button
            class="kin-btn kin-btn--small add-bottom"
            type="button"
            @click="addAt({ position: 'bottom' })"
          >
            <KinIcon name="plus" />
            {{ t.history.addBottom }}
          </button>
        </div>
        <ValueForm
          v-if="isAddAt({ position: 'top' })"
          :key="formKey"
          :def="def"
          :initial="null"
          :dashboard="dashboard"
          :stages="stages"
          :current-stage="currentStage"
          :busy="busy"
          @save="save"
          @cancel="form = null"
        />
        <p
          v-if="rows.length === 0"
          class="kin-muted"
        >
          {{ t.history.empty }}
        </p>
        <ol class="list">
          <li
            v-for="r in rows"
            :key="r.dto.id"
            class="item kin-hover-row"
          >
            <ValueForm
              v-if="isAddAt({ before_id: r.dto.id })"
              :key="formKey"
              :def="def"
              :initial="null"
              :dashboard="dashboard"
              :stages="stages"
              :current-stage="currentStage"
              :busy="busy"
              @save="save"
              @cancel="form = null"
            />
            <ValueForm
              v-if="editing?.id === r.dto.id"
              :key="formKey"
              :def="def"
              :initial="r.dto"
              :dashboard="dashboard"
              :stages="stages"
              :current-stage="currentStage"
              :busy="busy"
              @save="save"
              @cancel="form = null"
            />
            <div
              v-else
              class="line"
            >
              <span class="text">{{ r.view?.text ?? String(r.dto.value) }}</span>
              <span
                v-if="r.dto.id === current"
                class="badge"
              >{{ t.history.current }}</span>
              <span
                v-if="period(r.view)"
                class="kin-muted small"
              >{{ period(r.view) }}</span>
              <span
                v-if="r.dto.note"
                class="kin-muted small"
              >· {{ r.dto.note }}</span>
              <span
                v-for="l in r.view?.links ?? []"
                :key="l.target + l.anchor"
                class="kin-muted small"
              >⇢ {{ l.title }} ({{ t.history.anchor[l.anchor as 'start' | 'end' | 'none'] ?? l.anchor }})</span>
              <span
                v-for="m in r.view?.meta ?? []"
                :key="m.key"
                class="kin-muted small"
              >{{ m.name }}: {{ typeof m.value === 'object' ? JSON.stringify(m.value) : m.value }}</span>
              <span class="actions">
                <button
                  class="kin-icon-btn kin-semi"
                  type="button"
                  :disabled="r.i === 0 || busy"
                  :aria-label="t.history.moveUp"
                  :title="t.history.moveUp"
                  @click="move(r.dto, { before_id: values[r.i - 1]!.id })"
                ><KinIcon name="up" /></button>
                <button
                  class="kin-icon-btn kin-semi"
                  type="button"
                  :disabled="r.i === rows.length - 1 || busy"
                  :aria-label="t.history.moveDown"
                  :title="t.history.moveDown"
                  @click="move(r.dto, { after_id: values[r.i + 1]!.id })"
                ><KinIcon name="down" /></button>
                <button
                  v-if="!def.multiple"
                  class="kin-icon-btn kin-semi primary"
                  type="button"
                  :disabled="r.i === 0 || busy"
                  :aria-label="t.history.makePrimary"
                  :title="t.history.makePrimary"
                  @click="move(r.dto, { position: 'top' })"
                ><KinIcon name="star" /></button>
                <button
                  class="kin-icon-btn kin-semi"
                  type="button"
                  :aria-label="t.history.addAbove"
                  :title="t.history.addAbove"
                  @click="addAt({ before_id: r.dto.id })"
                ><KinIcon name="plus" /></button>
                <button
                  class="kin-icon-btn kin-semi edit"
                  type="button"
                  :aria-label="t.common.edit"
                  :title="t.common.edit"
                  @click="form = { edit: r.dto.id }"
                ><KinIcon name="edit" /></button>
                <button
                  class="kin-icon-btn kin-semi kin-btn--danger"
                  type="button"
                  :disabled="busy"
                  :aria-label="t.common.delete"
                  :title="t.common.delete"
                  @click="remove(r.dto)"
                ><KinIcon name="close" /></button>
              </span>
            </div>
          </li>
        </ol>
        <ValueForm
          v-if="isAddAt({ position: 'bottom' })"
          :key="formKey"
          :def="def"
          :initial="null"
          :dashboard="dashboard"
          :stages="stages"
          :current-stage="currentStage"
          :busy="busy"
          @save="save"
          @cancel="form = null"
        />
        <p
          v-if="error"
          class="kin-error"
          role="alert"
        >
          {{ error }}
        </p>
      </PopoverContent>
    </PopoverPortal>
  </PopoverRoot>
</template>

<style scoped>
.history {
  width: min(32rem, 95vw);
}

.head {
  margin: 0 0 10px;
  font-size: 1rem;
  font-weight: 800;
}

.list {
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-1);
  margin: var(--kin-space-2) 0;
  padding: 0;
  list-style: none;
}

.line {
  display: flex;
  flex-wrap: wrap;
  gap: 4px var(--kin-space-2);
  align-items: center;
  padding: 6px 0;
  border-bottom: 1px solid var(--kin-color-border);
}

.text {
  font-weight: 700;
}

.badge {
  padding: 1px 8px;
  color: var(--kin-color-accent-text);
  background: var(--kin-color-accent-soft);
  border-radius: var(--kin-radius-pill);
  font-size: 0.6875rem;
  font-weight: 600;
}

.small {
  font-size: var(--kin-font-tiny);
}

.actions {
  display: inline-flex;
  gap: 2px;
  margin-left: auto;
}
</style>
