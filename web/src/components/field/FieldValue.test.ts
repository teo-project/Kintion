import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import { storedRaw, valueIn } from '@/core/resolver'
import FieldValue from './FieldValue.vue'

const doc = (mime: string, name = 'Метрика.pdf', preview = false) => ({ id: 'v1', value: 'abc', meta: { file: { name, mime, size: 2048, preview } } })

function show(v: ReturnType<typeof doc>) {
  return mount(FieldValue, {
    props: { def: { type: 'file' }, value: v as never, text: v.meta.file.name, dashboard: 'd1' },
    global: { stubs: { RouterLink: true, ImageLightbox: true } },
  })
}

describe('поле «Файл» (KIN-27)', () => {
  it('значение: sha256 в value, имя, тип и размер — в meta (прочая meta сохраняется)', () => {
    const raw = storedRaw(doc('application/pdf'))
    expect(raw).toEqual({ sha256: 'abc', name: 'Метрика.pdf', mime: 'application/pdf', size: 2048, preview: false })
    expect(valueIn(raw, { note: 1 })).toEqual({ value: 'abc', meta: { note: 1, file: { name: 'Метрика.pdf', mime: 'application/pdf', size: 2048 } } })
  })

  it('PDF — в новой вкладке, прочее — скачиванием с именем, изображение — превью', () => {
    const pdf = show(doc('application/pdf')).find('a.name')
    expect(pdf.attributes('href')).toBe('/api/files/d1/abc')
    expect(pdf.attributes('target')).toBe('_blank')
    expect(pdf.attributes('download')).toBeUndefined()

    const zip = show(doc('application/zip', 'архив.zip')).find('a.name')
    expect(zip.attributes('download')).toBe('архив.zip')
    expect(zip.attributes('target')).toBeUndefined()

    const w = show(doc('image/jpeg', 'скан.jpg', true))
    // изображение без превью (огромный скан) — открывается в новой вкладке
    expect(show(doc('image/jpeg', 'большой.jpg')).find('button.view').exists()).toBe(false)
    expect(w.find('button.view img').attributes('src')).toBe('/api/files/d1/abc/preview?shape=square&size=64')
    expect(w.text()).toContain('2 КБ')
  })
})
