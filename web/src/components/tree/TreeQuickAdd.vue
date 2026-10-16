<script setup lang="ts">
// Меню (+) узла дерева (F-73, F-101): те же пункты и быстрая форма, что в
// карточке; после создания остаёмся в дереве — оно перечитывается (close).
import { computed, ref, shallowRef, toRef, watch } from 'vue'

import QuickAddForm from '@/components/quickadd/QuickAddForm.vue'
import { quickAddOptions, type QuickAddOption } from '@/core/quickAdd'
import { t } from '@/i18n'
import { provideRecord, useRecordState } from '@/ui/record'

/** group — фильтр группы дерева (F-130, F-129) */
const props = defineProps<{ d: string; id: string; x: number; y: number; group?: string }>()
/** close — меню или форма закрыты: дерево перечитывается (после создания новая связь в нём; id — созданный) */
const emit = defineEmits<{ close: [id?: string] }>()

const s = useRecordState(toRef(props, 'd'), toRef(props, 'id'))
provideRecord(s)
void s.load()

const options = computed(() => {
  const ctx = s.ctx.value
  const rec = s.rec.value
  if (!ctx || !rec || rec.deleted) return []
  return quickAddOptions(ctx, rec).filter(o => o.visible)
})

const chosen = shallowRef<QuickAddOption | null>(null)
const formOpen = ref(false)
const created = ref<string | undefined>()

function choose(o: QuickAddOption) {
  chosen.value = o
  formOpen.value = true
}

watch(formOpen, v => {
  if (!v) emit('close', created.value)
})
</script>

<template>
  <ul
    v-if="!chosen"
    class="tree-menu kin-popover"
    role="menu"
    :aria-label="t.quickAdd.menu"
    :style="{ left: `${x}px`, top: `${y}px` }"
  >
    <li
      v-for="o in options"
      :key="o.key"
    >
      <button
        class="kin-btn kin-btn--small item"
        type="button"
        role="menuitem"
        @click="choose(o)"
      >
        {{ o.item.name }}
      </button>
    </li>
    <li
      v-if="s.rec.value && !options.length"
      class="kin-muted empty"
    >
      —
    </li>
  </ul>
  <QuickAddForm
    v-if="chosen"
    v-model:open="formOpen"
    :option="chosen"
    :dashboard="d"
    :tree-group="group"
    @created="id => (created = id)"
  />
</template>

<style scoped>
.tree-menu {
  position: absolute;
  z-index: 20;
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-1);
  margin: 0;
  list-style: none;
}

.item {
  width: 100%;
  justify-content: flex-start;
}

.empty {
  padding: 4px 8px;
}
</style>
