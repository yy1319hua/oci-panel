<script setup lang="ts">
import { ref, shallowRef, onMounted, onUnmounted, nextTick } from 'vue'
import { Wifi, WifiOff, Trash2, Terminal } from 'lucide-vue-next'
import { toast } from '@/composables/useToast'
import { useAuthStore } from '@/stores/auth'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { sysApi } from '@/api'

interface LogEntry {
  message: string
  type: 'info' | 'error' | 'warning' | 'success'
}

const authStore = useAuthStore()
const logs = ref<LogEntry[]>([])
const isConnected = ref(false)
const ws = shallowRef<WebSocket | null>(null)
const connecting = ref(false)
const logConsole = ref<HTMLElement>()
let disposed = false
let manualClose = false // 用户主动断开后不再自动重连
let socketSeq = 0 // 当前有效 socket 序号，用于作废过期回调
let retries = 0 // 自动重连次数
let reconnectTimer: ReturnType<typeof setTimeout> | null = null

// 日志缓冲上限：实时日志流可能无限增长，超过上限后丢弃最旧的条目，
// 避免 DOM 节点与内存无界膨胀导致页面卡顿。
const MAX_LOG_ENTRIES = 1000
const MAX_RETRIES = 12
const RECONNECT_DELAY = 3000

const addLog = (message: string, type: LogEntry['type'] = 'info') => {
  const timestamp = new Date().toLocaleTimeString('zh-CN')
  logs.value.push({
    message: `[${timestamp}] ${message}`,
    type
  })
  if (logs.value.length > MAX_LOG_ENTRIES) {
    logs.value.splice(0, logs.value.length - MAX_LOG_ENTRIES)
  }

  nextTick(() => {
    if (logConsole.value) {
      logConsole.value.scrollTop = logConsole.value.scrollHeight
    }
  })
}

const clearReconnect = () => {
  if (reconnectTimer !== null) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
}

// appendServerLine 处理服务端推送的日志行。服务端历史回放/实时日志已是
// 「[时间] LEVEL: 内容」格式，识别后不再重复加本地时间戳，并据 LEVEL 着色。
const appendServerLine = (raw: string) => {
  const m = raw.match(/^\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\]\s*\[(INFO|WARN|ERROR|DEBUG|SUCCESS)\]\s*(.*)$/)
  if (m) {
    const typeMap: Record<string, LogEntry['type']> = {
      INFO: 'info',
      WARN: 'warning',
      ERROR: 'error',
      DEBUG: 'info',
      SUCCESS: 'success'
    }
    logs.value.push({ message: raw, type: typeMap[m[2]] ?? 'info' })
  } else {
    logs.value.push({ message: raw, type: 'info' })
  }
  if (logs.value.length > MAX_LOG_ENTRIES) {
    logs.value.splice(0, logs.value.length - MAX_LOG_ENTRIES)
  }
  nextTick(() => {
    if (logConsole.value) {
      logConsole.value.scrollTop = logConsole.value.scrollHeight
    }
  })
}

const scheduleReconnect = () => {
  if (disposed || manualClose || retries >= MAX_RETRIES) {
    if (retries >= MAX_RETRIES) {
      addLog('日志流重连次数过多，已停止自动重连，请手动点击「连接」', 'error')
    }
    return
  }
  clearReconnect()
  reconnectTimer = setTimeout(() => {
    retries++
    addLog(`正在尝试重新连接日志流（第 ${retries} 次）...`, 'warning')
    connectWebSocket()
  }, RECONNECT_DELAY)
}

const connectWebSocket = async () => {
  if (disposed || manualClose) return
  if (ws.value || connecting.value) return
  const seq = ++socketSeq
  connecting.value = true
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'

  try {
    if (!authStore.token) {
      toast.error('未登录，无法连接日志流')
      return
    }
    const ticketResponse = await sysApi.issueWebSocketTicket()
    if (disposed || seq !== socketSeq || manualClose) return
    const wsUrl = `${protocol}//${window.location.host}/ws/logs?ticket=${encodeURIComponent(ticketResponse.data.ticket)}`
    const socket = new WebSocket(wsUrl)
    ws.value = socket

    socket.onopen = () => {
      if (seq !== socketSeq || ws.value !== socket) return
      connecting.value = false
      isConnected.value = true
      retries = 0
      addLog('WebSocket 连接成功', 'success')
      toast.success('日志连接成功')
    }

    socket.onmessage = event => {
      if (seq !== socketSeq || ws.value !== socket) return
      appendServerLine(event.data)
    }

    socket.onerror = () => {
      if (seq !== socketSeq || ws.value !== socket) return
      addLog('WebSocket 连接错误', 'error')
    }

    socket.onclose = () => {
      if (seq !== socketSeq) return
      ws.value = null
      connecting.value = false
      isConnected.value = false
      addLog('WebSocket 连接已断开', 'warning')
      // 非主动断开则自动重连，保证日志持续显示
      if (!manualClose && !disposed) scheduleReconnect()
    }
  } catch {
    if (!disposed && seq === socketSeq && !manualClose) {
      toast.error('无法建立WebSocket连接')
      scheduleReconnect()
    }
  } finally {
    if (seq === socketSeq && !ws.value) connecting.value = false
  }
}

