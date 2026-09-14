<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { RefreshCw, Copy, Download, Trash2 } from 'lucide-vue-next'
import { toast } from '@/composables/useToast'
import { useAuthStore } from '@/stores/auth'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { sysApi } from '@/api'

/**
 * 日志页采用「混合模式」：首屏先走 HTTP 同步拉取服务端缓冲的历史日志，
 * 页面立刻有内容；随后建立 SSE 长连接，转为实时推送。
 *
 * 这样做的原因：SSE 建连虽然也会回放历史，但从「发起请求」到「首帧到达」仍有往返延迟。
 * 只依赖它做首屏，用户会先看到一段空白 —— 即使连接永远失败，那段空白也解释不了
 * 「为什么没日志」。而 HTTP 接口成功即代表服务端活着，失败也能给出明确错误。
 *
 * 【为什么用 SSE 而不是 WebSocket】
 * WebSocket 需要 HTTP Upgrade 后维持双向长连接，经 Cloudflare 隧道（尤其跨境链路）
 * 时非常脆弱，容易被中途掐断成 502；且服务端向同一连接并发写会 panic 崩进程。
 * SSE 是普通 HTTP 长连接，走标准 HTTP 语义，Cloudflare 支持稳定得多。
 *
 * 这里用 fetch + ReadableStream 而非浏览器原生 EventSource，原因是 EventSource
 * 无法携带自定义请求头 —— 而本接口要求 Authorization: Bearer <token>。
 */

type LogLevel = 'INFO' | 'WARN' | 'ERROR' | 'DEBUG' | 'SUCCESS'

interface LogEntry {
  seq: number
  ts: string
  level: LogLevel
  message: string
}

const authStore = useAuthStore()
const logs = ref<LogEntry[]>([])
const isConnected = ref(false)
const connecting = ref(false)
const loadingHistory = ref(false)
const logConsole = ref<HTMLElement>()

let disposed = false
let manualClose = false // 用户主动断开后不再自动重连
let streamSeq = 0 // 当前有效流的序号，用于作废过期回调
let retries = 0 // 自动重连次数
let reconnectTimer: ReturnType<typeof setTimeout> | null = null
let seqCounter = 0 // 日志条目的稳定唯一键
let abortController: AbortController | null = null // 用于主动中断 SSE 流

// 回放收口机制：服务端在建连后会连续推历史行，之后才转入实时。
// 两种行没有分隔标志，故用「静默窗口」判定回放结束 —— 一旦指定时间内
// 没有新行到达，就把这批行当作历史批次合并去重；若期间来了实时行，
// 也只是稍晚一点落到视图上，不会丢。
let replayBuffer: string[] = []
let replaying = false
let replayTimer: ReturnType<typeof setTimeout> | null = null
const REPLAY_IDLE_MS = 250

// 日志缓冲上限：实时日志流可能无限增长，超过上限后丢弃最旧的条目，
// 避免 DOM 节点与内存无界膨胀导致页面卡顿。
const MAX_LOG_ENTRIES = 2000
const MAX_RETRIES = 12
const RECONNECT_DELAY = 3000
const INITIAL_HISTORY_LINES = 500

// —— 筛选状态 ——
// 搜索关键字：只显示包含关键字的行（不是高亮，而是过滤）。
const keyword = ref('')
const levelFilter = ref<'ALL' | LogLevel>('ALL')
const lineLimit = ref(200)
const autoRefresh = ref(true)

// 新日志提示：用户上翻查看历史时不应被强行拽回底部。
const pendingCount = ref(0)
const atBottom = ref(true)

const levelOptions: Array<'ALL' | LogLevel> = ['ALL', 'INFO', 'SUCCESS', 'WARN', 'ERROR', 'DEBUG']
const lineOptions = [100, 200, 500, 1000, 2000]

/**
 * 过滤后的日志：按级别 + 关键字筛选，再截取最后 N 行。
 *
 * 顺序很关键 —— 必须先筛选再截断。若先截断，用户搜索一个只出现在
 * 较早位置的词时会得到空结果，而实际上匹配行是存在的。
 */
const filteredLogs = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  let result = logs.value

  if (levelFilter.value !== 'ALL') {
    result = result.filter(l => l.level === levelFilter.value)
  }
  if (kw) {
    result = result.filter(l => l.message.toLowerCase().includes(kw))
  }
  if (result.length > lineLimit.value) {
    result = result.slice(result.length - lineLimit.value)
  }
  return result
})

