<script setup lang="ts">
import { ref, useId } from 'vue'

import { t } from '@/i18n'

// подпись — отдельным <label for>: кнопка «показать» не входит в имя поля
defineProps<{ autocomplete: string; label?: string }>()
const id = useId()
const model = defineModel<string>({ required: true })

const shown = ref(false)
</script>

<template>
  <div class="pw kin-label">
    <label
      v-if="label"
      :for="id"
    >{{ label }}</label>
    <span class="wrap">
      <input
        :id="id"
        v-model="model"
        class="kin-input"
        :type="shown ? 'text' : 'password'"
        :autocomplete="autocomplete"
        required
      >
      <button
        class="eye"
        type="button"
        :aria-label="shown ? t.auth.hidePassword : t.auth.showPassword"
        :title="shown ? t.auth.hidePassword : t.auth.showPassword"
        :aria-pressed="shown"
        @click="shown = !shown"
      >
        <svg
          viewBox="0 0 24 24"
          aria-hidden="true"
        >
          <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z" />
          <circle
            cx="12"
            cy="12"
            r="3"
          />
          <path
            v-if="shown"
            d="M4 4l16 16"
          />
        </svg>
      </button>
    </span>
  </div>
</template>

<style scoped>
.wrap {
  position: relative;
  display: flex;
  flex-direction: column;
}

.kin-input {
  padding-right: 2.75rem;
}

.eye {
  position: absolute;
  top: 50%;
  right: 0.25rem;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 2rem;
  height: 2rem;
  background: transparent;
  border: 0;
  border-radius: var(--kin-radius);
  cursor: pointer;
  transform: translateY(-50%);
}

.eye svg {
  width: 1rem;
  height: 1rem;
  fill: none;
  stroke: var(--kin-color-text-muted);
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.eye:hover svg {
  stroke: var(--kin-color-accent-text);
}
</style>
