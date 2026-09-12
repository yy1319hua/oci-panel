<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Play, Square, Trash2, FileText, RefreshCw, ChevronLeft, ChevronRight, Loader2 } from 'lucide-vue-next'
import { taskApi, type TaskItem as Task, type TaskLog } from '@/api'
import { toast } from '@/composables/useToast'
import { useSelection } from '@/composables/useSelection'
import { usePagination } from '@/composables/usePagination'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Select } from '@/components/ui/select'
import { Dialog, DialogHeader, DialogTitle } from '@/components/ui/dialog'

const tasks = ref<Task[]>([])
const loading = ref(false)
const { currentPage, pageSize, totalPages, applyResult } = usePagination()
const statusFilter = ref('')
const {
  selectedIds: selectedTaskIds,
  isAllSelected,
  isIndeterminate,
  toggle: toggleSelectTask,
  toggleAll: toggleSelectAll,
  clear: clearSelection
} = useSelection(tasks, t => t.id)
const actionLoading = ref<Record<string, boolean>>({})

const showLogsModal = ref(false)
const currentTask = ref<Task | null>(null)
const logs = ref<TaskLog[]>([])
const logsLoading = ref(false)
const {
  currentPage: logsPage,
  pageSize: logsPageSize,
  totalPages: logsTotalPages,
  applyResult: applyLogsResult
} = usePagination(20)

const statusMap: Record<string, string> = {
  running: '运行中',
  stopped: '已停止',
  completed: '已完成',
  error: '失败'
}

const statusVariant = (status: string) => {
  switch (status) {
    case 'running':
      return 'success'
    case 'stopped':
      return 'warning'
    case 'completed':
      return 'info'
    case 'error':
      return 'destructive'
    default:
      return 'secondary'
  }
}

const loadTasks = async (page = 1) => {
  loading.value = true
  try {
    const response = await taskApi.list({
      page,
      pageSize,
      status: statusFilter.value
    })
    tasks.value = response.data.list || []
    applyResult(response.data)
  } catch (error: any) {
    toast.error(error.message || '加载失败')
  } finally {
    loading.value = false
  }
}

const startTask = async (taskId: string) => {
  actionLoading.value[taskId] = true
  try {
    await taskApi.start(taskId)
    toast.success('任务已启动')
    await loadTasks(currentPage.value)
  } catch (error: any) {
    toast.error(error.message || '启动失败')
  } finally {
    delete actionLoading.value[taskId]
  }
}

const stopTask = async (taskId: string) => {
  actionLoading.value[taskId] = true
  try {
    await taskApi.stop(taskId)
    toast.success('任务已停止')
    await loadTasks(currentPage.value)
  } catch (error: any) {
    toast.error(error.message || '停止失败')
  } finally {
    delete actionLoading.value[taskId]
  }
}

const deleteTask = async (taskId: string) => {
  if (!confirm('确定要删除此任务吗？相关日志也将被删除。')) return
  actionLoading.value[taskId] = true
  try {
    await taskApi.delete(taskId)
    toast.success('任务已删除')
    await loadTasks(currentPage.value)
  } catch (error: any) {
    toast.error(error.message || '删除失败')
  } finally {
    delete actionLoading.value[taskId]
  }
}

const batchDeleteTasks = async () => {
  if (!confirm(`确定要删除选中的 ${selectedTaskIds.value.length} 个任务吗？`)) return
  try {
    await taskApi.batchDelete(selectedTaskIds.value)
    toast.success(`成功删除 ${selectedTaskIds.value.length} 个任务`)
    clearSelection()
    await loadTasks(currentPage.value)
  } catch (error: any) {
    toast.error(error.message || '批量删除失败')
  }
}

const viewLogs = async (task: Task) => {
  currentTask.value = task
  showLogsModal.value = true
  logsPage.value = 1
  await loadLogs(1)
}

