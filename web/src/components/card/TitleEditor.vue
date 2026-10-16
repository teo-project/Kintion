<script setup lang="ts">
// Ручная подпись записи (F-71, `freeedit`): пусто — подпись по правилу шаблона.
import { PopoverContent, PopoverPortal, PopoverRoot, PopoverTrigger } from 'reka-ui'
import { ref, watch } from 'vue'

import { api } from '@/api/client'
import { t } from '@/i18n'
import { useRecord } from '@/ui/record'
import KinIcon from '../KinIcon.vue'

const props = defineProps<{ label: string }>()

/** ключ версии ручной подписи (план §4) */
const KEY = 'title'

const s = useRecord()
const open = ref(false)
const draft = ref('')

watch(open, v => {
  if (v) draft.value = s.rec.value?.title_manual ?? ''
})

async function save() {
  const rec = s.rec.value
  if (!rec) return
  const v = draft.value.trim()
  const ok = await s.save(KEY, rev => api.patchRecord(rec.id, { if_rev: rev, title_manual: v || null }))
  if (ok || !s.fieldErrors[KEY]) open.value = false
}
</script>

<template>
  <PopoverRoot v-model:open="open">
    <PopoverTrigger
      class="kin-icon-btn kin-semi"
      :aria-label="t.card.titleEdit"
      :title="t.card.titleEdit"
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
          <label class="kin-label">
            {{ props.label }}
            <input
              v-model="draft"
              class="kin-input"
              :placeholder="t.card.titleAuto"
            >
          </label>
          <p
            v-if="s.fieldErrors[KEY]"
            class="kin-error"
            role="alert"
          >
            {{ s.fieldErrors[KEY] }}
          </p>
          <div class="kin-actions">
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
              :disabled="s.saving[KEY]"
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
