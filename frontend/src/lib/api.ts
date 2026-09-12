import axios from 'axios'
import { toast } from '@/composables/useToast'

interface ApiAuth {
  getToken: () => string
  onUnauthorized: () => void
}

let auth: ApiAuth | undefined

// 应用入口装配认证回调，HTTP 层不再反向导入依赖 API 的 Pinia store。
export function configureApiAuth(handlers: ApiAuth) {
  auth = handlers
}

const api = axios.create({
  baseURL: '/api',
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json'
  }
})

api.interceptors.request.use(
  config => {
    const token = auth?.getToken()
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  error => {
    return Promise.reject(error)
  }
)

api.interceptors.response.use(
  response => {
    const data = response.data
    if (data.code && data.code !== 200) {
      const msg = data.message || '请求失败'
      toast.error(msg)
      return Promise.reject(new Error(msg))
    }
    return data
  },
  error => {
    if (error.response) {
      const { status, data } = error.response

      if (status === 401) {
        auth?.onUnauthorized()
        return Promise.reject(new Error('登录已过期，请重新登录'))
      }

      const msg = data.message || error.message || '请求失败'
      toast.error(msg)
      return Promise.reject(new Error(msg))
    }

    if (error.code === 'ECONNABORTED') {
      toast.error('请求超时，请稍后重试')
    } else if (!error.response) {
      toast.error('网络连接失败，请检查网络')
    }

    return Promise.reject(error)
  }
)

export default api
