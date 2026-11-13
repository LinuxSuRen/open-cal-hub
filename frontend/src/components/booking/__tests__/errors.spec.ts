// 错误码文案映射测试：已知 code 走本地文案、未知 code 回退服务端 message、不泄漏技术细节。
import { describe, expect, it } from 'vitest'
import { apiErrorText, errorText, errorMessages } from '../../../i18n/pub'
import { setLocale } from '../../../i18n'
import { ApiError } from '../../../api/client'

describe('errorText 错误码 → 本地文案', () => {
  it('已登记的 code 返回中文文案', () => {
    expect(errorText('booking/slot_unavailable', '服务端原始文案')).toBe('该时间段已被占用，请选择其他时间')
    expect(errorText('cal/share_expired', '')).toBe('分享链接已过期')
    expect(errorText('auth/weak_password', '')).toContain('8 位')
  })

  it('未知 code 回退服务端 message', () => {
    expect(errorText('some/future_code', '服务端兜底文案')).toBe('服务端兜底文案')
  })

  it('未知 code 且无 message 时给通用文案', () => {
    expect(errorText('some/future_code', '')).toBe('服务暂时不可用，请稍后重试')
  })

  it('英文语言环境返回英文文案', () => {
    setLocale('en')
    try {
      expect(errorText('booking/slot_unavailable', 'x')).toBe('That time is taken, please pick another')
      expect(errorText('cal/share_not_found', 'x')).toBe('Share link not found or revoked')
    } finally {
      setLocale('zh-CN')
    }
  })

  it('契约段位表中的 booking/cal 关键码均已登记', () => {
    for (const code of [
      'booking/event_type_not_found', 'booking/inactive', 'booking/host_not_found',
      'booking/slot_unavailable', 'booking/outside_range', 'booking/insufficient_notice',
      'booking/invalid_slot', 'booking/not_found', 'booking/already_cancelled',
      'cal/share_not_found', 'cal/share_expired',
    ]) {
      expect(errorMessages[code], `缺少错误码文案: ${code}`).toBeDefined()
    }
  })

  it('文案不含技术词（无 http/rpc/refused 等）', () => {
    for (const [code, entry] of Object.entries(errorMessages)) {
      expect(entry['zh-CN'], `${code} 文案为空`).toBeTruthy()
      expect(entry['zh-CN']).not.toMatch(/rpc|refused|grpc|panic|goroutine|http/i)
    }
  })
})

describe('apiErrorText 适配 ApiError', () => {
  it('ApiError 实例走 code 映射', () => {
    const err = new ApiError('该时间段已被占用，请选择其他时间', 'booking/slot_unavailable', 409)
    expect(apiErrorText(err)).toBe('该时间段已被占用，请选择其他时间')
  })

  it('未知 code 用 ApiError.message 兜底', () => {
    const err = new ApiError('后端新错误', 'new/unknown', 400)
    expect(apiErrorText(err)).toBe('后端新错误')
  })

  it('空对象按网络错误处理', () => {
    expect(apiErrorText(null)).toBe('无法连接服务器，请检查网络')
  })
})
