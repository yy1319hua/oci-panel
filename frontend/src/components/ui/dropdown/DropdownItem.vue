<script setup lang="ts">
defineProps<{
  destructive?: boolean
  disabled?: boolean
}>()

const emit = defineEmits<{
  click: []
}>()
</script>

<template>
  <button
    :class="[
      'flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors',
      disabled
        ? 'opacity-50 cursor-not-allowed'
        : destructive
          ? 'text-destructive hover:bg-destructive/10'
          : 'hover:bg-muted'
    ]"
    :disabled="disabled"
    @click="emit('click')"
  >
    <slot />
  </button>
</template>
