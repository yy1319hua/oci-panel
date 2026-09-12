<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick } from 'vue'

const props = defineProps<{
  align?: 'left' | 'right'
}>()

const isOpen = ref(false)
const dropdownRef = ref<HTMLElement>()
const triggerRef = ref<HTMLElement>()
const menuRef = ref<HTMLElement>()
const menuStyle = ref({ top: '0px', left: '0px' })

const updatePosition = () => {
  if (!triggerRef.value) return
  const rect = triggerRef.value.getBoundingClientRect()
  const menuWidth = 192

  let left = props.align === 'left' ? rect.left : rect.right - menuWidth
  left = Math.max(8, Math.min(left, window.innerWidth - menuWidth - 8))

  menuStyle.value = {
    top: `${rect.bottom + 8}px`,
    left: `${left}px`
  }
}

const toggle = () => {
  isOpen.value = !isOpen.value
  if (isOpen.value) {
    nextTick(updatePosition)
  }
}

const close = () => {
  isOpen.value = false
}

const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as Node
  if (dropdownRef.value && !dropdownRef.value.contains(target) && menuRef.value && !menuRef.value.contains(target)) {
    close()
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  window.addEventListener('scroll', close, true)
  window.addEventListener('resize', close)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  window.removeEventListener('scroll', close, true)
  window.removeEventListener('resize', close)
})

defineExpose({ close })
</script>

<template>
  <div ref="dropdownRef" class="relative inline-block">
    <div ref="triggerRef" @click="toggle">
      <slot name="trigger" />
    </div>
    <Teleport to="body">
      <Transition
        enter-active-class="transition ease-out duration-100"
        enter-from-class="transform opacity-0 scale-95"
        enter-to-class="transform opacity-100 scale-100"
        leave-active-class="transition ease-in duration-75"
        leave-from-class="transform opacity-100 scale-100"
        leave-to-class="transform opacity-0 scale-95"
      >
        <div
          v-if="isOpen"
          ref="menuRef"
          class="fixed z-[9999] w-48 rounded-lg border border-border bg-popover p-1 shadow-lg"
          :style="menuStyle"
          @click="close"
        >
          <slot />
        </div>
      </Transition>
    </Teleport>
  </div>
</template>
