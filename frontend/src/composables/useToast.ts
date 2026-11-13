// 轻提示：模块级状态 + App.vue 渲染 ToastHost；定时器到期自动清理。
import { reactive } from 'vue'
import { errorText } from './useApiError'

export interface Toast {
  id: number
  text: string
  kind: 'success' | 'error'
}

const state = reactive<{ toasts: Toast[] }>({ toasts: [] })
let seq = 0
const timers = new Map<number, ReturnType<typeof setTimeout>>()

function dismiss(id: number) {
  const i = state.toasts.findIndex(x => x.id === id)
  if (i >= 0) state.toasts.splice(i, 1)
  const timer = timers.get(id)
  if (timer) {
    clearTimeout(timer)
    timers.delete(id)
  }
}

export function toast(text: string, kind: Toast['kind'] = 'success') {
  const id = ++seq
  state.toasts.push({ id, text, kind })
  timers.set(id, setTimeout(() => dismiss(id), 2600))
}

/** 错误提示：自动映射 ApiError.code */
export function toastError(err: unknown) {
  toast(errorText(err), 'error')
}

export function useToast() {
  return { toasts: state.toasts, toast, toastError, dismiss }
}
