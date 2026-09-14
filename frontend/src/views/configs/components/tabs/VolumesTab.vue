<script setup lang="ts">
import { Loader2, HardDrive, Edit } from 'lucide-vue-next'
import { Card } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

// 配置详情「引导卷」标签页（C7 从 Configs.vue 抽出，纯展示）。
// 数据由抽屉加载并经 props 传入；编辑动作经 emit 上抛由抽屉打开 VolumeEditModal。
defineProps<{
  volumes: any[]
  loading: boolean
}>()

defineEmits<{
  edit: [volume: any]
}>()
</script>

<template>
  <div v-if="loading" class="flex items-center justify-center py-12">
    <Loader2 class="w-10 h-10 animate-spin text-primary" />
  </div>
  <div v-else-if="!volumes.length" class="text-center py-16">
    <div class="w-20 h-20 mx-auto mb-4 rounded-full bg-muted/50 flex items-center justify-center">
      <HardDrive class="w-10 h-10 text-muted-foreground" />
    </div>
    <p class="text-muted-foreground text-lg">暂无引导卷</p>
  </div>
  <div v-else class="space-y-4">
    <Card v-for="volume in volumes" :key="volume.id" class="p-5">
      <div class="flex flex-col sm:flex-row sm:justify-between sm:items-start gap-3 mb-4">
        <div class="flex items-center gap-4 min-w-0">
          <div class="w-12 h-12 rounded-lg bg-primary/10 flex items-center justify-center shrink-0">
            <HardDrive class="w-6 h-6 text-primary" />
          </div>
          <div class="min-w-0">
            <h4 class="font-semibold text-lg break-words">{{ volume.displayName }}</h4>
            <p class="text-xs text-muted-foreground font-mono break-all mt-1">{{ volume.id?.substring(0, 40) }}...</p>
          </div>
        </div>
        <div class="flex flex-wrap items-center gap-2 shrink-0">
          <Badge :variant="volume.attached ? 'success' : 'warning'">
            {{ volume.attached ? '已附加' : '未附加' }}
          </Badge>
          <Badge :variant="volume.state === 'AVAILABLE' ? 'success' : 'warning'">
            {{ volume.state }}
          </Badge>
          <Button size="sm" variant="outline" @click="$emit('edit', volume)">
            <Edit class="w-4 h-4" />
            编辑
          </Button>
        </div>
      </div>
      <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
        <div class="bg-muted/30 rounded-lg p-4">
          <span class="text-muted-foreground text-xs block mb-1">磁盘大小</span>
          <span class="font-semibold text-lg">{{ volume.sizeInGBs }} GB</span>
        </div>
        <div class="bg-muted/30 rounded-lg p-4">
          <span class="text-muted-foreground text-xs block mb-1">性能 (VPU/GB)</span>
          <span class="font-semibold text-lg">{{ volume.vpusPerGB || 10 }}</span>
        </div>
        <div v-if="volume.instanceName" class="bg-muted/30 rounded-lg p-4">
          <span class="text-muted-foreground text-xs block mb-1">附加实例</span>
          <span class="text-primary font-semibold">{{ volume.instanceName }}</span>
        </div>
        <div class="bg-muted/30 rounded-lg p-4">
          <span class="text-muted-foreground text-xs block mb-1">可用域</span>
          <span class="text-sm">{{ volume.availabilityDomain?.split(':').pop() || '-' }}</span>
        </div>
      </div>
    </Card>
  </div>
</template>
