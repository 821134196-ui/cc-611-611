<template>
  <div>
    <div class="page-head">
      <h2>审批队列</h2>
      <div class="sub">审批只对相应申请生效；若已被其他审批人处理，系统会明确提示冲突</div>
    </div>

    <div v-if="error" class="alert alert-error">{{ error }}</div>

    <div class="panel">
      <h3 style="margin-top:0">⏳ 待审批（{{ pending.length }}）</h3>
      <div v-if="!pending.length" class="muted small">没有待处理的申请</div>
      <div class="table-wrap">
        <table class="grid">
          <thead>
            <tr>
              <th>#</th><th>申请人</th><th>藏品</th><th>用途 / 渠道</th>
              <th>申请档位</th><th>编辑</th><th>期望到期</th><th>提交时间</th><th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="a in pending" :key="a.id">
              <td>{{ a.id }}</td>
              <td>{{ a.applicant?.username }}</td>
              <td>{{ a.artifact?.name }}<div class="muted small">{{ a.artifact?.code }}</div></td>
              <td>{{ a.purpose }}<div class="muted small">{{ a.channel }}</div></td>
              <td><span class="badge" :class="tierInfo(a.requested_tier).cls">{{ tierInfo(a.requested_tier).label }}</span></td>
              <td>{{ a.requested_edit ? '需要' : '不需要' }}</td>
              <td>{{ fmtDate(a.end_at) }}</td>
              <td>{{ fmtTime(a.created_at) }}</td>
              <td style="white-space:nowrap">
                <button class="btn btn-ok btn-sm" @click="openApprove(a)">批准</button>
                <button class="btn btn-danger btn-sm" @click="openReject(a)">驳回</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div class="panel">
      <h3 style="margin-top:0">已处理记录（{{ processed.length }}）</h3>
      <div class="table-wrap">
        <table class="grid">
          <thead>
            <tr><th>#</th><th>申请人</th><th>藏品</th><th>核定档位</th><th>授权到期</th><th>状态</th><th>审批人</th><th>操作</th></tr>
          </thead>
          <tbody>
            <tr v-for="a in processed" :key="a.id">
              <td>{{ a.id }}</td>
              <td>{{ a.applicant?.username }}</td>
              <td>{{ a.artifact?.name }}</td>
              <td><span class="badge" :class="tierInfo(a.resolution_tier).cls">{{ tierInfo(a.resolution_tier).label }}</span></td>
              <td>{{ fmtTime(a.expires_at) }}</td>
              <td><span class="badge" :class="statusInfo(effective(a)).cls">{{ statusInfo(effective(a)).label }}</span></td>
              <td>{{ a.reviewer_id || '—' }}</td>
              <td>
                <button v-if="effective(a) === 'approved'" class="btn btn-danger btn-sm" @click="openRevoke(a)">撤回</button>
                <span v-else class="muted small">—</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 批准弹窗 -->
    <div v-if="approveTarget" class="modal-mask" @click.self="approveTarget = null">
      <div class="modal">
        <h3>批准申请 #{{ approveTarget.id }}</h3>
        <div v-if="formError" class="alert alert-error">{{ formError }}</div>
        <div class="small muted">申请人：{{ approveTarget.applicant?.username }} · 用途：{{ approveTarget.purpose }}</div>
        <form class="form-grid mt16" @submit.prevent="confirmApprove">
          <label class="field">
            <b>允许下载的分辨率 *</b>
            <select v-model="decision.resolution_tier">
              <option value="preview">仅预览图（≤1200px）</option>
              <option value="original">原图（含预览图）</option>
            </select>
          </label>
          <label class="field">
            <b>授权到期时间 *</b>
            <input type="datetime-local" v-model="decision.expires_at" required />
          </label>
          <label class="field full checkbox">
            <input type="checkbox" v-model="decision.allow_edit" />
            <span>允许二次编辑</span>
          </label>
          <label class="field full"><b>审批备注</b><input v-model="decision.note" /></label>
          <div class="modal-actions full">
            <button type="button" class="btn" @click="approveTarget = null">取消</button>
            <button class="btn btn-ok" :disabled="busy">{{ busy ? '提交中…' : '确认批准' }}</button>
          </div>
        </form>
      </div>
    </div>

    <!-- 驳回弹窗 -->
    <div v-if="rejectTarget" class="modal-mask" @click.self="rejectTarget = null">
      <div class="modal">
        <h3>驳回申请 #{{ rejectTarget.id }}</h3>
        <label class="field"><b>驳回原因</b><textarea v-model="rejectNote"></textarea></label>
        <div class="modal-actions">
          <button class="btn" @click="rejectTarget = null">取消</button>
          <button class="btn btn-danger" :disabled="busy" @click="confirmReject">确认驳回</button>
        </div>
      </div>
    </div>

    <!-- 撤回弹窗 -->
    <div v-if="revokeTarget" class="modal-mask" @click.self="revokeTarget = null">
      <div class="modal">
        <h3>撤回授权 #{{ revokeTarget.id }}</h3>
        <p class="muted">撤回即刻生效，下载链接立即停止；历史与发表记录保留。</p>
        <label class="field"><b>撤回原因</b><textarea v-model="revokeReason"></textarea></label>
        <div class="modal-actions">
          <button class="btn" @click="revokeTarget = null">取消</button>
          <button class="btn btn-danger" :disabled="busy" @click="confirmRevoke">确认撤回</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api.js'
