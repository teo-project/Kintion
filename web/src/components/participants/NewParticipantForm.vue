<script setup lang="ts">
// Быстрое создание участника прямо в диалоге связи (F-87): поля —
// quick_fields шаблона по умолчанию сущности роли, как в быстром
// добавлении. Запись создаётся вместе со связью одной операцией; ничего
// не заполнено — добавить нельзя (пустых записей не бывает, F-88).
import { computed, reactive, ref, watch } from 'vue'

import type { PartialDate } from '@/core/dates'
import { validateDate } from '@/core/dates'
import { fitsRole } from '@/core/participants'
import { quickInputs, quickRecord } from '@/core/quickAdd'
import { defaultTemplate, type Registry } from '@/core/registry'
import { isDateLike } from '@/core/resolver'
import type { NewRecordIn, RoleDef } from '@/core/types'
import { coreWords, t } from '@/i18n'

import ValueEditor from '../field/ValueEditor.vue'
import PartialDateInput from '../PartialDateInput.vue'
import KinIcon from '../KinIcon.vue'

const props = defineProps<{ reg: Registry; role: RoleDef; dashboard: string; refId: string }>()
const emit = defineEmits<{ add: [rec: NewRecordIn, title: string]; cancel: [] }>()

/** сущности, записи которых могут стоять в роли */
const entities = computed(() => [...props.reg.entities.values()]
  .filter(e => fitsRole(props.reg, props.role, defaultTemplate(props.reg, e.id)))
  .map(e => ({ id: e.id, name: e.name ?? e.id })))
const entity = ref('')
const tpl = computed(() => (entity.value ? defaultTemplate(props.reg, entity.value) : undefined))
const inputs = computed(() => (tpl.value ? quickInputs(props.reg, coreWords(t), tpl.value) : []))
const values = reactive<Record<string, unknown>>({})
const error = ref('')

watch(entities, list => {
  if (!list.some(e => e.id === entity.value)) entity.value = list[0]?.id ?? ''
}, { immediate: true })
watch(entity, () => {
  for (const k of Object.keys(values)) delete values[k]
})

function add() {
  error.value = ''
  if (!tpl.value) return
  if (Object.values(values).some(v => isDateLike(v) && validateDate(v as PartialDate))) {
    error.value = t.card.dateError
    return
  }
  const rec = quickRecord(props.refId, tpl.value, inputs.value, values)
  if (!rec) {
    error.value = t.participants.newEmpty
    return
  }
  const title = inputs.value
    .filter(i => i.kind === 'field' && typeof values[i.addr] === 'string')
    .map(i => String(values[i.addr]).trim()).filter(Boolean).join(' ')
  emit('add', rec, title || tpl.value.name)
}

function onNote(addr: string, e: Event) {
  values[addr] = (e.target as HTMLInputElement).value
}
</script>

<template>
  <div class="new-participant kin-form">
    <label
      v-if="entities.length > 1"
      class="kin-label"
    >
      {{ t.quickAdd.entity }}
      <select
        v-model="entity"
        class="kin-input"
      >
        <option
          v-for="e in entities"
          :key="e.id"
          :value="e.id"
        >{{ e.name }}</option>
      </select>
    </label>
    <div
      v-for="inp in inputs"
      :key="inp.addr"
      class="kin-label"
    >
      <PartialDateInput
        v-if="inp.kind === 'start' || inp.kind === 'end'"
        :model-value="(values[inp.addr] as PartialDate | null) ?? null"
        :label="inp.label"
        @update:model-value="v => (values[inp.addr] = v)"
      />
      <template v-else>
        {{ inp.label }}
        <input
          v-if="inp.kind === 'note'"
          class="kin-input"
          :value="values[inp.addr] ?? ''"
          :aria-label="inp.label"
          @input="e => onNote(inp.addr, e)"
        >
        <ValueEditor
          v-else-if="inp.kind === 'field'"
          :model-value="values[inp.addr] ?? null"
          :def="{ ...inp.def, name: inp.label }"
          :dashboard="dashboard"
          @update:model-value="v => (values[inp.addr] = v)"
          @submit="add"
        />
      </template>
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
        class="kin-ibtn kin-ibtn--primary add-new"
        type="button"
        :aria-label="t.participants.addNew"
        :title="t.participants.addNew"
        @click="add"
      >
        <KinIcon name="check" />
      </button>
    </div>
  </div>
</template>

<style scoped>
.new-participant {
  max-width: none;
  padding: 12px;
  background: var(--kin-color-bg);
  border: 1px solid var(--kin-color-border);
  border-radius: 12px;
}
</style>
