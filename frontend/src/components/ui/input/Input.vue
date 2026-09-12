<script setup lang="ts">
import { computed } from 'vue'
import { cn } from '@/lib/utils'

interface Props {
  class?: string
  type?: string
  placeholder?: string
  disabled?: boolean
  modelValue?: string | number
  readonly?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  type: 'text',
  disabled: false,
  readonly: false
})

const emit = defineEmits<{
  'update:modelValue': [value: string | number]
}>()

const classes = computed(() =>
  cn(
    'flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 transition-all duration-200',
    props.class
  )
)

const handleInput = (event: Event) => {
  const target = event.target as HTMLInputElement
  emit('update:modelValue', target.value)
}
</script>

<template>
  <input
    :type="type"
    :class="classes"
    :placeholder="placeholder"
    :disabled="disabled"
    :readonly="readonly"
    :value="modelValue"
    @input="handleInput"
  />
</template>
