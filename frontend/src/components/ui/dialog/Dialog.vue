<script setup lang="ts">
import { watch } from 'vue'

interface Props {
  open?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  open: false
})

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

const close = () => {
  emit('update:open', false)
}

watch(
  () => props.open,
  isOpen => {
    if (isOpen) {
      document.body.style.overflow = 'hidden'
    } else {
      document.body.style.overflow = ''
    }
  }
)
</script>

<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition-opacity duration-200"
      leave-active-class="transition-opacity duration-200"
      enter-from-class="opacity-0"
      leave-to-class="opacity-0"
    >
      <div v-if="open" class="fixed inset-0 z-50 flex items-center justify-center">
        <div class="fixed inset-0 bg-black/80 backdrop-blur-sm" @click="close" />
        <Transition
          enter-active-class="transition-all duration-300"
          leave-active-class="transition-all duration-200"
          enter-from-class="opacity-0 scale-95"
          leave-to-class="opacity-0 scale-95"
        >
          <div
            v-if="open"
            class="relative z-50 w-full max-w-lg max-h-[90vh] overflow-auto rounded-lg border bg-card p-6 shadow-lg animate-in fade-in slide-in-from-bottom"
          >
            <slot />
          </div>
        </Transition>
      </div>
    </Transition>
  </Teleport>
</template>