const loadLogs = async (page = 1) => {
  if (!currentTask.value) return
  logsLoading.value = true
  try {
    const response = await taskApi.logs({
      taskId: currentTask.value.id,
      page,
      pageSize: logsPageSize
    })
    logs.value = response.data.list || []
    applyLogsResult(response.data)
  } catch (error: any) {
    toast.error(error.message || '加载日志失败')
  } finally {
    logsLoading.value = false
  }
}

const clearLogs = async () => {
  if (!currentTask.value || !confirm('确定要清空此任务的所有日志吗？')) return
  try {
    await taskApi.clearLogs(currentTask.value.id)
    toast.success('日志已清空')
    logs.value = []
    logsTotalPages.value = 0
  } catch (error: any) {
    toast.error(error.message || '清空日志失败')
  }
}

const _closeLogsModal = () => {
  showLogsModal.value = false
  currentTask.value = null
  logs.value = []
}
void _closeLogsModal

const formatTime = (time?: string) => {
  if (!time) return '-'
  return time.replace('T', ' ').substring(0, 19)
}

onMounted(() => {
  loadTasks()
})
</script>

<template>
  <div class="space-y-6">
    <!-- Header -->
    <div
      v-motion
      :initial="{ opacity: 0, y: -20 }"
      :enter="{ opacity: 1, y: 0 }"
      class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4"
    >
      <div class="flex items-center gap-4">
        <h1 class="text-3xl font-display font-bold">任务列表</h1>
        <Badge v-if="selectedTaskIds.length > 0" variant="secondary">已选择 {{ selectedTaskIds.length }} 项</Badge>
      </div>
      <div class="flex flex-wrap gap-2">
        <Button v-if="selectedTaskIds.length > 0" variant="destructive" @click="batchDeleteTasks">
          <Trash2 class="w-4 h-4" />
          批量删除
        </Button>
        <Select v-model="statusFilter" class="w-32" @change="loadTasks(1)">
          <option value="">全部状态</option>
          <option value="running">运行中</option>
          <option value="stopped">已停止</option>
          <option value="completed">已完成</option>
          <option value="error">失败</option>
        </Select>
        <Button variant="outline" @click="loadTasks(currentPage)">
          <RefreshCw class="w-4 h-4" />
          刷新
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
      <CardContent class="p-0">
        <!-- 加载 / 空态 -->
        <div v-if="loading" class="h-32 flex items-center justify-center">
          <Loader2 class="w-8 h-8 animate-spin text-primary" />
        </div>
        <div v-else-if="!tasks.length" class="h-32 flex items-center justify-center text-muted-foreground">
          暂无任务
        </div>
        <template v-else>
          <!-- 桌面端：表格 -->
          <div class="hidden lg:block overflow-x-auto">
            <div
              class="grid grid-cols-[2.5rem_9rem_7rem_minmax(0,1fr)_5rem_6rem_7rem_11rem_auto] gap-3 items-center px-4 py-3 text-xs font-medium text-muted-foreground border-b border-border/50 min-w-[1000px]"
            >
              <div class="flex items-center">
                <Checkbox
                  :model-value="isAllSelected"
                  :indeterminate="isIndeterminate"
                  @update:model-value="toggleSelectAll"
                />
              </div>
              <div>配置名</div>
              <div>区域</div>
              <div>配置</div>
              <div>间隔</div>
              <div>状态</div>
              <div>执行次数</div>
              <div>最后执行</div>
              <div class="text-right">操作</div>
            </div>
            <div
              v-for="(task, index) in tasks"
              :key="task.id"
              v-motion
              :initial="{ opacity: 0, x: -20 }"
              :enter="{ opacity: 1, x: 0, transition: { delay: 50 * index } }"
              class="grid grid-cols-[2.5rem_9rem_7rem_minmax(0,1fr)_5rem_6rem_7rem_11rem_auto] gap-3 items-center px-4 py-3 border-b border-border/50 last:border-0 group min-w-[1000px]"
            >
              <div class="flex items-center">
                <Checkbox
                  :model-value="selectedTaskIds.includes(task.id)"
                  @update:model-value="toggleSelectTask(task.id)"
                />
              </div>
              <div class="font-medium truncate">{{ task.username || '-' }}</div>
              <div><Badge variant="info">{{ task.ociRegion }}</Badge></div>
              <div class="min-w-0">
                <div class="text-sm truncate">{{ task.architecture }} / {{ task.operationSystem }}</div>
                <div class="text-xs text-muted-foreground">
                  {{ task.ocpus }}核 / {{ task.memory }}GB / {{ task.disk }}GB
                </div>
              </div>
              <div>{{ task.interval }}秒</div>
              <div>
                <Badge :variant="statusVariant(task.status) as any">
                  {{ statusMap[task.status] || task.status }}
                </Badge>
              </div>
              <div>
                <span>{{ task.executeCount }}</span>
                <span class="text-success ml-1">({{ task.successCount }} 成功)</span>
              </div>
              <div class="min-w-0">
                <div v-if="task.lastExecuteTime" class="text-sm">
                  {{ formatTime(task.lastExecuteTime) }}
                </div>
                <div class="text-xs text-muted-foreground truncate" :title="task.lastMessage">
                  {{ task.lastMessage || '-' }}
                </div>
              </div>
              <div class="flex justify-end gap-1">
                <Button
                  v-if="task.status === 'stopped'"
                  variant="ghost"
                  size="icon"
                  title="启动"
                  :disabled="actionLoading[task.id]"
                  @click="startTask(task.id)"
                >
                  <Play class="w-4 h-4 text-success" />
                </Button>
                <Button
                  v-if="task.status === 'running'"
                  variant="ghost"
                  size="icon"
                  title="停止"
                  :disabled="actionLoading[task.id]"
                  @click="stopTask(task.id)"
                >
                  <Square class="w-4 h-4 text-warning" />
                </Button>
                <Button variant="ghost" size="icon" title="查看日志" @click="viewLogs(task)">
                  <FileText class="w-4 h-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  title="删除"
                  class="text-destructive hover:text-destructive"
                  :disabled="actionLoading[task.id]"
                  @click="deleteTask(task.id)"
                >
                  <Trash2 class="w-4 h-4" />
                </Button>
              </div>
            </div>
          </div>

          <!-- 手机端：竖向卡片 -->
          <div class="lg:hidden divide-y divide-border/50">
            <div v-for="task in tasks" :key="task.id" class="p-4 space-y-3">
              <div class="flex items-start justify-between gap-3">
                <div class="flex items-center gap-3 min-w-0">
                  <Checkbox
                    :model-value="selectedTaskIds.includes(task.id)"
                    @update:model-value="toggleSelectTask(task.id)"
                  />
                  <span class="font-medium truncate">{{ task.username || '-' }}</span>
                </div>
                <Badge :variant="statusVariant(task.status) as any" class="shrink-0">
                  {{ statusMap[task.status] || task.status }}
                </Badge>
              </div>
              <div class="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
                <Badge variant="info">{{ task.ociRegion }}</Badge>
                <span>{{ task.architecture }} / {{ task.operationSystem }}</span>
                <span>{{ task.ocpus }}核 / {{ task.memory }}GB / {{ task.disk }}GB</span>
              </div>
              <div class="flex items-center justify-between text-sm">
                <span class="text-muted-foreground">间隔 {{ task.interval }}秒</span>
                <span>执行 {{ task.executeCount }} <span class="text-success">({{ task.successCount }} 成功)</span></span>
              </div>
              <div class="text-xs text-muted-foreground break-all">
                {{ task.lastExecuteTime ? formatTime(task.lastExecuteTime) + ' · ' : '' }}{{ task.lastMessage || '-' }}
              </div>
              <div class="flex flex-wrap gap-2">
                <Button
                  v-if="task.status === 'stopped'"
                  size="sm"
                  variant="success"
                  :disabled="actionLoading[task.id]"
                  @click="startTask(task.id)"
                >
                  <Play class="w-3.5 h-3.5" />启动
                </Button>
                <Button
                  v-if="task.status === 'running'"
                  size="sm"
                  variant="warning"
                  :disabled="actionLoading[task.id]"
                  @click="stopTask(task.id)"
                >
                  <Square class="w-3.5 h-3.5" />停止
                </Button>
                <Button size="sm" variant="outline" @click="viewLogs(task)">
                  <FileText class="w-3.5 h-3.5" />日志
                </Button>
                <Button size="sm" variant="destructive" :disabled="actionLoading[task.id]" @click="deleteTask(task.id)">
                  <Trash2 class="w-3.5 h-3.5" />删除
                </Button>
              </div>
            </div>
          </div>
        </template>

        <!-- Pagination -->
        <div v-if="totalPages > 1" class="flex items-center justify-center gap-2 p-4 border-t border-border/50">
          <Button variant="outline" size="sm" :disabled="currentPage === 1" @click="loadTasks(currentPage - 1)">
            <ChevronLeft class="w-4 h-4" />
            上一页
          </Button>
          <span class="px-4 text-sm text-muted-foreground">第 {{ currentPage }} / {{ totalPages }} 页</span>
          <Button
            variant="outline"
            size="sm"
            :disabled="currentPage === totalPages"
            @click="loadTasks(currentPage + 1)"
          >
            下一页
            <ChevronRight class="w-4 h-4" />
          </Button>
        </div>
      </CardContent>
    </Card>

    <!-- Logs Modal -->
    <Dialog v-model:open="showLogsModal">
      <DialogHeader class="mb-4">
        <div class="flex items-center justify-between">
          <div>
            <DialogTitle>任务执行日志</DialogTitle>
            <p class="text-sm text-muted-foreground mt-1">{{ currentTask?.username }} - {{ currentTask?.ociRegion }}</p>
          </div>
          <Button variant="outline" size="sm" @click="clearLogs">清空日志</Button>
        </div>
      </DialogHeader>

      <div class="max-h-96 overflow-y-auto space-y-2">
        <div v-if="logsLoading" class="text-center py-8">
          <Loader2 class="w-8 h-8 mx-auto animate-spin text-primary" />
        </div>
        <div v-else-if="!logs.length" class="text-center py-8 text-muted-foreground">暂无日志记录</div>
        <div
          v-for="log in logs"
          v-else
          :key="log.id"
          class="p-3 rounded-lg bg-secondary/50 border-l-4"
          :class="{
            'border-l-success': log.status === 'success',
            'border-l-destructive': log.status === 'error',
            'border-l-primary': log.status === 'info'
          }"
        >
          <div class="flex justify-between items-start mb-1">
            <Badge :variant="log.status === 'success' ? 'success' : log.status === 'error' ? 'destructive' : 'info'">
              {{ log.status === 'success' ? '成功' : log.status === 'error' ? '失败' : '信息' }}
            </Badge>
            <span class="text-xs text-muted-foreground">{{ formatTime(log.executeTime) }}</span>
          </div>
          <p class="text-sm break-all">{{ log.message }}</p>
        </div>
      </div>

      <div v-if="logsTotalPages > 1" class="flex items-center justify-center gap-2 pt-4 border-t border-border/50 mt-4">
        <Button variant="outline" size="sm" :disabled="logsPage === 1" @click="loadLogs(logsPage - 1)">上一页</Button>
        <span class="px-3 text-sm text-muted-foreground">{{ logsPage }} / {{ logsTotalPages }}</span>
        <Button variant="outline" size="sm" :disabled="logsPage === logsTotalPages" @click="loadLogs(logsPage + 1)">
          下一页
        </Button>
      </div>
    </Dialog>
  </div>
</template>
