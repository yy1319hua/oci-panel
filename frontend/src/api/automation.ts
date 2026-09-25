import { get, post } from './http'

/** 保活任务 */
export interface KeepaliveTask {
  id: number
  configId: string
  instanceId: string
  instanceName: string
  enabled: boolean
  intervalMin: number
  durationSec: number
  lastRunAt?: string
  lastResult: string
  status: string
}

/** 抢机任务 */
export interface GrabTask {
  id: number
  configId: string
  name: string
  ad: string
  shape: string
  ocpus: number
  memoryGB: number
  bootVolumeGB: number
  imageId: string
  subnetId: string
  instanceName: string
  intervalMin: number
  enabled: boolean
  lastTryAt?: string
  lastError: string
  tryCount: number
  status: string
}

/** 备份任务 */
export interface BackupTask {
  id: number
  configId: string
  volumeId: string
  volumeName: string
  retention: number
  intervalHour: number
  enabled: boolean
  lastRunAt?: string
  lastResult: string
}

/** 配额总览 */
export interface QuotaOverview {
  configId: string
  tenantName: string
  region: string
  instanceCount: number
  a1OcpusUsed: number
  a1MemoryUsed: number
  e2MicroUsed: number
  bootVolumeGB: number
  backupCount: number
  grabRunning: number
}

export const automationApi = {
  // 保活
  listKeepalive: () => get<KeepaliveTask[]>('/api/automation/keepalive/list'),
  saveKeepalive: (data: Partial<KeepaliveTask>) => post('/api/automation/keepalive/save', data),
  deleteKeepalive: (id: number) => post('/api/automation/keepalive/delete', { id }),
  runKeepalive: (id: number) => post('/api/automation/keepalive/run', { id }),
  // 抢机
  listGrab: () => get<GrabTask[]>('/api/automation/grab/list'),
  saveGrab: (data: Partial<GrabTask>) => post('/api/automation/grab/save', data),
  deleteGrab: (id: number) => post('/api/automation/grab/delete', { id }),
  runGrab: (id: number) => post('/api/automation/grab/run', { id }),
  // 备份
  listBackup: () => get<BackupTask[]>('/api/automation/backup/list'),
  saveBackup: (data: Partial<BackupTask>) => post('/api/automation/backup/save', data),
  deleteBackup: (id: number) => post('/api/automation/backup/delete', { id }),
  runBackup: (id: number) => post('/api/automation/backup/run', { id }),
  listVolumes: (configId: string) => get<Array<{ id: string; name: string; type: string; sizeGB: number }>>(
    `/api/automation/backup/volumes?configId=${encodeURIComponent(configId)}`
  ),
  // 告警 / 配额 / 推送设置
  getQuota: () => get<QuotaOverview[]>('/api/automation/quota'),
  saveSettings: (data: { trafficAlertThreshold: number }) => post('/api/automation/settings', data),
  clearAlert: (configId = '') => post(`/api/automation/alert/clear?configId=${encodeURIComponent(configId)}`, {}),
  getCpuMemory: (data: { configId: string; instanceId: string; hours: number }) =>
    post<{ time: string[]; cpu: number[]; memory: number[]; hasMemory: boolean }>(
      '/api/automation/metrics/cpu-memory', data
    ),
  // PushPlus
  getPushplus: () => get<{ enabled: boolean; tokenMasked: string }>('/api/automation/pushplus/get'),
  savePushplus: (data: { token: string }) => post('/api/automation/pushplus/save', data),
  testPushplus: () => post('/api/automation/pushplus/test', {})
}

/** 抢机表单下拉数据（AD / 镜像 / 子网）。 */
export const lookupApi = {
  ads: (configId: string) =>
    get<Array<{ value: string; label: string }>>(`/api/automation/lookup/ads?configId=${encodeURIComponent(configId)}`),
  images: (configId: string, shape: string) =>
    get<Array<{ value: string; label: string }>>(
      `/api/automation/lookup/images?configId=${encodeURIComponent(configId)}&shape=${encodeURIComponent(shape)}`
    ),
  subnets: (configId: string) =>
    get<Array<{ value: string; label: string }>>(
      `/api/automation/lookup/subnets?configId=${encodeURIComponent(configId)}`
    )
}
