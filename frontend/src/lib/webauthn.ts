import type { SerializedCredential } from '@/api/passkey'

// WebAuthn 前端工具（C8）：集中编解码与凭证序列化，消除 Login.vue / Settings.vue 的重复实现。
// SerializedCredential 的形状是与后端 /passkey/finish* 的约定（定义在 api 层），此处仅做类型引用（编译期擦除）。

/** base64url 字符串 → ArrayBuffer。WebAuthn 选项里的 challenge / id / user.id 等均为 base64url 编码。 */
function base64UrlToArrayBuffer(base64: string): ArrayBuffer {
  const binaryString = atob(base64.replace(/-/g, '+').replace(/_/g, '/'))
  const bytes = new Uint8Array(binaryString.length)
  for (let i = 0; i < binaryString.length; i++) {
    bytes[i] = binaryString.charCodeAt(i)
  }
  return bytes.buffer
}

/** ArrayBuffer → base64url 字符串。提交凭证时把二进制字段编码回 base64url（去除填充）。 */
function arrayBufferToBase64Url(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer)
  let binary = ''
  for (let i = 0; i < bytes.byteLength; i++) {
    binary += String.fromCharCode(bytes[i])
  }
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '')
}

/**
 * 通行密钥「登录」：用后端下发的 publicKey 选项调用 navigator.credentials.get，
 * 把断言（assertion）结果序列化为可提交给 /passkey/finishLogin 的凭证。
 * publicKey 用 any：其结构由浏览器 WebAuthn 规范决定，需直接喂给浏览器 API。
 */

export async function getPasskeyCredential(publicKey: any): Promise<SerializedCredential> {
  const credential = (await navigator.credentials.get({
    publicKey: {
      ...publicKey,
      challenge: base64UrlToArrayBuffer(publicKey.challenge),
      allowCredentials:
        publicKey.allowCredentials?.map((c: any) => ({
          ...c,
          id: base64UrlToArrayBuffer(c.id)
        })) || []
    }
  })) as PublicKeyCredential

  const assertion = credential.response as AuthenticatorAssertionResponse
  return {
    id: credential.id,
    rawId: arrayBufferToBase64Url(credential.rawId),
    type: credential.type,
    response: {
      clientDataJSON: arrayBufferToBase64Url(assertion.clientDataJSON),
      authenticatorData: arrayBufferToBase64Url(assertion.authenticatorData),
      signature: arrayBufferToBase64Url(assertion.signature),
      userHandle: assertion.userHandle ? arrayBufferToBase64Url(assertion.userHandle) : null
    }
  }
}

/**
 * 通行密钥「注册」：用后端下发的 publicKey 选项调用 navigator.credentials.create，
 * 把证明（attestation）结果序列化为可提交给 /passkey/finishRegistration 的凭证。
 */

export async function createPasskeyCredential(publicKey: any): Promise<SerializedCredential> {
  const credential = (await navigator.credentials.create({
    publicKey: {
      ...publicKey,
      challenge: base64UrlToArrayBuffer(publicKey.challenge),
      user: {
        ...publicKey.user,
        id: base64UrlToArrayBuffer(publicKey.user.id)
      },
      excludeCredentials:
        publicKey.excludeCredentials?.map((c: any) => ({
          ...c,
          id: base64UrlToArrayBuffer(c.id)
        })) || []
    }
  })) as PublicKeyCredential

  const attestation = credential.response as AuthenticatorAttestationResponse
  return {
    id: credential.id,
    rawId: arrayBufferToBase64Url(credential.rawId),
    type: credential.type,
    response: {
      clientDataJSON: arrayBufferToBase64Url(attestation.clientDataJSON),
      attestationObject: arrayBufferToBase64Url(attestation.attestationObject)
    }
  }
}