const isFiltering = computed(() => keyword.value.trim() !== '' || levelFilter.value !== 'ALL')

/** 解析服务端的 "[时间] LEVEL: 内容" 格式；无法识别时按 INFO 处理。 */
const parseServerLine = (raw: string): Omit<LogEntry, 'seq'> => {
  const m = raw.match(/^\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\]\s*(INFO|WARN|ERROR|DEBUG|SUCCESS)\s*[:：]\s*(.*)$/s)
  if (m) {
    return { ts: m[1], level: m[2] as LogLevel, message: m[3] }
  }
  return { ts: '', level: 'INFO', message: raw }
}

const scrollToBottom = () => {
  nextTick(() => {
    if (logConsole.value) {
      logConsole.value.scrollTop = logConsole.value.scrollHeight
    }
  })
}

/**
 * 追加一条日志。
 * 只有在用户本就停留在底部（或自动刷新开启且未上翻）时才自动滚动，
 * 否则只累加「N 条新日志」提示，避免打断用户查看历史。
 */
const appendLine = (raw: string) => {
  const parsed = parseServerLine(raw)
  logs.value.push({ seq: ++seqCounter, ...parsed })
  if (logs.value.length > MAX_LOG_ENTRIES) {
    logs.value.splice(0, logs.value.length - MAX_LOG_ENTRIES)
  }

  if (!autoRefresh.value) return
  if (atBottom.value) {
    scrollToBottom()
  } else {
    pendingCount.value++
  }
}

// onScroll 记录用户是否停留在底部（留 24px 容差，避免像素级抖动误判）。
const handleScroll = () => {
  const el = logConsole.value
  if (!el) return
  const distance = el.scrollHeight - el.scrollTop - el.clientHeight
  atBottom.value = distance < 24
  if (atBottom.value) pendingCount.value = 0
}

const jumpToLatest = () => {
  atBottom.value = true
  pendingCount.value = 0
  scrollToBottom()
}

const clearReconnect = () => {
  if (reconnectTimer !== null) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
}

const scheduleReplayFlush = () => {
  if (replayTimer !== null) clearTimeout(replayTimer)
  replayTimer = setTimeout(flushReplay, REPLAY_IDLE_MS)
}

/** 结束回放窗口：把暂存的历史行去重合并进视图，之后转入实时追加模式。 */
const flushReplay = () => {
  if (replayTimer !== null) {
    clearTimeout(replayTimer)
    replayTimer = null
  }
  if (!replaying) return
  replaying = false
  const batch = replayBuffer
  replayBuffer = []
  if (batch.length) mergeWithoutDuplicates(batch)
}

const replaceLogs = (lines: string[]) => {
  logs.value = lines.map(raw => ({ seq: ++seqCounter, ...parseServerLine(raw) }))
  pendingCount.value = 0
  scrollToBottom()
}

/**
 * 合并一批日志行，按内容跳过尾部已有的重复行。
 *
 * 为什么需要去重：首屏已经通过 HTTP 拉到最近 N 行，而 WebSocket 建连时
 * 服务端还会把缓冲区里的历史再回放一遍 —— 两批数据高度重叠。若直接追加，
 * 用户会看到成对的重复日志。
 *
 * 做法是拿新批次的首行去「当前视图尾部若干行」里找锚点，找到就把锚点之后
 * 的部分接上。只回看固定窗口（而非全表比对）是为了保持 O(窗口) 复杂度，
 * 日志高频推送时不至于卡住主线程。
 */
const mergeWithoutDuplicates = (lines: string[]) => {
  if (!lines.length) return

  const ANCHOR_WINDOW = 400
  const existing = logs.value
  const tailStart = Math.max(0, existing.length - ANCHOR_WINDOW)
  const first = lines[0]
  let anchor = -1
  for (let i = existing.length - 1; i >= tailStart; i--) {
    if (rawOf(existing[i]) === first) {
      anchor = i
      break
    }
  }

  if (anchor < 0) {
    // 完全没重叠（例如期间被 clear 过）：整批追加。
    for (const raw of lines) appendLine(raw)
    return
  }

  // 锚点之后的既有内容与这批的开头部分重复，按长度对齐后只追加新增部分。
  const alreadyHave = existing.length - anchor
  for (let i = alreadyHave; i < lines.length; i++) {
    appendLine(lines[i])
  }
}

