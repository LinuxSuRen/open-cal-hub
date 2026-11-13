// ApiError.code → 本地文案映射；未知 code 回退服务端 message；details 逐条附加。
// 映射键定义在 i18n/app.ts 的 error.code.*（common/auth/cal 段），booking/admin 段由对应 owner 补充。
import { ApiError } from '../api/client'
import { t } from '../i18n'

export function errorText(err: unknown): string {
  if (err instanceof ApiError) {
    const key = `error.code.${err.code}`
    const mapped = t(key)
    const main = mapped !== key ? mapped : (err.message || t('error.fallback'))
    const details = err.details ? Object.values(err.details).filter(Boolean) : []
    return details.length > 0 ? `${main}：${details.join('；')}` : main
  }
  return t('error.fallback')
}

export function useApiError() {
  return { errorText }
}
