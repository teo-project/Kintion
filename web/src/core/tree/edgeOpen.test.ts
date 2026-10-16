import { describe, expect, it } from 'vitest'

import { family } from './__tests__/family'
import { busSegments, edgeOpen } from './edgeOpen'
import { layoutTree } from './layout'

describe('двойной клик по линии (F-127)', () => {
  const l = layoutTree(family(), { depth: 10 })
  const busTo = (child: string) => l.edges.filter(e => e.type === 'bus' && e.target === child)

  it('линия от пары к ребёнку — меню из двух отношений, родные линии — по одному', () => {
    const e = busTo('andrei')[0]!
    const r = edgeOpen(l, e.id)
    expect(r.kind).toBe('menu')
    if (r.kind === 'menu') expect(r.ids).toHaveLength(2)
  })

  it('отдельная линия и линия от одного родителя — сразу карточка', () => {
    const singles = l.edges.filter(e => e.type === 'bus' && e.rels?.length === 1)
    expect(singles.length).toBeGreaterThan(0)
    for (const e of singles) expect(edgeOpen(l, e.id)).toEqual({ kind: 'card', id: e.rels![0] })
  })

  it('линия брака — карточка брака; неизвестная линия — ничего', () => {
    const s = l.edges.find(e => e.type === 'straight' && e.rels?.length)!
    expect(edgeOpen(l, s.id)).toEqual({ kind: 'card', id: s.rels![0] })
    expect(edgeOpen(l, 'nope')).toEqual({ kind: 'none' })
  })

  const node = (id: string) => l.nodes.find(n => n.id === id)!
  // карточки людей — по содержимому, плашки — своей высоты
  const h = (id: string) => (['person', 'ref'].includes(node(id).type) ? 266 : undefined)

  it('родители без пары: общий отрезок над ребёнком — меню из обоих отношений, свой — карточка', () => {
    const [ev, em] = [busTo('pavel').find(e => e.source === 'vera')!, busTo('pavel').find(e => e.source === 'mikhail')!]
    expect(ev && em).toBeTruthy()
    const p = node('pavel')
    const below = { x: p.x + p.w / 2 + 3, y: p.y - 6 }
    for (const e of [ev, em]) {
      const r = edgeOpen(l, e.id, below, h)
      expect(r.kind).toBe('menu')
      if (r.kind === 'menu') expect(r.ids.sort()).toEqual(['pc-mikhail', 'pc-vera'])
    }
    // собственная вертикаль Веры — без отношения Михаила (с ней может идти линия опеки Веры)
    const [x1, y1] = busSegments(l, ev, h)[0]!
    const own = edgeOpen(l, ev.id, { x: x1 + 2, y: y1 + 4 }, h)
    const ids = own.kind === 'menu' ? own.ids : own.kind === 'card' ? [own.id] : []
    expect(ids).toContain('pc-vera')
    expect(ids).not.toContain('pc-mikhail')
  })

  it('отчим: нижний отрезок к ребёнку общий с линией от пары', () => {
    const step = l.edges.find(e => e.id === 'l:pc-viktor')!
    const d = node('dmitry')
    const r = edgeOpen(l, step.id, { x: d.x + d.w / 2, y: d.y - 2 }, h)
    expect(r.kind).toBe('menu')
    if (r.kind === 'menu') expect(r.ids).toEqual(expect.arrayContaining(['pc-viktor', ...busTo('dmitry').find(e => e.id !== step.id)!.rels!]))
  })

  it('братья: общий ствол от пары к нескольким детям несёт отношения всех детей', () => {
    const kids = l.edges.filter(e => e.type === 'bus' && e.source === 'u-dmitry')
    expect(kids.length).toBe(2)
    const [x, y1, , y2] = busSegments(l, kids[0]!, h)[0]!
    expect(y1).toBe(node('u-dmitry').y + node('u-dmitry').h!)
    const r = edgeOpen(l, kids[0]!.id, { x, y: (y1 + y2) / 2 }, h)
    expect(r.kind).toBe('menu')
    if (r.kind === 'menu') expect(r.ids.sort()).toEqual([...kids[0]!.rels!, ...kids[1]!.rels!].sort())
  })

  it('пересечение линий под прямым углом — не общий отрезок', () => {
    const buses = l.edges.filter(e => e.type === 'bus' && e.rels?.length)
    let checked = 0
    for (const a of buses) {
      for (const sa of busSegments(l, a, h)) {
        if (sa[0] !== sa[2]) continue
        for (const b of buses) {
          if (b === a || b.rels!.some(r => a.rels!.includes(r))) continue
          for (const sb of busSegments(l, b, h)) {
            if (sb[1] !== sb[3]) continue
            const [x, y] = [sa[0], sb[1]]
            const inA = y > Math.min(sa[1], sa[3]) + 2 && y < Math.max(sa[1], sa[3]) - 2
            const inB = x > Math.min(sb[0], sb[2]) + 2 && x < Math.max(sb[0], sb[2]) - 2
            if (!inA || !inB) continue
            // точка — на пересечении; клик по вертикали a чуть выше — без отношений b
            const r = edgeOpen(l, a.id, { x, y }, h)
            const ids = r.kind === 'menu' ? r.ids : r.kind === 'card' ? [r.id] : []
            if (busSegments(l, b, h).some(s => s[0] === s[2] && Math.abs(s[0] - x) < 1.5 && y >= Math.min(s[1], s[3]) && y <= Math.max(s[1], s[3]))) continue
            expect(ids.some(i => b.rels!.includes(i)), `${a.id} × ${b.id}`).toBe(false)
            checked++
          }
        }
      }
    }
    expect(checked).toBeGreaterThan(0)
  })
})
