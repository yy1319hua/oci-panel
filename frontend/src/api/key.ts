import { get, post, type PageResult } from './http'

/** SSH 密钥列表项。 */
export interface KeyItem {
  id: string
  name: string
  keyType: string
  publicKey: string
  configName?: string
  createTime: string
}

/** 密钥列表分页查询参数。 */
export interface KeyListReq {
  page: number
  pageSize: number
  name?: string
  keyType?: string
}

export const keyApi = {
  list: (req: KeyListReq) => get<PageResult<KeyItem>>('/key/list', { params: req }),
  standalone: () => get<KeyItem[]>('/key/standalone'),
  create: (req: { name: string; publicKey: string }) => post('/key/create', req),
  update: (req: { id: string; name: string; publicKey: string }) => post('/key/update', req),
  delete: (ids: string[]) => post('/key/delete', { ids })
}
