<script setup lang="ts">
// Ввод частичной даты тремя полями (F-14): день, месяц, год — каждое
// необязательно. Проверка — ядром (validateDate): 29.02 без года можно,
// с невисокосным годом — нет; 31.04 — нет.
import { computed, ref, watch } from 'vue'

import { isEmptyDate, validateDate, type PartialDate } from '@/core/dates'
import { t } from '@/i18n'

const props = defineProps<{ modelValue: PartialDate | null; label?: string }>()
const emit = defineEmits<{ 'update:modelValue': [PartialDate | null] }>()

const day = ref('')
const month = ref('')
const year = ref('')

function sync(v: PartialDate | null) {
  day.value = v?.d != null ? String(v.d) : ''
  month.value = v?.m != null ? String(v.m) : ''
  year.value = v?.y != null ? String(v.y) : ''
}
sync(props.modelValue)

function num(s: string): number | null {
  const v = s.trim()
  return v === '' ? null : Number(v)
}

/** Собранная дата; пустые части — null; ни одной части — null. */
const parsed = computed<PartialDate | null>(() => {
  const p: PartialDate = { y: num(year.value), m: num(month.value), d: num(day.value) }
  return isEmptyDate(p) ? null : p
})

const error = computed(() => (parsed.value && validateDate(parsed.value) ? t.card.dateError : ''))

watch(() => props.modelValue, v => {
  if (JSON.stringify(v ?? null) !== JSON.stringify(parsed.value)) sync(v)
})
watch(parsed, v => emit('update:modelValue', v))

/** Только цифры с клавиатуры. */
const parts = { d: day, m: month, y: year }
function digits(e: Event, part: keyof typeof parts) {
  const el = e.target as HTMLInputElement
  const v = el.value.replace(/\D/g, '')
  if (v !== el.value) el.value = v
  parts[part].value = v
}
</script>

<template>
  <fieldset
    class="pdate"
    :aria-invalid="error ? 'true' : undefined"
  >
    <legend
      v-if="label"
      class="kin-label"
    >
      {{ label }}
    </legend>
    <div class="parts">
      <input
        class="kin-input part day"
        :value="day"
        inputmode="numeric"
        maxlength="2"
        :placeholder="t.dates.partDay"
        :aria-label="t.dates.partDay"
        :aria-invalid="error ? 'true' : undefined"
        @input="e => digits(e, 'd')"
      >
      <input
        class="kin-input part month"
        :value="month"
        inputmode="numeric"
        maxlength="2"
        :placeholder="t.dates.partMonth"
        :aria-label="t.dates.partMonth"
        :aria-invalid="error ? 'true' : undefined"
        @input="e => digits(e, 'm')"
      >
      <input
        class="kin-input part year"
        :value="year"
        inputmode="numeric"
        maxlength="4"
        :placeholder="t.dates.partYear"
        :aria-label="t.dates.partYear"
        :aria-invalid="error ? 'true' : undefined"
        @input="e => digits(e, 'y')"
      >
    </div>
    <p
      v-if="error"
      class="kin-error"
      role="alert"
    >
      {{ error }}
    </p>
  </fieldset>
</template>

<style scoped>
.pdate {
  margin: 0;
  padding: 0;
  border: none;
}

.parts {
  display: flex;
  gap: var(--kin-space-1);
}

.day,
.month {
  width: 4.5rem;
}

.year {
  width: 5.5rem;
}
</style>
