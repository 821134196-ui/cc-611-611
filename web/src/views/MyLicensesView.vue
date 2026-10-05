<template>
  <div>
    <div class="page-head">
      <h2>{{ store.isCurator ? '全部授权' : '我的授权' }}</h2>
      <div class="sub">
        清晰区分 <span class="badge badge-pending">待审批</span>
        <span class="badge badge-approved">可下载</span>
        <span class="badge badge-dead">已失效</span>
        ——同一图片的多次授权分别管理期限
      </div>
    </div>

    <div v-if="error" class="alert alert-error">{{ error }}</div>

    <div class="zone-cols">
      <!-- 待审批 -->
      <section class="zone zone-pending">
        <header class="zone-header">
          <span>⏳ 待审批</span>
          <span class="badge badge-pending">{{ pending.length }}</span>
        </header>
        <div class="zone-body">
          <article v-for="a in pending" :key="a.id" class="lic-card">
            <div class="row">
              <span class="title">{{ a.artifact?.name || ('藏品#' + a.artifact_id) }}</span>
              <span class="spacer"></span>
              <span class="badge" :class="tierInfo(a.requested_tier).cls">{{ tierInfo(a.requested_tier).label }}</span>
            </div>
            <div class="meta">用途：{{ a.purpose }}</div>
            <div class="meta">渠道：{{ a.channel }}</div>
            <div class="meta">申请人：{{ a.applicant?.username || '我' }} · 期望到期 {{ fmtDate(a.end_at) }}</div>
            <div class="meta" v-if="a.requested_edit">申请二次编辑：是</div>
            <div class="row mt8">
              <button class="btn btn-sm" @click="openDetail(a)">详情</button>
              <button v-if="store.isCurator" class="btn btn-primary btn-sm" @click="$router.push('/review')">去审批</button>
            </div>
          </article>
          <div v-if="!pending.length" class="zone-empty">暂无待审批申请</div>
        </div>
      </section>

      <!-- 可下载 -->
      <section class="zone zone-approved">
        <header class="zone-header">
          <span>✅ 可下载（授权有效）</span>
          <span class="badge badge-approved">{{ approved.length }}</span>
        </header>
        <div class="zone-body">
          <article v-for="a in approved" :key="a.id" class="lic-card">
            <div class="row">
              <span class="title">{{ a.artifact?.name || ('藏品#' + a.artifact_id) }}</span>
              <span class="spacer"></span>
              <span class="badge" :class="tierInfo(a.resolution_tier).cls">
                {{ tierInfo(a.resolution_tier).label }}
              </span>
            </div>
            <div class="meta">用途：{{ a.purpose }} · {{ a.channel }}</div>
            <div class="meta">到期：<b>{{ fmtTime(a.expires_at) }}</b></div>
            <div class="meta">二次编辑：{{ a.allow_edit ? '允许' : '不允许' }}</div>
            <div class="meta" v-if="pubsOf(a.id).length">
              已登记发表位置 {{ pubsOf(a.id).length }} 处
            </div>
            <div class="row mt8">
              <button class="btn btn-ok btn-sm" @click="download(a, 'preview')">下载预览图</button>
              <button v-if="a.resolution_tier === 'original'" class="btn btn-accent btn-sm" @click="download(a, 'original')">
                下载原图
              </button>
              <button v-else class="btn btn-sm" disabled title="本次授权未获准原图">原图未授权</button>
            </div>
            <div class="row">
              <button class="btn btn-sm" @click="openPub(a)">登记/查看发表位置</button>
              <button v-if="store.isCurator" class="btn btn-danger btn-sm" @click="openRevoke(a)">撤回授权</button>
            </div>
          </article>
          <div v-if="!approved.length" class="zone-empty">暂无有效授权</div>
        </div>
      </section>

      <!-- 已失效 -->
      <section class="zone zone-dead">
        <header class="zone-header">
          <span>🚫 已失效（历史保留）</span>
          <span class="badge badge-dead">{{ dead.length }}</span>
        </header>
        <div class="zone-body">
          <article v-for="a in dead" :key="a.id" class="lic-card dead">
            <div class="row">
              <span class="title">{{ a.artifact?.name || ('藏品#' + a.artifact_id) }}</span>
              <span class="spacer"></span>
              <span class="badge badge-dead">{{ statusInfo(effective(a)).label }}</span>
            </div>
            <div class="meta">用途：{{ a.purpose }} · {{ a.channel }}</div>
            <div class="meta" v-if="a.status === 'expired' || effective(a) === 'expired'">到期：{{ fmtTime(a.expires_at) }}</div>
            <div class="meta" v-if="a.status === 'revoked'">撤回时间：{{ fmtTime(a.revoked_at) }}<br />原因：{{ a.revoke_reason || '—' }}</div>
            <div class="meta" v-if="a.status === 'rejected'">驳回说明：{{ a.review_note || '—' }}</div>
            <div class="meta" v-if="pubsOf(a.id).length" style="color:var(--dead)">
              ⚠ 仍有 {{ pubsOf(a.id).length }} 处已发表引用，馆方可据此联系处理
            </div>
            <div class="row mt8">
              <button class="btn btn-sm" :disabled="true">下载已停止</button>
              <button class="btn btn-sm" @click="openPub(a)">查看发表位置</button>
            </div>
          </article>
          <div v-if="!dead.length" class="zone-empty">暂无失效授权</div>
        </div>
      </section>
    </div>

    <!-- 详情 -->
    <div v-if="detail" class="modal-mask" @click.self="detail = null">
      <div class="modal">
        <h3>申请 #{{ detail.id }} 详情</h3>
        <div class="table-wrap">
          <table class="grid">
            <tbody>
              <tr><td>藏品</td><td>{{ detail.artifact?.name }}（{{ detail.artifact?.code }}）</td></tr>
              <tr><td>申请人</td><td>{{ detail.applicant?.username }}</td></tr>
              <tr><td>用途 / 渠道</td><td>{{ detail.purpose }} / {{ detail.channel }}</td></tr>
              <tr><td>申请档位 / 编辑</td><td>{{ tierInfo(detail.requested_tier).label }} / {{ detail.requested_edit ? '需编辑' : '不需编辑' }}</td></tr>
              <tr><td>期望期限</td><td>{{ fmtDate(detail.start_at) }} ~ {{ fmtDate(detail.end_at) }}</td></tr>
              <tr><td>当前状态</td><td><span class="badge" :class="statusInfo(effective(detail)).cls">{{ statusInfo(effective(detail)).label }}</span></td></tr>
              <tr v-if="detail.status === 'approved' || effective(detail) === 'expired'">
                <td>核定结果</td>
                <td>{{ tierInfo(detail.resolution_tier).label }}，{{ detail.allow_edit ? '允许' : '不允许' }}二次编辑，到期 {{ fmtTime(detail.expires_at) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="modal-actions">
          <button class="btn" @click="detail = null">关闭</button>
        </div>
      </div>
    </div>

    <!-- 撤回 -->
    <div v-if="revokeTarget" class="modal-mask" @click.self="revokeTarget = null">
      <div class="modal">
        <h3>撤回授权 #{{ revokeTarget.id }}</h3>
        <p class="muted">撤回后该授权的原图与预览下载链接将<b>立即停止</b>，历史申请和已发表位置继续保留。</p>
        <label class="field"><b>撤回原因</b>
          <textarea v-model="revokeReason" placeholder="将随提醒邮件通知申请人"></textarea>
        </label>
        <div class="modal-actions">
          <button class="btn" @click="revokeTarget = null">取消</button>
          <button class="btn btn-danger" :disabled="busy" @click="confirmRevoke">{{ busy ? '处理中…' : '确认撤回' }}</button>
        </div>
      </div>
    </div>

    <!-- 发表位置 -->
    <div v-if="pubTarget" class="modal-mask" @click.self="pubTarget = null">
      <div class="modal" style="width:680px">
        <h3>发表位置登记 · {{ pubTarget.artifact?.name }}</h3>
        <p class="muted small">获准下载不等于已经发表。实际发表后请登记刊物条目或页面地址，撤回/过期后仍可在此查看。</p>

        <form class="form-grid" @submit.prevent="addPub">
          <label class="field"><b>引用标题 *</b><input v-model="pubForm.title" placeholder="例：典藏研究 2026 第3期 P12" required /></label>
          <label class="field"><b>发表日期</b><input type="date" v-model="pubForm.published_at" /></label>
          <label class="field full"><b>刊物 / 页面地址（URL）*</b>
            <input v-model="pubForm.url" placeholder="https://…" required />
          </label>
          <label class="field full"><b>备注</b><input v-model="pubForm.note" /></label>
          <div class="modal-actions full">
            <button class="btn btn-primary" :disabled="busy">{{ busy ? '提交中…' : '登记位置' }}</button>
          </div>
        </form>

        <div class="mt16">
          <div v-if="pubError" class="alert alert-error">{{ pubError }}</div>
          <div v-if="!pubList.length" class="muted small">尚未登记发表位置</div>
          <table v-else class="grid">
            <thead><tr><th>标题</th><th>地址</th><th>发表日期</th></tr></thead>
            <tbody>
              <tr v-for="p in pubList" :key="p.id">
                <td>{{ p.title }}<div v-if="p.note" class="muted small">{{ p.note }}</div></td>
                <td><a class="ext" :href="p.url" target="_blank" rel="noopener">{{ p.url }}</a></td>
                <td>{{ fmtDate(p.published_at) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="modal-actions"><button class="btn" @click="pubTarget = null">关闭</button></div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { api } from '../api.js'
import { store } from '../store.js'
import { toast } from '../toast.js'
import { statusInfo, tierInfo, fmtTime, fmtDate } from '../labels.js'

const items = ref([])
const error = ref('')
const busy = ref(false)
const detail = ref(null)
const revokeTarget = ref(null)
const revokeReason = ref('')
const pubTarget = ref(null)
const pubList = ref([])
const pubError = ref('')
const pubForm = ref({ title: '', url: '', published_at: '', note: '' })
const pubCache = ref({}) // appID -> publications

const effective = (a) => a.effective_status || a.status

const pending = computed(() => items.value.filter((a) => effective(a) === 'pending'))
const approved = computed(() => items.value.filter((a) => effective(a) === 'approved'))
const dead = computed(() =>
  items.value.filter((a) => ['revoked', 'expired', 'rejected'].includes(effective(a)))
)

function pubsOf(id) {
  return pubCache.value[id] || []
}

async function load() {
  error.value = ''
  try {
    const data = await api.get('/api/applications')
    items.value = data.items
    // 预取每条申请的发表位置计数（已缓存的不重复拉）
    await Promise.all(data.items.map(async (a) => {
      if (!pubCache.value[a.id]) {
        try {
          const r = await api.get(`/api/applications/${a.id}/publications`)
          pubCache.value[a.id] = r.items
        } catch { /* ignore */ }
      }
    }))
  } catch (e) {
    error.value = e.message
  }
}

async function download(a, resolution) {
  try {
    await api.download(`/api/applications/${a.id}/download?resolution=${resolution}`)
    toast(`${resolution === 'original' ? '原图' : '预览图'}下载已开始`, 'ok')
  } catch (e) {
    toast(e.message, 'err')
    if (e.status === 410 || e.status === 403) load() // 状态可能已被馆方变更
  }
}

function openDetail(a) { detail.value = a }

function openRevoke(a) {
  revokeTarget.value = a
  revokeReason.value = ''
}

async function confirmRevoke() {
  busy.value = true
  try {
    await api.post(`/api/applications/${revokeTarget.value.id}/revoke`, { reason: revokeReason.value })
    toast('授权已撤回，下载链接即刻失效', 'ok')
    revokeTarget.value = null
    await load()
  } catch (e) {
    toast(e.status === 409 ? '该授权状态已被其他操作改变，请刷新' : e.message, 'err')
    await load()
  } finally {
    busy.value = false
  }
}

async function openPub(a) {
  pubTarget.value = a
  pubError.value = ''
  pubForm.value = { title: '', url: '', published_at: '', note: '' }
  const r = await api.get(`/api/applications/${a.id}/publications`)
  pubList.value = r.items
  pubCache.value[a.id] = r.items
}

async function addPub() {
  pubError.value = ''
  busy.value = true
  try {
    await api.post(`/api/applications/${pubTarget.value.id}/publications`, { ...pubForm.value })
    toast('发表位置已登记', 'ok')
    pubForm.value = { title: '', url: '', published_at: '', note: '' }
    const r = await api.get(`/api/applications/${pubTarget.value.id}/publications`)
    pubList.value = r.items
    pubCache.value[pubTarget.value.id] = r.items
  } catch (e) {
    pubError.value = e.message
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>
