// 轻量 toast（产品 UI 无组件库场景）：模块级响应式队列 + 定时自清理。
import { reactive } from 'vue'

export type ToastKind = 'success' | 'error' | 'info'
export interface ToastItem { id: number; text: string; kind: ToastKind }

export const toasts = reactive<ToastItem[]>([])

let seq = 1

export function showToast(text: string, kind: ToastKind = 'info'): void {
  const id = seq++
  toasts.push({ id, text, kind })
  setTimeout(() => dismissToast(id), 3400)
}

export function dismissToast(id: number): void {
  const i = toasts.findIndex(t => t.id === id)
  if (i >= 0) toasts.splice(i, 1)
}
