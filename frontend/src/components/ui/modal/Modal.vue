<script setup lang="ts">
import { X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'

/**
 * 统一模态外壳（C8）：抽取此前在 3 个弹窗（EditInstance/SecurityList/VolumeEdit
 * 及其内嵌的「添加规则」弹窗）中逐字重复的脚手架——
 * Teleport → fade 过渡 → 遮罩（点击空白关闭）→ 面板（bg-card rounded-xl shadow-2xl border）
 * → 标题栏（标题 + 右上角 X）。面板正文与底部按钮仍由各调用方通过默认插槽提供，
 * 故渲染结果与改造前逐像素一致。
 *
 * - 标题：纯文本用 `title` prop；需要图标时用 `#title` 插槽（标题栏始终是 flex 容器，
 *   纯文本场景与原样式视觉等价）。
 * - `maxWidth`：面板最大宽度工具类（max-w-lg / max-w-2xl / max-w-4xl …）。
 * - `panelClass`：可滚动弹窗追加 `max-h-[90vh] flex flex-col`（base 已含 overflow-hidden）。
 * - `zClass`：遮罩层级，嵌套弹窗用 `z-[60]` 叠在父弹窗之上。
 */
interface Props {
  open: boolean
  title?: string
  maxWidth?: string
  panelClass?: string
  zClass?: string
}

withDefaults(defineProps<Props>(), {
  title: '',
  maxWidth: 'max-w-lg',
  panelClass: '',
  zClass: 'z-50'
})

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

const close = () => emit('update:open', false)
</script>

<template>
  <Teleport to="body">
    <Transition name="fade">
      <div
        v-if="open"
        :class="['fixed inset-0 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm', zClass]"
        @click.self="close"
      >
        <div
          :class="['bg-card rounded-xl shadow-2xl w-full overflow-hidden border border-border', maxWidth, panelClass]"
        >
          <div class="flex items-center justify-between p-6 border-b border-border">
            <h2 class="text-xl font-bold flex items-center gap-2">
              <slot name="title">{{ title }}</slot>
            </h2>
            <Button variant="ghost" size="icon" @click="close"><X class="w-5 h-5" /></Button>
          </div>
          <slot />
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
