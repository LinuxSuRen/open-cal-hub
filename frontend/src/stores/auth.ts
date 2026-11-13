// 认证 store（模块级 reactive，不引状态库）。
// access token 只存内存；刷新依赖 httpOnly Cookie；页面刷新时先 tryRefresh 再拉 me。
import { reactive } from 'vue'
import { setAccessToken, hasAccessToken, tryRefresh, authApi, type User } from '../api'
import { setLocale, type Locale } from '../i18n'

function applyLocale(user: User | null) {
  setLocale(user?.locale === 'en' ? 'en' : ('zh-CN' as Locale))
}

export const authState = reactive<{
  user: User | null
  booted: boolean
  loading: boolean
}>({
  user: null,
  booted: false,
  loading: false,
})

export async function bootAuth(): Promise<void> {
  if (authState.booted) return
  if (!hasAccessToken()) await tryRefresh()
  if (hasAccessToken()) {
    try {
      authState.user = await authApi.me()
    } catch {
      authState.user = null
    }
  }
  applyLocale(authState.user)
  authState.booted = true
}

export async function login(email: string, password: string): Promise<User> {
  authState.loading = true
  try {
    const res = await authApi.login(email, password)
    setAccessToken(res.access_token)
    authState.user = res.user
    applyLocale(authState.user)
    authState.booted = true
    return res.user
  } finally {
    authState.loading = false
  }
}

export async function logout(): Promise<void> {
  try { await authApi.logout() } catch { /* 忽略登出失败 */ }
  setAccessToken('')
  authState.user = null
  // 动态引入避免模块级依赖 router（测试环境无 window）
  const { router } = await import('../router')
  await router.push('/login')
}

export function isAdmin(): boolean {
  return authState.user?.role === 'admin'
}
