<script setup lang="ts">
import { Loader2, Server, Play, Square, RotateCcw, Globe, Settings, Trash2 } from 'lucide-vue-next'
import { Card } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { Instance } from '@/views/configs/types'

// 配置详情「实例列表」标签页（C7 从 Configs.vue 抽出，纯展示）。
// 所有实例操作经 emit 上抛由抽屉处理（控制/删除/换 IP/编辑），
// 与拆分前的语义、确认弹窗、loading 标识完全一致。
defineProps<{
  instances: Instance[]
  loading: boolean
  actionLoading: Record<string, boolean>
}>()

defineEmits<{
  control: [instanceId: string, action: string]
  terminate: [instanceId: string]
  'change-ip': [instanceId: string]
  edit: [instance: Instance]
}>()
</script>

<template>
  <div v-if="loading" class="flex items-center justify-center py-12">
    <Loader2 class="w-10 h-10 animate-spin text-primary" />
  </div>
  <div v-else-if="!instances.length" class="text-center py-16">
    <div class="w-20 h-20 mx-auto mb-4 rounded-full bg-muted/50 flex items-center justify-center">
      <Server class="w-10 h-10 text-muted-foreground" />
    </div>
    <p class="text-muted-foreground text-lg">暂无实例</p>
  </div>
  <div v-else class="grid grid-cols-1 xl:grid-cols-2 gap-4">
    <Card v-for="instance in instances" :key="instance.id" class="p-5 hover:border-primary/50 transition-colors">
      <div class="flex justify-between items-start mb-4">
        <div class="flex-1 min-w-0 pr-4">
          <h4 class="font-semibold text-lg break-words">{{ instance.displayName }}</h4>
          <p class="text-xs text-muted-foreground font-mono break-all mt-1">{{ instance.id }}</p>
        </div>
        <Badge
          :variant="instance.state === 'RUNNING' ? 'success' : instance.state === 'STOPPED' ? 'destructive' : 'warning'"
          class="shrink-0"
        >
          {{ instance.state }}
        </Badge>
      </div>
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 text-sm mb-5">
        <div class="bg-muted/30 rounded-lg p-3 sm:col-span-2">
          <span class="text-muted-foreground text-xs block mb-1">规格</span>
          <span class="font-medium break-words">{{ instance.shape }}</span>
        </div>
        <div class="bg-muted/30 rounded-lg p-3">
          <span class="text-muted-foreground text-xs block mb-1">CPU / 内存</span>
          <span class="font-medium break-words">{{ instance.ocpus }}核 / {{ instance.memory }}GB</span>
        </div>
        <div class="bg-muted/30 rounded-lg p-3">
          <span class="text-muted-foreground text-xs block mb-1">引导卷</span>
          <span class="font-medium">{{ instance.bootVolumeSize || '-' }} GB</span>
        </div>
        <div class="bg-muted/30 rounded-lg p-3 sm:col-span-2">
          <span class="text-muted-foreground text-xs block mb-1">区域</span>
          <span class="font-medium break-words">{{ instance.region }}</span>
        </div>
        <div class="bg-muted/30 rounded-lg p-3 sm:col-span-2">
          <span class="text-muted-foreground text-xs block mb-1">公网IP</span>
          <span class="font-mono text-sm font-medium break-words">{{ instance.publicIps?.join(', ') || '无' }}</span>
        </div>
        <div class="bg-muted/30 rounded-lg p-3 sm:col-span-2">
          <span class="text-muted-foreground text-xs block mb-1">内网IP</span>
          <span class="font-mono text-sm font-medium break-words">{{ instance.privateIps?.join(', ') || '无' }}</span>
        </div>
        <div v-if="instance.ipv6" class="bg-primary/5 border border-primary/20 rounded-lg p-3 sm:col-span-2">
          <span class="text-primary text-xs block mb-1">IPv6</span>
          <span class="font-mono text-sm text-primary break-all">{{ instance.ipv6 }}</span>
        </div>
      </div>
      <div class="flex flex-wrap gap-2">
        <Button
          size="sm"
          variant="success"
          :disabled="instance.state === 'RUNNING' || actionLoading[instance.id]"
          @click="$emit('control', instance.id, 'START')"
        >
          <Play class="w-3.5 h-3.5" />
          启动
        </Button>
        <Button
          size="sm"
          variant="warning"
          :disabled="instance.state !== 'RUNNING' || actionLoading[instance.id]"
          @click="$emit('control', instance.id, 'STOP')"
        >
          <Square class="w-3.5 h-3.5" />
          停止
        </Button>
        <Button
          size="sm"
          variant="outline"
          :disabled="instance.state !== 'RUNNING' || actionLoading[instance.id]"
          @click="$emit('control', instance.id, 'SOFTRESET')"
        >
          <RotateCcw class="w-3.5 h-3.5" />
          重启
        </Button>
        <Button
          size="sm"
          variant="outline"
          :disabled="actionLoading[instance.id]"
          @click="$emit('change-ip', instance.id)"
        >
          <Globe class="w-3.5 h-3.5" />
          更改IP
        </Button>
        <Button size="sm" variant="outline" @click="$emit('edit', instance)">
          <Settings class="w-3.5 h-3.5" />
          编辑配置
        </Button>
        <Button
          size="sm"
          variant="destructive"
          :disabled="actionLoading[instance.id]"
          @click="$emit('terminate', instance.id)"
        >
          <Trash2 class="w-3.5 h-3.5" />
          删除
        </Button>
      </div>
    </Card>
  </div>
</template>
