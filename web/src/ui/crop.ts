// Обрезки изображения (F-72, F-79): своя под каждое место показа, все — в
// meta значения фото (`meta.crop`), исходник один:
// - дерево — портретная (3:4);
// - сеть — квадрат; круг — маской (PNG с прозрачными углами), форма — из
//   `shapes` объявления meta с редактором `crop`;
// - карточка — произвольная, начальная — портретная.
// Координаты — в пикселях исходника с учётом поворота по EXIF.
// Старая обрезка (один квадрат `{ x, y, w, h, shape }`) читается как обрезка сети.
import type { Crop, PreviewShape } from '@/api/client'
import type { FieldDef, MetaDef } from '@/core/types'

/** Редактор meta обрезки в шаблоне. */
export const CROP_EDITOR = 'crop'
/** Формы обрезки сети, известные интерфейсу. */
const NETWORK_SHAPES: PreviewShape[] = ['square', 'circle']

/** Место показа фото (F-79). */
export type Place = 'card' | 'tree' | 'network'
/** Порядок в редакторе: сначала карточка. */
export const PLACES: Place[] = ['card', 'tree', 'network']
/** Портрет: ширина / высота. */
export const PORTRAIT = 3 / 4
/** Пропорции места (ширина / высота); null — свободные (карточка). */
export const ASPECT: Record<Place, number | null> = { card: null, tree: PORTRAIT, network: 1 }

/** Обрезка места: прямоугольник в пикселях исходника и форма превью. */
export interface PlaceCrop {
  x: number
  y: number
  w: number
  h: number
  shape: PreviewShape
}
export type Crops = Record<Place, PlaceCrop>

/** Объявление обрезки в поле (`meta` с `editor: crop`); нет — обрезку не пишем (F-12). */
export function cropMeta(def: Pick<FieldDef, 'meta'>): MetaDef | undefined {
  return def.meta?.find(m => m.editor === CROP_EDITOR)
}

/** Места показа из объявления (`places`, F-96: своё изображение — только карточка); нет — все. */
export function cropPlaces(m: MetaDef | undefined): Place[] {
  const list = Array.isArray(m?.places) ? m.places.filter((p): p is Place => PLACES.includes(p as Place)) : []
  return list.length ? list : PLACES
}

/** Формы обрезки сети из объявления; известные интерфейсу — квадрат и круг. */
export function cropShapes(m: MetaDef | undefined): PreviewShape[] {
  const list = Array.isArray(m?.shapes) ? m.shapes.filter((s): s is PreviewShape => NETWORK_SHAPES.includes(s as PreviewShape)) : []
  return list.length ? list : ['square']
}

function isRect(v: unknown): v is Crop {
  return !!v && typeof v === 'object' && typeof (v as Crop).w === 'number' && typeof (v as Crop).x === 'number'
}

const SHAPES: PreviewShape[] = ['square', 'circle', 'rect']

function asCrop(v: Crop, shape: PreviewShape): PlaceCrop {
  const sh = SHAPES.includes(v.shape as PreviewShape) ? (v.shape as PreviewShape) : shape
  return { x: v.x ?? 0, y: v.y ?? 0, w: v.w ?? 1, h: v.h ?? v.w ?? 1, shape: sh }
}

/** Форма, сохранённая в обрезке сети (старая обрезка — её же форма); нет — undefined. */
function storedNetworkShape(meta: unknown): unknown {
  if (!meta || typeof meta !== 'object') return undefined
  const v = 'network' in meta ? (meta as Record<string, unknown>).network : meta
  return v && typeof v === 'object' ? (v as Crop).shape : undefined
}

/** Сохранённые обрезки по местам; старый квадрат — обрезка сети. */
export function cropsOf(meta: unknown): Partial<Crops> {
  if (!meta || typeof meta !== 'object') return {}
  if (isRect(meta)) return { network: asCrop(meta, 'square') }
  const out: Partial<Crops> = {}
  for (const p of PLACES) {
    const v = (meta as Record<string, unknown>)[p]
    if (isRect(v)) out[p] = asCrop(v, p === 'network' ? 'square' : 'rect')
  }
  return out
}

/** Превью для места: обрезка (null — серверная по умолчанию) и форма. */
export function previewOf(meta: unknown, place: Place, fallback: PreviewShape = 'square'): { crop: PlaceCrop | null; shape: PreviewShape } {
  const c = cropsOf(meta)[place] ?? null
  if (place !== 'network') return { crop: c, shape: 'rect' }
  const stored = storedNetworkShape(meta)
  const shape = NETWORK_SHAPES.includes(stored as PreviewShape) ? (stored as PreviewShape) : fallback
  return { crop: c, shape }
}

/** Форма показа сохранённой обрезки сети; нет — по умолчанию. */
export function shapeOf(meta: unknown, fallback: PreviewShape): PreviewShape {
  return previewOf(meta, 'network', fallback).shape
}

/** Наибольший прямоугольник пропорций aspect по центру. */
export function centerCrop(width: number, height: number, aspect: number, shape: PreviewShape): PlaceCrop {
  let w = width
  let h = Math.round(w / aspect)
  if (h > height) {
    h = height
    w = Math.round(h * aspect)
  }
  w = Math.max(1, Math.min(w, width))
  h = Math.max(1, Math.min(h, height))
  return { x: Math.floor((width - w) / 2), y: Math.floor((height - h) / 2), w, h, shape }
}

/** Начальная обрезка места: дерево и карточка — портрет, сеть — квадрат. */
export function defaultCrop(place: Place, width: number, height: number, networkShape: PreviewShape = 'square'): PlaceCrop {
  return centerCrop(width, height, ASPECT[place] ?? PORTRAIT, place === 'network' ? networkShape : 'rect')
}

/** Все обрезки: сохранённые, недостающие — начальные. */
export function fullCrops(meta: unknown, width: number, height: number, networkShape: PreviewShape = 'square'): Crops {
  const have = cropsOf(meta)
  const out = {} as Crops
  for (const p of PLACES) {
    const c = have[p]
    out[p] = c ? clampCrop(c, width, height, ASPECT[p]) : defaultCrop(p, width, height, networkShape)
  }
  return out
}

/**
 * Обрезка внутри изображения, целые пиксели. aspect задан — пропорции
 * сохраняются (квадрат: w = h), null — свободные.
 */
export function clampCrop(c: PlaceCrop, width: number, height: number, aspect: number | null): PlaceCrop {
  let w = Math.max(1, Math.min(Math.round(c.w), width))
  let h = Math.max(1, Math.min(Math.round(c.h), height))
  if (aspect != null) {
    h = Math.round(w / aspect)
    if (h > height) {
      h = height
      w = Math.round(h * aspect)
    }
    if (aspect === 1) w = h = Math.min(w, h)
  }
  const x = Math.max(0, Math.min(Math.round(c.x), width - w))
  const y = Math.max(0, Math.min(Math.round(c.y), height - h))
  return { x, y, w, h, shape: c.shape }
}
