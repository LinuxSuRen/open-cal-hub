// IANA 时区清单：优先 Intl.supportedValuesOf，环境不支持时退回内置常用清单。
import { computed, type Ref } from 'vue'
import dayjs from 'dayjs'
import './useLocalTime' // 确保 timezone 插件已注册

const FALLBACK_ZONES = [
  'UTC',
  'Asia/Shanghai', 'Asia/Hong_Kong', 'Asia/Taipei', 'Asia/Tokyo', 'Asia/Seoul', 'Asia/Singapore',
  'Asia/Kolkata', 'Asia/Dubai', 'Asia/Bangkok', 'Asia/Jakarta',
  'Europe/London', 'Europe/Paris', 'Europe/Berlin', 'Europe/Madrid', 'Europe/Rome', 'Europe/Amsterdam',
  'Europe/Stockholm', 'Europe/Moscow', 'Europe/Istanbul',
  'America/New_York', 'America/Chicago', 'America/Denver', 'America/Los_Angeles', 'America/Toronto',
  'America/Sao_Paulo', 'America/Mexico_City',
  'Australia/Sydney', 'Australia/Melbourne', 'Pacific/Auckland',
]

export function listTimezones(): string[] {
  try {
    if (typeof Intl !== 'undefined' && typeof Intl.supportedValuesOf === 'function') {
      const list = Intl.supportedValuesOf('timeZone')
      if (Array.isArray(list) && list.length > 0) return list
    }
  } catch {
    // 忽略，走兜底清单
  }
  return FALLBACK_ZONES
}

export function guessTimezone(): string {
  try {
    return dayjs.tz.guess() || 'UTC'
  } catch {
    return 'UTC'
  }
}

export function isValidTimezone(tz: string): boolean {
  return dayjs.tz('2026-06-01T00:00:00Z', tz).isValid()
}

/** GMT+08:00 形式的偏移标签；无效时区返回空串 */
export function tzOffsetLabel(tz: string): string {
  if (!isValidTimezone(tz)) return ''
  return `GMT${dayjs().tz(tz).format('Z')}`
}

export interface TzOption {
  value: string
  label: string
}

/** 下拉选项；current 排在最前（不存在时也补入） */
export function timezoneOptions(current?: string): TzOption[] {
  const zones = listTimezones()
  const opts = zones.filter(z => z !== current).map(z => ({ value: z, label: `${z}  ${tzOffsetLabel(z)}`.trim() }))
  if (current) {
    if (isValidTimezone(current)) opts.unshift({ value: current, label: `${current}  ${tzOffsetLabel(current)}` })
    else opts.unshift({ value: current, label: current })
  }
  return opts
}

/** 响应式选项：current 为 getter，用户时区变化时自动重排 */
export function useTimezones(current?: Ref<string | undefined> | (() => string | undefined)) {
  const get = typeof current === 'function' ? current : current ? () => current.value : undefined
  const options = computed(() => timezoneOptions(get?.()))
  return { options, guessTimezone }
}
