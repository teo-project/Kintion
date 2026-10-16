import { defineStore } from 'pinia'
import { ref } from 'vue'

import { applyTheme, loadTheme, saveTheme, type ThemeChoice } from '@/theme'

export const useThemeStore = defineStore('theme', () => {
  const choice = ref<ThemeChoice>(loadTheme())

  function set(next: ThemeChoice) {
    choice.value = next
    saveTheme(next)
    applyTheme(next)
  }

  return { choice, set }
})
