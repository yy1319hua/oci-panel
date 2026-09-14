<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import {
  Info,
  Database,
  RefreshCw,
  Github,
  FileText,
  Server,
  Code,
  Loader2,
  FolderOpen,
  Activity,
  Send,
  Play,
  Square,
  TestTube,
  Shield,
  ShieldCheck,
  ShieldOff,
  Fingerprint,
  KeyRound,
  Trash2,
  Lock,
  Mail,
  User,
  Copy,
  Check,
  Plus,
  Clock
} from 'lucide-vue-next'
import { sysApi, telegramApi, passkeyApi, tokenApi } from '@/api'
import type { ApiToken, CreateTokenResult, TokenCallLog } from '@/api'
import { useAuthStore } from '@/stores/auth'
import { createPasskeyCredential } from '@/lib/webauthn'
import { toast } from '@/composables/useToast'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter
} from '@/components/ui/dialog'

const loading = ref(false)
const refreshing = ref(false)

const config = ref({
  keyDirPath: '',
  logLevel: ''
})
const logLevel = ref('info')
const updatingLogLevel = ref(false)

const cacheConfig = ref({
  cacheEnabled: false,
  cacheInterval: 30
})

const telegramConfig = ref({
  botToken: '',
  chatId: '',
  apiBase: '',
  enabled: false,
  running: false
})

const telegramLoading = ref(false)
const testingConnection = ref(false)
const sendingTestMessage = ref(false)

const mfaConfig = ref({
  enabled: false,
  secret: '',
  qrCode: ''
})
const mfaLoading = ref(false)
const mfaCode = ref('')
const showMfaSetup = ref(false)
const enablingMfa = ref(false)

const passkeyConfig = ref({
  enabled: false
})
const passkeyLoading = ref(false)
const registeringPasskey = ref(false)

const profile = ref({ account: '', email: '' })
const profileLoading = ref(false)
const passwordForm = ref({ oldPassword: '', newPassword: '', confirmPassword: '' })
const changingPassword = ref(false)
const emailForm = ref({ email: '' })
const savingEmail = ref(false)

const authStore = useAuthStore()
const editingAccount = ref('')
const savingAccount = ref(false)
const updateAccount = async () => {
  const newAccount = editingAccount.value.trim()
  if (!newAccount) {
    toast.error('账号不能为空')
    return
  }
  savingAccount.value = true
  try {
    const res = await sysApi.updateAccount(newAccount)
    // 后端改了账号并重签了 JWT，用新令牌刷新会话（无感重登）
    authStore.applyNewToken(res.data.token, res.data.account)
    profile.value.account = res.data.account
    editingAccount.value = ''
    toast.success('账号已更新')
  } catch (e: any) {
    toast.error(e?.response?.data?.error || '更新账号失败')
  } finally {
    savingAccount.value = false
  }
}

// ---- API 令牌管理 ----
const tokens = ref<ApiToken[]>([])
const tokensLoading = ref(false)
const createOpen = ref(false)
const creating = ref(false)
const createForm = ref({ name: '', expiresInDays: 0 })
const newToken = ref<CreateTokenResult | null>(null)
const copied = ref(false)
const revokingId = ref<number | null>(null)

// 调用记录（参考青龙面板）：按令牌查看最近的 API 调用明细
const callsOpen = ref(false)
const callsLoading = ref(false)
const callsToken = ref<ApiToken | null>(null)
const tokenCalls = ref<TokenCallLog[]>([])

const openCalls = async (t: ApiToken) => {
  callsToken.value = t
  tokenCalls.value = []
  callsOpen.value = true
  callsLoading.value = true
  try {
    const response = await tokenApi.calls(t.id)
    tokenCalls.value = response.data || []
  } catch {
    toast.error('加载调用记录失败')
  } finally {
    callsLoading.value = false
  }
}

// 状态码着色：2xx 成功、4xx 警告、5xx 错误
const statusClass = (code: number) =>
  code >= 500 ? 'text-destructive' : code >= 400 ? 'text-warning' : 'text-success'

// 调用记录拼成「一行一条」的纯文本行，风格与实时日志页保持一致：
// 2026-09-15 03:58:28 [API] GET /api/configs/list → 200 ip=1.2.3.4
// 拆成 head/tail 两段是为了让状态码单独着色。
// 【坑】Vue 会移除模板里标签之间的换行空白（是删除不是折叠），所以段间空格
// 必须写进这里的字符串（head 尾空格、tail 前导空格），容器再用 pre-wrap 保留，
// 否则会渲染成「→200ip=」。
const callHead = (c: TokenCallLog) => `${formatTime(c.createdAt)} [API] ${c.method} ${c.path} → `
const callTail = (c: TokenCallLog) => ` ip=${c.ip || '—'}`

// 按调用次数排序展示（调用多的令牌排前面更直观）
const sortedTokens = computed(() =>
  [...tokens.value].sort((a, b) => (b.callCount || 0) - (a.callCount || 0))
)

const loadTokens = async () => {
  tokensLoading.value = true
  try {
    const response = await tokenApi.list()
    if (response.data) {
      tokens.value = response.data
    }
  } catch {
    toast.error('加载 API 令牌失败')
  } finally {
    tokensLoading.value = false
  }
}

const openCreate = () => {
  createForm.value = { name: '', expiresInDays: 0 }
  newToken.value = null
  copied.value = false
  createOpen.value = true
}

