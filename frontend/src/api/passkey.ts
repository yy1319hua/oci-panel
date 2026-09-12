import { get, post } from './http'
import type { TokenData } from './sys'

/**
 * WebAuthn 选项信封：后端返回的 `{ publicKey: {...} }`（challenge / allowCredentials / user 等字段
 * 为 base64url 字符串，需在前端转成 ArrayBuffer，见 `src/lib/webauthn.ts`）。
 * publicKey 用 any：其结构由浏览器 WebAuthn 规范决定、字段繁多且需直接喂给
 * `navigator.credentials`，C8 会在 `webauthn.ts` 中集中封装转换逻辑。
 */
export interface PublicKeyOptions {
  publicKey: any
}

/** 序列化后的凭证（base64url 编码），提交给后端完成注册/登录。 */
export interface SerializedCredential {
  id: string
  rawId: string
  type: string
  response: Record<string, string | null>
}

export const passkeyApi = {
  status: () => get<{ enabled: boolean }>('/passkey/status'),
  beginLogin: () => post<PublicKeyOptions>('/passkey/beginLogin', {}),
  finishLogin: (credential: SerializedCredential) => post<TokenData>('/passkey/finishLogin', { credential }),
  beginRegistration: () => post<PublicKeyOptions>('/passkey/beginRegistration', {}),
  finishRegistration: (credential: SerializedCredential) => post('/passkey/finishRegistration', { credential }),
  disable: () => post('/passkey/disable', {})
}
