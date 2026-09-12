<script setup lang="ts">
import { inject, computed, type Ref } from 'vue'
import { cn } from '@/lib/utils'

interface Props {
  value: string
  class?: string
}

const props = defineProps<Props>()

const tabs = inject<{
  activeTab: Ref<string>
  setActiveTab: (value: string) => void
}>('tabs')

const isActive = computed(() => tabs?.activeTab.value === props.value)
</script>

<template>
  <button
    type="button"
    :class="
      cn(
        'inline-flex items-center justify-center whitespace-nowrap rounded-sm px-3 py-1.5 text-sm font-medium ring-offset-background transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50',
        isActive ? 'bg-background text-foreground shadow-sm' : 'hover:bg-background/50',
        props.class
      )
    "
    @click="tabs?.setActiveTab(value)"
  >
    <slot />
  </button>
</template>
