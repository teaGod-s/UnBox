import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import ThemedSelect from './ThemedSelect.vue'

const options = [
  { name: '线路一', value: 'a' },
  { name: '线路二', value: 'b' },
]

describe('ThemedSelect', () => {
  it('shows the selected option and emits the picked value', async () => {
    const wrapper = mount(ThemedSelect, { props: { modelValue: 'a', options }, attachTo: document.body })
    expect(wrapper.find('.themed-select-label').text()).toBe('线路一')

    await wrapper.find('.themed-select-trigger').trigger('click')
    expect(wrapper.findAll('.themed-select-option')).toHaveLength(2)

    await wrapper.findAll('.themed-select-option')[1].trigger('click')
    expect(wrapper.emitted('update:modelValue')![0]).toEqual(['b'])
    expect(wrapper.find('.themed-select-menu').exists()).toBe(false)
    wrapper.unmount()
  })

  it('closes when clicking outside', async () => {
    const wrapper = mount(ThemedSelect, { props: { modelValue: 'a', options }, attachTo: document.body })
    await wrapper.find('.themed-select-trigger').trigger('click')
    expect(wrapper.find('.themed-select-menu').exists()).toBe(true)

    document.body.click()
    await nextTick()
    expect(wrapper.find('.themed-select-menu').exists()).toBe(false)
    wrapper.unmount()
  })

  it('closes the previous select when another one is opened', async () => {
    const first = mount(ThemedSelect, { props: { modelValue: 'a', options }, attachTo: document.body })
    const second = mount(ThemedSelect, { props: { modelValue: 'a', options }, attachTo: document.body })
    await first.find('.themed-select-trigger').trigger('click')
    expect(first.find('.themed-select-menu').exists()).toBe(true)

    ;(second.find('.themed-select-trigger').element as HTMLElement).click()
    await nextTick()
    expect(first.find('.themed-select-menu').exists()).toBe(false)
    first.unmount()
    second.unmount()
  })

  it('scrolls the active option into view when the menu opens', async () => {
    // 站点多时列表很长：打开浮层应定位到当前选中项（居中），而不是从顶部开始。
    // jsdom 没有 scrollIntoView 实现（原型上不存在，vi.spyOn 无法挂），
    // 直接赋值并在测试末尾删除还原。
    const calls: Array<{ self: Element; arg: unknown }> = []
    const proto = Element.prototype as { scrollIntoView?: (arg?: ScrollIntoViewOptions | boolean) => void }
    proto.scrollIntoView = function (arg?: ScrollIntoViewOptions | boolean) {
      calls.push({ self: this as Element, arg })
    }

    const many = Array.from({ length: 30 }, (_, i) => ({ name: `站点${i + 1}`, value: `s${i + 1}` }))
    const wrapper = mount(ThemedSelect, { props: { modelValue: 's15', options: many }, attachTo: document.body })

    await wrapper.find('.themed-select-trigger').trigger('click')
    await nextTick()

    const active = wrapper.find('.themed-select-option.active').element
    expect(calls).toHaveLength(1)
    expect(calls[0].self).toBe(active)
    expect(calls[0].arg).toEqual({ block: 'center' })

    wrapper.unmount()
    delete proto.scrollIntoView
  })
})
