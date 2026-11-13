// 重复规则模型与 RRULE 字符串互转（纯逻辑）。weekday 语义遵循契约：0=周日。
export type RruleFreq = 'DAILY' | 'WEEKLY' | 'MONTHLY' | 'YEARLY'

export interface RruleModel {
  freq: RruleFreq
  interval: number
  byday: string[]
  endMode: 'never' | 'until' | 'count'
  /** YYYY-MM-DD */
  until: string
  count: number
}

export const RRULE_BYDAYS = ['MO', 'TU', 'WE', 'TH', 'FR', 'SA', 'SU'] as const

const BYDAY_ORDER: Record<string, number> = { MO: 0, TU: 1, WE: 2, TH: 3, FR: 4, SA: 5, SU: 6 }

export function weekdayNumToByday(n: number): string {
  return ['SU', 'MO', 'TU', 'WE', 'TH', 'FR', 'SA'][n] ?? 'MO'
}

export function bydayToWeekdayNum(b: string): number {
  const m: Record<string, number> = { MO: 1, TU: 2, WE: 3, TH: 4, FR: 5, SA: 6, SU: 0 }
  return m[b] ?? 1
}

function emptyModel(): RruleModel {
  return { freq: 'WEEKLY', interval: 1, byday: [], endMode: 'never', until: '', count: 10 }
}

/** 解析后端 rrule 字符串；空或无效返回 null（不重复） */
export function parseRrule(s: string | null | undefined): RruleModel | null {
  if (!s) return null
  const parts: Record<string, string> = {}
  for (const seg of s.split(';')) {
    const i = seg.indexOf('=')
    if (i > 0) parts[seg.slice(0, i)] = seg.slice(i + 1)
  }
  const freq = parts.FREQ as RruleFreq
  if (freq !== 'DAILY' && freq !== 'WEEKLY' && freq !== 'MONTHLY' && freq !== 'YEARLY') return null
  const m = emptyModel()
  m.freq = freq
  const interval = parseInt(parts.INTERVAL ?? '', 10)
  if (Number.isFinite(interval) && interval > 1) m.interval = interval
  m.byday = (parts.BYDAY ?? '')
    .split(',')
    .filter(d => BYDAY_ORDER[d] !== undefined)
    .sort((a, b) => BYDAY_ORDER[a] - BYDAY_ORDER[b])
  if (parts.UNTIL) {
    // 兼容 20261231T235959Z 与 2026-12-31 两种日期形式
    const digits = parts.UNTIL.replace(/\D/g, '')
    if (digits.length >= 8) {
      m.endMode = 'until'
      m.until = `${digits.slice(0, 4)}-${digits.slice(4, 6)}-${digits.slice(6, 8)}`
    }
  }
  if (m.endMode !== 'until' && parts.COUNT) {
    const count = parseInt(parts.COUNT, 10)
    if (Number.isFinite(count) && count > 0) {
      m.endMode = 'count'
      m.count = count
    }
  }
  return m
}

export type RrulePayload = {
  freq: RruleFreq
  interval?: number
  byday?: string[]
  until?: string
  count?: number
}

/** 模型 → 创建/更新接口的 rrule 字段；null 返回 undefined（不重复） */
export function buildRrulePayload(m: RruleModel | null): RrulePayload | undefined {
  if (!m) return undefined
  const p: RrulePayload = { freq: m.freq }
  if (m.interval > 1) p.interval = m.interval
  if (m.freq === 'WEEKLY' && m.byday.length > 0) {
    p.byday = [...m.byday].sort((a, b) => BYDAY_ORDER[a] - BYDAY_ORDER[b])
  }
  if (m.endMode === 'until' && m.until) {
    p.until = `${m.until}T235959Z`
  } else if (m.endMode === 'count' && m.count > 0) {
    p.count = m.count
  }
  return p
}
