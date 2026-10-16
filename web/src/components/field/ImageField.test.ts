import { flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { RecSpec } from '@/core/__tests__/kit'
import { envelope, mountRecord, serve } from '@/views/__tests__/card'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const liza: RecSpec = {
  id: 'liza', template: 'kin.core.person',
  fields: { name: ['Лиза'], photo: [{ value: 'old', meta: { crop: { x: 0, y: 0, w: 10, shape: 'square' } } }] },
}

describe('фото', () => {
  it('загрузка → обрезки по местам показа, сеть — кругом → новое значение сверху истории (F-79)', async () => {
    const calls = serve((m, u) => {
      if (m === 'GET' && u === '/api/records/liza') return [200, { record: envelope(liza), related: {} }]
      if (m === 'POST' && u === '/api/dashboards/d1/files') {
        return [201, { sha256: 'new', mime: 'image/png', size: 1, width: 400, height: 300, created_at: '' }]
      }
      if (m === 'POST' && u === '/api/records/liza/fields/photo/values') return [200, { record: envelope(liza), related: {} }]
      return undefined
    })
    const w = await mountRecord('liza', true)
    // фото в шапке — обрезка карточки; старая обрезка была только квадратом (сеть) — портрет по умолчанию
    expect(w.find('.head img').attributes('src')).toBe('/api/files/d1/old/preview?shape=rect&size=256')

    // кнопка открывает диалог загрузки (F-107), файл — кнопкой «Выбрать файл»
    await w.find('.field-row .upload').trigger('click')
    await flushPromises()
    const input = document.body.querySelector('.image-pick input[type=file]') as HTMLInputElement
    Object.defineProperty(input, 'files', { value: [new File(['x'], 'a.png', { type: 'image/png' })] })
    input.dispatchEvent(new Event('change'))
    await flushPromises()
    expect(calls.find(c => c.url === '/api/dashboards/d1/files')!.body).toBeInstanceOf(FormData)
    expect(document.body.querySelector('.image-pick')).toBeNull() // загружено — диалог закрыт

    const img = document.body.querySelector('.crop-dialog img') as HTMLImageElement
    expect(img.getAttribute('src')).toBe('/api/files/d1/new')
    Object.defineProperty(img, 'naturalWidth', { value: 400 })
    Object.defineProperty(img, 'naturalHeight', { value: 300 })
    img.dispatchEvent(new Event('load'))
    await flushPromises()
    const network = [...document.body.querySelectorAll('.crop-dialog [role=tab]')].find(b => b.textContent?.trim() === 'Сеть') as HTMLButtonElement
    expect(document.body.querySelector('.crop-dialog [role=radio]')).toBeNull() // форма — только у сети
    network.click()
    await flushPromises()
    const circle = [...document.body.querySelectorAll('.crop-dialog [role=radio]')].find(b => b.textContent?.trim() === 'Круг') as HTMLButtonElement
    circle.click()
    await flushPromises()
    ;(document.body.querySelector('.crop-dialog .save') as HTMLButtonElement).click()
    await flushPromises()
    const add = calls.find(c => c.url === '/api/records/liza/fields/photo/values')!
    expect(add.body).toEqual({
      if_rev: 1, value: 'new', position: 'top',
      meta: {
        crop: {
          card: { x: 87, y: 0, w: 225, h: 300, shape: 'rect' },
          tree: { x: 87, y: 0, w: 225, h: 300, shape: 'rect' },
          network: { x: 50, y: 0, w: 300, h: 300, shape: 'circle' },
        },
      },
    })
  })
})
