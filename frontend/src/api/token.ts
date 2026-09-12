import { get, post } from './http'

/** API 令牌（后台生成、可轮换/吊销）。 */
export interface ApiToken {
  id: number
  name: string
  prefix: string
  scope: 'full' | 'readonly'
  expiresAt: string | null
  lastUsedAt: string | null
  createdAt: string
}

/** 创建令牌的结果：token 为明文，仅返回一次。 */
export interface CreateTokenResult {
  token: string
  info: ApiToken
}

export const tokenApi = {
  create: (req: { name: string; expiresInDays?: number }) =>
    post<CreateTokenResult>('/token', req),
  list: () => get<ApiToken[]>('/token/list'),
  revoke: (id: number) => post('/token/revoke', { id })
}
