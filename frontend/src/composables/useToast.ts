import { ref } from 'vue'

export interface Toast {
  id: number
  message: string
  type: 'success' | 'error' | 'warning' | 'info'
}

const toasts = ref<Toast[]>([])
let id = 0

export function useToast() {
  const show = (message: string, type: Toast['type'] = 'info', duration = 3000) => {
    const toast: Toast = {
      id: id++,
      message,
      type
    }

    toasts.value.push(toast)

    if (duration > 0) {
      setTimeout(() => {
        remove(toast.id)
      }, duration)
    }

    return toast.id
  }

  const remove = (toastId: number) => {
    const index = toasts.value.findIndex(t => t.id === toastId)
    if (index > -1) {
      toasts.value.splice(index, 1)
    }
  }

  const success = (message: string, duration?: number) => show(message, 'success', duration)
  const error = (message: string, duration?: number) => show(message, 'error', duration)
  const warning = (message: string, duration?: number) => show(message, 'warning', duration)
  const info = (message: string, duration?: number) => show(message, 'info', duration)

  return {
    toasts,
    show,
    remove,
    success,
    error,
    warning,
    info
  }
}

export const toast = useToast()
