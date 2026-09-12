<script setup lang="ts">
import { ref, reactive, watch } from 'vue'
import { Loader2, AlertTriangle, Settings, HardDrive, Globe, Wrench, Zap } from 'lucide-vue-next'
import { instanceApi } from '@/api'
import { toast } from '@/composables/useToast'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Modal } from '@/components/ui/modal'

interface Instance {
  id: string
  displayName: string
  ocpus?: number
  memory?: number
  bootVolumeSize?: number
  bootVolumeVpu?: number
  ipv6?: string
  shape?: string
}

const props = defineProps<{
  open: boolean
  instance: Instance | null
  userId: string
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  refresh: []
}>()

const updating = ref(false)

const form = reactive({
  displayName: '',
  ocpus: 2,
  memoryInGBs: 12,
  bootVolumeSize: 50,
  vpusPerGB: 10,
  currentIpv6: '',
  keepBackup: false,
  sshPort: 22,
  retainNatGw: false,
  retainNlb: false
})

watch(
  () => props.instance,
  instance => {
    if (instance) {
      form.displayName = instance.displayName
      form.ocpus = instance.ocpus || 2
      form.memoryInGBs = instance.memory || 12
      form.bootVolumeSize = instance.bootVolumeSize || 50
      form.vpusPerGB = instance.bootVolumeVpu || 10
      form.currentIpv6 = instance.ipv6 || ''
    }
  },
  { immediate: true }
)

const close = () => emit('update:open', false)

const updateInstanceName = async () => {
  if (!form.displayName) {
    toast.warning('请输入实例名称')
    return
  }
  updating.value = true
  try {
    await instanceApi.updateName({
      userId: props.userId,
      instanceId: props.instance?.id,
      displayName: form.displayName
    })
    toast.success('实例名称更新成功')
    emit('refresh')
  } catch (error: any) {
    toast.error(error.message || '更新失败')
  } finally {
    updating.value = false
  }
}

const updateInstanceConfig = async () => {
  updating.value = true
  try {
    await instanceApi.updateConfig({
      userId: props.userId,
      instanceId: props.instance?.id,
      ocpus: form.ocpus,
      memoryInGBs: form.memoryInGBs
    })
    toast.success('实例配置更新成功')
    emit('refresh')
  } catch (error: any) {
    toast.error(error.message || '更新失败')
  } finally {
    updating.value = false
  }
}

const updateBootVolume = async () => {
  updating.value = true
  try {
    await instanceApi.updateBootVolume({
      userId: props.userId,
      instanceId: props.instance?.id,
      sizeInGBs: form.bootVolumeSize,
      vpusPerGB: form.vpusPerGB
    })
    toast.success('引导卷配置更新成功')
    emit('refresh')
  } catch (error: any) {
    toast.error(error.message || '更新失败')
  } finally {
    updating.value = false
  }
}

const attachIPv6 = async () => {
  if (form.currentIpv6) {
    toast.warning('该实例已有IPv6地址')
    return
  }
  if (!confirm('确定要为该实例附加IPv6地址吗？')) return
  updating.value = true
  try {
    const response = await instanceApi.attachIPv6({
      userId: props.userId,
      instanceId: props.instance?.id
    })
    if (response.data?.ipv6) {
      form.currentIpv6 = response.data.ipv6
      toast.success(`IPv6附加成功: ${response.data.ipv6}`)
    } else {
      toast.success('IPv6附加成功')
    }
    emit('refresh')
  } catch (error: any) {
    toast.error(error.message || '附加失败')
  } finally {
    updating.value = false
  }
}

const startAutoRescue = async () => {
  if (!confirm('自动救援将会关闭实例，备份引导卷，创建47GB新引导卷并重新启动。此操作需要5-10分钟，确定继续吗？')) return
  updating.value = true
  try {
    await instanceApi.autoRescue({
      userId: props.userId,
      instanceId: props.instance?.id,
      instanceName: form.displayName,
      keepBackup: form.keepBackup
    })
    toast.success('自动救援任务已启动，请等待5-10分钟完成')
    close()
  } catch (error: any) {
    toast.error(error.message || '启动失败')
  } finally {
    updating.value = false
  }
}

