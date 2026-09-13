<script setup lang="ts">
import { Loader2, BarChart3 } from 'lucide-vue-next'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'

// 配置详情「流量统计」标签页（C7 从 Configs.vue 抽出）。
// 查询表单对象由抽屉以 props 传入并在此就地 v-model 双向绑定（与原内联实现共享同一响应式对象，
// 故抽屉对 form.instanceId 的 watch 仍会触发加载 VNIC）；点击查询经 emit('query') 上抛。
interface ValueLabel {
  value: string
  label: string
}

defineProps<{
  form: { instanceId: string; vnicId: string; startTime: string; endTime: string }
  condition: { instances: ValueLabel[] }
  vnics: ValueLabel[]
  traffic: { time: string[]; inbound: string[]; outbound: string[] }
  loading: boolean
}>()

defineEmits<{
  query: []
}>()

// 后端返回的数值单位是 MB；流量大时自动换算成 GB，避免出现一长串数字。
const formatTraffic = (mb?: string) => {
  const v = Number(mb)
  if (!mb || isNaN(v)) return '0'
  if (v >= 1024) return `${(v / 1024).toFixed(2)} GB`
  return `${v.toFixed(2)} MB`
}
</script>

<template>
  <div class="space-y-4">
    <Card class="p-4">
      <h4 class="font-semibold mb-4 flex items-center gap-2">
        <BarChart3 class="w-5 h-5 text-primary" />
        查询条件
      </h4>
      <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div>
          <label class="block text-sm text-muted-foreground mb-1">选择实例</label>
          <select
            v-model="form.instanceId"
            class="w-full h-10 px-3 rounded-md border border-input bg-background text-sm"
          >
            <option value="">请选择实例</option>
            <option v-for="inst in condition.instances" :key="inst.value" :value="inst.value">
              {{ inst.label }}
            </option>
          </select>
        </div>
        <div>
          <label class="block text-sm text-muted-foreground mb-1">选择VNIC</label>
          <select
            v-model="form.vnicId"
            class="w-full h-10 px-3 rounded-md border border-input bg-background text-sm"
            :disabled="!vnics.length"
          >
            <option value="">请选择VNIC</option>
            <option v-for="vnic in vnics" :key="vnic.value" :value="vnic.value">
              {{ vnic.label }}
            </option>
          </select>
        </div>
        <div>
          <label class="block text-sm text-muted-foreground mb-1">开始时间</label>
          <input
            v-model="form.startTime"
            type="datetime-local"
            step="1"
            class="w-full h-10 px-3 rounded-md border border-input bg-background text-sm text-foreground"
          />
        </div>
        <div>
          <label class="block text-sm text-muted-foreground mb-1">结束时间</label>
          <input
            v-model="form.endTime"
            type="datetime-local"
            step="1"
            class="w-full h-10 px-3 rounded-md border border-input bg-background text-sm text-foreground"
          />
        </div>
      </div>
      <Button class="mt-4" :disabled="loading" @click="$emit('query')">
        <Loader2 v-if="loading" class="w-4 h-4 animate-spin" />
        {{ loading ? '查询中...' : '查询流量' }}
      </Button>
    </Card>

    <div v-if="loading" class="flex items-center justify-center py-12">
      <Loader2 class="w-10 h-10 animate-spin text-primary" />
    </div>
    <Card v-else-if="traffic.time?.length" class="p-4">
      <div class="flex items-baseline justify-between mb-4">
        <h4 class="font-semibold">流量数据</h4>
        <span class="text-xs text-muted-foreground">聚合粒度按时间跨度自适应（大跨度自动变粗）</span>
      </div>
      <div class="overflow-x-auto">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>时间</TableHead>
              <TableHead class="text-success">入站</TableHead>
              <TableHead class="text-primary">出站</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="(time, index) in traffic.time" :key="index">
              <TableCell class="text-muted-foreground">{{ time }}</TableCell>
              <TableCell class="text-success">{{ formatTraffic(traffic.inbound[index]) }}</TableCell>
              <TableCell class="text-primary">{{ formatTraffic(traffic.outbound[index]) }}</TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>
    </Card>
    <div v-else class="text-center py-16">
      <div class="w-20 h-20 mx-auto mb-4 rounded-full bg-muted/50 flex items-center justify-center">
        <BarChart3 class="w-10 h-10 text-muted-foreground" />
      </div>
      <p class="text-muted-foreground">请选择实例和VNIC后查询流量数据</p>
    </div>
  </div>
</template>
