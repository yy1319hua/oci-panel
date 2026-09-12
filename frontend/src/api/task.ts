import { post, type PageResult } from './http'

/** 实例创建任务列表项。 */
export interface TaskItem {
  id: string
  username: string
  ociRegion: string
  architecture: string
  operationSystem: string
  ocpus: number
  memory: number
  disk: number
  interval: number
  status: string
  executeCount: number
  successCount: number
  lastExecuteTime?: string
  lastMessage?: string
}

/** 任务执行日志。 */
export interface TaskLog {
  id: string
  status: string
  message: string
  executeTime: string
}

/** 创建实例任务的入参，配置与密钥 ID 均为后端生成的 UUID。 */
export interface CreateTaskReq {
  userId: string
  ociRegion: string
  ocpus: number
  memory: number
  disk: number
  bootVolumeVpu?: number
  architecture: string
  operationSystem: string
  imageId?: string
  sshKeyId: string
  interval: number
  executeOnce?: boolean
}

export const taskApi = {
  create: (req: CreateTaskReq) => post('/task/create', req),
  list: (req: { page: number; pageSize: number; status?: string }) => post<PageResult<TaskItem>>('/task/list', req),
  start: (taskId: string) => post('/task/start', { taskId }),
  stop: (taskId: string) => post('/task/stop', { taskId }),
  delete: (taskId: string) => post('/task/delete', { taskId }),
  batchDelete: (taskIds: string[]) => post('/task/batchDelete', { taskIds }),
  logs: (req: { taskId: string; page: number; pageSize: number }) => post<PageResult<TaskLog>>('/task/logs', req),
  clearLogs: (taskId: string) => post('/task/clearLogs', { taskId })
}
