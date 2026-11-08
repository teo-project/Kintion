import { describe, expect, it } from 'vitest'

import { family } from './__tests__/family'
import { jointLines, litSiblings, upLines } from './highlight'
import { layoutTree } from './layout'

describe('подсветка линий вверх (F-127)', () => {
  const l = layoutTree(family(), { depth: 10 })
  const busTo = (child: string) => l.edges.filter(e => e.type === 'bus' && e.target === child)
  const straightAt = (node: string) => l.edges.filter(e => e.type === 'straight' && (e.source === node || e.target === node)).map(e => e.id)

  it('линия ребёнка — она и линии брака к родителям, без других детей', () => {
    const e = busTo('andrei')[0]!
    const set = upLines(l, e.id)
    expect([...set].sort()).toEqual([e.id, ...straightAt('u-sergei2')].sort())
    expect(set.has(busTo('natalia')[0]!.id)).toBe(false)
  })

  it('родители без отношения — обе линии к ребёнку', () => {
    const both = busTo('pavel')
    expect(both).toHaveLength(2)
    expect([...upLines(l, both[0]!.id)].sort()).toEqual(both.map(e => e.id).sort())
  })

  it('линия брака — линии к обоим участникам, дети не подсвечиваются', () => {
    const lines = straightAt('u-sergei2')
    const set = upLines(l, lines[0]!)
    expect([...set].sort()).toEqual([...lines].sort())
    expect(set.has(busTo('andrei')[0]!.id)).toBe(false)
  })

  it('неизвестная линия — пусто', () => {
    expect(upLines(l, 'nope').size).toBe(0)
  })

  it('«Брат / сестра»: линия или плашка — все линии к участникам и сама плашка', () => {
    const g = layoutTree(family(), { depth: 10 })
    expect(g.nodes.some(n => n.type === 'siblings' && n.id === 'sib-top')).toBe(true)
    const lines = g.edges.filter(e => e.source === 'sib-top').map(e => e.id).sort()
    expect(lines.length).toBeGreaterThan(1)
    expect([...upLines(g, lines[0]!)].sort()).toEqual(lines)
    expect([...jointLines(g, 'sib-top')].sort()).toEqual(lines)
    expect(litSiblings(g, lines[1]!, null)).toBe('sib-top')
    expect(litSiblings(g, null, 'sib-top')).toBe('sib-top')
    expect(litSiblings(g, null, null)).toBeNull()
  })
})
