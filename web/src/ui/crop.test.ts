import { describe, expect, it } from 'vitest'

import { centerCrop, clampCrop, cropMeta, cropShapes, cropsOf, fullCrops, previewOf, shapeOf } from './crop'

describe('обрезки по местам показа (F-79)', () => {
  it('центр и границы: квадрат, портрет, свободные пропорции', () => {
    expect(centerCrop(400, 300, 1, 'circle')).toEqual({ x: 50, y: 0, w: 300, h: 300, shape: 'circle' })
    expect(centerCrop(400, 300, 3 / 4, 'rect')).toEqual({ x: 87, y: 0, w: 225, h: 300, shape: 'rect' })
    expect(clampCrop({ x: 390, y: -5, w: 500, h: 500, shape: 'square' }, 400, 300, 1)).toEqual({ x: 100, y: 0, w: 300, h: 300, shape: 'square' })
    expect(clampCrop({ x: 10.4, y: 20.6, w: 0, h: 0, shape: 'square' }, 400, 300, 1)).toEqual({ x: 10, y: 21, w: 1, h: 1, shape: 'square' })
    expect(clampCrop({ x: 0, y: 0, w: 120, h: 400, shape: 'rect' }, 400, 300, 3 / 4)).toEqual({ x: 0, y: 0, w: 120, h: 160, shape: 'rect' })
    expect(clampCrop({ x: 0, y: 0, w: 380, h: 100, shape: 'rect' }, 400, 300, null)).toEqual({ x: 0, y: 0, w: 380, h: 100, shape: 'rect' })
  })

  it('недостающие — начальные: дерево и карточка — портрет, сеть — квадрат', () => {
    const all = fullCrops({ card: { x: 0, y: 0, w: 400, h: 100 } }, 400, 300, 'circle')
    expect(all.card).toEqual({ x: 0, y: 0, w: 400, h: 100, shape: 'rect' })
    expect(all.tree).toEqual({ x: 87, y: 0, w: 225, h: 300, shape: 'rect' })
    expect(all.network).toEqual({ x: 50, y: 0, w: 300, h: 300, shape: 'circle' })
  })

  it('старая обрезка (один квадрат) — обрезка сети; превью по месту', () => {
    expect(cropsOf({ x: 1, y: 2, w: 30, shape: 'circle' })).toEqual({ network: { x: 1, y: 2, w: 30, h: 30, shape: 'circle' } })
    const meta = { card: { x: 0, y: 0, w: 40, h: 60 }, network: { x: 5, y: 5, w: 20, h: 20, shape: 'circle' } }
    expect(previewOf(meta, 'card')).toEqual({ crop: { x: 0, y: 0, w: 40, h: 60, shape: 'rect' }, shape: 'rect' })
    expect(previewOf(meta, 'tree')).toEqual({ crop: null, shape: 'rect' }) // серверный портрет по центру
    expect(previewOf(meta, 'network', 'square').shape).toBe('circle')
  })

  it('формы сети — из объявления meta, неизвестные отбрасываются', () => {
    const m = cropMeta({ meta: [{ key: 'c', name: 'Обрезка', editor: 'crop', shapes: ['circle', 'hexagon'] }] })
    expect(m?.key).toBe('c')
    expect(cropShapes(m)).toEqual(['circle'])
    expect(cropShapes(undefined)).toEqual(['square'])
    expect(shapeOf({ shape: 'circle' }, 'square')).toBe('circle')
    expect(shapeOf({ x: 1 }, 'circle')).toBe('circle')
  })
})
