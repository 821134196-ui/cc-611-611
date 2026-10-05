import { store } from './store.js'

// 统一 fetch 封装：自动带 token，401 时清理会话。
async function request(method, path, body) {
  const opts = {
    method,
    headers: { 'Content-Type': 'application/json' }
  }
  if (store.token) opts.headers.Authorization = `Bearer ${store.token}`
  if (body !== undefined) opts.body = JSON.stringify(body)

  const resp = await fetch(path, opts)
  if (resp.status === 401) {
    store.clear()
    location.hash = '#/login'
    throw new Error('登录已失效，请重新登录')
  }
  const text = await resp.text()
  let data = null
  if (text) {
    try { data = JSON.parse(text) } catch { data = { raw: text } }
  }
  if (!resp.ok) {
    const err = new Error(data?.error || `请求失败 (${resp.status})`)
    err.status = resp.status
    err.data = data
    throw err
  }
  return data
}

export const api = {
  get: (p) => request('GET', p),
  post: (p, b) => request('POST', p, b || {}),
  // 带鉴权头的文件下载地址（用于 <a> / window.open）
  authedUrl: (p) => p,
  // 用 fetch 拉取 Blob，保证带 token，并用浏览器另存
  async download(path, filename) {
    const resp = await fetch(path, {
      headers: { Authorization: `Bearer ${store.token}` }
    })
    if (resp.status === 401) {
      store.clear()
      location.hash = '#/login'
      throw new Error('登录已失效')
    }
    if (!resp.ok) {
      let msg = `无法下载 (${resp.status})`
      try {
        const j = await resp.json()
        if (j.error) msg = j.error
      } catch { /* ignore */ }
      const err = new Error(msg)
      err.status = resp.status
      throw err
    }
    const blob = await resp.blob()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = filename || ''
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
  },
  exportUrl: (path) => {
    // CSV 导出用带 token 的临时地址：先取 blob 再保存。
    return api.download(path, 'publications.csv')
  }
}