const disconnectWebSocket = (manual = false) => {
  manualClose = manual
  clearReconnect()
  socketSeq++ // 作废当前 socket 的所有回调
  connecting.value = false
  isConnected.value = false
  const socket = ws.value
  ws.value = null
  if (socket) {
    socket.onopen = null
    socket.onmessage = null
    socket.onerror = null
    socket.onclose = null
    socket.close()
    if (!disposed) addLog('WebSocket 连接已断开', 'warning')
  }
}

const toggleConnection = () => {
  if (ws.value || connecting.value) {
    disconnectWebSocket(true)
    toast.info('已断开连接')
  } else {
    manualClose = false
    retries = 0
    connectWebSocket()
  }
}

const clearLogs = () => {
  logs.value = []
  toast.info('日志已清空')
}

const getLogColor = (type: LogEntry['type']) => {
  switch (type) {
    case 'success':
      return 'text-success'
    case 'error':
      return 'text-destructive'
    case 'warning':
      return 'text-warning'
    default:
      return 'text-primary'
  }
}

onMounted(() => {
  // 进入页面即自动连接，直接显示日志，无需手动点击「连接」；
  // 切走再回来也会重新挂载并自动重连，不再需要手动操作。
  manualClose = false
  connectWebSocket()
})

onUnmounted(() => {
  disposed = true
  clearReconnect()
  disconnectWebSocket(false)
})
</script>

<template>
  <div class="space-y-6">
    <!-- Header -->
    <div
      v-motion
      :initial="{ opacity: 0, y: -20 }"
      :enter="{ opacity: 1, y: 0 }"
      class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4"
    >
      <div class="flex items-center gap-3">
        <h1 class="text-3xl font-display font-bold">实时日志</h1>
        <Badge :variant="isConnected ? 'success' : 'secondary'" class="gap-1">
          <span class="relative flex h-2 w-2">
            <span
              v-if="isConnected"
              class="animate-ping absolute inline-flex h-full w-full rounded-full bg-success opacity-75"
            />
            <span
              class="relative inline-flex rounded-full h-2 w-2"
              :class="isConnected ? 'bg-success' : 'bg-muted-foreground'"
            />
          </span>
          {{ isConnected ? '已连接' : (connecting ? '连接中' : '未连接') }}
        </Badge>
      </div>
      <div class="flex gap-2">
        <Button variant="outline" @click="clearLogs">
          <Trash2 class="w-4 h-4" />
          清空日志
        </Button>
        <Button :variant="isConnected ? 'destructive' : 'default'" :disabled="connecting" @click="toggleConnection">
          <WifiOff v-if="isConnected" class="w-4 h-4" />
          <Wifi v-else class="w-4 h-4" />
          {{ connecting ? '连接中...' : isConnected ? '断开连接' : '连接' }}
        </Button>
      </div>
    </div>

    <!-- Console Card -->
    <Card
      v-motion
      :initial="{ opacity: 0, y: 20 }"
      :enter="{ opacity: 1, y: 0, transition: { delay: 100 } }"
      class="border-border/50"
    >
      <CardHeader class="border-b border-border/50 py-3">
        <CardTitle class="flex items-center gap-2 text-base">
          <Terminal class="w-4 h-4 text-primary" />
          控制台输出
        </CardTitle>
      </CardHeader>
      <CardContent class="p-0">
        <div ref="logConsole" class="bg-background rounded-b-lg p-4 h-[60vh] sm:h-[600px] overflow-y-auto font-mono text-xs sm:text-sm">
          <div v-for="(log, index) in logs" :key="index" class="mb-1 leading-relaxed" :class="getLogColor(log.type)">
            {{ log.message }}
          </div>
          <div v-if="!logs.length" class="text-muted-foreground text-center py-8">
            <Terminal class="w-12 h-12 mx-auto mb-4 opacity-50" />
            <p v-if="connecting">正在连接日志流...</p>
            <p v-else-if="isConnected">等待日志输出...</p>
            <p v-else>未连接，请点击「连接」按钮</p>
          </div>
        </div>
      </CardContent>
    </Card>
  </div>
</template>
