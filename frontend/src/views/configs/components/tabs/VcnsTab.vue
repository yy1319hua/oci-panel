<script setup lang="ts">
import { Loader2, Network, Shield } from 'lucide-vue-next'
import { Card } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

// 配置详情「VCN 网络」标签页（C7 从 Configs.vue 抽出，纯展示）。
// 「安全列表」动作经 emit 上抛由抽屉打开 SecurityListModal。
defineProps<{
  vcns: any[]
  loading: boolean
}>()

defineEmits<{
  security: [vcn: any]
}>()
</script>

<template>
  <div v-if="loading" class="flex items-center justify-center py-12">
    <Loader2 class="w-10 h-10 animate-spin text-primary" />
  </div>
  <div v-else-if="!vcns.length" class="text-center py-16">
    <div class="w-20 h-20 mx-auto mb-4 rounded-full bg-muted/50 flex items-center justify-center">
      <Network class="w-10 h-10 text-muted-foreground" />
    </div>
    <p class="text-muted-foreground text-lg">暂无VCN</p>
  </div>
  <div v-else class="space-y-4">
    <Card v-for="vcn in vcns" :key="vcn.id" class="p-5">
      <div class="flex justify-between items-start mb-4">
        <div class="flex items-center gap-4">
          <div class="w-12 h-12 rounded-lg bg-primary/10 flex items-center justify-center">
            <Network class="w-6 h-6 text-primary" />
          </div>
          <div>
            <h4 class="font-semibold text-lg">{{ vcn.displayName }}</h4>
            <p class="text-sm text-muted-foreground font-mono">CIDR: {{ vcn.cidrBlock }}</p>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <Badge variant="success">{{ vcn.state }}</Badge>
          <Button size="sm" variant="outline" @click="$emit('security', vcn)">
            <Shield class="w-4 h-4" />
            安全列表
          </Button>
        </div>
      </div>
      <div v-if="vcn.createTime" class="text-sm text-muted-foreground mb-4">创建时间: {{ vcn.createTime }}</div>
      <div v-if="vcn.subnets?.length">
        <h5 class="text-sm font-medium mb-3">子网 ({{ vcn.subnets.length }}个)</h5>
        <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
          <div v-for="subnet in vcn.subnets" :key="subnet.id" class="bg-muted/30 rounded-lg p-4">
            <div class="flex justify-between items-center mb-2">
              <span class="font-medium">{{ subnet.displayName }}</span>
              <Badge :variant="subnet.isPublic ? 'success' : 'warning'" class="text-xs">
                {{ subnet.isPublic ? '公有' : '私有' }}
              </Badge>
            </div>
            <p class="text-sm text-muted-foreground font-mono">{{ subnet.cidrBlock }}</p>
          </div>
        </div>
      </div>
    </Card>
  </div>
</template>
