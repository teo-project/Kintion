<script setup lang="ts">
// Участие во всплывающей форме (F-69): период и пометка; убрать участника
// (ниже min — недоступно, Д-22).
import { PopoverContent, PopoverPortal, PopoverRoot, PopoverTrigger } from 'reka-ui'
import { ref, watch } from 'vue'

import { api } from '@/api/client'
import { validateDate, type PartialDate } from '@/core/dates'
import { participantsRev } from '@/core/participants'
import type { ParticipantDTO } from '@/core/types'
import { t } from '@/i18n'
import { useRecord } from '@/ui/record'

import PartialDateInput from '../PartialDateInput.vue'
import KinIcon from '../KinIcon.vue'

const props = defineProps<{ participant: ParticipantDTO; title: string; canRemove: boolean }>()

const s = useRecord()
const open = ref(false)
const from = ref<PartialDate | null>(null)
const to = ref<PartialDate | null>(null)
const note = ref('')
const error = ref('')

watch(open, v => {
  if (!v) return
  from.value = props.participant.valid_from
  to.value = props.participant.valid_to
  note.value = props.participant.note ?? ''
  error.value = ''
})

const key = () => participantsRev(props.participant.role)

async function save() {
  const rec = s.rec.value
  if (!rec) return
  if ([from.value, to.value].some(d => d && validateDate(d))) {
    error.value = t.card.dateError
    return
  }
  const ok = await s.save(key(), rev => api.patchParticipant(rec.id, props.participant.id, {
    if_rev: rev, valid_from: from.value, valid_to: to.value, note: note.value.trim() || null,
  }))
  if (ok || !s.fieldErrors[key()]) open.value = false
}

async function remove() {
  const rec = s.rec.value
  if (!rec) return
  const ok = await s.save(key(), rev => api.deleteParticipant(rec.id, props.participant.id, rev))
  if (ok || !s.fieldErrors[key()]) open.value = false
}
</script>

<template>
  <PopoverRoot v-model:open="open">
    <PopoverTrigger
      class="kin-icon-btn kin-semi"
      :aria-label="`${t.common.edit}: ${title}`"
      :title="`${t.common.edit}: ${title}`"
    >
      <KinIcon name="edit" />
    </PopoverTrigger>
    <PopoverPortal>
      <PopoverContent
        class="kin-popover"
        side="bottom"
        align="start"
        :side-offset="4"
      >
        <form
          class="kin-form"
          @submit.prevent="save"
        >
          <strong>{{ title }}</strong>
          <div class="kin-row dates">
            <PartialDateInput
              v-model="from"
              :label="t.history.from"
            />
            <PartialDateInput
              v-model="to"
              :label="t.history.to"
            />
          </div>
          <label class="kin-label">
            {{ t.history.note }}
            <input
              v-model="note"
              class="kin-input"
            >
          </label>
          <p
            v-if="error || s.fieldErrors[key()]"
            class="kin-error"
            role="alert"
          >
            {{ error || s.fieldErrors[key()] }}
          </p>
          <div class="kin-actions">
            <button
              class="kin-ibtn kin-ibtn--danger remove"
              type="button"
              :disabled="!canRemove || s.saving[key()]"
              :title="canRemove ? t.participants.remove : t.participants.atMin"
              :aria-label="t.participants.remove"
              @click="remove"
            >
              <KinIcon name="trash" />
            </button>
            <button
              class="kin-ibtn"
              type="button"
              :aria-label="t.common.cancel"
              :title="t.common.cancel"
              @click="open = false"
            >
              <KinIcon name="close" />
            </button>
            <button
              class="kin-ibtn kin-ibtn--primary"
              type="submit"
              :disabled="s.saving[key()]"
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
.dates {
  align-items: flex-start;
}
</style>
