<script setup lang="ts">
import { ref, reactive, watch } from 'vue'
import { Loader2, Shield, Plus, Unlock, Trash2, Pencil } from 'lucide-vue-next'
import { vcnApi, type SecurityRule } from '@/api'
import { toast } from '@/composables/useToast'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import { Modal } from '@/components/ui/modal'

interface VCN {
  id: string
  displayName: string
}

const props = defineProps<{
  open: boolean
  vcn: VCN | null
  userId: string
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  refresh: []
}>()

const loading = ref(false)
const releasing = ref(false)
const deleting = ref(false)
const addingRule = ref(false)
const savingRule = ref(false)
const deletingRuleKey = ref('')
const showAddRuleModal = ref(false)

const securityList = ref<{ ingressRules?: SecurityRule[]; egressRules?: SecurityRule[] } | null>(null)

const addRuleForm = reactive({
  isIngress: true,
  protocol: '6',
  cidr: '0.0.0.0/0',
  portMin: 1,
  portMax: 65535,
  description: ''
})

// 编辑规则：editingRule 保存原始规则（作为后端定位键），editForm 为编辑中的值。
const showEditRuleModal = ref(false)
const editingIsIngress = ref(true)
const editingRule = ref<SecurityRule | null>(null)
const editForm = reactive({
  protocol: '6',
  cidr: '0.0.0.0/0',
  portMin: 1,
  portMax: 65535,
  description: ''
})

watch(
  () => props.open,
  async open => {
    if (open && props.vcn) {
      await loadSecurityList()
    }
  }
)

const close = () => emit('update:open', false)

const loadSecurityList = async () => {
  if (!props.vcn) return
  loading.value = true
  try {
    const response = await vcnApi.securityList({
      configId: props.userId,
      vcnId: props.vcn.id
    })
    securityList.value = response.data
  } catch (error: any) {
    toast.error(error.message || '获取安全列表失败')
  } finally {
    loading.value = false
  }
}

const formatPortRange = (rule: SecurityRule) => {
  if (rule.protocolName === '所有协议' || rule.protocolName === 'all' || rule.protocol === 'all') return '所有'
  if (rule.protocolName === 'ICMP' || rule.protocolName === 'ICMPv6') {
    if (rule.icmpType !== undefined && rule.icmpType !== null) {
      return `Type: ${rule.icmpType}${rule.icmpCode !== undefined && rule.icmpCode !== null ? ', Code: ' + rule.icmpCode : ''}`
    }
    return '所有'
  }
  if (rule.portRangeMin && rule.portRangeMax) {
    if (rule.portRangeMin === rule.portRangeMax) return String(rule.portRangeMin)
    return `${rule.portRangeMin}-${rule.portRangeMax}`
  }
  return '所有'
}

/** 规则的唯一标识：协议 + 来源/目标 + 端口范围（与后端定位键一致）。 */
const ruleKey = (rule: SecurityRule) =>
  `${rule.protocol}|${rule.source || rule.destination || ''}|${rule.portRangeMin || 0}-${rule.portRangeMax || 0}`

const isBusy = (rule: SecurityRule) => savingRule.value || deletingRuleKey.value === ruleKey(rule)

const openAddRuleForm = (type: 'ingress' | 'egress') => {
  addRuleForm.isIngress = type === 'ingress'
  addRuleForm.protocol = '6'
  addRuleForm.cidr = '0.0.0.0/0'
  addRuleForm.portMin = 1
  addRuleForm.portMax = 65535
  addRuleForm.description = ''
  showAddRuleModal.value = true
}

const submitAddRule = async () => {
  if (!addRuleForm.cidr) {
    toast.warning('请输入CIDR地址')
    return
  }
  addingRule.value = true
  try {
    const params: any = {
      configId: props.userId,
      vcnId: props.vcn?.id,
      isIngress: addRuleForm.isIngress,
      protocol: addRuleForm.protocol,
      description: addRuleForm.description || undefined
    }
    if (addRuleForm.protocol === '6' || addRuleForm.protocol === '17') {
      params.portMin = addRuleForm.portMin
      params.portMax = addRuleForm.portMax
    }
    if (addRuleForm.isIngress) {
      params.source = addRuleForm.cidr
    } else {
      params.destination = addRuleForm.cidr
    }
    await vcnApi.addSecurityRule(params)
    toast.success('安全规则添加成功')
    showAddRuleModal.value = false
    await loadSecurityList()
  } catch (error: any) {
    toast.error(error.message || '添加失败')
  } finally {
    addingRule.value = false
  }
}

