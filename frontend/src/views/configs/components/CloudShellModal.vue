<script setup lang="ts">
import { ref, reactive, watch } from 'vue'
import { Loader2, Terminal, Copy, Check } from 'lucide-vue-next'
import { instanceApi } from '@/api'
import { toast } from '@/composables/useToast'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Input } from '@/components/ui/input'
import { Modal } from '@/components/ui/modal'

const props = defineProps<{
  open: boolean
  instanceId: string
  userId: string
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

const creating = ref(false)
const publicKey = ref('')
const result = reactive({
  connectionId: '',
  connectionString: ''
})

watch(
  () => props.open,
  open => {
    if (open) {
      publicKey.value = ''
      result.connectionId = ''
      result.connectionString = ''
    }
  }
)

const close = () => emit('update:open', false)

const copyToClipboard = (text: string) => {
  navigator.clipboard
    .writeText(text)
    .then(() => {
      toast.success('已复制到剪贴板')
    })
    .catch(() => {
      toast.error('复制失败')
    })
}

const createCloudShell = async () => {
  if (!publicKey.value.trim()) {
    toast.warning('请输入SSH公钥')
    return
  }
  creating.value = true
  try {
    const response = await instanceApi.createCloudShell({
      userId: props.userId,
      instanceId: props.instanceId,
      publicKey: publicKey.value
    })
    if (response.data) {
      result.connectionId = response.data.connectionId || ''
      result.connectionString = response.data.connectionString || ''
      toast.success('Cloud Shell连接创建成功')
    }
  } catch (error: any) {
    toast.error(error.message || '创建失败')
  } finally {
    creating.value = false
  }
}
</script>

<template>
  <Modal :open="open" max-width="max-w-2xl" @update:open="emit('update:open', $event)">
    <template #title>
      <Terminal class="w-5 h-5 text-primary" />
      Cloud Shell 连接
    </template>

    <div class="p-6 space-y-4">
      <div class="bg-info/10 border border-info/30 rounded-lg p-4 text-sm text-info">
        请提供SSH公钥以创建Cloud Shell连接
      </div>

      <div v-if="!result.connectionString">
        <label class="block text-sm font-medium mb-2">SSH 公钥</label>
        <Textarea
          v-model="publicKey"
          :rows="5"
          class="font-mono text-xs"
          placeholder="ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC..."
        />
        <p class="text-xs text-muted-foreground mt-2">粘贴您的 SSH 公钥（通常位于 ~/.ssh/id_rsa.pub）</p>
      </div>

      <div v-else class="space-y-4">
        <div>
          <label class="block text-sm font-medium mb-2">连接ID</label>
          <div class="flex gap-2">
            <Input :model-value="result.connectionId" readonly class="flex-1 font-mono text-xs bg-muted/50" />
            <Button variant="outline" size="icon" @click="copyToClipboard(result.connectionId)">
              <Copy class="w-4 h-4" />
            </Button>
          </div>
        </div>

        <div>
          <label class="block text-sm font-medium mb-2">连接字符串</label>
          <div class="flex gap-2">
            <Input :model-value="result.connectionString" readonly class="flex-1 font-mono text-xs bg-muted/50" />
            <Button variant="outline" size="icon" @click="copyToClipboard(result.connectionString)">
              <Copy class="w-4 h-4" />
            </Button>
          </div>
        </div>

        <div class="bg-success/10 border border-success/30 rounded-lg p-4 text-sm text-success flex items-center gap-2">
          <Check class="w-5 h-5" />
          连接创建成功！请使用SSH客户端连接。
        </div>
      </div>
    </div>

    <div class="p-6 border-t border-border flex gap-3">
      <Button variant="outline" class="flex-1" @click="close">关闭</Button>
      <Button
        v-if="!result.connectionString"
        class="flex-1"
        :disabled="creating || !publicKey.trim()"
        @click="createCloudShell"
      >
        <Loader2 v-if="creating" class="w-4 h-4 animate-spin" />
        {{ creating ? '创建中...' : '创建连接' }}
      </Button>
    </div>
  </Modal>
</template>
