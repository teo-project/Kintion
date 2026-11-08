<script setup lang="ts">
// Доступ к дашборду (F-03): новому гостю право на группу выдаётся по нику,
// гостю дашборда — по аккаунту (ник не нужен, без ника виден логин) —
// просмотр или правка; наследуется вниз по вложенности групп (F-105). Право на
// техническую группу гостя «Созданные <ник>» — может ли он создавать записи (F-149).
import { computed, onMounted, ref } from 'vue'

import { api } from '@/api/client'
import type { GroupDTO, MemberDTO } from '@/api/types'
import { errorText, t } from '@/i18n'
import KinIcon from './KinIcon.vue'

const props = defineProps<{ dashboard: string }>()

const groups = ref<GroupDTO[]>([])
const members = ref<MemberDTO[]>([])
const nick = ref('')
const group = ref('')
const canEdit = ref(false)
const error = ref('')
const busy = ref(false)
// группа, добавляемая гостю дашборда, — по аккаунту
const adding = ref<Record<string, string>>({})

async function load() {
  error.value = ''
  try {
    const [g, m] = await Promise.all([api.groups(props.dashboard), api.members(props.dashboard)])
    groups.value = g
    members.value = m
    if (!group.value) group.value = g[0]?.id ?? ''
  } catch (e) {
    error.value = errorText(e)
  }
}
onMounted(load)

const groupName = computed(() => {
  const m = new Map(groups.value.map(g => [g.id, g.name]))
  return (id: string) => m.get(id) ?? '—'
})

async function run(fn: () => Promise<MemberDTO[]>) {
  error.value = ''
  busy.value = true
  try {
    members.value = await fn()
    return true
  } catch (e) {
    error.value = errorText(e)
    return false
  } finally {
    busy.value = false
  }
}

async function grant() {
  if (await run(() => api.putGrant(props.dashboard, { nick: nick.value, group_id: group.value, can_edit: canEdit.value }))) {
    nick.value = ''
  }
}

function removeMember(acc: string) {
  void run(() => api.deleteMember(props.dashboard, acc))
}

function revoke(groupId: string, acc: string) {
  void run(() => api.deleteGrant(props.dashboard, groupId, acc))
}

function setMode(m: MemberDTO, groupId: string, edit: boolean) {
  void run(() => api.putGrant(props.dashboard, { account_id: m.account_id, group_id: groupId, can_edit: edit }))
}

function freeGroups(m: MemberDTO) {
  return groups.value.filter(g => !m.grants.some(x => x.group_id === g.id))
}

async function addGroup(m: MemberDTO) {
  const groupId = adding.value[m.account_id]
  if (!groupId) return
  if (await run(() => api.putGrant(props.dashboard, { account_id: m.account_id, group_id: groupId, can_edit: false }))) {
    adding.value[m.account_id] = ''
  }
}
</script>

<template>
  <section class="kin-island pane-island">
    <h3 class="kin-section-title">
      {{ t.sharing.access }}
    </h3>
    <p class="kin-muted hint">
      {{ t.sharing.accessHint }}
    </p>
    <p
      v-if="!groups.length"
      class="kin-muted"
    >
      {{ t.sharing.noGroups }}
    </p>
    <form
      v-else
      class="grant"
      @submit.prevent="grant"
    >
      <input
        v-model="nick"
        class="kin-input"
        name="grant-nick"
        autocomplete="off"
        :placeholder="t.sharing.grantNick"
        :aria-label="t.sharing.grantNick"
        required
      >
      <select
        v-model="group"
        class="kin-input"
        :aria-label="t.sharing.grantGroup"
      >
        <option
          v-for="g in groups"
          :key="g.id"
          :value="g.id"
        >
          {{ g.name }}
        </option>
      </select>
      <select
        v-model="canEdit"
        class="kin-input"
        :aria-label="t.sharing.grantRead"
      >
        <option :value="false">
          {{ t.sharing.grantRead }}
        </option>
        <option :value="true">
          {{ t.sharing.grantEdit }}
        </option>
      </select>
      <button
        class="kin-btn kin-btn--primary"
        type="submit"
        :disabled="busy"
      >
        {{ t.sharing.grantAdd }}
      </button>
    </form>
  </section>
  <section class="kin-island pane-island">
    <h3 class="kin-section-title">
      {{ t.sharing.members }}
    </h3>
    <p
      v-if="!members.length"
      class="kin-muted"
    >
      {{ t.sharing.noMembers }}
    </p>
    <ul class="members">
      <li
        v-for="m in members"
        :key="m.account_id"
      >
        <div class="kin-row">
          <strong class="grow">{{ m.nick ?? m.login }}</strong>
          <span
            v-if="!m.accepted"
            class="kin-muted"
          >{{ t.sharing.pending }}</span>
          <button
            class="kin-ibtn"
            type="button"
            :disabled="busy"
            :aria-label="t.sharing.removeMember"
            :title="t.sharing.removeMember"
            @click="removeMember(m.account_id)"
          >
            <KinIcon name="trash" />
          </button>
        </div>
        <ul class="grants">
          <li
            v-for="g in m.grants"
            :key="g.group_id"
            class="kin-row"
          >
            <span
              class="grow"
              :title="g.group_id === m.own_group ? t.sharing.ownGroupHint : undefined"
            >{{ groupName(g.group_id) }}</span>
            <select
              class="kin-input mode"
              :value="g.can_edit"
              :disabled="busy"
              :aria-label="groupName(g.group_id)"
              @change="setMode(m, g.group_id, ($event.target as HTMLSelectElement).value === 'true')"
            >
              <option :value="false">
                {{ g.group_id === m.own_group ? t.sharing.ownGroupRead : t.sharing.grantRead }}
              </option>
              <option :value="true">
                {{ g.group_id === m.own_group ? t.sharing.ownGroupEdit : t.sharing.grantEdit }}
              </option>
            </select>
            <button
              class="kin-ibtn"
              type="button"
              :disabled="busy"
              :aria-label="t.sharing.revoke"
              :title="t.sharing.revoke"
              @click="revoke(g.group_id, m.account_id)"
            >
              <KinIcon name="close" />
            </button>
          </li>
          <li
            v-if="freeGroups(m).length"
            class="kin-row"
          >
            <select
              v-model="adding[m.account_id]"
              class="kin-input grow"
              :aria-label="t.sharing.grantGroup"
            >
              <option value="">
                {{ t.sharing.addGroup }}
              </option>
              <option
                v-for="g in freeGroups(m)"
                :key="g.id"
                :value="g.id"
              >
                {{ g.name }}
              </option>
            </select>
            <button
              class="kin-btn"
              type="button"
              :disabled="busy || !adding[m.account_id]"
              @click="addGroup(m)"
            >
              {{ t.sharing.grantAdd }}
            </button>
          </li>
        </ul>
      </li>
    </ul>
    <p
      v-if="error"
      class="kin-error"
      role="alert"
    >
      {{ error }}
    </p>
  </section>
</template>

<style scoped>
.hint {
  margin: 0 0 12px;
  font-size: var(--kin-font-small);
}
.grant {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
  gap: 8px;
  margin-bottom: 8px;
}
.grow {
  flex: 1;
  min-width: 0;
}
.members,
.grants {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 6px;
}
.members > li + li {
  border-top: 1px solid var(--kin-color-border);
  padding-top: 8px;
}
.grants {
  padding-left: 16px;
  margin-top: 4px;
}
.mode {
  width: auto;
}
</style>
