<script setup lang="ts">
import Toast from './Toast.vue'
import { useToast } from '@/composables/useToast'

const { toasts, remove } = useToast()
</script>

<template>
  <Teleport to="body">
    <div class="fixed top-4 right-4 z-[100] flex flex-col gap-2 w-full max-w-sm">
      <TransitionGroup
        enter-active-class="transition-all duration-300"
        leave-active-class="transition-all duration-300"
        enter-from-class="opacity-0 translate-x-full"
        leave-to-class="opacity-0 translate-x-full"
      >
        <Toast
          v-for="toast in toasts"
          :key="toast.id"
          :id="toast.id"
          :message="toast.message"
          :type="toast.type"
          @close="remove"
        />
      </TransitionGroup>
    </div>
  </Teleport>
</template>
