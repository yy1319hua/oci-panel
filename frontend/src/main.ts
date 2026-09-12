import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { MotionPlugin } from '@vueuse/motion'
import router from './router'
import App from './App.vue'
import { useAuthStore } from './stores/auth'
import { configureApiAuth } from './lib/api'
import './assets/index.css'

const app = createApp(App)

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
