import { describe, expect, it } from 'vitest'

import { family } from './__tests__/family'
import { BADGE_D, BADGE_GAP, placeBadges, refBadges, type Entry, type Placed, type Target } from './badges'
import { layoutTree, PERSON_W, PHOTO_MID } from './layout'

const W = PERSON_W
/** входы линий как у обычной карточки: верх, низ, оба бока */
const lines = (w: number, h: number): Entry[] => [
  { x: w / 2, y: 0, nx: 0, ny: -1 },
  { x: w / 2, y: h, nx: 0, ny: 1 },
  { x: 0, y: PHOTO_MID, nx: -1, ny: 0 },
  { x: w, y: PHOTO_MID, nx: 1, ny: 0 },
]

/** значки не перекрываются, не лежат на линиях и не заходят внутрь карточки дальше, чем наполовину */
function check(ps: Placed[], w: number, h: number, entries: Entry[]) {
  for (let i = 0; i < ps.length; i++) {
    const a = ps[i]!
    for (let j = i + 1; j < ps.length; j++) {
      const b = ps[j]!
      expect(Math.hypot(a.x - b.x, a.y - b.y), `${a.id} / ${b.id}`).toBeGreaterThanOrEqual(BADGE_D + BADGE_GAP - 1e-6)
    }
    for (const e of entries) {
      const t = Math.max(0, (a.x - e.x) * e.nx + (a.y - e.y) * e.ny)
      expect(Math.hypot(a.x - e.x - e.nx * t, a.y - e.y - e.ny * t), `${a.id} на линии`).toBeGreaterThanOrEqual(BADGE_D / 2 + BADGE_GAP - 1e-6)
    }
    // центр — на краю или снаружи, не внутри карточки
    const inside = a.x > 1e-6 && a.x < w - 1e-6 && a.y > 1e-6 && a.y < h - 1e-6
    expect(inside, `${a.id} внутри`).toBe(false)
  }
}

const fan = (n: number, dx: (i: number) => number, dy: (i: number) => number): Target[] =>
  Array.from({ length: n }, (_, i) => ({ id: `r${i}`, dx: dx(i), dy: dy(i) }))

describe('размещение значков отсылок (F-117)', () => {
  it('одна копия справа в том же ряду — на правом краю, со стороны копии', () => {
    const [p] = placeBadges(W, 266, [{ id: 'a', dx: 500, dy: 0 }], lines(W, 266))
    expect(p!.x).toBe(W)
    expect(p!.y).toBeCloseTo(133, 0)
  })

  it('копия выше — на верхнем краю, но не на входе линии', () => {
    const [p] = placeBadges(W, 266, [{ id: 'a', dx: 0, dy: -400 }], lines(W, 266))
    expect(p!.y).toBe(0)
    expect(Math.abs(p!.x - W / 2)).toBeGreaterThanOrEqual(BADGE_D / 2 + BADGE_GAP)
  })

  it('много копий с одной стороны — без перекрытий, переходят за угол', () => {
    const ps = placeBadges(W, 266, fan(20, () => 600, i => i - 10), lines(W, 266))
    expect(ps).toHaveLength(20)
    check(ps, W, 266, lines(W, 266))
    expect(ps.some(p => p.x !== W)).toBe(true)
  })

  it('углы: копии по диагонали вверх-влево и вверх-вправо не перекрываются', () => {
    const ts = [...fan(4, () => -300, () => -300), ...fan(4, () => 300, () => -300).map(t => ({ ...t, id: `q${t.id}` })), { id: 'top', dx: -10, dy: -500 }, { id: 'left', dx: -500, dy: -100 }]
    const ps = placeBadges(W, 266, ts, lines(W, 266))
    expect(ps).toHaveLength(ts.length)
    check(ps, W, 266, lines(W, 266))
  })

  it('низкая карточка: значки сбоку не свисают ниже края', () => {
    const h = 120
    const ps = placeBadges(W, h, fan(6, () => 400, i => 40 + i), lines(W, h))
    check(ps, W, h, lines(W, h))
    for (const p of ps) expect(p.y).toBeLessThanOrEqual(h)
  })

  it('узкий и широкий узел', () => {
    for (const w of [40, 600]) {
      const ps = placeBadges(w, 200, fan(12, i => Math.cos(i) * 500, i => Math.sin(i) * 500), lines(w, 200))
      expect(ps).toHaveLength(12)
      check(ps, w, 200, lines(w, 200))
    }
  })

  it('весь край занят — следующее кольцо снаружи, без перекрытий', () => {
    const h = 60
    const ps = placeBadges(40, h, fan(40, i => Math.cos(i / 6) * 300, i => Math.sin(i / 6) * 300), lines(40, h))
    expect(ps).toHaveLength(40)
    check(ps, 40, h, lines(40, h))
    expect(ps.some(p => p.x < -1 || p.y < -1 || p.x > 41 || p.y > h + 1)).toBe(true)
  })

  it('по раскладке: на каждую отсылку основной карточки — свой значок, по настоящей высоте', () => {
    const l = layoutTree(family(), { depth: 10 })
    const h = 266
    const all = refBadges(l, () => ({ w: W, h }))
    const mains = new Set(l.nodes.filter(n => n.type === 'person').map(n => n.person))
    const expected = l.nodes.filter(n => n.type === 'ref' && mains.has(n.person)).map(n => n.id).sort()
    expect(expected.length).toBeGreaterThan(0)
    expect([...all.values()].flat().map(b => b.id).sort()).toEqual(expected)
    for (const list of all.values()) {
      check(list, W, h, [])
      for (const b of list) expect(b.y).toBeLessThanOrEqual(h)
    }
  })
})

describe('значки остаются со стороны копии', () => {
  it('много копий справа — ни одного значка на левой стороне', () => {
    const ps = placeBadges(W, 266, fan(30, () => 600, i => i * 3 - 45), lines(W, 266))
    expect(ps).toHaveLength(30)
    check(ps, W, 266, lines(W, 266))
    for (const p of ps) expect(p.x, p.id).toBeGreaterThan(W / 2 - BADGE_D)
    expect(ps.some(p => p.x > W + 1)).toBe(true)
  })
})
