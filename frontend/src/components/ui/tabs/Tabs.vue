<script setup lang="ts">
import { provide, ref, watch } from 'vue'
import { cn } from '@/lib/utils'

interface Props {
  defaultValue?: string
  modelValue?: string
  class?: string
}

const props = defineProps<Props>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const activeTab = ref(props.modelValue || props.defaultValue || '')

watch(
  () => props.modelValue,
  val => {
    if (val) activeTab.value = val
  }
)

const setActiveTab = (value: string) => {
  activeTab.value = value
  emit('update:modelValue', value)
}

provide('tabs', {
  activeTab,
  setActiveTab
})
</script>

<template>
  <div :class="cn('', props.class)">
    <slot />
  </div>
</template>
