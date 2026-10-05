// 状态 / 档位的中文展示与配色，供所有页面统一使用。
export const STATUS = {
  pending:  { label: '待审批', cls: 'badge-pending' },
  approved: { label: '可下载', cls: 'badge-approved' },
  rejected: { label: '已驳回', cls: 'badge-dead' },
  revoked:  { label: '已撤回', cls: 'badge-dead' },
  expired:  { label: '已过期', cls: 'badge-dead' }
}

export const TIER = {
  preview:  { label: '预览图', cls: 'tier-preview' },
  original: { label: '原图',   cls: 'tier-original' }
}

export function statusInfo(s) {
  return STATUS[s] || { label: s, cls: 'badge-pending' }
}

export function tierInfo(t) {
  return TIER[t] || { label: t, cls: '' }
}

export function fmtTime(t) {
  if (!t) return '—'
  const d = new Date(t)
  if (Number.isNaN(d.getTime())) return '—'
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

export function fmtDate(t) {
  if (!t) return '—'
  const d = new Date(t)
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

// datetime-local 输入框需要的格式
export function toLocalInput(d) {
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`
}
