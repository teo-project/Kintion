<script setup lang="ts">
// Аккаунты сервера (F-28): ссылки-приглашения на регистрацию — админу в
// режиме invite, любому в invite-all (свои); список и создание аккаунтов — админу.
import { computed, onMounted, ref } from 'vue'

import { api } from '@/api/client'
import type { AdminAccountDTO, InviteDTO, RegisterStatusDTO } from '@/api/types'
import { errorText, fmt, t } from '@/i18n'
import { useSessionStore } from '@/stores/session'
import KinIcon from './KinIcon.vue'

const admin = computed(() => !!useSessionStore().account?.is_admin)

const accounts = ref<AdminAccountDTO[]>([])
const invites = ref<InviteDTO[]>([])
const mode = ref<RegisterStatusDTO['mode'] | ''>('')
const link = ref('')
const copied = ref(false)
const error = ref('')
const login = ref('')
const nick = ref('')
const password = ref('')
const busy = ref(false)

async function load() {
  error.value = ''
  try {
    const s = await api.registerStatus()
    mode.value = s.mode
    const [a, i] = await Promise.all([
      admin.value ? api.adminAccounts() : Promise.resolve([]),
      canInvite.value ? api.invites() : Promise.resolve([]),
    ])
    accounts.value = a
    invites.value = i
  } catch (e) {
    error.value = errorText(e)
  }
}
onMounted(load)

/** приглашать может: в invite — админ, в invite-all — любой */
const canInvite = computed(() => mode.value === 'invite-all' || (mode.value === 'invite' && admin.value))

const modeText = () =>
  mode.value === 'open'
    ? t.sharing.modeOpen
    : mode.value === 'closed'
      ? t.sharing.modeClosed
      : mode.value === 'invite-all' ? t.sharing.modeInviteAll : t.sharing.modeInvite

async function createInvite() {
  error.value = ''
  copied.value = false
  try {
    const inv = await api.createInvite()
    link.value = `${location.origin}${import.meta.env.BASE_URL}register?token=${encodeURIComponent(inv.token ?? '')}`
    invites.value = await api.invites()
  } catch (e) {
    error.value = errorText(e)
  }
}

async function copy() {
  try {
    await navigator.clipboard.writeText(link.value)
    copied.value = true
  } catch {
    // буфер недоступен — ссылку видно в поле
  }
}

async function deleteInvite(id: string) {
  error.value = ''
  try {
    await api.deleteInvite(id)
    invites.value = invites.value.filter(x => x.id !== id)
  } catch (e) {
    error.value = errorText(e)
  }
}

async function createAccount() {
  error.value = ''
  busy.value = true
  try {
    const a = await api.adminCreateAccount({ login: login.value, password: password.value, nick: nick.value })
    accounts.value = [...accounts.value, a]
    login.value = nick.value = password.value = ''
  } catch (e) {
    error.value = errorText(e)
  } finally {
    busy.value = false
  }
}

const date = (s: string) => new Date(s).toLocaleDateString()
</script>

<template>
  <section
    v-if="admin || canInvite"
    class="kin-island admin"
  >
    <h2 class="kin-section-title">
      {{ admin ? t.sharing.admin : t.sharing.invitesTitle }}
    </h2>
    <p class="kin-muted hint">
      {{ fmt(t.sharing.adminHint, { mode: modeText() }) }}
    </p>
    <button
      v-if="canInvite"
      class="kin-btn"
      type="button"
      @click="createInvite"
    >
      <KinIcon name="plus" />
      {{ t.sharing.createInvite }}
    </button>
    <div
      v-if="link"
      class="link"
    >
      <p class="kin-muted">
        {{ t.sharing.inviteCreated }}
      </p>
      <div class="kin-row">
        <input
          class="kin-input grow"
          readonly
          :value="link"
          :aria-label="t.sharing.createInvite"
          @focus="($event.target as HTMLInputElement).select()"
        >
        <button
          class="kin-btn kin-btn--small"
          type="button"
          @click="copy"
        >
          {{ copied ? t.sharing.copied : t.sharing.copy }}
        </button>
      </div>
    </div>
    <template v-if="invites.length">
      <h3 class="kin-section-title sub">
        {{ t.sharing.invites }}
      </h3>
      <ul class="rows">
        <li
          v-for="i in invites"
          :key="i.id"
          class="kin-row"
        >
          <span class="grow">{{ fmt(t.sharing.inviteUntil, { date: date(i.expires_at) }) }}</span>
          <button
            class="kin-ibtn"
            type="button"
            :aria-label="t.sharing.deleteInvite"
            :title="t.sharing.deleteInvite"
            @click="deleteInvite(i.id)"
          >
            <KinIcon name="trash" />
          </button>
        </li>
      </ul>
    </template>
    <template v-if="admin">
      <h3 class="kin-section-title sub">
        {{ t.sharing.accounts }}
      </h3>
      <ul class="rows">
        <li
          v-for="a in accounts"
          :key="a.id"
          class="kin-row"
        >
          <span class="grow">{{ a.login }}<span
            v-if="a.nick"
            class="kin-muted"
          > · {{ a.nick }}</span></span>
          <span
            v-if="a.is_admin"
            class="kin-muted"
          >{{ t.sharing.adminMark }}</span>
        </li>
      </ul>
      <form
        class="create"
        @submit.prevent="createAccount"
      >
        <input
          v-model="login"
          class="kin-input"
          name="new-login"
          autocomplete="off"
          :placeholder="t.sharing.login"
          :aria-label="t.sharing.login"
          required
        >
        <input
          v-model="nick"
          class="kin-input"
          name="new-nick"
          autocomplete="off"
          :placeholder="t.sharing.nick"
          :aria-label="t.sharing.nick"
        >
        <input
          v-model="password"
          class="kin-input"
          name="new-password"
          type="password"
          autocomplete="new-password"
          :placeholder="t.sharing.password"
          :aria-label="t.sharing.password"
          required
        >
        <button
          class="kin-btn"
          type="submit"
          :disabled="busy"
        >
          {{ t.sharing.createAccount }}
        </button>
      </form>
    </template>
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
.grow {
  flex: 1;
  min-width: 0;
}
.hint {
  margin: 0 0 12px;
  font-size: var(--kin-font-small);
}
.link {
  margin-top: 12px;
}
.sub {
  margin-top: 16px;
}
.rows {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 6px;
}
.create {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 8px;
  margin-top: 12px;
}
</style>
