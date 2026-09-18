<script setup lang="ts">
import { ref, reactive, watch } from 'vue'
import { X, Loader2, RefreshCw, Settings, Server, HardDrive, Network, BarChart3, Wallet } from 'lucide-vue-next'
import { ociApi, instanceApi } from '@/api'
import { toast } from '@/composables/useToast'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Dialog, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import type { Config, Instance } from '@/views/configs/types'
import type { CostStats } from '@/api'
import BasicInfoTab from './tabs/BasicInfoTab.vue'
import InstancesTab from './tabs/InstancesTab.vue'
import VolumesTab from './tabs/VolumesTab.vue'
import VcnsTab from './tabs/VcnsTab.vue'
import TrafficTab from './tabs/TrafficTab.vue'
import CostTab from './tabs/CostTab.vue'
import UserListCard from './UserListCard.vue'
import EditInstanceModal from './EditInstanceModal.vue'
import VolumeEditModal from './VolumeEditModal.vue'
import SecurityListModal from './SecurityListModal.vue'

// 配置详情抽屉（C7 从 Configs.vue 抽出）：承载详情数据加载、5 个标签页、用户管理、实例操作、
// 流量查询与 3 个子组件弹窗。逻辑由 Configs.vue 原样迁移（加载时序、懒加载 watch、刷新派发不变），
// 标签页改为纯展示子组件、动作经事件回传，行为与拆分前一致。
const props = defineProps<{
  open: boolean
  config: Config | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

const close = () => emit('update:open', false)

// 详情与标签页状态
const configDetails = ref<any>(null)
const loadingDetails = ref(false)
const loadingTab = ref(false)
const loadingTraffic = ref(false)

const activeTab = ref('basic')
const tabInstances = ref<Instance[]>([])
const tabVolumes = ref<any[]>([])
const tabVCNs = ref<any[]>([])
const tabTenant = ref<any>(null)
const tabTraffic = ref<{ time: string[]; inbound: string[]; outbound: string[] }>({
  time: [],
  inbound: [],
  outbound: []
})
const instanceActionLoading = reactive<Record<string, boolean>>({})

// 成本统计
const tabCost = ref<CostStats | null>(null)
const loadingCost = ref(false)
const costError = ref('')
const costDays = ref(30)

// 流量查询
const trafficCondition = ref<{ instances: { value: string; label: string }[] }>({ instances: [] })
const trafficVnics = ref<{ value: string; label: string }[]>([])
const trafficForm = ref({ instanceId: '', vnicId: '', startTime: '', endTime: '' })

// 用户编辑
const showEditUserModal = ref(false)
const editingUser = ref<any>(null)
const userForm = ref({ email: '', dbUserName: '', description: '' })

// 子组件弹窗状态
const showEditInstanceModal = ref(false)
const showVolumeEditModal = ref(false)
const showSecurityListModal = ref(false)
const selectedInstance = ref<Instance | null>(null)
const selectedVolume = ref<any>(null)
const selectedVcn = ref<any>(null)

const tabs = [
  { key: 'basic', label: '基本信息', icon: Settings },
  { key: 'instances', label: '实例列表', icon: Server },
  { key: 'volumes', label: '引导卷', icon: HardDrive },
  { key: 'vcns', label: 'VCN网络', icon: Network },
  { key: 'traffic', label: '流量统计', icon: BarChart3 },
  { key: 'cost', label: '成本统计', icon: Wallet }
]

const pad = (n: number) => n.toString().padStart(2, '0')
// datetime-local 控件需要的本地墙钟格式：YYYY-MM-DDTHH:mm
const toLocalInput = (date: Date) =>
  `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
// 将 datetime-local（本地墙钟）转换为 UTC 的 ISO 字符串，避免时区错位
const toUtcIso = (local: string) => {
  if (!local) return ''
  const d = new Date(local) // 无时区的 YYYY-MM-DDTHH:mm 会被当作本地时间解析
  return isNaN(d.getTime()) ? local : d.toISOString()
}

// 打开抽屉时初始化详情（原 viewConfigDetails 主体，去掉自管的显隐开关）
const initDetails = async (config: Config) => {
  activeTab.value = 'basic'
  tabInstances.value = []
  tabVolumes.value = []
  tabVCNs.value = []
  tabTenant.value = null
  tabTraffic.value = { time: [], inbound: [], outbound: [] }
  tabCost.value = null
  costError.value = ''
  costDays.value = 30
  trafficCondition.value = { instances: [] }
  trafficVnics.value = []
  trafficForm.value = { instanceId: '', vnicId: '', startTime: '', endTime: '' }
  loadingDetails.value = true
  try {
    const response = await ociApi.details(config.id)
    configDetails.value = response.data
    await loadTenant()
  } catch (error: any) {
    toast.error(error.message || '加载配置详情失败')
    close()
  } finally {
    loadingDetails.value = false
  }
}

const resetDetails = () => {
  configDetails.value = null
  tabInstances.value = []
  tabVolumes.value = []
  tabVCNs.value = []
  tabTenant.value = null
  tabTraffic.value = { time: [], inbound: [], outbound: [] }
  tabCost.value = null
  costError.value = ''
}

watch(
  () => props.open,
  isOpen => {
    if (isOpen && props.config) initDetails(props.config)
    else if (!isOpen) resetDetails()
  }
)

// 标签页数据加载
const loadTenant = async (clearCache = false) => {
  if (!configDetails.value) return
  loadingTab.value = true
  try {
    const response = await ociApi.tenantInfo({ configId: configDetails.value.userId, clearCache })
    tabTenant.value = response.data
  } catch (error: any) {
    toast.error(error.message || '加载租户详情失败')
    tabTenant.value = null
  } finally {
    loadingTab.value = false
  }
}

const loadInstances = async (clearCache = false) => {
  if (!configDetails.value) return
  loadingTab.value = true
  try {
    const response = await ociApi.detailsInstances({ configId: configDetails.value.userId, clearCache })
    tabInstances.value = response.data || []
  } catch (error: any) {
    toast.error(error.message || '加载实例列表失败')
    tabInstances.value = []
  } finally {
    loadingTab.value = false
  }
}

const loadVolumes = async (clearCache = false) => {
  if (!configDetails.value) return
  loadingTab.value = true
  try {
    const response = await ociApi.detailsVolumes({ configId: configDetails.value.userId, clearCache })
    tabVolumes.value = response.data || []
  } catch (error: any) {
    toast.error(error.message || '加载存储卷列表失败')
    tabVolumes.value = []
  } finally {
    loadingTab.value = false
  }
}

const loadVCNs = async (clearCache = false) => {
  if (!configDetails.value) return
  loadingTab.value = true
  try {
    const response = await ociApi.detailsVCNs({ configId: configDetails.value.userId, clearCache })
    tabVCNs.value = response.data || []
  } catch (error: any) {
    toast.error(error.message || '加载VCN列表失败')
    tabVCNs.value = []
  } finally {
    loadingTab.value = false
  }
}

// 成本统计（每日费用）：失败不弹 toast，改为在标签页内展示可读错误（多为权限未授予）。
const loadCost = async (days = costDays.value) => {
  if (!configDetails.value) return
  costDays.value = days
  loadingCost.value = true
  costError.value = ''
  try {
    const response = await ociApi.dailyCost({ configId: configDetails.value.userId, days })
    tabCost.value = response.data || null
  } catch (error: any) {
    tabCost.value = null
    costError.value = error.message || '查询成本失败'
  } finally {
    loadingCost.value = false
  }
}

const loadTrafficCondition = async () => {
  if (!configDetails.value) return
  try {
    const response = await ociApi.trafficCondition(configDetails.value.userId)
    trafficCondition.value = response.data || { instances: [] }
    const now = new Date()
    const oneHourAgo = new Date(now.getTime() - 60 * 60 * 1000)
    trafficForm.value.endTime = toLocalInput(now)
    trafficForm.value.startTime = toLocalInput(oneHourAgo)
  } catch (error) {
    console.error('加载流量条件失败:', error)
  }
}

const loadInstanceVnics = async () => {
  if (!configDetails.value || !trafficForm.value.instanceId) return
  try {
    const response = await ociApi.trafficVnics({
      configId: configDetails.value.userId,
      instanceId: trafficForm.value.instanceId
    })
    trafficVnics.value = response.data || []
    trafficForm.value.vnicId = ''
  } catch {
    trafficVnics.value = []
  }
}

const loadTrafficData = async () => {
  if (!configDetails.value || !trafficForm.value.instanceId || !trafficForm.value.vnicId) {
    toast.error('请选择实例和VNIC')
    return
  }
  loadingTraffic.value = true
  try {
    const response = await ociApi.trafficData({
      configId: configDetails.value.userId,
      instanceId: trafficForm.value.instanceId,
      vnicId: trafficForm.value.vnicId,
      startTime: toUtcIso(trafficForm.value.startTime),
      endTime: toUtcIso(trafficForm.value.endTime)
    })
    tabTraffic.value = response.data || { time: [], inbound: [], outbound: [] }
    if (!tabTraffic.value.time || tabTraffic.value.time.length === 0) {
      toast.info('查询成功，但该时间段内所选 VNIC 无流量监控数据（可能该时段确实无流量）')
    }
  } catch (error: any) {
    toast.error(error.message || '加载流量数据失败')
    tabTraffic.value = { time: [], inbound: [], outbound: [] }
  } finally {
    loadingTraffic.value = false
  }
}

const refreshCurrentTab = async () => {
  if (activeTab.value === 'basic') await loadTenant(true)
  else if (activeTab.value === 'instances') await loadInstances(true)
  else if (activeTab.value === 'volumes') await loadVolumes(true)
  else if (activeTab.value === 'vcns') await loadVCNs(true)
  else if (activeTab.value === 'traffic') await loadTrafficCondition()
  else if (activeTab.value === 'cost') await loadCost()
}

watch(activeTab, newTab => {
  if (newTab === 'basic' && !tabTenant.value) loadTenant()
  else if (newTab === 'instances' && tabInstances.value.length === 0) loadInstances()
  else if (newTab === 'volumes' && tabVolumes.value.length === 0) loadVolumes()
  else if (newTab === 'vcns' && tabVCNs.value.length === 0) loadVCNs()
  else if (newTab === 'traffic' && trafficCondition.value.instances.length === 0) loadTrafficCondition()
  else if (newTab === 'cost' && !tabCost.value && !loadingCost.value) loadCost()
})

watch(
  () => trafficForm.value.instanceId,
  newVal => {
    if (newVal) loadInstanceVnics()
    else {
      trafficVnics.value = []
      trafficForm.value.vnicId = ''
    }
  }
)

// 用户管理
const editUser = (user: any) => {
  editingUser.value = user
  userForm.value = { email: user.email || '', dbUserName: user.name || '', description: '' }
  showEditUserModal.value = true
}
const closeEditUserModal = () => {
  showEditUserModal.value = false
  editingUser.value = null
  userForm.value = { email: '', dbUserName: '', description: '' }
}
const saveUserInfo = async () => {
  if (!editingUser.value) return
  try {
    await ociApi.updateUserInfo({
      ociCfgId: configDetails.value?.userId,
      userId: editingUser.value.id,
      email: userForm.value.email,
      dbUserName: userForm.value.dbUserName,
      description: userForm.value.description
    })
    toast.success('用户信息更新成功')
    closeEditUserModal()
    await loadTenant(true)
  } catch (error: any) {
    toast.error(error.message || '更新失败')
  }
}
const resetUserPassword = async (user: any) => {
  if (!confirm(`确定要重置用户 ${user.name} 的密码吗？`)) return
  try {
    await ociApi.resetPassword({ ociCfgId: configDetails.value?.userId, userId: user.id })
    toast.success('密码重置成功')
  } catch (error: any) {
    toast.error(error.message || '重置密码失败')
  }
}
const clearUserMfa = async (user: any) => {
  if (!confirm(`确定要清除用户 ${user.name} 的 MFA 设备吗？`)) return
  try {
    await ociApi.deleteMfaDevice({ ociCfgId: configDetails.value?.userId, userId: user.id })
    toast.success('MFA 设备清除成功')
    await loadTenant(true)
  } catch (error: any) {
    toast.error(error.message || 'MFA 清除失败')
  }
}
const clearUserApiKeys = async (user: any) => {
  if (!confirm(`确定要清除用户 ${user.name} 的所有 API 密钥吗？`)) return
  try {
    await ociApi.deleteApiKey({ ociCfgId: configDetails.value?.userId, userId: user.id })
    toast.success('API 密钥清除成功')
  } catch (error: any) {
    toast.error(error.message || 'API 密钥清除失败')
  }
}
const deleteUser = async (user: any) => {
  if (!confirm(`确定要删除用户 ${user.name} 吗？此操作不可恢复！`)) return
  try {
    await ociApi.deleteUser({ ociCfgId: configDetails.value?.userId, userId: user.id })
    toast.success('用户删除成功')
    await loadTenant(true)
  } catch (error: any) {
    toast.error(error.message || '删除用户失败')
  }
}

// 实例操作
const controlInstance = async (instanceId: string, action: string) => {
  const actionMap: Record<string, { fn: (userId: string, instanceId: string) => Promise<unknown>; message: string }> = {
    START: { fn: instanceApi.start, message: '启动' },
    STOP: { fn: instanceApi.stop, message: '停止' },
    SOFTRESET: { fn: instanceApi.reboot, message: '重启' }
  }
  instanceActionLoading[instanceId] = true
  try {
    await actionMap[action].fn(configDetails.value?.userId, instanceId)
    toast.success(`${actionMap[action].message}操作已提交`)
    setTimeout(() => loadInstances(true), 3000)
  } catch (error: any) {
    toast.error(error.message || '操作失败')
  } finally {
    delete instanceActionLoading[instanceId]
  }
}

const terminateInstance = async (instanceId: string) => {
  if (!confirm('确定要删除此实例吗？此操作不可恢复！')) return
  instanceActionLoading[instanceId] = true
  try {
    await instanceApi.terminate(configDetails.value?.userId, instanceId)
    toast.success('删除操作已提交')
    setTimeout(() => loadInstances(true), 3000)
  } catch (error: any) {
    toast.error(error.message || '删除失败')
  } finally {
    delete instanceActionLoading[instanceId]
  }
}

const changeIP = async (instanceId: string) => {
  if (!confirm('确定要更改此实例的公网IP吗？')) return
  instanceActionLoading[instanceId] = true
  try {
    const response = await instanceApi.changeIP(configDetails.value?.userId, instanceId)
    toast.success(response.data?.newIP ? `IP更改成功，新IP: ${response.data.newIP}` : 'IP更换请求已提交')
    setTimeout(() => loadInstances(true), 2000)
  } catch (error: any) {
    toast.error(error.message || '更换IP失败')
  } finally {
    delete instanceActionLoading[instanceId]
  }
}

// 打开子组件弹窗
const openEditInstance = (instance: Instance) => {
  selectedInstance.value = instance
  showEditInstanceModal.value = true
}
const openVolumeEdit = (volume: any) => {
  selectedVolume.value = volume
  showVolumeEditModal.value = true
}
const openSecurityList = (vcn: any) => {
  selectedVcn.value = vcn
  showSecurityListModal.value = true
}
</script>

<template>
  <Teleport to="body">
    <Transition name="fade">
      <div
        v-if="open"
        class="fixed inset-0 z-50 flex items-center justify-center p-2 sm:p-4 bg-black/70 backdrop-blur-sm"
        @click.self="close"
      >
        <div
          v-motion
          :initial="{ opacity: 0, scale: 0.95 }"
          :enter="{ opacity: 1, scale: 1 }"
          class="bg-card rounded-xl shadow-2xl w-full max-w-6xl min-w-0 max-h-[90vh] overflow-hidden flex flex-col border border-border sm:m-4"
        >
          <div class="flex items-center justify-between p-4 sm:p-6 border-b border-border bg-card/80 backdrop-blur">
            <div>
              <h2 class="text-xl font-bold">配置详情</h2>
              <p class="text-sm text-muted-foreground mt-1">{{ configDetails?.username }}</p>
            </div>
            <Button variant="ghost" size="icon" @click="close"><X class="w-5 h-5" /></Button>
          </div>
          <div class="flex-1 min-w-0 overflow-y-auto p-4 sm:p-6">
            <div v-if="loadingDetails" class="flex items-center justify-center py-12">
              <Loader2 class="w-10 h-10 animate-spin text-primary" />
            </div>
            <div v-else-if="configDetails" class="space-y-6">
              <!-- Tabs -->
              <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                <div class="flex gap-1 p-1 bg-muted/50 rounded-lg flex-wrap">
                  <button
                    v-for="tab in tabs"
                    :key="tab.key"
                    :class="[
                      'flex items-center gap-2 px-3 sm:px-4 py-2 rounded-md text-sm font-medium transition-all',
                      activeTab === tab.key
                        ? 'bg-primary text-primary-foreground shadow-sm'
                        : 'text-muted-foreground hover:text-foreground hover:bg-muted'
                    ]"
                    @click="activeTab = tab.key"
                  >
                    <component :is="tab.icon" class="w-4 h-4" />
                    {{ tab.label }}
                  </button>
                </div>
                <Button variant="outline" size="sm" class="shrink-0 self-start lg:self-auto" :disabled="loadingTab" @click="refreshCurrentTab">
                  <RefreshCw :class="['w-4 h-4', loadingTab && 'animate-spin']" />
                  刷新
                </Button>
              </div>

              <!-- Basic Info Tab -->
              <div v-show="activeTab === 'basic'" class="space-y-6">
                <BasicInfoTab :config-details="configDetails" :tenant="tabTenant" :loading="loadingTab" />
                <UserListCard
                  v-if="tabTenant?.userList?.length"
                  :users="tabTenant.userList"
                  @edit="editUser"
                  @reset-password="resetUserPassword"
                  @clear-mfa="clearUserMfa"
                  @clear-api-keys="clearUserApiKeys"
                  @delete="deleteUser"
                />
              </div>

              <!-- Instances Tab -->
              <div v-show="activeTab === 'instances'">
                <InstancesTab
                  :instances="tabInstances"
                  :loading="loadingTab"
                  :action-loading="instanceActionLoading"
                  @control="controlInstance"
                  @terminate="terminateInstance"
                  @change-ip="changeIP"
                  @edit="openEditInstance"
                />
              </div>

              <!-- Volumes Tab -->
              <div v-show="activeTab === 'volumes'">
                <VolumesTab :volumes="tabVolumes" :loading="loadingTab" @edit="openVolumeEdit" />
              </div>

              <!-- VCNs Tab -->
              <div v-show="activeTab === 'vcns'">
                <VcnsTab :vcns="tabVCNs" :loading="loadingTab" @security="openSecurityList" />
              </div>

              <!-- Traffic Tab -->
              <TrafficTab
                v-show="activeTab === 'traffic'"
                :form="trafficForm"
                :condition="trafficCondition"
                :vnics="trafficVnics"
                :traffic="tabTraffic"
                :loading="loadingTraffic"
                @query="loadTrafficData"
              />

              <!-- Cost Tab -->
              <CostTab
                v-show="activeTab === 'cost'"
                :cost="tabCost"
                :loading="loadingCost"
                :error="costError"
                :days="costDays"
                @reload="loadCost"
              />
            </div>
          </div>
          <div class="p-6 border-t border-border bg-card/80 backdrop-blur">
            <div class="flex justify-end">
              <Button variant="outline" @click="close">关闭</Button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>

  <!-- 编辑用户弹窗 -->
  <Dialog v-model:open="showEditUserModal">
    <DialogHeader class="mb-4">
      <DialogTitle>编辑用户信息</DialogTitle>
      <DialogDescription>
        修改用户
        <span class="text-primary font-medium">{{ editingUser?.name }}</span>
        的信息
      </DialogDescription>
    </DialogHeader>
    <form class="space-y-4" @submit.prevent="saveUserInfo">
      <div>
        <label class="block text-sm font-medium mb-2">用户名</label>
        <Input v-model="userForm.dbUserName" placeholder="输入用户名" required />
      </div>
      <div>
        <label class="block text-sm font-medium mb-2">邮箱</label>
        <Input v-model="userForm.email" type="email" placeholder="输入邮箱地址" required />
      </div>
      <div>
        <label class="block text-sm font-medium mb-2">描述（可选）</label>
        <Textarea v-model="userForm.description" :rows="3" placeholder="输入用户描述" />
      </div>
      <DialogFooter class="mt-6">
        <Button type="button" variant="outline" @click="closeEditUserModal">取消</Button>
        <Button type="submit">保存</Button>
      </DialogFooter>
    </form>
  </Dialog>

  <!-- 子组件弹窗 -->
  <EditInstanceModal
    v-model:open="showEditInstanceModal"
    :instance="selectedInstance"
    :user-id="configDetails?.userId || ''"
    @refresh="loadInstances(true)"
  />
  <VolumeEditModal
    v-model:open="showVolumeEditModal"
    :volume="selectedVolume"
    :user-id="configDetails?.userId || ''"
    @refresh="loadVolumes(true)"
  />
  <SecurityListModal
    v-model:open="showSecurityListModal"
    :vcn="selectedVcn"
    :user-id="configDetails?.userId || ''"
    @refresh="loadVCNs(true)"
  />
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
