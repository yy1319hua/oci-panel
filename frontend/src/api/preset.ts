import { get, post } from './http'

/** 预设配置（实例创建参数模板）。包含 Presets 页所需的全部字段；
 *  Configs 页只用其中子集，结构兼容。 */
export interface Preset {
  id: string
  name: string
  ocpus: number
  memory: number
  disk: number
  bootVolumeVpu: number
  architecture: string
  operationSystem: string
  imageId: string
  sshKeyId: string
  sshKeyName: string
  description: string
  createTime: string
}

/** 创建/更新预设的入参（id 仅更新时需要）。 */
export interface PresetForm {
  name: string
  ocpus: number
  memory: number
  disk: number
  bootVolumeVpu: number
  architecture: string
  operationSystem: string
  imageId: string
  sshKeyId: string
  description: string
}

export const presetApi = {
  list: () => get<Preset[]>('/preset/list'),
  create: (req: PresetForm) => post('/preset/create', req),
  update: (req: PresetForm & { id: string }) => post('/preset/update', req),
  delete: (id: string) => post('/preset/delete', { id })
}
