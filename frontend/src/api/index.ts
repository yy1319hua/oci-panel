// 类型化 API 层（C6）：集中封装后端端点，按域分模块。
// 调用方统一从 `@/api` 导入域对象（如 `import { ociApi, sysApi } from '@/api'`），
// 运行时等价于原先的 `api.post/api.get`，但带请求/响应类型。

export type { ApiResponse, PageResult, ValueLabel } from './http'

export { sysApi } from './sys'
export type {
  SysConfig,
  LoginResultData,
  TokenData,
  WebSocketTicket,
  AuthStatus,
  MfaSecret,
  RecentLogs
} from './sys'

export { passkeyApi } from './passkey'
export type { PublicKeyOptions, SerializedCredential } from './passkey'

export { telegramApi } from './telegram'
export type { TelegramConfig } from './telegram'

export { bootVolumeApi } from './bootVolume'

export { tokenApi } from './token'
export type { ApiToken, CreateTokenResult, TokenCallLog } from './token'

export { instanceApi } from './instance'

export { ociApi, vcnApi } from './oci'
export type {
  ConfigItem,
  ConfigDetails,
  InstanceInfo,
  VolumeInfo,
  VCNInfo,
  TenantInfo,
  TrafficCondition,
  TrafficData,
  InstanceTrafficStat,
  MonthlyTrafficStats,
  DailyCost,
  CostStats,
  SecurityRule,
  SecurityListData,
  AddSecurityRuleReq,
  UpdateSecurityRuleReq,
  DeleteSecurityRuleReq
} from './oci'
