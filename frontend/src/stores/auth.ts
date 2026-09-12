import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { sysApi } from '@/api/sys'

export interface User {
  username: string
  account: string
}

export interface LoginResult {
  needMfa: boolean
  needPasskey: boolean
  passkeyEnabled: boolean
  mfaTicket?: string
}

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string>(localStorage.getItem('token') || '')
  const user = ref<User | null>(JSON.parse(localStorage.getItem('user') || 'null'))
  const pendingAccount = ref<string>('')
  const pendingMfaTicket = ref<string>('')

  const isAuthenticated = computed(() => !!token.value)

  async function login(account: string, password: string): Promise<LoginResult> {
    const response = await sysApi.login(account, password)

    if (response.data.needMfa || response.data.needPasskey) {
      pendingAccount.value = account
      pendingMfaTicket.value = response.data.mfaTicket || ''
      return {
        needMfa: response.data.needMfa || false,
        needPasskey: response.data.needPasskey || false,
        passkeyEnabled: response.data.passkeyEnabled || false,
        mfaTicket: response.data.mfaTicket
      }
    }

    pendingMfaTicket.value = ''
    token.value = response.data.token || ''
    user.value = {
      username: response.data.username || '',
      account: account
    }

    localStorage.setItem('token', token.value)
    localStorage.setItem('user', JSON.stringify(user.value))

    return { needMfa: false, needPasskey: false, passkeyEnabled: false }
  }

  function setToken(newToken: string, username: string): void {
    token.value = newToken
    user.value = {
      username: username,
      account: pendingAccount.value || username
    }
    localStorage.setItem('token', token.value)
    localStorage.setItem('user', JSON.stringify(user.value))
    pendingAccount.value = ''
    pendingMfaTicket.value = ''
  }

  async function verifyMfa(code: string): Promise<void> {
    if (!pendingMfaTicket.value) {
      throw new Error('MFA 登录流程已失效，请重新登录')
    }
    const response = await sysApi.checkMfaCode(pendingMfaTicket.value, code)

    token.value = response.data.token
    user.value = {
      username: response.data.username,
      account: pendingAccount.value
    }

    localStorage.setItem('token', token.value)
    localStorage.setItem('user', JSON.stringify(user.value))
    pendingAccount.value = ''
    pendingMfaTicket.value = ''
  }

  function clearPendingVerification() {
    pendingAccount.value = ''
    pendingMfaTicket.value = ''
  }

  // applyNewToken 在后端改了账号并重签 JWT 后，用新令牌刷新会话（无感重登）。
  function applyNewToken(newToken: string, account: string): void {
    token.value = newToken
    user.value = {
      username: account,
      account: account
    }
    localStorage.setItem('token', newToken)
    localStorage.setItem('user', JSON.stringify(user.value))
  }

  function logout() {
    token.value = ''
    user.value = null
    clearPendingVerification()
    localStorage.removeItem('token')
    localStorage.removeItem('user')
  }

  function checkAuth() {
    if (!token.value) {
      logout()
    }
  }

  return {
    token,
    user,
    isAuthenticated,
    login,
    setToken,
    verifyMfa,
    clearPendingVerification,
    applyNewToken,
    logout,
    checkAuth
  }
})
