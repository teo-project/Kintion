<script setup lang="ts">
// Панель выделения дерева (F-131): сколько выделено, «Добавить в группу»
// (в том числе в новую, F-122) и «Убрать из группы» — одной массовой
// операцией через API (журнал и версии — у каждой записи).
import { onMounted, ref, shallowRef } from 'vue'

import { api } from '@/api/client'
import type { GroupDTO } from '@/api/types'
import KinIcon from '@/components/KinIcon.vue'
import { errorText, fmt, t } from '@/i18n'

const props = defineProps<{ d: string; ids: string[] }>()
/** clear — снять выделение; changed — метки изменились (дерево перечитывается); groups — создана группа */
const emit = defineEmits<{ clear: []; changed: []; groups: [] }>()

const NEW = '+new'
const BULK = 1000
const groups = shallowRef<GroupDTO[]>([])
const group = ref('')
const newName = ref('')
const busy = ref(false)
const error = ref('')
const done = ref('')

async function loadGroups() {
  try {
    groups.value = await api.groups(props.d)
  } catch (e) {
    error.value = errorText(e)
  }
}
onMounted(loadGroups)

async function run(kind: 'add' | 'remove') {
  error.value = ''
  done.value = ''
  busy.value = true
  // состав и дашборд — на момент нажатия (выделение может меняться во время операции)
  const d = props.d
  const ids = [...props.ids]
  let changed = 0
  let parts = 0
  try {
    let g = group.value
    if (g === NEW) {
      const name = newName.value.trim()
      if (!name) return
      g = (await api.createGroup(d, { name })).id
      emit('groups')
      await loadGroups()
      group.value = g
      newName.value = ''
    }
    if (!g) return
    // предел API — 1000 записей за операцию: больше — частями
    for (let i = 0; i < ids.length; i += BULK) {
      changed += (await api.groupMembers(d, g, { [kind]: ids.slice(i, i + BULK) })).changed
      parts++
    }
    done.value = fmt(t.tree.done, { n: changed })
  } catch (e) {
    error.value = errorText(e)
  } finally {
    busy.value = false
    // и при ошибке в следующей части — прошедшие уже изменили метки
    if (parts) emit('changed')
  }
}
</script>

<template>
  <aside
    class="tree-selection kin-island"
    :aria-label="t.tree.selection"
    data-testid="tree-selection"
  >
    <div class="kin-row head">
      <strong>{{ fmt(t.tree.selected, { n: ids.length }) }}</strong>
      <button
        class="kin-ibtn kin-ibtn--sm"
        type="button"
        :aria-label="t.tree.clearSelection"
        :title="t.tree.clearSelection"
        @click="emit('clear')"
      >
        <KinIcon name="close" />
      </button>
    </div>
    <select
      v-model="group"
      class="kin-input"
      :aria-label="t.groups.choose"
    >
      <option
        value=""
        disabled
      >
        {{ t.groups.choose }}
      </option>
      <option
        v-for="g in groups"
        :key="g.id"
        :value="g.id"
      >
        {{ g.name }}
      </option>
      <option :value="NEW">
        {{ t.tree.newGroup }}
      </option>
    </select>
    <input
      v-if="group === NEW"
      v-model="newName"
      class="kin-input"
      :placeholder="t.groups.name"
      :aria-label="t.groups.name"
    >
    <button
      class="kin-btn kin-btn--small"
      type="button"
      data-testid="sel-add"
      :disabled="busy || !group || (group === NEW && !newName.trim())"
      @click="run('add')"
    >
      {{ t.tree.addToGroup }}
    </button>
    <button
      class="kin-btn kin-btn--small"
      type="button"
      data-testid="sel-remove"
      :disabled="busy || !group || group === NEW"
      @click="run('remove')"
    >
      {{ t.tree.removeFromGroup }}
    </button>
    <p
      v-if="done"
      class="kin-muted"
      role="status"
    >
      {{ done }}
    </p>
    <p
      v-if="error"
      class="kin-error"
      role="alert"
    >
      {{ error }}
    </p>
  </aside>
</template>

<style scoped>
.tree-selection {
  position: absolute;
  z-index: 6;
  top: 64px;
  right: var(--kin-space-2);
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-2);
  width: 15rem;
  max-width: calc(100% - 2 * var(--kin-space-2));
  box-sizing: border-box;
  padding: 12px;
  border-radius: 16px;
  font-size: var(--kin-font-small);
}

.head {
  justify-content: space-between;
}

.tree-selection p {
  margin: 0;
}
</style>
