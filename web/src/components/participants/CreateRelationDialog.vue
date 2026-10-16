<script setup lang="ts">
// Создание отношения или события с участниками (F-10, F-42, Д-22): участники
// по ролям шаблона, этап и дата — одной пакетной операцией. Участника можно
// тут же создать быстрой формой (F-87) — он появится вместе со связью. Пока
// min ролей не выполнены, «Создать» недоступна (сервер тоже откажет).
import { DialogClose, DialogContent, DialogOverlay, DialogPortal, DialogRoot, DialogTitle } from 'reka-ui'
import { computed, ref, watch } from 'vue'

import { api } from '@/api/client'
import type { PartialDate } from '@/core/dates'
import { validateDate } from '@/core/dates'
import { canJoin, fitsRole, limits } from '@/core/participants'
import { periodLabel, type Registry } from '@/core/registry'
import { STAGE, type Ctx } from '@/core/resolver'
import type { NewRecordIn, RecordDTO, RoleDef, TemplateDef } from '@/core/types'
import { coreWords, errorText, fmt, t } from '@/i18n'

import PartialDateInput from '../PartialDateInput.vue'

import GroupPicker from '../GroupPicker.vue'
import RecordPicker from '../RecordPicker.vue'
import NewParticipantForm from './NewParticipantForm.vue'
import KinIcon from '../KinIcon.vue'

const props = defineProps<{
  dashboard: string
  reg: Registry
  template: TemplateDef | null
  /** запись, из карточки которой создают: сразу ставится в первую подходящую роль */
  preset?: { id: string; title: string; template: TemplateDef | undefined } | null
  /** группы новой записи по умолчанию и предлагаемые (F-04) */
  initialGroups?: string[]
  suggestGroups?: string[]
}>()
const open = defineModel<boolean>('open', { required: true })
const emit = defineEmits<{ created: [id: string] }>()

const NEW = 'new'

/** id — запись дашборда или временный id новой записи пакета (fresh) */
interface Pick { role: string; id: string; title: string; fresh?: NewRecordIn }

const picks = ref<Pick[]>([])
const stage = ref('')
const start = ref<PartialDate | null>(null)
const groups = ref<string[]>([])
const picking = ref('')
/** роль, для которой открыта форма нового участника (F-87) */
const creating = ref('')
const freshSeq = ref(0)
const error = ref('')
const busy = ref(false)

watch(open, v => {
  if (!v) return
  picks.value = []
  stage.value = ''
  start.value = null
  picking.value = ''
  creating.value = ''
  error.value = ''
  groups.value = [...(props.initialGroups ?? [])]
  const tpl = props.template
  const p = props.preset
  if (tpl && p) {
    const role = (tpl.roles ?? []).find(r => fitsRole(props.reg, r, p.template))
    if (role) picks.value.push({ role: role.key, id: p.id, title: p.title })
  }
}, { immediate: true })

const lim = computed(() => (props.template ? limits(props.template, picks.value.map(p => p.role)) : null))
const stages = computed(() => (props.template?.stages ?? []).filter(s => s.name))
const startLabel = computed(() => (props.template ? periodLabel(props.reg, props.template, 'start', coreWords(t)) : null))

function limitOf(role: string) {
  return lim.value?.roles.find(r => r.role.key === role)
}

function acceptFor(role: RoleDef) {
  return (cand: RecordDTO, ctx: Ctx) =>
    !!props.template && canJoin(ctx, props.template, role, cand)
    && !picks.value.some(p => p.role === role.key && p.id === cand.id)
}

function pick(role: string, id: string, title: string) {
  picks.value.push({ role, id, title })
  picking.value = ''
}

function addFresh(role: string, rec: NewRecordIn, title: string) {
  picks.value.push({ role, id: rec.ref, title, fresh: rec })
  creating.value = ''
}

function startCreate(role: string) {
  creating.value = creating.value === role ? '' : role
  picking.value = ''
  freshSeq.value++
}

function unpick(i: number) {
  picks.value.splice(i, 1)
}

const dateBad = computed(() => !!start.value && !!validateDate(start.value))
const canCreate = computed(() => !!lim.value?.complete && !dateBad.value && !busy.value)

