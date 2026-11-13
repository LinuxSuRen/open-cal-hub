// 时区工具：浏览器时区探测 + 常用时区清单（公开页访客选择、设置页资料）。
import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'
import timezone from 'dayjs/plugin/timezone'

dayjs.extend(utc)
dayjs.extend(timezone)

export function browserTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
  } catch {
    return 'UTC'
  }
}

/** 当前偏移标签，如 "GMT+08:00"。 */
export function tzOffsetLabel(tz: string): string {
  try {
    return `GMT${dayjs.tz(Date.now(), tz).format('ZZ')}`
  } catch {
    return ''
  }
}

/** 常用时区（含常见中文用户区域）；合并主机时区由调用方追加。 */
export const COMMON_TIMEZONES: string[] = [
  'Pacific/Honolulu',
  'America/Los_Angeles',
  'America/Denver',
  'America/Chicago',
  'America/New_York',
  'America/Sao_Paulo',
  'Europe/London',
  'Europe/Paris',
  'Europe/Berlin',
  'Europe/Moscow',
  'Africa/Cairo',
  'Asia/Dubai',
  'Asia/Karachi',
  'Asia/Kolkata',
  'Asia/Bangkok',
  'Asia/Shanghai',
  'Asia/Hong_Kong',
  'Asia/Taipei',
  'Asia/Singapore',
  'Asia/Tokyo',
  'Asia/Seoul',
  'Australia/Sydney',
  'Pacific/Auckland',
  'UTC',
]

/** 去重合并：常用清单 + 追加项（主机/浏览器时区置顶展示由调用方处理）。 */
export function timezoneOptions(...extra: string[]): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const tz of [...extra.filter(Boolean), ...COMMON_TIMEZONES]) {
    if (!seen.has(tz)) {
      seen.add(tz)
      out.push(tz)
    }
  }
  return out
}
