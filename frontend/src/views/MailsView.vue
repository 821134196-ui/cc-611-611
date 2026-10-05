<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api.js'

const items = ref([])
const KIND = {
  approved: '批准通知',
  rejected: '拒绝通知',
  revoked: '撤回通知',
  expiring: '到期通知',
  publication: '新发表登记',
}

onMounted(async () => {
  const { items: list } = await api.get('/api/mails')
  items.value = list
})

function fmt(t) { return new Date(t).toLocaleString('zh-CN') }
</script>

<template>
  <div class="card">
    <h2>模拟邮件记录</h2>
    <div class="sub">系统不连接真实 SMTP：审批结果、撤回、到期、发表登记等提醒在此落库留痕。</div>
    <div v-if="items.length">
      <div class="mail-item" v-for="m in items" :key="m.id">
        <div class="head">
          <span>{{ m.subject }} <span class="kind-tag">{{ KIND[m.kind] || m.kind }}</span></span>
          <span class="muted" style="white-space:nowrap">{{ fmt(m.createdAt) }}</span>
        </div>
        <div class="muted">收件人：{{ m.toEmail }}</div>
        <div class="body">{{ m.body }}</div>
      </div>
    </div>
    <div v-else class="empty">暂无邮件</div>
  </div>
</template>
