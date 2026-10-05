<template>
  <div>
    <div class="page-head">
      <h2>藏品目录</h2>
      <div class="sub">浏览馆藏图片，选择藏品提交使用授权申请</div>
    </div>

    <div class="panel filters">
      <label class="field" style="flex:1; min-width:240px">
        <b>检索</b>
        <input v-model="keyword" placeholder="按名称 / 编号 / 朝代检索" @keyup.enter="load" />
      </label>
      <button class="btn btn-primary" @click="load">检索</button>
      <button v-if="store.isCurator" class="btn btn-accent" @click="showUpload = true">＋ 录入藏品</button>
    </div>

    <div v-if="error" class="alert alert-error">{{ error }}</div>

    <div class="art-grid">
      <div v-for="a in artifacts" :key="a.id" class="art-card">
        <img :src="imgUrl(a.id)" :alt="a.name" />
        <div class="body">
          <div class="name">{{ a.name }}</div>
          <div class="code">{{ a.code }} · {{ a.dynasty }} · {{ a.category }}</div>
          <div class="muted small" style="min-height:38px">{{ a.description }}</div>
          <div class="muted small">原图 {{ a.original_w }}×{{ a.original_h }} px</div>
          <div class="actions">
            <button v-if="!store.isCurator" class="btn btn-primary btn-sm" @click="openApply(a)">申请使用</button>
            <router-link v-else class="btn btn-sm" :to="'/review?artifact=' + a.id">相关申请</router-link>
          </div>
        </div>
      </div>
      <div v-if="!artifacts.length" class="muted" style="grid-column:1/-1; text-align:center; padding:40px">
        暂无藏品
      </div>
    </div>

    <!-- 申请弹窗 -->
    <div v-if="target" class="modal-mask" @click.self="target = null">
      <div class="modal">
        <h3>申请使用《{{ target.name }}》</h3>
        <div v-if="applyError" class="alert alert-error">{{ applyError }}</div>
        <form class="form-grid" @submit.prevent="submitApply">
          <label class="field full">
            <b>用途说明 *</b>
            <textarea v-model="form.purpose" placeholder="例如：学术期刊第 3 期配图、展览导览网页……" required></textarea>
          </label>
          <label class="field">
            <b>使用渠道 *</b>
            <select v-model="form.channel" required>
              <option value="">请选择</option>
              <option>纸质刊物</option>
              <option>电子刊物</option>
              <option>官方网页</option>
              <option>社交媒体</option>
              <option>展览展示</option>
              <option>其他</option>
            </select>
          </label>
          <label class="field">
            <b>申请分辨率 *</b>
            <select v-model="form.requested_tier">
              <option value="preview">仅预览图（≤1200px）</option>
              <option value="original">需要原图</option>
            </select>
          </label>
          <label class="field">
            <b>授权开始</b>
            <input type="datetime-local" v-model="form.start_at" />
          </label>
          <label class="field">
            <b>期望到期时间 *</b>
            <input type="datetime-local" v-model="form.end_at" required />
          </label>
          <label class="field full checkbox">
            <input type="checkbox" v-model="form.requested_edit" />
            <span>需要对图片进行二次编辑（裁剪、调色、合成等）</span>
          </label>
          <div class="modal-actions full">
            <button type="button" class="btn" @click="target = null">取消</button>
            <button class="btn btn-primary" :disabled="submitting">{{ submitting ? '提交中…' : '提交申请' }}</button>
          </div>
        </form>
      </div>
    </div>

    <!-- 馆方录入弹窗 -->
    <div v-if="showUpload" class="modal-mask" @click.self="showUpload = false">
      <div class="modal">
        <h3>录入新藏品</h3>
        <div v-if="applyError" class="alert alert-error">{{ applyError }}</div>
        <form class="form-grid" @submit.prevent="uploadArtifact">
          <label class="field"><b>编号 *</b><input v-model="up.code" required /></label>
          <label class="field"><b>名称 *</b><input v-model="up.name" required /></label>
          <label class="field"><b>朝代</b><input v-model="up.dynasty" /></label>
          <label class="field"><b>类别</b><input v-model="up.category" /></label>
          <label class="field full"><b>描述</b><textarea v-model="up.description"></textarea></label>
          <label class="field full"><b>图片文件（JPG/PNG）*</b>
            <input type="file" accept="image/jpeg,image/png" @change="onFile" required />
          </label>
          <div class="modal-actions full">
            <button type="button" class="btn" @click="showUpload = false">取消</button>
            <button class="btn btn-primary" :disabled="submitting">上传并保存</button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api.js'
import { store } from '../store.js'
import { toast } from '../toast.js'
import { toLocalInput } from '../labels.js'

const artifacts = ref([])
const keyword = ref('')
const error = ref('')
const target = ref(null)
const showUpload = ref(false)
const submitting = ref(false)
const applyError = ref('')

const blankForm = () => ({
  purpose: '',
  channel: '纸质刊物',
  requested_tier: 'preview',
  requested_edit: false,
  start_at: toLocalInput(new Date()),
  end_at: toLocalInput(new Date(Date.now() + 90 * 864e5))
})
const form = ref(blankForm())
const up = ref({ code: '', name: '', dynasty: '', category: '', description: '' })
let file = null

function imgUrl(id) {
  return `/api/artifacts/${id}/image?token=${encodeURIComponent(store.token)}`
}

async function load() {
  error.value = ''
  try {
    const q = keyword.value ? `?keyword=${encodeURIComponent(keyword.value)}` : ''
    const data = await api.get('/api/artifacts' + q)
    artifacts.value = data.items
  } catch (e) {
    error.value = e.message
  }
}

function openApply(a) {
  target.value = a
  applyError.value = ''
  form.value = blankForm()
}

async function submitApply() {
  applyError.value = ''
  submitting.value = true
  try {
    await api.post('/api/applications', {
      artifact_id: target.value.id,
      ...form.value
    })
    target.value = null
    toast('申请已提交，等待馆方审批', 'ok')
  } catch (e) {
    applyError.value = e.message
  } finally {
    submitting.value = false
  }
}

function onFile(e) {
  file = e.target.files[0] || null
}

async function uploadArtifact() {
  applyError.value = ''
  if (!file) { applyError.value = '请选择图片'; return }
  submitting.value = true
  try {
    const fd = new FormData()
    Object.entries(up.value).forEach(([k, v]) => fd.append(k, v))
    fd.append('file', file)
    const resp = await fetch('/api/artifacts', {
      method: 'POST',
      headers: { Authorization: `Bearer ${store.token}` },
      body: fd
    })
    if (!resp.ok) {
      const j = await resp.json().catch(() => ({}))
      throw new Error(j.error || '上传失败')
    }
    showUpload.value = false
    up.value = { code: '', name: '', dynasty: '', category: '', description: '' }
    toast('藏品已录入，预览图已生成', 'ok')
    load()
  } catch (e) {
    applyError.value = e.message
  } finally {
    submitting.value = false
  }
}

onMounted(load)
</script>
