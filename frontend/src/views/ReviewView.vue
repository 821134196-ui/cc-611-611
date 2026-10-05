<script setup>
import { ref, onMounted, inject, computed } from 'vue'
import { api, STATUS, LEVELS, CHANNELS, fmtDate } from '../api.js'

const toast = inject('toast')
const apps = ref([])
const filter = ref('pending')
const reviewFor = ref(null)
const form = ref({ approve: true, maxResolution: 2, canEdit: false, expireAt: '', note: '' })

async function load() {
  const { items } = await api.get('/api/applications?scope=' + filter.value)
  apps.value = items
}
onMounted(load)

const counts = ref({ pending: 0 })
async function loadCounts() {
  const { items } = await api.get('/api/applications?scope=pending')
  counts.value.pending = items.length
}
onMounted(loadCounts)

function openReview(app, approve) {
  reviewFor.value = app
  form.value = {
    approve,
    maxResolution: app.maxResolution || 2,
    canEdit: false,
    expireAt: fmtDate(app.expireAt),
    note: '',
  }
}

async function submitReview() {
  try {
    await api.post(`/api/applications/${reviewFor.value.id}/review`, { ...form.value })
    toast(form.value.approve ? '已批准，申请人将收到通知（模拟邮件）' : '已拒绝并通知申请人')
    reviewFor.value = null
    await Promise.all([load(), loadCounts()])
  } catch (e) {
    // 并发冲突时后端返回 409 + 当前状态
    toast(e.message, false)
    await Promise.all([load(), loadCounts()])
  }
}

async function revoke(app) {
  const reason = prompt(`撤回《${app.image?.title}》对 ${app.applicant?.name} 的授权。\n撤回后旧下载链接立即停止提供原图（历史记录与发表位置保留）。\n请填写撤回原因：`)
  if (reason === null) return
  try {
    await api.post(`/api/applications/${app.id}/revoke`, { reason })
    toast('授权已撤回，已通知申请人')
    load()
  } catch (e) {
    toast(e.message, false)
  }
}

function statusMeta(s) { return STATUS[s] || { label: s, cls: '' } }
</script>

<template>
  <div class="card">
    <h2>审批与撤回 <span class="badge badge-pending" v-if="counts.pending">待处理 {{ counts.pending }}</span></h2>
    <div class="sub">审批只对当前这条申请生效；同一申请被并发处理时，仅第一个审批动作成功，其余返回冲突，不会产生互相矛盾的结果。</div>
    <div class="toolbar">
      <button class="btn" :class="{ secondary: filter !== 'pending' }" @click="filter='pending'; load()">待审批</button>
      <button class="btn" :class="{ secondary: filter !== 'all' }" @click="filter='all'; load()">全部申请</button>
    </div>

    <table v-if="apps.length">
      <thead>
        <tr><th>藏品</th><th>申请人</th><th>用途 / 渠道</th><th>申请期限</th><th>当前状态/权限</th><th>操作</th></tr>
      </thead>
      <tbody>
        <tr v-for="a in apps" :key="a.id">
          <td><b>{{ a.image?.title }}</b><div class="muted">{{ a.image?.accessionNo }}</div></td>
          <td>{{ a.applicant?.name }}<div class="muted">{{ a.applicant?.email }}</div></td>
          <td>
            <div style="max-width:260px">{{ a.purpose }}</div>
            <div class="muted">{{ CHANNELS[a.channel] || a.channel }}</div>
          </td>
          <td class="muted">{{ fmtDate(a.startAt) }}<br />至 {{ fmtDate(a.expireAt) }}</td>
          <td>
            <span class="badge" :class="statusMeta(a.effectiveStatus).cls">{{ statusMeta(a.effectiveStatus).label }}</span>
            <div v-if="a.effectiveStatus === 'approved'" style="margin-top:4px">
              <span class="badge badge-level">最高 {{ LEVELS[a.maxResolution]?.short }}</span>
              <span class="badge" :class="a.canEdit ? 'badge-edit' : ''">{{ a.canEdit ? '可编辑' : '禁编辑' }}</span>
            </div>
            <div v-if="a.revokeReason" class="muted" style="margin-top:4px">撤回：{{ a.revokeReason }}</div>
          </td>
          <td style="white-space:nowrap">
            <template v-if="a.status === 'pending'">
              <button class="btn small" @click="openReview(a, true)">批准</button>
              <button class="btn small danger" style="margin-left:6px" @click="openReview(a, false)">拒绝</button>
            </template>
            <template v-else-if="a.effectiveStatus === 'approved'">
              <button class="btn small danger" @click="revoke(a)">撤回授权</button>
            </template>
            <span v-else class="muted">已处理</span>
          </td>
        </tr>
      </tbody>
    </table>
    <div v-else class="empty">没有相关申请</div>
  </div>

  <!-- 审批弹窗 -->
  <div v-if="reviewFor" class="login-wrap" style="background:rgba(40,30,20,.55)" @click.self="reviewFor = null">
    <div class="login-card" style="width:520px">
      <h1>{{ form.approve ? '批准' : '拒绝' }}申请</h1>
      <p class="sub">《{{ reviewFor.image?.title }}》 — 申请人 {{ reviewFor.applicant?.name }}（{{ reviewFor.applicant?.email }}）</p>

      <template v-if="form.approve">
        <label>允许下载的最高分辨率</label>
        <select v-model.number="form.maxResolution">
          <option :value="1">1 级 · 预览图（600px，仅可在线查看）</option>
          <option :value="2">2 级 · 网页版（1600px，网页/新媒体用图）</option>
          <option :value="3">3 级 · 原图 / 印刷版（刊物印刷）</option>
        </select>
        <label style="margin-top:12px; display:flex; align-items:center; gap:8px">
          <input type="checkbox" v-model="form.canEdit" style="width:auto" />
          允许申请人二次编辑（裁切、调色、合成等）
        </label>
        <label>授权截止日期（可调整申请期限）</label>
        <input type="date" v-model="form.expireAt" />
      </template>

      <label>{{ form.approve ? '附加条件 / 审核说明' : '拒绝理由' }}</label>
      <textarea v-model="form.note" :placeholder="form.approve ? '如：限图录一次印刷，不得转授第三方' : '如：用途材料不完整，请补充印刷数量'"></textarea>

      <div style="display:flex;gap:10px;margin-top:16px">
        <button class="btn" @click="submitReview">{{ form.approve ? '确认批准' : '确认拒绝' }}</button>
        <button class="btn secondary" @click="reviewFor = null">取消</button>
      </div>
      <p class="muted" style="margin-top:12px">提交即对该申请加锁；若同时有另一审批动作先完成，本操作将收到“已被处理”的冲突提示。</p>
    </div>
  </div>
</template>
