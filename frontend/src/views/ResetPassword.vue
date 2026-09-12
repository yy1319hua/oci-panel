<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute, useRouter, RouterLink } from 'vue-router'
import { Cloud, Lock, Loader2, CheckCircle2 } from 'lucide-vue-next'
import { toast } from '@/composables/useToast'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardContent } from '@/components/ui/card'
import { sysApi } from '@/api'

const route = useRoute()
const router = useRouter()

const token = ref('')
const password = ref('')
const confirm = ref('')
const submitting = ref(false)
const done = ref(false)
const error = ref('')

onMounted(() => {
  token.value = (route.query.token as string) || ''
  if (!token.value) error.value = '链接无效：缺少重置令牌'
})

const handleSubmit = async () => {
  error.value = ''
  if (!token.value) {
    error.value = '链接无效：缺少重置令牌'
    return
  }
  if (password.value.length < 6) {
    error.value = '新密码长度至少 6 位'
    return
  }
  if (password.value !== confirm.value) {
    error.value = '两次输入的密码不一致'
    return
  }
  submitting.value = true
  try {
    await sysApi.resetPassword(token.value, password.value)
    done.value = true
    toast.success('密码重置成功')
  } catch (err: any) {
    error.value = err.message || '重置失败'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="min-h-screen flex items-center justify-center p-4 relative overflow-hidden">
    <Card class="relative z-10 w-full max-w-md glass border-border/50">
      <CardContent class="p-6 sm:p-8">
        <div class="text-center mb-6">
          <div
            class="inline-flex items-center justify-center w-14 h-14 bg-gradient-to-br from-primary to-cyan-400 rounded-2xl mb-3 shadow-lg"
          >
            <Cloud class="w-7 h-7 text-white" />
          </div>
          <h1 class="text-2xl font-display font-bold text-gradient">设置新密码</h1>
        </div>

        <div v-if="done" class="space-y-5 text-center">
          <CheckCircle2 class="w-12 h-12 text-success mx-auto" />
          <p class="text-sm text-muted-foreground">密码重置成功，请使用新密码登录。</p>
          <RouterLink to="/login">
            <Button class="w-full">去登录</Button>
          </RouterLink>
        </div>

        <form v-else class="space-y-5" @submit.prevent="handleSubmit">
          <div>
            <label class="block text-sm font-medium mb-2">
              <Lock class="w-4 h-4 inline mr-2 text-muted-foreground" />
              新密码
            </label>
            <Input v-model="password" type="password" placeholder="至少 6 位" required class="h-11" />
          </div>
          <div>
            <label class="block text-sm font-medium mb-2">
              <Lock class="w-4 h-4 inline mr-2 text-muted-foreground" />
              确认新密码
            </label>
            <Input v-model="confirm" type="password" placeholder="再次输入新密码" required class="h-11" />
          </div>

          <p v-if="error" class="p-3 bg-destructive/10 border border-destructive/30 rounded-lg text-destructive text-sm">
            {{ error }}
          </p>

          <Button type="submit" :disabled="submitting || !token" class="w-full h-11">
            <Loader2 v-if="submitting" class="w-4 h-4 mr-2 animate-spin" />
            重置密码
          </Button>

          <div class="text-center">
            <RouterLink to="/login" class="text-sm text-muted-foreground hover:text-foreground">返回登录</RouterLink>
          </div>
        </form>
      </CardContent>
    </Card>
  </div>
</template>
