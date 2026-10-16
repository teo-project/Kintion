import { describe, expect, it } from 'vitest'

import { family } from './__tests__/family'
import { layoutTree } from './layout'
import { suggestExisting } from './suggest'

describe('подсказки «Существующий» (F-130)', () => {
  const input = family()
  const l = layoutTree(input, { depth: 99 })
  const row = (id: string) => l.nodes.find(n => n.type === 'person' && n.person === id)!.row

  it('ребёнок: сначала дети партнёра, без своих детей; все — ниже', () => {
    // у Андрея с Ириной — Дмитрий; у Ирины с Виктором — Алёна
    const s = suggestExisting(input, l, 'andrei', 'down')
    expect(s[0]).toBe('alyona')
    expect(s).not.toContain('dmitry')
    for (const id of s) expect(row(id)).toBeGreaterThan(row('andrei'))
  })

  it('родитель: только выше, без своих родителей; в пределах двух шагов — даже далеко', () => {
    const s = suggestExisting(input, l, 'maksim', 'up')
    expect(s).not.toContain('natalia')
    expect(s).not.toContain('oleg')
    for (const id of s) expect(row(id)).toBeLessThan(row('maksim'))
    // бабушка с дедушкой — через родителя (два шага)
    expect(s).toContain('sergei')
  })

  it('без узла в дереве — пусто', () => {
    expect(suggestExisting(input, l, 'nobody', 'down')).toEqual([])
  })
})
