import { get, post } from './http'

/** Telegram 通知配置。 */
export interface TelegramConfig {
  botToken: string
  chatId: string
  apiBase: string
  enabled: boolean
  running: boolean
}

export const telegramApi = {
  getConfig: () => get<TelegramConfig>('/telegram/getConfig'),
  updateConfig: (req: { botToken: string; chatId: string; apiBase: string; enabled: boolean }) =>
    post('/telegram/updateConfig', req),
  testConnection: () => post('/telegram/testConnection', {}),
  sendTestMessage: () => post('/telegram/sendTestMessage', {}),
  startBot: () => post('/telegram/startBot', {}),
  stopBot: () => post('/telegram/stopBot', {})
}
