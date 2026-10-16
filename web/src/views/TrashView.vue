<script setup lang="ts">
// Корзина (F-44): записи, удалённые мягко; восстановить или очистить
// окончательно (с подтверждением). Записи журнала после очистки остаются.
import {
  AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogOverlay,
  AlertDialogPortal, AlertDialogRoot, AlertDialogTitle,
} from 'reka-ui'
import { computed, ref, shallowRef, watch } from 'vue'
import { RouterLink } from 'vue-router'

import { api } from '@/api/client'
import type { RecordDTO, RelatedDTO } from '@/api/types'
import { photoOf } from '@/core/card'
import type { Registry } from '@/core/registry'
import { recordTitle, tplOf } from '@/core/resolver'
import RecordAvatar from '@/components/RecordAvatar.vue'
import { errorText, fmt, t } from '@/i18n'
import { rememberDashboard, useDashboardsStore } from '@/stores/dashboards'
import { makeCtx } from '@/ui/ctx'
import KinIcon from '../components/KinIcon.vue'

const props = defineProps<{ d: string }>()

const PAGE = 100

const dashboards = useDashboardsStore()
const reg = shallowRef<Registry | null>(null)
const items = shallowRef<RecordDTO[]>([])
const related = shallowRef<RelatedDTO>({})
const total = ref(0)
const error = ref('')
const loading = ref(false)
const busy = ref<string | null>(null)

const rows = computed(() => {
  if (!reg.value) return []
  const ctx = makeCtx(reg.value, items.value, related.value)
  return items.value.map(rec => ({
    id: rec.id,
    title: recordTitle(ctx, rec),
    template: tplOf(ctx, rec)?.name ?? '',
    photo: photoOf(ctx, rec),
  }))
})

/** первая страница или следующая (more) — корзина бывает большой */
async function load(more = false) {
  loading.value = true
  error.value = ''
  try {
    const d = props.d
    const [r, out] = await Promise.all([
      dashboards.registry(d),
      api.records(d, { deleted: true, limit: PAGE, offset: more ? items.value.length : 0 }),
    ])
    if (d !== props.d) return
    reg.value = r
    items.value = more ? [...items.value, ...out.items] : out.items
    related.value = more ? { ...related.value, ...out.related } : out.related
    total.value = out.total
  } catch (e) {
    error.value = errorText(e)
  } finally {
    loading.value = false
  }
}

watch(() => props.d, d => {
  rememberDashboard(d)
  void load()
}, { immediate: true })

async function act(id: string, fn: (id: string) => Promise<unknown>) {
  busy.value = id
  error.value = ''
  try {
    await fn(id)
    await load()
  } catch (e) {
    error.value = errorText(e)
  } finally {
    busy.value = null
  }
}

const purging = ref<string | null>(null)

function purge() {
  const id = purging.value
  purging.value = null
  if (id) void act(id, api.purgeRecord)
}
</script>

<template>
  <main class="page kin-page">
    <div class="kin-page-head">
      <RouterLink
        class="kin-ibtn kin-ibtn--ghost back"
        :to="{ name: 'dashboard', params: { d } }"
        :aria-label="t.card.toDashboard"
        :title="t.card.toDashboard"
      >
        <KinIcon name="back" />
      </RouterLink>
      <h1 class="kin-page-title">
        {{ t.trash.title }}
      </h1>
    </div>
    <p
      v-if="error"
      class="kin-notice kin-notice--error"
      role="alert"
    >
      {{ error }}
    </p>
    <section class="kin-island list-island">
      <p
        v-if="!loading && rows.length === 0 && !error"
        class="kin-muted empty"
      >
        {{ t.trash.empty }}
      </p>
      <ul class="list">
        <li
          v-for="r in rows"
          :key="r.id"
          class="row"
          data-test="trash-row"
        >
          <RecordAvatar
            :dashboard="d"
            :photo="r.photo"
            :title="r.title"
          />
          <RouterLink
            class="title"
            :to="{ name: 'record', params: { d, id: r.id } }"
          >
            {{ r.title }}
          </RouterLink>
          <span class="kin-muted template">{{ r.template }}</span>
          <span class="kin-spacer" />
          <button
            class="kin-ibtn"
            type="button"
            :disabled="busy === r.id"
            :aria-label="t.trash.restore"
            :title="t.trash.restore"
            @click="act(r.id, api.restoreRecord)"
          >
            <KinIcon name="restore" />
          </button>
          <button
            class="kin-ibtn kin-ibtn--danger"
            type="button"
            :disabled="busy === r.id"
            :aria-label="t.trash.purge"
            :title="t.trash.purge"
            @click="purging = r.id"
          >
            <KinIcon name="trash" />
          </button>
        </li>
      </ul>
    </section>
    <div
      v-if="total > rows.length"
      class="kin-row"
    >
      <span class="kin-muted">{{ fmt(t.dashboard.total, { n: total }) }}</span>
      <button
        class="kin-btn"
        type="button"
        :disabled="loading"
        @click="load(true)"
      >
        {{ t.dashboard.more }}
      </button>
    </div>

    <AlertDialogRoot
      :open="purging !== null"
      @update:open="v => { if (!v) purging = null }"
    >
      <AlertDialogPortal>
        <AlertDialogOverlay class="kin-overlay" />
        <AlertDialogContent class="kin-dialog">
          <AlertDialogTitle>{{ t.trash.purgeConfirm }}</AlertDialogTitle>
          <AlertDialogDescription class="kin-muted">
            {{ t.trash.purgeHint }}
          </AlertDialogDescription>
          <div class="kin-actions actions">
            <AlertDialogCancel
              class="kin-ibtn"
              :aria-label="t.common.cancel"
              :title="t.common.cancel"
            >
              <KinIcon name="close" />
            </AlertDialogCancel>
            <button
              class="kin-ibtn kin-ibtn--danger"
              type="button"
              :aria-label="t.trash.purge"
              :title="t.trash.purge"
              @click="purge"
            >
              <KinIcon name="trash" />
            </button>
          </div>
        </AlertDialogContent>
      </AlertDialogPortal>
    </AlertDialogRoot>
  </main>
</template>

<style scoped>
.page {
  max-width: 54rem;
}

.list-island {
  padding: 6px;
}

.empty {
  margin: 0;
  padding: var(--kin-space-4) var(--kin-space-3);
  text-align: center;
}

.list {
  margin: 0;
  padding: 0;
  list-style: none;
}

.row {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 12px;
  align-items: center;
  min-height: 48px;
  padding: 6px 10px;
  border-radius: 12px;
}

.row:hover {
  background: var(--kin-color-surface-raised);
}

.row :deep(.avatar) {
  width: 34px;
  height: 34px;
  font-size: var(--kin-font-tiny);
}

.title {
  color: var(--kin-color-text);
  font-weight: 700;
}

.template {
  font-size: var(--kin-font-tiny);
}

.actions {
  margin-top: var(--kin-space-3);
}
</style>
