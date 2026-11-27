<script setup lang="ts">
// ThemedSelect —— 完全跟随主题的下拉选择，替代浏览器原生 <select>
// （原生弹出层由系统绘制，暗色主题下会闪出白底列表）。
// 浮层复用 .history-dropdown，因此自动获得各主题的面板材质与 8bit 方角。
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'

export interface ThemedOption {
  name: string
  value: string
}

const props = defineProps<{
  modelValue: string
  options: ThemedOption[]
  ariaLabel?: string
}>()
const emit = defineEmits<{ 'update:modelValue': [string] }>()

const open = ref(false)
const root = ref<HTMLElement | null>(null)
const menu = ref<HTMLElement | null>(null)

function currentName(): string {
  return props.options.find((o) => o.value === props.modelValue)?.name ?? props.options[0]?.name ?? ''
}

function toggle() {
  open.value = !open.value
}

function choose(value: string) {
  emit('update:modelValue', value)
  open.value = false
}

function onDocClick(e: MouseEvent) {
  // 点自己内部不关（交给选项自己的点击处理）；点其它下拉也会走到这里，
  // 于是同一时刻只会有一个菜单是打开的。
  if (!root.value?.contains(e.target as Node)) open.value = false
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') open.value = false
}

function unbind() {
  document.removeEventListener('click', onDocClick)
  document.removeEventListener('keydown', onKeydown)
}

// 打开时把当前选中项滚到可视区中间：站点多时不必每次从顶部往下滑，
// 上下还能看到相邻站点，正好落在当前站点的相对位置。
function scrollActiveIntoView() {
  const active = menu.value?.querySelector<HTMLElement>('.themed-select-option.active')
  // 部分环境（如 jsdom）没有实现 scrollIntoView，存在性保护避免抛错。
  if (typeof active?.scrollIntoView === 'function') active.scrollIntoView({ block: 'center' })
}

// 打开时才挂全局监听；监听在本次点击派发结束后才挂上，不会立刻把自己关掉。
watch(open, (isOpen) => {
  if (isOpen) {
    document.addEventListener('click', onDocClick)
    document.addEventListener('keydown', onKeydown)
    // 等浮层渲染出选项节点后再定位（v-if 打开瞬间 DOM 还不存在）。
    void nextTick(scrollActiveIntoView)
  } else {
    unbind()
  }
})

onBeforeUnmount(unbind)
</script>

<template>
  <span ref="root" class="themed-select">
    <button
      type="button"
      class="themed-select-trigger"
      :aria-label="ariaLabel"
      aria-haspopup="listbox"
      :aria-expanded="open"
      @click="toggle"
    >
      <span class="themed-select-label">{{ currentName() }}</span>
      <span class="themed-select-caret" aria-hidden="true">▾</span>
    </button>
    <div v-if="open" ref="menu" class="history-dropdown themed-select-menu" role="listbox">
      <button
        v-for="opt in options"
        :key="opt.value"
        type="button"
        class="themed-select-option"
        :class="{ active: opt.value === modelValue }"
        role="option"
        :aria-selected="opt.value === modelValue"
        @click="choose(opt.value)"
      >{{ opt.name }}</button>
    </div>
  </span>
</template>
