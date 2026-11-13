// 公开预约槽位纯计算：分组、时区格式化、日期窗口与分钟换算。
// 全部为纯函数（vitest 锁定行为）；组件只做渲染。
import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'
import timezone from 'dayjs/plugin/timezone'

dayjs.extend(utc)
dayjs.extend(timezone)

export interface SlotItem { start: string; end: string; available: boolean }
export interface SlotDay { date: string; slots: SlotItem[] }

export type PeriodKey = 'morning' | 'afternoon' | 'evening'
export type SlotGroups = Record<PeriodKey, SlotItem[]>

/** 槽位在指定时区的钟点（0-23）。 */
export function slotHour(slot: SlotItem, tz: string): number {
  return dayjs(slot.start).tz(tz).hour()
}

/** 按访客时区把槽位分为上午（<12）/ 下午（12-17）/ 晚间（>=18）。 */
export function groupSlotsByPeriod(slots: SlotItem[], tz: string): SlotGroups {
  const groups: SlotGroups = { morning: [], afternoon: [], evening: [] }
  for (const s of slots) {
    const h = slotHour(s, tz)
    if (h < 12) groups.morning.push(s)
    else if (h < 18) groups.afternoon.push(s)
    else groups.evening.push(s)
  }
  return groups
}

/** 某日是否存在可约槽位（日期条置灰依据）。 */
export function dayHasAvailable(day: SlotDay | undefined): boolean {
  return !!day && day.slots.some(s => s.available)
}

/** HH:mm（指定时区钟面）。注意：ISO 输入用 dayjs(iso).tz(tz) 转换；dayjs.tz(str, tz) 是"按该时区解释字符串"。 */
export function fmtTime(iso: string, tz: string): string {
  return dayjs(iso).tz(tz).format('HH:mm')
}

/** 起止区间文案：同日 "09:00–09:30"，跨日补日期。 */
export function fmtTimeRange(startIso: string, endIso: string, tz: string): string {
  const a = dayjs(startIso).tz(tz)
  const b = dayjs(endIso).tz(tz)
  if (a.isSame(b, 'day')) return `${a.format('HH:mm')}–${b.format('HH:mm')}`
  return `${a.format('M月D日 HH:mm')} – ${b.format('M月D日 HH:mm')}`
}

/** 管理台/工作台用：浏览器本地时间展示。 */
export function fmtLocal(iso: string | null | undefined): string {
  if (!iso) return '—'
  return dayjs(iso).format('YYYY-MM-DD HH:mm')
}

/** 指定时区当天的日期串 YYYY-MM-DD。 */
export function dateInTz(iso: string, tz: string): string {
  return dayjs(iso).tz(tz).format('YYYY-MM-DD')
}

/** 指定时区的"今天" YYYY-MM-DD。 */
export function todayInTz(tz: string): string {
  return dayjs.tz(Date.now(), tz).format('YYYY-MM-DD')
}

export function addDays(date: string, n: number): string {
  return dayjs(date).add(n, 'day').format('YYYY-MM-DD')
}

/** [start, start+n) 的日期串列表。 */
export function dateRange(start: string, n: number): string[] {
  return Array.from({ length: n }, (_, i) => addDays(start, i))
}

/** 星期序号：0=周日 … 6=周六（契约对齐）。 */
export function weekdayOf(date: string): number {
  return dayjs(date).day()
}

/** 分钟数(0..1439) → "HH:mm"。 */
export function minutesToHHmm(m: number): string {
  const h = Math.floor(m / 60)
  const mm = m % 60
  return `${String(h).padStart(2, '0')}:${String(mm).padStart(2, '0')}`
}

/** "HH:mm" → 分钟数；非法输入返回 null。 */
export function HHmmToMinutes(v: string): number | null {
  const m = /^(\d{1,2}):(\d{2})$/.exec(v.trim())
  if (!m) return null
  const h = Number(m[1])
  const mm = Number(m[2])
  if (h > 24 || mm > 59) return null
  return h * 60 + mm
}

/** 本地日期+时间+时区 → UTC RFC3339（Z 后缀，秒级）。 */
export function rfc3339FromLocal(date: string, time: string, tz: string): string {
  return dayjs.tz(`${date}T${time}:00`, tz).utc().format('YYYY-MM-DDTHH:mm:ss[Z]')
}

/** 时长分钟 → "30 分钟" / "1 小时 5 分"。 */
export function formatDuration(minutes: number): string {
  const h = Math.floor(minutes / 60)
  const m = minutes % 60
  if (h === 0) return `${m} 分钟`
  if (m === 0) return h === 1 ? '1 小时' : `${h} 小时`
  return `${h} 小时 ${m} 分`
}

/** 人类可读日期：zh → "11月16日 周一"；en → "Mon, Nov 16"。10 位日期串按正午取样避免日期漂移。 */
export function humanDate(dateOrIso: string, tz: string, locale: 'zh-CN' | 'en'): string {
  const date = dateOrIso.length === 10 ? new Date(`${dateOrIso}T12:00:00Z`) : new Date(dateOrIso)
  const loc = locale === 'en' ? 'en-US' : 'zh-CN'
  const opts: Intl.DateTimeFormatOptions =
    dateOrIso.length === 10
      ? { month: 'short', day: 'numeric', weekday: 'short' }
      : { month: 'short', day: 'numeric', weekday: 'short', timeZone: tz }
  try {
    return new Intl.DateTimeFormat(loc, opts).format(date)
  } catch {
    return dateOrIso
  }
}
