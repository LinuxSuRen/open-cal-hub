import { describe, expect, it } from 'vitest'
import { buildRrulePayload, bydayToWeekdayNum, parseRrule, weekdayNumToByday } from '../rrule'

describe('parseRrule', () => {
  it('空串与无效串返回 null', () => {
    expect(parseRrule('')).toBeNull()
    expect(parseRrule(undefined)).toBeNull()
    expect(parseRrule('FREQ=BOGUS')).toBeNull()
  })

  it('解析每周规则并按周一到周日排序 byday', () => {
    const m = parseRrule('FREQ=WEEKLY;INTERVAL=2;BYDAY=SU,MO,WE;COUNT=10')
    expect(m).not.toBeNull()
    expect(m!.freq).toBe('WEEKLY')
    expect(m!.interval).toBe(2)
    expect(m!.byday).toEqual(['MO', 'WE', 'SU'])
    expect(m!.endMode).toBe('count')
    expect(m!.count).toBe(10)
  })

  it('解析 until 日期', () => {
    const m = parseRrule('FREQ=DAILY;UNTIL=20261231T235959Z')
    expect(m!.endMode).toBe('until')
    expect(m!.until).toBe('2026-12-31')
  })

  it('缺失 interval 默认 1', () => {
    expect(parseRrule('FREQ=MONTHLY')!.interval).toBe(1)
  })
})

describe('buildRrulePayload', () => {
  it('null 返回 undefined', () => {
    expect(buildRrulePayload(null)).toBeUndefined()
  })

  it('默认值省略，仅发送非默认字段', () => {
    const p = buildRrulePayload(parseRrule('FREQ=WEEKLY;BYDAY=MO,SU')!)
    expect(p).toEqual({ freq: 'WEEKLY', byday: ['MO', 'SU'] })
  })

  it('until 输出 UTC 秒级，count 输出次数', () => {
    const until = buildRrulePayload(parseRrule('FREQ=DAILY;UNTIL=20261231T235959Z')!)!
    expect(until.until).toBe('2026-12-31T235959Z')
    const count = buildRrulePayload(parseRrule('FREQ=MONTHLY;COUNT=5')!)!
    expect(count.count).toBe(5)
    expect(count.interval).toBeUndefined()
  })
})

describe('weekday 映射', () => {
  it('契约 weekday 0=周日 与 BYDAY 互转', () => {
    expect(weekdayNumToByday(0)).toBe('SU')
    expect(weekdayNumToByday(1)).toBe('MO')
    expect(weekdayNumToByday(6)).toBe('SA')
    expect(bydayToWeekdayNum('SU')).toBe(0)
    expect(bydayToWeekdayNum('MO')).toBe(1)
  })
})
