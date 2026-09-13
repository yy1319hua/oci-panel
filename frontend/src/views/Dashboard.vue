<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import {
  Server,
  Activity,
  TrendingUp,
  ArrowRight,
  Zap,
  Cpu,
  Network,
  Clock,
  BarChart3,
  Settings,
  FileText,
  Eye
} from 'lucide-vue-next'
import { sysApi, ociApi, type InstanceInfo } from '@/api'
import { toast } from '@/composables/useToast'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

const router = useRouter()

const loading = ref(true)
const version = ref('')
const configId = ref('')

const instances = ref<InstanceInfo[]>([])

const INSTANCE_COLORS = ['#22d3ee', '#34d399', '#fbbf24', '#a78bfa', '#f472b6']

interface TrafficInstance {
  id: string
  name: string
  total: number
  inbound: number
  outbound: number
  pct: number
  color: string
}

const traffic = ref<{
  totalBytes: number
  inboundBytes: number
  outboundBytes: number
  billableBytes: number
  freeAllowance: number
  allowancePct: number
  instances: TrafficInstance[]
  daily: number[]
}>({
  totalBytes: 0,
  inboundBytes: 0,
  outboundBytes: 0,
  billableBytes: 0,
  freeAllowance: 0,
  allowancePct: 0,
  instances: [],
  daily: []
})

const hasInstance = computed(() => instances.value.length > 0)
const runningCount = computed(() => instances.value.filter(i => i.state === 'RUNNING').length)

const stateVariant = (state: string) => {
  switch (state) {
    case 'RUNNING':
      return 'success'
    case 'STOPPED':
      return 'destructive'
    default:
      return 'warning'
  }
}

const stateLabel = (state: string) => {
  const map: Record<string, string> = {
    RUNNING: '运行中',
    STOPPED: '已停止',
    PROVISIONING: '创建中',
    STARTING: '启动中',
    STOPPING: '停止中',
    TERMINATING: '终止中',
    TERMINATED: '已终止'
  }
  return map[state] || state
}

const formatUptime = (createTime?: string) => {
  if (!createTime) return '—'
  const t = new Date(createTime)
  if (isNaN(t.getTime())) return '—'
  const diff = Date.now() - t.getTime()
  if (diff < 0) return '—'
  const d = Math.floor(diff / 86400000)
  const h = Math.floor((diff % 86400000) / 3600000)
  const m = Math.floor((diff % 3600000) / 60000)
  if (d > 0) return `${d} 天 ${h} 小时`
  if (h > 0) return `${h} 小时 ${m} 分`
  return `${m} 分钟`
}

// 字节格式化为人类可读（B/KB/MB/GB/TB）
const formatBytes = (bytes: number) => {
  let v = bytes || 0
  if (v < 0) v = 0
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return { value: v.toFixed(2), unit: units[i] }
}

// 逐日合计的趋势线（方案 A 的 sparkline）
const sparkPoints = computed(() => {
  const s = traffic.value.daily
  if (!s.length) return ''
  const max = Math.max(...s, 0.0001)
  const n = s.length
  return s
    .map((v, i) => {
      const x = n === 1 ? 50 : (i / (n - 1)) * 100
      const y = 30 - (v / max) * 28 - 1
      return `${x.toFixed(1)},${y.toFixed(1)}`
    })
    .join(' ')
})

const quickActions = [
  { title: '流量查询', description: '查看实例入站/出站流量', icon: BarChart3, path: '/configs', variant: 'outline' as const },
  { title: '实例详情', description: '实例规格 / IP / 引导卷', icon: Eye, path: '/configs', variant: 'outline' as const },
  { title: '实时日志', description: '后端与 API 调用日志', icon: FileText, path: '/logs', variant: 'outline' as const },
  { title: '系统设置', description: '账号 / 日志 / Telegram', icon: Settings, path: '/settings', variant: 'outline' as const }
]

const loadVersion = async () => {
  try {
    const res = await sysApi.getVersion()
    if (res.data?.version) version.value = res.data.version
  } catch {
    /* 版本号非关键，失败不影响主流程 */
  }
}

