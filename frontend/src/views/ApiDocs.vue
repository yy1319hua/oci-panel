<script setup lang="ts">
import { ref, computed } from 'vue'
import { RouterLink } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { Copy, Check, KeyRound, Server, Globe, Terminal, Lock, Unlock, ArrowLeft, Cloud } from 'lucide-vue-next'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { toast } from '@/composables/useToast'

// API 文档页（免登录公开）：列出全部接口，供机器人 / AI 对接使用。
// 「复制全部」按钮输出完整 Markdown 文本，可直接粘贴给 AI。

interface ApiEndpoint {
  method: string
  path: string
  desc: string
  auth: 'token' | 'public'
  body?: string
}

interface ApiGroup {
  name: string
  icon: any
  desc: string
  endpoints: ApiEndpoint[]
}

const origin = computed(() => (typeof window !== 'undefined' ? window.location.origin : 'https://panel.example.com'))
const authStore = useAuthStore()

const groups: ApiGroup[] = [
  {
    name: '认证与令牌',
    icon: KeyRound,
    desc: '登录获取 JWT（管理员），或使用后台生成的 API Token',
    endpoints: [
      { method: 'POST', path: '/api/sys/login', desc: '管理员登录，返回 JWT', auth: 'public', body: '{"account":"admin","password":"***"}' },
      { method: 'POST', path: '/api/token', desc: '创建 API Token（仅管理员 JWT，明文仅返回一次）', auth: 'token', body: '{"name":"bot","expiresInDays":30,"scope":"full"}' },
      { method: 'GET', path: '/api/token/list', desc: '列出所有 API Token', auth: 'token' },
      { method: 'POST', path: '/api/token/revoke', desc: '吊销指定 Token', auth: 'token', body: '{"id":1}' }
    ]
  },
  {
    name: '机器人摘要',
    icon: Globe,
    desc: '为机器人 / 第三方提供的紧凑只读接口',
    endpoints: [
      { method: 'GET', path: '/api/bot/summary', desc: '实例概况摘要（配置数 / 实例数 / 运行数 / 公网 IP）', auth: 'token' }
    ]
  },
  {
    name: '配置管理',
    icon: Server,
    desc: 'OCI 配置的增删改查与详情',
    endpoints: [
      { method: 'POST', path: '/api/oci/userPage', desc: '分页查询配置列表', auth: 'token', body: '{"page":1,"pageSize":10,"username":""}' },
      { method: 'POST', path: '/api/oci/addCfg', desc: '新增配置', auth: 'token' },
      { method: 'POST', path: '/api/oci/updateCfgName', desc: '更新配置名称 / 密钥', auth: 'token' },
      { method: 'POST', path: '/api/oci/removeCfg', desc: '删除配置', auth: 'token', body: '{"ids":["1"]}' },
      { method: 'POST', path: '/api/oci/details', desc: '获取配置详情', auth: 'token', body: '{"id":"1"}' },
      { method: 'POST', path: '/api/oci/details/instances', desc: '获取配置下的实例列表', auth: 'token', body: '{"configId":"1"}' },
      { method: 'POST', path: '/api/oci/details/volumes', desc: '获取配置下的引导卷', auth: 'token' },
      { method: 'POST', path: '/api/oci/details/vcns', desc: '获取配置下的 VCN 网络', auth: 'token' },
      { method: 'POST', path: '/api/oci/tenant/info', desc: '获取租户信息', auth: 'token' }
    ]
  },
  {
    name: '实例管理',
    icon: Server,
    desc: '实例的查询、开停机、换 IP 等操作',
    endpoints: [
      { method: 'POST', path: '/api/instance/list', desc: '列出实例', auth: 'token', body: '{"userId":"1","compartmentId":"ocid1..."}' },
      { method: 'POST', path: '/api/instance/start', desc: '启动实例', auth: 'token', body: '{"userId":"1","instanceId":"ocid1..."}' },
      { method: 'POST', path: '/api/instance/stop', desc: '停止实例', auth: 'token' },
      { method: 'POST', path: '/api/instance/reboot', desc: '重启实例', auth: 'token' },
      { method: 'POST', path: '/api/instance/terminate', desc: '删除实例（不可恢复）', auth: 'token' },
      { method: 'POST', path: '/api/instance/updateName', desc: '修改实例名称', auth: 'token' },
      { method: 'POST', path: '/api/instance/changeIP', desc: '更换公网 IP', auth: 'token' },
      { method: 'POST', path: '/api/instance/attachIPv6', desc: '附加 IPv6', auth: 'token' }
    ]
  },
  {
    name: '系统',
    icon: Terminal,
    desc: '系统状态与缓存',
    endpoints: [
      { method: 'GET', path: '/api/sys/getGlance', desc: '总览统计（配置数 / 任务数）', auth: 'token' },
      { method: 'GET', path: '/api/sys/getSysCfg', desc: '系统配置', auth: 'token' },
      { method: 'POST', path: '/api/sys/refreshCache', desc: '手动刷新缓存', auth: 'token' }
    ]
  }
]