const enable500Mbps = async () => {
  if (!confirm('此操作将创建NAT网关和网络负载均衡器来实现500Mbps下行带宽。仅AMD E2.1.Micro实例支持，确定继续吗？'))
    return
  updating.value = true
  try {
    await instanceApi.enable500Mbps({
      userId: props.userId,
      instanceId: props.instance?.id,
      sshPort: form.sshPort || 22
    })
    toast.success('500Mbps开启任务已启动')
    close()
  } catch (error: any) {
    toast.error(error.message || '开启失败')
  } finally {
    updating.value = false
  }
}

const disable500Mbps = async () => {
  if (!confirm('确定要关闭500Mbps下行带宽吗？')) return
  updating.value = true
  try {
    await instanceApi.disable500Mbps({
      userId: props.userId,
      instanceId: props.instance?.id,
      retainNatGw: form.retainNatGw,
      retainNlb: form.retainNlb
    })
    toast.success('500Mbps关闭任务已启动')
    close()
  } catch (error: any) {
    toast.error(error.message || '关闭失败')
  } finally {
    updating.value = false
  }
}
</script>

<template>
  <Modal
    :open="open"
    title="编辑实例配置"
    max-width="max-w-2xl"
    panel-class="max-h-[90vh] flex flex-col"
    @update:open="emit('update:open', $event)"
  >
    <div class="flex-1 overflow-y-auto p-6 space-y-4">
      <!-- 实例名称 -->
      <Card class="p-4 bg-muted/30">
        <h4 class="text-sm font-semibold mb-3 flex items-center gap-2">
          <Settings class="w-4 h-4 text-primary" />
          修改实例名称
        </h4>
        <div class="flex gap-3">
          <Input v-model="form.displayName" placeholder="请输入实例名称" class="flex-1" />
          <Button :disabled="updating" @click="updateInstanceName">
            <Loader2 v-if="updating" class="w-4 h-4 animate-spin" />
            修改名称
          </Button>
        </div>
      </Card>

      <!-- CPU和内存 -->
      <Card class="p-4 bg-muted/30">
        <h4 class="text-sm font-semibold mb-3 flex items-center gap-2">
          <Settings class="w-4 h-4 text-primary" />
          修改CPU和内存
        </h4>
        <div
          class="bg-warning/10 border border-warning/30 rounded-lg p-3 mb-3 text-xs text-warning flex items-center gap-2"
        >
          <AlertTriangle class="w-4 h-4 shrink-0" />
          修改CPU和内存需要停止实例
        </div>
        <div class="grid grid-cols-2 gap-3 mb-3">
          <div>
            <label class="block text-xs text-muted-foreground mb-1">CPU核心数 (OCPUs)</label>
            <Input v-model.number="form.ocpus" type="number" min="1" max="64" />
          </div>
          <div>
            <label class="block text-xs text-muted-foreground mb-1">内存 (GB)</label>
            <Input v-model.number="form.memoryInGBs" type="number" min="1" max="1024" />
          </div>
        </div>
        <Button class="w-full" :disabled="updating" @click="updateInstanceConfig">
          <Loader2 v-if="updating" class="w-4 h-4 animate-spin" />
          修改CPU和内存
        </Button>
      </Card>

      <!-- 引导卷配置 -->
      <Card class="p-4 bg-muted/30">
        <h4 class="text-sm font-semibold mb-3 flex items-center gap-2">
          <HardDrive class="w-4 h-4 text-primary" />
          修改引导卷大小及VPU
        </h4>
        <div class="grid grid-cols-2 gap-3 mb-3">
          <div>
            <label class="block text-xs text-muted-foreground mb-1">引导卷大小 (GB)</label>
            <Input v-model.number="form.bootVolumeSize" type="number" min="50" max="32768" />
          </div>
          <div>
            <label class="block text-xs text-muted-foreground mb-1">VPU/GB (性能)</label>
            <select
              v-model.number="form.vpusPerGB"
              class="w-full h-10 px-3 rounded-md border border-input bg-background text-sm"
            >
              <option v-for="v in [10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120]" :key="v" :value="v">
                {{ v }} VPU
              </option>
            </select>
          </div>
        </div>
        <Button class="w-full" :disabled="updating" @click="updateBootVolume">
          <Loader2 v-if="updating" class="w-4 h-4 animate-spin" />
          修改引导卷配置
        </Button>
      </Card>

      <!-- IPv6 -->
      <Card class="p-4 bg-muted/30">
        <h4 class="text-sm font-semibold mb-3 flex items-center gap-2">
          <Globe class="w-4 h-4 text-primary" />
          附加IPv6地址
        </h4>
        <div class="flex items-center justify-between">
          <div class="text-sm text-muted-foreground">
            <span v-if="form.currentIpv6">
              当前IPv6:
              <span class="text-primary font-mono">{{ form.currentIpv6 }}</span>
            </span>
            <span v-else>该实例未分配IPv6地址</span>
          </div>
          <Button :disabled="updating || !!form.currentIpv6" @click="attachIPv6">
            <Loader2 v-if="updating" class="w-4 h-4 animate-spin" />
            {{ form.currentIpv6 ? '已有IPv6' : '附加IPv6' }}
          </Button>
        </div>
      </Card>

      <!-- 自动救援 -->
      <Card class="p-4 bg-muted/30">
        <h4 class="text-sm font-semibold mb-3 flex items-center gap-2">
          <Wrench class="w-4 h-4 text-primary" />
          自动救援/缩小硬盘
        </h4>
        <div class="bg-warning/10 border border-warning/30 rounded-lg p-3 mb-3 text-xs text-warning">
          此操作会关闭实例，备份引导卷，创建47GB新引导卷并重新启动。过程需要5-10分钟。
        </div>
        <div class="flex items-center justify-between">
          <label class="flex items-center gap-2 text-sm cursor-pointer">
            <Checkbox v-model="form.keepBackup" />
            <span>保留原引导卷备份</span>
          </label>
          <Button variant="warning" :disabled="updating" @click="startAutoRescue">
            <Loader2 v-if="updating" class="w-4 h-4 animate-spin" />
            开始救援
          </Button>
        </div>
      </Card>

      <!-- 500Mbps -->
      <Card class="p-4 bg-muted/30">
        <h4 class="text-sm font-semibold mb-3 flex items-center gap-2">
          <Zap class="w-4 h-4 text-primary" />
          500Mbps下行带宽 (仅AMD实例)
        </h4>
        <div class="bg-info/10 border border-info/30 rounded-lg p-3 mb-3 text-xs text-info">
          通过NAT网关和网络负载均衡器实现500Mbps下行带宽。仅支持AMD E2.1.Micro实例。
        </div>
        <div class="flex items-center gap-2 mb-3">
          <label class="text-sm">SSH端口:</label>
          <Input v-model.number="form.sshPort" type="number" min="1" max="65535" class="w-24" placeholder="22" />
        </div>
        <div class="flex gap-2 mb-3">
          <Button variant="success" class="flex-1" :disabled="updating" @click="enable500Mbps">
            <Loader2 v-if="updating" class="w-4 h-4 animate-spin" />
            开启500Mbps
          </Button>
          <Button variant="destructive" class="flex-1" :disabled="updating" @click="disable500Mbps">
            <Loader2 v-if="updating" class="w-4 h-4 animate-spin" />
            关闭500Mbps
          </Button>
        </div>
        <div class="flex items-center gap-4 text-xs text-muted-foreground">
          <label class="flex items-center gap-1 cursor-pointer">
            <Checkbox v-model="form.retainNatGw" />
            <span>保留NAT网关</span>
          </label>
          <label class="flex items-center gap-1 cursor-pointer">
            <Checkbox v-model="form.retainNlb" />
            <span>保留负载均衡器</span>
          </label>
        </div>
      </Card>
    </div>

    <div class="p-6 border-t border-border">
      <Button variant="outline" class="w-full" @click="close">关闭</Button>
    </div>
  </Modal>
</template>
