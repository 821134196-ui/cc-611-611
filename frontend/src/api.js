// 统一 API 封装：同源请求，cookie 会话
async function request(path, options = {}) {
  const opts = { credentials: 'same-origin', headers: {}, ...options }
  if (opts.body && typeof opts.body !== 'string' && !(opts.body instanceof FormData)) {
    opts.headers['Content-Type'] = 'application/json'
    opts.body = JSON.stringify(opts.body)
  }
  const res = await fetch(path, opts)
  const isJSON = (res.headers.get('content-type') || '').includes('application/json')
  const data = isJSON ? await res.json() : await res.text()
  if (!res.ok) {
    const msg = (data && data.error) || `请求失败 (${res.status})`
    throw new Error(msg)
  }
  return data
}

export const api = {
  get: (p) => request(p),
  post: (p, body) => request(p, { method: 'POST', body }),
  upload: (p, formData) => request(p, { method: 'POST', body: formData }),
  raw: (p) => fetch(p, { credentials: 'same-origin' }),
}

export const CHANNELS = {
  print: '刊物',
  web: '网页',
  social: '新媒体',
}

export const LEVELS = {
  1: { label: '预览图（600px）', short: '预览' },
  2: { label: '网页版（1600px）', short: '网页' },
  3: { label: '原图 / 印刷版', short: '原图' },
}

export const STATUS = {
  pending: { label: '待审批', cls: 'badge-pending' },
  approved: { label: '可下载', cls: 'badge-approved' },
  rejected: { label: '已拒绝', cls: 'badge-rejected' },
  revoked: { label: '已撤回', cls: 'badge-revoked' },
  expired: { label: '已过期', cls: 'badge-expired' },
}

export function fmtDate(s) {
  if (!s) return '—'
  return String(s).slice(0, 10)
}