// 生成可直接发给 AI 的完整 Markdown 文档
const buildMarkdown = () => {
  const lines: string[] = []
  lines.push('# OCI Panel API 文档')
  lines.push('')
  lines.push(`Base URL: ${origin.value}`)
  lines.push('')
  lines.push('## 认证')
  lines.push('')
  lines.push('所有需要鉴权的接口，在请求头携带：')
  lines.push('')
  lines.push('```')
  lines.push('Authorization: Bearer <API_TOKEN>')
  lines.push('Content-Type: application/json')
  lines.push('```')
  lines.push('')
  lines.push('- API Token 在面板「系统设置 → API 令牌」生成，等价于管理员私钥。')
  lines.push(`- 例：\`curl -s ${origin.value}/api/bot/summary -H "Authorization: Bearer $TOKEN"\``)
  lines.push('')
  groups.forEach(g => {
    lines.push(`## ${g.name}`)
    lines.push('')
    lines.push(g.desc)
    lines.push('')
    lines.push('| 方法 | 路径 | 说明 | 鉴权 |')
    lines.push('| --- | --- | --- | --- |')
    g.endpoints.forEach(e => {
      lines.push(`| ${e.method} | ${e.path} | ${e.desc} | ${e.auth === 'public' ? '公开' : 'Token'} |`)
    })
    lines.push('')
    g.endpoints.forEach(e => {
      if (e.body) {
        lines.push(`### ${e.method} ${e.path}`)
        lines.push('')
        lines.push('请求体示例：')
        lines.push('```json')
        lines.push(e.body)
        lines.push('```')
        lines.push('')
      }
    })
  })
  lines.push('## 通用响应格式')
  lines.push('')
  lines.push('```json')
  lines.push('{ "code": 200, "message": "success", "data": {} }')
  lines.push('```')
  return lines.join('\n')
}

const copied = ref(false)
const copyAll = async () => {
  const text = buildMarkdown()
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
    } else {
      const ta = document.createElement('textarea')
      ta.value = text
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      document.body.removeChild(ta)
    }
    copied.value = true
    toast.success('已复制完整 API 文档，可粘贴发送给 AI')
    setTimeout(() => (copied.value = false), 2000)
  } catch {
    toast.error('复制失败，请手动选择文本')
  }
}

const copyOne = async (e: ApiEndpoint) => {
  const text = `${e.method} ${origin.value}${e.path}\nAuthorization: Bearer <TOKEN>\n${e.body ? 'Content-Type: application/json\n\n' + e.body : ''}`
  try {
    await navigator.clipboard.writeText(text)
    toast.success('已复制')
  } catch {
    toast.error('复制失败')
  }
}
</script>