async function create() {
  const tpl = props.template
  if (!tpl || !canCreate.value) return
  error.value = ''
  busy.value = true
  const rec: NewRecordIn = { ref: NEW, template: tpl.id }
  if (start.value) rec.start = { value: start.value }
  if (stage.value) rec.fields = { [STAGE]: [{ value: stage.value }] }
  if (groups.value.length) rec.groups = groups.value
  try {
    const out = await api.operations(props.dashboard, {
      records: [rec, ...picks.value.flatMap(p => (p.fresh ? [p.fresh] : []))],
      participants: picks.value.map(p => ({ container: NEW, entity: p.id, role: p.role })),
      // строго: неверное значение нового участника отменяет всё — пустых записей не бывает (F-88)
      strict: true,
    })
    open.value = false
    const id = out.ids[NEW]
    if (id) emit('created', id)
  } catch (e) {
    error.value = errorText(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <DialogRoot v-model:open="open">
    <DialogPortal>
      <DialogOverlay class="kin-overlay" />
      <DialogContent
        class="kin-dialog create-relation"
        :aria-describedby="undefined"
      >
        <DialogTitle>{{ template ? fmt(t.participants.createTitle, { name: template.name }) : '' }}</DialogTitle>
        <form
          v-if="template"
          class="kin-form"
          @submit.prevent="create"
        >
          <div
            v-for="role in template.roles ?? []"
            :key="role.key"
            class="role"
          >
            <div class="kin-row">
              <span class="kin-muted role-name">{{ role.forms.default }}</span>
              <span
                v-if="limitOf(role.key)?.missing"
                class="kin-error need"
              >{{ fmt(t.participants.need, { n: limitOf(role.key)!.missing }) }}</span>
              <button
                class="kin-ibtn kin-ibtn--sm add"
                type="button"
                :disabled="!limitOf(role.key)?.canAdd"
                :aria-label="t.participants.add"
                :title="limitOf(role.key)?.canAdd ? t.participants.add : t.participants.full"
                @click="picking = picking === role.key ? '' : role.key; creating = ''"
              >
                <KinIcon name="search" />
              </button>
              <button
                class="kin-ibtn kin-ibtn--sm new"
                type="button"
                :disabled="!limitOf(role.key)?.canAdd"
                :aria-label="t.participants.createNew"
                :title="t.participants.createNew"
                @click="startCreate(role.key)"
              >
                <KinIcon name="plus" />
              </button>
            </div>
            <ul class="picked">
              <li
                v-for="(p, i) in picks"
                v-show="p.role === role.key"
                :key="`${p.role}-${p.id}`"
                class="kin-row"
              >
                <span>{{ p.title }}</span>
                <button
                  class="kin-icon-btn"
                  type="button"
                  :aria-label="`${t.participants.remove}: ${p.title}`"
                  :title="`${t.participants.remove}: ${p.title}`"
                  @click="unpick(i)"
                >
                  <KinIcon name="close" />
                </button>
              </li>
            </ul>
            <NewParticipantForm
              v-if="creating === role.key"
              :reg="reg"
              :role="role"
              :dashboard="dashboard"
              :ref-id="`p${freshSeq}`"
              @add="(rec, title) => addFresh(role.key, rec, title)"
              @cancel="creating = ''"
            />
            <RecordPicker
              v-if="picking === role.key"
              :dashboard="dashboard"
              :reg="reg"
              :accept="acceptFor(role)"
              @pick="(id, title) => pick(role.key, id, title)"
            />
          </div>
          <p
            v-if="lim && lim.missingTotal"
            class="kin-error need"
          >
            {{ fmt(t.participants.needTotal, { n: lim.missingTotal }) }}
          </p>

          <label
            v-if="stages.length"
            class="kin-label"
          >
            {{ t.systemFields.stage }}
            <select
              v-model="stage"
              class="kin-input"
            >
              <option value="">{{ t.common.empty }}</option>
              <option
                v-for="st in stages"
                :key="st.key"
                :value="st.key"
              >{{ st.name }}</option>
            </select>
          </label>
          <PartialDateInput
            v-if="startLabel"
            v-model="start"
            :label="startLabel"
          />
          <GroupPicker
            v-model="groups"
            :dashboard="dashboard"
            :suggest="suggestGroups"
          />
          <p
            v-if="error"
            class="kin-error"
            role="alert"
          >
            {{ error }}
          </p>
          <div class="kin-actions">
            <DialogClose
              class="kin-ibtn"
              :aria-label="t.common.cancel"
              :title="t.common.cancel"
            >
              <KinIcon name="close" />
            </DialogClose>
            <button
              class="kin-ibtn kin-ibtn--primary submit"
              type="submit"
              :disabled="!canCreate"
              :aria-label="t.common.create"
              :title="t.common.create"
            >
              <KinIcon name="plus" />
            </button>
          </div>
        </form>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<style scoped>
.kin-form {
  max-width: none;
}

.role {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.role-name {
  flex: 1;
  min-width: 9rem;
  font-size: var(--kin-font-tiny);
  letter-spacing: 0.02em;
}

.need {
  font-size: var(--kin-font-tiny);
}

.picked {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.picked li {
  padding: 4px 4px 4px 12px;
  background: var(--kin-color-surface-sunken);
  border: 2px solid var(--kin-color-border-strong);
  border-radius: var(--kin-radius);
  font-weight: 600;
}

.picked li span {
  flex: 1;
}
</style>
