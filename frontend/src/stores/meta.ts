// 站点元信息缓存：认证页与 App 外壳共用；失败时保持默认值，不阻塞流程。
import { reactive } from 'vue'
import { authApi } from '../api/auth'

const FALLBACK = { siteName: 'CalHub', registrationMode: 'open', version: '' }

export const metaState = reactive({
  loaded: false,
  siteName: FALLBACK.siteName,
  registrationMode: FALLBACK.registrationMode,
  version: FALLBACK.version,
})

let fetching: Promise<void> | null = null

export function ensureMeta(): Promise<void> {
  fetching ??= (async () => {
    try {
      const m = await authApi.meta()
      if (m) {
        metaState.siteName = m.site_name || FALLBACK.siteName
        metaState.registrationMode = m.registration_mode || FALLBACK.registrationMode
        metaState.version = m.version ?? ''
      }
    } catch {
      // 保持默认值
    }
    metaState.loaded = true
  })()
  return fetching
}
