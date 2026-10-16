<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'

import { api } from '@/api/client'
import type { CardMode } from '@/api/types'
import { errorText, fmt, t } from '@/i18n'
import { useSessionStore } from '@/stores/session'
import { usePrefs } from '@/ui/prefs'
import { PROJECT_REPO_URL } from '@/ui/project'
import KinIcon from '../components/KinIcon.vue'

const session = useSessionStore()
const router = useRouter()

const current = ref('')
const next = ref('')
const repeat = ref('')
const error = ref('')
const done = ref(false)
const busy = ref(false)

async function change() {
  error.value = ''
  done.value = false
  if (next.value !== repeat.value) {
    error.value = t.auth.passwordMismatch
    return
  }
  busy.value = true
  try {
    await api.changePassword(current.value, next.value)
    current.value = next.value = repeat.value = ''
    done.value = true
  } catch (e) {
    error.value = errorText(e)
  } finally {
    busy.value = false
  }
}

// запомненный выбор правки поля с историей (F-80) и его сброс
const { prefs, loadPrefs, setHistoryEditPref, setCardModePref } = usePrefs()
const prefsError = ref('')
onMounted(async () => {
  try {
    await loadPrefs()
  } catch (e) {
    prefsError.value = errorText(e)
  }
})
const historyEdit = computed(() => prefs.value?.history_edit ?? '')
const historyEditText = computed(() => {
  const h = t.card.historyAsk
  return historyEdit.value === 'replace' ? h.always_replace : historyEdit.value === 'add' ? h.always_add : h.ask
})

// режим карточек (F-108): хранится на сервере у аккаунта
const CARD_MODES: CardMode[] = ['read', 'inline', 'form']
const cardMode = computed(() => prefs.value?.card_mode ?? 'read')
const cardModeHint = (m: CardMode) => t.card.cardMode[`${m}Hint`]

async function setCardMode(m: CardMode) {
  prefsError.value = ''
  try {
    await setCardModePref(m)
  } catch (e) {
    prefsError.value = errorText(e)
  }
}

async function resetHistoryEdit() {
  prefsError.value = ''
  try {
    await setHistoryEditPref('')
  } catch (e) {
    prefsError.value = errorText(e)
  }
}

// тексты лицензий сторонних библиотек (F-91): отдаёт Go только вошедшим
const licensesUrl = `${import.meta.env.BASE_URL}api/licenses`

async function logout() {
  await session.logout()
  await router.replace({ name: 'login' })
}
</script>

<template>
  <main class="account kin-page">
    <div class="kin-page-head">
      <h1 class="kin-page-title">
        {{ t.auth.account }}
      </h1>
    </div>
    <section class="kin-island">
      <p class="kin-row signed-in">
        <span>{{ fmt(t.auth.signedInAs, { login: session.account?.login ?? '' }) }}</span>
        <button
          class="kin-ibtn"
          type="button"
          :aria-label="t.auth.logout"
          :title="t.auth.logout"
          @click="logout"
        >
          <KinIcon name="logout" />
        </button>
      </p>
    </section>
    <section class="kin-island">
      <h2 class="kin-section-title">
        {{ t.card.cardMode.setting }}
      </h2>
      <div
        class="card-mode"
        role="radiogroup"
        :aria-label="t.card.cardMode.setting"
      >
        <label
          v-for="m in CARD_MODES"
          :key="m"
          class="kin-row mode-option"
        >
          <input
            type="radio"
            name="card-mode"
            :value="m"
            :checked="cardMode === m"
            :disabled="!prefs"
            @change="setCardMode(m)"
          >
          <span>
            <span class="mode-name">{{ t.card.cardMode[m] }}</span>
            <span class="kin-muted mode-hint">{{ cardModeHint(m) }}</span>
          </span>
        </label>
      </div>
    </section>
    <section class="kin-island">
      <h2 class="kin-section-title">
        {{ t.card.historyAsk.setting }}
      </h2>
      <p class="kin-row history-edit">
        <span>{{ historyEditText }}</span>
        <button
          v-if="historyEdit"
          class="kin-btn kin-btn--small"
          type="button"
          @click="resetHistoryEdit"
        >
          {{ t.card.historyAsk.reset }}
        </button>
      </p>
      <p
        v-if="prefsError"
        class="kin-error"
        role="alert"
      >
        {{ prefsError }}
      </p>
    </section>
    <section class="kin-island">
      <h2 class="kin-section-title">
        {{ t.auth.changePassword }}
      </h2>
      <form
        class="kin-form"
        @submit.prevent="change"
      >
        <label class="kin-label">
          {{ t.auth.currentPassword }}
          <input
            v-model="current"
            class="kin-input"
            type="password"
            autocomplete="current-password"
            required
          >
        </label>
        <label class="kin-label">
          {{ t.auth.newPassword }}
          <input
            v-model="next"
            class="kin-input"
            type="password"
            autocomplete="new-password"
            required
          >
        </label>
        <label class="kin-label">
          {{ t.auth.passwordRepeat }}
          <input
            v-model="repeat"
            class="kin-input"
            type="password"
            autocomplete="new-password"
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
        <p
          v-if="done"
          class="kin-notice"
          role="status"
        >
          {{ t.auth.passwordChanged }}
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
        </div>
      </form>
    </section>
    <section class="kin-island">
      <h2 class="kin-section-title">
        {{ t.auth.about }}
      </h2>
      <p class="kin-row">
        <a
          :href="licensesUrl"
          target="_blank"
          rel="noopener"
        >{{ t.auth.thirdPartyLicenses }}</a>
      </p>
      <p class="kin-row">
        <a
          class="repo"
          :href="PROJECT_REPO_URL"
          target="_blank"
          rel="noopener"
        ><KinIcon name="github" />{{ t.auth.sourceCode }}</a>
      </p>
    </section>
  </main>
</template>

<style scoped>
.repo {
  display: inline-flex;
  gap: var(--kin-space-2);
  align-items: center;
}

.card-mode {
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-2);
}

.mode-option {
  align-items: baseline;
}

.mode-name {
  display: block;
  font-weight: 600;
}

.mode-hint {
  font-size: var(--kin-font-tiny);
}

.account {
  max-width: 40rem;
}

.signed-in,
.history-edit {
  justify-content: space-between;
  margin: 0;
}

.kin-form {
  max-width: none;
}
</style>
