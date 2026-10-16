<script setup lang="ts">
// Вопрос при правке поля с историей (F-80): заменить текущее значение или
// добавить новое в историю; галочка запоминает выбор на сервере, сбросить —
// в настройках аккаунта.
import { DialogContent, DialogDescription, DialogOverlay, DialogPortal, DialogRoot, DialogTitle } from 'reka-ui'
import { computed, ref, watch } from 'vue'

import { fmt, t } from '@/i18n'
import { useRecord, type HistoryChoice } from '@/ui/record'
import KinCheckbox from '../KinCheckbox.vue'
import KinIcon from '../KinIcon.vue'

const s = useRecord()
const remember = ref(false)
const open = computed(() => !!s.historyAsk.value)

watch(open, v => {
  if (v) remember.value = false
})

function choose(v: HistoryChoice | null) {
  void s.answerHistory(v, remember.value)
}
</script>

<template>
  <DialogRoot
    :open="open"
    @update:open="v => { if (!v) choose(null) }"
  >
    <DialogPortal>
      <DialogOverlay class="kin-overlay" />
      <DialogContent class="kin-dialog history-choice">
        <DialogTitle>{{ fmt(t.card.historyAsk.title, { field: s.historyAsk.value?.label ?? '' }) }}</DialogTitle>
        <DialogDescription class="kin-muted">
          {{ t.card.historyAsk.text }}
        </DialogDescription>
        <div class="kin-form">
          <KinCheckbox
            v-model="remember"
            name="history-remember"
          >
            {{ t.card.historyAsk.remember }}
          </KinCheckbox>
          <div class="kin-row">
            <button
              class="kin-ibtn"
              type="button"
              :aria-label="t.common.cancel"
              :title="t.common.cancel"
              @click="choose(null)"
            >
              <KinIcon name="close" />
            </button>
            <button
              class="kin-btn kin-btn--primary replace"
              type="button"
              @click="choose('replace')"
            >
              {{ t.card.historyAsk.replace }}
            </button>
            <button
              class="kin-btn add"
              type="button"
              @click="choose('add')"
            >
              {{ t.card.historyAsk.add }}
            </button>
          </div>
        </div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>
