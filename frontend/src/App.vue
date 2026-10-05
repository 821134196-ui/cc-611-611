<script setup>
import { ref, onMounted, computed, provide } from 'vue'
import { api } from './api.js'
import CollectionView from './views/CollectionView.vue'
import MyGrantsView from './views/MyGrantsView.vue'
import ReviewView from './views/ReviewView.vue'
import PublicationsView from './views/PublicationsView.vue'
import MailsView from './views/MailsView.vue'

const user = ref(null)
const loading = ref(true)
const tab = ref('collection')
const toast = ref(null)

function showToast(msg, ok = true) {
  toast.value = { msg, ok }
  setTimeout(() => (toast.value = null), 2600)
}
provide('toast', showToast)

onMounted(async () => {
  try {
    const { user: u } = await api.get('/api/me')
    user.value = u
  } finally {
    loading.value = false
  }
})

async function login(username) {
  const { user: u } = await api.post('/api/auth/login', { username })
  user.value = u
  tab.value = u.role === 'staff' ? 'review' : 'grants'
}
async function logout() {
  await api.post('/api/auth/logout', {})
  user.value = null
}

const tabs = computed(() => {
  if (!user.value) return []
  if (user.value.role === 'staff') {
    return [
      { key: 'review', label: '审批与撤回' },
      { key: 'collection', label: '馆藏图片' },
      { key: 'publications', label: '发表位置' },
      { key: 'mails', label: '邮件记录' },
    ]
  }
  return [
    { key: 'grants', label: '我的授权' },
    { key: 'collection', label: '馆藏目录' },
    { key: 'publications', label: '我的发表登记' },
    { key: 'mails', label: '站内通知' },
  ]
})
</script>

<template>
  <div v-if="loading" class="container empty">加载中…</div>

  <div v-else-if="!user" class="login-wrap">
    <div class="login-card">
      <h1>馆藏图片授权管理系统</h1>
      <p class="sub">申请 · 审批 · 下载 · 引用撤回追溯</p>
      <form @submit.prevent="(($event) => { const f = new FormData($event.target); login(f.get('username')).catch(e => showToast(e.message, false)) })">
        <label>用户名</label>
        <input name="username" placeholder="演示账号：curator / alice / bob" required />
        <div style="margin-top:16px"><button class="btn" style="width:100%">登 录</button></div>
      </form>
      <div class="quick">
        快速登录：
        <button @click="login('curator').catch(e=>showToast(e.message,false))">curator 馆方</button>
        <button @click="login('alice').catch(e=>showToast(e.message,false))">alice 申请人</button>
        <button @click="login('bob').catch(e=>showToast(e.message,false))">bob 申请人</button>
      </div>
    </div>
  </div>

  <div v-else>
    <header class="topbar">
      <div class="logo">🏛 馆藏图片授权<small>Museum Image Licensing</small></div>
      <nav>
        <button v-for="t in tabs" :key="t.key" :class="{ active: tab === t.key }" @click="tab = t.key">{{ t.label }}</button>
      </nav>
      <div class="who">
        {{ user.name }}（{{ user.role === 'staff' ? '馆方' : '申请人' }}）
        <button @click="logout">退出</button>
      </div>
    </header>

    <main class="container">
      <CollectionView v-if="tab === 'collection'" />
      <MyGrantsView v-else-if="tab === 'grants'" @go-collection="tab = 'collection'" />
      <ReviewView v-else-if="tab === 'review'" />
      <PublicationsView v-else-if="tab === 'publications'" />
      <MailsView v-else-if="tab === 'mails'" />
    </main>
  </div>

  <div v-if="toast" class="toast" :class="toast.ok ? 'ok' : 'err'">{{ toast.msg }}</div>
</template>
