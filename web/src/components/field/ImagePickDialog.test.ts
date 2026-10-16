import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, ref } from 'vue'

import ImagePickDialog from './ImagePickDialog.vue'

afterEach(() => {
  document.body.innerHTML = ''
})

/** диалог открыт; upload — что вызвал диалог */
async function open(upload: (f: File) => Promise<void>) {
  const isOpen = ref(true)
  const w = mount(defineComponent(() => () => h(ImagePickDialog, {
    'open': isOpen.value,
    'onUpdate:open': (v: boolean) => (isOpen.value = v),
    upload,
  })), { attachTo: document.body })
  await flushPromises()
  return { w, isOpen }
}

const png = () => new File(['x'], 'a.png', { type: 'image/png' })
const txt = () => new File(['x'], 'a.txt', { type: 'text/plain' })

function files(list: File[]) {
  return Object.assign([...list], { item: (i: number) => list[i] ?? null }) as unknown as FileList
}

function paste(list: File[]) {
  const e = new Event('paste', { bubbles: true, cancelable: true })
  Object.defineProperty(e, 'clipboardData', { value: { files: files(list) } })
  document.dispatchEvent(e)
}

function drop(list: File[]) {
  const e = new Event('drop', { bubbles: true, cancelable: true })
  Object.defineProperty(e, 'dataTransfer', { value: { files: files(list) } })
  document.body.querySelector('.image-pick .drop')!.dispatchEvent(e)
}

describe('диалог загрузки изображения (F-107)', () => {
  it('Ctrl+V из буфера: картинка загружается, диалог закрывается', async () => {
    const upload = vi.fn(async () => {})
    const { isOpen } = await open(upload)
    paste([png()])
    await flushPromises()
    expect(upload).toHaveBeenCalledWith(expect.objectContaining({ name: 'a.png' }))
    expect(isOpen.value).toBe(false)
  })

  it('перетаскивание файла; не изображение — ошибка в диалоге', async () => {
    const upload = vi.fn(async () => {})
    const { isOpen } = await open(upload)
    drop([txt()])
    await flushPromises()
    expect(upload).not.toHaveBeenCalled()
    expect(document.body.querySelector('.image-pick [role=alert]')!.textContent).toContain('Это не изображение')
    drop([png()])
    await flushPromises()
    expect(upload).toHaveBeenCalledOnce()
    expect(isOpen.value).toBe(false)
  })

  it('в буфере нет картинки, сервер отказал — ошибка, диалог открыт', async () => {
    const upload = vi.fn(async () => { throw new Error('слишком большой файл') })
    const { isOpen } = await open(upload)
    paste([])
    await flushPromises()
    expect(document.body.querySelector('.image-pick [role=alert]')!.textContent).toContain('В буфере обмена нет изображения')
    paste([png()])
    await flushPromises()
    expect(document.body.querySelector('.image-pick [role=alert]')!.textContent).toContain('слишком большой файл')
    expect(isOpen.value).toBe(true)
  })

  it('пока файл загружается, диалог не закрыть (Esc, «Отмена»)', async () => {
    let done!: () => void
    const upload = vi.fn(() => new Promise<void>(r => (done = r)))
    const { isOpen } = await open(upload)
    paste([png()])
    await flushPromises()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    ;(document.body.querySelector('.image-pick .kin-actions button') as HTMLButtonElement).click()
    await flushPromises()
    expect(isOpen.value).toBe(true)
    done()
    await flushPromises()
    expect(isOpen.value).toBe(false)
  })
})