/** 把内部条目还原成服务端原始行格式（用于去重比对）。 */
const rawOf = (entry: LogEntry) => (entry.ts ? `[${entry.ts}] ${entry.level}: ${entry.message}` : entry.message)

/**
 * 首屏加载历史日志（HTTP 同步）。
 * 失败不阻断流程 —— WebSocket 仍会尝试连接并回放历史，届时同样有内容。
 */
const loadHistory = async () => {
  if (!authStore.token) return
  loadingHistory.value = true
  try {
    const res = await sysApi.getRecentLogs({ lines: INITIAL_HISTORY_LINES })
    if (disposed) return
    replaceLogs(res.data?.lines ?? [])
  } catch {
    // 静默：WebSocket 回放会兜底，不必用 toast 打扰用户。
  } finally {
    if (!disposed) loadingHistory.value = false
  }
}

const scheduleReconnect = () => {
  if (disposed || manualClose || retries >= MAX_RETRIES) return
  clearReconnect()
  reconnectTimer = setTimeout(() => {
    retries++
    connectLogStream()
  }, RECONNECT_DELAY)
}

/**
 * 解析一段 SSE 文本流，把完整的帧交给 onFrame 处理，返回尚未收完的残留文本。
 *
 * SSE 帧之间以空行（\n\n）分隔；每条帧内可能含多行 `data: xxx`，
 * 以及 `:` 开头的注释行（如建连握手 `: open`、心跳 `: keepalive`）——注释行需忽略。
 *
 * 【为什么必须自己解析而非等整段响应】SSE 是流式的，服务端会持续 flush。
 * 若用 response.text() 一次性读取，等于等到连接结束才拿到数据，实时性完全丧失。
 * 这里按块累积、按空行切帧，保证每条日志一到就渲染。
 */
const consumeSSEBuffer = (buffer: string, onFrame: (data: string) => void): string => {
  // 统一换行，兼容服务端可能出现的 \r\n。
  let buf = buffer.replace(/\r\n/g, '\n')
  let sep = buf.indexOf('\n\n')
  while (sep !== -1) {
    const frame = buf.slice(0, sep)
    buf = buf.slice(sep + 2)
    handleSSEFrame(frame, onFrame)
    sep = buf.indexOf('\n\n')
  }
  return buf
}

/** 处理单个 SSE 帧：拼接所有 data 行，忽略注释行与事件名。 */
const handleSSEFrame = (frame: string, onFrame: (data: string) => void) => {
  const dataLines: string[] = []
  for (const line of frame.split('\n')) {
    if (!line || line.startsWith(':')) continue // 空行 / 注释（心跳）
    if (line.startsWith('data:')) {
      // 规范：data 字段值前置一个空格需被去掉（若存在）。
      dataLines.push(line.slice(5).replace(/^ /, ''))
    }
    // event: / id: / retry: 字段本页暂不消费，直接忽略。
  }
  if (dataLines.length) onFrame(dataLines.join('\n'))
}

/**
 * 建立 SSE 日志流。
 *
 * 用 fetch + ReadableStream 读 text/event-stream：相比 EventSource 的好处是
 * 可以自定义 Authorization 请求头（EventSource 不支持），与项目其余接口共用同一套 JWT。
 */
