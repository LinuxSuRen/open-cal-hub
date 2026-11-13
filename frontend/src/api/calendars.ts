// 日历与事件 API（owner: frontend-a）。契约 §3。
import { get, post, patch, del } from './client'
import type { EventInstance } from './types'

export interface Calendar {
  id: number
  name: string
  description: string
  color: string
  timezone: string
  is_default: boolean
  owner: { id: number; username: string; display_name: string }
  role: 'owner' | 'editor' | 'viewer'
  created_at: string
}

export interface ShareLink {
  id: number
  token: string
  url: string
  label: string
  created_at: string
  expires_at: string | null
}

export interface CalendarMember {
  user_id: number
  username: string
  display_name: string
  role: 'viewer' | 'editor'
  created_at: string
}

export interface EventInput {
  summary: string
  start: string
  end: string
  all_day?: boolean
  tz?: string
  location?: string
  description?: string
  status?: string
  reminders?: number[]
  rrule?: { freq: 'DAILY' | 'WEEKLY' | 'MONTHLY' | 'YEARLY'; interval?: number; byday?: string[]; until?: string; count?: number }
}

export const calApi = {
  list: () => get<Calendar[]>('/api/v1/calendars'),
  create: (p: { name: string; color?: string; description?: string; timezone?: string }) => post<Calendar>('/api/v1/calendars', p),
  update: (id: number, p: Partial<{ name: string; color: string; description: string; timezone: string }>) => patch<Calendar>(`/api/v1/calendars/${id}`, p),
  remove: (id: number) => del<null>(`/api/v1/calendars/${id}`),
  events: (calId: number, from: string, to: string, includeCancelled = false) => {
    const q = new URLSearchParams({ from, to, include_cancelled: String(includeCancelled) })
    return get<EventInstance[]>(`/api/v1/calendars/${calId}/events?${q.toString()}`)
  },
  createEvent: (calId: number, p: EventInput) => post<EventInstance>(`/api/v1/calendars/${calId}/events`, p),
  updateEvent: (eventId: number, p: Partial<EventInput>, scope: 'single' | 'all' = 'all') =>
    patch<EventInstance>(`/api/v1/events/${eventId}?scope=${scope}`, p),
  deleteEvent: (eventId: number, scope: 'single' | 'all' = 'all') => del<null>(`/api/v1/events/${eventId}?scope=${scope}`),
  importIcs: (calId: number, ics: string) => post<{ imported: number; skipped: number }>(`/api/v1/calendars/${calId}/import`, { ics }),
  shareLinks: (calId: number) => get<ShareLink[]>(`/api/v1/calendars/${calId}/share-links`),
  createShareLink: (calId: number, p: { label?: string; expires_at?: string }) => post<ShareLink>(`/api/v1/calendars/${calId}/share-links`, p),
  revokeShareLink: (id: number) => del<null>(`/api/v1/share-links/${id}`),
  members: (calId: number) => get<CalendarMember[]>(`/api/v1/calendars/${calId}/members`),
  addMember: (calId: number, p: { username_or_email: string; role: 'viewer' | 'editor' }) => post<null>(`/api/v1/calendars/${calId}/members`, p),
  removeMember: (calId: number, userId: number) => del<null>(`/api/v1/calendars/${calId}/members/${userId}`),
}
