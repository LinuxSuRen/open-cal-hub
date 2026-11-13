import { describe, expect, it } from 'vitest'
import { computeRange, shiftAnchor, startOfWeekKey, MAX_RANGE_DAYS } from '../useCalendarRange'

const TZ = 'Asia/Shanghai'

describe('startOfWeekKey', () => {
  it('周日锚点回退到上一个周一', () => {
    expect(startOfWeekKey('2026-11-01')).toBe('2026-10-26')
  })
  it('周一锚点即本周一', () => {
    expect(startOfWeekKey('2026-11-09')).toBe('2026-11-09')
  })
})

describe('computeRange month', () => {
  it('11 月网格从 10 月 26 日开始，共 42 天，UTC 边界正确', () => {
    const r = computeRange('2026-11-13', 'month', TZ)
    expect(r.days).toHaveLength(42)
    expect(r.days[0].format('YYYY-MM-DD')).toBe('2026-10-26')
    expect(r.days[41].format('YYYY-MM-DD')).toBe('2026-12-06')
    expect(r.from).toBe('2026-10-25T16:00:00Z')
    expect(r.to).toBe('2026-12-06T16:00:00Z')
  })
  it('仅 5 周的月份生成 35 天', () => {
    // 2026-02-01 是周日，2 月 28 天：lead=6，ceil(34/7)=5
    const r = computeRange('2026-02-10', 'month', TZ)
    expect(r.days).toHaveLength(35)
    expect(r.days[0].format('YYYY-MM-DD')).toBe('2026-01-26')
  })
  it('夏令时切换不产生日期漂移', () => {
    const r = computeRange('2026-03-13', 'month', 'America/New_York')
    expect(r.days[0].format('YYYY-MM-DD')).toBe('2026-02-23')
    // 3 月 8 日凌晨美国进入夏令时，日历日仍是 3 月 8 日
    expect(r.days[13].format('YYYY-MM-DD')).toBe('2026-03-08')
    // 网格首日 EST -5，末日次日 EDT -4
    expect(r.from).toBe('2026-02-23T05:00:00Z')
    expect(r.to).toBe('2026-04-06T04:00:00Z')
  })
})

describe('computeRange week/day/list', () => {
  it('周视图为锚点所在自然周 7 天', () => {
    const r = computeRange('2026-11-13', 'week', TZ)
    expect(r.days.map(d => d.format('YYYY-MM-DD'))).toEqual([
      '2026-11-09', '2026-11-10', '2026-11-11', '2026-11-12', '2026-11-13', '2026-11-14', '2026-11-15',
    ])
    expect(r.from).toBe('2026-11-08T16:00:00Z')
    expect(r.to).toBe('2026-11-15T16:00:00Z')
  })
  it('日视图覆盖单日', () => {
    const r = computeRange('2026-11-13', 'day', TZ)
    expect(r.days).toHaveLength(1)
    expect(r.from).toBe('2026-11-12T16:00:00Z')
    expect(r.to).toBe('2026-11-13T16:00:00Z')
  })
  it('列表视图为 31 天且不超过上限', () => {
    const r = computeRange('2026-11-13', 'list', TZ)
    expect(r.days).toHaveLength(31)
    expect(r.days.length).toBeLessThanOrEqual(MAX_RANGE_DAYS)
    expect(r.to).toBe('2026-12-13T16:00:00Z')
  })
})

describe('shiftAnchor', () => {
  it('月视图 1 月 31 日后翻到 2 月 1 日', () => {
    expect(shiftAnchor('2026-01-31', 'month', 1)).toBe('2026-02-01')
    expect(shiftAnchor('2026-03-31', 'month', -1)).toBe('2026-02-01')
  })
  it('周视图跨 7 天，日视图跨 1 天', () => {
    expect(shiftAnchor('2026-11-13', 'week', 1)).toBe('2026-11-20')
    expect(shiftAnchor('2026-11-13', 'day', -1)).toBe('2026-11-12')
  })
})
