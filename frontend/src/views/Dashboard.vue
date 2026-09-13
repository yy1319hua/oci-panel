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
const traffic = ref<{ inbound: number; outbound: number; unit: string; series: number[] }>({
  inbound: 0,
  outbound: 0,
  unit: 'MB',
  series: []
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

const fmtTime = (d: Date) => {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

const formatTraffic = (mb: number) => {
  if (mb >= 1024) return { value: (mb / 1024).toFixed(2), unit: 'GB' }
  return { value: mb.toFixed(2), unit: 'MB' }
}

const sparkPoints = computed(() => {
  const s = traffic.value.series
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

    // 3. 近 24h 流量（best-effort）
    try {
      const cond = await ociApi.trafficCondition(cfg.id)
      const inst = cond.data.instances?.[0]
      if (inst) {
        const vnics = await ociApi.trafficVnics({ configId: cfg.id, instanceId: inst.value })
        const vnic = vnics.data?.[0]
        if (vnic) {
          const end = new Date()
          const start = new Date(end.getTime() - 24 * 3600 * 1000)
          const data = await ociApi.trafficData({
            configId: cfg.id,
            instanceId: inst.value,
            vnicId: vnic.value,
            startTime: fmtTime(start),
            endTime: fmtTime(end)
          })
          const inbound = (data.data.inbound || []).reduce((s, v) => s + (parseFloat(v) || 0), 0)
          const outbound = (data.data.outbound || []).reduce((s, v) => s + (parseFloat(v) || 0), 0)
          const f = formatTraffic(inbound + outbound)
          traffic.value = { inbound, outbound, unit: f.unit, series: data.data.outbound?.map(v => parseFloat(v) || 0) || [] }
        }
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
            流量概览（近 24 小时）
          </CardTitle>
          <ArrowRight class="w-4 h-4 text-muted-foreground" />
        </CardHeader>
        <CardContent>
          <div class="flex items-end gap-8">
            <div>
              <p class="text-sm text-muted-foreground">入站</p>
              <p class="text-2xl font-bold font-display">{{ traffic.inbound.toFixed(2) }}<span class="text-base ml-1 text-muted-foreground">{{ traffic.unit }}</span></p>
            </div>
            <div>
              <p class="text-sm text-muted-foreground">出站</p>
              <p class="text-2xl font-bold font-display">{{ traffic.outbound.toFixed(2) }}<span class="text-base ml-1 text-muted-foreground">{{ traffic.unit }}</span></p>
            </div>
          </div>
          <div class="mt-4 h-[30px] w-full">
            <svg v-if="sparkPoints" viewBox="0 0 100 30" preserveAspectRatio="none" class="w-full h-full">
              <polyline :points="sparkPoints" fill="none" stroke="rgb(34 211 238)" stroke-width="1.5" vector-effect="non-scaling-stroke" />
            </svg>
            <div v-else class="h-full flex items-center text-xs text-muted-foreground">暂无流量数据</div>
          </div>
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
