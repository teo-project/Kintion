<script setup lang="ts">
// Регистрация (F-28): по ссылке-приглашению админа (?token=) или открытая.
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { api } from '@/api/client'
import type { RegisterStatusDTO } from '@/api/types'
import AuthShell from '@/components/auth/AuthShell.vue'
import AuthSubmit from '@/components/auth/AuthSubmit.vue'
import PasswordInput from '@/components/auth/PasswordInput.vue'
import { errorText, t } from '@/i18n'
import { useSessionStore } from '@/stores/session'

const session = useSessionStore()
const route = useRoute()
const router = useRouter()

const token = computed(() => (typeof route.query.token === 'string' ? route.query.token : ''))
const status = ref<RegisterStatusDTO | null>(null)
const login = ref('')
const nick = ref('')
const password = ref('')
const repeat = ref('')
const error = ref('')
const busy = ref(false)

onMounted(async () => {
  try {
    status.value = await api.registerStatus(token.value || undefined)
  } catch (e) {
    error.value = errorText(e)
  }
})

/** почему регистрироваться нельзя; '' — можно */
const blocked = computed(() => {
  const s = status.value
  if (!s) return ''
  if (s.mode === 'open') return ''
  if (s.mode === 'invite' || s.mode === 'invite-all') return s.invite_valid ? '' : t.sharing.inviteInvalid
  return t.sharing.registrationClosed
})

async function submit() {
  error.value = ''
  if (password.value !== repeat.value) {
    error.value = t.auth.passwordMismatch
    return
  }
  busy.value = true
  try {
    await session.register({ token: token.value || undefined, login: login.value, password: password.value, nick: nick.value })
    await router.replace('/')
  } catch (e) {
    error.value = errorText(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <AuthShell
    :title="t.sharing.registerTitle"
    :hint="t.sharing.registerHint"
  >
    <p
      v-if="blocked"
      class="kin-notice"
      role="alert"
    >
      {{ blocked }}
    </p>
    <form
      v-else-if="status"
      class="kin-form"
      @submit.prevent="submit"
    >
      <label class="kin-label">
        {{ t.auth.login }}
        <input
          v-model="login"
          class="kin-input"
          autocomplete="username"
          required
        >
      </label>
      <label class="kin-label">
        {{ t.sharing.nick }}
        <input
          v-model="nick"
          class="kin-input"
          autocomplete="nickname"
        >
        <span class="kin-muted">{{ t.sharing.nickHint }}</span>
      </label>
      <PasswordInput
        v-model="password"
        autocomplete="new-password"
        :label="t.auth.password"
      />
      <PasswordInput
        v-model="repeat"
        autocomplete="new-password"
        :label="t.auth.passwordRepeat"
      />
      <p
        v-if="error"
        class="kin-error"
        role="alert"
      >
        {{ error }}
      </p>
      <AuthSubmit
        :label="t.sharing.registerSubmit"
        :busy="busy"
      />
    </form>
    <p
      v-else-if="error"
      class="kin-error"
      role="alert"
    >
      {{ error }}
    </p>
    <RouterLink
      class="auth-link"
      :to="{ name: 'login' }"
    >
      {{ t.auth.loginSubmit }}
    </RouterLink>
  </AuthShell>
</template>

<style scoped>
.auth-link {
  display: block;
  margin-top: 16px;
  text-align: center;
  font-size: var(--kin-font-small);
}
</style>
