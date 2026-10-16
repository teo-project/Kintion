import { describe, expect, it } from 'vitest'

import { ctxOf, ruSchema, type RecSpec } from '../__tests__/kit'
import { family } from './__tests__/family'
import { personView, treeInput } from './graph'
import { labelBox, labelLines, layoutTree, PERSON_W } from './layout'
import type { TreeLayout, TreeNode } from './model'

const people = (l: TreeLayout) => l.nodes.filter(n => n.type === 'person')
const nodeOf = (l: TreeLayout, person: string) => l.nodes.find(n => n.type === 'person' && n.person === person)!
const rowOf = (l: TreeLayout, person: string) => nodeOf(l, person).row

/** узлы одного ряда не перекрываются */
function noOverlap(l: TreeLayout) {
  const byRow = new Map<number, TreeNode[]>()
  for (const n of l.nodes) if (n.type !== 'siblings') (byRow.get(n.row) ?? byRow.set(n.row, []).get(n.row)!).push(n)
  for (const [r, ns] of byRow) {
    const s = [...ns].sort((a, b) => a.x - b.x)
    for (let i = 1; i < s.length; i++) expect(s[i]!.x, `ряд ${r}: ${s[i - 1]!.id} / ${s[i]!.id}`).toBeGreaterThanOrEqual(s[i - 1]!.x + s[i - 1]!.w)
  }
}

