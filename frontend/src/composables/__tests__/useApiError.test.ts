import { describe, expect, it } from 'vitest'
import { ApiError } from '../../api/client'
import { errorText } from '../useApiError'

describe('errorText 错误码映射', () => {
  it('已知 code 映射为中文文案', () => {
    const err = new ApiError('邮箱或密码不正确', 'auth/invalid_credentials', 401)
    expect(errorText(err)).toBe('邮箱或密码不正确，请检查后重试')
  })

  it('未知 code 回退服务端 message', () => {
    const err = new ApiError('服务端自定义说明', 'cal/some_new_code', 400)
    expect(errorText(err)).toBe('服务端自定义说明')
  })

  it('未知 code 且无 message 时回退兜底文案', () => {
    const err = new ApiError('', 'x/unknown', 500)
    expect(errorText(err)).toBe('操作失败，请稍后重试')
  })

  it('details 逐条附加到文案末尾', () => {
    const err = new ApiError('bad', 'common/validation_failed', 400, { email: '邮箱格式不正确', name: '名称不能为空' })
    expect(errorText(err)).toBe('请求参数有误，请检查填写内容后重试：邮箱格式不正确；名称不能为空')
  })

  it('timeout 与 network 映射到专用文案', () => {
    expect(errorText(new ApiError('请求超时，请检查网络后重试', 'timeout', 0))).toBe('请求超时，请检查网络后重试')
    expect(errorText(new ApiError('无法连接后端服务，请确认服务已启动', 'network', 0))).toBe('无法连接后端服务，请确认服务已启动')
  })

  it('非 ApiError 输入返回兜底文案', () => {
    expect(errorText(new Error('boom'))).toBe('操作失败，请稍后重试')
    expect(errorText(undefined)).toBe('操作失败，请稍后重试')
  })

  it('契约内 cal 段错误码均有映射', () => {
    const codes = [
      'cal/calendar_not_found', 'cal/event_not_found', 'cal/invalid_rrule', 'cal/invalid_range',
      'cal/import_failed', 'cal/share_not_found', 'cal/share_expired', 'cal/member_exists', 'cal/member_not_found',
    ]
    for (const code of codes) {
      const text = errorText(new ApiError('x', code, 404))
      expect(text).not.toBe('x')
      expect(text.length).toBeGreaterThan(3)
    }
  })
})
