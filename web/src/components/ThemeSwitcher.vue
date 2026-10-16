<script setup lang="ts">
// Выбор темы (F-90): иконка-кнопка перебирает «как в системе → светлая →
// тёмная»; текущий выбор — в иконке, подписи и подсказке.
import { computed } from 'vue'

import { t } from '@/i18n'
import { useThemeStore } from '@/stores/theme'
import { themeChoices } from '@/theme'

import KinIcon from './KinIcon.vue'

const theme = useThemeStore()

const ICONS = { system: 'monitor', light: 'sun', dark: 'moon' } as const

const label = computed(() => `${t.theme.label}: ${t.theme[theme.choice]}`)

function next() {
  const i = themeChoices.indexOf(theme.choice)
  theme.set(themeChoices[(i + 1) % themeChoices.length]!)
}
</script>

<template>
  <button
    class="kin-ibtn kin-ibtn--ghost theme-switcher"
    type="button"
    :aria-label="label"
    :title="label"
    :data-theme-choice="theme.choice"
    @click="next"
  >
    <KinIcon :name="ICONS[theme.choice]" />
  </button>
</template>