// 行内编辑：把现有规则预填到编辑表单（协议为 all/ICMP 时端口字段不参与提交）
const openEditRuleForm = (rule: SecurityRule, isIngress: boolean) => {
  editingIsIngress.value = isIngress
  editingRule.value = rule
  editForm.protocol = rule.protocol
  editForm.cidr = (isIngress ? rule.source : rule.destination) || '0.0.0.0/0'
  editForm.portMin = rule.portRangeMin || 1
  editForm.portMax = rule.portRangeMax || 65535
  editForm.description = rule.description || ''
  showEditRuleModal.value = true
}

const submitEditRule = async () => {
  if (!editingRule.value || !props.vcn) return
  if (!editForm.cidr) {
    toast.warning('请输入CIDR地址')
    return
  }
  savingRule.value = true
  try {
    const newRule: SecurityRule = {
      protocol: editForm.protocol,
      source: editingIsIngress.value ? editForm.cidr : '',
      destination: editingIsIngress.value ? '' : editForm.cidr,
      portRangeMin: 0,
      portRangeMax: 0,
      description: editForm.description
    }
    if (editForm.protocol === '6' || editForm.protocol === '17') {
      newRule.portRangeMin = editForm.portMin
      newRule.portRangeMax = editForm.portMax
    }
    await vcnApi.updateSecurityRule({
      configId: props.userId,
      vcnId: props.vcn.id,
      isIngress: editingIsIngress.value,
      oldRule: editingRule.value,
      newRule
    })
    toast.success('安全规则修改成功')
    showEditRuleModal.value = false
    await loadSecurityList()
  } catch (error: any) {
    toast.error(error.message || '修改失败')
  } finally {
    savingRule.value = false
  }
}

const deleteRule = async (rule: SecurityRule, isIngress: boolean) => {
  if (!props.vcn) return
  const target = (isIngress ? rule.source : rule.destination) || '0.0.0.0/0'
  if (!confirm(`确定要删除该${isIngress ? '入站' : '出站'}规则吗？\n协议 ${rule.protocolName || rule.protocol} · ${target} · 端口 ${formatPortRange(rule)}`))
    return
  deletingRuleKey.value = ruleKey(rule)
  try {
    await vcnApi.deleteSecurityRule({
      configId: props.userId,
      vcnId: props.vcn.id,
      isIngress,
      rule
    })
    toast.success('安全规则删除成功')
    await loadSecurityList()
  } catch (error: any) {
    toast.error(error.message || '删除失败')
  } finally {
    deletingRuleKey.value = ''
  }
}

const releaseAllRules = async () => {
  if (!confirm('确定要放行所有规则吗？这将允许所有入站和出站流量。')) return
  releasing.value = true
  try {
    await vcnApi.releaseSecurityRules({
      configId: props.userId,
      vcnId: props.vcn?.id
    })
    toast.success('安全规则已放行')
    await loadSecurityList()
  } catch (error: any) {
    toast.error(error.message || '放行失败')
  } finally {
    releasing.value = false
  }
}

