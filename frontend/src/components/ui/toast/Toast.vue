<script setup lang="ts">
import { computed } from 'vue'
import { X, CheckCircle, XCircle, AlertTriangle, Info } from 'lucide-vue-next'
import { cn } from '@/lib/utils'

interface Props {
  id: number
  message: string
  type?: 'success' | 'error' | 'warning' | 'info'
}

const props = withDefaults(defineProps<Props>(), {
  type: 'info'
})

const emit = defineEmits<{
  close: [id: number]
}>()

const icon = computed(() => {
  switch (props.type) {
    case 'success':
      return CheckCircle
    case 'error':
      return XCircle
    case 'warning':
      return AlertTriangle
    default:
      return Info
  }
})

const iconClass = computed(() => {
  switch (props.type) {
    case 'success':
      return 'text-success'
    case 'error':
      return 'text-destructive'
    case 'warning':
      return 'text-warning'
    default:
      return 'text-primary'
  }
})

const borderClass = computed(() => {
  switch (props.type) {
    case 'success':
      return 'border-l-success'
    case 'error':
      return 'border-l-destructive'
    case 'warning':
      return 'border-l-warning'
    default:
      return 'border-l-primary'
  }
})
</script>

<template>
  <div
    :class="
      cn(
        'pointer-events-auto relative flex w-full items-center gap-3 overflow-hidden rounded-md border bg-card p-4 shadow-lg transition-all border-l-4',
        borderClass
      )
    "
  >
    <component :is="icon" :class="cn('h-5 w-5 shrink-0', iconClass)" />
    <p class="text-sm font-medium flex-1">{{ message }}</p>
    <button
      class="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-md hover:bg-secondary transition-colors"
      @click="emit('close', id)"
    >
      <X class="h-4 w-4" />
    </button>
  </div>
</template>
