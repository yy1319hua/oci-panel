import { get, post } from './http'

/** 系统配置（密钥目录 / 日志级别 / 缓存开关）。 */
export interface SysConfig {
  keyDirPath: string
  logLevel: string
  cacheEnabled?: boolean
  cacheInterval?: number
}

/** 登录结果：未完成时返回需要的二次验证标志，完成时带 token。 */
export interface LoginResultData {
  needMfa?: boolean
  needPasskey?: boolean
  passkeyEnabled?: boolean
  mfaTicket?: string
  token?: string
  username?: string
}

/** 颁发 token 的结果（MFA / Passkey 验证成功后）。 */
export interface TokenData {
  token: string
  username: string
}

export interface WebSocketTicket {
  ticket: string
}

/** 认证状态（MFA / Passkey 是否启用）。 */
export interface AuthStatus {
  mfaEnabled: boolean
  passkeyEnabled: boolean
}

/** 生成 MFA 密钥的结果。 */
export interface MfaSecret {
  secret: string
  qrCode: string
}

/** 概览统计。 */
export interface Glance {
  totalConfigs: number
  totalTasks: number
}

/** 版本信息（统一数据源，后端返回固定常量）。 */
export interface VersionResponse {
  version: string
}

/** 当前管理员资料。 */
export interface Profile {
  account: string
  email: string
}

/** 历史日志（服务端环形缓冲快照），供日志页首屏立即渲染。 */
export interface RecentLogs {
  lines: string[]
  count: number
}

export const sysApi = {
  login: (account: string, password: string) => post<LoginResultData>('/sys/login', { account, password }),
  checkMfaCode: (ticket: string, code: string) => post<TokenData>('/sys/checkMfaCode', { ticket, code }),
  issueWebSocketTicket: () => post<WebSocketTicket>('/sys/wsTicket', {}),
  getSysCfg: () => get<SysConfig>('/sys/getSysCfg'),
  updateCacheCfg: (req: { cacheEnabled: boolean; cacheInterval: number }) => post('/sys/updateCacheCfg', req),
  refreshCache: () => post('/sys/refreshCache', {}),
  getAuthStatus: () => get<AuthStatus>('/sys/getAuthStatus'),
  generateMfaSecret: () => post<MfaSecret>('/sys/generateMfaSecret', {}),
  enableMfa: (req: { secret: string; code: string }) => post('/sys/enableMfa', req),
  disableMfa: () => post('/sys/disableMfa', {}),
  getProfile: () => get<Profile>('/sys/getProfile'),
  changePassword: (req: { oldPassword: string; newPassword: string }) => post('/sys/changePassword', req),
  updateEmail: (req: { email: string }) => post('/sys/updateEmail', req),
  updateLogLevel: (level: string) => post('/sys/updateLogLevel', { level }),
  updateAccount: (account: string) => post<{ token: string; account: string }>('/sys/updateAccount', { account }),
  getGlance: () => get<Glance>('/sys/getGlance'),
  /**
   * 拉取服务端缓冲的历史日志。日志页首屏用它立即渲染，
   * 不必等 WebSocket 建连完成（后者要经历取 ticket → 升级 → 回放三步）。
   */
  getRecentLogs: (params?: { lines?: number; level?: string }) =>
    get<RecentLogs>('/sys/recentLogs', { params }),
  getVersion: () => get<VersionResponse>('/sys/getVersion'),
  requestPasswordReset: (email: string) => post('/sys/requestPasswordReset', { email }),
  resetPassword: (token: string, newPassword: string) => post('/sys/resetPassword', { token, newPassword })
}
