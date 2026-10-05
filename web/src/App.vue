<template>
  <div v-if="!isLogin">
    <header class="topbar">
      <div class="logo">🏛 藏品图片授权系统</div>
      <nav>
        <router-link to="/artifacts">藏品目录</router-link>
        <router-link to="/my">{{ store.isCurator ? '全部授权' : '我的授权' }}</router-link>
        <router-link v-if="store.isCurator" to="/review">审批队列</router-link>
        <router-link to="/publications">发表位置</router-link>
        <router-link to="/emails">邮件提醒</router-link>
      </nav>
      <div class="userbox">
        <span>{{ store.user?.username }}</span>
        <span class="role-tag">{{ store.isCurator ? '馆方审批人' : '申请人' }}</span>
        <button class="btn-link" @click="logout">退出</button>
      </div>
    </header>
    <main class="page">
      <router-view />
    </main>
  </div>
  <router-view v-else />

  <div :class="['toast', { show: toastState.show, err: toastState.kind === 'err', ok: toastState.kind === 'ok' }]">
    {{ toastState.message }}
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { store } from './store.js'
import { toastState } from './toast.js'

const route = useRoute()
const router = useRouter()
const isLogin = computed(() => route.path === '/login' || !store.token)

function logout() {
  store.clear()
  router.push('/login')
}
</script>
