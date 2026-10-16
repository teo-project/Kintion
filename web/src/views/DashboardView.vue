<script setup lang="ts">
// Экран дашборда: вкладки по сущностям реестра, поиск, фильтр по группе,
// список с подписями резолвера и «Создать» с выбором шаблона (F-62).
import {
  DialogClose, DialogContent, DialogOverlay, DialogPortal, DialogRoot, DialogTitle, TabsList, TabsRoot, TabsTrigger,
} from 'reka-ui'
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { api } from '@/api/client'
import type { GroupDTO, RecordDTO, RelatedDTO } from '@/api/types'
import GroupPicker from '@/components/GroupPicker.vue'
import KinIcon from '@/components/KinIcon.vue'
import CreateRelationDialog from '@/components/participants/CreateRelationDialog.vue'
import RecordList from '@/components/RecordList.vue'
import TreeView from '@/components/tree/TreeView.vue'
import TreeSelection from '@/components/tree/TreeSelection.vue'
import type { Registry } from '@/core/registry'
import type { SchemaTemplateDTO, TemplateDef } from '@/core/types'
import { errorText, fmt, t } from '@/i18n'
import { rememberDashboard, useDashboardsStore } from '@/stores/dashboards'
import { makeCtx } from '@/ui/ctx'
import { newDraft } from '@/ui/drafts'

const props = defineProps<{ d: string }>()

const PAGE = 50

const route = useRoute()
const router = useRouter()
const dashboards = useDashboardsStore()

const reg = shallowRef<Registry | null>(null)
// реестр правят и другие экраны (настройки дашборда: быстрая форма) — видно сразу
watch(() => dashboards.current(props.d), r => {
  if (r && reg.value && r !== reg.value) reg.value = r
})
const groups = ref<GroupDTO[]>([])
const items = shallowRef<RecordDTO[]>([])
const related = shallowRef<RelatedDTO>({})
const total = ref(0)
/** режим выделения в списках (F-131) */
const selected = ref<Set<string>>(new Set())
const selectedIds = computed(() => [...selected.value])
function toggleSel(id: string, on: boolean) {
  const next = new Set(selected.value)
  if (on) next.add(id)
  else next.delete(id)
  selected.value = next
}
const error = ref('')
const loading = ref(false)
const search = ref(typeof route.query.q === 'string' ? route.query.q : '')

const entities = computed(() => (reg.value ? [...reg.value.entities.values()] : []))
const entity = computed(() => {
  const e = route.query.e
  return typeof e === 'string' && reg.value?.entities.has(e) ? e : (entities.value[0]?.id ?? '')
})
/** ракурс: списки, дерево (F-110) или граф — заготовка (F-121); выбирается в верхнем меню */
const view = computed(() => (route.query.v === 'tree' || route.query.v === 'graph' ? route.query.v : 'list'))
const group = computed(() => (typeof route.query.g === 'string' ? route.query.g : ''))
/** человек в центре дерева — из иконки дерева в «Связях» (F-129) */
const focus = computed(() => (typeof route.query.focus === 'string' ? route.query.focus : ''))
const query = computed(() => (typeof route.query.q === 'string' ? route.query.q : ''))
const ctx = computed(() => (reg.value ? makeCtx(reg.value, items.value, related.value) : null))

/** Шаблоны текущей сущности: по умолчанию — первым (F-62). */
const templates = computed<SchemaTemplateDTO[]>(() => {
  const list = reg.value ? [...reg.value.templates.values()].filter(x => x.entity === entity.value) : []
  return list.sort((a, b) => Number(b.default) - Number(a.default))
})

/** Запись-контейнер (отношение, событие) создаётся диалогом с участниками; прочие — сразу. */
function isContainer(tpl: TemplateDef): boolean {
  return !!reg.value?.entities.get(tpl.entity)?.container
}
const relOpen = ref(false)
const relTemplate = shallowRef<TemplateDef | null>(null)

function setQuery(patch: Record<string, string>) {
  const next: Record<string, string> = {}
  for (const [k, v] of Object.entries({ ...route.query, ...patch })) if (typeof v === 'string' && v) next[k] = v
  void router.replace({ query: next })
}

let seq = 0
async function fetchList(append = false, keep = false) {
  if (!reg.value || !entity.value) return
  const my = ++seq
  loading.value = true
  error.value = ''
  try {
    const out = await api.records(props.d, {
      entity: entity.value, q: query.value || undefined, group: group.value || undefined,
      limit: keep ? Math.min(Math.max(items.value.length, PAGE), 200) : PAGE, offset: append ? items.value.length : 0,
    })
    if (my !== seq) return
    items.value = append ? [...items.value, ...out.items] : out.items
    related.value = append ? { ...related.value, ...out.related } : out.related
    total.value = out.total
    if (!append && selected.value.size) {
      const shown = new Set(items.value.map(i => i.id))
      selected.value = new Set([...selected.value].filter(id => shown.has(id)))
    }
  } catch (e) {
    if (my === seq) error.value = errorText(e)
  } finally {
    if (my === seq) loading.value = false
  }
}

