<template>
  <div class="login-wrap">
    <div class="login-card">
      <h1>藏品图片授权与引用撤回</h1>
      <div class="hint">博物馆馆藏图片使用申请 · 审核 · 下载与引用追踪平台</div>

      <div class="login-tabs">
        <button :class="{ active: mode === 'login' }" @click="mode = 'login'">登录</button>
        <button :class="{ active: mode === 'register' }" @click="mode = 'register'">注册申请人</button>
      </div>

      <div v-if="error" class="alert alert-error">{{ error }}</div>

      <form @submit.prevent="submit" class="form-grid" style="grid-template-columns:1fr">
        <label class="field">
          <b>用户名</b>
          <input v-model="username" autocomplete="username" placeholder="申请人或馆方账号" />
        </label>
        <label class="field">
          <b>密码</b>
          <input v-model="password" type="password" autocomplete="current-password" placeholder="至少 4 位" />
        </label>
        <label v-if="mode === 'register'" class="field">
          <b>邮箱（用于接收提醒）</b>
          <input v-model="email" type="email" placeholder="name@example.com" />
        </label>
        <button class="btn btn-primary" style="justify-content:center; padding:10px" :disabled="loading">
          {{ loading ? '请稍候…' : (mode === 'login' ? '登录' : '注册并进入') }}
        </button>
      </form>

      <div class="hint mt16">
        演示账号：馆方 <code>curator / 123456</code>；申请人 <code>applicant / 123456</code>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api.js'
import { store } from '../store.js'

const mode = ref('login')
const username = ref('')
const password = ref('')
const email = ref('')
const error = ref('')
const loading = ref(false)
const router = useRouter()

async function submit() {
  error.value = ''
  if (!username.value || !password.value) {
    error.value = '请输入用户名和密码'
    return
  }
  loading.value = true
  try {
    const path = mode.value === 'login' ? '/api/auth/login' : '/api/auth/register'
    const payload = { username: username.value, password: password.value }
    if (mode.value === 'register') payload.email = email.value
    const data = await api.post(path, payload)
    store.setSession(data.token, data.user)
    router.push(data.user.role === 'curator' ? '/review' : '/artifacts')
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}
</script>