const createToken = async () => {
  if (!createForm.value.name.trim()) {
    toast.error('请填写令牌名称')
    return
  }
  creating.value = true
  try {
    const response = await tokenApi.create({
      name: createForm.value.name.trim(),
      expiresInDays: Number(createForm.value.expiresInDays) || 0
    })
    if (response.data) {
      newToken.value = response.data
      copied.value = false
      await loadTokens()
    }
  } catch {
    toast.error('创建失败')
  } finally {
    creating.value = false
  }
}

const copyToken = async () => {
  if (!newToken.value) return
  try {
    await navigator.clipboard.writeText(newToken.value.token)
    copied.value = true
    toast.success('已复制到剪贴板')
  } catch {
    toast.error('复制失败，请手动选择复制')
  }
}

const revokeToken = async (id: number) => {
  if (!confirm('确定吊销该令牌？吊销后使用该令牌的机器人将立即失效。')) return
  revokingId.value = id
  try {
    await tokenApi.revoke(id)
    toast.success('令牌已吊销')
    await loadTokens()
  } catch {
    toast.error('吊销失败')
  } finally {
    revokingId.value = null
  }
}

const formatTime = (t: string | null) => (t ? t.replace('T', ' ').slice(0, 19) : '—')

const loadProfile = async () => {
  profileLoading.value = true
  try {
    const response = await sysApi.getProfile()
    if (response.data) {
      profile.value = response.data
      emailForm.value.email = response.data.email || ''
    }
  } catch {
    toast.error('加载账户资料失败')
  } finally {
    profileLoading.value = false
  }
}

const changePassword = async () => {
  if (!passwordForm.value.oldPassword || !passwordForm.value.newPassword) {
    toast.error('请填写完整')
    return
  }
  if (passwordForm.value.newPassword !== passwordForm.value.confirmPassword) {
    toast.error('两次输入的新密码不一致')
    return
  }
  if (passwordForm.value.newPassword.length < 6) {
    toast.error('新密码至少 6 位')
    return
  }
  changingPassword.value = true
  try {
    await sysApi.changePassword({
      oldPassword: passwordForm.value.oldPassword,
      newPassword: passwordForm.value.newPassword
    })
    toast.success('密码修改成功')
    passwordForm.value = { oldPassword: '', newPassword: '', confirmPassword: '' }
  } catch {
    toast.error('修改失败，请检查原密码是否正确')
  } finally {
    changingPassword.value = false
  }
}

const saveEmail = async () => {
  savingEmail.value = true
  try {
    await sysApi.updateEmail({ email: emailForm.value.email })
    profile.value.email = emailForm.value.email
    toast.success('邮箱已保存')
  } catch {
    toast.error('保存邮箱失败')
  } finally {
    savingEmail.value = false
  }
}

const loadConfig = async () => {
  loading.value = true
  try {
    const response = await sysApi.getSysCfg()
    if (response.data) {
      config.value = response.data
      logLevel.value = response.data.logLevel || 'info'
      cacheConfig.value.cacheEnabled = response.data.cacheEnabled || false
      cacheConfig.value.cacheInterval = response.data.cacheInterval || 30
    }
  } catch {
    toast.error('加载系统配置失败')
  } finally {
    loading.value = false
  }
}

const updateLogLevel = async () => {
  updatingLogLevel.value = true
  try {
    await sysApi.updateLogLevel(logLevel.value)
    config.value.logLevel = logLevel.value
    toast.success('日志级别已更新为 ' + logLevel.value)
  } catch (e: any) {
    toast.error(e?.response?.data?.error || '更新日志级别失败')
    logLevel.value = config.value.logLevel || 'info'
  } finally {
    updatingLogLevel.value = false
  }
}

const updateCacheConfig = async () => {
  try {
    await sysApi.updateCacheCfg({
      cacheEnabled: cacheConfig.value.cacheEnabled,
      cacheInterval: cacheConfig.value.cacheInterval
    })
    toast.success('缓存配置已更新')
  } catch {
    toast.error('更新缓存配置失败')
  }
}

const refreshCache = async () => {
  refreshing.value = true
  try {
    await sysApi.refreshCache()
    toast.success('缓存刷新任务已启动')
  } catch {
    toast.error('刷新缓存失败')
  } finally {
    refreshing.value = false
  }
}

const loadTelegramConfig = async () => {
  telegramLoading.value = true
  try {
    const response = await telegramApi.getConfig()
    if (response.data) {
      telegramConfig.value = response.data
    }
  } catch {
    toast.error('加载 Telegram 配置失败')
  } finally {
    telegramLoading.value = false
  }
}

const updateTelegramConfig = async () => {
  try {
    await telegramApi.updateConfig({
      botToken: telegramConfig.value.botToken,
      chatId: telegramConfig.value.chatId,
      apiBase: telegramConfig.value.apiBase,
      enabled: telegramConfig.value.enabled
    })
    toast.success('Telegram 配置已更新')
    await loadTelegramConfig()
  } catch {
    toast.error('更新 Telegram 配置失败')
  }
}

const testTelegramConnection = async () => {
  testingConnection.value = true
  try {
    await telegramApi.testConnection()
    toast.success('连接测试成功')
  } catch {
    toast.error('连接测试失败，请检查 Bot Token')
  } finally {
    testingConnection.value = false
  }
}