async function openDashboard(d: string) {
  seq++ // ответы списка прежнего дашборда больше не применяются
  selected.value = new Set()
  reg.value = null
  items.value = []
  related.value = {}
  error.value = ''
  rememberDashboard(d)
  try {
    const [r, g] = await Promise.all([dashboards.registry(d), api.groups(d), dashboards.load()])
    if (d !== props.d) return
    reg.value = dashboards.current(d) ?? r
    groups.value = g
    if (view.value === 'list') await fetchList()
  } catch (e) {
    error.value = errorText(e)
  }
}
watch(() => props.d, openDashboard, { immediate: true })

/** создана группа в дереве (F-131) — список фильтра перечитать */
async function reloadGroups() {
  const d = props.d
  try {
    const g = await api.groups(d)
    if (d === props.d) groups.value = g
  } catch {
    // список останется прежним до следующего открытия
  }
}
// список дашбордов загрузился после ошибки (повтор из переключателя) — открыть заново
watch(() => dashboards.loaded, l => {
  if (l && error.value && !reg.value) void openDashboard(props.d)
})

watch([entity, group, query], () => {
  selected.value = new Set()
  items.value = [] // прежняя выдача не должна оставаться выделяемой
  if (view.value === 'list') void fetchList()
})
watch(view, v => {
  if (v === 'list') void fetchList()
})

let timer: ReturnType<typeof setTimeout> | undefined
watch(search, v => {
  clearTimeout(timer)
  timer = setTimeout(() => setQuery({ q: v.trim() }), 300)
})
onBeforeUnmount(() => clearTimeout(timer))

const createOpen = ref(false)
const createError = ref('')
/** группы новой записи (F-04): по умолчанию — группа текущего фильтра */
const createGroups = ref<string[]>([])

async function create(tpl: SchemaTemplateDTO) {
  createError.value = ''
  if (isContainer(tpl.definition)) {
    createOpen.value = false
    relTemplate.value = tpl.definition
    relOpen.value = true
    return
  }
  // черновик (F-88): запись появится с первым заполненным полем
  const id = newDraft({ dashboard: props.d, template: tpl.key, groups: [...createGroups.value] })
  createOpen.value = false
  await router.push({ name: 'record', params: { d: props.d, id } })
}

function onCreate() {
  createGroups.value = group.value ? [group.value] : []
  const only = templates.value.length === 1 && groups.value.length === 0 ? templates.value[0] : undefined
  if (only) void create(only)
  else createOpen.value = true
}

function openRecord(id: string) {
  void router.push({ name: 'record', params: { d: props.d, id } })
}

/** название текущего дашборда — заголовок страницы */
const title = computed(() => dashboards.list.find(x => x.id === props.d)?.name ?? '')

function onGroup(e: Event) {
  setQuery({ g: (e.target as HTMLSelectElement).value })
}
</script>

