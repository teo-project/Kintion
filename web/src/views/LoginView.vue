<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import AuthShell from '@/components/auth/AuthShell.vue'
import AuthSubmit from '@/components/auth/AuthSubmit.vue'
import PasswordInput from '@/components/auth/PasswordInput.vue'
import { api } from '@/api/client'
import { errorText, t } from '@/i18n'
import { useSessionStore } from '@/stores/session'

const session = useSessionStore()
const route = useRoute()
const router = useRouter()

const login = ref('')
const password = ref('')
const error = ref('')
const busy = ref(false)
// ссылка на регистрацию — только когда она открыта для всех (F-28)
const canRegister = ref(false)
onMounted(async () => {
  try {
    canRegister.value = (await api.registerStatus()).mode === 'open'
  } catch {
    // без ответа — без ссылки
  }
})

/** Куда вернуться после входа: только путь этого приложения. */
function nextPath(): string {
  const n = route.query.next
  return typeof n === 'string' && n.startsWith('/') && !n.startsWith('//') ? n : '/'
}

async function submit() {
  error.value = ''
  busy.value = true
  try {
    await session.login(login.value, password.value)
    await router.replace(nextPath())
  } catch (e) {
    error.value = errorText(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <AuthShell
    :title="t.auth.loginTitle"
    :hint="t.auth.loginHint"
  >
    <form
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
      <PasswordInput
        v-model="password"
        autocomplete="current-password"
        :label="t.auth.password"
      />
      <p
        v-if="error"
        class="kin-error"
        role="alert"
      >
        {{ error }}
      </p>
      <AuthSubmit
        :label="t.auth.loginSubmit"
        :busy="busy"
      />
    </form>
    <RouterLink
      v-if="canRegister"
      class="auth-link"
      :to="{ name: 'register' }"
    >
      {{ t.sharing.registerLink }}
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
