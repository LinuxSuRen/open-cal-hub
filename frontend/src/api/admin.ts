// 管理端 API（owner: frontend-b）。契约 §6。
import { get, patch, put, post, del } from './client'
import type { User } from './types'

export interface AdminStats {
  users: number
  active_users_30d: number
  calendars: number
  events: number
  bookings_30d: number
  pending_outbox: number
}

export interface AdminUser extends User {
  calendars_count?: number
}

export interface AdminBooking {
  id: number
  event_type_name: string
  host_display_name: string
  guest_name: string
  guest_email: string
  start: string
  end: string
  status: string
  created_at: string
}

export interface AuditLog {
  id: number
  actor_id: number | null
  action: string
  target_type: string
  target_id: string
  detail: Record<string, unknown>
  ip: string
  created_at: string
}

export interface SiteSettings {
  site_name: string
  registration_mode: 'open' | 'invite'
  require_email_verification: boolean
  booking_horizon_days: number
}

export interface OutboxMail {
  id: number
  to_addr: string
  subject: string
  body_text: string
  kind: string
  status: string
  created_at: string
}

export const adminApi = {
  stats: () => get<AdminStats>('/api/v1/admin/stats'),
  users: (params: { query?: string; status?: string; role?: string; page?: number; page_size?: number }) => {
    const q = new URLSearchParams()
    if (params.query) q.set('query', params.query)
    if (params.status) q.set('status', params.status)
    if (params.role) q.set('role', params.role)
    q.set('page', String(params.page ?? 1))
    q.set('page_size', String(params.page_size ?? 20))
    return import('./client').then(m => m.getList<AdminUser>(`/api/v1/admin/users?${q}`))
  },
  updateUser: (id: number, p: { status?: string; role?: string }) => patch<User>(`/api/v1/admin/users/${id}`, p),
  deleteUser: (id: number) => del<null>(`/api/v1/admin/users/${id}`),
  sendReset: (id: number) => post<null>(`/api/v1/admin/users/${id}/send-reset`, {}),
  bookings: (params: { query?: string; status?: string; page?: number; page_size?: number }) => {
    const q = new URLSearchParams()
    if (params.query) q.set('query', params.query)
    if (params.status) q.set('status', params.status)
    q.set('page', String(params.page ?? 1))
    q.set('page_size', String(params.page_size ?? 20))
    return import('./client').then(m => m.getList<AdminBooking>(`/api/v1/admin/bookings?${q}`))
  },
  auditLogs: (page = 1, pageSize = 20) => import('./client').then(m => m.getList<AuditLog>(`/api/v1/admin/audit-logs?page=${page}&page_size=${pageSize}`)),
  settings: () => get<SiteSettings>('/api/v1/admin/settings'),
  updateSettings: (p: Partial<SiteSettings>) => put<SiteSettings>('/api/v1/admin/settings', p),
  emails: (page = 1, pageSize = 20) => import('./client').then(m => m.getList<OutboxMail>(`/api/v1/admin/emails?page=${page}&page_size=${pageSize}`)),
  retryEmail: (id: number) => post<null>(`/api/v1/admin/emails/${id}/retry`, {}),
  inviteCodes: () => get<{ code: string; created_at: string; expires_at: string | null; used_at: string | null }[]>('/api/v1/admin/invite-codes'),
  createInviteCodes: (count = 1, expiresInHours = 168) => post<{ codes: string[] }>('/api/v1/admin/invite-codes', { count, expires_in_hours: expiresInHours }),
  deleteInviteCode: (code: string) => del<null>(`/api/v1/admin/invite-codes/${encodeURIComponent(code)}`),
}