import { toast } from '../toast.js'
import { statusInfo, tierInfo, fmtTime, fmtDate, toLocalInput } from '../labels.js'

const route = useRoute()
const items = ref([])
const error = ref('')
const busy = ref(false)
const formError = ref('')
const approveTarget = ref(null)
const rejectTarget = ref(null)
const revokeTarget = ref(null)
const rejectNote = ref('')
const revokeReason = ref('')
const decision = ref({ resolution_tier: 'preview', expires_at: '', allow_edit: false, note: '' })

const effective = (a) => a.effective_status || a.status
const pending = computed(() => items.value.filter((a) => a.status === 'pending'))
const processed = computed(() => items.value.filter((a) => a.status !== 'pending'))

async function load() {
  error.value = ''
  try {
    let url = '/api/applications'
    if (route.query.artifact) url += `?artifact_id=${route.query.artifact}`
    const data = await api.get(url)
    items.value = data.items
  } catch (e) {
    error.value = e.message
  }
}

function openApprove(a) {
  approveTarget.value = a
  formError.value = ''
  decision.value = {
    resolution_tier: a.requested_tier === 'original' ? 'original' : 'preview',
    expires_at: toLocalInput(new Date(a.end_at || Date.now() + 90 * 864e5)),
    allow_edit: !!a.requested_edit,
    note: ''
  }
}

// 冲突提示专门处理：409 说明另一位审批人的决定已经生效。
async function decide(a, payload) {
  busy.value = true
  try {
    await api.post(`/api/applications/${a.id}/decide`, payload)
    toast('审批已提交', 'ok')
    return true
  } catch (e) {
    if (e.status === 409) {
      toast('该申请刚被其他审批人处理，页面已刷新，最终结果以列表为准', 'err')
    } else {
      toast(e.message, 'err')
    }
    return false
  } finally {
    busy.value = false
    await load()
  }
}

async function confirmApprove() {
  if (!decision.value.expires_at) { formError.value = '请选择到期时间'; return }
  const ok = await decide(approveTarget.value, { approve: true, ...decision.value })
  if (ok) approveTarget.value = null
}

function openReject(a) { rejectTarget.value = a; rejectNote.value = '' }

async function confirmReject() {
  const ok = await decide(rejectTarget.value, { approve: false, note: rejectNote.value })
  if (ok) rejectTarget.value = null
}

function openRevoke(a) { revokeTarget.value = a; revokeReason.value = '' }

async function confirmRevoke() {
  busy.value = true
  try {
    await api.post(`/api/applications/${revokeTarget.value.id}/revoke`, { reason: revokeReason.value })
    toast('授权已撤回', 'ok')
    revokeTarget.value = null
  } catch (e) {
    toast(e.status === 409 ? '状态已被改变，已刷新' : e.message, 'err')
  } finally {
    busy.value = false
    await load()
  }
}

onMounted(load)
</script>
