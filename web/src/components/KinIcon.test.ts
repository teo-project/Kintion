import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import KinIcon from './KinIcon.vue'

describe('KinIcon', () => {
  it('рисует иконку Lucide с классом kin-icon, скрытую от чтеца', () => {
    const w = mount(KinIcon, { props: { name: 'history' } })
    const svg = w.find('svg')
    expect(svg.exists()).toBe(true)
    expect(svg.classes()).toContain('kin-icon')
    expect(svg.classes()).toContain('lucide')
    expect(svg.attributes('aria-hidden')).toBe('true')
    expect(svg.attributes('stroke-width')).toBe('2')
  })
})
