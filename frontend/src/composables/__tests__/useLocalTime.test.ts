import { beforeAll, describe, expect, it } from 'vitest'
import dayjs from 'dayjs'
import {
  allDayDateKey,
  allDayEndKeyInclusive,
  combineToUtc,
  fmtTime,
  todayKey,
  toUserTz,
  tzDate,
  userTimezone,
} from '../useLocalTime'
import { authState } from '../../stores/auth'
import type { User } from '../../api/types'

function setUser(tz: string) {
  authState.user = { timezone: tz } as User
}

describe('userTimezone', () => {
  it('优先取用户设置，未登录回退浏览器猜测', () => {
    setUser('Asia/Shanghai')
    expect(userTimezone()).toBe('Asia/Shanghai')
    authState.user = null
    expect(typeof userTimezone()).toBe('string')
    expect(userTimezone()).not.toBe('')
  })
})

describe('toUserTz / fmtTime', () => {
  beforeAll(() => setUser('Asia/Shanghai'))

  it('UTC RFC3339 转用户时区', () => {
    const d = toUserTz('2026-11-13T08:00:00Z')
    expect(d.format('YYYY-MM-DD HH:mm')).toBe('2026-11-13 16:00')
    expect(fmtTime('2026-11-13T08:00:00Z')).toBe('16:00')
  })

  it('切换到纽约时区后按 -5 偏移显示', () => {
    setUser('America/New_York')
    expect(fmtTime('2026-11-13T08:00:00Z')).toBe('03:00')
  })
})

describe('夏令时边界', () => {
  it('纽约 2026-03-08 凌晨 2 点进入夏令时', () => {
    setUser('America/New_York')
    // 3 月 7 日 09:00 为 EST(-5)
    expect(combineToUtc('2026-03-07', 9, 0, 'America/New_York')).toBe('2026-03-07T14:00:00Z')
    // 3 月 8 日 09:00 已是 EDT(-4)
    expect(combineToUtc('2026-03-08', 9, 0, 'America/New_York')).toBe('2026-03-08T13:00:00Z')
  })
  it('上海无夏令时，全年 +8', () => {
    expect(combineToUtc('2026-03-08', 9, 0, 'Asia/Shanghai')).toBe('2026-03-08T01:00:00Z')
  })
})

describe('全天事件日期语义', () => {
  it('UTC 日期提取与排他结束转含', () => {
    expect(allDayDateKey('2026-11-13T00:00:00Z')).toBe('2026-11-13')
    expect(allDayEndKeyInclusive('2026-11-15T00:00:00Z')).toBe('2026-11-14')
  })
})

describe('tzDate / todayKey', () => {
  it('tzDate 钉住时区零点', () => {
    setUser('Asia/Shanghai')
    const d = tzDate('2026-11-13', 'Asia/Shanghai')
    expect(d.format('YYYY-MM-DD HH:mm')).toBe('2026-11-13 00:00')
    expect(d.utc().format()).toBe('2026-11-12T16:00:00Z')
    expect(dayjs.tz.guess).toBeDefined()
  })
  it('todayKey 返回当日日期键', () => {
    expect(todayKey()).toMatch(/^\d{4}-\d{2}-\d{2}$/)
  })
})
