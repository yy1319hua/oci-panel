<script setup lang="ts">
import { ref, computed } from 'vue'
import { Loader2, Wallet, AlertTriangle, ShieldCheck, RefreshCw } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import type { DailyCost, CostStats } from '@/api'

// 配置详情「成本统计」标签页：逐日费用（第一时间发现扣费）。
// 数据由抽屉加载后以 props 传入，刷新经 emit('reload', days) 上抛，避免组件内重复持有请求状态。
const props = defineProps<{
  cost: CostStats | null
  loading: boolean
  error: string
  days: number
}>()

const emit = defineEmits<{
  reload: [days: number]
}>()

const DAY_OPTIONS = [7, 30, 90]

const selectedDays = ref(props.days || 30)

const pickDays = (d: number) => {
  selectedDays.value = d
  emit('reload', d)
}

/** 有费用的天数（>0），用于「是否已扣费」的即时判断。 */
const chargedDays = computed(() => (props.cost?.days || []).filter(d => d.amount > 0))

/** 柱状图高度百分比：以最大值为基准。 */
const barPct = (amount: number) => {
  const max = Math.max(...(props.cost?.days || []).map(d => d.amount), 0)
  if (max <= 0) return 0
  return Math.max(2, (amount / max) * 100)
}

const formatAmount = (v: number, currency = 'USD') => {
  const symbol = currency === 'USD' ? '$' : currency === 'CNY' ? '¥' : ''
  return `${symbol}${(v || 0).toFixed(4)}`
}

// 仅展示最近 N 根柱子（默认取后 30 天，避免 90 天时过密）
const chartDays = computed(() => {
  const list = props.cost?.days || []
  return list.slice(-30)
})
</script>

