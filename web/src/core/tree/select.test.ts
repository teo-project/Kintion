import { describe, expect, it } from 'vitest'

import { family } from './__tests__/family'
import { layoutTree } from './layout'
import { personsInBox, toggle } from './select'

describe('выделение в дереве (F-131)', () => {
  const l = layoutTree(family(), { depth: 99 })
  const node = (id: string) => l.nodes.find(n => n.type === 'person' && n.person === id)!

  it('рамка — люди, чьи узлы она задевает; направление рамки не важно', () => {
    const a = node('andrei')
    const box = { x0: a.x + a.w - 2, y0: a.y + 10, x1: a.x + 5, y1: a.y + 20 }
    expect(personsInBox(l, box)).toEqual(['andrei'])
    expect(personsInBox(l, { x0: -1e6, y0: -1e6, x1: 1e6, y1: 1e6 }).sort()).toEqual(family().persons.map(p => p.id).sort())
    expect(personsInBox(l, { x0: -1e6, y0: -1e6, x1: -1e6 + 1, y1: -1e6 + 1 })).toEqual([])
    // ниже отрисованного узла — мимо
    const under = { x0: a.x + 5, y0: a.y + 200, x1: a.x + 10, y1: a.y + 210 }
    expect(personsInBox(l, under, () => 150)).toEqual([])
    expect(personsInBox(l, under, () => 250)).toEqual(['andrei'])
  })

  it('Ctrl + клик — переключает', () => {
    const s = toggle(new Set(['a']), 'b')
    expect([...s]).toEqual(['a', 'b'])
    expect([...toggle(s, 'a')]).toEqual(['b'])
  })
})
