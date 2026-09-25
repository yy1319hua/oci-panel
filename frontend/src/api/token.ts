import { get, post } from './http'

/** API 令牌（后台生成、可轮换/吊销）。 */
export interface ApiToken {
  id: number
  name: string
  prefix: string
  scope: string
  expiresAt: string | null
  lastUsedAt: string | null
  callCount: number
  lastUsedIp: string
  createdAt: string
}

/** 创建令牌的结果：token 为明文，仅返回一次。 */
export interface CreateTokenResult {
  token: string
  info: ApiToken
}

/** 一条令牌调用记录（参考青龙面板的调用日志）。 */
export interface TokenCallLog {
  id: number
  tokenId: number
  method: string
  path: string
  statusCode: number
  ip: string
  createdAt: string
}

export const tokenApi = {
  create: (req: { name: string; expiresInDays?: number; scope?: string }) =>
    post<CreateTokenResult>('/token', req),
  list: () => get<ApiToken[]>('/token/list'),
  calls: (id: number) => get<TokenCallLog[]>(`/token/calls?id=${id}`),
  revoke: (id: number) => post('/token/revoke', { id })
}
