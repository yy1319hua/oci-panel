<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { Settings2, ListTodo, Plus, Server, ArrowRight, Activity, TrendingUp, Zap } from 'lucide-vue-next'
import { sysApi } from '@/api'
import { toast } from '@/composables/useToast'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

const router = useRouter()

const stats = ref({
  totalConfigs: 0,
  totalTasks: 0
})

const loading = ref(true)

const loadStats = async () => {
  try {
    const response = await sysApi.getGlance()
    if (response.data) {
      stats.value = response.data
    }
  } catch (error: any) {
    console.error('加载统计数据失败:', error)
    toast.error('加载统计数据失败')
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadStats()
})

const statsCards = [
  {
    title: '配置总数',
    key: 'totalConfigs',
    icon: Settings2,
    gradient: 'from-primary to-cyan-400',
    bgGradient: 'from-primary/10 to-cyan-400/10'
  },
  {
    title: '任务总数',
    key: 'totalTasks',
    icon: ListTodo,
    gradient: 'from-warning to-amber-400',
    bgGradient: 'from-warning/10 to-amber-400/10'
  }
]

const quickActions = [
  {
    title: '添加配置',
    description: '创建新的OCI账户配置',
    icon: Plus,
    path: '/configs',
    variant: 'outline' as const
  },
  {
    title: '管理实例',
    description: '查看和管理云服务器实例',
    icon: Server,
    path: '/configs',
    variant: 'outline' as const
  }
]
</script>

<template>
  <div class="space-y-8">
    <!-- Header -->
    <div v-motion :initial="{ opacity: 0, y: -20 }" :enter="{ opacity: 1, y: 0 }">
      <h1 class="text-3xl font-display font-bold">系统概览</h1>
      <p class="text-muted-foreground mt-1">欢迎回来，查看您的云资源状态</p>
    </div>

    <!-- Stats Grid -->
    <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
      <Card
        v-for="(card, index) in statsCards"
        :key="card.key"
        v-motion
        :initial="{ opacity: 0, y: 20, scale: 0.95 }"
        :enter="{ opacity: 1, y: 0, scale: 1, transition: { delay: 100 + index * 100 } }"
        class="group relative overflow-hidden border-border/50 hover:border-primary/50 transition-all duration-300"
      >
        <!-- Background Gradient -->
        <div
          :class="`absolute inset-0 bg-gradient-to-br ${card.bgGradient} opacity-0 group-hover:opacity-100 transition-opacity duration-500`"
        />

        <CardContent class="p-6 relative">
          <div class="flex items-center gap-4">
            <div
              :class="`w-14 h-14 bg-gradient-to-br ${card.gradient} rounded-xl flex items-center justify-center shadow-lg group-hover:scale-110 transition-transform duration-300`"
            >
              <component :is="card.icon" class="w-7 h-7 text-primary-foreground" />
            </div>
            <div>
              <p class="text-sm text-muted-foreground">{{ card.title }}</p>
              <p class="text-3xl font-bold font-display mt-1">
                <template v-if="loading">
                  <span class="inline-block w-12 h-8 bg-secondary rounded animate-pulse" />
                </template>
                <template v-else>
                  {{ stats[card.key as keyof typeof stats] }}
                </template>
              </p>
            </div>
          </div>

          <!-- Decorative Element -->
          <div
            class="absolute -right-8 -bottom-8 w-32 h-32 rounded-full bg-gradient-to-br opacity-10 group-hover:opacity-20 transition-opacity"
            :class="card.gradient"
          />
        </CardContent>
      </Card>
    </div>

    <!-- Quick Actions -->
    <Card
      v-motion
      :initial="{ opacity: 0, y: 20 }"
      :enter="{ opacity: 1, y: 0, transition: { delay: 300 } }"
      class="border-border/50"
    >
      <CardHeader>
        <CardTitle class="flex items-center gap-2">
          <Zap class="w-5 h-5 text-primary" />
          快速操作
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <button
            v-for="(action, index) in quickActions"
            :key="action.title"
            v-motion
            :initial="{ opacity: 0, x: -20 }"
            :enter="{ opacity: 1, x: 0, transition: { delay: 400 + index * 100 } }"
            class="h-auto py-4 px-6 rounded-lg border border-border/50 bg-card/50 hover:bg-secondary/50 hover:border-primary/30 transition-all duration-300 group text-left"
            @click="router.push(action.path)"
          >
            <div class="flex items-center gap-4 w-full">
              <div
                class="w-10 h-10 rounded-lg bg-secondary/80 flex items-center justify-center group-hover:bg-primary/20 transition-colors"
              >
                <component
                  :is="action.icon"
                  class="w-5 h-5 text-muted-foreground group-hover:text-primary transition-colors"
                />
              </div>
              <div class="flex-1">
                <p class="font-semibold text-foreground">{{ action.title }}</p>
                <p class="text-xs text-muted-foreground">{{ action.description }}</p>
              </div>
              <ArrowRight
                class="w-4 h-4 text-muted-foreground opacity-0 group-hover:opacity-100 group-hover:translate-x-1 group-hover:text-primary transition-all"
              />
            </div>
          </button>
        </div>
      </CardContent>
    </Card>

    <!-- System Status -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
      <Card
        v-motion
        :initial="{ opacity: 0, y: 20 }"
        :enter="{ opacity: 1, y: 0, transition: { delay: 500 } }"
        class="border-border/50"
      >
        <CardContent class="p-6">
          <div class="flex items-center justify-between">
            <div>
              <p class="text-sm text-muted-foreground">系统状态</p>
              <p class="text-lg font-semibold mt-1 text-success">正常运行</p>
            </div>
            <div class="w-10 h-10 rounded-full bg-success/10 flex items-center justify-center">
              <Activity class="w-5 h-5 text-success" />
            </div>
          </div>
        </CardContent>
      </Card>

      <Card
        v-motion
        :initial="{ opacity: 0, y: 20 }"
        :enter="{ opacity: 1, y: 0, transition: { delay: 600 } }"
        class="border-border/50"
      >
        <CardContent class="p-6">
          <div class="flex items-center justify-between">
            <div>
              <p class="text-sm text-muted-foreground">API状态</p>
              <p class="text-lg font-semibold mt-1 text-success">连接正常</p>
            </div>
            <div class="w-10 h-10 rounded-full bg-success/10 flex items-center justify-center">
              <TrendingUp class="w-5 h-5 text-success" />
            </div>
          </div>
        </CardContent>
      </Card>

      <Card
        v-motion
        :initial="{ opacity: 0, y: 20 }"
        :enter="{ opacity: 1, y: 0, transition: { delay: 700 } }"
        class="border-border/50"
      >
        <CardContent class="p-6">
          <div class="flex items-center justify-between">
            <div>
              <p class="text-sm text-muted-foreground">版本</p>
              <p class="text-lg font-semibold mt-1 font-mono">v1.0.0</p>
            </div>
            <div class="w-10 h-10 rounded-full bg-primary/10 flex items-center justify-center">
              <Server class="w-5 h-5 text-primary" />
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  </div>
</template>