<template>
  <div class="min-h-screen bg-background">
    <!-- 顶部栏：返回面板 / 登录 -->
    <header class="sticky top-0 z-30 h-16 border-b border-border/50 flex items-center justify-between px-4 sm:px-6 glass">
      <div class="flex items-center gap-3">
        <div class="w-9 h-9 bg-gradient-to-br from-primary to-cyan-400 rounded-xl flex items-center justify-center">
          <Cloud class="w-5 h-5 text-white" />
        </div>
        <span class="font-display font-bold text-primary">OCI Panel · API 文档</span>
      </div>
      <RouterLink to="/">
        <Button variant="outline" size="sm">
          <ArrowLeft class="w-4 h-4" />
          {{ authStore.isAuthenticated ? '返回面板' : '去登录' }}
        </Button>
      </RouterLink>
    </header>

    <div class="max-w-5xl mx-auto p-4 sm:p-6 space-y-6">
    <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4">
      <div>
        <h1 class="text-3xl font-display font-bold">API 文档</h1>
        <p class="text-muted-foreground mt-1">供机器人 / AI 对接面板接口使用</p>
      </div>
      <Button @click="copyAll">
        <Check v-if="copied" class="w-4 h-4" />
        <Copy v-else class="w-4 h-4" />
        {{ copied ? '已复制' : '复制全部文档' }}
      </Button>
    </div>

    <!-- 认证说明 -->
    <Card class="border-border/50">
      <CardHeader class="border-b border-border/50">
        <CardTitle class="flex items-center gap-2">
          <KeyRound class="w-5 h-5 text-primary" />
          认证方式
        </CardTitle>
      </CardHeader>
      <CardContent class="p-4 sm:p-6 space-y-4">
        <p class="text-sm text-muted-foreground">
          除公开接口外，所有接口需在请求头携带 API Token。Token 在面板
          <span class="text-foreground font-medium">「系统设置 → API 令牌」</span> 生成，等价于管理员私钥，可随时吊销。
        </p>
        <div class="rounded-lg bg-secondary/50 p-4 font-mono text-xs sm:text-sm overflow-x-auto">
          <div>Authorization: Bearer &lt;API_TOKEN&gt;</div>
          <div>Content-Type: application/json</div>
        </div>
        <div class="rounded-lg bg-secondary/50 p-4 font-mono text-xs sm:text-sm overflow-x-auto">
          <div class="text-muted-foreground"># 示例：拉取机器人摘要</div>
          <div>curl -s {{ origin }}/api/bot/summary \</div>
          <div class="pl-4">-H "Authorization: Bearer $TOKEN"</div>
        </div>
        <div class="rounded-lg bg-secondary/50 p-4 font-mono text-xs sm:text-sm overflow-x-auto">
          <div class="text-muted-foreground"># 通用响应格式</div>
          <div>{ "code": 200, "message": "success", "data": {} }</div>
        </div>
      </CardContent>
    </Card>

    <!-- 接口分组 -->
    <Card v-for="g in groups" :key="g.name" class="border-border/50">
      <CardHeader class="border-b border-border/50">
        <CardTitle class="flex items-center gap-2">
          <component :is="g.icon" class="w-5 h-5 text-primary" />
          {{ g.name }}
          <Badge variant="secondary">{{ g.endpoints.length }}</Badge>
        </CardTitle>
        <p class="text-sm text-muted-foreground mt-1">{{ g.desc }}</p>
      </CardHeader>
      <CardContent class="p-0 divide-y divide-border/50">
        <div
          v-for="e in g.endpoints"
          :key="e.method + e.path"
          class="flex flex-wrap items-center gap-3 px-4 sm:px-6 py-3"
        >
          <Badge :variant="e.method === 'GET' ? 'info' : 'success'" class="font-mono shrink-0">{{ e.method }}</Badge>
          <code class="font-mono text-sm flex-1 min-w-0 break-all">{{ e.path }}</code>
          <Badge :variant="e.auth === 'public' ? 'secondary' : 'warning'" class="gap-1 shrink-0">
            <Unlock v-if="e.auth === 'public'" class="w-3 h-3" />
            <Lock v-else class="w-3 h-3" />
            {{ e.auth === 'public' ? '公开' : 'Token' }}
          </Badge>
          <Button variant="ghost" size="icon" title="复制此接口" @click="copyOne(e)">
            <Copy class="w-4 h-4" />
          </Button>
          <p class="w-full text-xs text-muted-foreground mt-1">{{ e.desc }}</p>
          <pre
            v-if="e.body"
            class="w-full mt-2 rounded bg-secondary/50 p-3 font-mono text-xs overflow-x-auto"
          >{{ e.body }}</pre>
        </div>
      </CardContent>
    </Card>
    </div>
  </div>
</template>
