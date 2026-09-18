import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { MotionPlugin } from '@vueuse/motion'
import router from './router'
import App from './App.vue'
import { useAuthStore } from './stores/auth'
import { configureApiAuth } from './lib/api'
import { theme } from './composables/useTheme'
import './assets/index.css'

const app = createApp(App)

// 主题：接管根节点 dark 类并监听系统偏好变化
// （首屏的 dark 类由 index.html 里的内联脚本预判并打好，这里只负责后续同步）
theme.init()

const pinia = createPinia()
app.use(pinia)
const auth = useAuthStore(pinia)
configureApiAuth({
  getToken: () => auth.token,
  onUnauthorized: () => {
    auth.logout()
    window.location.href = '/login'
  }
})
app.use(router)
app.use(MotionPlugin)

app.mount('#app')
