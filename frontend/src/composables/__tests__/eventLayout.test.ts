import { describe, expect, it } from 'vitest'
import type { Calendar } from '../../api/calendars'
import type { EventInstance } from '../../api/types'
import { daySegments, eventsOnDay, groupByDay, layoutTimed, monthCell } from '../eventLayout'
import { tzDate } from '../useLocalTime'

const TZ = 'Asia/Shanghai'

const cal: Calendar = {
  id: 1, name: '工作', description: '', color: '#4F6EF7', timezone: TZ, is_default: true,
  owner: { id: 1, username: 'rick', display_name: 'Rick' }, role: 'owner', created_at: '',
}

function ev(p: Partial<EventInstance>): EventInstance {
  return {
    id: 1, uid: 'u', calendar_id: 1, summary: '日程', location: '', description: '',
    start: '2026-11-13T01:00:00Z', end: '2026-11-13T02:00:00Z', all_day: false, tz: TZ,
    status: 'confirmed', rrule: '', recurring: false, recurrence_id: null, reminders: [], master_id: 1,
    ...p,
  }
}

describe('layoutTimed 车道排布', () => {
  it('重叠簇分车道，簇外重置', () => {
    const laid = layoutTimed([
      { id: 'a', startMin: 0, endMin: 60 },
      { id: 'b', startMin: 30, endMin: 90 },
      { id: 'c', startMin: 30, endMin: 60 },
      { id: 'd', startMin: 120, endMin: 180 },
    ])
    const byId = Object.fromEntries(laid.map(x => [x.id, x]))
    expect(byId.a.lane).toBe(0)
    expect(byId.b.lane).toBe(1)
    expect(byId.c.lane).toBe(2)
    expect([byId.a.lanes, byId.b.lanes, byId.c.lanes]).toEqual([3, 3, 3])
    expect(byId.d.lane).toBe(0)
    expect(byId.d.lanes).toBe(1)
  })

  it('相邻不重叠的事件共用车道', () => {
    const laid = layoutTimed([
      { id: 'a', startMin: 0, endMin: 60 },
      { id: 'b', startMin: 60, endMin: 120 },
    ])
    expect(laid.every(x => x.lanes === 1)).toBe(true)
  })
})

describe('eventsOnDay / daySegments', () => {
  const day = tzDate('2026-11-13', TZ)

  it('跨午夜事件按天边界裁剪并标记延续', () => {
    const item = { ev: ev({ start: '2026-11-13T14:00:00Z', end: '2026-11-13T18:00:00Z' }), cal }
    const { timed } = eventsOnDay([item], day, TZ)
    expect(timed).toHaveLength(1)
    expect(timed[0].startMin).toBe(22 * 60)
    expect(timed[0].endMin).toBe(24 * 60)
    expect(timed[0].continuesAfter).toBe(true)
    expect(timed[0].continuesBefore).toBe(false)
  })

  it('跨视图起始边界的事件裁剪出当日片段', () => {
    // 前一天 22:00（+08）开始、当天 01:00 结束
    const item = { ev: ev({ start: '2026-11-12T14:00:00Z', end: '2026-11-12T17:00:00Z' }), cal }
    const { timed } = eventsOnDay([item], day, TZ)
    expect(timed[0].startMin).toBe(0)
    expect(timed[0].endMin).toBe(60)
    expect(timed[0].continuesBefore).toBe(true)
  })

  it('全天事件按 UTC 日期区间覆盖，排他结束', () => {
    const item = { ev: ev({ all_day: true, start: '2026-11-13T00:00:00Z', end: '2026-11-15T00:00:00Z' }), cal }
    expect(eventsOnDay([item], tzDate('2026-11-12', TZ), TZ).allDay).toHaveLength(0)
    expect(eventsOnDay([item], tzDate('2026-11-13', TZ), TZ).allDay).toHaveLength(1)
    expect(eventsOnDay([item], tzDate('2026-11-14', TZ), TZ).allDay).toHaveLength(1)
    expect(eventsOnDay([item], tzDate('2026-11-15', TZ), TZ).allDay).toHaveLength(0)
  })

  it('当天无交集的事件不出现', () => {
    const item = { ev: ev({ start: '2026-11-14T01:00:00Z', end: '2026-11-14T02:00:00Z' }), cal }
    expect(eventsOnDay([item], day, TZ).timed).toHaveLength(0)
  })

  it('daySegments 输出带车道信息', () => {
    const items = [
      { ev: ev({ id: 1, start: '2026-11-13T01:00:00Z', end: '2026-11-13T02:00:00Z' }), cal },
      { ev: ev({ id: 2, start: '2026-11-13T01:30:00Z', end: '2026-11-13T02:30:00Z' }), cal },
    ]
    const seg = daySegments(items, day, TZ)
    expect(seg.timed).toHaveLength(2)
    expect(seg.timed.every(s => s.lanes === 2)).toBe(true)
  })
})

describe('monthCell 折叠', () => {
  it('全天条不占限时名额，超出部分折叠计数', () => {
    const day = tzDate('2026-11-13', TZ)
    const items = [
      { ev: ev({ id: 100, all_day: true, start: '2026-11-13T00:00:00Z', end: '2026-11-14T00:00:00Z' }), cal },
      ...Array.from({ length: 5 }, (_, i) => ({
        ev: ev({ id: i + 1, start: `2026-11-13T0${i + 1}:00:00Z`, end: `2026-11-13T0${i + 1}:30:00Z` }),
        cal,
      })),
    ]
    const cell = monthCell(items, day, TZ, 3)
    expect(cell.allDay).toHaveLength(1)
    expect(cell.timed).toHaveLength(2)
    expect(cell.hidden).toBe(3)
  })
})

describe('groupByDay 聚合', () => {
  it('跨天限时日程落入两个本地日期', () => {
    const items = [{ ev: ev({ start: '2026-11-13T15:30:00Z', end: '2026-11-13T16:30:00Z' }), cal }]
    const map = groupByDay(items, TZ)
    expect(map.get('2026-11-13')).toHaveLength(1)
    expect(map.get('2026-11-14')).toHaveLength(1)
    expect(map.has('2026-11-15')).toBe(false)
  })

  it('整点结束的次日零点事件不溢出到结束日', () => {
    // 11-14 00:00（+08）整点结束：结束日本身无日程
    const items = [{ ev: ev({ start: '2026-11-13T15:00:00Z', end: '2026-11-13T16:00:00Z' }), cal }]
    const map = groupByDay(items, TZ)
    expect(map.has('2026-11-14')).toBe(false)
  })

  it('多日全天事件按 UTC 日期铺开且全天排在最前', () => {
    const items = [
      { ev: ev({ id: 2, start: '2026-11-13T02:00:00Z', end: '2026-11-13T03:00:00Z' }), cal },
      { ev: ev({ id: 1, all_day: true, start: '2026-11-13T00:00:00Z', end: '2026-11-14T00:00:00Z' }), cal },
    ]
    const map = groupByDay(items, TZ)
    const list = map.get('2026-11-13') ?? []
    expect(list.map(x => x.ev.id)).toEqual([1, 2])
  })
})
