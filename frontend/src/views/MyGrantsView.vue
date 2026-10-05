<script setup>
import { ref, onMounted, inject, computed } from 'vue'
import { api, STATUS, LEVELS, CHANNELS, fmtDate } from '../api.js'

const toast = inject('toast')
const emit = defineEmits(['go-collection'])
const apps = ref([])
const detailId = ref(null)
const pubForm = ref({ title: '', publisher: '', url: '', channel: 'web', publishedAt: '', note: '' })
const pubFor = ref(null)

async function load() {
  const { items } = await api.get('/api/applications?scope=mine')
  apps.value = items
}
onMounted(load)

// 三个清晰分区：待审批 / 可下载 / 已失效（拒绝、撤回、过期）
const pending = computed(() => apps.value.filter(a => a.effectiveStatus === 'pending'))
const downloadable = computed(() => apps.value.filter(a => a.effectiveStatus === 'approved'))
const invalid = computed(() => apps.value.filter(a => ['rejected', 'revoked', 'expired'].includes(a.effectiveStatus)))

async function download(app, level) {
  try {
    const { url } = await api.get(`/api/applications/${app.id}/download-token?level=${level}`)
    // 直接触发浏览器下载；旧链接在撤回/过期后由后端拒绝
    window.location.href = url
    toast(`已开始下载${LEVELS[level].short}版：${app.image.title}`)
  } catch (e) {
    toast(e.message, false)
  }
}

async function toggleDetail(app) {
  if (detailId.value === app.id) { detailId.value = null; return }
  const { item } = await api.get(`/api/applications/${app.id}`)
  const idx = apps.value.findIndex(a => a.id === app.id)
  if (idx >= 0) apps.value[idx] = item
  detailId.value = app.id
}

function openPub(app) {
  pubFor.value = app
  pubForm.value = {
    title: app.image ? app.image.title : '',
    publisher: '', url: '',
    channel: app.channel || 'web',
    publishedAt: new Date().toISOString().slice(0, 10),
    note: '',
  }
}

async function submitPub() {
  if (!pubForm.value.url.trim()) return toast('请填写页面地址 URL', false)
  try {
    await api.post(`/api/applications/${pubFor.value.id}/publications`, pubForm.value)
    toast('发表位置已登记，馆方将收到通知（模拟邮件）')
    pubFor.value = null
    load()
  } catch (e) {
    toast(e.message, false)
  }
}

function statusMeta(s) { return STATUS[s] || { label: s, cls: '' } }
</script>

