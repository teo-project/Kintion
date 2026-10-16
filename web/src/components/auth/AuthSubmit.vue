<script setup lang="ts">
// Кнопка отправки формы входа: иконка-стрелка, подпись — в aria-label и title (F-90).
defineProps<{ label: string; busy?: boolean }>()
</script>

<template>
  <button
    class="submit"
    type="submit"
    :aria-label="label"
    :title="label"
    :disabled="busy"
  >
    <svg
      viewBox="0 0 24 24"
      aria-hidden="true"
    >
      <path d="M5 12h14M13 6l6 6-6 6" />
    </svg>
  </button>
</template>

<style scoped>
.submit {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 2.75rem;
  margin-top: var(--kin-space-1);
  background: var(--kin-color-accent);
  border: 0;
  border-radius: 12px;
  cursor: pointer;
}

.submit:disabled {
  opacity: 0.5;
  cursor: default;
}

.submit svg {
  width: 1.25rem;
  height: 1.25rem;
  fill: none;
  stroke: var(--kin-color-accent-contrast);
  stroke-width: 2.4;
  stroke-linecap: round;
  stroke-linejoin: round;
}
</style>
