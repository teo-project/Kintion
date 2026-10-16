<script setup lang="ts">
// Группы дашборда (F-02): плоский список, каждая группа один раз (F-105);
// создать, переименовать, задать «Входит в» и «Включает в себя», удалить.
// Удаление снимает метки с записей и разрывает связи группы: вложенные к её
// родителям не переходят (F-105); записи не удаляются (Д-29). Без прав (0.5).
import { PopoverContent, PopoverPortal, PopoverRoot, PopoverTrigger } from 'reka-ui'
import { computed, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'

import { api } from '@/api/client'
import type { GroupDTO } from '@/api/types'
import { errorText, fmt, t } from '@/i18n'
import { rememberDashboard } from '@/stores/dashboards'
import { ancestors, descendants } from '@/ui/groups'
import { useGroups } from '@/ui/useGroups'
import GroupPicker from '../components/GroupPicker.vue'
import KinCheckbox from '../components/KinCheckbox.vue'
import KinIcon from '../components/KinIcon.vue'

const props = defineProps<{ d: string }>()

const state = computed(() => useGroups(props.d))
const groups = computed(() => state.value.groups.value)
const error = ref('')

watch(() => props.d, d => {
  rememberDashboard(d)
  void state.value.reload().catch(e => (error.value = errorText(e)))
}, { immediate: true })

const name = ref('')
const parents = ref<string[]>([])
const busy = ref(false)

async function run(fn: () => Promise<unknown>): Promise<boolean> {
  error.value = ''
  busy.value = true
  try {
    await fn()
    await state.value.reload()
    return true
  } catch (e) {
    error.value = errorText(e)
    return false
  } finally {
    busy.value = false
  }
}

async function create() {
  const n = name.value.trim()
  if (!n) return
  const body = parents.value.length ? { name: n, parents: parents.value } : { name: n }
  if (await run(() => api.createGroup(props.d, body))) {
    name.value = ''
    parents.value = []
  }
}

/** правка группы во всплывающей форме */
const editing = ref<string | null>(null)
const editName = ref('')
const editParents = ref<string[]>([])
const editChildren = ref<string[]>([])
const editAuto = ref(false)
const editSurnames = ref('')
const confirmDelete = ref(false)

function openEdit(g: GroupDTO, v: boolean) {
  editing.value = v ? g.id : null
  editName.value = g.name
  editParents.value = [...g.parents]
  editChildren.value = [...g.children]
  editAuto.value = g.auto_surname
  editSurnames.value = g.surnames.join('\n')
  confirmDelete.value = false
}

/** нельзя выбрать: в «Входит в» — саму группу и вложенные в неё, в «Включает
 * в себя» — саму группу и те, куда она входит (цикл, F-105) */
const notParents = (g: GroupDTO) => [...descendants(groups.value, g.id)]
const notChildren = (g: GroupDTO) => [...ancestors(groups.value, g.id)]

const sameSet = (a: string[], b: string[]) => a.length === b.length && a.every(x => b.includes(x))
const splitSurnames = (s: string) => s.split(/[,;\n]/).map(x => x.trim()).filter(Boolean)

async function saveEdit(g: GroupDTO) {
  const patch: { name?: string; parents?: string[]; children?: string[]; auto_surname?: boolean; surnames?: string[] } = {}
  const n = editName.value.trim()
  if (n && n !== g.name) patch.name = n
  if (!sameSet(editParents.value, g.parents)) patch.parents = editParents.value
  if (!sameSet(editChildren.value, g.children)) patch.children = editChildren.value
  const sn = splitSurnames(editSurnames.value)
  if (editAuto.value !== g.auto_surname) patch.auto_surname = editAuto.value
  if (sn.join('\n') !== g.surnames.join('\n')) patch.surnames = sn
  if (!Object.keys(patch).length || await run(() => api.patchGroup(props.d, g.id, patch))) editing.value = null
}

async function remove(g: GroupDTO) {
  if (!confirmDelete.value) {
    confirmDelete.value = true
    return
  }
  if (await run(() => api.deleteGroup(props.d, g.id))) editing.value = null
}
</script>

<template>
  <main class="groups kin-page">
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
        {{ t.groups.title }}
      </h1>
    </div>
    <p
      v-if="error"
      class="kin-notice kin-notice--error"
      role="alert"
    >
      {{ error }}
    </p>

    <form
      class="kin-island create"
      @submit.prevent="create"
    >
      <input
        v-model="name"
        class="kin-input"
        :placeholder="t.groups.name"
        :aria-label="t.groups.name"
      >
      <div
        v-if="groups.length"
        class="kin-row create-parents"
      >
        <span class="kin-muted">{{ t.groups.parents }}</span>
        <GroupPicker
          v-model="parents"
          :dashboard="d"
          :label="t.groups.parents"
          no-offers
        />
      </div>
      <button
        class="kin-ibtn kin-ibtn--primary"
        type="submit"
        :disabled="busy || !name.trim()"
        :aria-label="t.common.create"
        :title="t.common.create"
      >
        <KinIcon name="plus" />
      </button>
    </form>

    <section class="kin-island list-island">
      <p
        v-if="!groups.length"
        class="kin-muted empty"
      >
        {{ t.groups.empty }}
      </p>
      <ul
        v-else
        class="tree"
      >
        <li
          v-for="g in groups"
          :key="g.id"
          class="kin-row kin-hover-row"
        >
          <RouterLink :to="{ name: 'dashboard', params: { d }, query: { g: g.id } }">
            {{ g.name }}
          </RouterLink>
          <span class="kin-muted count">{{ fmt(t.groups.count, { n: g.count }) }}</span>
          <span
            v-if="g.parents.length"
            class="kin-muted count parents-count"
          >{{ fmt(t.groups.parentsCount, { n: g.parents.length }) }}</span>
          <span
            v-if="g.children.length"
            class="kin-muted count children-count"
          >{{ fmt(t.groups.childrenCount, { n: g.children.length }) }}</span>
          <PopoverRoot
            :open="editing === g.id"
            @update:open="v => openEdit(g, v)"
          >
            <PopoverTrigger
              class="kin-icon-btn kin-semi"
              :aria-label="`${t.common.edit}: ${g.name}`"
              :title="`${t.common.edit}: ${g.name}`"
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
                  @submit.prevent="saveEdit(g)"
                >
                  <label class="kin-label">
                    {{ t.groups.name }}
                    <input
                      v-model="editName"
                      class="kin-input"
                    >
                  </label>
                  <div class="kin-label edit-parents">
                    {{ t.groups.parents }}
                    <GroupPicker
                      v-model="editParents"
                      :dashboard="d"
                      :exclude="notParents(g)"
                      :label="t.groups.parents"
                      no-offers
                    />
                  </div>
                  <div class="kin-label edit-children">
                    {{ t.groups.children }}
                    <GroupPicker
                      v-model="editChildren"
                      :dashboard="d"
                      :exclude="notChildren(g)"
                      :label="t.groups.children"
                      no-offers
                    />
                  </div>
                  <KinCheckbox
                    v-model="editAuto"
                    class="edit-auto"
                  >
                    {{ t.groups.autoSurname }}
                  </KinCheckbox>
                  <label
                    v-if="editAuto"
                    class="kin-label edit-surnames"
                  >
                    {{ t.groups.surnames }}
                    <textarea
                      v-model="editSurnames"
                      class="kin-input"
                      rows="3"
                    />
                  </label>
                  <p
                    v-if="confirmDelete"
                    class="kin-muted hint"
                  >
                    {{ t.groups.deleteHint }}
                  </p>
                  <div class="kin-actions">
                    <button
                      class="kin-ibtn kin-ibtn--primary"
                      type="submit"
                      :disabled="busy"
                      :aria-label="t.common.save"
                      :title="t.common.save"
                    >
                      <KinIcon name="check" />
                    </button>
                    <button
                      class="kin-ibtn kin-ibtn--danger delete"
                      :class="{ confirm: confirmDelete }"
                      type="button"
                      :disabled="busy"
                      :aria-label="confirmDelete ? t.groups.deleteConfirm : t.common.delete"
                      :title="confirmDelete ? t.groups.deleteConfirm : t.common.delete"
                      @click="remove(g)"
                    >
                      <KinIcon name="trash" />
                    </button>
                  </div>
                </form>
              </PopoverContent>
            </PopoverPortal>
          </PopoverRoot>
        </li>
      </ul>
    </section>
  </main>
</template>

<style scoped>
.groups {
  max-width: 54rem;
}

.count,
.hint {
  font-size: var(--kin-font-tiny);
}

.create {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kin-space-2);
  align-items: center;
  padding: 10px;
  border-radius: 16px;
}

.create input {
  flex: 1;
  min-width: 12rem;
}

.create-parents {
  gap: var(--kin-space-2);
}

.list-island {
  padding: 6px;
}

.empty {
  margin: 0;
  padding: var(--kin-space-4) var(--kin-space-3);
  text-align: center;
}

.tree {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
}

.tree > li {
  min-height: 40px;
  padding-right: 10px;
  border-radius: 12px;
}

.tree > li:hover {
  background: var(--kin-color-surface-raised);
}

.tree a {
  font-weight: 700;
}

.delete.confirm {
  color: var(--kin-color-accent-contrast);
  background: var(--kin-color-danger);
}
</style>