describe('раскладка дерева', () => {
  const l = layoutTree(family(), { depth: 10 })

  it('каждый человек — один раз, узлы не перекрываются', () => {
    const ids = people(l).map(n => n.person)
    expect(new Set(ids).size).toBe(ids.length)
    expect(ids.length).toBe(family().persons.length)
    noOverlap(l)
  })

  it('ряды: дети ниже родителей, поздний ребёнок — на ряд ниже (F-115), супруги — в одном ряду', () => {
    expect(rowOf(l, 'ivan')).toBe(rowOf(l, 'pyotr') + 1)
    expect(rowOf(l, 'nikolai')).toBe(rowOf(l, 'pyotr') + 2)
    expect(rowOf(l, 'tamara')).toBe(rowOf(l, 'sergei'))
    expect(rowOf(l, 'semyon')).toBe(rowOf(l, 'tamara') - 1)
    expect(rowOf(l, 'daria')).toBe(rowOf(l, 'artyom'))
    expect(rowOf(l, 'lev')).toBe(Math.max(...people(l).map(n => n.row)))
  })

  it('дети висят на узле пары; без отношения — линии от обоих родителей на одной высоте (F-112)', () => {
    const toKirill = l.edges.filter(e => e.target === 'kirill')
    expect(toKirill.map(e => e.source)).toEqual(['u-nik2'])
    const toPavel = l.edges.filter(e => e.target === 'pavel' && e.type === 'bus')
    expect(toPavel.map(e => e.source).sort()).toEqual(['mikhail', 'vera'])
    expect(toPavel[0]!.busY).toBe(toPavel[1]!.busY)
    expect(l.nodes.some(n => n.type === 'union' && n.relation?.startsWith('gap:'))).toBe(false)
  })

  it('петля — отсылка «↪», брак нарисован один раз (F-117)', () => {
    const refs = l.nodes.filter(n => n.type === 'ref')
    expect(refs.map(n => n.person)).toContain('nikolai')
    expect(l.nodes.filter(n => n.id === 'u-nik2')).toHaveLength(1)
  })

  it('несколько браков — раньше левее (F-112)', () => {
    const x = (id: string) => l.nodes.find(n => n.id === id)!.x
    expect(x('u-sergei1')).toBeLessThan(nodeOf(l, 'sergei').x)
    expect(x('u-sergei2')).toBeGreaterThan(nodeOf(l, 'sergei').x)
  })

  it('пунктир и подписи: приёмная, отчим, опекун (F-118)', () => {
    const sonya = l.edges.find(e => e.target === 'sonya')!
    expect(sonya.dashed).toBe(true)
    expect(sonya.label).toBe('Приёмный')
    const step = l.edges.find(e => e.id === 'l:pc-viktor')!
    expect(step).toMatchObject({ source: 'viktor', target: 'dmitry', dashed: true, label: 'Отчим' })
    expect(l.edges.find(e => e.target === 'maksim')!.dashed).toBeUndefined()
  })

  it('«Брат / сестра» без родителей — узел над всеми (без заглушки-родителя, F-42)', () => {
    const sib = l.nodes.find(n => n.type === 'siblings')!
    expect(l.edges.filter(e => e.source === sib.id).map(e => e.target).sort()).toEqual(['fyodor', 'maria', 'pyotr'])
  })

  it('предки из других родов — над ребёнком', () => {
    const cx = (n: TreeNode) => n.x + n.w / 2
    const semyon = nodeOf(l, 'semyon')
    const tamara = nodeOf(l, 'tamara')
    expect(Math.abs(cx(semyon) - cx(tamara))).toBeLessThan(PERSON_W * 3)
  })

  it('линия к позднему ребёнку обходит промежуточный ряд (F-110)', () => {
    const e = l.edges.find(x => x.target === 'nikolai')!
    expect(e.via).toBeDefined()
    const mid = l.nodes.filter(n => n.row === rowOf(l, 'ivan'))
    expect(mid.every(n => e.via!.x <= n.x - 8 || e.via!.x >= n.x + n.w + 8)).toBe(true)
  })

  it('глубина — вверх от самого молодого ряда (F-110)', () => {
    const d3 = layoutTree(family(), { depth: 3 })
    const rows = new Set(people(d3).map(n => n.row))
    expect(rows.size).toBe(3)
    expect(people(d3).map(n => n.person)).toContain('lev')
    expect(people(d3).map(n => n.person)).not.toContain('sergei')
    noOverlap(d3)
  })

  it('«Скрыть умерших»: живые — все, скрытые — только развилки между живыми (F-119)', () => {
    const h = layoutTree(family(), { depth: 10, hideDead: true })
    const alive = family().persons.filter(p => !p.dead).map(p => p.id).sort()
    expect(people(h).map(n => n.person).sort()).toEqual(alive)
    const dots = h.nodes.filter(n => n.type === 'dot')
    expect(dots.length).toBeGreaterThan(0)
    for (const d of dots) {
      const touching = h.edges.filter(e => e.source === d.id || e.target === d.id)
      expect(touching.length, d.id).toBeGreaterThanOrEqual(1)
    }
    noOverlap(h)
  })

  it('родной и неродной без пары: неродной — отдельной пунктирной линией (F-118)', () => {
    const l2 = layoutTree({
      persons: [{ id: 'm', year: 1980 }, { id: 's', year: 2010 }, { id: 'k', year: 2010 }],
      couples: [],
      siblings: [],
      descents: [
        { id: 'd1', parent: 'm', child: 'k' },
        { id: 'd2', parent: 's', child: 'k', dashed: true, label: 'Отчим' },
      ],
    }, { depth: 10 })
    expect(l2.edges.find(e => e.id === 'l:d2')).toMatchObject({ source: 's', dashed: true, label: 'Отчим' })
    expect(l2.edges.find(e => e.source === 'm' && e.target === 'k')!.dashed).toBeUndefined()
    const row = (id: string) => l2.nodes.find(n => n.person === id)!.row
    expect(row('s')).toBeLessThan(row('k'))
  })

  it('родитель отдельной линией с другим ребёнком — всё равно выше (F-115)', () => {
    const l3 = layoutTree({
      persons: [{ id: 'a', year: 1942 }, { id: 'b', year: 1962 }, { id: 'c', year: 1980 }, { id: 'd', year: 2009 }],
      couples: [],
      siblings: [],
      descents: [
        { id: 'ac', parent: 'a', child: 'c' },
        { id: 'bc', parent: 'b', child: 'c', dashed: true },
        { id: 'bd', parent: 'b', child: 'd' },
      ],
    }, { depth: 10 })
    const row = (id: string) => l3.nodes.find(n => n.person === id)!.row
    expect(row('b')).toBeLessThan(row('c'))
    expect(row('b')).toBeLessThan(row('d'))
    expect(row('a')).toBeLessThan(row('c'))
  })

  it('случайные графы: родитель всегда выше ребёнка, узлы не перекрываются', () => {
    // разреженные и плотные (несколько браков, отдельные линии, нет годов) — Q-36
    for (const [seed0, rounds, density, coupleOdds] of [[7, 300, 1.5, 3], [11, 1500, 2, 2]] as const) {
      let seed: number = seed0
      const rnd = (n: number) => (seed = (seed * 16807) % 2147483647) % n
      for (let round = 0; round < rounds; round++) {
        const n = 4 + rnd(12)
        const ids = Array.from({ length: n }, (_, i) => `p${i}`)
        const input = {
          persons: ids.map((id, i) => ({ id, year: rnd(6) === 0 ? undefined : 1900 + i * 7 + rnd(5), dead: rnd(3) === 0 })),
          couples: [] as { id: string; a: string; b: string; order: string; label: string }[],
          siblings: [],
          descents: [] as { id: string; parent: string; child: string; dashed?: boolean }[],
        }
        for (let k = 0; k < n * density; k++) {
          const a = rnd(n)
          const b = rnd(n)
          if (a === b) continue
          if (rnd(coupleOdds) === 0) input.couples.push({ id: `c${k}`, a: ids[a]!, b: ids[b]!, order: String(k).padStart(3, '0'), label: 'Брак' })
          else if (a < b) input.descents.push({ id: `d${k}`, parent: ids[a]!, child: ids[b]!, dashed: rnd(4) === 0 })
        }
        for (const hideDead of [false, true]) {
          const l3 = layoutTree(input, { depth: 99, hideDead })
          noOverlap(l3)
          const nodeIds = l3.nodes.map(x => x.id)
          expect(new Set(nodeIds).size, `${seed0}/${round}: id узлов`).toBe(nodeIds.length)
          // прямая линия пары — только к её участнику
          const byId = new Map(l3.nodes.map(x => [x.id, x]))
          for (const e of l3.edges.filter(x => x.type === 'straight')) {
            const [u, p] = [byId.get(e.source)!, byId.get(e.target)!].sort(x => (x.type === 'union' ? -1 : 1))
            const c = input.couples.find(x => x.id === u!.id)
            if (c && p!.person && !p!.person.startsWith('j:')) expect([c.a, c.b], `${seed0}/${round}: ${e.id}`).toContain(p!.person)
          }
          if (hideDead) continue
          const row = (id: string) => l3.nodes.find(x => x.type === 'person' && x.person === id)?.row
          for (const d of input.descents) {
            const pr = row(d.parent)
            const cr = row(d.child)
            if (pr !== undefined && cr !== undefined) expect(pr, `${seed0}/${round}: ${d.parent} → ${d.child}`).toBeLessThan(cr)
          }
        }
      }
    }
  }, 30_000) // 1800 графов — дольше таймаута по умолчанию под нагрузкой

  it('петля из пар A–B, B–C, A–C: каждый узел отношения — один раз (Q-36)', () => {
    const tri = layoutTree({
      persons: [{ id: 'A', year: 1950 }, { id: 'B', year: 1951 }, { id: 'C', year: 1952 }], siblings: [], descents: [],
      couples: [{ id: 'ab', a: 'A', b: 'B', order: '1', label: 'Брак' }, { id: 'bc', a: 'B', b: 'C', order: '2', label: 'Брак' }, { id: 'ac', a: 'A', b: 'C', order: '3', label: 'Брак' }],
    }, { depth: 10 })
    const ids = tri.nodes.map(n => n.id)
    expect(new Set(ids).size).toBe(ids.length)
    expect(tri.nodes.filter(n => n.type === 'union').map(n => n.id).sort()).toEqual(['ab', 'ac', 'bc'])
    noOverlap(tri)
  })

  it('пара, поставленная позже, связана с обоими супругами', () => {
    for (const n of l.nodes.filter(x => x.type === 'union')) {
      const touching = l.edges.filter(e => e.type === 'straight' && (e.source === n.id || e.target === n.id))
      expect(touching.length, n.id).toBe(2)
    }
  })

  it('пустая модель', () => {
    expect(layoutTree({ persons: [], couples: [], siblings: [], descents: [] }, { depth: 10 })).toEqual({ nodes: [], edges: [] })
  })
})

