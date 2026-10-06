<script setup lang="ts">
// ContentCardList.vue — 「内容展示样式」（列表 / 卡片）的统一渲染。
//
// 首页观看记录、点播收藏、点播列表、搜索结果四处结构相同，此前各自复制了一份
// 卡片标记；归一成 ContentCardItem 后由本组件统一渲染。
//
// 列表模式统一为两行（片名 / 站点·补充），卡片模式的徽标仍按原有类名输出，
// 因此样式表里 .content-card-* 那一套不需要任何改动。
import { ref } from 'vue'
import { contentCardSubtitle, type ContentCardItem } from '../contentCardItem'
import { imgError, scrollxEnter, scrollxLeave } from '../contentListDom'
import type { ContentCardStyle } from '../contentCardStyle'

const props = defineProps<{
  items: ContentCardItem[]
  style: ContentCardStyle
  /** 附加到 ul 的容器类（如 home-list / favorites-list），由各页面决定。 */
  listClass?: string
  /** 可删除的列表（观看记录 / 收藏）才显示行内删除按钮与卡片删除遮罩。 */
  deletable?: boolean
  /** 当前展开了删除遮罩的条目 key；由父级持有，避免组件自己记状态。 */
  activeDeleteKey?: string | null
}>()

// 只上报下标：各页面的数据源数组与 items 一一对应，父级用下标就能取回自己的
// 原始对象（类型不外泄），组件也不必理解任何业务字段。
const emit = defineEmits<{
  select: [index: number]
  contextmenu: [index: number, event: MouseEvent]
  remove: [index: number]
  requestDelete: []
}>()

function onContextMenu(index: number, event: MouseEvent) {
  if (!props.deletable) return
  emit('contextmenu', index, event)
}

// 点播列表页的「回到顶部」需要滚动 ul；把 DOM 元素留在组件内，只暴露这一个动作，
// 免得父级拿到组件实例却去摸它的内部节点。
const listEl = ref<HTMLElement | null>(null)

defineExpose({
  scrollToTop: () => listEl.value?.scrollTo({ top: 0, behavior: 'smooth' }),
})
</script>

<template>
  <ul ref="listEl" :class="[listClass, { 'content-card-grid': style === 'grid' }]">
    <li
      v-for="(item, index) in items"
      :key="item.key"
      class="content-row"
      :class="{ 'content-card': style === 'grid' }"
      @click="emit('select', index)"
      @contextmenu="onContextMenu(index, $event)"
    >
      <img v-if="item.logo" :src="item.logo" class="thumb" loading="lazy" referrerpolicy="no-referrer" @error="imgError" />
      <template v-if="style === 'grid'">
        <span v-if="item.site" class="content-card-badge content-card-site">{{ item.site }}</span>
        <span v-if="item.progress" class="content-card-badge content-card-progress scrollx" @mouseenter="scrollxEnter" @mouseleave="scrollxLeave">
          <span class="scrollx-inner">
            <span v-if="item.progress.ep" class="cc-ep">{{ item.progress.ep }}</span>
            <span v-if="item.progress.text" class="cc-prog">{{ item.progress.text }}</span>
          </span>
        </span>
        <span class="content-card-badge content-card-title scrollx" @mouseenter="scrollxEnter" @mouseleave="scrollxLeave"><span class="scrollx-inner">{{ item.title }}</span></span>
        <div v-if="deletable && activeDeleteKey === item.key" class="content-card-delete-mask" @click.stop>
          <button type="button" class="content-card-delete-action" @click.stop="emit('requestDelete')">删除</button>
        </div>
      </template>
      <template v-else>
        <span class="content-row-info">
          <span class="name">{{ item.title }}</span>
          <span class="sub">{{ contentCardSubtitle(item) }}</span>
        </span>
        <button v-if="deletable" class="row-delete" type="button" title="删除" @click.stop="emit('remove', index)">删除</button>
      </template>
    </li>
  </ul>
</template>
