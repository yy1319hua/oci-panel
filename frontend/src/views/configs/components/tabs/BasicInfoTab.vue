<script setup lang="ts">
import { ref } from 'vue'
import { Loader2, Settings, Copy, Edit, Check, X } from 'lucide-vue-next'
import { ociApi } from '@/api'
import { toast } from '@/composables/useToast'
import { Card } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

// 配置详情「基本信息」标签页——配置与租户信息卡片（C7 从 Configs.vue 抽出）。
// 密码过期时间编辑为该卡片的局部交互，故连同其 API 调用一并下沉到本组件；
// 成功后就地更新传入的 tenant 对象（与原内联实现共享同一响应式对象）。
const props = defineProps<{
  configDetails: any
  tenant: any
  loading: boolean
}>()

const editingPasswordExpiry = ref(false)
const passwordExpiryInput = ref(0)
const updatingPasswordExpiry = ref(false)

const copyToClipboard = (text: string) => {
  navigator.clipboard
    .writeText(text)
    .then(() => toast.success('已复制到剪贴板'))
    .catch(() => toast.error('复制失败'))
}

const startEditPasswordExpiry = () => {
  passwordExpiryInput.value = props.tenant?.passwordExpiresAfter || 0
  editingPasswordExpiry.value = true
}
const cancelEditPasswordExpiry = () => {
  editingPasswordExpiry.value = false
}
const savePasswordExpiry = async () => {
  updatingPasswordExpiry.value = true
  try {
    await ociApi.updatePwdEx({
      cfgId: props.configDetails?.userId,
      passwordExpiresAfter: passwordExpiryInput.value
    })
    toast.success('密码过期时间更新成功')
    if (props.tenant) props.tenant.passwordExpiresAfter = passwordExpiryInput.value
    editingPasswordExpiry.value = false
  } catch (error: any) {
    toast.error(error.message || '更新失败')
  } finally {
    updatingPasswordExpiry.value = false
  }
}
</script>

<template>
  <Card v-if="loading" class="p-8">
    <div class="flex items-center justify-center">
      <Loader2 class="w-8 h-8 animate-spin text-primary" />
    </div>
  </Card>
  <Card v-else class="p-6">
    <h3 class="text-lg font-semibold mb-4 flex items-center gap-2">
      <Settings class="w-5 h-5 text-primary" />
      配置与租户信息
    </h3>
    <div class="grid grid-cols-1 md:grid-cols-2 gap-6 text-sm">
      <div class="space-y-1">
        <label class="text-muted-foreground text-xs uppercase tracking-wide">配置名称</label>
        <p class="font-medium text-lg">{{ configDetails.username }}</p>
      </div>
      <div class="space-y-1">
        <label class="text-muted-foreground text-xs uppercase tracking-wide">当前区域</label>
        <p>
          <Badge variant="info" class="text-sm">{{ configDetails.region }}</Badge>
        </p>
      </div>
      <div v-if="tenant" class="space-y-1 md:col-span-2">
        <label class="text-muted-foreground text-xs uppercase tracking-wide">租户名称</label>
        <p class="font-medium text-lg">{{ tenant.name }}</p>
      </div>
      <div v-if="tenant" class="space-y-1 md:col-span-2">
        <label class="text-muted-foreground text-xs uppercase tracking-wide">租户ID</label>
        <div class="flex items-center gap-2 bg-muted/50 p-3 rounded-lg">
          <code class="text-xs font-mono flex-1 break-all">{{ tenant.id }}</code>
          <Button variant="ghost" size="icon" class="h-8 w-8 shrink-0" @click="copyToClipboard(tenant.id)">
            <Copy class="w-4 h-4" />
          </Button>
        </div>
      </div>
      <div v-if="tenant" class="space-y-1">
        <label class="text-muted-foreground text-xs uppercase tracking-wide">主区域</label>
        <p class="font-medium">{{ tenant.homeRegionKey }}</p>
      </div>
      <div v-if="tenant?.createTime" class="space-y-1">
        <label class="text-muted-foreground text-xs uppercase tracking-wide">账户创建时间</label>
        <p>{{ tenant.createTime }}</p>
      </div>
      <div class="space-y-1 md:col-span-2">
        <label class="text-muted-foreground text-xs uppercase tracking-wide">用户ID</label>
        <div class="bg-muted/50 p-3 rounded-lg">
          <code class="text-xs font-mono break-all">{{ configDetails.userId }}</code>
        </div>
      </div>
      <div class="space-y-1">
        <label class="text-muted-foreground text-xs uppercase tracking-wide">指纹</label>
        <div class="bg-muted/50 p-3 rounded-lg">
          <code class="text-xs font-mono break-all">{{ configDetails.fingerprint }}</code>
        </div>
      </div>
      <div class="space-y-1">
        <label class="text-muted-foreground text-xs uppercase tracking-wide">密钥文件</label>
        <p class="text-sm">{{ configDetails.keyPath }}</p>
      </div>
    </div>
    <!-- 密码过期时间设置 -->
    <div v-if="tenant" class="mt-6 pt-6 border-t border-border">
      <label class="text-muted-foreground text-xs uppercase tracking-wide block mb-2">密码过期时间</label>
      <div class="flex items-center gap-2">
        <Input
          v-if="editingPasswordExpiry"
          v-model.number="passwordExpiryInput"
          type="number"
          min="0"
          class="w-32"
          placeholder="0"
        />
        <span v-else class="font-medium">
          {{ tenant.passwordExpiresAfter === 0 ? '永不过期' : tenant.passwordExpiresAfter + ' 天' }}
        </span>
        <Button
          v-if="!editingPasswordExpiry"
          variant="ghost"
          size="icon"
          class="h-8 w-8"
          @click="startEditPasswordExpiry"
        >
          <Edit class="w-4 h-4" />
        </Button>
        <div v-else class="flex gap-1">
          <Button
            variant="ghost"
            size="icon"
            class="h-8 w-8 text-success"
            :disabled="updatingPasswordExpiry"
            @click="savePasswordExpiry"
          >
            <Check class="w-4 h-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            class="h-8 w-8 text-destructive"
            :disabled="updatingPasswordExpiry"
            @click="cancelEditPasswordExpiry"
          >
            <X class="w-4 h-4" />
          </Button>
        </div>
      </div>
      <p class="text-xs text-muted-foreground mt-1">设置为 0 表示永不过期</p>
    </div>
    <div v-if="tenant?.regions?.length" class="mt-6 pt-6 border-t border-border">
      <label class="text-muted-foreground text-xs uppercase tracking-wide block mb-3">
        订阅区域 ({{ tenant.regions.length }})
      </label>
      <div class="flex flex-wrap gap-2">
        <Badge v-for="region in tenant.regions" :key="region" variant="secondary">{{ region }}</Badge>
      </div>
    </div>
  </Card>
</template>