const sendTelegramTestMessage = async () => {
  sendingTestMessage.value = true
  try {
    await telegramApi.sendTestMessage()
    toast.success('测试消息发送成功')
  } catch {
    toast.error('发送测试消息失败')
  } finally {
    sendingTestMessage.value = false
  }
}

const startTelegramBot = async () => {
  try {
    await telegramApi.startBot()
    toast.success('Telegram Bot 已启动')
    await loadTelegramConfig()
  } catch {
    toast.error('启动 Bot 失败')
  }
}

const stopTelegramBot = async () => {
  try {
    await telegramApi.stopBot()
    toast.success('Telegram Bot 已停止')
    await loadTelegramConfig()
  } catch {
    toast.error('停止 Bot 失败')
  }
}

const loadAuthStatus = async () => {
  mfaLoading.value = true
  passkeyLoading.value = true
  try {
    const response = await sysApi.getAuthStatus()
    if (response.data) {
      mfaConfig.value.enabled = response.data.mfaEnabled
      passkeyConfig.value.enabled = response.data.passkeyEnabled
    }
  } catch {
    toast.error('加载认证状态失败')
  } finally {
    mfaLoading.value = false
    passkeyLoading.value = false
  }
}

const toggleMfaSetup = async () => {
  if (mfaConfig.value.enabled) {
    try {
      await sysApi.disableMfa()
      mfaConfig.value.enabled = false
      mfaConfig.value.secret = ''
      mfaConfig.value.qrCode = ''
      showMfaSetup.value = false
      toast.success('MFA 已禁用')
    } catch {
      toast.error('禁用 MFA 失败')
    }
  } else {
    if (passkeyConfig.value.enabled) {
      toast.error('请先禁用 Passkey 后再启用 MFA')
      return
    }
    showMfaSetup.value = true
    try {
      const response = await sysApi.generateMfaSecret()
      if (response.data) {
        mfaConfig.value.secret = response.data.secret
        mfaConfig.value.qrCode = response.data.qrCode
      }
    } catch {
      toast.error('生成 MFA 密钥失败')
      showMfaSetup.value = false
    }
  }
}

const cancelMfaSetup = () => {
  showMfaSetup.value = false
  mfaConfig.value.secret = ''
  mfaConfig.value.qrCode = ''
  mfaCode.value = ''
}

const enableMfa = async () => {
  if (!mfaCode.value || mfaCode.value.length !== 6) {
    toast.error('请输入6位验证码')
    return
  }
  enablingMfa.value = true
  try {
    await sysApi.enableMfa({
      secret: mfaConfig.value.secret,
      code: mfaCode.value
    })
    mfaConfig.value.enabled = true
    showMfaSetup.value = false
    mfaCode.value = ''
    toast.success('MFA 已启用')
  } catch {
    toast.error('验证码错误，请重试')
  } finally {
    enablingMfa.value = false
  }
}

const registerPasskey = async () => {
  if (mfaConfig.value.enabled) {
    toast.error('请先禁用 MFA 后再启用 Passkey')
    return
  }
  registeringPasskey.value = true
  try {
    const beginResponse = await passkeyApi.beginRegistration()
    const credentialData = await createPasskeyCredential(beginResponse.data.publicKey)
    await passkeyApi.finishRegistration(credentialData)
    passkeyConfig.value.enabled = true
    toast.success('Passkey 注册成功')
  } catch (err: any) {
    if (err.name === 'NotAllowedError') {
      toast.error('用户取消了操作')
    } else {
      toast.error('Passkey 注册失败: ' + (err.message || '未知错误'))
    }
  } finally {
    registeringPasskey.value = false
  }
}

const disablePasskey = async () => {
  try {
    await passkeyApi.disable()
    passkeyConfig.value.enabled = false
    toast.success('Passkey 已禁用')
  } catch {
    toast.error('禁用 Passkey 失败')
  }
}

onMounted(() => {
  loadConfig()
  loadTelegramConfig()
  loadAuthStatus()
  loadProfile()
  loadTokens()
  sysApi.getVersion().then(r => { if (r.data?.version) version.value = r.data.version }).catch(() => {})
})

const version = ref('')
const systemInfo = computed(() => [
  { label: '应用名称', value: 'OCI Panel', icon: Server },
  { label: '版本号', value: version.value ? `v${version.value}` : '—', icon: Info },
  { label: '后端框架', value: 'Gin (Go)', icon: Code },
  { label: '前端框架', value: 'Vue 3 + Vite + Tailwind CSS', icon: Code },
  { label: '数据库', value: 'SQLite', icon: Database }
])
</script>