describe('модель дерева из записей kin.core', () => {
  const person = (id: string, gender: string, extra: Record<string, unknown> = {}) => ({
    id, template: 'kin.core.person', fields: { name: [id], gender: [gender] }, ...extra,
  })
  const ctx = ctxOf(ruSchema(), [
    person('ivan', 'male', { start: { y: 1950 } }),
    person('anna', 'female', { end: { y: 2001 } }),
    person('kid', 'female'),
    person('step', 'male'),
    person('flirt', 'male'),
    { id: 'm', template: 'kin.core.relationship', fields: { stage: ['marriage'] }, parts: [['partner', 'ivan'], ['partner', 'anna']] },
    { id: 'f', template: 'kin.core.relationship', fields: { stage: ['flirt'] }, parts: [['partner', 'anna'], ['partner', 'flirt']] },
    { id: 'p1', template: 'kin.core.parent_child', parts: [['parent', 'ivan'], ['child', 'kid']] },
    { id: 'p2', template: 'kin.core.parent_child', fields: { kind: ['step'] }, parts: [['parent', 'step'], ['child', 'kid']] },
    { id: 's', template: 'kin.core.sibling', parts: [['sibling', 'ivan'], ['sibling', 'step']] },
    { id: 'w', template: 'kin.core.friend', parts: [['friend', 'ivan'], ['friend', 'flirt']] },
  ])
  const all = Object.values(ctx.records).filter(r => r !== undefined)
  const input = treeInput(ctx, all.filter(r => r.entity !== 'kin.core.person'), all.filter(r => r.entity === 'kin.core.person'))

  it('по tree_link: пара на любом этапе (F-120), родитель — ребёнок, брат / сестра; дружба — нет', () => {
    expect(input.couples.map(c => c.id)).toEqual(['m', 'f'])
    expect(input.couples[0]!.label).toBe('Брак')
    expect(input.siblings.map(s => s.id)).toEqual(['s'])
    expect(input.descents.map(d => [d.id, d.parent, d.child, !!d.dashed])).toEqual([['p1', 'ivan', 'kid', false], ['p2', 'step', 'kid', true]])
    expect(input.descents[1]).toMatchObject({ kind: 'Неродной', label: 'Отчим' })
    expect(input.persons.map(p => p.id).sort()).toEqual(['anna', 'flirt', 'ivan', 'kid', 'step'])
    expect(input.persons.find(p => p.id === 'ivan')).toMatchObject({ year: 1950, dead: false })
    expect(input.persons.find(p => p.id === 'anna')!.dead).toBe(true)
  })

  it('пара из трёх и более: каждый узел отношения открывает его карточку (Q-36)', () => {
    const c3 = ctxOf(ruSchema(), [
      person('a', 'male'), person('b', 'female'), person('c', 'female'),
      { id: 'tri', template: 'kin.core.relationship', fields: { stage: ['marriage'] }, parts: [['partner', 'a'], ['partner', 'b'], ['partner', 'c']] },
    ])
    const recs = Object.values(c3.records).filter(r => r !== undefined)
    const inp = treeInput(c3, recs.filter(r => r.entity !== 'kin.core.person'), recs.filter(r => r.entity === 'kin.core.person'))
    expect(inp.couples.map(c => [c.id, c.relation])).toEqual([['tri', 'tri'], ['tri:2', 'tri']])
    const unions = layoutTree(inp, { depth: 10 }).nodes.filter(n => n.type === 'union')
    expect(unions.map(n => n.relation)).toEqual(['tri', 'tri'])
  })

  it('ФИО в узле — полностью, с отчеством и вторым именем, если заполнены (F-123)', () => {
    const c = ctxOf(ruSchema(), [
      { id: 'x', template: 'kin.core.person', fields: { name: ['Иван'], patronymic: ['Петрович'], surname: ['Сидоров'] } },
      { id: 'y', template: 'kin.core.person', fields: { name: ['Анна'], middle_name: ['Мария'], surname: ['Петрова', 'Иванова'] } },
    ])
    expect(personView(c, c.records.x!).title).toBe('Иван Петрович Сидоров')
    expect(personView(c, c.records.y!).title).toBe('Анна Мария Петрова (Иванова)')
  })

  it('умерший — дата смерти или признак `end_flag` шаблона; ключа нет — только по дате (F-126)', () => {
    const schema = ruSchema()
    const tpl = schema.templates.find(x => x.key === 'kin.core.person')!.definition
    const recs: RecSpec[] = [
      { id: 'flag', template: 'kin.core.person', fields: { name: ['a'], deceased: [true] } },
      { id: 'date', template: 'kin.core.person', fields: { name: ['b'] }, end: { y: 1990 } },
      { id: 'alive', template: 'kin.core.person', fields: { name: ['c'], deceased: [false] } },
    ]
    delete tpl.end_flag
    const without = ctxOf(schema, recs)
    expect(['flag', 'date', 'alive'].map(id => personView(without, without.records[id]!).dead)).toEqual([false, true, false])
    tpl.end_flag = 'deceased'
    const withFlag = ctxOf(schema, recs)
    expect(['flag', 'date', 'alive'].map(id => personView(withFlag, withFlag.records[id]!).dead)).toEqual([true, true, false])
  })
})

describe('подписи на линиях (F-129)', () => {
  it('целиком в одну строку; длиннее 30 символов — две, без обрезки', () => {
    expect(labelLines('Брак')).toEqual(['Брак'])
    const long = 'Гражданский брак с венчанием в соборе'
    const two = labelLines(long)
    expect(two).toHaveLength(2)
    expect(two.join(' ')).toBe(long)
    const word = 'О'.repeat(35)
    expect(labelLines(word)).toHaveLength(2)
    expect(labelLines(word).join('')).toBe(word)
    expect(labelBox('Брак').w).toBeLessThan(labelBox('Незарегистрированный брак').w)
    expect(labelBox(long).h).toBeGreaterThan(labelBox('Брак').h)
  })
})

describe('завершённость записи (F-126)', () => {
  it('привязанный к событию конец без своей даты — тоже конец', () => {
    const c = ctxOf(ruSchema(), [
      { id: 'l', template: 'kin.core.person', fields: { name: ['a'] } },
    ])
    c.records.l!.end = { value: null, link: 'ev' } as never
    expect(personView(c, c.records.l!).dead).toBe(true)
  })
})
