// 日历组件共享常量与纯辅助
import { t } from '../../i18n'

/** 日历颜色候选 */
export const CALENDAR_COLORS = ['#4F6EF7', '#7C5CFC', '#1F9D61', '#0E9BB5', '#B97E0D', '#D5484F', '#E0598C', '#5B7043']

/** 网格表头顺序：周一在前；weekday 0=周日（契约） */
export const GRID_WEEKDAYS = [1, 2, 3, 4, 5, 6, 0] as const

export function weekdayLabel(n: number): string {
  return t(`date.weekday.${n}`)
}

/** 两位数字补零 */
export function pad2(n: number): string {
  return String(n).padStart(2, '0')
}

export function fillParam(text: string, key: string, value: number | string): string {
  return text.replace(`{${key}}`, String(value))
}

/** 提醒文案：0=开始时；按天/小时/分钟分级 */
export function reminderText(n: number): string {
  if (n <= 0) return t('ev.reminders.atStart')
  if (n % 1440 === 0) return fillParam(t('ev.reminders.beforeDays'), 'n', n / 1440)
  if (n % 60 === 0) return fillParam(t('ev.reminders.beforeHours'), 'n', n / 60)
  return fillParam(t('ev.reminders.beforeMinutes'), 'n', n)
}

/** 事件柔和底色：6 位十六进制 + 12% 透明度 */
export function softBg(hex: string): string {
  const m = /^#?([0-9a-fA-F]{6})$/.exec(hex.trim())
  return m ? `#${m[1]}1F` : hex
}
