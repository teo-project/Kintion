<script setup lang="ts">
// Свой ник — адрес для приглашений в чужие дашборды (F-03).
import { ref } from 'vue'

import { api } from '@/api/client'
import { errorText, t } from '@/i18n'
import { useSessionStore } from '@/stores/session'
import KinIcon from './KinIcon.vue'

const session = useSessionStore()
const nick = ref(session.account?.nick ?? '')
const error = ref('')
const done = ref(false)
const busy = ref(false)

async function save() {
  error.value = ''
  done.value = false
  busy.value = true
  try {
    const acc = await api.setNick(nick.value)
    session.setAccount(acc)
    nick.value = acc.nick ?? ''
    done.value = true
  } catch (e) {
    error.value = errorText(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section class="kin-island">
    <h2 class="kin-section-title">
      {{ t.sharing.nick }}
    </h2>
    <form
      class="kin-row"
      @submit.prevent="save"
    >
      <input
        v-model="nick"
        class="kin-input grow"
        name="nick"
        autocomplete="nickname"
        :aria-label="t.sharing.nick"
        :placeholder="t.sharing.nickNone"
      >
      <button
        class="kin-ibtn kin-ibtn--primary"
        type="submit"
        :disabled="busy"
        :aria-label="t.common.save"
        :title="t.common.save"
      >
        <KinIcon name="check" />
      </button>
    </form>
    <p class="kin-muted hint">
      {{ t.sharing.nickHint }}
    </p>
    <p
      v-if="done"
      class="kin-muted"
      role="status"
    >
      {{ t.sharing.nickSaved }}
    </p>
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
}
.hint {
  margin: 8px 0 0;
  font-size: var(--kin-font-small);
}
</style>
