<script setup lang="ts">
import { Users, Mail, Edit, KeyRound, LockKeyhole, ShieldOff, Trash2 } from 'lucide-vue-next'
import { Card } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

// 租户「用户管理卡片」（C7 从 Configs.vue 基本信息标签页抽出，纯展示）。
// 用户操作（编辑/重置密码/清除 MFA/清除 API/删除）经 emit 上抛由抽屉处理，
// 抽屉在成功后刷新租户信息——与拆分前一致。
defineProps<{
  users: any[]
}>()

defineEmits<{
  edit: [user: any]
  'reset-password': [user: any]
  'clear-mfa': [user: any]
  'clear-api-keys': [user: any]
  delete: [user: any]
}>()
</script>

<template>
  <Card class="p-6">
    <h3 class="text-lg font-semibold mb-4 flex items-center gap-2">
      <Users class="w-5 h-5 text-primary" />
      用户列表 ({{ users.length }})
    </h3>
    <div class="space-y-4">
      <div
        v-for="user in users"
        :key="user.id"
        class="border border-border rounded-lg p-4 hover:border-primary/50 transition-colors"
      >
        <div class="flex justify-between items-start mb-3">
          <div class="flex-1">
            <h4 class="font-semibold text-lg">{{ user.name }}</h4>
            <p v-if="user.email" class="text-sm text-muted-foreground mt-1 flex items-center gap-1">
              <Mail class="w-3.5 h-3.5" />
              {{ user.email }}
            </p>
          </div>
          <div class="flex gap-2 items-center">
            <Badge v-if="user.isMfaActivated" variant="success" class="text-xs">MFA</Badge>
            <Badge v-if="user.emailVerified" variant="info" class="text-xs">已验证</Badge>
            <Badge :variant="user.state === 'ACTIVE' ? 'success' : 'destructive'" class="text-xs">
              {{ user.state }}
            </Badge>
          </div>
        </div>
        <div class="text-xs text-muted-foreground mb-4">
          创建时间: {{ user.createTime }}
          <span v-if="user.lastSuccessfulLoginTime" class="ml-4">最近登录: {{ user.lastSuccessfulLoginTime }}</span>
        </div>
        <div class="flex flex-wrap gap-2">
          <Button size="sm" variant="outline" @click="$emit('edit', user)">
            <Edit class="w-3.5 h-3.5" />
            编辑
          </Button>
          <Button size="sm" variant="warning" @click="$emit('reset-password', user)">
            <KeyRound class="w-3.5 h-3.5" />
            重置密码
          </Button>
          <Button v-if="user.isMfaActivated" size="sm" variant="outline" @click="$emit('clear-mfa', user)">
            <LockKeyhole class="w-3.5 h-3.5" />
            清除MFA
          </Button>
          <Button size="sm" variant="outline" @click="$emit('clear-api-keys', user)">
            <ShieldOff class="w-3.5 h-3.5" />
            清除API
          </Button>
          <Button size="sm" variant="destructive" @click="$emit('delete', user)">
            <Trash2 class="w-3.5 h-3.5" />
            删除
          </Button>
        </div>
      </div>
    </div>
  </Card>
</template>
