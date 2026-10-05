<template>
  <div>
    <div class="page-head">
      <h2>已发表引用位置</h2>
      <div class="sub">
        即使授权已撤回或过期，登记过的引用位置仍完整保留，便于联系使用方处理
      </div>
    </div>

    <div class="panel filters">
      <label class="field"><b>关键字</b>
        <input v-model="f.keyword" placeholder="标题 / 页面地址" @keyup.enter="load" />
      </label>
      <label v-if="store.isCurator" class="field"><b>申请人</b>
        <input v-model="f.applicant" placeholder="用户名" @keyup.enter="load" />
      </label>
      <label v-if="store.isCurator" class="field"><b>藏品编号</b>
        <input v-model="f.artifact_id" placeholder="如 A001" @keyup.enter="load" />
      </label>
      <button class="btn btn-primary" @click="load">检索</button>
      <button v-if="store.isCurator" class="btn btn-accent" @click="exportCsv">导出 CSV</button>
    </div>

    <div v-if="store.isCurator" class="alert alert-ok" style="background:var(--warn-bg); color:var(--warn); border-color:#ecd9b5">
      列表默认包含 <b>已撤回 / 已过期</b> 授权下仍挂在外部刊物或网页上的引用，状态列以红色标注。
    </div>

    <div v-if="error" class="alert alert-error">{{ error }}</div>

    <div class="panel table-wrap">
      <table class="grid">
        <thead>
          <tr>
            <th>藏品</th><th v-if="store.isCurator">申请人</th><th>引用标题</th><th>发表位置</th>
            <th>授权用途/渠道</th><th>授权状态</th><th>发表日期</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in rows" :key="p.id">
            <td>
              {{ p.application?.artifact?.code || p.artifact?.code }}
              <div class="muted small">{{ p.application?.artifact?.name || p.artifact?.name }}</div>
            </td>
            <td v-if="store.isCurator">{{ p.application?.applicant?.username }}</td>
            <td>{{ p.title }}<div v-if="p.note" class="muted small">{{ p.note }}</div></td>
            <td><a class="ext" :href="p.url" target="_blank" rel="noopener">{{ p.url }}</a></td>
            <td class="small">{{ p.application?.purpose }}<div class="muted">{{ p.application?.channel }}</div></td>
            <td>
              <span class="badge" :class="stale(p.effective_status) ? 'badge-dead' : 'badge-approved'">
                {{ statusInfo(p.effective_status).label }}
              </span>
              <div v-if="stale(p.effective_status)" class="small pub-status-stale">
                {{ p.effective_status === 'revoked' ? '已撤回：' + (p.application?.revoke_reason || '') : '授权已到期' }}
              </div>
            </td>
            <td>{{ fmtDate(p.published_at) }}</td>
          </tr>
          <tr v-if="!rows.length">
            <td :colspan="store.isCurator ? 7 : 6" class="muted" style="text-align:center; padding:28px">
              暂无记录
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api.js'
import { store } from '../store.js'
import { toast } from '../toast.js'
import { statusInfo, fmtDate } from '../labels.js'

const f = ref({ keyword: '', applicant: '', artifact_id: '' })
const rows = ref([])
const error = ref('')

const stale = (s) => s === 'revoked' || s === 'expired' || s === 'rejected'

function queryString() {
  const p = new URLSearchParams()
  if (f.value.keyword) p.set('keyword', f.value.keyword)
  if (store.isCurator && f.value.applicant) p.set('applicant', f.value.applicant)
  if (store.isCurator && f.value.artifact_id) {
    p.set('artifact_code', f.value.artifact_id.trim())
  }
  const qs = p.toString()
  return qs ? '?' + qs : ''
}

async function load() {
  error.value = ''
  try {
    const data = await api.get('/api/publications' + queryString())
    rows.value = data.items
  } catch (e) {
    error.value = e.message
  }
}

async function exportCsv() {
  try {
    // 直接用带 token 的请求拿 blob
    const resp = await fetch('/api/publications/export' + queryString(), {
      headers: { Authorization: `Bearer ${store.token}` }
    })
    if (!resp.ok) throw new Error('导出失败')
    const blob = await resp.blob()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'publications.csv'
    a.click()
    URL.revokeObjectURL(url)
    toast('CSV 已导出', 'ok')
  } catch (e) {
    toast(e.message, 'err')
  }
}

onMounted(load)
</script>
