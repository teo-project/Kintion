import { describe, expect, it } from 'vitest'

import { applyTheme, loadTheme, saveTheme } from './theme'

function memory(): Storage {
  const m = new Map<string, string>()
  return {
    get length() {
      return m.size
    },
    clear: () => m.clear(),
    getItem: (k) => m.get(k) ?? null,
    key: (i) => [...m.keys()][i] ?? null,
    removeItem: (k) => void m.delete(k),
    setItem: (k, v) => void m.set(k, v),
  }
}

describe('theme', () => {
  it('defaults to system and ignores garbage', () => {
    const s = memory()
    expect(loadTheme(s)).toBe('system')
    s.setItem('kintion.theme', 'purple')
    expect(loadTheme(s)).toBe('system')
  })

  it('round-trips the choice', () => {
    const s = memory()
    saveTheme('dark', s)
    expect(loadTheme(s)).toBe('dark')
  })

  it('sets data-theme only for explicit choice', () => {
    const el = document.createElement('html')
    applyTheme('light', el)
    expect(el.getAttribute('data-theme')).toBe('light')
    applyTheme('system', el)
    expect(el.hasAttribute('data-theme')).toBe(false)
  })
})

describe('favicon', () => {
  it('follows the system scheme only: the theme switch does not touch it', () => {
    document.head.innerHTML = '<link id="favicon" rel="icon" href="/favicon.svg">'
    const href = () => document.getElementById('favicon')?.getAttribute('href')
    applyTheme('light')
    expect(href()).toBe('/favicon.svg')
    applyTheme('dark')
    expect(href()).toBe('/favicon.svg')
  })
})
