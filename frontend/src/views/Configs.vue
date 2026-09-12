<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useMotion } from '@vueuse/motion'
import { RouterLink } from 'vue-router'
import {
  Plus,
  Search,
  Trash2,
  Edit,
  Eye,
  ChevronLeft,
  ChevronRight,
  Loader2,
  Upload,
  X,
  Check,
  MoreHorizontal
} from 'lucide-vue-next'
import { ociApi, taskApi, presetApi, keyApi, type KeyItem, type ImageInfo, type Preset } from '@/api'
import { toast } from '@/composables/useToast'
import { useSelection } from '@/composables/useSelection'
import { usePagination } from '@/composables/usePagination'
import { formatFileSize, debounce } from '@/lib/utils'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Switch } from '@/components/ui/switch'
import { Dialog, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { Dropdown, DropdownItem } from '@/components/ui/dropdown'
import type { Config } from '@/views/configs/types'
import ConfigDetailsDrawer from './configs/components/ConfigDetailsDrawer.vue'

// 配置列表状态
const configs = ref<Config[]>([])
const loading = ref(false)
const searchText = ref('')
const { currentPage, pageSize, totalPages, applyResult } = usePagination()
const {
  selectedIds: selectedConfigIds,
  isAllSelected,
  isIndeterminate,
  toggle: toggleSelectConfig,
  toggleAll: toggleSelectAll,
  clear: clearSelection
} = useSelection(configs, c => c.id)

// 弹窗状态
const showAddModal = ref(false)
const showCreateInstanceModal = ref(false)
const showBatchCreateModal = ref(false)
const detailsOpen = ref(false)
const editingConfig = ref<Config | null>(null)
const selectedConfigForInstance = ref<Config | null>(null)
const selectedConfigForDetails = ref<Config | null>(null)

// 加载状态
const submitting = ref(false)
const submittingInstance = ref(false)
const loadingImages = ref(false)

// 文件上传
const isDragging = ref(false)
const uploadedFile = ref<File | null>(null)
const fileInput = ref<HTMLInputElement>()

// 表单
const form = ref({ username: '', configContent: '' })
const instanceForm = ref({
  ociRegion: '',
  ocpus: 1,
  memory: 6,
  disk: 50,
  bootVolumeVpu: 10,
  architecture: 'ARM',
  operationSystem: 'Ubuntu',
  imageId: '',
  sshKeyId: '',
  interval: 60,
  isTaskMode: true
})
const sshKeys = ref<KeyItem[]>([])
const availableImages = ref<ImageInfo[]>([])
const filteredImages = ref<ImageInfo[]>([])
const presets = ref<Preset[]>([])
const selectedPresetId = ref('')

// 工具函数
const parseConfigContent = (content: string) => {
  const config: Record<string, string> = {}
  content.split('\n').forEach(line => {
    const [key, value] = line.split('=').map(s => s.trim())
    if (key && value) config[key.toLowerCase()] = value
  })
  return {
    ociTenantId: config['tenancy'] || '',
    ociUserId: config['user'] || '',
    ociFingerprint: config['fingerprint'] || '',
    ociRegion: config['region'] || ''
  }
}

// API 操作
const loadConfigs = async (page = 1) => {
  loading.value = true
  try {
    const response = await ociApi.userPage({ page, pageSize, username: searchText.value })
    configs.value = response.data.list || []
    applyResult(response.data)
  } catch (error: any) {
    toast.error(error.message || '加载失败')
  } finally {
    loading.value = false
  }
}

const loadSSHKeys = async () => {
  try {
    const response = await keyApi.standalone()
    sshKeys.value = response.data || []
  } catch {
    sshKeys.value = []
  }
}

const loadPresets = async () => {
  try {
    const response = await presetApi.list()
    presets.value = response.data || []
  } catch {
    presets.value = []
  }
}

const applyPreset = (presetId: string) => {
  if (!presetId) return
  const preset = presets.value.find(p => p.id === presetId)
  if (preset) {
    instanceForm.value.ocpus = preset.ocpus
    instanceForm.value.memory = preset.memory
    instanceForm.value.disk = preset.disk
    instanceForm.value.bootVolumeVpu = preset.bootVolumeVpu
    instanceForm.value.architecture = preset.architecture
    instanceForm.value.operationSystem = preset.operationSystem
    if (preset.imageId) {
      instanceForm.value.imageId = preset.imageId
    }
    if (preset.sshKeyId) {
      instanceForm.value.sshKeyId = preset.sshKeyId
    }
    if (selectedConfigForInstance.value) {
      loadImages(selectedConfigForInstance.value.id, instanceForm.value.ociRegion, preset.architecture)
    }
    toast.success(`已应用预设: ${preset.name}`)
  }
}

const loadImages = async (configId: string, region: string, architecture: string) => {
  if (!configId || !region || !architecture) return
  loadingImages.value = true
  try {
    const response = await ociApi.images({ configId, region, architecture })
    availableImages.value = response.data || []
    filterImagesByOS()
  } catch {
    availableImages.value = []
    filteredImages.value = []
  } finally {
    loadingImages.value = false
  }
}

const filterImagesByOS = () => {
  const os = instanceForm.value.operationSystem.toLowerCase()
  filteredImages.value = availableImages.value.filter(img =>
    img.operatingSystem.toLowerCase().includes(os === 'oracle linux' ? 'oracle' : os)
  )
  instanceForm.value.imageId = filteredImages.value.length > 0 ? filteredImages.value[0].id : ''
}

// 配置操作
const handleSearch = debounce(() => {
  clearSelection()
  loadConfigs(1)
}, 300)
const closeModal = () => {
  showAddModal.value = false
  editingConfig.value = null
  form.value = { username: '', configContent: '' }
  uploadedFile.value = null
}
const handleFileDrop = (e: DragEvent) => {
  isDragging.value = false
  if (e.dataTransfer?.files?.length) uploadedFile.value = e.dataTransfer.files[0]
}
const handleFileSelect = (e: Event) => {
  const target = e.target as HTMLInputElement
  if (target.files?.length) uploadedFile.value = target.files[0]
}
const clearFile = () => {
  uploadedFile.value = null
  if (fileInput.value) fileInput.value.value = ''
}

const submitForm = async () => {
  if (!uploadedFile.value && !editingConfig.value) {
    toast.error('请选择密钥文件')
    return
  }
  submitting.value = true
  try {
    let keyPath = ''
    if (uploadedFile.value) {
      const formData = new FormData()
      formData.append('file', uploadedFile.value)
      const uploadResponse = await ociApi.uploadKey(formData)
      keyPath = uploadResponse.data
    }
    const parsedConfig = parseConfigContent(form.value.configContent)
    if (editingConfig.value) {
      await ociApi.updateCfgName({
        id: editingConfig.value.id,
        username: form.value.username,
        ociKeyPath: uploadedFile.value ? keyPath : undefined
      })
      toast.success('配置已更新')
    } else {
      await ociApi.addCfg({
        username: form.value.username,
        tenantName: form.value.username,
        ...parsedConfig,
        ociKeyPath: keyPath
      })
      toast.success('配置已添加')
    }
    closeModal()
    await loadConfigs(currentPage.value)
  } catch (error: any) {
    toast.error(error.message || '操作失败')
  } finally {
    submitting.value = false
  }
}

const editConfig = (config: Config) => {
  editingConfig.value = config
  form.value = {
    username: config.username,
    configContent: `user=${config.ociUserId || ''}\nfingerprint=${config.ociFingerprint || ''}\ntenancy=${config.ociTenantId || ''}\nregion=${config.ociRegion || ''}`
  }
  showAddModal.value = true
}

const deleteConfig = async (id: string) => {
  if (!confirm('确定要删除此配置吗？')) return
  try {
    await ociApi.removeCfg([id])
    toast.success('配置已删除')
    await loadConfigs(currentPage.value)
  } catch (error: any) {
    toast.error(error.message || '删除失败')
  }
}

const batchDeleteConfigs = async () => {
  if (!confirm(`确定要删除选中的 ${selectedConfigIds.value.length} 个配置吗？`)) return
  try {
    await ociApi.removeCfg(selectedConfigIds.value)
    toast.success(`成功删除 ${selectedConfigIds.value.length} 个配置`)
    clearSelection()
    await loadConfigs(currentPage.value)
  } catch (error: any) {
    toast.error(error.message || '批量删除失败')
  }
}

// 创建实例
const createInstance = async (config: Config) => {
  selectedConfigForInstance.value = config
  instanceForm.value.ociRegion = config.ociRegion
  selectedPresetId.value = ''
  await Promise.all([loadSSHKeys(), loadPresets()])
  await loadImages(config.id, config.ociRegion, instanceForm.value.architecture)
  showCreateInstanceModal.value = true
}
const closeInstanceModal = () => {
  showCreateInstanceModal.value = false
  selectedConfigForInstance.value = null
  availableImages.value = []
  filteredImages.value = []
}
const onArchitectureChange = () => {
  if (selectedConfigForInstance.value)
    loadImages(selectedConfigForInstance.value.id, instanceForm.value.ociRegion, instanceForm.value.architecture)
}
const onOperationSystemChange = () => {
  filterImagesByOS()
}

const submitInstanceTask = async () => {
  const config = selectedConfigForInstance.value
  if (!config) {
    toast.warning('请先选择配置')
    return
  }
  if (!instanceForm.value.sshKeyId) {
    toast.warning('请选择SSH公钥')
    return
  }
  submittingInstance.value = true
  try {
    await taskApi.create({
      userId: config.id,
      ociRegion: instanceForm.value.ociRegion,
      ocpus: instanceForm.value.ocpus,
      memory: instanceForm.value.memory,
      disk: instanceForm.value.disk,
      bootVolumeVpu: instanceForm.value.bootVolumeVpu,
      architecture: instanceForm.value.architecture,
      operationSystem: instanceForm.value.operationSystem,
      imageId: instanceForm.value.imageId || undefined,
      sshKeyId: instanceForm.value.sshKeyId,
      interval: instanceForm.value.interval || 60,
      executeOnce: !instanceForm.value.isTaskMode
    })
    toast.success(instanceForm.value.isTaskMode ? '任务已创建，可在任务列表查看' : '实例创建请求已提交')
    closeInstanceModal()
  } catch (error: any) {
    toast.error(error.message || '创建失败')
  } finally {
    submittingInstance.value = false
  }
}

// 批量创建实例
const batchCreateInstance = async () => {
  if (selectedConfigIds.value.length === 0) {
    toast.warning('请先选择配置')
    return
  }
  selectedPresetId.value = ''
  await Promise.all([loadSSHKeys(), loadPresets()])
  showBatchCreateModal.value = true
}
const closeBatchCreateModal = () => {
  showBatchCreateModal.value = false
}

const submitBatchInstanceTask = async () => {
  if (!instanceForm.value.sshKeyId) {
    toast.warning('请选择SSH公钥')
    return
  }
  submittingInstance.value = true
  try {
    const ids = [...selectedConfigIds.value]
    // 并发创建（替代逐个 await 的串行），并逐项汇总成功/失败结果
    const results = await Promise.allSettled(
      ids.map(configId => {
        const config = configs.value.find(c => c.id === configId)
        return taskApi.create({
          userId: configId,
          ociRegion: config?.ociRegion || instanceForm.value.ociRegion,
          ocpus: instanceForm.value.ocpus,
          memory: instanceForm.value.memory,
          disk: instanceForm.value.disk,
          architecture: instanceForm.value.architecture,
          operationSystem: instanceForm.value.operationSystem,
          sshKeyId: instanceForm.value.sshKeyId,
          interval: instanceForm.value.interval || 60
        })
      })
    )
    const succeeded = results.filter(r => r.status === 'fulfilled').length
    const failedIds = ids.filter((_, i) => results[i].status === 'rejected')
    if (failedIds.length === 0) {
      toast.success(`已为 ${succeeded} 个配置创建定时任务`)
    } else {
      const failedNames = failedIds.map(id => configs.value.find(c => c.id === id)?.username || `#${id}`)
      toast.warning(`成功 ${succeeded} 个，失败 ${failedIds.length} 个：${failedNames.join('、')}`)
    }
    closeBatchCreateModal()
    clearSelection()
  } catch (error: any) {
    toast.error(error.message || '批量创建失败')
  } finally {
    submittingInstance.value = false
  }
}

// 配置详情（打开抽屉，详情逻辑由 ConfigDetailsDrawer 承载）
const viewConfigDetails = (config: Config) => {
  selectedConfigForDetails.value = config
  detailsOpen.value = true
}

onMounted(() => {
  loadConfigs()
})

const headerRef = ref<HTMLElement>()
useMotion(headerRef, { initial: { opacity: 0, y: -20 }, enter: { opacity: 1, y: 0 } })
</script>

<template>
  <div class="space-y-6">
    <!-- Header -->
    <div ref="headerRef" class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4">
      <div class="flex items-center gap-4">
        <h1 class="text-3xl font-display font-bold">配置管理</h1>
        <Badge v-if="selectedConfigIds.length > 0" variant="secondary">已选择 {{ selectedConfigIds.length }} 项</Badge>
      </div>
      <div class="flex gap-2">
        <Button v-if="selectedConfigIds.length > 0" variant="success" @click="batchCreateInstance">
          <Plus class="w-4 h-4" />
          批量创建实例
        </Button>
        <Button v-if="selectedConfigIds.length > 0" variant="destructive" @click="batchDeleteConfigs">
          <Trash2 class="w-4 h-4" />
          批量删除
        </Button>
        <Button @click="showAddModal = true">
          <Plus class="w-4 h-4" />
          添加配置
        </Button>
      </div>
    </div>

    <!-- Main Card -->
    <Card
      v-motion
      :initial="{ opacity: 0, y: 20 }"
      :enter="{ opacity: 1, y: 0, transition: { delay: 100 } }"
      class="border-border/50"
    >
      <CardHeader class="border-b border-border/50 pb-4">
        <div class="relative max-w-md">
          <Search class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
          <Input v-model="searchText" placeholder="搜索配置名..." class="pl-10" @input="handleSearch" />
        </div>
      </CardHeader>
      <CardContent class="p-0">
        <!-- 加载 / 空态 -->
        <div v-if="loading" class="h-32 flex items-center justify-center">
          <Loader2 class="w-8 h-8 animate-spin text-primary" />
        </div>
        <div v-else-if="!configs.length" class="h-32 flex items-center justify-center text-muted-foreground">
          暂无配置
        </div>
        <template v-else>
          <!-- 桌面端：表格行 -->
          <div class="hidden lg:block">
            <div
              class="grid grid-cols-[2.5rem_minmax(0,1fr)_12rem_8rem_6rem_auto] gap-4 items-center px-4 py-3 text-xs font-medium text-muted-foreground border-b border-border/50"
            >
              <div class="flex items-center">
                <Checkbox
                  :model-value="isAllSelected"
                  :indeterminate="isIndeterminate"
                  @update:model-value="toggleSelectAll"
                />
              </div>
              <div>配置名</div>
              <div>租户名称</div>
              <div>区域</div>
              <div>实例</div>
              <div class="text-right">操作</div>
            </div>
            <div
              v-for="(config, index) in configs"
              :key="config.id"
              v-motion
              :initial="{ opacity: 0, x: -20 }"
              :enter="{ opacity: 1, x: 0, transition: { delay: 50 * index } }"
              class="grid grid-cols-[2.5rem_minmax(0,1fr)_12rem_8rem_6rem_auto] gap-4 items-center px-4 py-3 border-b border-border/50 last:border-0"
            >
              <div class="flex items-center">
                <Checkbox
                  :model-value="selectedConfigIds.includes(config.id)"
                  @update:model-value="toggleSelectConfig(config.id)"
                />
              </div>
              <div class="font-medium truncate">{{ config.username }}</div>
              <div class="text-muted-foreground truncate">{{ config.tenantName || '-' }}</div>
              <div><Badge variant="info">{{ config.ociRegion }}</Badge></div>
              <div>
                <span>{{ config.instanceCount || 0 }}</span>
                <span v-if="config.runningInstances" class="text-success text-xs ml-1">
                  ({{ config.runningInstances }}运行)
                </span>
              </div>
              <div class="flex justify-end items-center gap-1">
                <Button size="sm" variant="success" @click="createInstance(config)">
                  <Plus class="w-3.5 h-3.5" />
                  <span class="ml-1">创建实例</span>
                </Button>
                <Button size="sm" variant="outline" @click="viewConfigDetails(config)">
                  <Eye class="w-3.5 h-3.5" />
                  <span class="ml-1">详情</span>
                </Button>
                <Dropdown align="right">
                  <template #trigger>
                    <Button size="sm" variant="ghost"><MoreHorizontal class="w-4 h-4" /></Button>
                  </template>
                  <DropdownItem @click="editConfig(config)">
                    <Edit class="w-4 h-4" />
                    编辑配置
                  </DropdownItem>
                  <DropdownItem destructive @click="deleteConfig(config.id)">
                    <Trash2 class="w-4 h-4" />
                    删除配置
                  </DropdownItem>
                </Dropdown>
              </div>
            </div>
          </div>

          <!-- 手机端：竖向卡片 -->
          <div class="lg:hidden divide-y divide-border/50">
            <div v-for="config in configs" :key="config.id" class="p-4 space-y-3">
              <div class="flex items-start justify-between gap-3">
                <div class="flex items-center gap-3 min-w-0">
                  <Checkbox
                    :model-value="selectedConfigIds.includes(config.id)"
                    @update:model-value="toggleSelectConfig(config.id)"
                  />
                  <div class="min-w-0">
                    <p class="font-medium truncate">{{ config.username }}</p>
                    <p class="text-xs text-muted-foreground truncate">{{ config.tenantName || '-' }}</p>
                  </div>
                </div>
                <Badge variant="info" class="shrink-0">{{ config.ociRegion }}</Badge>
              </div>
              <div class="flex items-center justify-between text-sm">
                <span class="text-muted-foreground">实例数</span>
                <span>
                  {{ config.instanceCount || 0 }}
                  <span v-if="config.runningInstances" class="text-success ml-1">
                    ({{ config.runningInstances }}运行)
                  </span>
                </span>
              </div>
              <div class="flex flex-wrap gap-2">
                <Button size="sm" variant="success" @click="createInstance(config)">
                  <Plus class="w-3.5 h-3.5" />创建实例
                </Button>
                <Button size="sm" variant="outline" @click="viewConfigDetails(config)">
                  <Eye class="w-3.5 h-3.5" />详情
                </Button>
                <Button size="sm" variant="outline" @click="editConfig(config)">
                  <Edit class="w-3.5 h-3.5" />编辑
                </Button>
                <Button size="sm" variant="destructive" @click="deleteConfig(config.id)">
                  <Trash2 class="w-3.5 h-3.5" />删除
                </Button>
              </div>
            </div>
          </div>
        </template>
        <div v-if="totalPages > 1" class="flex items-center justify-center gap-2 p-4 border-t border-border/50">
          <Button variant="outline" size="sm" :disabled="currentPage === 1" @click="loadConfigs(currentPage - 1)">
            <ChevronLeft class="w-4 h-4" />
            上一页
          </Button>
          <span class="px-4 text-sm text-muted-foreground">第 {{ currentPage }} / {{ totalPages }} 页</span>
          <Button
            variant="outline"
            size="sm"
            :disabled="currentPage === totalPages"
            @click="loadConfigs(currentPage + 1)"
          >
            下一页
            <ChevronRight class="w-4 h-4" />
          </Button>
        </div>
      </CardContent>
    </Card>

    <!-- Add/Edit Config Modal -->
    <Dialog v-model:open="showAddModal">
      <DialogHeader class="mb-4">
        <DialogTitle>{{ editingConfig ? '编辑配置' : '添加OCI配置' }}</DialogTitle>
        <DialogDescription>{{ editingConfig ? '修改现有配置信息' : '添加新的Oracle Cloud配置' }}</DialogDescription>
      </DialogHeader>

      <!-- 甲骨文 API 信息获取指引（可展开） -->
      <details class="rounded-lg border border-border bg-muted/40 p-3 text-sm">
        <summary class="cursor-pointer select-none font-medium text-primary">
          如何获取甲骨文（Oracle Cloud）API 信息？
        </summary>
        <div class="mt-3 space-y-2 text-muted-foreground leading-relaxed">
          <p>本面板通过 Oracle 官方 API 管理云资源，需要一份 <b class="text-foreground">API 密钥</b>。在 Oracle Cloud 控制台按以下步骤获取：</p>
          <ol class="list-decimal pl-5 space-y-1">
            <li>登录 <a href="https://console.us-ashburn-1.oraclecloud.com" target="_blank" rel="noopener" class="text-primary hover:underline">Oracle Cloud 控制台</a></li>
            <li>右上角头像 → <b>用户设置</b>，左侧找到 <b>API Keys</b></li>
            <li>点 <b>Add API Key</b> → 选 <b>Generate API Key Pair</b></li>
            <li>下载 <b>.pem 私钥</b>（留作下方「密钥文件」），并复制页面显示的 <b>Configuration File Preview</b></li>
            <li>把预览里的 <code class="px-1 bg-background rounded">user=</code> / <code class="px-1 bg-background rounded">fingerprint=</code> / <code class="px-1 bg-background rounded">tenancy=</code> / <code class="px-1 bg-background rounded">region=</code> 四行粘贴到下方「配置内容」</li>
            <li>把下载的 <b>.pem 私钥</b> 拖到下方「密钥文件」</li>
          </ol>
          <p class="text-xs pt-1">
            建议为面板单独建一个<b>最小权限用户</b>，密钥泄露也只影响该用户。详见
            <a href="https://docs.oracle.com/en-us/iaas/Content/API/Concepts/apisigningkey.htm" target="_blank" rel="noopener" class="text-primary hover:underline">Oracle 官方文档</a>。
          </p>
        </div>
      </details>

      <form class="space-y-4" @submit.prevent="submitForm">
        <div>
          <label class="block text-sm font-medium mb-2">配置名称</label>
          <Input v-model="form.username" placeholder="例: 我的OCI配置" required />
        </div>
        <div>
          <label class="block text-sm font-medium mb-2">配置内容</label>
          <Textarea
            v-model="form.configContent"
            :rows="6"
            class="font-mono text-sm"
            placeholder="user=ocid1.user.oc1..xxx&#10;fingerprint=xx:xx:xx&#10;tenancy=ocid1.tenancy.oc1..xxx&#10;region=ap-singapore-1"
            required
          />
          <p class="text-xs text-muted-foreground mt-2">
            直接粘贴 Oracle 控制台 <b>API Keys → Add API Key</b> 显示的 <b>Configuration File Preview</b>
            （<code class="px-1 bg-background rounded">user</code> / <code class="px-1 bg-background rounded">fingerprint</code> /
            <code class="px-1 bg-background rounded">tenancy</code> / <code class="px-1 bg-background rounded">region</code> 四行，可含 [DEFAULT]）。
          </p>
        </div>
        <div>
          <label class="block text-sm font-medium mb-2">密钥文件</label>
          <div
            :class="[
              'border-2 border-dashed rounded-lg p-6 transition-colors cursor-pointer text-center',
              isDragging ? 'border-primary bg-primary/5' : 'border-border hover:border-primary/50'
            ]"
            @drop.prevent="handleFileDrop"
            @dragover.prevent="isDragging = true"
            @dragleave.prevent="isDragging = false"
            @click="fileInput?.click()"
          >
            <input ref="fileInput" type="file" accept=".pem,.key" class="hidden" @change="handleFileSelect" />
            <div v-if="!uploadedFile">
              <Upload class="mx-auto h-10 w-10 text-muted-foreground mb-2" />
              <p class="text-sm text-muted-foreground">点击或拖拽文件</p>
              <p class="text-xs text-muted-foreground mt-1">即上一步下载的 .pem 私钥文件（仅本面板使用，不会外传）</p>
            </div>
            <div v-else class="flex items-center justify-between">
              <div class="flex items-center gap-3">
                <div class="w-10 h-10 rounded-lg bg-success/10 flex items-center justify-center">
                  <Check class="w-5 h-5 text-success" />
                </div>
                <div class="text-left">
                  <p class="text-sm font-medium">{{ uploadedFile.name }}</p>
                  <p class="text-xs text-muted-foreground">{{ formatFileSize(uploadedFile.size) }}</p>
                </div>
              </div>
              <Button type="button" variant="ghost" size="icon" @click.stop="clearFile"><X class="w-4 h-4" /></Button>
            </div>
          </div>
        </div>
        <DialogFooter class="mt-6">
          <Button type="button" variant="outline" @click="closeModal">取消</Button>
          <Button type="submit" :disabled="submitting">
            <Loader2 v-if="submitting" class="w-4 h-4 animate-spin" />
            {{ submitting ? '提交中...' : '提交' }}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>

    <!-- Create Instance Modal -->
    <Dialog v-model:open="showCreateInstanceModal">
      <DialogHeader class="mb-4">
        <DialogTitle>创建实例任务</DialogTitle>
        <DialogDescription>
          为配置
          <span class="text-primary font-medium">{{ selectedConfigForInstance?.username }}</span>
          创建实例
        </DialogDescription>
      </DialogHeader>
      <form class="space-y-4" @submit.prevent="submitInstanceTask">
        <div v-if="presets.length > 0">
          <label class="block text-sm font-medium mb-2">选择预设</label>
          <select
            v-model="selectedPresetId"
            class="w-full h-10 px-3 rounded-md border border-input bg-background text-sm"
            @change="applyPreset(selectedPresetId)"
          >
            <option value="">手动填写配置</option>
            <option v-for="preset in presets" :key="preset.id" :value="preset.id">
              {{ preset.name }} ({{ preset.ocpus }}核 {{ preset.memory }}GB {{ preset.architecture }})
            </option>
          </select>
          <p class="text-xs text-muted-foreground mt-1">
            选择预设后自动填充配置，或在
            <RouterLink to="/presets" class="text-primary hover:underline">预设配置</RouterLink>
            中管理
          </p>
        </div>
        <div>
          <label class="block text-sm font-medium mb-2">区域</label>
          <Input v-model="instanceForm.ociRegion" placeholder="例: ap-singapore-1" required />
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label class="block text-sm font-medium mb-2">CPU核心数</label>
            <Input v-model.number="instanceForm.ocpus" type="number" step="0.1" min="0.1" required />
          </div>
          <div>
            <label class="block text-sm font-medium mb-2">内存(GB)</label>
            <Input v-model.number="instanceForm.memory" type="number" step="0.1" min="0.1" required />
          </div>
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-3 gap-4">
          <div>
            <label class="block text-sm font-medium mb-2">磁盘(GB)</label>
            <Input v-model.number="instanceForm.disk" type="number" min="50" required />
          </div>
          <div>
            <label class="block text-sm font-medium mb-2">VPU/GB</label>
            <select
              v-model.number="instanceForm.bootVolumeVpu"
              class="w-full h-10 px-3 rounded-md border border-input bg-background text-sm"
            >
              <option v-for="vpu in [10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120]" :key="vpu" :value="vpu">
                {{ vpu }}
              </option>
            </select>
          </div>
          <div>
            <label class="block text-sm font-medium mb-2">架构</label>
            <select
              v-model="instanceForm.architecture"
              class="w-full h-10 px-3 rounded-md border border-input bg-background text-sm"
              @change="onArchitectureChange"
            >
              <option value="ARM">ARM</option>
              <option value="AMD">AMD</option>
            </select>
          </div>
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label class="block text-sm font-medium mb-2">操作系统</label>
            <select
              v-model="instanceForm.operationSystem"
              class="w-full h-10 px-3 rounded-md border border-input bg-background text-sm"
              @change="onOperationSystemChange"
            >
              <option value="Ubuntu">Ubuntu</option>
              <option value="CentOS">CentOS</option>
              <option value="Oracle Linux">Oracle Linux</option>
            </select>
          </div>
          <div>
            <label class="block text-sm font-medium mb-2">系统版本</label>
            <select
              v-model="instanceForm.imageId"
              class="w-full h-10 px-3 rounded-md border border-input bg-background text-sm"
              :disabled="loadingImages"
            >
              <option value="">
                {{ loadingImages ? '加载中...' : filteredImages.length === 0 ? '无可用镜像' : '自动选择最新' }}
              </option>
              <option v-for="img in filteredImages" :key="img.id" :value="img.id">
                {{ img.operatingSystem }} {{ img.operatingSystemVersion }}
              </option>
            </select>
          </div>
        </div>
        <div>
          <label class="block text-sm font-medium mb-2">SSH公钥</label>
          <select
            v-model="instanceForm.sshKeyId"
            class="w-full h-10 px-3 rounded-md border border-input bg-background text-sm"
            required
          >
            <option value="">请选择SSH公钥</option>
            <option v-for="key in sshKeys" :key="key.id" :value="key.id">{{ key.name }}</option>
          </select>
          <p class="text-xs text-muted-foreground mt-2">
            请先在
            <RouterLink to="/keys" class="text-primary hover:underline">密钥管理</RouterLink>
            中添加SSH公钥
          </p>
        </div>
        <div class="flex items-center gap-3 py-2">
          <Switch v-model="instanceForm.isTaskMode" />
          <span class="text-sm font-medium">抢占实例任务</span>
          <span class="text-xs text-muted-foreground">（持续尝试直到成功）</span>
        </div>
        <div v-if="instanceForm.isTaskMode">
          <label class="block text-sm font-medium mb-2">执行间隔（秒）</label>
          <Input v-model.number="instanceForm.interval" type="number" min="10" placeholder="60" />
        </div>
        <DialogFooter class="mt-6">
          <Button type="button" variant="outline" @click="closeInstanceModal">取消</Button>
          <Button type="submit" :disabled="submittingInstance">
            <Loader2 v-if="submittingInstance" class="w-4 h-4 animate-spin" />
            {{ submittingInstance ? '创建中...' : instanceForm.isTaskMode ? '创建任务' : '创建实例' }}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>

    <!-- Batch Create Instance Modal -->
    <Dialog v-model:open="showBatchCreateModal">
      <DialogHeader class="mb-4">
        <DialogTitle>批量创建实例任务</DialogTitle>
        <DialogDescription>
          将为
          <span class="text-primary font-medium">{{ selectedConfigIds.length }}</span>
          个配置批量创建实例
        </DialogDescription>
      </DialogHeader>
      <form class="space-y-4" @submit.prevent="submitBatchInstanceTask">
        <div v-if="presets.length > 0">
          <label class="block text-sm font-medium mb-2">选择预设</label>
          <select
            v-model="selectedPresetId"
            class="w-full h-10 px-3 rounded-md border border-input bg-background text-sm"
            @change="applyPreset(selectedPresetId)"
          >
            <option value="">手动填写配置</option>
            <option v-for="preset in presets" :key="preset.id" :value="preset.id">
              {{ preset.name }} ({{ preset.ocpus }}核 {{ preset.memory }}GB {{ preset.architecture }})
            </option>
          </select>
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label class="block text-sm font-medium mb-2">CPU核心数</label>
            <Input v-model.number="instanceForm.ocpus" type="number" step="0.1" min="0.1" required />
          </div>
          <div>
            <label class="block text-sm font-medium mb-2">内存(GB)</label>
            <Input v-model.number="instanceForm.memory" type="number" step="0.1" min="0.1" required />
          </div>
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label class="block text-sm font-medium mb-2">磁盘(GB)</label>
            <Input v-model.number="instanceForm.disk" type="number" min="50" required />
          </div>
          <div>
            <label class="block text-sm font-medium mb-2">架构</label>
            <select
              v-model="instanceForm.architecture"
              class="w-full h-10 px-3 rounded-md border border-input bg-background text-sm"
            >
              <option value="ARM">ARM</option>
              <option value="AMD">AMD</option>
            </select>
          </div>
        </div>
        <div>
          <label class="block text-sm font-medium mb-2">操作系统</label>
          <select
            v-model="instanceForm.operationSystem"
            class="w-full h-10 px-3 rounded-md border border-input bg-background text-sm"
          >
            <option value="Ubuntu">Ubuntu</option>
            <option value="CentOS">CentOS</option>
            <option value="Oracle Linux">Oracle Linux</option>
          </select>
        </div>
        <div>
          <label class="block text-sm font-medium mb-2">SSH公钥</label>
          <select
            v-model="instanceForm.sshKeyId"
            class="w-full h-10 px-3 rounded-md border border-input bg-background text-sm"
            required
          >
            <option value="">请选择SSH公钥</option>
            <option v-for="key in sshKeys" :key="key.id" :value="key.id">{{ key.name }}</option>
          </select>
        </div>
        <div>
          <label class="block text-sm font-medium mb-2">执行间隔（秒）</label>
          <Input v-model.number="instanceForm.interval" type="number" min="10" placeholder="60" />
        </div>
        <DialogFooter class="mt-6">
          <Button type="button" variant="outline" @click="closeBatchCreateModal">取消</Button>
          <Button type="submit" :disabled="submittingInstance">
            <Loader2 v-if="submittingInstance" class="w-4 h-4 animate-spin" />
            {{ submittingInstance ? '创建中...' : '批量创建' }}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>

    <!-- 配置详情抽屉 -->
    <ConfigDetailsDrawer v-model:open="detailsOpen" :config="selectedConfigForDetails" />
  </div>
</template>
