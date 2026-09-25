<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useMotion } from '@vueuse/motion'
import {
  Cloud,
  LayoutDashboard,
  Settings2,
  FileText,
  Settings,
  LogOut,
  Menu,
  X,
  User,
  Clock,
  BookOpen,
  Monitor,
  Sun,
  Moon,
  Workflow
} from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { toast } from '@/composables/useToast'
import { theme } from '@/composables/useTheme'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()

const sidebarOpen = ref(false)
const currentTime = ref('')

// 主题三态：system → light → dark，按钮图标即当前状态
const THEME_META = {
  system: { icon: Monitor, label: '跟随系统', next: '切换为浅色模式' },
  light: { icon: Sun, label: '浅色模式', next: '切换为深色模式' },
  dark: { icon: Moon, label: '深色模式', next: '切换为跟随系统' }
} as const

const themeMeta = computed(() => THEME_META[theme.mode.value])

const navItems = [
  { path: '/', label: '概览', icon: LayoutDashboard },
  { path: '/configs', label: '配置管理', icon: Settings2 },
  { path: '/automation', label: '自动化', icon: Workflow },
  { path: '/logs', label: '实时日志', icon: FileText },
  { path: '/docs', label: 'API 文档', icon: BookOpen },
  { path: '/settings', label: '系统设置', icon: Settings }
]

const updateTime = () => {
  const now = new Date()
  currentTime.value = now.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit'
  })
}

let timer: ReturnType<typeof setInterval>

// ESC 关闭抽屉；窗口尺寸变化时同步抽屉状态（桌面展开 / 手机收起）。
const onKeydown = (e: KeyboardEvent) => {
  if (e.key === 'Escape' && sidebarOpen.value && window.innerWidth < 1024) {
    sidebarOpen.value = false
  }
}
const onResize = () => {
  sidebarOpen.value = window.innerWidth >= 1024
}

onMounted(() => {
  updateTime()
  timer = setInterval(updateTime, 1000)
  // 初始按当前视口宽度决定抽屉状态：桌面展开、手机收起
  sidebarOpen.value = window.innerWidth >= 1024
  window.addEventListener('keydown', onKeydown)
  window.addEventListener('resize', onResize)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
  if (typeof window !== 'undefined') {
    window.removeEventListener('keydown', onKeydown)
    window.removeEventListener('resize', onResize)
  }
})

// 手机端切换页面后自动收起侧边栏抽屉
watch(
  () => route.path,
  () => {
    if (typeof window !== 'undefined' && window.innerWidth < 1024) {
      sidebarOpen.value = false
    }
  }
)

const handleLogout = () => {
  authStore.logout()
  toast.info('已退出登录')
  router.push('/login')
}

const sidebarRef = ref<HTMLElement>()
const mainRef = ref<HTMLElement>()

// 注意：侧边栏不能用 useMotion 做入场动画——它会把 translateX 写成内联样式，
// 覆盖 Tailwind 的 -translate-x-full 类，导致抽屉无法收起/展开。
// 这里仅对主内容区做入场动画。
useMotion(mainRef, {
  initial: { opacity: 0, scale: 0.98 },
  enter: { opacity: 1, scale: 1, transition: { delay: 200, duration: 400 } }
})

const isActivePath = (path: string) => {
  if (path === '/') return route.path === '/'
  return route.path.startsWith(path)
}
</script>

