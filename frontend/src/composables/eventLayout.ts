// 日程布局纯逻辑：按天切片、跨天/跨月边界裁剪、周/日视图时间轴车道排布、月格与列表聚合。
import dayjs from 'dayjs'
import type { EventInstance } from '../api/types'
import type { Calendar } from '../api/calendars'
import { allDayDateKey, toTz, type Dayjs } from './useLocalTime'

export interface CalEvent {
  ev: EventInstance
  cal: Calendar
}

export interface TimedSegBase {
  item: CalEvent
  /** 当日分钟，已按天边界裁剪 */
  startMin: number
  endMin: number
  /** 开始于本视图之前 */
  continuesBefore: boolean
  /** 结束于本视图之后 */
  continuesAfter: boolean
}

export interface TimedSeg extends TimedSegBase {
  lane: number
  lanes: number
}

export interface DaySlice {
  allDay: CalEvent[]
  timed: TimedSeg[]
}

/** 全天事件的覆盖判定用 UTC 日期语义（排他结束） */
export function isAllDayOn(item: CalEvent, dayKey: string): boolean {
  const s = allDayDateKey(item.ev.start)
  const e = allDayDateKey(item.ev.end)
  return s <= dayKey && dayKey < e
}

export function eventsOnDay(items: CalEvent[], day: Dayjs, tz: string): { allDay: CalEvent[]; timed: TimedSegBase[] } {
  const dayKey = day.format('YYYY-MM-DD')
  const dayStart = day
  const dayEnd = day.add(1, 'day')
  const allDay: CalEvent[] = []
  const timed: TimedSegBase[] = []
  for (const item of items) {
    const ev = item.ev
    if (ev.all_day) {
      if (isAllDayOn(item, dayKey)) allDay.push(item)
      continue
    }
    const s = toTz(ev.start, tz)
    const e = toTz(ev.end, tz)
    if (!e.isAfter(dayStart) || !s.isBefore(dayEnd)) continue
    const startMin = Math.max(0, s.diff(dayStart, 'minute'))
    const endMin = Math.min(24 * 60, e.diff(dayStart, 'minute'))
    timed.push({
      item,
      startMin,
      endMin: Math.max(endMin, startMin),
      continuesBefore: s.isBefore(dayStart),
      continuesAfter: e.isAfter(dayEnd),
    })
  }
  allDay.sort(cmpCalEvent)
  timed.sort((a, b) => a.startMin - b.startMin || b.endMin - a.endMin)
  return { allDay, timed }
}

/** 排序：全天在前，其次按开始时间 */
export function cmpCalEvent(a: CalEvent, b: CalEvent): number {
  if (a.ev.all_day !== b.ev.all_day) return a.ev.all_day ? -1 : 1
  if (a.ev.start === b.ev.start) return a.ev.summary.localeCompare(b.ev.summary)
  return a.ev.start < b.ev.start ? -1 : 1
}

/**
 * 时间轴车道排布：重叠的事件分组为簇，簇内贪心分配车道。
 * 返回带 lane（第几条）与 lanes（簇内总车道数）的新数组，宽度 = 1/lanes。
 */
export function layoutTimed<T extends { startMin: number; endMin: number }>(items: T[]): (T & { lane: number; lanes: number })[] {
  const sorted = [...items].sort((a, b) => a.startMin - b.startMin || b.endMin - a.endMin)
  const out: (T & { lane: number; lanes: number })[] = []
  let cluster: (T & { lane: number; lanes: number })[] = []
  let clusterEnd = -1
  const laneEnds: number[] = []
  const flush = () => {
    for (const it of cluster) it.lanes = laneEnds.length
    out.push(...cluster)
    cluster = []
    laneEnds.length = 0
    clusterEnd = -1
  }
  for (const it of sorted) {
    if (cluster.length > 0 && it.startMin >= clusterEnd) flush()
    let lane = laneEnds.findIndex(end => end <= it.startMin)
    if (lane === -1) {
      lane = laneEnds.length
      laneEnds.push(it.endMin)
    } else {
      laneEnds[lane] = it.endMin
    }
    cluster.push({ ...it, lane, lanes: 1 })
    clusterEnd = Math.max(clusterEnd, it.endMin)
  }
  flush()
  return out
}

/** 单日完整切片：全天 + 已排车道的限时日程 */
export function daySegments(items: CalEvent[], day: Dayjs, tz: string): DaySlice {
  const { allDay, timed } = eventsOnDay(items, day, tz)
  return { allDay, timed: layoutTimed(timed) }
}

export interface MonthCellData {
  allDay: CalEvent[]
  timed: TimedSeg[]
  /** 放不下被折叠的限时日程数 */
  hidden: number
}

/** 月格单元：全天条全部展示，限时日程最多显示到 maxVisible，超出折叠 */
export function monthCell(items: CalEvent[], day: Dayjs, tz: string, maxVisible = 3): MonthCellData {
  const { allDay, timed } = daySegments(items, day, tz)
  const visible = Math.max(0, maxVisible - allDay.length)
  return { allDay, timed: timed.slice(0, visible), hidden: Math.max(0, timed.length - visible) }
}

function eachDateKey(fromKey: string, toKeyInclusive: string): string[] {
  const keys: string[] = []
  let cur = dayjs(fromKey)
  const end = dayjs(toKeyInclusive)
  if (!cur.isValid() || !end.isValid() || end.isBefore(cur)) return keys
  while (!cur.isAfter(end) && keys.length <= 400) {
    keys.push(cur.format('YYYY-MM-DD'))
    cur = cur.add(1, 'day')
  }
  return keys
}

/** 日程覆盖的本地日期键（全天按 UTC 日期区间，限时按用户时区日期区间） */
export function coverLocalKeys(ev: EventInstance, tz: string): string[] {
  if (ev.all_day) {
    return eachDateKey(allDayDateKey(ev.start), dayjs.utc(ev.end).subtract(1, 'minute').format('YYYY-MM-DD'))
  }
  const s = toTz(ev.start, tz)
  const e = toTz(ev.end, tz).subtract(1, 'minute')
  return eachDateKey(s.format('YYYY-MM-DD'), e.format('YYYY-MM-DD'))
}

/** 列表/工作台聚合：dayKey → 当日日程（已排序） */
export function groupByDay(items: CalEvent[], tz: string): Map<string, CalEvent[]> {
  const map = new Map<string, CalEvent[]>()
  for (const item of items) {
    for (const key of coverLocalKeys(item.ev, tz)) {
      const list = map.get(key)
      if (list) list.push(item)
      else map.set(key, [item])
    }
  }
  for (const list of map.values()) list.sort(cmpCalEvent)
  return map
}
