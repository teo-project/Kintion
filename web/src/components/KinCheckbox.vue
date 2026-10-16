<script setup lang="ts">
// Общий чекбокс «Ноктюрна» (F-90, F-132): Reka UI Checkbox, токены, иконка Lucide.
// Подпись — слот (клик по ней переключает); без слота — только квадрат, тогда
// нужен aria-label. Значение — v-model (boolean).
import { CheckboxIndicator, CheckboxRoot } from 'reka-ui'

import KinIcon from './KinIcon.vue'

defineProps<{ disabled?: boolean; name?: string; ariaLabel?: string }>()
const model = defineModel<boolean>({ default: false })
</script>

<template>
  <label
    class="kin-check"
    :class="{ 'is-disabled': disabled }"
  >
    <CheckboxRoot
      v-model="model"
      class="kin-check-box"
      :disabled="disabled"
      :data-name="name"
      :aria-label="ariaLabel"
    >
      <CheckboxIndicator class="kin-check-mark">
        <KinIcon name="check" />
      </CheckboxIndicator>
    </CheckboxRoot>
    <span
      v-if="$slots.default"
      class="kin-check-text"
    ><slot /></span>
  </label>
</template>
