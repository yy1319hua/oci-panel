import { computed, ref, watch } from 'vue'

/**
 * 主题模式
 * - system: 跟随系统 prefers-color-scheme
 * - light:  强制浅色
 * - dark:   强制深色
 */
export type ThemeMode = 'system' | 'light' | 'dark'

/** localStorage 键名。index.html 里的防闪烁内联脚本读的是同一个键，改动需同步 */
const STORAGE_KEY = 'theme-mode'

const DARK_QUERY = '(prefers-color-scheme: dark)'

function isSystemDark(): boolean {
  if (typeof window === 'undefined' || !window.matchMedia) return true
  return window.matchMedia(DARK_QUERY).matches
}

function readStoredMode(): ThemeMode {
  if (typeof localStorage === 'undefined') return 'system'
  const raw = localStorage.getItem(STORAGE_KEY)
  return raw === 'light' || raw === 'dark' || raw === 'system' ? raw : 'system'
}

const mode = ref<ThemeMode>(readStoredMode())
const systemDark = ref<boolean>(isSystemDark())

/** 当前是否实际处于深色（system 模式下由系统偏好决定） */
const isDark = computed(() => (mode.value === 'system' ? systemDark.value : mode.value === 'dark'))

let transitionTimer: number | undefined

/**
 * 把主题应用到根节点。
 * 只有在切换时才临时挂上 transition 类，避免首屏加载时元素逐个做动画。
 */
function apply(animate: boolean) {
  if (typeof document === 'undefined') return
  const root = document.documentElement
  if (animate) {
    root.classList.add('theme-transition')
    if (transitionTimer) window.clearTimeout(transitionTimer)
    transitionTimer = window.setTimeout(() => {
      root.classList.remove('theme-transition')
      transitionTimer = undefined
    }, 260)
  }
  root.classList.toggle('dark', isDark.value)
  // 给浏览器自身控件（滚动条、表单）一个提示
  root.style.colorScheme = isDark.value ? 'dark' : 'light'
}

watch(isDark, () => apply(true), { flush: 'post' })

// 仅在浏览器环境且尚未初始化时执行一次
let inited = false
function init() {
  if (inited || typeof window === 'undefined' || !window.matchMedia) return
  inited = true
  apply(false)
  const mq = window.matchMedia(DARK_QUERY)
  const onChange = (e: MediaQueryListEvent) => {
    systemDark.value = e.matches
  }
  // Safari < 14 只有 addListener
  if (mq.addEventListener) mq.addEventListener('change', onChange)
  else mq.addListener(onChange)
}

export function useTheme() {
  const setMode = (next: ThemeMode) => {
    mode.value = next
    try {
      localStorage.setItem(STORAGE_KEY, next)
    } catch {
      // 隐私模式下 localStorage 可能不可写，此时仅内存生效
    }
  }

  /** 三态循环：跟随系统 → 浅色 → 深色 → 跟随系统 */
  const toggle = () => {
    const order: ThemeMode[] = ['system', 'light', 'dark']
    const idx = order.indexOf(mode.value)
    setMode(order[(idx + 1) % order.length])
  }

  return {
    mode,
    isDark,
    setMode,
    toggle,
    init
  }
}

export const theme = useTheme()
