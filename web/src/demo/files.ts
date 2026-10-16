// Файлы демо-сборки: изображения в памяти страницы и их превью. Превью
// строится в canvas так же, как на сервере (server/internal/files/preview.go):
// без обрезки — центральный квадрат (rect — портрет 3:4), круг — с
// прозрачными углами, иначе — на белом. Адреса — data: URL, поэтому
// снимок страницы (design-export) открывается без сервера.
import type { Crop, PreviewShape } from '@/api/client'

interface Img {
  el: HTMLImageElement
  url: string
  width: number
  height: number
}

const images = new Map<string, Img>()
const previews = new Map<string, string>()

/** Загрузить изображение (data: URL) и запомнить под sha. */
export async function addImage(sha: string, url: string): Promise<{ width: number; height: number }> {
  const el = new Image()
  el.src = url
  await el.decode()
  const img = { el, url, width: el.naturalWidth, height: el.naturalHeight }
  images.set(sha, img)
  return img
}

export function fileUrl(sha: string): string {
  return images.get(sha)?.url ?? ''
}

interface Box {
  x: number
  y: number
  w: number
  h: number
}

/** Обрезка по размеру изображения; без неё — как Crop.Fit на сервере. */
function fit(c: Crop | null, shape: PreviewShape, width: number, height: number): Box {
  if (c && c.w != null) return { x: c.x ?? 0, y: c.y ?? 0, w: c.w, h: c.h ?? c.w }
  if (shape === 'rect') {
    let w = width
    let h = Math.floor((width * 4) / 3)
    if (h > height) {
      w = Math.floor((height * 3) / 4)
      h = height
    }
    w = Math.max(1, w)
    h = Math.max(1, h)
    return { x: Math.floor((width - w) / 2), y: Math.floor((height - h) / 2), w, h }
  }
  const side = Math.min(width, height)
  return { x: Math.floor((width - side) / 2), y: Math.floor((height - side) / 2), w: side, h: side }
}

export function previewUrl(sha: string, crop: Crop | null, shape: PreviewShape, size: number): string {
  const img = images.get(sha)
  if (!img) return ''
  const b = fit(crop, shape, img.width, img.height)
  const key = `${sha}_${shape}_${b.x}_${b.y}_${b.w}_${b.h}_${size}`
  const cached = previews.get(key)
  if (cached) return cached
  let ow = size
  let oh = size
  if (shape === 'rect' && b.w !== b.h) {
    if (b.w > b.h) oh = Math.max(1, Math.round((size * b.h) / b.w))
    else ow = Math.max(1, Math.round((size * b.w) / b.h))
  }
  const canvas = document.createElement('canvas')
  canvas.width = ow
  canvas.height = oh
  const g = canvas.getContext('2d')
  if (!g) return img.url
  if (shape === 'circle') {
    g.beginPath()
    g.arc(ow / 2, oh / 2, Math.min(ow, oh) / 2, 0, Math.PI * 2)
    g.clip()
  } else {
    g.fillStyle = 'white' // как JPEG сервера: прозрачное — на белом
    g.fillRect(0, 0, ow, oh)
  }
  g.drawImage(img.el, b.x, b.y, b.w, b.h, 0, 0, ow, oh)
  const url = canvas.toDataURL(shape === 'circle' ? 'image/png' : 'image/jpeg', 0.85)
  previews.set(key, url)
  return url
}