<template>
  <div class="space-y-6">
    <!-- Header -->
    <div v-motion :initial="{ opacity: 0, y: -20 }" :enter="{ opacity: 1, y: 0 }">
      <h1 class="text-2xl sm:text-3xl font-display font-bold">系统设置</h1>
      <p class="text-muted-foreground mt-1">管理系统配置和查看系统信息</p>
    </div>

    <div class="grid gap-6">
      <!-- System Info Card -->
      <Card
        v-motion
        :initial="{ opacity: 0, y: 20 }"
        :enter="{ opacity: 1, y: 0, transition: { delay: 100 } }"
        class="border-border/50"
      >
        <CardHeader class="border-b border-border/50">
          <CardTitle class="flex items-center gap-2">
            <Info class="w-5 h-5 text-primary" />
            系统信息
          </CardTitle>
        </CardHeader>
        <CardContent class="p-0">
          <div class="divide-y divide-border/50">
            <div
              v-for="(item, index) in systemInfo"
              :key="item.label"
              v-motion
              :initial="{ opacity: 0, x: -20 }"
              :enter="{ opacity: 1, x: 0, transition: { delay: 150 + index * 50 } }"
              class="flex items-center justify-between px-4 sm:px-6 py-4"
            >
              <div class="flex items-center gap-3">
                <div class="w-8 h-8 rounded bg-secondary flex items-center justify-center">
                  <component :is="item.icon" class="w-4 h-4 text-muted-foreground" />
                </div>
                <span class="text-muted-foreground">{{ item.label }}</span>
              </div>
              <span class="font-medium">{{ item.value }}</span>
            </div>
            <div class="flex items-center justify-between px-4 sm:px-6 py-4">
              <div class="flex items-center gap-3">
                <div class="w-8 h-8 rounded bg-success/10 flex items-center justify-center">
                  <Activity class="w-4 h-4 text-success" />
                </div>
                <span class="text-muted-foreground">运行状态</span>
              </div>
              <Badge variant="success">正常运行</Badge>
            </div>
          </div>
        </CardContent>
      </Card>

      <!-- Cache Settings Card -->
      <Card
        v-motion
        :initial="{ opacity: 0, y: 20 }"
        :enter="{ opacity: 1, y: 0, transition: { delay: 200 } }"
        class="border-border/50"
      >
        <CardHeader class="border-b border-border/50">
          <CardTitle class="flex items-center gap-2">
            <Database class="w-5 h-5 text-primary" />
            缓存设置
          </CardTitle>
        </CardHeader>
        <CardContent class="p-0 divide-y divide-border/50">
          <div class="flex items-center justify-between px-4 sm:px-6 py-4">
            <div>
              <p class="font-medium">启用数据缓存</p>
              <p class="text-sm text-muted-foreground mt-1">
                启用后将定时缓存配置的实例数据到数据库，减少对OCI API的请求
              </p>
            </div>
            <Switch v-model="cacheConfig.cacheEnabled" @update:model-value="updateCacheConfig" />
          </div>
          <div class="flex items-center justify-between px-4 sm:px-6 py-4">
            <div>
              <p class="font-medium">缓存刷新间隔</p>
              <p class="text-sm text-muted-foreground mt-1">定时任务检查并更新缓存的间隔时间（分钟）</p>
            </div>
            <div class="flex items-center gap-2">
              <Input
                v-model.number="cacheConfig.cacheInterval"
                type="number"
                min="5"
                max="1440"
                class="w-20 text-center"
                :disabled="!cacheConfig.cacheEnabled"
                @change="updateCacheConfig"
              />
              <span class="text-muted-foreground">分钟</span>
            </div>
          </div>
          <div class="flex items-center justify-between px-4 sm:px-6 py-4">
            <div>
              <p class="font-medium">手动刷新缓存</p>
              <p class="text-sm text-muted-foreground mt-1">立即更新所有配置的缓存数据</p>
            </div>
            <Button :disabled="!cacheConfig.cacheEnabled || refreshing" @click="refreshCache">
              <RefreshCw v-if="!refreshing" class="w-4 h-4" />
              <Loader2 v-else class="w-4 h-4 animate-spin" />
              {{ refreshing ? '刷新中...' : '立即刷新' }}
            </Button>
          </div>
        </CardContent>
      </Card>

      <!-- Telegram Settings Card -->
      <Card
        v-motion
        :initial="{ opacity: 0, y: 20 }"
        :enter="{ opacity: 1, y: 0, transition: { delay: 300 } }"
        class="border-border/50"
      >
        <CardHeader class="border-b border-border/50">
          <CardTitle class="flex items-center gap-2">
            <Send class="w-5 h-5 text-primary" />
            Telegram 通知
          </CardTitle>
        </CardHeader>
        <CardContent class="p-0 divide-y divide-border/50">
          <div v-if="telegramLoading" class="text-center py-8">
            <Loader2 class="w-8 h-8 mx-auto animate-spin text-primary" />
          </div>
          <template v-else>
            <div class="flex items-center justify-between px-4 sm:px-6 py-4">
              <div>
                <p class="font-medium">启用 Telegram 通知</p>
                <p class="text-sm text-muted-foreground mt-1">接收任务执行结果和系统通知</p>
              </div>
              <Switch v-model="telegramConfig.enabled" @update:model-value="updateTelegramConfig" />
            </div>
            <div class="px-4 sm:px-6 py-4 space-y-4">
              <div>
                <label class="text-sm font-medium mb-2 block">Bot Token</label>
                <Input
                  v-model="telegramConfig.botToken"
                  placeholder="输入 Telegram Bot Token"
                  class="font-mono text-sm"
                  @blur="updateTelegramConfig"
                />
                <p class="text-xs text-muted-foreground mt-1">从 @BotFather 获取</p>
              </div>
              <div>
                <label class="text-sm font-medium mb-2 block">Chat ID</label>
                <Input
                  v-model="telegramConfig.chatId"
                  placeholder="输入您的 Telegram Chat ID"
                  class="font-mono"
                  @blur="updateTelegramConfig"
                />
                <p class="text-xs text-muted-foreground mt-1">从 @userinfobot 获取</p>
              </div>
              <div>
                <label class="text-sm font-medium mb-2 block">TG 反代地址</label>
                <Input
                  v-model="telegramConfig.apiBase"
                  placeholder="https://api.telegram.org（国内不通时填反代地址）"
                  class="font-mono text-sm"
                  @blur="updateTelegramConfig"
                />
                <p class="text-xs text-muted-foreground mt-1">
                  留空使用官方地址；部署在国内网络不通时，填写可访问的 Telegram 反代
                </p>
              </div>
            </div>
            <div class="flex items-center justify-between px-4 sm:px-6 py-4">
              <div>
                <p class="font-medium">Bot 运行状态</p>
                <p class="text-sm text-muted-foreground mt-1">Bot 启动后可通过 /start 命令与 Bot 交互</p>
              </div>
              <div class="flex items-center gap-2">
                <Badge :variant="telegramConfig.running ? 'success' : 'secondary'">
                  {{ telegramConfig.running ? '运行中' : '已停止' }}
                </Badge>
                <Button v-if="telegramConfig.running" variant="outline" size="sm" @click="stopTelegramBot">
                  <Square class="w-4 h-4" />
                  停止
                </Button>
                <Button
                  v-else
                  variant="outline"
                  size="sm"
                  :disabled="!telegramConfig.enabled || !telegramConfig.botToken || !telegramConfig.chatId"
                  @click="startTelegramBot"
                >
                  <Play class="w-4 h-4" />
                  启动
                </Button>
              </div>
            </div>
            <div class="flex items-center justify-between px-4 sm:px-6 py-4">
              <div>
                <p class="font-medium">测试功能</p>
                <p class="text-sm text-muted-foreground mt-1">测试连接或发送测试消息</p>
              </div>
              <div class="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  :disabled="!telegramConfig.botToken || testingConnection"
                  @click="testTelegramConnection"
                >
                  <Loader2 v-if="testingConnection" class="w-4 h-4 animate-spin" />
                  <TestTube v-else class="w-4 h-4" />
                  测试连接
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  :disabled="
                    !telegramConfig.enabled || !telegramConfig.botToken || !telegramConfig.chatId || sendingTestMessage
                  "
                  @click="sendTelegramTestMessage"
                >
                  <Loader2 v-if="sendingTestMessage" class="w-4 h-4 animate-spin" />
                  <Send v-else class="w-4 h-4" />
                  发送测试
                </Button>
              </div>
            </div>
          </template>
        </CardContent>
      </Card>

      <!-- MFA Settings Card -->
      <Card
        v-motion
        :initial="{ opacity: 0, y: 20 }"
        :enter="{ opacity: 1, y: 0, transition: { delay: 350 } }"
        class="border-border/50"
      >
        <CardHeader class="border-b border-border/50">
          <CardTitle class="flex items-center gap-2">
            <Shield class="w-5 h-5 text-primary" />
            双因素认证 (MFA)
          </CardTitle>
        </CardHeader>
        <CardContent class="p-0 divide-y divide-border/50">
          <div v-if="mfaLoading" class="text-center py-8">
            <Loader2 class="w-8 h-8 mx-auto animate-spin text-primary" />
          </div>
          <template v-else>
            <div class="flex items-center justify-between px-4 sm:px-6 py-4">
              <div>
                <p class="font-medium">启用 MFA 验证</p>
                <p class="text-sm text-muted-foreground mt-1">启用后登录时需要输入验证器 App 生成的动态验证码</p>
              </div>
              <div class="flex items-center gap-2">
                <Badge :variant="mfaConfig.enabled ? 'success' : 'secondary'">
                  <component :is="mfaConfig.enabled ? ShieldCheck : ShieldOff" class="w-3 h-3 mr-1" />
                  {{ mfaConfig.enabled ? '已启用' : '未启用' }}
                </Badge>
                <Button variant="outline" size="sm" @click="toggleMfaSetup">
                  {{ mfaConfig.enabled ? '禁用' : '设置' }}
                </Button>
              </div>
            </div>
            <div v-if="showMfaSetup && !mfaConfig.enabled" class="px-4 sm:px-6 py-4 space-y-4">
              <div class="flex flex-col items-center">
                <p class="text-sm text-muted-foreground mb-4">
                  请使用 Google Authenticator、Microsoft Authenticator 或其他 TOTP 应用扫描二维码
                </p>
                <img
                  v-if="mfaConfig.qrCode"
                  :src="mfaConfig.qrCode"
                  alt="MFA QR Code"
                  class="w-48 h-48 border rounded-lg p-2 bg-white"
                />
                <p class="text-xs text-muted-foreground mt-2">
                  或手动输入密钥:
                  <code class="bg-secondary px-2 py-1 rounded text-xs">{{ mfaConfig.secret }}</code>
                </p>
              </div>
              <div>
                <label class="text-sm font-medium mb-2 block">输入验证码以确认启用</label>
                <div class="flex gap-2">
                  <Input
                    v-model="mfaCode"
                    type="text"
                    placeholder="输入6位验证码"
                    maxlength="6"
                    class="w-40 text-center font-mono tracking-widest"
                    @keyup.enter="enableMfa"
                  />
                  <Button :disabled="enablingMfa || mfaCode.length !== 6" @click="enableMfa">
                    <Loader2 v-if="enablingMfa" class="w-4 h-4 animate-spin" />
                    <ShieldCheck v-else class="w-4 h-4" />
                    确认启用
                  </Button>
                  <Button variant="outline" @click="cancelMfaSetup">取消</Button>
                </div>
              </div>
            </div>
          </template>
        </CardContent>
      </Card>

      <!-- Passkey Settings Card -->
      <Card
        v-motion
        :initial="{ opacity: 0, y: 20 }"
        :enter="{ opacity: 1, y: 0, transition: { delay: 375 } }"
        class="border-border/50"
      >
        <CardHeader class="border-b border-border/50">
          <CardTitle class="flex items-center gap-2">
            <Fingerprint class="w-5 h-5 text-primary" />
            通行密钥 (Passkey)
          </CardTitle>
        </CardHeader>
        <CardContent class="p-0 divide-y divide-border/50">
          <div v-if="passkeyLoading" class="text-center py-8">
            <Loader2 class="w-8 h-8 mx-auto animate-spin text-primary" />
          </div>
          <template v-else>
            <div class="flex items-center justify-between px-4 sm:px-6 py-4">
              <div>
                <p class="font-medium">启用 Passkey 验证</p>
                <p class="text-sm text-muted-foreground mt-1">使用指纹、面容或安全密钥登录，无需输入验证码</p>
              </div>
              <div class="flex items-center gap-2">
                <Badge :variant="passkeyConfig.enabled ? 'success' : 'secondary'">
                  <component :is="passkeyConfig.enabled ? KeyRound : ShieldOff" class="w-3 h-3 mr-1" />
                  {{ passkeyConfig.enabled ? '已启用' : '未启用' }}
                </Badge>
                <Button
                  v-if="!passkeyConfig.enabled"
                  variant="outline"
                  size="sm"
                  :disabled="registeringPasskey"
                  @click="registerPasskey"
                >
                  <Loader2 v-if="registeringPasskey" class="w-4 h-4 animate-spin" />
                  <Fingerprint v-else class="w-4 h-4" />
                  注册
                </Button>
                <Button v-else variant="outline" size="sm" @click="disablePasskey">
                  <Trash2 class="w-4 h-4" />
                  删除
                </Button>
              </div>
            </div>
            <div class="px-4 sm:px-6 py-4">
              <p class="text-xs text-muted-foreground">
                提示：Passkey 和 MFA 只能二选一启用。启用 Passkey 后，登录时可以选择使用 Passkey 或账号密码。
              </p>
            </div>
          </template>
        </CardContent>
      </Card>

      <!-- Account Security Card -->
      <Card
        v-motion
        :initial="{ opacity: 0, y: 20 }"
        :enter="{ opacity: 1, y: 0, transition: { delay: 390 } }"
        class="border-border/50"
      >
        <CardHeader class="border-b border-border/50">
          <CardTitle class="flex items-center gap-2">
            <Lock class="w-5 h-5 text-primary" />
            账户安全
          </CardTitle>
        </CardHeader>
        <CardContent class="p-0 divide-y divide-border/50">
          <div v-if="profileLoading" class="text-center py-8">
            <Loader2 class="w-8 h-8 mx-auto animate-spin text-primary" />
          </div>
          <template v-else>
            <div class="flex items-center justify-between px-4 sm:px-6 py-4">
              <div class="min-w-0 flex-1 pr-4">
                <p class="font-medium">登录账号</p>
                <p class="text-sm text-muted-foreground mt-1">当前管理员账号（数据库托管，可在此修改）</p>
                <div class="mt-3 flex items-center gap-2">
                  <Input
                    v-model="editingAccount"
                    :placeholder="profile.account || '新账号'"
                    class="font-mono max-w-[220px]"
                    @keyup.enter="updateAccount"
                  />
                  <Button :disabled="savingAccount || !editingAccount.trim()" @click="updateAccount">
                    <Loader2 v-if="savingAccount" class="w-4 h-4 animate-spin" />
                    <User v-else class="w-4 h-4" />
                    保存
                  </Button>
                </div>
              </div>
            </div>
            <div class="px-4 sm:px-6 py-4 space-y-2">
              <label class="text-sm font-medium block">邮箱（用于密码重置通知）</label>
              <div class="flex gap-2">
                <Input
                  v-model="emailForm.email"
                  type="email"
                  placeholder="you@example.com"
                  class="flex-1"
                  @keyup.enter="saveEmail"
                />
                <Button :disabled="savingEmail" @click="saveEmail">
                  <Loader2 v-if="savingEmail" class="w-4 h-4 animate-spin" />
                  <Mail v-else class="w-4 h-4" />
                  保存
                </Button>
              </div>
            </div>
            <div class="px-4 sm:px-6 py-4 space-y-3">
              <p class="font-medium">修改密码</p>
              <Input v-model="passwordForm.oldPassword" type="password" placeholder="当前密码" />
              <Input v-model="passwordForm.newPassword" type="password" placeholder="新密码（至少6位）" />
              <Input v-model="passwordForm.confirmPassword" type="password" placeholder="确认新密码" />
              <Button :disabled="changingPassword" @click="changePassword" class="w-full">
                <Loader2 v-if="changingPassword" class="w-4 h-4 animate-spin" />
                <Lock v-else class="w-4 h-4" />
                修改密码
              </Button>
            </div>
          </template>
        </CardContent>
      </Card>

      <!-- API Token Card -->
      <Card
        v-motion
        :initial="{ opacity: 0, y: 20 }"
        :enter="{ opacity: 1, y: 0, transition: { delay: 395 } }"
        class="border-border/50"
      >
        <CardHeader class="border-b border-border/50">
          <CardTitle class="flex items-center gap-2">
            <KeyRound class="w-5 h-5 text-primary" />
            API 令牌
          </CardTitle>
        </CardHeader>
        <CardContent class="p-0 divide-y divide-border/50">
          <div class="flex items-center justify-between px-4 sm:px-6 py-4">
            <div class="pr-4">
              <p class="font-medium">机器人 / 第三方调用凭据</p>
              <p class="text-sm text-muted-foreground mt-1">
                生成的令牌等价于管理员私钥，可通过
                <code class="bg-secondary px-1 rounded">Authorization: Bearer &lt;token&gt;</code>
                调用整个 /api 接口；可随时吊销或定期更换。
              </p>
            </div>
            <Button @click="openCreate">
              <Plus class="w-4 h-4" />
              创建令牌
            </Button>
          </div>
          <div v-if="tokensLoading" class="text-center py-8">
            <Loader2 class="w-8 h-8 mx-auto animate-spin text-primary" />
          </div>
          <div v-else-if="tokens.length === 0" class="px-4 sm:px-6 py-8 text-center text-muted-foreground text-sm">
            暂无令牌，点击「创建令牌」生成一个。
          </div>
          <div v-else class="divide-y divide-border/50">
            <div
              v-for="t in sortedTokens"
              :key="t.id"
              class="flex flex-wrap items-center justify-between gap-3 px-4 sm:px-6 py-3"
            >
              <div class="min-w-0 flex-1 w-full sm:w-auto">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="font-medium break-words min-w-0 w-full sm:w-auto sm:flex-1">{{ t.name }}</span>
                  <Badge
                    :variant="t.scope === 'full' ? 'success' : 'secondary'"
                    class="shrink-0 whitespace-nowrap"
                  >
                    {{ t.scope }}
                  </Badge>
                  <Badge variant="secondary" class="gap-1 shrink-0 whitespace-nowrap">
                    <Activity class="w-3 h-3" />
                    调用 {{ t.callCount || 0 }} 次
                  </Badge>
                </div>
                <p class="text-xs text-muted-foreground mt-1 font-mono break-all">{{ t.prefix }}••••••••</p>
                <div class="text-xs text-muted-foreground mt-1.5 space-y-1">
                  <p class="flex items-center gap-1.5 flex-wrap">
                    <Clock class="w-3 h-3 shrink-0" />
                    创建 {{ formatTime(t.createdAt) }}
                    <span class="text-muted-foreground/40">·</span>
                    过期 {{ t.expiresAt ? formatTime(t.expiresAt) : '永不过期' }}
                  </p>
                  <p class="flex items-center gap-1.5 flex-wrap">
                    <Activity class="w-3 h-3 shrink-0" />
                    最近使用 {{ formatTime(t.lastUsedAt) }}
                    <template v-if="t.lastUsedIp">
                      <span class="text-muted-foreground/40">·</span>
                      来源 {{ t.lastUsedIp }}
                    </template>
                  </p>
                </div>
              </div>
              <div class="flex items-center gap-2 w-full sm:w-auto justify-end sm:justify-start">
                <Button variant="outline" size="sm" @click="openCalls(t)">
                  <Activity class="w-4 h-4" />
                  调用记录
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  :disabled="revokingId === t.id"
                  @click="revokeToken(t.id)"
                >
                  <Loader2 v-if="revokingId === t.id" class="w-4 h-4 animate-spin" />
                  <Trash2 v-else class="w-4 h-4" />
                  吊销
                </Button>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      <!-- System Configuration Card -->
      <Card
        v-motion
        :initial="{ opacity: 0, y: 20 }"
        :enter="{ opacity: 1, y: 0, transition: { delay: 400 } }"
        class="border-border/50"
      >
        <CardHeader class="border-b border-border/50">
          <CardTitle class="flex items-center gap-2">
            <FolderOpen class="w-5 h-5 text-primary" />
            系统配置
          </CardTitle>
        </CardHeader>
        <CardContent class="p-0 divide-y divide-border/50">
          <div v-if="loading" class="text-center py-8">
            <Loader2 class="w-8 h-8 mx-auto animate-spin text-primary" />
          </div>
          <template v-else>
            <div class="flex items-center justify-between px-4 sm:px-6 py-4">
              <span class="text-muted-foreground">密钥目录</span>
              <span class="font-mono text-sm">{{ config.keyDirPath || 'N/A' }}</span>
            </div>
            <div class="flex items-center justify-between px-4 sm:px-6 py-4">
              <div>
                <p class="text-muted-foreground">日志级别</p>
                <p class="text-xs text-muted-foreground mt-1">修改即时生效，并写入配置文件持久化</p>
              </div>
              <select
                v-model="logLevel"
                :disabled="updatingLogLevel"
                class="rounded-md border border-border bg-background px-3 py-2 text-sm font-medium"
                @change="updateLogLevel"
              >
                <option value="debug">debug</option>
                <option value="info">info</option>
                <option value="warn">warn</option>
                <option value="error">error</option>
              </select>
            </div>
          </template>
        </CardContent>
      </Card>

      <!-- About Card -->
      <Card
        v-motion
        :initial="{ opacity: 0, y: 20 }"
        :enter="{ opacity: 1, y: 0, transition: { delay: 500 } }"
        class="border-border/50"
      >
        <CardHeader class="border-b border-border/50">
          <CardTitle class="flex items-center gap-2">
            <Info class="w-5 h-5 text-primary" />
            关于
          </CardTitle>
        </CardHeader>
        <CardContent class="py-6">
          <p class="text-muted-foreground leading-relaxed mb-6">
            OCI Panel 是一个基于 Go + Vue 3 开发的 Oracle Cloud Infrastructure 管理面板，
            提供实例管理、网络配置、任务调度等功能，帮助用户更便捷地管理 OCI 资源。
          </p>
          <div class="flex flex-wrap gap-3">
            <a
              href="https://github.com/adiecho/oci-panel"
              target="_blank"
              rel="noopener noreferrer"
              class="inline-flex items-center justify-center gap-2 rounded-md border border-input bg-transparent px-4 py-2 text-sm font-medium transition-all duration-200 hover:bg-accent hover:text-accent-foreground"
            >
              <Github class="w-4 h-4" />
              GitHub
            </a>
            <a
              href="https://docs.oracle.com/iaas"
              target="_blank"
              rel="noopener noreferrer"
              class="inline-flex items-center justify-center gap-2 rounded-md border border-input bg-transparent px-4 py-2 text-sm font-medium transition-all duration-200 hover:bg-accent hover:text-accent-foreground"
            >
              <FileText class="w-4 h-4" />
              OCI文档
            </a>
          </div>
        </CardContent>
      </Card>

      <!-- Create Token Dialog -->
      <Dialog v-model:open="createOpen">
        <DialogHeader>
          <DialogTitle>创建 API 令牌</DialogTitle>
          <DialogDescription>
            令牌生成后明文仅显示一次，请立即复制保存。
          </DialogDescription>
        </DialogHeader>

        <div v-if="!newToken" class="space-y-4">
          <div>
            <label class="text-sm font-medium mb-2 block">名称</label>
            <Input
              v-model="createForm.name"
              placeholder="例如：我的机器人"
              @keyup.enter="createToken"
            />
          </div>
          <div>
            <label class="text-sm font-medium mb-2 block">过期时间</label>
            <select
              v-model.number="createForm.expiresInDays"
              class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
            >
              <option :value="0">永不过期</option>
              <option :value="30">30 天</option>
              <option :value="90">90 天</option>
              <option :value="180">180 天</option>
              <option :value="365">365 天</option>
            </select>
          </div>
        </div>

        <div v-else class="space-y-4">
          <p class="text-sm text-amber-600 dark:text-amber-400">
            ⚠️ 以下为完整令牌，<b>仅显示这一次</b>，关闭后无法再次查看：
          </p>
          <div class="flex items-center gap-2">
            <code class="flex-1 break-all rounded bg-secondary px-3 py-2 text-xs font-mono">{{ newToken.token }}</code>
            <Button variant="outline" size="sm" @click="copyToken">
              <Check v-if="copied" class="w-4 h-4" />
              <Copy v-else class="w-4 h-4" />
              {{ copied ? '已复制' : '复制' }}
            </Button>
          </div>
        </div>

        <DialogFooter class="mt-6">
          <Button v-if="!newToken" :disabled="creating" @click="createToken">
            <Loader2 v-if="creating" class="w-4 h-4 animate-spin" />
            <Plus v-else class="w-4 h-4" />
            生成
          </Button>
          <Button v-else @click="createOpen = false">完成</Button>
        </DialogFooter>
      </Dialog>

      <!-- 调用记录弹窗（参考青龙面板） -->
      <Dialog v-model:open="callsOpen">
        <DialogHeader>
          <DialogTitle>调用记录</DialogTitle>
          <DialogDescription>
            令牌
            <span class="text-primary font-medium">{{ callsToken?.name }}</span>
            的最近 API 调用（最多保留 200 条）
          </DialogDescription>
        </DialogHeader>

        <div class="mt-4 max-h-[60vh] overflow-y-auto">
          <div v-if="callsLoading" class="text-center py-8">
            <Loader2 class="w-8 h-8 mx-auto animate-spin text-primary" />
          </div>
          <div v-else-if="tokenCalls.length === 0" class="text-center py-8 text-muted-foreground text-sm">
            暂无调用记录
          </div>
          <div v-else class="divide-y divide-border/30">
            <div
              v-for="call in tokenCalls"
              :key="call.id"
              class="py-2 font-mono text-xs leading-relaxed hover:bg-muted/40 whitespace-pre-wrap break-words"
            >
              <span class="text-foreground/80">{{ callHead(call) }}</span>
              <span :class="statusClass(call.statusCode)">{{ call.statusCode }}</span>
              <span class="text-muted-foreground">{{ callTail(call) }}</span>
            </div>
          </div>
        </div>

        <DialogFooter class="mt-6">
          <Button variant="outline" @click="callsOpen = false">关闭</Button>
        </DialogFooter>
      </Dialog>
    </div>
  </div>
</template>
