<script setup lang="ts">
import { ref, reactive, watch } from 'vue'
import { Loader2, HardDrive, AlertTriangle, Trash2 } from 'lucide-vue-next'
import { bootVolumeApi } from '@/api'
import { toast } from '@/composables/useToast'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Modal } from '@/components/ui/modal'

interface Volume {
  id: string
  displayName: string
  instanceId?: string
  instanceName?: string
  sizeInGBs: number
  vpusPerGB?: number
}

const props = defineProps<{
  open: boolean
  volume: Volume | null
  userId: string
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  refresh: []
}>()

const updating = ref(false)
const deleting = ref(false)

const form = reactive({
  volumeId: '',
  displayName: '',
  instanceName: '',
  sizeInGBs: 50,
  vpusPerGB: 10,
  originalSize: 50,
  originalVpu: 10
})

watch(
  () => props.volume,
  volume => {
    if (volume) {
      form.volumeId = volume.id
      form.displayName = volume.displayName
      form.instanceName = volume.instanceName || ''
      form.sizeInGBs = volume.sizeInGBs || 50
      form.vpusPerGB = volume.vpusPerGB || 10
      form.originalSize = volume.sizeInGBs || 50
      form.originalVpu = volume.vpusPerGB || 10
    }
  },
  { immediate: true }
)

const close = () => emit('update:open', false)

const updateVolumeConfig = async () => {
  if (!form.volumeId) {
    toast.warning('缺少引导卷ID')
    return
  }
  if (form.sizeInGBs < form.originalSize) {
    toast.warning('磁盘大小只能增大，不能缩小')
    return
  }
  if (!confirm('确定要修改引导卷配置吗？修改可能需要重启实例才能生效。')) return

  updating.value = true
  try {
    await bootVolumeApi.update({
      userId: props.userId,
      bootVolumeId: form.volumeId,
      sizeInGBs: form.sizeInGBs,
      vpusPerGB: form.vpusPerGB
    })
    toast.success('引导卷配置更新成功')
    emit('refresh')
    close()
  } catch (error: any) {
    toast.error(error.message || '更新失败')
  } finally {
    updating.value = false
  }
}

const deleteVolume = async () => {
  if (!form.volumeId) {
    toast.warning('缺少引导卷ID')
    return
  }
  if (!confirm(`确定要删除引导卷 "${form.displayName}" 吗？此操作不可恢复！`)) return

  deleting.value = true
  try {
    await bootVolumeApi.delete({
      userId: props.userId,
      bootVolumeId: form.volumeId
    })
    toast.success('引导卷删除成功')
    emit('refresh')
    close()
  } catch (error: any) {
    toast.error(error.message || '删除失败')
  } finally {
    deleting.value = false
  }
}
</script>

<template>
  <Modal :open="open" title="编辑引导卷" @update:open="emit('update:open', $event)">
    <div class="p-6 space-y-4">
      <!-- 引导卷信息 -->
      <div class="bg-muted/30 rounded-lg p-4">
        <div class="flex items-center gap-3 mb-3">
          <div class="w-10 h-10 rounded-lg bg-primary/10 flex items-center justify-center">
            <HardDrive class="w-5 h-5 text-primary" />
          </div>
          <div>
            <h5 class="font-semibold">{{ form.displayName }}</h5>
            <p class="text-xs text-muted-foreground font-mono">{{ form.volumeId?.substring(0, 40) }}...</p>
          </div>
        </div>
        <div v-if="form.instanceName" class="text-sm">
          <span class="text-muted-foreground">附加实例:</span>
          <span class="ml-2 text-primary">{{ form.instanceName }}</span>
        </div>
      </div>

      <!-- 磁盘大小 -->
      <div>
        <label class="block text-sm font-medium mb-2">磁盘大小 (GB)</label>
        <div class="flex items-center gap-3">
          <Input
            v-model.number="form.sizeInGBs"
            type="number"
            min="50"
            max="32768"
            class="flex-1"
            placeholder="输入磁盘大小"
          />
          <span class="text-muted-foreground text-sm">当前: {{ form.originalSize }} GB</span>
        </div>
        <p class="text-xs text-muted-foreground mt-1">最小 50GB，最大 32768GB。只能增大，不能缩小。</p>
      </div>

      <!-- VPU性能 -->
      <div>
        <label class="block text-sm font-medium mb-2">性能 (VPU/GB)</label>
        <select
          v-model.number="form.vpusPerGB"
          class="w-full h-10 px-3 rounded-md border border-input bg-background text-sm"
        >
          <option :value="10">10 VPU/GB - 平衡性能</option>
          <option :value="20">20 VPU/GB - 高性能</option>
          <option :value="30">30 VPU/GB - 更高性能</option>
          <option :value="40">40 VPU/GB - 高级性能</option>
          <option :value="50">50 VPU/GB - 极高性能</option>
          <option :value="60">60 VPU/GB - 超高性能</option>
          <option :value="70">70 VPU/GB - 顶级性能</option>
          <option :value="80">80 VPU/GB - 旗舰性能</option>
          <option :value="90">90 VPU/GB - 极致性能</option>
          <option :value="100">100 VPU/GB - 最高性能</option>
          <option :value="110">110 VPU/GB - 超旗舰</option>
          <option :value="120">120 VPU/GB - 极限性能</option>
        </select>
        <p class="text-xs text-muted-foreground mt-1">当前: {{ form.originalVpu }} VPU/GB</p>
      </div>

      <!-- 警告提示 -->
      <div class="bg-warning/10 border border-warning/30 rounded-lg p-3 flex items-start gap-2">
        <AlertTriangle class="w-4 h-4 text-warning shrink-0 mt-0.5" />
        <p class="text-xs text-warning">修改引导卷配置可能需要重启实例才能生效。磁盘大小只能增大，不能缩小。</p>
      </div>
    </div>

    <div class="p-6 border-t border-border space-y-3">
      <div class="flex gap-3">
        <Button variant="outline" class="flex-1" @click="close">取消</Button>
        <Button class="flex-1" :disabled="updating" @click="updateVolumeConfig">
          <Loader2 v-if="updating" class="w-4 h-4 animate-spin" />
          {{ updating ? '更新中...' : '保存修改' }}
        </Button>
      </div>
      <Button variant="destructive" class="w-full" :disabled="deleting" @click="deleteVolume">
        <Loader2 v-if="deleting" class="w-4 h-4 animate-spin" />
        <Trash2 v-else class="w-4 h-4" />
        {{ deleting ? '删除中...' : '删除引导卷' }}
      </Button>
    </div>
  </Modal>
</template>
