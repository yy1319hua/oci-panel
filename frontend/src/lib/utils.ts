import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function formatDate(date: string | Date | undefined | null): string {
  if (!date) return '-'
  const d = typeof date === 'string' ? new Date(date) : date
  return d.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit'
  })
}

export function formatFileSize(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

export function truncateId(id: string, length: number = 20): string {
  if (!id) return '-'
  if (id.length <= length) return id
  return id.substring(0, length) + '...'
}

export function copyToClipboard(text: string): Promise<void> {
  return navigator.clipboard.writeText(text)
}

// debounce 防抖：延迟 delay 毫秒后执行 fn，期间重复调用会重置计时器。
// 用于搜索框等高频输入场景，避免每次按键都触发一次请求。
export function debounce<T extends (...args: unknown[]) => void>(fn: T, delay = 300): (...args: Parameters<T>) => void {
  let timer: ReturnType<typeof setTimeout> | undefined
  return (...args: Parameters<T>) => {
    if (timer) clearTimeout(timer)
    timer = setTimeout(() => fn(...args), delay)
  }
}
