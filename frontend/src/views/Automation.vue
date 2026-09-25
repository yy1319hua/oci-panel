<script setup lang="ts">
/**
 * 自动化控制台（手机端优先设计：单列布局、大按钮、抽屉式表单）。
 * 四个分区：保活 / 抢机 / 备份 / 告警&配额。另有 PushPlus 推送设置。
 */
import { ref, onMounted, computed } from 'vue'
import {
  HeartPulse, Target, DatabaseBackup, BellRing, Plus, Trash2, Play, Activity,
  Loader2, X, RefreshCw, Smartphone, ChevronRight
} from 'lucide-vue-next'
import { automationApi, lookupApi, ociApi } from '@/api'
import type { KeepaliveTask, GrabTask, BackupTask, QuotaOverview, ConfigItem } from '@/api'
import { toast } from '@/composables/useToast'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import { Dialog, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'

// ---------- 基础状态 ----------
const activeTab = ref<'keepalive' | 'grab' | 'backup' | 'alert' | 'metrics'>('keepalive')
const loading = ref(false)
const configs = ref<ConfigItem[]>([])

const tabs = [
  { key: 'keepalive', label: '保活', icon: HeartPulse },
  { key: 'grab', label: '抢机', icon: Target },
  { key: 'backup', label: '备份', icon: DatabaseBackup },
  { key: 'alert', label: '告警', icon: BellRing },
  { key: 'metrics', label: '监控', icon: Activity }
] as const

// ---------- 保活 ----------
const keepaliveTasks = ref<KeepaliveTask[]>([])
const showKeepaliveForm = ref(false)
const kForm = ref({ id: 0, configId: '', instanceId: '', instanceName: '', enabled: true, intervalMin: 60, durationSec: 120 })
const kInstances = ref<Array<{ value: string; label: string }>>([])

// ---------- 抢机 ----------
const grabTasks = ref<GrabTask[]>([])
const showGrabForm = ref(false)
const gForm = ref({ id: 0, configId: '', name: '', ad: '', shape: 'VM.Standard.A1.Flex', ocpus: 2, memoryGB: 12, bootVolumeGB: 50, imageId: '', subnetId: '', instanceName: '', intervalMin: 5, enabled: true })
const gShapes = ['VM.Standard.A1.Flex', 'VM.Standard.E2.1.Micro']
const gAds = ref<Array<{ value: string; label: string }>>([])
const gImages = ref<Array<{ value: string; label: string }>>([])
const gSubnets = ref<Array<{ value: string; label: string }>>([])

/** 拉取抢机表单三个下拉的数据（选配置后调用）。 */
async function loadGrabOptions(cfgId: string) {
  gAds.value = []; gImages.value = []; gSubnets.value = []
  if (!cfgId) return
  try {
    const [ads, subs] = await Promise.all([lookupApi.ads(cfgId), lookupApi.subnets(cfgId)])
    gAds.value = ads.data || []
    gSubnets.value = subs.data || []
  } catch { /* 失败时保持空，用户仍可手填 OCID 场景见后端日志 */ }
}

/** 按形状拉取可用系统镜像。 */
async function loadImages(cfgId: string, shape: string) {
  gImages.value = []
  if (!cfgId || !shape) return
  try {
    const res = await lookupApi.images(cfgId, shape)
    gImages.value = res.data || []
  } catch { /* 同上 */ }
}

// ---------- 备份 ----------
const backupTasks = ref<BackupTask[]>([])
const showBackupForm = ref(false)
const bForm = ref({ id: 0, configId: '', volumeId: '', volumeName: '', retention: 3, intervalHour: 24, enabled: true })
const bVolumes = ref<Array<{ id: string; name: string; type: string; sizeGB: number }>>([])

// ---------- 告警 / 配额 / PushPlus ----------
const quota = ref<QuotaOverview[]>([])
const alertThreshold = ref(80)
const ppToken = ref('')
const ppMasked = ref('')
const savingAlert = ref(false)
const testingPP = ref(false)

const isEdit = computed(() => (activeTab.value === 'keepalive' ? kForm.value.id > 0 : activeTab.value === 'grab' ? gForm.value.id > 0 : bForm.value.id > 0))

async function loadAll() {
  loading.value = true
  try {
    const [cfgs, ks, gs, bs, qs] = await Promise.all([
      ociApi.userPage({ page: 1, pageSize: 100 }), automationApi.listKeepalive(), automationApi.listGrab(),
      automationApi.listBackup(), automationApi.getQuota()
    ])
    configs.value = cfgs.data?.list || []
    keepaliveTasks.value = ks.data || []
    grabTasks.value = gs.data || []
    backupTasks.value = bs.data || []
    quota.value = qs.data || []
  } catch (e: any) {
    toast.error(e?.message || '加载失败')
  } finally {
    loading.value = false
  }
}

async function loadInstancesFor(cfgId: string) {
  if (!cfgId) return
  try {
    const res = await ociApi.detailsInstances({ configId: cfgId })
    kInstances.value = (res.data || []).map((i: any) => ({ value: i.id, label: i.displayName || i.id }))
  } catch { /* 忽略，保持空列表 */ }
}

// ---------- 保活操作 ----------
function openKeepalive() {
  kForm.value = { id: 0, configId: configs.value[0]?.id || '', instanceId: '', instanceName: '', enabled: true, intervalMin: 60, durationSec: 120 }
  kInstances.value = []
  if (kForm.value.configId) loadInstancesFor(kForm.value.configId)
  showKeepaliveForm.value = true
}
async function saveKeepalive() {
  if (!kForm.value.configId || !kForm.value.instanceId) return toast.error('请选择配置和实例')
  try {
    const name = kInstances.value.find(i => i.value === kForm.value.instanceId)?.label || kForm.value.instanceId
    await automationApi.saveKeepalive({ ...kForm.value, instanceName: name })
    toast.success('已保存')
    showKeepaliveForm.value = false
    loadAll()
  } catch (e: any) { toast.error(e?.message || '保存失败') }
}
async function toggleKeepalive(t: KeepaliveTask) {
  await automationApi.saveKeepalive({ ...t, enabled: !t.enabled })
  t.enabled = !t.enabled
}
async function delKeepalive(t: KeepaliveTask) {
  if (!confirm(`删除保活任务「${t.instanceName}」？`)) return
  await automationApi.deleteKeepalive(t.id)
  toast.success('已删除'); loadAll()
}
async function runKeepalive(t: KeepaliveTask) {
  toast.info('执行中，结果将推送到 TG / PushPlus')
  await automationApi.runKeepalive(t.id)
  setTimeout(loadAll, 3000)
}

// ---------- 抢机操作 ----------
function openGrab() {
  gForm.value = { id: 0, configId: configs.value[0]?.id || '', name: '', ad: '', shape: 'VM.Standard.A1.Flex', ocpus: 2, memoryGB: 12, bootVolumeGB: 50, imageId: '', subnetId: '', instanceName: '', intervalMin: 5, enabled: true }
  if (gForm.value.configId) {
    loadGrabOptions(gForm.value.configId)
    loadImages(gForm.value.configId, gForm.value.shape)
  }
  showGrabForm.value = true
}
async function saveGrab() {
  if (!gForm.value.configId || !gForm.value.name || !gForm.value.ad || !gForm.value.imageId || !gForm.value.subnetId) {
    return toast.error('请完整填写任务名、配置、可用域、镜像、子网')
  }
  if (gForm.value.bootVolumeGB > 200) return toast.error('引导卷上限 200GB（免费总额）')
  try {
    await automationApi.saveGrab(gForm.value)
    toast.success('已保存')
    showGrabForm.value = false
    loadAll()
  } catch (e: any) { toast.error(e?.message || '保存失败') }
}
async function toggleGrab(t: GrabTask) {
  await automationApi.saveGrab({ ...t, enabled: !t.enabled, status: !t.enabled ? 'running' : 'paused' })
  t.enabled = !t.enabled
  loadAll()
}
async function delGrab(t: GrabTask) {
  if (!confirm(`删除抢机任务「${t.name}」？`)) return
  await automationApi.deleteGrab(t.id)
  toast.success('已删除'); loadAll()
}
async function runGrab(t: GrabTask) {
  toast.info('尝试抢机中…')
  await automationApi.runGrab(t.id)
  setTimeout(loadAll, 5000)
}

// ---------- 备份操作 ----------
function openBackup() {
  bForm.value = { id: 0, configId: configs.value[0]?.id || '', volumeId: '', volumeName: '', retention: 3, intervalHour: 24, enabled: true }
  bVolumes.value = []
  if (bForm.value.configId) loadVolumes(bForm.value.configId)
  showBackupForm.value = true
}
async function loadVolumes(cfgId: string) {
  if (!cfgId) return
  try {
    const res = await automationApi.listVolumes(cfgId)
    bVolumes.value = res.data || []
  } catch { /* 空 */ }
}
async function saveBackup() {
  if (!bForm.value.configId || !bForm.value.volumeId) return toast.error('请选择配置和卷')
  if (bForm.value.retention > 5) return toast.error('保留份数上限 5（免费备份槽）')
  const v = bVolumes.value.find(x => x.id === bForm.value.volumeId)
  try {
    await automationApi.saveBackup({ ...bForm.value, volumeName: v?.name || bForm.value.volumeId })
    toast.success('已保存')
    showBackupForm.value = false
    loadAll()
  } catch (e: any) { toast.error(e?.message || '保存失败') }
}
async function toggleBackup(t: BackupTask) {
  await automationApi.saveBackup({ ...t, enabled: !t.enabled })
  t.enabled = !t.enabled
}
async function delBackup(t: BackupTask) {
  if (!confirm(`删除备份任务「${t.volumeName}」？已创建的备份不受影响。`)) return
  await automationApi.deleteBackup(t.id)
  toast.success('已删除'); loadAll()
}
async function runBackup(t: BackupTask) {
  toast.info('备份执行中…')
  await automationApi.runBackup(t.id)
  setTimeout(loadAll, 5000)
}

// ---------- 告警 / PushPlus ----------
const cpuThreshold = ref(0)
async function loadSettings() {
  try {
    const res = await automationApi.getSettings()
    if (res.data?.trafficAlertThreshold) alertThreshold.value = res.data.trafficAlertThreshold
    cpuThreshold.value = res.data?.cpuAlertThreshold ?? 0
  } catch { /* 保持默认 */ }
}
async function saveAlert() {
  savingAlert.value = true
  try {
    await automationApi.saveSettings({ trafficAlertThreshold: alertThreshold.value, cpuAlertThreshold: cpuThreshold.value })
    toast.success('阈值已保存')
  } catch (e: any) { toast.error(e?.message || '保存失败') } finally { savingAlert.value = false }
}

// ---------- 监控曲线 ----------
const chartW = 600, chartH = 200, chartPad = 28
const mConfigId = ref('')
const mInstanceId = ref('')
const mHours = ref(24)
const mLoading = ref(false)
const mInstances = ref<Array<{ value: string; label: string }>>([])
const mChart = ref<{ time: string[]; cpu: number[]; memory: number[]; hasMemory: boolean } | null>(null)

function loadMetricsInstances(cfgId: string) {
  mInstanceId.value = ''
  mChart.value = null
  if (!cfgId) return
  ociApi.detailsInstances({ configId: cfgId }).then(res => {
    mInstances.value = (res.data || []).map((i: any) => ({ value: i.id, label: i.displayName || i.id }))
  }).catch(() => { mInstances.value = [] })
}
async function loadMetrics() {
  if (!mConfigId.value || !mInstanceId.value) return
  mLoading.value = true
  try {
    const res = await automationApi.getCpuMemory({ configId: mConfigId.value, instanceId: mInstanceId.value, hours: mHours.value })
    mChart.value = res.data || null
  } catch (e: any) { toast.error(e?.message || '查询失败') } finally { mLoading.value = false }
}
const cpuPoints = computed(() => pointsOf(mChart.value?.cpu))
const memPoints = computed(() => pointsOf(mChart.value?.memory))
function pointsOf(arr?: number[]): string {
  if (!arr || arr.length === 0) return ''
  const n = arr.length
  const w = chartW - chartPad * 2, h = chartH - chartPad * 2
  return arr.map((v, i) => {
    const x = chartPad + (n === 1 ? w / 2 : (i / (n - 1)) * w)
    const y = chartPad + h - Math.min(v, 100) / 100 * h
    return `${x.toFixed(1)},${y.toFixed(1)}`
  }).join(' ')
}

async function savePushplus() {
  try {
    await automationApi.savePushplus({ token: ppToken.value })
    const res = await automationApi.getPushplus()
    ppMasked.value = res.data?.tokenMasked || ''
    toast.success('PushPlus 已保存')
  } catch (e: any) { toast.error(e?.message || '保存失败') }
}
async function testPushplus() {
  testingPP.value = true
  try {
    await automationApi.testPushplus()
    toast.success('测试消息已发送，请查看微信')
  } catch (e: any) { toast.error(e?.message || '测试失败') } finally { testingPP.value = false }
}

onMounted(async () => {
  await loadAll()
  await loadSettings()
  try {
    const res = await automationApi.getPushplus()
    ppMasked.value = res.data?.tokenMasked || ''
  } catch { /* ignore */ }
})

const statusBadge = (t: { enabled?: boolean; status?: string }) =>
  t.status === 'success' ? '已抢到' : t.enabled ? '运行中' : '已停用'
</script>

<template>
  <div class="max-w-3xl mx-auto px-3 py-4 space-y-4 pb-24">
    <!-- 标题 -->
    <div class="flex items-center justify-between">
      <h1 class="text-lg font-semibold">自动化</h1>
      <Button variant="ghost" size="icon" :disabled="loading" @click="loadAll">
        <RefreshCw class="w-4 h-4" :class="{ 'animate-spin': loading }" />
      </Button>
    </div>

    <!-- Tab 栏（手机端横向滚动友好） -->
    <div class="grid grid-cols-4 gap-2 sticky top-0 z-10 bg-background pt-1">
      <button v-for="t in tabs" :key="t.key"
        class="flex flex-col items-center gap-1 py-2 rounded-lg border text-xs transition-colors"
        :class="activeTab === t.key ? 'bg-primary text-primary-foreground border-primary' : 'bg-card'"
        @click="activeTab = t.key">
        <component :is="t.icon" class="w-4 h-4" />
        {{ t.label }}
      </button>
    </div>

    <!-- ============ 保活 ============ -->
    <template v-if="activeTab === 'keepalive'">
      <div class="flex items-center justify-between text-sm text-muted-foreground">
        <span>防止实例被 OCI 判定空闲回收（免费）</span>
        <Button size="sm" @click="openKeepalive"><Plus class="w-4 h-4 mr-1" />新建</Button>
      </div>
      <Card v-for="t in keepaliveTasks" :key="t.id" class="overflow-hidden">
        <CardContent class="p-4 space-y-2">
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0">
              <div class="font-medium truncate">{{ t.instanceName }}</div>
              <div class="text-xs text-muted-foreground">每 {{ t.intervalMin }} 分钟 · 负载 {{ t.durationSec }}s</div>
            </div>
            <Switch :model-value="t.enabled" @update:model-value="toggleKeepalive(t)" />
          </div>
          <div class="flex items-center gap-2 flex-wrap">
            <Badge :variant="t.enabled ? 'default' : 'secondary'">{{ statusBadge(t) }}</Badge>
            <span class="text-xs text-muted-foreground truncate">{{ t.lastResult || '未执行' }}</span>
          </div>
          <div class="flex gap-2 pt-1">
            <Button size="sm" variant="outline" class="flex-1" @click="runKeepalive(t)"><Play class="w-3 h-3 mr-1" />立即执行</Button>
            <Button size="sm" variant="destructive" @click="delKeepalive(t)"><Trash2 class="w-3 h-3" /></Button>
          </div>
        </CardContent>
      </Card>
      <p v-if="!keepaliveTasks.length" class="text-center text-sm text-muted-foreground py-8">暂无保活任务</p>
    </template>

    <!-- ============ 抢机 ============ -->
    <template v-if="activeTab === 'grab'">
      <div class="flex items-center justify-between text-sm text-muted-foreground">
        <span>Out of Capacity 时自动重试开机</span>
        <Button size="sm" @click="openGrab"><Plus class="w-4 h-4 mr-1" />新建</Button>
      </div>
      <Card v-for="t in grabTasks" :key="t.id">
        <CardContent class="p-4 space-y-2">
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0">
              <div class="font-medium truncate">{{ t.name }}</div>
              <div class="text-xs text-muted-foreground truncate">{{ t.shape }} · {{ t.ocpus }} OCPU / {{ t.memoryGB }}GB</div>
            </div>
            <Switch :model-value="t.enabled" @update:model-value="toggleGrab(t)" />
          </div>
          <div class="flex items-center gap-2 flex-wrap">
            <Badge :variant="t.status === 'success' ? 'default' : t.enabled ? 'default' : 'secondary'">{{ statusBadge(t) }}</Badge>
            <span class="text-xs text-muted-foreground">已尝试 {{ t.tryCount }} 次 · 每 {{ t.intervalMin }} 分钟</span>
          </div>
          <div class="text-xs text-muted-foreground truncate">{{ t.lastError || '暂无错误' }}</div>
          <div class="flex gap-2 pt-1">
            <Button size="sm" variant="outline" class="flex-1" :disabled="t.status === 'success'" @click="runGrab(t)">
              <Play class="w-3 h-3 mr-1" />立即尝试
            </Button>
            <Button size="sm" variant="destructive" @click="delGrab(t)"><Trash2 class="w-3 h-3" /></Button>
          </div>
        </CardContent>
      </Card>
      <p v-if="!grabTasks.length" class="text-center text-sm text-muted-foreground py-8">暂无抢机任务</p>
    </template>

    <!-- ============ 备份 ============ -->
    <template v-if="activeTab === 'backup'">
      <div class="flex items-center justify-between text-sm text-muted-foreground">
        <span>定时备份卷 · 保留 ≤5 份（永不收费）</span>
        <Button size="sm" @click="openBackup"><Plus class="w-4 h-4 mr-1" />新建</Button>
      </div>
      <Card v-for="t in backupTasks" :key="t.id">
        <CardContent class="p-4 space-y-2">
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0">
              <div class="font-medium truncate">{{ t.volumeName }}</div>
              <div class="text-xs text-muted-foreground">每 {{ t.intervalHour }} 小时 · 保留 {{ t.retention }} 份</div>
            </div>
            <Switch :model-value="t.enabled" @update:model-value="toggleBackup(t)" />
          </div>
          <div class="flex items-center gap-2 flex-wrap">
            <Badge :variant="t.enabled ? 'default' : 'secondary'">{{ statusBadge(t) }}</Badge>
            <span class="text-xs text-muted-foreground truncate">{{ t.lastResult || '未执行' }}</span>
          </div>
          <div class="flex gap-2 pt-1">
            <Button size="sm" variant="outline" class="flex-1" @click="runBackup(t)"><Play class="w-3 h-3 mr-1" />立即备份</Button>
            <Button size="sm" variant="destructive" @click="delBackup(t)"><Trash2 class="w-3 h-3" /></Button>
          </div>
        </CardContent>
      </Card>
      <p v-if="!backupTasks.length" class="text-center text-sm text-muted-foreground py-8">暂无备份任务</p>
    </template>

    <!-- ============ 告警 & 配额 ============ -->
    <template v-if="activeTab === 'alert'">
      <!-- 流量告警 -->
      <Card>
        <CardHeader class="pb-2"><CardTitle class="text-base">流量超额告警</CardTitle></CardHeader>
        <CardContent class="space-y-3">
          <div class="flex items-center gap-3">
            <Input type="number" v-model.number="alertThreshold" min="1" max="100" class="w-24" />
            <span class="text-sm text-muted-foreground">% 时推送告警（10TB 免费额度）</span>
          </div>
          <div class="flex gap-2">
            <Button size="sm" :disabled="savingAlert" @click="saveAlert">保存阈值</Button>
            <Button size="sm" variant="outline" @click="automationApi.clearAlert(); toast.success('已重置，下轮检查重新计')">重置告警状态</Button>
          </div>
        </CardContent>
      </Card>

      <!-- PushPlus -->
      <Card>
        <CardHeader class="pb-2">
          <CardTitle class="text-base flex items-center gap-2"><Smartphone class="w-4 h-4" />PushPlus 微信推送</CardTitle>
        </CardHeader>
        <CardContent class="space-y-3">
          <p class="text-xs text-muted-foreground">TG 不在线时更及时的通知通道。到 pushplus.plus 微信扫码登录，复制 token 粘贴到这里。留空保存 = 关闭。</p>
          <Input v-model="ppToken" :placeholder="ppMasked ? `当前已配置（${ppMasked}），输入新值覆盖` : '粘贴 PushPlus token'" />
          <div class="flex gap-2">
            <Button size="sm" @click="savePushplus">保存</Button>
            <Button size="sm" variant="outline" :disabled="testingPP" @click="testPushplus">
              <Loader2 v-if="testingPP" class="w-3 h-3 mr-1 animate-spin" />发送测试
            </Button>
          </div>
        </CardContent>
      </Card>

      <!-- 配额总览 -->
      <Card>
        <CardHeader class="pb-2"><CardTitle class="text-base">免费额度总览</CardTitle></CardHeader>
        <CardContent class="space-y-3">
          <div v-for="q in quota" :key="q.configId" class="rounded-lg border p-3 space-y-2">
            <div class="font-medium text-sm truncate">{{ q.tenantName || q.configId }}</div>
            <div class="grid grid-cols-2 gap-2 text-xs">
              <div class="flex justify-between"><span class="text-muted-foreground">实例数</span><span>{{ q.instanceCount }}</span></div>
              <div class="flex justify-between"><span class="text-muted-foreground">A1 OCPU</span><span :class="q.a1OcpusUsed >= 4 ? 'text-destructive' : ''">{{ q.a1OcpusUsed }}/4</span></div>
              <div class="flex justify-between"><span class="text-muted-foreground">A1 内存</span><span :class="q.a1MemoryUsed >= 24 ? 'text-destructive' : ''">{{ q.a1MemoryUsed }}/24GB</span></div>
              <div class="flex justify-between"><span class="text-muted-foreground">E2.Micro</span><span :class="q.e2MicroUsed >= 2 ? 'text-destructive' : ''">{{ q.e2MicroUsed }}/2</span></div>
              <div class="flex justify-between"><span class="text-muted-foreground">引导卷</span><span :class="q.bootVolumeGB >= 200 ? 'text-destructive' : ''">{{ q.bootVolumeGB }}/200GB</span></div>
              <div class="flex justify-between"><span class="text-muted-foreground">备份槽</span><span :class="q.backupCount >= 5 ? 'text-destructive' : ''">{{ q.backupCount }}/5</span></div>
            </div>
          </div>
          <p v-if="!quota.length" class="text-center text-sm text-muted-foreground py-4">暂无配置</p>
        </CardContent>
      </Card>

      <!-- CPU 阈值告警 -->
      <Card>
        <CardHeader class="pb-2"><CardTitle class="text-base">CPU 阈值告警</CardTitle></CardHeader>
        <CardContent class="space-y-3">
          <p class="text-xs text-muted-foreground">每 15 分钟检查一次全部运行中实例的近 1 小时 CPU 均值，超阈值推送告警（每实例每小时最多一次）。设为 0 = 关闭。</p>
          <div class="flex items-center gap-3">
            <Input type="number" v-model.number="cpuThreshold" min="0" max="100" class="w-24" />
            <span class="text-sm text-muted-foreground">% CPU 均值告警（0 关闭）</span>
          </div>
          <Button size="sm" :disabled="savingAlert" @click="saveAlert">保存阈值</Button>
        </CardContent>
      </Card>
    </template>

    <!-- ============ 监控曲线 ============ -->
    <template v-if="activeTab === 'metrics'">
      <Card>
        <CardHeader class="pb-2"><CardTitle class="text-base">CPU / 内存监控曲线</CardTitle></CardHeader>
        <CardContent class="space-y-3">
          <select v-model="mConfigId" class="w-full h-10 rounded-md border bg-background px-3 text-sm" @change="loadMetricsInstances(mConfigId)">
            <option value="" disabled>选择 OCI 配置</option>
            <option v-for="c in configs" :key="c.id" :value="c.id">{{ c.username }}</option>
          </select>
          <select v-model="mInstanceId" class="w-full h-10 rounded-md border bg-background px-3 text-sm">
            <option value="" disabled>选择实例</option>
            <option v-for="i in mInstances" :key="i.value" :value="i.value">{{ i.label }}</option>
          </select>
          <div class="flex items-center gap-2">
            <select v-model.number="mHours" class="h-10 rounded-md border bg-background px-3 text-sm">
              <option :value="6">近 6 小时</option>
              <option :value="24">近 24 小时</option>
              <option :value="72">近 3 天</option>
              <option :value="168">近 7 天</option>
            </select>
            <Button size="sm" :disabled="mLoading || !mInstanceId" @click="loadMetrics">
              <Loader2 v-if="mLoading" class="w-3 h-3 mr-1 animate-spin" />查询
            </Button>
          </div>

          <div v-if="mChart" class="rounded-lg border p-3 overflow-x-auto">
            <svg :viewBox="`0 0 ${chartW} ${chartH}`" class="w-full min-w-[320px]" style="height:180px">
              <line v-for="g in 4" :key="g" :x1="chartPad" :x2="chartW - chartPad"
                :y1="chartPad + (g - 1) * (chartH - chartPad * 2) / 3" :y2="chartPad + (g - 1) * (chartH - chartPad * 2) / 3"
                stroke="currentColor" stroke-opacity="0.1" />
              <polyline :points="cpuPoints" fill="none" stroke="#f59e0b" stroke-width="2" />
              <polyline v-if="mChart.hasMemory" :points="memPoints" fill="none" stroke="#3b82f6" stroke-width="2" stroke-dasharray="4 3" />
            </svg>
            <div class="flex justify-between text-[10px] text-muted-foreground mt-1">
              <span>{{ mChart.time[0] }}</span><span>{{ mChart.time[mChart.time.length - 1] }}</span>
            </div>
            <div class="flex gap-4 text-xs mt-2">
              <span class="flex items-center gap-1"><i class="inline-block w-3 h-0.5 bg-amber-500" />CPU%</span>
              <span v-if="mChart.hasMemory" class="flex items-center gap-1"><i class="inline-block w-3 h-0.5 bg-blue-500" />内存%</span>
            </div>
          </div>
          <p v-else class="text-center text-sm text-muted-foreground py-6">选择配置和实例后点查询</p>
        </CardContent>
      </Card>
    </template>

    <!-- ============ 保活表单 ============ -->
    <Dialog :open="showKeepaliveForm" @update:open="showKeepaliveForm = $event">
      <DialogHeader>
        <DialogTitle>{{ kForm.id ? '编辑' : '新建' }}保活任务</DialogTitle>
        <DialogDescription>通过 OCI Run Command 定期对实例注入 CPU+网络负载（免费）</DialogDescription>
      </DialogHeader>
      <div class="space-y-3 py-2">
        <select v-model="kForm.configId" class="w-full h-10 rounded-md border bg-background px-3 text-sm"
          @change="loadInstancesFor(kForm.configId)">
          <option value="" disabled>选择 OCI 配置</option>
          <option v-for="c in configs" :key="c.id" :value="c.id">{{ c.username }}</option>
        </select>
        <select v-model="kForm.instanceId" class="w-full h-10 rounded-md border bg-background px-3 text-sm">
          <option value="" disabled>选择实例</option>
          <option v-for="i in kInstances" :key="i.value" :value="i.value">{{ i.label }}</option>
        </select>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="text-xs text-muted-foreground">间隔（分钟 ≥30）</label>
            <Input type="number" v-model.number="kForm.intervalMin" min="30" />
          </div>
          <div>
            <label class="text-xs text-muted-foreground">负载时长（秒 ≤600）</label>
            <Input type="number" v-model.number="kForm.durationSec" min="30" max="600" />
          </div>
        </div>
        <p class="text-xs text-muted-foreground">需要实例上 Oracle Cloud Agent 处于运行状态（官方镜像默认开启）。</p>
      </div>
      <DialogFooter>
        <Button variant="outline" @click="showKeepaliveForm = false"><X class="w-4 h-4 mr-1" />取消</Button>
        <Button @click="saveKeepalive">保存</Button>
      </DialogFooter>
    </Dialog>

    <!-- ============ 抢机表单 ============ -->
    <Dialog :open="showGrabForm" @update:open="showGrabForm = $event">
      <DialogHeader>
        <DialogTitle>{{ gForm.id ? '编辑' : '新建' }}抢机任务</DialogTitle>
        <DialogDescription>仅支持 Always Free 形状，抢到后自动停止并推送通知</DialogDescription>
      </DialogHeader>
      <div class="space-y-3 py-2 max-h-[60vh] overflow-y-auto">
        <Input v-model="gForm.name" placeholder="任务名称（如：抢首尔A1）" />
        <select v-model="gForm.configId" class="w-full h-10 rounded-md border bg-background px-3 text-sm"
          @change="() => { loadGrabOptions(gForm.configId); loadImages(gForm.configId, gForm.shape) }">
          <option value="" disabled>选择 OCI 配置</option>
          <option v-for="c in configs" :key="c.id" :value="c.id">{{ c.username }}</option>
        </select>
        <select v-model="gForm.shape" class="w-full h-10 rounded-md border bg-background px-3 text-sm"
          @change="loadImages(gForm.configId, gForm.shape)">
          <option v-for="s in gShapes" :key="s" :value="s">{{ s }}</option>
        </select>
        <select v-model="gForm.ad" class="w-full h-10 rounded-md border bg-background px-3 text-sm">
          <option value="" disabled>选择可用域</option>
          <option v-for="a in gAds" :key="a.value" :value="a.value">{{ a.label }}</option>
        </select>
        <select v-model="gForm.imageId" class="w-full h-10 rounded-md border bg-background px-3 text-sm">
          <option value="" disabled>{{ gImages.length ? '选择系统镜像' : '先选配置和形状' }}</option>
          <option v-for="i in gImages" :key="i.value" :value="i.value">{{ i.label }}</option>
        </select>
        <select v-model="gForm.subnetId" class="w-full h-10 rounded-md border bg-background px-3 text-sm">
          <option value="" disabled>{{ gSubnets.length ? '选择子网' : '先选配置' }}</option>
          <option v-for="n in gSubnets" :key="n.value" :value="n.value">{{ n.label }}</option>
        </select>
        <div class="grid grid-cols-3 gap-3">
          <div><label class="text-xs text-muted-foreground">OCPU</label><Input type="number" v-model.number="gForm.ocpus" min="1" max="4" /></div>
          <div><label class="text-xs text-muted-foreground">内存 GB</label><Input type="number" v-model.number="gForm.memoryGB" min="1" max="24" /></div>
          <div><label class="text-xs text-muted-foreground">引导卷 GB</label><Input type="number" v-model.number="gForm.bootVolumeGB" min="47" max="200" /></div>
        </div>
        <Input v-model="gForm.instanceName" placeholder="实例名（可选）" />
        <div><label class="text-xs text-muted-foreground">重试间隔（分钟 ≥1）</label><Input type="number" v-model.number="gForm.intervalMin" min="1" /></div>
      </div>
      <DialogFooter>
        <Button variant="outline" @click="showGrabForm = false"><X class="w-4 h-4 mr-1" />取消</Button>
        <Button @click="saveGrab">保存</Button>
      </DialogFooter>
    </Dialog>

    <!-- ============ 备份表单 ============ -->
    <Dialog :open="showBackupForm" @update:open="showBackupForm = $event">
      <DialogHeader>
        <DialogTitle>{{ bForm.id ? '编辑' : '新建' }}备份任务</DialogTitle>
        <DialogDescription>增量备份 · 滚动删除最旧 · 总数永不超 5（免费上限）</DialogDescription>
      </DialogHeader>
      <div class="space-y-3 py-2">
        <select v-model="bForm.configId" class="w-full h-10 rounded-md border bg-background px-3 text-sm"
          @change="loadVolumes(bForm.configId)">
          <option value="" disabled>选择 OCI 配置</option>
          <option v-for="c in configs" :key="c.id" :value="c.id">{{ c.username }}</option>
        </select>
        <select v-model="bForm.volumeId" class="w-full h-10 rounded-md border bg-background px-3 text-sm">
          <option value="" disabled>选择卷</option>
          <option v-for="v in bVolumes" :key="v.id" :value="v.id">{{ v.name }}（{{ v.type === 'boot' ? '引导卷' : '块卷' }} {{ v.sizeGB }}GB）</option>
        </select>
        <div class="grid grid-cols-2 gap-3">
          <div><label class="text-xs text-muted-foreground">保留份数（≤5）</label><Input type="number" v-model.number="bForm.retention" min="1" max="5" /></div>
          <div><label class="text-xs text-muted-foreground">间隔（小时 ≥6）</label><Input type="number" v-model.number="bForm.intervalHour" min="6" /></div>
        </div>
      </div>
      <DialogFooter>
        <Button variant="outline" @click="showBackupForm = false"><X class="w-4 h-4 mr-1" />取消</Button>
        <Button @click="saveBackup">保存</Button>
      </DialogFooter>
    </Dialog>
  </div>
</template>
