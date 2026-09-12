<script setup lang="ts">
import { onMounted } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { Toaster } from '@/components/ui/toast'

const authStore = useAuthStore()

onMounted(() => {
  authStore.checkAuth()
})
</script>

<template>
  <div id="app" class="min-h-screen bg-background text-foreground">
    <router-view />
    <Toaster />
  </div>
</template>
