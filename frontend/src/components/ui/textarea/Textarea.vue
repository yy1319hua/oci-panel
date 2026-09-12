<script setup lang="ts">
import { computed } from 'vue'
import { cn } from '@/lib/utils'

interface Props {
  class?: string
  placeholder?: string
  disabled?: boolean
  modelValue?: string
  rows?: number
}

const props = withDefaults(defineProps<Props>(), {
  disabled: false,
  rows: 4
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const classes = computed(() =>
  cn(
    'flex min-h-[80px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 transition-all duration-200',
    props.class
  )
)

const handleInput = (event: Event) => {
  const target = event.target as HTMLTextAreaElement
  emit('update:modelValue', target.value)
}
</script>

<template>
  <textarea
    :class="classes"
    :placeholder="placeholder"
    :disabled="disabled"
    :rows="rows"
    :value="modelValue"
    @input="handleInput"
  />
</template>
