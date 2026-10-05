<script setup>
import { ref, onMounted, inject } from 'vue'
import { api, CHANNELS, fmtDate } from '../api.js'

const toast = inject('toast')
const images = ref([])
const q = ref('')
const showApply = ref(false)
const selected = ref(null)
const form = ref({ purpose: '', channel: 'web', startAt: '', expireAt: '' })
const submitting = ref(false)
const isStaff = ref(false)
const showUpload = ref(false)
const uploadForm = ref({ title: '', accessionNo: '', dynasty: '', category: '' })
const uploadFile = ref(null)

async function load() {
  const { items } = await api.get('/api/images?q=' + encodeURIComponent(q.value))
  images.value = items
  const { user } = await api.get('/api/me')
  isStaff.value = user?.role === 'staff'
}
onMounted(load)

function openApply(img) {
  selected.value = img
  const today = new Date().toISOString().slice(0, 10)
  const nextYear = new Date(Date.now() + 365 * 864e5).toISOString().slice(0, 10)
  form.value = { purpose: '', channel: 'web', startAt: today, expireAt: nextYear }
  showApply.value = true
}

async function submitApply() {
  if (!form.value.purpose.trim()) return toast('请填写用途说明', false)
  submitting.value = true
  try {
    await api.post('/api/applications', { imageId: selected.value.id, ...form.value })
    toast('申请已提交，请等待馆方审批')
    showApply.value = false
  } catch (e) {
    toast(e.message, false)
  } finally {
    submitting.value = false
  }
}

function onFile(e) { uploadFile.value = e.target.files[0] }

async function submitUpload() {
  if (!uploadFile.value) return toast('请选择图片文件', false)
  const fd = new FormData()
  fd.append('file', uploadFile.value)
  Object.entries(uploadForm.value).forEach(([k, v]) => fd.append(k, v))
  try {
    await api.upload('/api/images', fd)
    toast('图片已入库（已自动生成预览/网页/原图三级文件）')
    showUpload.value = false
    uploadForm.value = { title: '', accessionNo: '', dynasty: '', category: '' }
    uploadFile.value = null
    load()
  } catch (e) {
    toast(e.message, false)
  }
}
</script>

<template>
  <div class="card">
    <h2>馆藏目录</h2>
    <div class="sub">预览图对所有登录用户开放；原图（印刷级）仅在申请获批后凭授权下载。</div>
    <div class="toolbar">
      <input v-model="q" placeholder="搜索名称 / 编号 / 朝代" @keyup.enter="load" />
      <button class="btn secondary" @click="load">搜索</button>
      <span style="flex:1"></span>
      <button v-if="isStaff" class="btn secondary" @click="showUpload = true">＋ 录入新藏品图片</button>
    </div>
  </div>

  <div class="grid grid-cols-3">
    <div class="artifact" v-for="img in images" :key="img.id">
      <img :src="img.previewUrl" :alt="img.title" />
      <div class="meta">
        <div class="title">{{ img.title }}</div>
        <div class="acc">{{ img.accessionNo }} · {{ img.dynasty }} · {{ img.category }}</div>
        <div class="muted">{{ img.width }} × {{ img.height }} px</div>
        <div class="spacer"></div>
        <button v-if="!isStaff" class="btn" @click="openApply(img)">申请使用授权</button>
      </div>
    </div>
  </div>
  <div v-if="!images.length" class="card empty">没有符合条件的藏品</div>

  <!-- 申请弹窗 -->
  <div v-if="showApply" class="login-wrap" style="background:rgba(40,30,20,.55)" @click.self="showApply = false">
    <div class="login-card" style="width:520px">
      <h1>申请使用《{{ selected?.title }}》</h1>
      <p class="sub">{{ selected?.accessionNo }} · 同一藏品可分别提交多次授权，期限各自独立管理</p>
      <label>用途说明（用于什么刊物/页面、具体用法）</label>
      <textarea v-model="form.purpose" placeholder="例如：年度图录第 42 页跨页配图，首版印刷 3000 册"></textarea>
      <div class="form-row">
        <div>
          <label>使用渠道</label>
          <select v-model="form.channel">
            <option v-for="(label, key) in CHANNELS" :key="key" :value="key">{{ label }}</option>
          </select>
        </div>
      </div>
      <div class="form-row">
        <div><label>授权开始</label><input type="date" v-model="form.startAt" /></div>
        <div><label>授权截止</label><input type="date" v-model="form.expireAt" /></div>
      </div>
      <p class="muted" style="margin-top:10px">
        提示：获准下载不等于已获发表。正式发表后请到「我的授权」登记刊物或页面地址。
      </p>
      <div style="display:flex;gap:10px;margin-top:14px">
        <button class="btn" :disabled="submitting" @click="submitApply">提交申请</button>
        <button class="btn secondary" @click="showApply = false">取消</button>
      </div>
    </div>
  </div>

  <!-- 上传弹窗（馆方） -->
  <div v-if="showUpload" class="login-wrap" style="background:rgba(40,30,20,.55)" @click.self="showUpload = false">
    <div class="login-card" style="width:520px">
      <h1>录入藏品图片</h1>
      <p class="sub">系统将自动生成 600px 预览版、1600px 网页版与原图三个文件</p>
      <label>藏品编号</label><input v-model="uploadForm.accessionNo" placeholder="如 GU-0007" />
      <label>名称</label><input v-model="uploadForm.title" />
      <div class="form-row">
        <div><label>朝代</label><input v-model="uploadForm.dynasty" /></div>
        <div><label>类别</label><input v-model="uploadForm.category" /></div>
      </div>
      <label>原始图片（JPG / PNG）</label>
      <input type="file" accept="image/jpeg,image/png" @change="onFile" />
      <div style="display:flex;gap:10px;margin-top:16px">
        <button class="btn" @click="submitUpload">入库</button>
        <button class="btn secondary" @click="showUpload = false">取消</button>
      </div>
    </div>
  </div>
</template>