<template>
  <div class="card">
    <h2>我的授权</h2>
    <div class="sub">每次申请独立管理期限与权限。<b>获准下载 ≠ 已获发表</b>，正式发表后请登记引用位置。</div>
  </div>

  <!-- 待审批 -->
  <div class="section-head">⏳ 待审批 <span class="count">{{ pending.length }} 项</span></div>
  <div class="card" v-if="pending.length">
    <table>
      <thead><tr><th>藏品</th><th>用途</th><th>渠道</th><th>申请期限</th><th>状态</th></tr></thead>
      <tbody>
        <tr v-for="a in pending" :key="a.id">
          <td><b>{{ a.image?.title }}</b><div class="muted">{{ a.image?.accessionNo }}</div></td>
          <td style="max-width:280px">{{ a.purpose }}</td>
          <td>{{ CHANNELS[a.channel] || a.channel }}</td>
          <td>{{ fmtDate(a.startAt) }} 至 {{ fmtDate(a.expireAt) }}</td>
          <td><span class="badge" :class="statusMeta(a.effectiveStatus).cls">{{ statusMeta(a.effectiveStatus).label }}</span></td>
        </tr>
      </tbody>
    </table>
  </div>
  <div class="card empty" v-else>暂无待审批申请</div>

  <!-- 可下载 -->
  <div class="section-head">✅ 可下载（已批准、有效期内）<span class="count">{{ downloadable.length }} 项</span></div>
  <div class="card" v-if="downloadable.length">
    <table>
      <thead><tr><th>藏品</th><th>用途/期限</th><th>权限</th><th>下载</th><th>发表与详情</th></tr></thead>
      <tbody>
        <template v-for="a in downloadable" :key="a.id">
          <tr>
            <td><b>{{ a.image?.title }}</b><div class="muted">{{ a.image?.accessionNo }}</div></td>
            <td>
              <div style="max-width:260px">{{ a.purpose }}</div>
              <div class="muted">截止 {{ fmtDate(a.expireAt) }}</div>
            </td>
            <td>
              <span class="badge badge-level">最高 {{ LEVELS[a.maxResolution]?.short }}</span>
              <span v-if="a.canEdit" class="badge badge-edit">允许二次编辑</span>
              <span v-else class="muted">（不得二次编辑）</span>
            </td>
            <td>
              <button class="btn small secondary" @click="download(a, 1)" :disabled="a.maxResolution < 1">预览版</button>
              <button class="btn small secondary" style="margin-top:4px" @click="download(a, 2)" :disabled="a.maxResolution < 2">网页版</button>
              <button class="btn small" style="margin-top:4px" @click="download(a, 3)" :disabled="a.maxResolution < 3">原图</button>
            </td>
            <td>
              <button class="btn small" @click="openPub(a)">登记发表位置</button>
              <button class="btn small secondary" style="margin-left:6px" @click="toggleDetail(a)">详情</button>
            </td>
          </tr>
          <tr v-if="detailId === a.id">
            <td colspan="5">
              <div class="detail-box">
                <div class="row"><b>审核说明</b><span>{{ a.reviewNote || '—' }}</span></div>
                <div class="row"><b>已登记发表</b></div>
                <div v-if="a.publications?.length">
                  <table>
                    <thead><tr><th>刊物/页面</th><th>地址</th><th>发表日期</th></tr></thead>
                    <tbody>
                      <tr v-for="p in a.publications" :key="p.id">
                        <td>{{ p.title }}<div class="muted">{{ p.publisher }}</div></td>
                        <td><a :href="p.url" target="_blank" rel="noopener">{{ p.url }}</a></td>
                        <td>{{ fmtDate(p.publishedAt) }}</td>
                      </tr>
                    </tbody>
                  </table>
                </div>
                <div v-else class="muted">尚未登记。下载使用后如已正式发表，请尽快登记，以便馆方在授权撤回时可以联系到使用方。</div>
              </div>
            </td>
          </tr>
        </template>
      </tbody>
    </table>
  </div>
  <div class="card empty" v-else>暂无可下载授权，可到馆藏目录提交申请</div>

  <!-- 已失效 -->
  <div class="section-head">🚫 已失效（拒绝 / 撤回 / 过期，历史记录保留）<span class="count">{{ invalid.length }} 项</span></div>
  <div class="card" v-if="invalid.length">
    <table>
      <thead><tr><th>藏品</th><th>用途</th><th>期限</th><th>状态</th><th>说明</th><th>详情</th></tr></thead>
      <tbody>
        <tr v-for="a in invalid" :key="a.id">
          <td><b>{{ a.image?.title }}</b><div class="muted">{{ a.image?.accessionNo }}</div></td>
          <td style="max-width:240px">{{ a.purpose }}</td>
          <td class="muted">{{ fmtDate(a.startAt) }} 至 {{ fmtDate(a.expireAt) }}</td>
          <td><span class="badge" :class="statusMeta(a.effectiveStatus).cls">{{ statusMeta(a.effectiveStatus).label }}</span></td>
          <td class="muted" style="max-width:220px">
            <span v-if="a.effectiveStatus === 'revoked'">撤回原因：{{ a.revokeReason || '—' }}；旧下载链接已停止提供原图</span>
            <span v-else-if="a.effectiveStatus === 'expired'">授权已到期，旧链接失效；如需继续使用请重新申请</span>
            <span v-else>{{ a.reviewNote || '未获批准' }}</span>
          </td>
          <td><button class="btn small secondary" @click="toggleDetail(a)">查看发表记录</button></td>
        </tr>
      </tbody>
    </table>
  </div>
  <div class="card empty" v-else>暂无失效记录</div>

  <!-- 发表登记弹窗 -->
  <div v-if="pubFor" class="login-wrap" style="background:rgba(40,30,20,.55)" @click.self="pubFor = null">
    <div class="login-card" style="width:540px">
      <h1>登记发表位置</h1>
      <p class="sub">《{{ pubFor.image?.title }}》 — 这些信息在授权撤回或到期后仍会保留，供馆方追溯引用</p>
      <label>刊物名称 / 页面名称 *</label>
      <input v-model="pubForm.title" placeholder="如《馆藏精粹（2026）》第 42 页" />
      <div class="form-row">
        <div><label>出版方 / 站点</label><input v-model="pubForm.publisher" placeholder="艺文出版社" /></div>
        <div>
          <label>渠道</label>
          <select v-model="pubForm.channel">
            <option v-for="(label, key) in CHANNELS" :key="key" :value="key">{{ label }}</option>
          </select>
        </div>
      </div>
      <label>页面地址 URL *</label>
      <input v-model="pubForm.url" placeholder="https://..." />
      <label>发表日期</label>
      <input type="date" v-model="pubForm.publishedAt" />
      <label>备注（印数、版式等）</label>
      <input v-model="pubForm.note" />
      <div style="display:flex;gap:10px;margin-top:16px">
        <button class="btn" @click="submitPub">提交登记</button>
        <button class="btn secondary" @click="pubFor = null">取消</button>
      </div>
    </div>
  </div>
</template>
