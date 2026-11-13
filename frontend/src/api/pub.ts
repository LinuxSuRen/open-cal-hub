// 公开访问 API（owner: frontend-b）。契约 §4。
import { get, post } from './client'
import type { EventInstance } from './types'
import type { AvailabilityRule, AvailabilityDate } from './booking'

export interface PublicHost {
  display_name: string
  bio: string
  timezone: string
  event_types: { slug: string; name: string; duration_minutes: number; color: string }[]
}

export interface PublicEventType {
  slug: string
  name: string
  description: string
  duration_minutes: number
  color: string
  location_type: string
  range_type: 'rolling' | 'fixed'
  range_days: number
  start_date: string | null
  end_date: string | null
  availability: { timezone: string; rules: AvailabilityRule[]; dates: AvailabilityDate[] }
  buffers: { before: number; after: number }
  slot_interval: number
  min_notice_minutes: number
}

export interface SlotDay { date: string; slots: { start: string; end: string; available: boolean }[] }

export interface PublicShare {
  site_name: string
  calendar: { name: string; description: string; color: string }
  owner_display_name: string
  timezone: string
}

export interface GuestBookingView {
  id: number
  event_type_name: string
  host_display_name: string
  start: string
  end: string
  status: string
  created_at: string
}

export const pubApi = {
  host: (username: string) => get<PublicHost>(`/api/v1/pub/u/${encodeURIComponent(username)}`),
  eventType: (username: string, slug: string) => get<PublicEventType>(`/api/v1/pub/u/${encodeURIComponent(username)}/${encodeURIComponent(slug)}`),
  slots: (username: string, slug: string, start: string, days: number, tz: string) =>
    get<{ days: SlotDay[] }>(`/api/v1/pub/u/${encodeURIComponent(username)}/${encodeURIComponent(slug)}/slots?start=${start}&days=${days}&tz=${encodeURIComponent(tz)}`),
  book: (username: string, slug: string, p: { start: string; name: string; email: string; note?: string; timezone: string; company?: string }) =>
    post<{ booking: GuestBookingView; manage_url: string }>(`/api/v1/pub/u/${encodeURIComponent(username)}/${encodeURIComponent(slug)}/bookings`, p),
  share: (token: string) => get<PublicShare>(`/api/v1/pub/share/${encodeURIComponent(token)}`),
  shareEvents: (token: string, from: string, to: string) =>
    get<EventInstance[]>(`/api/v1/pub/share/${encodeURIComponent(token)}/events?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`),
  bookingByToken: (token: string) => get<{ event_type: { name: string; duration_minutes: number }; host_display_name: string; start: string; end: string; status: string; guest_name: string; guest_note: string; timezone: string }>(`/api/v1/pub/b/${encodeURIComponent(token)}`),
  cancelByToken: (token: string, reason?: string) => post<null>(`/api/v1/pub/b/${encodeURIComponent(token)}/cancel`, { reason }),
  rescheduleByToken: (token: string, start: string) => post<GuestBookingView>(`/api/v1/pub/b/${encodeURIComponent(token)}/reschedule`, { start }),
}