const connectLogStream = async () => {
  if (disposed || manualClose) return
  if (abortController) return
  if (!authStore.token) return

  const seq = ++streamSeq
  connecting.value = true
  const controller = new AbortController()
  abortController = controller

  try {
    const res = await fetch('/api/sys/logs/stream', {
      method: 'GET',
      headers: {
        Authorization: `Bearer ${authStore.token}`,
        Accept: 'text/event-stream'
      },
      signal: controller.signal
    })

    if (disposed || seq !== streamSeq) return

    if (!res.ok || !res.body) {
      // 401 由全局拦截处理不到（非 axios），这里显式提示重登。
      if (res.status === 401) {
        authStore.logout?.()
      }
      throw new Error(`日志流连接失败（HTTP ${res.status}）`)
    }

    // 收到响应头即视为已连接；服务端会立刻推一个 ": open" 握手帧。
    connecting.value = false
    isConnected.value = true
    retries = 0

    // 建连后服务端会立刻回放历史缓冲。这些行与 HTTP 首屏拉取的内容高度重叠，
    // 先暂存起来走合并去重，避免页面出现成对重复日志。
    replayBuffer = []
    replaying = true

    const reader = res.body.getReader()
    const decoder = new TextDecoder('utf-8')
    let pending = ''

    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      if (disposed || seq !== streamSeq) return
      pending += decoder.decode(value, { stream: true })
      pending = consumeSSEBuffer(pending, data => {
        if (replaying) {
          // 回放行与实时行无显式分隔，用静默窗口收口：建连后极短时间内到达的行
          // 视为回放批次，统一去重合并；之后的行按实时追加。
          replayBuffer.push(data)
          scheduleReplayFlush()
        } else {
          appendLine(data)
        }
      })
    }
  } catch (err) {
    // 主动中断（切页/手动断开）导致的 abort 不算错误。
    if (controller.signal.aborted || disposed || seq !== streamSeq) return
    // 其余情况（网络断开/隧道抖动）交给重连。
  } finally {
    if (seq === streamSeq) {
      flushReplay()
      abortController = null
      connecting.value = false
      isConnected.value = false
      if (!manualClose && !disposed) scheduleReconnect()
    }
  }
}

const disconnectLogStream = () => {
  manualClose = true
  clearReconnect()
  flushReplay()
  streamSeq++ // 作废当前流的所有回调
  connecting.value = false
  isConnected.value = false
  if (abortController) {
    abortController.abort()
    abortController = null
  }
}

/** 手动刷新：重新拉一次服务端历史（清掉当前视图），并确保实时流已连上。 */
const refresh = async () => {
  await loadHistory()
  if (!abortController && !connecting.value) {
    manualClose = false
    retries = 0
    connectLogStream()
  }
  toast.success('日志已刷新')
}

const clearLogs = () => {
  logs.value = []
  pendingCount.value = 0
  toast.info('当前视图已清空（服务端缓冲不受影响）')
}

const copyLogs = async () => {
  const text = filteredLogs.value.map(l => (l.ts ? `[${l.ts}] ${l.level}: ${l.message}` : l.message)).join('\n')
  if (!text) {
    toast.info('没有可复制的日志')
    return
  }
  try {
    await navigator.clipboard.writeText(text)
    toast.success(`已复制 ${filteredLogs.value.length} 行日志`)
  } catch {
    toast.error('复制失败，请检查浏览器剪贴板权限')
  }
}

