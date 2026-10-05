// 轻量全局 toast。
import { reactive } from 'vue'

export const toastState = reactive({ show: false, message: '', kind: '', timer: null })

export function toast(message, kind = '') {
  toastState.message = message
  toastState.kind = kind
  toastState.show = true
  if (toastState.timer) clearTimeout(toastState.timer)
  toastState.timer = setTimeout(() => { toastState.show = false }, 3200)
}