const loadOverview = async () => {
  loading.value = true
  try {
    // 1. 取唯一配置
    const cfgRes = await ociApi.userPage({ page: 1, pageSize: 1 })
    const cfg = cfgRes.data.list?.[0]
    if (!cfg) {
      loading.value = false
      return
    }
    configId.value = cfg.id

    // 2. 实例列表（并行发起，失败互不影响）
    try {
      const instRes = await ociApi.detailsInstances({ configId: cfg.id })
      instances.value = instRes.data || []
    } catch {
      instances.value = []
    }

    // 3. 账号级月度流量（总流量 + 每实例明细 + 实际/计费）
    try {
      const mRes = await ociApi.monthlyTraffic(cfg.id)
      const m = mRes.data
      const total = (m.inboundTraffic || 0) + (m.outboundTraffic || 0)
      const insts: TrafficInstance[] = (m.instances || []).map((it, i) => {
        const t = (it.inbound || 0) + (it.outbound || 0)
        return {
          id: it.instanceId,
          name: it.displayName || it.instanceId,
          total: t,
          inbound: it.inbound || 0,
          outbound: it.outbound || 0,
          pct: total > 0 ? Math.round((t / total) * 100) : 0,
          color: INSTANCE_COLORS[i % INSTANCE_COLORS.length]
        }
      })
      const daily = (m.dailyLabels || []).map((_, i) => (m.dailyInbound[i] || 0) + (m.dailyOutbound[i] || 0))
      const allowancePct = m.freeAllowance > 0 ? Math.min(100, (total / m.freeAllowance) * 100) : 0
      traffic.value = {
        totalBytes: total,
        inboundBytes: m.inboundTraffic || 0,
        outboundBytes: m.outboundTraffic || 0,
        billableBytes: m.billableTraffic || 0,
        freeAllowance: m.freeAllowance || 0,
        allowancePct,
        instances: insts,
        daily
      }
    } catch {
      /* 流量查询失败不阻断概览 */
    }
  } catch (error: any) {
    toast.error(error.message || '加载概览失败')
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadVersion()
  loadOverview()
})
</script>

