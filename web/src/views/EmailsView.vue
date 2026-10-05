<template>
  <div>
    <div class="page-head">
      <h2>邮件提醒</h2>
      <div class="sub">模拟邮件接口：审批结果、授权撤回与到期提醒均在此留档，不实际发送 SMTP</div>
    </div>

    <div v-if="error" class="alert alert-error">{{ error }}</div>

    <div v-if="!emails.length" class="panel muted" style="text-align:center; padding:36px">
      暂无提醒邮件
    </div>

    <div v-for="m in emails" :key="m.id" class="email-item">
      <div class="head">
        <div>
          <span class="email-kind">{{ kindLabel(m.kind) }}</span>
          <span class="subj">{{ m.subject }}</span>
        </div>
        <div class="muted small">{{ fmtTime(m.created_at) }}</div>
      </div>
      <div v-if="store.isCurator" class="muted small mt8">收件人：{{ m.to_address || ('user#' + m.to_user_id) }}</div>
      <div class="body">{{ m.body }}</div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api.js'
import { store } from '../store.js'
import { fmtTime } from '../labels.js'

const emails = ref([])
const error = ref('')

const KIND = {
  approved: '授权通过',
  rejected: '授权驳回',
  revoked: '授权撤回',
  expired: '授权到期'
}
const kindLabel = (k) => KIND[k] || k

onMounted(async () => {
  try {
    const data = await api.get('/api/emails')
    emails.value = data.items
  } catch (e) {
    error.value = e.message
  }
})
</script>
