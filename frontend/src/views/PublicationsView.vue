<script setup>
import { ref, onMounted, inject } from 'vue'
import { api, STATUS, CHANNELS, fmtDate } from '../api.js'

const toast = inject('toast')
const items = ref([])
const q = ref('')
const isStaff = ref(false)

async function load() {
  const [{ items: list }, { user }] = await Promise.all([
    api.get('/api/publications?q=' + encodeURIComponent(q.value)),
    api.get('/api/me'),
  ])
  items.value = list
  isStaff.value = user?.role === 'staff'
}
onMounted(load)

function exportCSV() {
  // 浏览器直接下载导出文件（含全部已登记位置，即使授权已撤回/过期）
  window.location.href = '/api/publications/export.csv'
}

function statusMeta(s) { return STATUS[s] || { label: s, cls: '' } }
</script>

<template>
  <div class="card">
    <h2>{{ isStaff ? '全部引用位置（撤回/过期后仍保留，可导出联系使用方）' : '我的发表登记' }}</h2>
    <div class="sub">获准下载不等于已经发表；申请人发表后登记的刊物或页面地址在此汇总。</div>
    <div class="toolbar">
      <input v-model="q" placeholder="按网址 / 刊物名 / 出版方搜索" @keyup.enter="load" />
      <button class="btn secondary" @click="load">搜索</button>
      <span style="flex:1"></span>
      <button v-if="isStaff" class="btn" @click="exportCSV">⬇ 导出 CSV（供联系使用方）</button>
    </div>

    <table v-if="items.length">
      <thead>
        <tr><th>发表时间</th><th>藏品</th><th>刊物 / 页面</th><th>地址</th><th v-if="isStaff">申请人/联系方式</th><th>授权状态</th></tr>
      </thead>
      <tbody>
        <tr v-for="p in items" :key="p.id">
          <td class="muted" style="white-space:nowrap">{{ fmtDate(p.publishedAt) }}</td>
          <td>
            <b>{{ p.application?.image?.title }}</b>
            <div class="muted">{{ p.application?.image?.accessionNo }}</div>
          </td>
          <td>
            {{ p.title }}
            <div class="muted">{{ p.publisher }} · {{ CHANNELS[p.channel] || p.channel }}</div>
            <div v-if="p.note" class="muted">{{ p.note }}</div>
          </td>
          <td style="max-width:300px; word-break:break-all"><a :href="p.url" target="_blank" rel="noopener">{{ p.url }}</a></td>
          <td v-if="isStaff">
            {{ p.application?.applicant?.name }}
            <div class="muted">{{ p.application?.applicant?.email }}</div>
          </td>
          <td><span class="badge" :class="statusMeta(p.application?.effectiveStatus)?.cls">{{ statusMeta(p.application?.effectiveStatus)?.label }}</span></td>
        </tr>
      </tbody>
    </table>
    <div v-else class="empty">暂无登记记录</div>
  </div>
</template>