<template>
  <div class="space-y-4">
    <!-- 额度提示 -->
    <Card class="p-4">
      <div class="flex flex-wrap items-center justify-between gap-4">
        <div class="flex items-center gap-3">
          <div
            class="w-10 h-10 rounded-lg flex items-center justify-center"
            :class="chargedDays.length ? 'bg-destructive/10' : 'bg-success/10'"
          >
            <AlertTriangle v-if="chargedDays.length" class="w-5 h-5 text-destructive" />
            <ShieldCheck v-else class="w-5 h-5 text-success" />
          </div>
          <div>
            <p class="font-semibold">
              {{ chargedDays.length ? '检测到产生费用' : '免费额度内，暂无费用' }}
            </p>
            <p class="text-xs text-muted-foreground">
              甲骨文每月赠送 10TB 出站流量，入站免费；超出部分按量计费。账单数据有数小时延迟。
            </p>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <Button
            v-for="d in DAY_OPTIONS"
            :key="d"
            size="sm"
            :variant="selectedDays === d ? 'default' : 'outline'"
            :disabled="loading"
            @click="pickDays(d)"
          >
            近 {{ d }} 天
          </Button>
          <Button size="sm" variant="ghost" :disabled="loading" @click="emit('reload', selectedDays)">
            <RefreshCw :class="['w-4 h-4', loading && 'animate-spin']" />
          </Button>
        </div>
      </div>
    </Card>

    <!-- 本月合计 -->
    <div class="grid grid-cols-1 sm:grid-cols-3 gap-4">
      <Card class="p-4">
        <p class="text-sm text-muted-foreground">本月累计费用</p>
        <p
          class="text-2xl font-bold font-display mt-1"
          :class="cost && cost.monthToDate > 0 ? 'text-destructive' : 'text-success'"
        >
          {{ formatAmount(cost?.monthToDate || 0, cost?.currency) }}
        </p>
      </Card>
      <Card class="p-4">
        <p class="text-sm text-muted-foreground">统计区间</p>
        <p class="text-2xl font-bold font-display mt-1">{{ cost?.daysCount || 0 }} <span class="text-base text-muted-foreground">天</span></p>
      </Card>
      <Card class="p-4">
        <p class="text-sm text-muted-foreground">计费币种</p>
        <p class="text-2xl font-bold font-display mt-1">{{ cost?.currency || 'USD' }}</p>
      </Card>
    </div>

    <!-- 加载/错误 -->
    <div v-if="loading" class="flex items-center justify-center py-12">
      <Loader2 class="w-10 h-10 animate-spin text-primary" />
    </div>
    <Card v-else-if="error" class="p-6 text-center space-y-3">
      <AlertTriangle class="w-8 h-8 text-warning mx-auto" />
      <p class="text-sm text-muted-foreground">{{ error }}</p>
      <p class="text-xs text-muted-foreground">
        若提示权限不足，请在 Oracle 控制台为用户组授予 <code class="px-1 bg-muted rounded">USAGE_REPORT_READ</code> 权限。
      </p>
    </Card>

    <template v-else-if="cost && cost.days.length">
      <!-- 趋势柱状图 -->
      <Card class="p-4">
        <div class="flex items-baseline justify-between mb-4">
          <h4 class="font-semibold flex items-center gap-2">
            <Wallet class="w-5 h-5 text-primary" />
            每日费用趋势
          </h4>
          <span class="text-xs text-muted-foreground">最近 {{ chartDays.length }} 天</span>
        </div>
        <div class="flex items-end gap-[3px] h-32">
          <div
            v-for="d in chartDays"
            :key="d.date"
            class="flex-1 rounded-t transition-all"
            :class="d.amount > 0 ? 'bg-destructive' : 'bg-muted'"
            :style="{ height: (d.amount > 0 ? barPct(d.amount) : 2) + '%' }"
            :title="`${d.date} · ${formatAmount(d.amount, d.currency)}`"
          ></div>
        </div>
        <div class="flex justify-between text-xs text-muted-foreground mt-2">
          <span>{{ chartDays[0]?.date }}</span>
          <span>{{ chartDays[chartDays.length - 1]?.date }}</span>
        </div>
      </Card>

      <!-- 逐日明细 -->
      <Card class="p-4">
        <h4 class="font-semibold mb-4">每日费用明细</h4>

        <!-- 手机端：卡片式堆叠，避免窄屏把日期/徽章挤到换行 -->
        <div class="sm:hidden max-h-[400px] overflow-y-auto">
          <div
            v-for="d in [...cost.days].reverse()"
            :key="d.date"
            class="flex items-center justify-between gap-3 py-3 border-b border-border/50 last:border-0"
          >
            <p class="font-mono text-sm whitespace-nowrap">{{ d.date }}</p>
            <div class="flex items-center gap-2 shrink-0">
              <span class="tabular-nums text-sm" :class="d.amount > 0 ? 'text-destructive font-medium' : ''">
                {{ formatAmount(d.amount, d.currency) }}
              </span>
              <span
                class="px-2 py-0.5 rounded-full text-xs font-medium whitespace-nowrap"
                :class="d.amount > 0 ? 'bg-destructive/15 text-destructive' : 'bg-success/15 text-success'"
              >
                {{ d.amount > 0 ? '已扣费' : '免费' }}
              </span>
            </div>
          </div>
        </div>

        <!-- 桌面端：保持原表格 -->
        <div class="hidden sm:block overflow-x-auto max-h-[400px] overflow-y-auto">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>日期</TableHead>
                <TableHead class="text-right">费用</TableHead>
                <TableHead class="text-right">状态</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="d in [...cost.days].reverse()" :key="d.date">
                <TableCell class="text-muted-foreground font-mono">{{ d.date }}</TableCell>
                <TableCell class="text-right tabular-nums" :class="d.amount > 0 ? 'text-destructive font-medium' : ''">
                  {{ formatAmount(d.amount, d.currency) }}
                </TableCell>
                <TableCell class="text-right">
                  <span
                    class="px-2 py-0.5 rounded-full text-xs font-medium"
                    :class="d.amount > 0 ? 'bg-destructive/15 text-destructive' : 'bg-success/15 text-success'"
                  >
                    {{ d.amount > 0 ? '已扣费' : '免费' }}
                  </span>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </div>
      </Card>
    </template>

    <div v-else class="text-center py-16">
      <div class="w-20 h-20 mx-auto mb-4 rounded-full bg-muted/50 flex items-center justify-center">
        <Wallet class="w-10 h-10 text-muted-foreground" />
      </div>
      <p class="text-muted-foreground">暂无成本数据</p>
    </div>
  </div>
</template>
