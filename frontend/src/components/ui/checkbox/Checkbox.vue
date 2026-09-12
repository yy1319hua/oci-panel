<script setup lang="ts">
import { computed } from 'vue'
import { Check, Minus } from 'lucide-vue-next'
import { cn } from '@/lib/utils'

interface Props {
  modelValue?: boolean
  indeterminate?: boolean
  disabled?: boolean
  class?: string
}

const props = withDefaults(defineProps<Props>(), {
  modelValue: false,
  indeterminate: false,
  disabled: false
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()

const toggle = () => {
  if (!props.disabled) {
    emit('update:modelValue', !props.modelValue)
  }
}

const classes = computed(() =>
  cn(
    'peer h-4 w-4 shrink-0 rounded-sm border border-primary ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 transition-all duration-200 cursor-pointer',
    (props.modelValue || props.indeterminate) && 'bg-primary text-primary-foreground',
    props.class
  )
)
</script>

<template>
  <button
    type="button"
    role="checkbox"
    :aria-checked="indeterminate ? 'mixed' : modelValue"
    :disabled="disabled"
    :class="classes"
    @click="toggle"
  >
    <span class="flex items-center justify-center text-current">
      <Minus v-if="indeterminate" class="h-3.5 w-3.5" />
      <Check v-else-if="modelValue" class="h-3.5 w-3.5" />
    </span>
  </button>
</template>
