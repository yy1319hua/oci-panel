// 类型化 API 层（C6）：集中封装后端端点，按域分模块。
// 调用方统一从 `@/api` 导入域对象（如 `import { ociApi, taskApi } from '@/api'`），
// 运行时等价于原先的 `api.post/api.get`，但带请求/响应类型。

export type { ApiResponse, PageResult, ValueLabel } from './http'

export { sysApi } from './sys'
export type { SysConfig, LoginResultData, TokenData, WebSocketTicket, AuthStatus, MfaSecret, Glance } from './sys'

export { passkeyApi } from './passkey'
export type { PublicKeyOptions, SerializedCredential } from './passkey'

export { telegramApi } from './telegram'
export type { TelegramConfig } from './telegram'

export { presetApi } from './preset'
export type { Preset, PresetForm } from './preset'

export { keyApi } from './key'
export type { KeyItem, KeyListReq } from './key'

export { taskApi } from './task'
export type { TaskItem, TaskLog, CreateTaskReq } from './task'

export { bootVolumeApi } from './bootVolume'

export { tokenApi } from './token'
export type { ApiToken, CreateTokenResult } from './token'

export { instanceApi } from './instance'

export { ociApi, vcnApi } from './oci'
export type {
  ConfigItem,
  ConfigDetails,
  InstanceInfo,
  VolumeInfo,
  VCNInfo,
  TenantInfo,
  ImageInfo,
  TrafficCondition,
  TrafficData,
  SecurityListData,
  AddSecurityRuleReq
} from './oci'
