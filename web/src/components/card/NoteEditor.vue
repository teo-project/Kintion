<script setup lang="ts">
// Примечание записи (F-17): в режиме чтения — текст (пустое не показывается),
// в режиме редактирования (F-85) — поле ввода, сохранение при потере фокуса
// (F-86, F-45).
import { api } from '@/api/client'
import { NOTE } from '@/core/resolver'
import { useRecord, type SaveResult } from '@/ui/record'

import EditCell from '../field/EditCell.vue'

defineProps<{ label: string; dashboard: string }>()

const s = useRecord()

function save(v: unknown): Promise<SaveResult> {
  const rec = s.rec.value
  if (!rec) return Promise.resolve('error')
  const text = typeof v === 'string' ? v.trim() : ''
  if (s.draft.value) {
    if (text) return s.createWith(NOTE, { note: text })
    s.clearDraftValue(NOTE)
    return Promise.resolve('ok')
  }
  return s.saveField(NOTE, rev => api.patchRecord(rec.id, { if_rev: rev, note: text || null }))
}
</script>

<template>
  <div class="note">
    <label
      v-if="s.editing.value"
      class="kin-label wide"
    >
      {{ label }}
      <EditCell
        :def="{ type: 'text', name: label }"
        :value="s.rec.value?.note ?? null"
        :dashboard="dashboard"
        :save="save"
        multiline
      />
    </label>
    <p
      v-else-if="s.rec.value?.note"
      class="text"
    >
      {{ s.rec.value.note }}
    </p>
    <p
      v-if="s.fieldErrors[NOTE]"
      class="kin-error"
      role="alert"
    >
      {{ s.fieldErrors[NOTE] }}
    </p>
  </div>
</template>

<style scoped>
.note:empty {
  display: none;
}

.text {
  margin: 0;
  color: var(--kin-color-text-muted);
  white-space: pre-wrap;
}

.wide {
  max-width: 36rem;
}

.wide :deep(textarea) {
  width: 100%;
}
</style>
