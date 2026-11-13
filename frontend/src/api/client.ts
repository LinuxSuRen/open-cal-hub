// HTTP 封装核心：超时、网络错误包装、错误包络、成功包络解包、令牌刷新。
// 各领域资源 API 在同目录领域文件中定义（auth/calendars/booking/pub/admin）。契约见 docs/api-contract.md。

export class ApiError extends Error {
  code: string
  status: number
  details?: Record<string, string>

  constructor(message: string, code: string, status: number, details?: Record<string, string>) {
    super(message)
    this.code = code
    this.status = status
    this.details = details
  }
}

const TIMEOUT_MS = 15000
let accessToken = '' // 内存持有，不落 localStorage

export function setAccessToken(t: string) { accessToken = t }
export function hasAccessToken() { return accessToken !== '' }

export interface Envelope<T> { data: T; page?: number; page_size?: number; total?: number }
export interface ListResult<T> { data: T[]; page: number; page_size: number; total: number }

async function request<T>(method: string, path: string, body?: unknown, opts?: { retryRefresh?: boolean; keepEnvelope?: boolean }): Promise<T> {
  const ctrl = new AbortController()
  const timer = setTimeout(() => ctrl.abort(), TIMEOUT_MS)
  try {
    const headers: Record<string, string> = {}
    if (body !== undefined) headers['Content-Type'] = 'application/json'
    if (accessToken) headers['Authorization'] = `Bearer ${accessToken}`
    const res = await fetch(path, {
      method, headers,
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: ctrl.signal,
      credentials: 'same-origin',
    })
    if (res.status === 401 && opts?.retryRefresh !== false && accessToken) {
      const ok = await tryRefresh()
      if (ok) return request<T>(method, path, body, { retryRefresh: false, keepEnvelope: opts?.keepEnvelope })
    }
    if (!res.ok) {
      let code = 'common/internal'
      let message = `请求失败（HTTP ${res.status}）`
      let details: Record<string, string> | undefined
      try {
        const j = await res.json()
        if (j?.error?.code) code = j.error.code
        if (j?.error?.message) message = j.error.message
        if (j?.details) details = j.details
      } catch { /* 非 JSON 响应，用兜底文案 */ }
      throw new ApiError(message, code, res.status, details)
    }
    const text = await res.text()
    if (!text) return null as T
    let parsed: unknown
    try {
      parsed = JSON.parse(text)
    } catch {
      return null as T
    }
    // 成功包络 {"data": ...}：默认自动解包；getList 需要分页字段时保留整包
    if (!opts?.keepEnvelope && parsed !== null && typeof parsed === 'object' && 'data' in (parsed as Record<string, unknown>)) {
      return (parsed as { data: T }).data
    }
    return parsed as T
  } catch (err) {
    if (err instanceof ApiError) throw err
    if (err instanceof DOMException && err.name === 'AbortError') {
      throw new ApiError('请求超时，请检查网络后重试', 'timeout', 0)
    }
    throw new ApiError('无法连接后端服务，请确认服务已启动', 'network', 0)
  } finally {
    clearTimeout(timer)
  }
}

export const get = <T,>(path: string): Promise<T> => request<T>('GET', path)
export const post = <T,>(path: string, body?: unknown): Promise<T> => request<T>('POST', path, body ?? {})
export const patch = <T,>(path: string, body?: unknown): Promise<T> => request<T>('PATCH', path, body ?? {})
export const put = <T,>(path: string, body?: unknown): Promise<T> => request<T>('PUT', path, body ?? {})
export const del = <T,>(path: string): Promise<T> => request<T>('DELETE', path)

export async function getList<T>(path: string): Promise<ListResult<T>> {
  const env = await request<Envelope<T> | null>('GET', path, undefined, { keepEnvelope: true })
  return { data: (env?.data ?? []) as T[], page: env?.page ?? 1, page_size: env?.page_size ?? 20, total: env?.total ?? 0 }
}

// ---- 刷新令牌（httpOnly Cookie） ----
let refreshing: Promise<boolean> | null = null
export async function tryRefresh(): Promise<boolean> {
  refreshing ??= (async () => {
    try {
      const res = await fetch('/api/v1/auth/refresh', {
        method: 'POST', credentials: 'same-origin',
        headers: { 'X-Requested-With': 'fetch' },
      })
      if (!res.ok) return false
      const j = await res.json()
      accessToken = j.data?.access_token ?? ''
      return accessToken !== ''
    } catch {
      return false
    } finally {
      setTimeout(() => { refreshing = null }, 0)
    }
  })()
  return refreshing
}
