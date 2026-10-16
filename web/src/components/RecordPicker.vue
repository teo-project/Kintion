<script setup lang="ts">
// Выбор записи дашборда поиском (по видимым, F-07): цель ссылки `ref`,
// привязка даты или значения к записи сущности с `anchor` (F-83).
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'

import { api } from '@/api/client'
import type { RecordDTO, RelatedDTO } from '@/api/types'
import type { Registry } from '@/core/registry'
import { recordTitle, tplOf, type Ctx } from '@/core/resolver'
import { errorText, t } from '@/i18n'
import { makeCtx } from '@/ui/ctx'

const props = defineProps<{
  dashboard: string
  reg: Registry
  /** не предлагать эти записи (саму карточку) */
  exclude?: string[]
  /** только эти сущности; нет — любые */
  entities?: string[]
  /** дополнительный отбор (роль, вложенность, события окончания) */
  accept?: (rec: RecordDTO, ctx: Ctx) => boolean
}>()
const emit = defineEmits<{ pick: [id: string, title: string] }>()

const q = ref('')
const items = shallowRef<RecordDTO[]>([])
const related = shallowRef<RelatedDTO>({})
const error = ref('')

const rows = computed(() => {
  const ctx = makeCtx(props.reg, items.value, related.value)
  return items.value
    .filter(r => !props.exclude?.includes(r.id))
    .filter(r => !props.entities?.length || props.entities.includes(r.entity))
    .filter(r => !props.accept || props.accept(r, ctx))
    .map(r => ({ id: r.id, title: recordTitle(ctx, r), template: tplOf(ctx, r)?.name ?? '' }))
})

let seq = 0
async function search() {
  const my = ++seq
  error.value = ''
  try {
    const entity = props.entities?.length === 1 ? props.entities[0] : undefined
    const limit = props.accept ? 50 : 20
    const out = await api.records(props.dashboard, { q: q.value.trim() || undefined, entity, limit })
    if (my !== seq) return
    items.value = out.items
    related.value = out.related
  } catch (e) {
    if (my === seq) error.value = errorText(e)
  }
}

let timer: ReturnType<typeof setTimeout> | undefined
watch(q, () => {
  clearTimeout(timer)
  timer = setTimeout(() => void search(), 250)
})
onBeforeUnmount(() => clearTimeout(timer))
void search()
</script>

<template>
  <div class="picker">
    <input
      v-model="q"
      class="kin-input"
      type="search"
      :placeholder="t.card.refPick"
      :aria-label="t.card.refPick"
    >
    <p
      v-if="error"
      class="kin-error"
    >
      {{ error }}
    </p>
    <ul class="results">
      <li
        v-for="r in rows"
        :key="r.id"
      >
        <button
          class="kin-btn kin-btn--small result"
          type="button"
          @click="emit('pick', r.id, r.title)"
        >
          {{ r.title }} <span class="kin-muted">{{ r.template }}</span>
        </button>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.picker {
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-2);
}

.results {
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-1);
  max-height: 12rem;
  margin: 0;
  padding: 0;
  overflow: auto;
  list-style: none;
}

.result {
  width: 100%;
  justify-content: space-between;
}
</style>
