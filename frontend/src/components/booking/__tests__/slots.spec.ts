// 槽位分组与时区格式化纯函数测试（vitest，node 环境即可运行）。
import { describe, expect, it } from 'vitest'
import {
  HHmmToMinutes,
  addDays,
  dateRange,
  fmtTime,
  fmtTimeRange,
  formatDuration,
  groupSlotsByPeriod,
  dayHasAvailable,
  dateInTz,
  minutesToHHmm,
  rfc3339FromLocal,
  todayInTz,
  weekdayOf,
  humanDate,
  type SlotItem,
} from '../slots'

const slot = (iso: string, available = true): SlotItem => ({ start: iso, end: iso, available })

describe('groupSlotsByPeriod 按访客时区分组', () => {
  const slots: SlotItem[] = [
    slot('2026-11-16T01:00:00Z'), // 上海 09:00 上午；纽约(EST, UTC-5) 前一日 20:00 晚间
    slot('2026-11-16T04:30:00Z'), // 上海 12:30 下午
    slot('2026-11-16T10:00:00Z'), // 上海 18:00 晚间
    slot('2026-11-16T03:59:00Z'), // 上海 11:59 上午
    slot('2026-11-16T09:59:00Z', false), // 上海 17:59 下午（已占用）
  ]

  it('上海时区：上午 <12、下午 12-17、晚间 >=18', () => {
    const g = groupSlotsByPeriod(slots, 'Asia/Shanghai')
    expect(g.morning.map(s => s.start)).toEqual(['2026-11-16T01:00:00Z', '2026-11-16T03:59:00Z'])
    expect(g.afternoon.map(s => s.start)).toEqual(['2026-11-16T04:30:00Z', '2026-11-16T09:59:00Z'])
    expect(g.evening.map(s => s.start)).toEqual(['2026-11-16T10:00:00Z'])
  })

  it('同一批槽位在纽约时区落到不同分组', () => {
    const g = groupSlotsByPeriod(slots, 'America/New_York')
    // 01:00Z → 前日 20:00 晚间；04:30Z → 前日 23:30 晚间；10:00Z → 05:00 上午；03:59Z → 前日 22:59 晚间；09:59Z → 04:59 上午
    expect(g.morning.map(s => s.start)).toEqual(['2026-11-16T10:00:00Z', '2026-11-16T09:59:00Z'])
    expect(g.afternoon).toEqual([])
    expect(g.evening.map(s => s.start)).toEqual([
      '2026-11-16T01:00:00Z',
      '2026-11-16T04:30:00Z',
      '2026-11-16T03:59:00Z',
    ])
  })

  it('空输入与 UTC', () => {
    const g = groupSlotsByPeriod([], 'UTC')
    expect(g).toEqual({ morning: [], afternoon: [], evening: [] })
    const u = groupSlotsByPeriod([slot('2026-11-16T23:30:00Z')], 'UTC')
    expect(u.evening).toHaveLength(1)
  })
})

describe('dayHasAvailable 日期条置灰依据', () => {
  it('无可约槽位或缺失都视为不可约', () => {
    expect(dayHasAvailable({ date: '2026-11-16', slots: [slot('2026-11-16T01:00:00Z', false)] })).toBe(false)
    expect(dayHasAvailable({ date: '2026-11-16', slots: [] })).toBe(false)
    expect(dayHasAvailable(undefined)).toBe(false)
    expect(dayHasAvailable({ date: '2026-11-16', slots: [slot('2026-11-16T01:00:00Z', false), slot('2026-11-16T02:00:00Z')] })).toBe(true)
  })
})

describe('时区切换格式化', () => {
  const iso = '2026-11-16T08:00:00Z'

  it('同一时刻在不同时区的钟面时间', () => {
    expect(fmtTime(iso, 'Asia/Shanghai')).toBe('16:00')
    expect(fmtTime(iso, 'America/New_York')).toBe('03:00') // 11 月已进入 EST(UTC-5)
    expect(fmtTime(iso, 'UTC')).toBe('08:00')
  })

  it('夏令时边界：7 月纽约为 EDT(UTC-4)', () => {
    expect(fmtTime('2026-07-16T08:00:00Z', 'America/New_York')).toBe('04:00')
    expect(fmtTime('2026-07-16T08:00:00Z', 'Asia/Shanghai')).toBe('16:00')
  })

  it('同日与跨日的区间文案', () => {
    expect(fmtTimeRange('2026-11-16T08:00:00Z', '2026-11-16T08:30:00Z', 'Asia/Shanghai')).toBe('16:00–16:30')
    // 上海 11-16 18:00 → 11-17 00:30，跨日补日期
    expect(fmtTimeRange('2026-11-16T10:00:00Z', '2026-11-16T16:30:00Z', 'Asia/Shanghai')).toBe('11月16日 18:00 – 11月17日 00:30')
  })

  it('dateInTz 按时区取日期串', () => {
    expect(dateInTz('2026-11-16T16:30:00Z', 'Asia/Shanghai')).toBe('2026-11-17')
    expect(dateInTz('2026-11-16T16:30:00Z', 'UTC')).toBe('2026-11-16')
  })
})

describe('日期窗口与分钟换算', () => {
  it('addDays / dateRange / weekdayOf（0=周日）', () => {
    expect(addDays('2026-11-30', 1)).toBe('2026-12-01')
    expect(dateRange('2026-11-16', 3)).toEqual(['2026-11-16', '2026-11-17', '2026-11-18'])
    expect(weekdayOf('2026-11-16')).toBe(1) // 周一
    expect(weekdayOf('2026-11-15')).toBe(0) // 周日
  })

  it('分钟与 HH:mm 互转', () => {
    expect(minutesToHHmm(540)).toBe('09:00')
    expect(minutesToHHmm(1439)).toBe('23:59')
    expect(HHmmToMinutes('09:30')).toBe(570)
    expect(HHmmToMinutes('23:59')).toBe(1439)
    expect(HHmmToMinutes('25:00')).toBeNull()
    expect(HHmmToMinutes('abc')).toBeNull()
  })

  it('本地日期时间 + 时区 → UTC RFC3339', () => {
    expect(rfc3339FromLocal('2026-11-16', '09:30', 'Asia/Shanghai')).toBe('2026-11-16T01:30:00Z')
    expect(rfc3339FromLocal('2026-11-16', '09:30', 'UTC')).toBe('2026-11-16T09:30:00Z')
  })

  it('todayInTz 返回当天日期串', () => {
    expect(todayInTz('UTC')).toMatch(/^\d{4}-\d{2}-\d{2}$/)
  })

  it('formatDuration 中文时长', () => {
    expect(formatDuration(30)).toBe('30 分钟')
    expect(formatDuration(60)).toBe('1 小时')
    expect(formatDuration(75)).toBe('1 小时 15 分')
    expect(formatDuration(125)).toBe('2 小时 5 分')
  })

  it('humanDate 中文与英文字面', () => {
    const zh = humanDate('2026-11-16', 'Asia/Shanghai', 'zh-CN')
    expect(zh).toContain('11月16日')
    expect(zh).toContain('周一')
    const en = humanDate('2026-11-16T08:00:00Z', 'Asia/Shanghai', 'en')
    expect(en).toContain('Nov 16')
  })
})
