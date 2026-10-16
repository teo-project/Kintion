import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { SaveResult } from '@/ui/record'

import EditCell from './EditCell.vue'
import ValueEditor from './ValueEditor.vue'

afterEach(() => {
  vi.useRealTimers()
  document.body.innerHTML = ''
})

function cell(save: (v: unknown) => Promise<SaveResult>, value: unknown = 'было') {
  return mount(EditCell, {
    props: { def: { type: 'text', name: 'Имя' }, value, dashboard: 'd1', save },
    attachTo: document.body,
  })
}

describe('поле ввода карточки: сохранение при потере фокуса (F-86)', () => {
  it('изменено в фокусе — жёлтый; сохранено — салатовый, гаснет за 3 с', async () => {
    vi.useFakeTimers()
    const save = vi.fn(async () => 'ok' as const)
    const w = cell(save)
    const input = w.find('input')
    await input.trigger('focusin')
    await input.setValue('стало')
    expect(w.classes()).toContain('cell--dirty')
    await input.trigger('focusout')
    await flushPromises()
    expect(save).toHaveBeenCalledWith('стало')
    expect(w.classes()).toContain('cell--saved')
    vi.advanceTimersByTime(3000)
    await flushPromises()
    expect(w.classes()).not.toContain('cell--saved')
  })

  it('без изменений — не сохраняет; Esc — возврат к сохранённому', async () => {
    const save = vi.fn(async () => 'ok' as const)
    const w = cell(save)
    const input = w.find('input')
    await input.trigger('focusin')
    await input.setValue('стало')
    await input.trigger('keydown', { key: 'Escape' })
    expect((input.element as HTMLInputElement).value).toBe('было')
    expect(w.classes()).not.toContain('cell--dirty')
    await input.trigger('focusout')
    await flushPromises()
    expect(save).not.toHaveBeenCalled()
  })

  it('не сохранилось — красный до следующего фокуса', async () => {
    const w = cell(async () => 'error')
    const input = w.find('input')
    await input.trigger('focusin')
    await input.setValue('стало')
    await input.trigger('focusout')
    await flushPromises()
    expect(w.classes()).toContain('cell--error')
    expect((input.element as HTMLInputElement).value).toBe('стало') // введённое не теряется
    await input.trigger('focusin')
    expect(w.classes()).not.toContain('cell--error')
  })

  it('конфликт версии — красный и текущее значение поля (F-45)', async () => {
    const w = cell(async () => {
      await w.setProps({ value: 'чужое' })
      return 'conflict'
    })
    const input = w.find('input')
    await input.trigger('focusin')
    await input.setValue('моё')
    await input.trigger('focusout')
    await flushPromises()
    expect(w.classes()).toContain('cell--error')
    expect((input.element as HTMLInputElement).value).toBe('чужое')
  })
})

describe('поле «да/нет»: сегменты в один клик', () => {
  function boolCell(save: (v: unknown) => Promise<SaveResult>, value: unknown) {
    return mount(EditCell, {
      props: { def: { type: 'bool', name: 'Умер' }, value, dashboard: 'd1', save },
      attachTo: document.body,
    })
  }

  it('клик сразу сохраняет; повторный клик по выбранному очищает (пусто ≠ «нет»)', async () => {
    const save = vi.fn(async () => 'ok' as const)
    const w = boolCell(save, null)
    const [yes, no] = w.findAll('.seg-item')
    await yes!.trigger('click')
    await flushPromises()
    expect(save).toHaveBeenLastCalledWith(true)
    await w.setProps({ value: true })
    expect(w.findAll('.seg-item')[0]!.attributes('aria-pressed')).toBe('true')
    await yes!.trigger('click')
    await flushPromises()
    expect(save).toHaveBeenLastCalledWith(null)
    await w.setProps({ value: null })
    await no!.trigger('click')
    await flushPromises()
    expect(save).toHaveBeenLastCalledWith(false)
  })
})

describe('выбор или своё (F-100): список в портале', () => {
  const def = { type: 'choice' as const, free: true, name: 'Форма', options: [{ key: 'ooo', name: 'ООО' }] }

  it('уход фокуса в открытый список не сохраняет; выбор варианта — сохраняет сразу', async () => {
    const save = vi.fn(async () => 'ok' as const)
    const w = mount(EditCell, { props: { def, value: null, dashboard: 'd1', save }, attachTo: document.body })
    const ed = w.findComponent(ValueEditor)
    await w.find('input').trigger('focusin')
    ed.vm.$emit('popup', true)
    await w.find('input').trigger('focusout')
    await flushPromises()
    expect(save).not.toHaveBeenCalled()
    // выбор: значение, затем «выбрано» (фокус уже вне поля — потери фокуса больше не будет)
    ed.vm.$emit('update:modelValue', 'ooo')
    ed.vm.$emit('pick')
    ed.vm.$emit('popup', false)
    await flushPromises()
    expect(save).toHaveBeenCalledTimes(1)
    expect(save).toHaveBeenCalledWith('ooo')
  })

  it('список закрыт без выбора, фокус вне поля — сохраняет введённое', async () => {
    const save = vi.fn(async () => 'ok' as const)
    const w = mount(EditCell, { props: { def, value: null, dashboard: 'd1', save }, attachTo: document.body })
    const ed = w.findComponent(ValueEditor)
    await w.find('input').trigger('focusin')
    ed.vm.$emit('popup', true)
    ed.vm.$emit('update:modelValue', { free: 'ЖСК' })
    await w.find('input').trigger('focusout')
    ed.vm.$emit('popup', false)
    await flushPromises()
    expect(save).toHaveBeenCalledWith({ free: 'ЖСК' })
  })
})