const deleteVcn = async () => {
  if (!confirm(`确定要删除VCN "${props.vcn?.displayName}" 吗？此操作将删除VCN及其所有子网、网关等资源，且不可恢复！`))
    return
  deleting.value = true
  try {
    await vcnApi.delete({
      configId: props.userId,
      vcnId: props.vcn?.id
    })
    toast.success('VCN删除成功')
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
  <Modal
    :open="open"
    max-width="max-w-4xl"
    panel-class="max-h-[90vh] flex flex-col"
    @update:open="emit('update:open', $event)"
  >
    <template #title>
      <Shield class="w-5 h-5 text-primary" />
      安全列表 - {{ vcn?.displayName }}
    </template>

    <div class="flex-1 overflow-y-auto p-6">
      <div v-if="loading" class="flex items-center justify-center py-12">
        <Loader2 class="w-10 h-10 animate-spin text-primary" />
      </div>

      <div v-else-if="securityList" class="space-y-6">
        <!-- 操作按钮 -->
        <div class="flex gap-2">
          <Button variant="success" :disabled="releasing" @click="releaseAllRules">
            <Loader2 v-if="releasing" class="w-4 h-4 animate-spin" />
            <Unlock v-else class="w-4 h-4" />
            放行所有规则
          </Button>
          <Button variant="destructive" :disabled="deleting" @click="deleteVcn">
            <Loader2 v-if="deleting" class="w-4 h-4 animate-spin" />
            <Trash2 v-else class="w-4 h-4" />
            删除VCN
          </Button>
        </div>

        <!-- 入站规则 -->
        <Card class="p-4">
          <div class="flex justify-between items-center mb-4">
            <h3 class="text-lg font-semibold text-success flex items-center gap-2">入站规则 (Ingress)</h3>
            <Button size="sm" @click="openAddRuleForm('ingress')">
              <Plus class="w-4 h-4" />
              添加规则
            </Button>
          </div>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>协议</TableHead>
                <TableHead>来源</TableHead>
                <TableHead>端口</TableHead>
                <TableHead>描述</TableHead>
                <TableHead class="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="(rule, index) in securityList.ingressRules" :key="'in-' + index">
                <TableCell>
                  <Badge variant="info">{{ rule.protocolName }}</Badge>
                </TableCell>
                <TableCell class="font-mono text-xs">{{ rule.source }}</TableCell>
                <TableCell>{{ formatPortRange(rule) }}</TableCell>
                <TableCell class="text-muted-foreground">{{ rule.description || '-' }}</TableCell>
                <TableCell class="text-right">
                  <div class="flex justify-end gap-1">
                    <Button size="sm" variant="ghost" :disabled="isBusy(rule)" @click="openEditRuleForm(rule, true)">
                      <Pencil class="w-3.5 h-3.5" />
                    </Button>
                    <Button size="sm" variant="ghost" :disabled="isBusy(rule)" @click="deleteRule(rule, true)">
                      <Loader2 v-if="deletingRuleKey === ruleKey(rule)" class="w-3.5 h-3.5 animate-spin" />
                      <Trash2 v-else class="w-3.5 h-3.5 text-destructive" />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
              <TableRow v-if="!securityList.ingressRules?.length">
                <TableCell colspan="5" class="text-center text-muted-foreground py-8">暂无入站规则</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </Card>

        <!-- 出站规则 -->
        <Card class="p-4">
          <div class="flex justify-between items-center mb-4">
            <h3 class="text-lg font-semibold text-primary flex items-center gap-2">出站规则 (Egress)</h3>
            <Button size="sm" @click="openAddRuleForm('egress')">
              <Plus class="w-4 h-4" />
              添加规则
            </Button>
          </div>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>协议</TableHead>
                <TableHead>目标</TableHead>
                <TableHead>端口</TableHead>
                <TableHead>描述</TableHead>
                <TableHead class="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="(rule, index) in securityList.egressRules" :key="'out-' + index">
                <TableCell>
                  <Badge variant="info">{{ rule.protocolName }}</Badge>
                </TableCell>
                <TableCell class="font-mono text-xs">{{ rule.destination }}</TableCell>
                <TableCell>{{ formatPortRange(rule) }}</TableCell>
                <TableCell class="text-muted-foreground">{{ rule.description || '-' }}</TableCell>
                <TableCell class="text-right">
                  <div class="flex justify-end gap-1">
                    <Button size="sm" variant="ghost" :disabled="isBusy(rule)" @click="openEditRuleForm(rule, false)">
                      <Pencil class="w-3.5 h-3.5" />
                    </Button>
                    <Button size="sm" variant="ghost" :disabled="isBusy(rule)" @click="deleteRule(rule, false)">
                      <Loader2 v-if="deletingRuleKey === ruleKey(rule)" class="w-3.5 h-3.5 animate-spin" />
                      <Trash2 v-else class="w-3.5 h-3.5 text-destructive" />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
              <TableRow v-if="!securityList.egressRules?.length">
                <TableCell colspan="5" class="text-center text-muted-foreground py-8">暂无出站规则</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </Card>
      </div>
    </div>

    <div class="p-6 border-t border-border">
      <Button variant="outline" class="w-full" @click="close">关闭</Button>
    </div>
  </Modal>

  <!-- 添加规则弹窗 -->
  <Modal :open="showAddRuleModal" max-width="max-w-lg" z-class="z-[60]" @update:open="showAddRuleModal = $event">
    <template #title>添加{{ addRuleForm.isIngress ? '入站' : '出站' }}规则</template>

    <div class="p-6 space-y-4">
      <div>
        <label class="block text-sm font-medium mb-2">协议</label>
        <select
          v-model="addRuleForm.protocol"
          class="w-full h-10 px-3 rounded-md border border-input bg-background text-sm"
        >
          <option value="all">所有协议</option>
          <option value="6">TCP</option>
          <option value="17">UDP</option>
          <option value="1">ICMP</option>
        </select>
      </div>

      <div>
        <label class="block text-sm font-medium mb-2">
          {{ addRuleForm.isIngress ? '来源 CIDR' : '目标 CIDR' }}
        </label>
        <Input v-model="addRuleForm.cidr" placeholder="0.0.0.0/0 或 ::/0" />
      </div>

      <div v-if="['6', '17'].includes(addRuleForm.protocol)" class="grid grid-cols-2 gap-4">
        <div>
          <label class="block text-sm font-medium mb-2">端口范围(最小)</label>
          <Input v-model.number="addRuleForm.portMin" type="number" min="1" max="65535" placeholder="1" />
        </div>
        <div>
          <label class="block text-sm font-medium mb-2">端口范围(最大)</label>
          <Input v-model.number="addRuleForm.portMax" type="number" min="1" max="65535" placeholder="65535" />
        </div>
      </div>

      <div>
        <label class="block text-sm font-medium mb-2">描述(可选)</label>
        <Input v-model="addRuleForm.description" placeholder="规则描述" />
      </div>
    </div>

    <div class="p-6 border-t border-border flex gap-3">
      <Button variant="outline" class="flex-1" @click="showAddRuleModal = false">取消</Button>
      <Button class="flex-1" :disabled="addingRule" @click="submitAddRule">
        <Loader2 v-if="addingRule" class="w-4 h-4 animate-spin" />
        {{ addingRule ? '添加中...' : '添加规则' }}
      </Button>
    </div>
  </Modal>

  <!-- 编辑规则弹窗 -->
  <Modal :open="showEditRuleModal" max-width="max-w-lg" z-class="z-[60]" @update:open="showEditRuleModal = $event">
    <template #title>编辑{{ editingIsIngress ? '入站' : '出站' }}规则</template>

    <div class="p-6 space-y-4">
      <p class="text-xs text-muted-foreground">
        原规则：协议 {{ editingRule?.protocolName || editingRule?.protocol }} ·
        {{ editingIsIngress ? editingRule?.source : editingRule?.destination }} ·
        端口 {{ editingRule ? formatPortRange(editingRule) : '-' }}
      </p>

      <div>
        <label class="block text-sm font-medium mb-2">协议</label>
        <select
          v-model="editForm.protocol"
          class="w-full h-10 px-3 rounded-md border border-input bg-background text-sm"
        >
          <option value="all">所有协议</option>
          <option value="6">TCP</option>
          <option value="17">UDP</option>
          <option value="1">ICMP</option>
        </select>
      </div>

      <div>
        <label class="block text-sm font-medium mb-2">
          {{ editingIsIngress ? '来源 CIDR' : '目标 CIDR' }}
        </label>
        <Input v-model="editForm.cidr" placeholder="0.0.0.0/0 或 ::/0" />
      </div>

      <div v-if="['6', '17'].includes(editForm.protocol)" class="grid grid-cols-2 gap-4">
        <div>
          <label class="block text-sm font-medium mb-2">端口范围(最小)</label>
          <Input v-model.number="editForm.portMin" type="number" min="1" max="65535" placeholder="1" />
        </div>
        <div>
          <label class="block text-sm font-medium mb-2">端口范围(最大)</label>
          <Input v-model.number="editForm.portMax" type="number" min="1" max="65535" placeholder="65535" />
        </div>
      </div>

      <div>
        <label class="block text-sm font-medium mb-2">描述(可选)</label>
        <Input v-model="editForm.description" placeholder="规则描述" />
      </div>

      <p class="text-xs text-muted-foreground flex items-start gap-1.5">
        <Shield class="w-3.5 h-3.5 shrink-0 mt-0.5" />
        <span>
          甲骨文安全列表为整表覆盖，本操作按「协议 + CIDR + 端口」定位原规则后替换。若你同时修改了协议或端口，
          定位仍以原值为准，因此不会误改其他规则。
        </span>
      </p>
    </div>

    <div class="p-6 border-t border-border flex gap-3">
      <Button variant="outline" class="flex-1" @click="showEditRuleModal = false">取消</Button>
      <Button class="flex-1" :disabled="savingRule" @click="submitEditRule">
        <Loader2 v-if="savingRule" class="w-4 h-4 animate-spin" />
        {{ savingRule ? '保存中...' : '保存修改' }}
      </Button>
    </div>
  </Modal>
</template>