const downloadLogs = () => {
  if (!filteredLogs.value.length) {
    toast.info('没有可下载的日志')
    return
  }
  const text = filteredLogs.value.map(l => (l.ts ? `[${l.ts}] ${l.level}: ${l.message}` : l.message)).join('\n')
  const blob = new Blob([text], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  const stamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19)
  a.href = url
  a.download = `panel-log-${stamp}.log`
  // 必须挂到文档里再点击：部分浏览器对未挂载的 <a> 不触发下载。
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  // 延迟释放：click 触发的下载是异步的，立即 revoke 会让部分浏览器
  // （Safari / Firefox）拿到空文件。
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

const levelClass = (level: LogLevel) => {
  switch (level) {
    case 'ERROR':
      return 'text-destructive'
    case 'WARN':
      return 'text-warning'
    case 'SUCCESS':
      return 'text-success'
    case 'DEBUG':
      return 'text-muted-foreground'
    default:
      return 'text-foreground'
  }
}

// 自动刷新关闭时，正在上翻的用户不应被新日志打扰；
// 打开时若已停在底部，则立刻滚到最新。
watch(autoRefresh, enabled => {
  if (enabled && atBottom.value) {
    pendingCount.value = 0
    scrollToBottom()
  }
})

onMounted(async () => {
  // 先同步拉历史（立即出内容），再建立 SSE 长连接转实时推送。
  manualClose = false
  await loadHistory()
  connectLogStream()
})

onUnmounted(() => {
  disposed = true
  clearReconnect()
  if (replayTimer !== null) {
    clearTimeout(replayTimer)
    replayTimer = null
  }
  disconnectLogStream()
})
</script>

<template>
  <div class="space-y-6">
    <!-- Header -->
    <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4">
      <div class="flex items-center gap-3 flex-wrap">
        <h1 class="text-2xl sm:text-3xl font-display font-bold">面板日志</h1>
        <span class="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
          <span class="relative flex h-2 w-2">
            <span
              v-if="isConnected"
              class="animate-ping absolute inline-flex h-full w-full rounded-full bg-success opacity-75"
            />
            <span
              class="relative inline-flex rounded-full h-2 w-2"
              :class="isConnected ? 'bg-success' : connecting ? 'bg-warning' : 'bg-muted-foreground'"
            />
          </span>
          {{ isConnected ? '已连接 · 实时推送' : connecting ? '连接中' : '未连接' }}
        </span>
        <span class="text-xs text-muted-foreground">
          共 {{ logs.length }} 行<template v-if="isFiltering">，匹配 {{ filteredLogs.length }} 行</template>
        </span>
      </div>
      <div class="flex flex-wrap gap-2">
        <Button variant="outline" size="sm" :disabled="loadingHistory" @click="refresh">
          <RefreshCw class="w-4 h-4" :class="loadingHistory ? 'animate-spin' : ''" />
          刷新
        </Button>
        <Button variant="outline" size="sm" @click="copyLogs">
          <Copy class="w-4 h-4" />
          复制
        </Button>
        <Button variant="outline" size="sm" @click="downloadLogs">
          <Download class="w-4 h-4" />
          下载
        </Button>
        <Button variant="outline" size="sm" @click="clearLogs">
          <Trash2 class="w-4 h-4" />
          清空
        </Button>
      </div>
    </div>

    <!-- Console Card -->
    <Card class="border-border/50">
      <CardContent class="p-0">
        <!-- 工具栏 -->
        <div class="flex flex-wrap items-center gap-2 border-b border-border/50 px-4 py-3">
          <input
            v-model="keyword"
            type="text"
            placeholder="搜索日志内容（只显示匹配行）"
            class="h-8 flex-1 min-w-[180px] rounded-md border border-border/60 bg-background px-3 text-sm
                   placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary"
          />
          <select
            v-model="levelFilter"
            class="h-8 rounded-md border border-border/60 bg-background px-2 text-sm focus:outline-none"
          >
            <option v-for="lv in levelOptions" :key="lv" :value="lv">
              {{ lv === 'ALL' ? '全部级别' : lv }}
            </option>
          </select>
          <select
            v-model.number="lineLimit"
            class="h-8 rounded-md border border-border/60 bg-background px-2 text-sm focus:outline-none"
          >
            <option v-for="n in lineOptions" :key="n" :value="n">最近 {{ n }} 行</option>
          </select>
          <label class="inline-flex items-center gap-1.5 text-xs text-muted-foreground select-none cursor-pointer">
            <input v-model="autoRefresh" type="checkbox" class="accent-primary" />
            自动刷新
          </label>
        </div>

        <!-- 日志区 -->
        <div class="relative">
          <div
            ref="logConsole"
            class="bg-background p-4 h-[60vh] sm:h-[620px] overflow-y-auto font-mono text-xs
                   leading-relaxed antialiased"
            @scroll.passive="handleScroll"
          >
            <div
              v-for="log in filteredLogs"
              :key="log.seq"
              class="flex gap-2 py-[1px] hover:bg-muted/40"
              :class="levelClass(log.level)"
            >
              <span v-if="log.ts" class="shrink-0 text-muted-foreground/70">{{ log.ts }}</span>
              <span class="shrink-0 w-16 font-semibold" :class="levelClass(log.level)">{{ log.level }}</span>
              <span class="whitespace-pre-wrap break-words min-w-0">{{ log.message }}</span>
            </div>

            <div v-if="!filteredLogs.length" class="text-muted-foreground text-center py-12">
              <p v-if="loadingHistory">正在加载历史日志...</p>
              <p v-else-if="isFiltering">没有匹配的日志行</p>
              <p v-else-if="isConnected">等待日志输出...</p>
              <p v-else>暂无日志</p>
            </div>
          </div>

          <!-- 新日志提示：上翻时不强拽回底部，点一下才回最新 -->
          <button
            v-if="pendingCount > 0"
            class="absolute bottom-4 left-1/2 -translate-x-1/2 rounded-full bg-primary px-3 py-1.5
                   text-xs font-medium text-primary-foreground shadow-lg hover:opacity-90"
            @click="jumpToLatest"
          >
            ↓ {{ pendingCount }} 条新日志
          </button>
        </div>
      </CardContent>
    </Card>
  </div>
</template>
