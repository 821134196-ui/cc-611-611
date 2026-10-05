// 极简全局状态：当前登录用户与 token。
import { reactive } from 'vue'

export const store = reactive({
  token: localStorage.getItem('ml_token') || '',
  user: JSON.parse(localStorage.getItem('ml_user') || 'null'),

  setSession(token, user) {
    this.token = token
    this.user = user
    localStorage.setItem('ml_token', token)
    localStorage.setItem('ml_user', JSON.stringify(user))
  },
  clear() {
    this.token = ''
    this.user = null
    localStorage.removeItem('ml_token')
    localStorage.removeItem('ml_user')
  },
  get isCurator() {
    return this.user?.role === 'curator'
  }
})