<template>
  <div class="flex h-[100dvh] overflow-hidden bg-background">
    <!-- Sidebar -->
    <aside
      ref="sidebarRef"
      :class="
        cn(
          'w-64 border-r border-border/50 flex-shrink-0 transition-all duration-300 glass fixed lg:relative h-full z-40 lg:translate-x-0',
          !sidebarOpen && '-translate-x-full'
        )
      "
    >
      <div class="h-full flex flex-col">
        <!-- Logo -->
        <div class="p-6 border-b border-border/50">
          <div class="flex items-center gap-3">
            <div
              class="w-10 h-10 bg-gradient-to-br from-primary to-cyan-400 rounded-xl flex items-center justify-center"
            >
              <Cloud class="w-6 h-6 text-white" />
            </div>
            <div>
              <h1 class="text-xl font-display font-bold text-primary">OCI Panel</h1>
            </div>
          </div>
        </div>

        <!-- Navigation -->
        <nav class="flex-1 p-4 space-y-1 overflow-y-auto">
          <router-link
            v-for="(item, index) in navItems"
            :key="item.path"
            :to="item.path"
            v-motion
            :initial="{ opacity: 0, x: -20 }"
            :enter="{ opacity: 1, x: 0, transition: { delay: 100 + index * 50 } }"
            :class="
              cn(
                'flex items-center gap-3 px-4 py-3 rounded-lg transition-all duration-200 group',
                isActivePath(item.path)
                  ? 'bg-primary/10 text-primary border border-primary/30'
                  : 'text-muted-foreground hover:bg-secondary hover:text-foreground'
              )
            "
          >
            <component
              :is="item.icon"
              :class="
                cn('w-5 h-5 transition-transform group-hover:scale-110', isActivePath(item.path) && 'text-primary')
              "
            />
            <span class="font-medium">{{ item.label }}</span>
          </router-link>
        </nav>

        <!-- User Info -->
        <div class="p-4 border-t border-border/50">
          <div class="flex items-center gap-3 mb-3 px-2">
            <div
              class="w-10 h-10 bg-gradient-to-br from-success to-emerald-400 rounded-full flex items-center justify-center"
            >
              <User class="w-6 h-6 text-success-foreground" />
            </div>
            <div class="flex-1 min-w-0">
              <p class="text-sm font-medium truncate">{{ authStore.user?.username || '管理员' }}</p>
              <p class="text-xs text-muted-foreground">系统管理员</p>
            </div>
          </div>
          <Button variant="secondary" class="w-full justify-start gap-2" @click="handleLogout">
            <LogOut class="w-4 h-4" />
            退出登录
          </Button>
        </div>
      </div>
    </aside>

    <!-- Sidebar Overlay (Mobile) -->
    <Transition
      enter-active-class="transition-opacity duration-300"
      leave-active-class="transition-opacity duration-300"
      enter-from-class="opacity-0"
      leave-to-class="opacity-0"
    >
      <div v-if="sidebarOpen" class="fixed inset-0 bg-black/50 z-30 lg:hidden" @click="sidebarOpen = false" />
    </Transition>

    <!-- Main Content -->
    <div ref="mainRef" class="flex-1 flex flex-col overflow-hidden">
      <!-- Topbar -->
      <header
        class="h-16 border-b border-border/50 flex items-center justify-between px-4 sm:px-6 bg-background flex-shrink-0"
      >
        <div class="flex items-center gap-4">
          <Button variant="ghost" size="icon" class="lg:hidden" @click="sidebarOpen = !sidebarOpen">
            <Menu v-if="!sidebarOpen" class="w-5 h-5" />
            <X v-else class="w-5 h-5" />
          </Button>
          <h2 class="text-lg font-semibold hidden sm:block">
            {{ navItems.find(item => isActivePath(item.path))?.label || '概览' }}
          </h2>
        </div>

        <div class="flex items-center gap-4">
          <Button
            variant="ghost"
            size="icon"
            class="text-muted-foreground hover:text-foreground"
            :title="`${themeMeta.label} · ${themeMeta.next}`"
            :aria-label="themeMeta.label"
            @click="theme.toggle()"
          >
            <component :is="themeMeta.icon" class="w-5 h-5" />
          </Button>
          <div class="flex items-center gap-2 text-sm text-muted-foreground">
            <Clock class="w-4 h-4" />
            <span class="font-mono whitespace-nowrap">{{ currentTime }}</span>
          </div>
        </div>
      </header>

      <!-- Page Content -->
      <main class="flex-1 overflow-y-auto p-4 sm:p-6">
        <router-view v-slot="{ Component }">
          <Transition
            enter-active-class="transition-all duration-300"
            leave-active-class="transition-all duration-200"
            enter-from-class="opacity-0 translate-y-4"
            leave-to-class="opacity-0"
            mode="out-in"
          >
            <component :is="Component" />
          </Transition>
        </router-view>
      </main>
    </div>
  </div>
</template>
