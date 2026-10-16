<script setup lang="ts">
// Переключатель дашбордов (F-01) и создание нового.
import { DialogClose, DialogContent, DialogOverlay, DialogPortal, DialogRoot, DialogTitle, DialogTrigger } from 'reka-ui'
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { errorText, t } from '@/i18n'
import DashboardSettings from './DashboardSettings.vue'
import KinIcon from './KinIcon.vue'
import { useDashboardsStore } from '@/stores/dashboards'

const props = defineProps<{ current: string }>()

const dashboards = useDashboardsStore()
const router = useRouter()
const route = useRoute()
// прямой заход в «Группы», «Журнал», «Корзину» — список дашбордов ещё не загружен
const loadError = ref('')
function loadList() {
  loadError.value = ''
  dashboards.load().catch((e: unknown) => { loadError.value = errorText(e) })
}
loadList()
const open = ref(false)
const settingsOpen = ref(false)
const title = ref('')
const error = ref('')

function go(e: Event) {
  const id = (e.target as HTMLSelectElement).value
  if (id === props.current) return
  // тот же раздел и ракурс в другом дашборде (F-121); карточка записи — к спискам
  const name = route.name === 'groups' || route.name === 'audit' || route.name === 'trash' ? route.name : 'dashboard'
  const v = name === 'dashboard' && typeof route.query.v === 'string' ? { v: route.query.v } : undefined
  void router.push({ name, params: { d: id }, query: v })
}

async function create() {
  error.value = ''
  try {
    const d = await dashboards.create(title.value)
    open.value = false
    title.value = ''
    await router.push({ name: 'dashboard', params: { d: d.id } })
  } catch (e) {
    error.value = errorText(e)
  }
}
</script>

<template>
  <div class="switcher">
    <select
      class="kin-input"
      :value="current"
      :aria-label="t.dashboard.switcher"
      :title="t.dashboard.switcher"
      @change="go"
    >
      <option
        v-for="d in dashboards.list"
        :key="d.id"
        :value="d.id"
      >
        {{ d.name }}
      </option>
    </select>
    <button
      v-if="loadError"
      class="kin-ibtn load-error"
      type="button"
      :aria-label="`${loadError}. ${t.dashboard.switcherRetry}`"
      :title="`${loadError}. ${t.dashboard.switcherRetry}`"
      @click="loadList"
    >
      <KinIcon name="restore" />
    </button>
    <span
      v-if="loadError"
      class="load-error-text"
      role="alert"
    >{{ loadError }}</span>
    <button
      class="kin-ibtn settings-btn"
      type="button"
      :aria-label="t.dashboard.settings"
      :title="t.dashboard.settings"
      @click="settingsOpen = true"
    >
      <KinIcon name="settings" />
    </button>
    <DashboardSettings
      v-model:open="settingsOpen"
      :dashboard="current"
      :name="dashboards.list.find(x => x.id === current)?.name ?? ''"
    />
    <DialogRoot v-model:open="open">
      <DialogTrigger
        class="kin-ibtn new-dashboard"
        :aria-label="t.dashboard.newDashboard"
        :title="t.dashboard.newDashboard"
      >
        <KinIcon name="plus" />
      </DialogTrigger>
      <DialogPortal>
        <DialogOverlay class="kin-overlay" />
        <DialogContent
          class="kin-dialog"
          :aria-describedby="undefined"
        >
          <DialogTitle>{{ t.dashboard.newDashboard }}</DialogTitle>
          <form
            class="kin-form wide"
            @submit.prevent="create"
          >
            <label class="kin-label">
              {{ t.dashboard.newDashboardName }}
              <input
                v-model="title"
                class="kin-input"
                required
              >
            </label>
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
                class="kin-ibtn kin-ibtn--primary"
                type="submit"
                :aria-label="t.common.create"
                :title="t.common.create"
              >
                <KinIcon name="check" />
              </button>
            </div>
          </form>
        </DialogContent>
      </DialogPortal>
    </DialogRoot>
  </div>
</template>

<style scoped>
.switcher {
  display: flex;
  gap: var(--kin-space-2);
  align-items: center;
}

.switcher {
  min-width: 0;
}
.switcher select {
  min-width: 0;
  min-height: var(--kin-icon-btn);
  max-width: 14rem;
  color: var(--kin-color-text-muted);
  background-color: var(--kin-color-surface-raised);
  border-color: transparent;
}

.wide {
  max-width: none;
}
.load-error,
.load-error-text {
  color: var(--kin-color-danger-text);
}
.load-error-text {
  font-size: var(--kin-font-small);
  max-width: 14rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
