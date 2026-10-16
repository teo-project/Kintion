<script setup lang="ts">
// Меню (+) карточки (F-73, F-43): пункты `ui.quick_add` всех шаблонов
// дашборда для этой записи; в режиме разработки скрытые пункты видны
// недоступными с причиной. Пункт открывает быструю форму.
import { PopoverContent, PopoverPortal, PopoverRoot, PopoverTrigger } from 'reka-ui'
import { computed, ref, shallowRef } from 'vue'

import { quickAddOptions, type QuickAddOption } from '@/core/quickAdd'
import { t } from '@/i18n'
import { useRecord } from '@/ui/record'

import QuickAddForm from './QuickAddForm.vue'
import KinIcon from '../KinIcon.vue'

defineProps<{ dashboard: string }>()

const s = useRecord()
const dev = import.meta.env.DEV

const options = computed(() => {
  const ctx = s.ctx.value
  const rec = s.rec.value
  if (!ctx || !rec || rec.deleted) return []
  return quickAddOptions(ctx, rec).filter(o => o.visible || dev)
})

const open = ref(false)
const formOpen = ref(false)
const chosen = shallowRef<QuickAddOption | null>(null)

function choose(o: QuickAddOption) {
  chosen.value = o
  open.value = false
  formOpen.value = true
}
</script>

<template>
  <PopoverRoot
    v-if="options.some(o => o.visible) || (dev && options.length)"
    v-model:open="open"
  >
    <PopoverTrigger
      class="kin-ibtn kin-ibtn--primary quick-add"
      :aria-label="t.quickAdd.menu"
      :title="t.quickAdd.menu"
    >
      <KinIcon name="plus" />
    </PopoverTrigger>
    <PopoverPortal>
      <PopoverContent
        class="kin-popover"
        side="bottom"
        align="start"
        :side-offset="4"
      >
        <ul class="menu">
          <li
            v-for="o in options"
            :key="o.key"
          >
            <button
              class="kin-btn kin-btn--small item"
              type="button"
              :disabled="!o.visible"
              :title="o.visible ? o.template.name : o.reason"
              @click="choose(o)"
            >
              {{ o.item.name }}
            </button>
          </li>
        </ul>
      </PopoverContent>
    </PopoverPortal>
  </PopoverRoot>
  <QuickAddForm
    v-if="chosen"
    v-model:open="formOpen"
    :option="chosen"
    :dashboard="dashboard"
  />
</template>

<style scoped>
.menu {
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-1);
  margin: 0;
  padding: 0;
  list-style: none;
}

.item {
  width: 100%;
  justify-content: flex-start;
}
</style>
