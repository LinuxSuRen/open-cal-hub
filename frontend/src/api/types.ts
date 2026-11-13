// 共享领域类型（与 docs/api-contract.md 对齐；各领域专属类型放各自领域文件）。

export interface User {
  id: number
  email: string
  username: string
  display_name: string
  bio: string
  timezone: string
  locale: string
  role: string
  status: string
  email_verified: boolean
  created_at: string
  last_login_at: string | null
}

export interface Meta {
  site_name: string
  version: string
  registration_mode: string
  features: Record<string, boolean>
  default_locale: string
}

/** 事件实例（后端已展开重复规则） */
export interface EventInstance {
  id: number
  uid: string
  calendar_id: number
  summary: string
  location: string
  description: string
  start: string
  end: string
  all_day: boolean
  tz: string
  status: string
  rrule: string
  recurring: boolean
  recurrence_id: string | null
  reminders: number[]
  master_id: number
}
