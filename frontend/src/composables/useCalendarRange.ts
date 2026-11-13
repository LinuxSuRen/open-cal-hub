// 视图 → 拉取范围：月/周/日为网格天数，列表为 31 天；跨度上限 62 天（契约 cal/invalid_range）。
// 日期推演用“无时区日历运算”做，再逐天钉到用户时区，避免夏令时导致的日期漂移。
import dayjs from 'dayjs'
import { tzDate, type Dayjs } from './useLocalTime'

export type CalView = 'month' | 'week' | 'day' | 'list'
export const MAX_RANGE_DAYS = 62
export const LIST_VIEW_DAYS = 31

/** 周一为首的周起点；入参为 YYYY-MM-DD */
export function startOfWeekKey(dateKey: string): string {
  const d = dayjs(dateKey)
  return d.subtract((d.day() + 6) % 7, 'day').format('YYYY-MM-DD')
}

export interface CalRange {
  /** UTC RFC3339，含 */
  from: string
  /** UTC RFC3339，排他 */
  to: string
  /** 网格天（用户时区零点）；跨度被裁剪时仅含前 MAX_RANGE_DAYS 天 */
  days: Dayjs[]
}

/**
 * anchorKey：锚点日期 YYYY-MM-DD（用户时区语义）
 * month：覆盖锚点整月的完整周网格；week：锚点所在自然周；day：单日；list：锚点起 31 天
 */
export function computeRange(anchorKey: string, view: CalView, tz: string): CalRange {
  const naive = dayjs(anchorKey)
  let startKey: string
  let numDays: number
  if (view === 'month') {
    const firstKey = naive.startOf('month').format('YYYY-MM-DD')
    startKey = startOfWeekKey(firstKey)
    const lead = dayjs(firstKey).diff(dayjs(startKey), 'day')
    const weeks = Math.ceil((lead + dayjs(firstKey).daysInMonth()) / 7)
    numDays = weeks * 7
  } else if (view === 'week') {
    startKey = startOfWeekKey(anchorKey)
    numDays = 7
  } else if (view === 'day') {
    startKey = anchorKey
    numDays = 1
  } else {
    startKey = anchorKey
    numDays = LIST_VIEW_DAYS
  }
  const capped = Math.min(numDays, MAX_RANGE_DAYS)
  const days = Array.from({ length: capped }, (_, i) => tzDate(dayjs(startKey).add(i, 'day').format('YYYY-MM-DD'), tz))
  const from = days[0].utc().format()
  const to = days[capped - 1].add(1, 'day').utc().format()
  return { from, to, days }
}

/** 锚点跳转：view 下的上一个/下一个时段 */
export function shiftAnchor(anchorKey: string, view: CalView, dir: 1 | -1): string {
  const n = dayjs(anchorKey)
  if (view === 'month') return n.add(dir, 'month').date(1).format('YYYY-MM-DD')
  if (view === 'week') return n.add(dir * 7, 'day').format('YYYY-MM-DD')
  return n.add(dir, 'day').format('YYYY-MM-DD')
}

/** 工具栏区间标题；日视图返回日期部分，星期由调用方用 date.weekday.* 组合 */
export function rangeTitle(anchorKey: string, view: CalView): string {
  const n = dayjs(anchorKey)
  if (view === 'month') return n.format('YYYY年M月')
  if (view === 'week') {
    const s = dayjs(startOfWeekKey(anchorKey))
    const e = s.add(6, 'day')
    return s.year() === e.year() ? `${s.format('YYYY年M月D日')} – ${e.format('M月D日')}` : `${s.format('YYYY年M月D日')} – ${e.format('YYYY年M月D日')}`
  }
  return n.format('YYYY年M月D日')
}
