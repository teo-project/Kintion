// Выбор темы: системная (по prefers-color-scheme), светлая или тёмная.
export const themeChoices = ['system', 'light', 'dark'] as const
export type ThemeChoice = (typeof themeChoices)[number]

const storageKey = 'kintion.theme'

function isChoice(v: unknown): v is ThemeChoice {
  return typeof v === 'string' && (themeChoices as readonly string[]).includes(v)
}

export function loadTheme(storage: Pick<Storage, 'getItem'> | undefined = globalThis.localStorage): ThemeChoice {
  try {
    const v = storage?.getItem(storageKey)
    return isChoice(v) ? v : 'system'
  } catch {
    return 'system'
  }
}

export function saveTheme(
  choice: ThemeChoice,
  storage: Pick<Storage, 'setItem'> | undefined = globalThis.localStorage,
): void {
  try {
    storage?.setItem(storageKey, choice)
  } catch {
    // хранилище недоступно (приватный режим) — выбор действует до перезагрузки
  }
}

export function applyTheme(choice: ThemeChoice, root: HTMLElement = document.documentElement): void {
  if (choice === 'system') {
    root.removeAttribute('data-theme')
  } else {
    root.setAttribute('data-theme', choice)
  }
}