<template>
  <div class="space-y-8">
    <!-- Header -->
    <div v-motion :initial="{ opacity: 0, y: -20 }" :enter="{ opacity: 1, y: 0 }">
      <h1 class="text-3xl font-display font-bold">系统概览</h1>
      <p class="text-muted-foreground mt-1">欢迎回来，这是你的云资源状态</p>
    </div>

    <!-- 实例状态 -->
    <div v-motion :initial="{ opacity: 0, y: 20 }" :enter="{ opacity: 1, y: 0, transition: { delay: 100 } }">
      <div class="flex items-center gap-2 mb-3">
        <Server class="w-5 h-5 text-primary" />
        <h2 class="text-lg font-semibold">实例状态</h2>
        <span v-if="!loading" class="text-sm text-muted-foreground">
          （{{ runningCount }}/{{ instances.length }} 运行中）
        </span>
      </div>

      <div v-if="loading" class="h-24 flex items-center justify-center">
        <Activity class="w-6 h-6 animate-spin text-primary" />
      </div>
      <div v-else-if="!hasInstance" class="text-muted-foreground text-sm py-8 text-center border border-dashed rounded-lg">
        暂无实例，请在「配置管理」中添加配置并查看详情
      </div>
      <div v-else class="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <Card
          v-for="(inst, idx) in instances"
          :key="inst.id"
          v-motion
          :initial="{ opacity: 0, y: 20 }"
          :enter="{ opacity: 1, y: 0, transition: { delay: 150 + idx * 80 } }"
          class="border-border/50 hover:border-primary/40 transition-all"
        >
          <CardContent class="p-5">
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <p class="font-semibold truncate">{{ inst.displayName }}</p>
                <p class="text-xs text-muted-foreground mt-0.5">{{ inst.region }} · {{ inst.shape }}</p>
              </div>
              <span
                class="shrink-0 px-2 py-0.5 rounded-full text-xs font-medium"
                :class="{
                  'bg-success/15 text-success': stateVariant(inst.state) === 'success',
                  'bg-destructive/15 text-destructive': stateVariant(inst.state) === 'destructive',
                  'bg-warning/15 text-warning': stateVariant(inst.state) === 'warning'
                }"
              >
                {{ stateLabel(inst.state) }}
              </span>
            </div>

            <div class="grid grid-cols-2 gap-y-2 gap-x-4 mt-4 text-sm">
              <div class="flex items-center gap-2 text-muted-foreground">
                <Network class="w-4 h-4" />
                <span class="truncate">{{ inst.publicIps?.[0] || '无公网IP' }}</span>
              </div>
              <div class="flex items-center gap-2 text-muted-foreground">
                <Cpu class="w-4 h-4" />
                <span>{{ inst.ocpus }} OCPU / {{ inst.memory }} GB</span>
              </div>
              <div class="flex items-center gap-2 text-muted-foreground">
                <Clock class="w-4 h-4" />
                <span>运行 {{ formatUptime(inst.createTime) }}</span>
              </div>
              <div class="flex items-center gap-2 text-muted-foreground">
                <Activity class="w-4 h-4" />
                <span>{{ inst.imageName || '—' }}</span>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>

    <!-- 流量概览 + 快捷操作 -->
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
      <!-- 流量概览 -->
      <Card
        v-motion
        :initial="{ opacity: 0, y: 20 }"
        :enter="{ opacity: 1, y: 0, transition: { delay: 250 } }"
        class="lg:col-span-2 border-border/50 cursor-pointer hover:border-primary/40 transition-all"
        @click="router.push('/configs')"
      >
        <CardHeader class="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle class="flex items-center gap-2 text-base">
            <TrendingUp class="w-5 h-5 text-primary" />
            流量概览（本月 · 账号汇总）
          </CardTitle>
          <ArrowRight class="w-4 h-4 text-muted-foreground" />
        </CardHeader>
        <CardContent>
          <div class="flex items-end gap-8">
            <div>
              <p class="text-sm text-muted-foreground">总流量（实际）</p>
              <p class="text-2xl font-bold font-display">
                {{ formatBytes(traffic.totalBytes).value }}<span class="text-base ml-1 text-muted-foreground">{{ formatBytes(traffic.totalBytes).unit }}</span>
              </p>
            </div>
            <div>
              <p class="text-sm text-muted-foreground">计费</p>
              <p class="text-2xl font-bold font-display">
                {{ formatBytes(traffic.billableBytes).value }}<span class="text-base ml-1 text-muted-foreground">{{ formatBytes(traffic.billableBytes).unit }}</span>
              </p>
            </div>
          </div>

          <div class="mt-4 flex items-center justify-between text-xs text-muted-foreground">
            <span>免费额度 {{ formatBytes(traffic.freeAllowance).value }} {{ formatBytes(traffic.freeAllowance).unit }} · 已用 {{ traffic.allowancePct.toFixed(1) }}%</span>
            <span class="text-emerald-400">入 {{ formatBytes(traffic.inboundBytes).value }}{{ formatBytes(traffic.inboundBytes).unit }} / 出 {{ formatBytes(traffic.outboundBytes).value }}{{ formatBytes(traffic.outboundBytes).unit }}</span>
          </div>
          <div class="mt-1 h-2 rounded-full bg-slate-800 overflow-hidden">
            <div class="h-full rounded-full" :style="{ width: traffic.allowancePct + '%', background: 'linear-gradient(90deg,#22d3ee,#34d399)' }"></div>
          </div>

          <svg v-if="sparkPoints" viewBox="0 0 100 30" preserveAspectRatio="none" class="w-full h-[44px] mt-4">
            <polyline :points="sparkPoints" fill="none" stroke="#22d3ee" stroke-width="1.5" vector-effect="non-scaling-stroke" />
          </svg>
          <div v-else class="h-[44px] mt-4 flex items-center text-xs text-muted-foreground">暂无流量数据</div>

          <!-- 每实例占比 -->
          <div v-if="traffic.instances.length" class="mt-4 space-y-3">
            <div
              v-for="inst in traffic.instances"
              :key="inst.id"
              class="grid grid-cols-[120px_1fr_120px] items-center gap-3 text-sm"
            >
              <div class="flex items-center gap-2 min-w-0">
                <span class="w-2.5 h-2.5 rounded-full shrink-0" :style="{ background: inst.color }"></span>
                <span class="truncate text-foreground/90">{{ inst.name }}</span>
              </div>
              <div class="h-2 rounded-full bg-slate-800 overflow-hidden">
                <div class="h-full rounded-full" :style="{ width: inst.pct + '%', background: inst.color }"></div>
              </div>
              <div class="text-right text-muted-foreground tabular-nums">
                {{ formatBytes(inst.total).value }} {{ formatBytes(inst.total).unit }} · {{ inst.pct }}%
              </div>
            </div>
          </div>
          <div v-else class="mt-4 text-xs text-muted-foreground">暂无实例流量明细</div>
        </CardContent>
      </Card>

      <!-- 快捷操作 -->
      <Card
        v-motion
        :initial="{ opacity: 0, y: 20 }"
        :enter="{ opacity: 1, y: 0, transition: { delay: 300 } }"
        class="border-border/50"
      >
        <CardHeader>
          <CardTitle class="flex items-center gap-2">
            <Zap class="w-5 h-5 text-primary" />
            快捷操作
          </CardTitle>
        </CardHeader>
        <CardContent class="space-y-3">
          <button
            v-for="(action, index) in quickActions"
            :key="action.title"
            v-motion
            :initial="{ opacity: 0, x: -20 }"
            :enter="{ opacity: 1, x: 0, transition: { delay: 350 + index * 80 } }"
            class="h-auto py-3 px-4 w-full rounded-lg border border-border/50 bg-card/50 hover:bg-secondary/50 hover:border-primary/30 transition-all duration-300 group text-left flex items-center gap-3"
            @click="router.push(action.path)"
          >
            <div class="w-9 h-9 rounded-lg bg-secondary/80 flex items-center justify-center group-hover:bg-primary/20 transition-colors">
              <component :is="action.icon" class="w-5 h-5 text-muted-foreground group-hover:text-primary transition-colors" />
            </div>
            <div class="flex-1">
              <p class="font-semibold text-foreground text-sm">{{ action.title }}</p>
              <p class="text-xs text-muted-foreground">{{ action.description }}</p>
            </div>
            <ArrowRight class="w-4 h-4 text-muted-foreground opacity-0 group-hover:opacity-100 group-hover:translate-x-1 group-hover:text-primary transition-all" />
          </button>
        </CardContent>
      </Card>
    </div>

    <!-- 版本 -->
    <div class="flex items-center justify-center text-xs text-muted-foreground pt-2" v-if="version">
      OCI Panel · v{{ version }}
    </div>
  </div>
</template>
