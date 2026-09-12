<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter, RouterLink } from 'vue-router'
import { useMotion } from '@vueuse/motion'
import { Cloud, Lock, User, ArrowRight, Loader2, Shield, Fingerprint } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { toast } from '@/composables/useToast'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardContent } from '@/components/ui/card'
import { Dialog, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { passkeyApi, sysApi } from '@/api'
import { getPasskeyCredential } from '@/lib/webauthn'

const router = useRouter()
const authStore = useAuthStore()

const form = ref({
  account: '',
  password: ''
})

const loading = ref(false)
const error = ref('')
const needMfa = ref(false)
const needPasskey = ref(false)
const passkeyEnabled = ref(false)
const mfaCode = ref('')
const verifyingMfa = ref(false)
const verifyingPasskey = ref(false)

const loginMode = ref<'passkey' | 'password'>('password')
const checkingPasskey = ref(true)

const cardRef = ref<HTMLElement>()

useMotion(cardRef, {
  initial: { opacity: 0, y: 50, scale: 0.95 },
  enter: {
    opacity: 1,
    y: 0,
    scale: 1,
    transition: {
      duration: 600,
      ease: [0.16, 1, 0.3, 1]
    }
  }
})

onMounted(async () => {
  try {
    const res = await passkeyApi.status()
    if (res.data?.enabled) {
      loginMode.value = 'passkey'
      passkeyEnabled.value = true
    }
  } catch {
    // passkey status check failed, default to password
  } finally {
    checkingPasskey.value = false
  }
})

const handleLogin = async () => {
  error.value = ''
  loading.value = true

  try {
    const result = await authStore.login(form.value.account, form.value.password)
    if (result.needMfa || result.needPasskey) {
      needMfa.value = result.needMfa
      needPasskey.value = result.needPasskey
      passkeyEnabled.value = result.passkeyEnabled
      loading.value = false
      return
    }
    toast.success('登录成功')
    router.push('/')
  } catch (err: any) {
    error.value = err.message || '登录失败，请检查账号密码'
    toast.error(error.value)
  } finally {
    loading.value = false
  }
}

const handleMfaVerify = async () => {
  if (!mfaCode.value || mfaCode.value.length !== 6) {
    error.value = '请输入6位验证码'
    return
  }
  error.value = ''
  verifyingMfa.value = true

  try {
    await authStore.verifyMfa(mfaCode.value)
    toast.success('登录成功')
    router.push('/')
  } catch (err: any) {
    error.value = err.message || '验证码错误'
    toast.error(error.value)
  } finally {
    verifyingMfa.value = false
  }
}

const handlePasskeyLogin = async () => {
  error.value = ''
  verifyingPasskey.value = true

  try {
    const beginResponse = await passkeyApi.beginLogin()
    const credentialData = await getPasskeyCredential(beginResponse.data.publicKey)
    const finishResponse = await passkeyApi.finishLogin(credentialData)
    authStore.setToken(finishResponse.data.token, finishResponse.data.username)
    toast.success('登录成功')
    router.push('/')
  } catch (err: any) {
    if (err.name === 'NotAllowedError') {
      error.value = '用户取消了操作'
    } else {
      error.value = err.message || 'Passkey 验证失败'
    }
    toast.error(error.value)
  } finally {
    verifyingPasskey.value = false
  }
}

const switchToPassword = () => {
  loginMode.value = 'password'
  error.value = ''
}

const switchToPasskey = () => {
  loginMode.value = 'passkey'
  error.value = ''
}

const backToLogin = () => {
  needMfa.value = false
  needPasskey.value = false
  mfaCode.value = ''
  error.value = ''
  authStore.clearPendingVerification()
}

// 忘记密码：发送重置邮件
const showForgot = ref(false)
const forgotEmail = ref('')
const sendingReset = ref(false)
const resetSent = ref(false)

const openForgot = () => {
  showForgot.value = true
  forgotEmail.value = ''
  resetSent.value = false
  error.value = ''
}

const sendReset = async () => {
  if (!forgotEmail.value) {
    error.value = '请输入邮箱'
    return
  }
  sendingReset.value = true
  error.value = ''
  try {
    await sysApi.requestPasswordReset(forgotEmail.value)
    resetSent.value = true
    toast.success('若该邮箱已绑定账号，重置邮件已发送')
  } catch (err: any) {
    error.value = err.message || '发送失败'
  } finally {
    sendingReset.value = false
  }
}
</script>

<template>
  <div class="min-h-screen flex items-center justify-center p-4 relative overflow-hidden">
    <!-- Animated Background -->
    <div class="absolute inset-0 mesh-gradient" />
    <div class="absolute inset-0 grid-pattern opacity-30" />
    <div class="absolute inset-0 noise-overlay" />

    <!-- Floating Elements -->
    <div class="absolute top-1/4 left-1/4 w-64 h-64 bg-primary/10 rounded-full blur-3xl animate-pulse" />
    <div class="absolute bottom-1/4 right-1/4 w-96 h-96 bg-cyan-500/5 rounded-full blur-3xl animate-pulse delay-1000" />

    <!-- Login Card -->
    <Card ref="cardRef" class="relative z-10 w-full max-w-md glass border-border/50">
      <CardContent class="p-6 sm:p-8">
        <!-- Logo -->
        <div class="text-center mb-8">
          <div
            v-motion
            :initial="{ scale: 0, rotate: -180 }"
            :enter="{ scale: 1, rotate: 0, transition: { delay: 200, duration: 500 } }"
            class="inline-flex items-center justify-center w-16 h-16 bg-gradient-to-br from-primary to-cyan-400 rounded-2xl mb-4 shadow-lg"
          >
            <Cloud class="w-8 h-8 text-white" />
          </div>
          <h1
            v-motion
            :initial="{ opacity: 0, y: 20 }"
            :enter="{ opacity: 1, y: 0, transition: { delay: 300 } }"
            class="text-3xl font-display font-bold text-gradient"
          >
            OCI Panel
          </h1>
          <p
            v-motion
            :initial="{ opacity: 0 }"
            :enter="{ opacity: 1, transition: { delay: 400 } }"
            class="text-muted-foreground mt-2"
          >
            Oracle Cloud Infrastructure 管理面板
          </p>
        </div>

        <!-- Loading passkey status -->
        <div v-if="checkingPasskey" class="flex justify-center py-8">
          <Loader2 class="w-6 h-6 animate-spin text-muted-foreground" />
        </div>

        <!-- Main login area (not in MFA/Passkey verification flow) -->
        <template v-else-if="!needMfa && !needPasskey">
          <!-- Passkey Login -->
          <div v-if="loginMode === 'passkey'" class="space-y-6">
            <div class="text-center mb-4">
              <div
                v-motion
                :initial="{ scale: 0 }"
                :enter="{ scale: 1, transition: { delay: 300, duration: 400 } }"
                class="inline-flex items-center justify-center w-14 h-14 bg-primary/10 rounded-full mb-3"
              >
                <Fingerprint class="w-7 h-7 text-primary" />
              </div>
              <p
                v-motion
                :initial="{ opacity: 0 }"
                :enter="{ opacity: 1, transition: { delay: 400 } }"
                class="text-sm text-muted-foreground"
              >
                使用通行密钥快速登录
              </p>
            </div>

            <div v-motion :initial="{ opacity: 0, y: 20 }" :enter="{ opacity: 1, y: 0, transition: { delay: 500 } }">
              <Button
                :disabled="verifyingPasskey"
                class="w-full h-12 text-base font-medium group"
                @click="handlePasskeyLogin"
              >
                <template v-if="!verifyingPasskey">
                  <Fingerprint class="w-5 h-5 mr-2" />
                  使用通行密钥登录
                </template>
                <template v-else>
                  <Loader2 class="w-4 h-4 mr-2 animate-spin" />
                  验证中...
                </template>
              </Button>
            </div>

            <div
              v-motion
              :initial="{ opacity: 0 }"
              :enter="{ opacity: 1, transition: { delay: 600 } }"
              class="text-center"
            >
              <button
                type="button"
                class="text-sm text-muted-foreground hover:text-foreground transition-colors"
                @click="switchToPassword"
              >
                使用密码登录
              </button>
            </div>
          </div>

          <!-- Password Login -->
          <form v-else class="space-y-6" @submit.prevent="handleLogin">
            <div v-motion :initial="{ opacity: 0, x: -20 }" :enter="{ opacity: 1, x: 0, transition: { delay: 500 } }">
              <label class="block text-sm font-medium mb-2">
                <User class="w-4 h-4 inline mr-2 text-muted-foreground" />
                账号
              </label>
              <Input
                v-model="form.account"
                type="text"
                placeholder="请输入账号"
                required
                autocomplete="username"
                class="h-11 bg-secondary/50 border-border/50 focus:border-primary"
              />
            </div>

            <div v-motion :initial="{ opacity: 0, x: -20 }" :enter="{ opacity: 1, x: 0, transition: { delay: 600 } }">
              <label class="block text-sm font-medium mb-2">
                <Lock class="w-4 h-4 inline mr-2 text-muted-foreground" />
                密码
              </label>
              <Input
                v-model="form.password"
                type="password"
                placeholder="请输入密码"
                required
                autocomplete="current-password"
                class="h-11 bg-secondary/50 border-border/50 focus:border-primary"
              />
            </div>

            <div v-motion :initial="{ opacity: 0, y: 20 }" :enter="{ opacity: 1, y: 0, transition: { delay: 700 } }">
              <Button
                type="submit"
                :disabled="loading"
                :loading="loading"
                class="w-full h-11 text-base font-medium group"
              >
                <template v-if="!loading">
                  登录
                  <ArrowRight class="w-4 h-4 ml-2 transition-transform group-hover:translate-x-1" />
                </template>
                <template v-else>
                  <Loader2 class="w-4 h-4 mr-2 animate-spin" />
                  登录中...
                </template>
              </Button>
            </div>

            <div
              v-motion
              :initial="{ opacity: 0 }"
              :enter="{ opacity: 1, transition: { delay: 800 } }"
              class="flex items-center justify-center gap-4 text-sm"
            >
              <button
                type="button"
                class="text-muted-foreground hover:text-foreground transition-colors"
                @click="openForgot"
              >
                忘记密码？
              </button>
              <span class="text-border">|</span>
              <RouterLink to="/docs" class="text-muted-foreground hover:text-foreground transition-colors">
                API 文档
              </RouterLink>
            </div>

            <div
              v-if="passkeyEnabled"
              v-motion
              :initial="{ opacity: 0 }"
              :enter="{ opacity: 1, transition: { delay: 850 } }"
              class="text-center"
            >
              <button
                type="button"
                class="text-sm text-muted-foreground hover:text-foreground transition-colors"
                @click="switchToPasskey"
              >
                <Fingerprint class="w-4 h-4 inline mr-1" />
                使用通行密钥登录
              </button>
            </div>
          </form>
        </template>

        <!-- MFA/Passkey Verification (after password login) -->
        <div v-else class="space-y-6">
          <!-- Passkey Option -->
          <div v-if="passkeyEnabled" class="space-y-4">
            <div class="text-center mb-4">
              <div class="inline-flex items-center justify-center w-12 h-12 bg-primary/10 rounded-full mb-3">
                <Fingerprint class="w-6 h-6 text-primary" />
              </div>
              <p class="text-sm text-muted-foreground">使用 Passkey 快速登录</p>
            </div>

            <Button :disabled="verifyingPasskey" class="w-full h-11 text-base font-medium" @click="handlePasskeyLogin">
              <template v-if="!verifyingPasskey">
                <Fingerprint class="w-4 h-4 mr-2" />
                使用 Passkey 登录
              </template>
              <template v-else>
                <Loader2 class="w-4 h-4 mr-2 animate-spin" />
                验证中...
              </template>
            </Button>

            <div v-if="needMfa" class="relative">
              <div class="absolute inset-0 flex items-center">
                <span class="w-full border-t border-border" />
              </div>
              <div class="relative flex justify-center text-xs uppercase">
                <span class="bg-card px-2 text-muted-foreground">或</span>
              </div>
            </div>
          </div>

          <!-- MFA Option -->
          <form v-if="needMfa" class="space-y-4" @submit.prevent="handleMfaVerify">
            <div v-if="!passkeyEnabled" class="text-center mb-4">
              <div class="inline-flex items-center justify-center w-12 h-12 bg-primary/10 rounded-full mb-3">
                <Shield class="w-6 h-6 text-primary" />
              </div>
              <p class="text-sm text-muted-foreground">请输入验证器 App 中的验证码</p>
            </div>

            <div v-motion :initial="{ opacity: 0, y: 20 }" :enter="{ opacity: 1, y: 0 }">
              <Input
                v-model="mfaCode"
                type="text"
                placeholder="输入6位验证码"
                maxlength="6"
                :autofocus="!passkeyEnabled"
                class="h-12 text-center text-xl font-mono tracking-[0.5em] bg-secondary/50 border-border/50 focus:border-primary"
                @keyup.enter="handleMfaVerify"
              />
            </div>

            <Button
              type="submit"
              :disabled="verifyingMfa || mfaCode.length !== 6"
              class="w-full h-11 text-base font-medium"
            >
              <template v-if="!verifyingMfa">
                <Shield class="w-4 h-4 mr-2" />
                验证 MFA
              </template>
              <template v-else>
                <Loader2 class="w-4 h-4 mr-2 animate-spin" />
                验证中...
              </template>
            </Button>
          </form>

          <Button type="button" variant="ghost" class="w-full" @click="backToLogin">返回登录</Button>
        </div>

        <!-- Error Message -->
        <Transition
          enter-active-class="transition-all duration-300"
          leave-active-class="transition-all duration-300"
          enter-from-class="opacity-0 -translate-y-2"
          leave-to-class="opacity-0 -translate-y-2"
        >
          <div
            v-if="error"
            class="mt-4 p-3 bg-destructive/10 border border-destructive/30 rounded-lg text-destructive text-sm"
          >
            {{ error }}
          </div>
        </Transition>

        <!-- Footer -->
        <p
          v-motion
          :initial="{ opacity: 0 }"
          :enter="{ opacity: 1, transition: { delay: 800 } }"
          class="text-center text-xs text-muted-foreground mt-6"
        >
          安全登录 · 数据加密传输
        </p>
      </CardContent>
    </Card>

    <!-- 忘记密码弹窗 -->
    <Dialog v-model:open="showForgot">
      <DialogHeader class="mb-4">
        <DialogTitle>重置密码</DialogTitle>
        <DialogDescription>输入账号绑定的邮箱，我们将发送重置链接。</DialogDescription>
      </DialogHeader>
      <div v-if="!resetSent" class="space-y-4">
        <div>
          <label class="block text-sm font-medium mb-2">邮箱</label>
          <Input
            v-model="forgotEmail"
            type="email"
            placeholder="请输入绑定邮箱"
            @keyup.enter="sendReset"
          />
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" @click="showForgot = false">取消</Button>
          <Button type="button" :disabled="sendingReset" @click="sendReset">
            <Loader2 v-if="sendingReset" class="w-4 h-4 animate-spin" />
            发送重置邮件
          </Button>
        </DialogFooter>
      </div>
      <div v-else class="space-y-4">
        <p class="text-sm text-muted-foreground">
          如果该邮箱已绑定账号，重置链接已发送，请在 30 分钟内点击邮件中的链接设置新密码。
        </p>
        <DialogFooter>
          <Button type="button" @click="showForgot = false">我知道了</Button>
        </DialogFooter>
      </div>
    </Dialog>
  </div>
</template>
