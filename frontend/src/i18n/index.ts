// 极简 i18n：核心 + 各领域模块静态合并；zh-CN 为主，en 兜底。
// 领域文案文件：app.ts（frontend-a）、booking.ts / admin.ts / pub.ts（frontend-b）。
import { messages as coreMsgs } from './core'
import { messages as appMsgs } from './app'
import { messages as bookingMsgs } from './booking'
import { messages as adminMsgs } from './admin'
import { messages as pubMsgs } from './pub'

export type Locale = 'zh-CN' | 'en'
type Entry = { 'zh-CN': string; en: string }
type Dict = Record<string, Entry>

const dict: Dict = { ...coreMsgs, ...appMsgs, ...bookingMsgs, ...adminMsgs, ...pubMsgs }

let current: Locale = 'zh-CN'
export function setLocale(l: Locale) { current = l }
export function getLocale(): Locale { return current }

export function t(key: string): string {
  const entry = dict[key]
  if (!entry) return key
  return entry[current] ?? entry['zh-CN'] ?? key
}
