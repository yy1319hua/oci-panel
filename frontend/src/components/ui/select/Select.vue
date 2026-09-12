<script setup lang="ts">
import { computed } from 'vue'
import { cn } from '@/lib/utils'

interface Props {
  class?: string
  modelValue?: string | number
  disabled?: boolean
  placeholder?: string
}

const props = withDefaults(defineProps<Props>(), {
  disabled: false
})

const emit = defineEmits<{
  'update:modelValue': [value: string | number]
}>()

const classes = computed(() =>
  cn(
    'flex h-10 w-full items-center justify-between rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 [&>span]:line-clamp-1 transition-all duration-200',
    props.class
  )
)

const handleChange = (event: Event) => {
  const target = event.target as HTMLSelectElement
  emit('update:modelValue', target.value)
}
</script>

<template>
  <select :class="classes" :disabled="disabled" :value="modelValue" @change="handleChange">
    <option v-if="placeholder" value="" disabled>{{ placeholder }}</option>
    <slot />
  </select>
</template>
