// 用户时区本地化：所有 UTC 时间展示前必须经过这里转换（契约：API 一律 UTC RFC3339 Z）。
// 全天事件的日期语义固定在 UTC（契约：all_day 时为该日期 00:00 UTC），不走用户时区。
import { ref, onUnmounted } from 'vue'
import dayjs from 'dayjs'
import utcPlugin from 'dayjs/plugin/utc'
import tzPlugin from 'dayjs/plugin/timezone'
import { authState } from '../stores/auth'

dayjs.extend(utcPlugin)
dayjs.extend(tzPlugin)

export type Dayjs = dayjs.Dayjs

/** 当前用户时区；未登录时退回浏览器猜测 */
export function userTimezone(): string {
  const tz = authState.user?.timezone
  if (tz) return tz
  try {
    return dayjs.tz.guess() || 'UTC'
  } catch {
    return 'UTC'
  }
}

/** UTC RFC3339 → 指定时区实例。注意：dayjs.tz 对带偏移的字符串不换算，必须先经 utc 解析 */
export function toTz(v: string, tz: string): Dayjs {
  return dayjs.utc(v).tz(tz)
}

/** UTC RFC3339 → 用户时区实例 */
export function toUserTz(v: string): Dayjs {
  return toTz(v, userTimezone())
}

/** 以指定时区构造某日零点；dateKey 为 YYYY-MM-DD */
export function tzDate(dateKey: string, tz: string): Dayjs {
  return dayjs.tz(dateKey, tz)
}

/** 用户时区的当前时间 */
export function nowInUserTz(): Dayjs {
  return dayjs().tz(userTimezone())
}

/** 用户时区今天的 dateKey */
export function todayKey(): string {
  return nowInUserTz().format('YYYY-MM-DD')
}

export function dayKeyOf(d: Dayjs): string {
  return d.format('YYYY-MM-DD')
}

/** 一天内的分钟数 0..1439 */
export function minutesOfDay(d: Dayjs): number {
  return d.hour() * 60 + d.minute()
}

// ---- 展示格式化（均为用户时区） ----
export function fmtTime(v: string): string {
  return toUserTz(v).format('HH:mm')
}
export function fmtDateKey(v: string): string {
  return toUserTz(v).format('YYYY-MM-DD')
}
export function fmtDateCn(v: string): string {
  return toUserTz(v).format('M月D日')
}
export function fmtDateTime(v: string): string {
  return toUserTz(v).format('M月D日 HH:mm')
}

// ---- 全天事件（UTC 日期语义） ----
export function allDayDateKey(v: string): string {
  return dayjs.utc(v).format('YYYY-MM-DD')
}
/** 全天事件的排他结束日期前一天（含） */
export function allDayEndKeyInclusive(v: string): string {
  return dayjs.utc(v).subtract(1, 'day').format('YYYY-MM-DD')
}

/** 把用户时区的日期 + 时间按墙钟语义合成为 UTC RFC3339（正确处理夏令时） */
export function combineToUtc(dateKey: string, hour: number, minute: number, tz: string): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return dayjs.tz(`${dateKey}T${pad(hour)}:${pad(minute)}:00`, tz).utc().format()
}

/**
 * 周期性刷新的当前时间（用户时区），组件卸载自动清理定时器。
 * 用于今日高亮与周/日视图的当前时间指示线。
 */
export function useNow(intervalMs = 60000): import('vue').Ref<Dayjs> {
  const now = ref<Dayjs>(nowInUserTz())
  const timer = setInterval(() => {
    now.value = nowInUserTz()
  }, intervalMs)
  onUnmounted(() => clearInterval(timer))
  return now
}
