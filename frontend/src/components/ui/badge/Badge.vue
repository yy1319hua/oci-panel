<script setup lang="ts">
import { computed } from 'vue'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/utils'

const badgeVariants = cva(
  'inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold transition-colors focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2',
  {
    variants: {
      variant: {
        default: 'border-transparent bg-primary text-primary-foreground',
        secondary: 'border-transparent bg-secondary text-secondary-foreground',
        destructive: 'border-transparent bg-destructive text-destructive-foreground',
        outline: 'text-foreground',
        success: 'border-transparent bg-success/20 text-success',
        warning: 'border-transparent bg-warning/20 text-warning',
        info: 'border-transparent bg-primary/20 text-primary'
      }
    },
    defaultVariants: {
      variant: 'default'
    }
  }
)

type BadgeVariants = VariantProps<typeof badgeVariants>

interface Props {
  variant?: BadgeVariants['variant']
  class?: string
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'default'
})

const classes = computed(() => cn(badgeVariants({ variant: props.variant }), props.class))
</script>

<template>
  <div :class="classes">
    <slot />
  </div>
</template>