<template>
  <main
    v-if="view === 'tree' && reg"
    class="tree-screen"
  >
    <TreeView
      :d="d"
      :reg="reg"
      :group="group"
      :focus="focus"
      @groups="reloadGroups"
    >
      <select
        class="kin-input group-filter"
        :value="group"
        :aria-label="t.dashboard.group"
        :title="t.dashboard.group"
        @change="onGroup"
      >
        <option value="">
          {{ t.dashboard.allGroups }}
        </option>
        <option
          v-for="g in groups"
          :key="g.id"
          :value="g.id"
        >
          {{ g.name }}
        </option>
      </select>
    </TreeView>
  </main>
  <main
    v-else-if="view === 'graph'"
    class="tree-screen graph-soon"
  >
    <p
      v-if="error"
      class="kin-notice kin-notice--error"
      role="alert"
    >
      {{ error }}
    </p>
    <p
      v-else
      class="kin-muted"
    >
      {{ t.nav.graphSoon }}
    </p>
  </main>
  <main
    v-else
    class="dashboard kin-page"
  >
    <div class="kin-page-head">
      <h1 class="kin-page-title">
        {{ title || t.nav.dashboard }}
      </h1>
      <span class="kin-spacer" />
      <button
        v-if="reg"
        class="kin-ibtn kin-ibtn--primary create"
        type="button"
        :disabled="templates.length === 0"
        :aria-label="t.common.create"
        :title="t.common.create"
        @click="onCreate"
      >
        <KinIcon name="plus" />
      </button>
    </div>
    <p
      v-if="error"
      class="kin-notice kin-notice--error"
      role="alert"
    >
      {{ error }}
    </p>
    <template v-if="reg">
      <div class="toolbar kin-island">
        <TabsRoot
          v-if="view === 'list'"
          :model-value="entity"
          @update:model-value="v => setQuery({ e: String(v) })"
        >
          <TabsList class="kin-tabs">
            <TabsTrigger
              v-for="e in entities"
              :key="e.id"
              class="kin-tab"
              :value="e.id"
            >
              {{ e.name ?? e.id }}
            </TabsTrigger>
          </TabsList>
        </TabsRoot>
        <label
          v-if="view === 'list'"
          class="search-box"
        >
          <KinIcon name="search" />
          <input
            v-model="search"
            class="kin-input search"
            type="search"
            :placeholder="t.dashboard.searchPlaceholder"
            :aria-label="t.common.search"
          >
        </label>
        <select
          class="kin-input group-filter"
          :value="group"
          :aria-label="t.dashboard.group"
          :title="t.dashboard.group"
          @change="onGroup"
        >
          <option value="">
            {{ t.dashboard.allGroups }}
          </option>
          <option
            v-for="g in groups"
            :key="g.id"
            :value="g.id"
          >
            {{ g.name }}
          </option>
        </select>
      </div>
      <div class="kin-island list-island">
        <RecordList
          v-if="ctx"
          :dashboard="d"
          :items="items"
          :ctx="ctx"
          :groups="groups"
          selectable
          :selected="selected"
          :empty-text="loading ? t.common.loading : (query || group ? t.dashboard.nothingFound : t.dashboard.empty)"
          @toggle="toggleSel"
        />
        <TreeSelection
          v-if="selected.size"
          class="list-selection"
          :d="d"
          :ids="selectedIds"
          @clear="selected = new Set()"
          @changed="fetchList(false, true)"
          @groups="reloadGroups"
        />
      </div>
      <div
        v-if="view === 'list' && items.length > 0"
        class="kin-row footer"
      >
        <span class="kin-muted">{{ fmt(t.dashboard.total, { n: total }) }}</span>
        <button
          v-if="items.length < total"
          class="kin-btn"
          type="button"
          :disabled="loading"
          @click="fetchList(true)"
        >
          {{ t.dashboard.more }}
        </button>
      </div>
    </template>
    <p
      v-else-if="!error"
      class="kin-muted"
    >
      {{ t.common.loading }}
    </p>

    <DialogRoot v-model:open="createOpen">
      <DialogPortal>
        <DialogOverlay class="kin-overlay" />
        <DialogContent
          class="kin-dialog"
          :aria-describedby="undefined"
        >
          <DialogTitle>{{ t.dashboard.createTitle }}</DialogTitle>
          <DialogClose
            class="kin-ibtn kin-dialog-x"
            :aria-label="t.common.cancel"
            :title="t.common.cancel"
          >
            <KinIcon name="close" />
          </DialogClose>
          <GroupPicker
            v-model="createGroups"
            class="create-groups"
            :dashboard="d"
          />
          <p class="kin-section-title">
            {{ t.dashboard.template }}
          </p>
          <ul class="templates">
            <li
              v-for="x in templates"
              :key="x.id"
            >
              <button
                class="kin-btn"
                type="button"
                @click="create(x)"
              >
                <KinIcon name="plus" />
                {{ x.definition.name }}
              </button>
            </li>
          </ul>
          <p
            v-if="createError"
            class="kin-error"
            role="alert"
          >
            {{ createError }}
          </p>
        </DialogContent>
      </DialogPortal>
    </DialogRoot>
    <CreateRelationDialog
      v-if="reg"
      v-model:open="relOpen"
      :dashboard="d"
      :reg="reg"
      :template="relTemplate"
      :initial-groups="createGroups"
      @created="openRecord"
    />
  </main>
</template>

<style scoped>
/* дерево — во всю ширину и высоту под верхним меню (F-120) */
.tree-screen {
  position: relative;
  flex: 1;
  min-height: 0;
}
.graph-soon {
  display: grid;
  place-items: center;
  padding: var(--kin-space-4);
  text-align: center;
}
.toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kin-space-2);
  align-items: center;
  padding: 8px;
  border-radius: 16px;
}

.search-box {
  position: relative;
  display: flex;
  flex: 1;
  align-items: center;
  min-width: 12rem;
  color: var(--kin-color-text-muted);
}

.search-box .kin-icon {
  position: absolute;
  left: 12px;
  pointer-events: none;
}

.search {
  flex: 1;
  padding-left: 36px;
  background: var(--kin-color-bg);
  border-color: transparent;
}

.group-filter {
  max-width: 14rem;
  background-color: var(--kin-color-bg);
  border-color: transparent;
}

.list-island :deep(.list-selection) {
  position: fixed;
  top: 80px;
  right: var(--kin-space-2);
}

@media (width <= 640px) {
  .list-island :deep(.list-selection) {
    top: auto;
    bottom: var(--kin-space-2);
  }
}

.list-island {
  padding: 0;
}

.create-groups {
  margin-bottom: var(--kin-space-3);
}

.templates {
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.templates .kin-btn {
  width: 100%;
}

.footer {
  padding: 0 var(--kin-space-1);
  font-size: var(--kin-font-tiny);
}
</style>
