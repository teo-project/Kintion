import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import { t } from '@/i18n'
import { createPinia } from 'pinia'

import AuthShell from './AuthShell.vue'
import PasswordInput from './PasswordInput.vue'

describe('AuthShell', () => {
  it('shows the form slot, the news block and an animated background', () => {
    const wrapper = mount(AuthShell, {
      props: { title: t.auth.loginTitle, hint: t.auth.loginHint },
      slots: { default: '<form id="f"></form>' },
      global: { plugins: [createPinia()] },
    })
    expect(wrapper.find('h1').text()).toBe(t.auth.loginTitle)
    expect(wrapper.find('form#f').exists()).toBe(true)
    expect(wrapper.find('.tagline').text()).toBe(t.tagline)
    expect(wrapper.findAll('.news li')).toHaveLength(6)
    expect(wrapper.find('svg.tree').attributes('aria-hidden')).toBe('true')
  })
})

describe('PasswordInput', () => {
  it('toggles between hidden and visible', async () => {
    const wrapper = mount(PasswordInput, { props: { modelValue: 'secret', autocomplete: 'current-password' } })
    const input = wrapper.find('input')
    expect(input.attributes('type')).toBe('password')
    await wrapper.find('button').trigger('click')
    expect(input.attributes('type')).toBe('text')
    expect(wrapper.find('button').attributes('aria-label')).toBe(t.auth.hidePassword)
  })
})
