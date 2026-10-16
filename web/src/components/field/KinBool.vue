<script setup lang="ts">
// Поле «да/нет» в один клик: компактные сегменты «Да | Нет» (.kin-seg).
// Повторный клик по выбранному очищает значение: пусто ≠ «нет» (F-42).
import { t } from '@/i18n'

defineProps<{ modelValue: boolean | null | undefined; ariaLabel?: string; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean | null] }>()
</script>

<template>
  <div
    class="kin-seg kin-seg--compact kin-bool"
    role="group"
    :aria-label="ariaLabel"
  >
    <button
      class="seg-item"
      :class="{ active: modelValue === true }"
      type="button"
      :aria-pressed="modelValue === true"
      :disabled="disabled"
      @click="emit('update:modelValue', modelValue === true ? null : true)"
    >
      {{ t.values.yes }}
    </button>
    <button
      class="seg-item"
      :class="{ active: modelValue === false }"
      type="button"
      :aria-pressed="modelValue === false"
      :disabled="disabled"
      @click="emit('update:modelValue', modelValue === false ? null : false)"
    >
      {{ t.values.no }}
    </button>
  </div>
</template>
