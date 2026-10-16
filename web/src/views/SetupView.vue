<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'

import AuthShell from '@/components/auth/AuthShell.vue'
import AuthSubmit from '@/components/auth/AuthSubmit.vue'
import PasswordInput from '@/components/auth/PasswordInput.vue'
import { errorText, t } from '@/i18n'
import { useSessionStore } from '@/stores/session'

const session = useSessionStore()
const router = useRouter()

const token = ref('')
const login = ref('')
const password = ref('')
const repeat = ref('')
const error = ref('')
const busy = ref(false)

async function submit() {
  error.value = ''
  if (password.value !== repeat.value) {
    error.value = t.auth.passwordMismatch
    return
  }
  busy.value = true
  try {
    await session.setup(token.value.trim(), login.value, password.value)
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
    :title="t.auth.setupTitle"
    :hint="t.auth.setupHint"
  >
    <form
      class="kin-form"
      @submit.prevent="submit"
    >
      <label class="kin-label">
        {{ t.auth.token }}
        <input
          v-model="token"
          class="kin-input"
          autocomplete="off"
          required
        >
      </label>
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
        :label="t.auth.setupSubmit"
        :busy="busy"
      />
    </form>
  </AuthShell>
</template>
