// 预约配置与我的预约 API（owner: frontend-b）。契约 §5。
import { get, post, patch, del } from './client'

export interface AvailabilityRule { weekday: number; start_minute: number; end_minute: number }
export interface AvailabilityDate { date: string; kind: 'available' | 'unavailable'; start_minute: number; end_minute: number }
export interface AvailabilitySet {
  id: number
  name: string
  timezone: string
  is_default: boolean
  rules: AvailabilityRule[]
  dates: AvailabilityDate[]
}

export interface EventType {
  id: number
  slug: string
  name: string
  description: string
  duration_minutes: number
  location_type: 'none' | 'online' | 'in_person' | 'phone'
  location_value: string
  color: string
  buffer_before: number
  buffer_after: number
  slot_interval: number
  min_notice_minutes: number
  range_type: 'rolling' | 'fixed'
  range_days: number
  start_date: string | null
  end_date: string | null
  availability_id: number | null
  calendar_id: number
  active: boolean
  booking_url: string
  created_at: string
  updated_at: string
}

export interface ExternalFeed {
  id: number
  name: string
  url: string
  color: string
  last_synced_at: string | null
  last_error: string
}

export interface MyBooking {
  id: number
  event_type: { id: number; name: string; slug: string }
  guest_name: string
  guest_email: string
  guest_note: string
  start: string
  end: string
  status: string
  created_at: string
}

export const bookingApi = {
  availabilitySets: () => get<AvailabilitySet[]>('/api/v1/availability-sets'),
  createAvailabilitySet: (p: { name: string; timezone: string; is_default?: boolean; rules: AvailabilityRule[]; dates?: AvailabilityDate[] }) =>
    post<AvailabilitySet>('/api/v1/availability-sets', p),
  updateAvailabilitySet: (id: number, p: Partial<{ name: string; timezone: string; is_default: boolean; rules: AvailabilityRule[]; dates: AvailabilityDate[] }>) =>
    patch<AvailabilitySet>(`/api/v1/availability-sets/${id}`, p),
  deleteAvailabilitySet: (id: number) => del<null>(`/api/v1/availability-sets/${id}`),
  eventTypes: () => get<EventType[]>('/api/v1/event-types'),
  createEventType: (p: Partial<EventType>) => post<EventType>('/api/v1/event-types', p),
  updateEventType: (id: number, p: Partial<EventType>) => patch<EventType>(`/api/v1/event-types/${id}`, p),
  deleteEventType: (id: number) => del<null>(`/api/v1/event-types/${id}`),
  feeds: () => get<ExternalFeed[]>('/api/v1/external-feeds'),
  createFeed: (p: { name: string; url: string; color?: string }) => post<ExternalFeed>('/api/v1/external-feeds', p),
  deleteFeed: (id: number) => del<null>(`/api/v1/external-feeds/${id}`),
  syncFeed: (id: number) => post<{ events: number; last_error: string }>(`/api/v1/external-feeds/${id}/sync`, {}),
  myBookings: (params: { from?: string; to?: string; status?: string; page?: number; page_size?: number }) => {
    const q = new URLSearchParams()
    if (params.from) q.set('from', params.from)
    if (params.to) q.set('to', params.to)
    if (params.status) q.set('status', params.status)
    q.set('page', String(params.page ?? 1))
    q.set('page_size', String(params.page_size ?? 20))
    return import('./client').then(m => m.getList<MyBooking>(`/api/v1/me/bookings?${q}`))
  },
  cancelBooking: (id: number, reason?: string) => post<null>(`/api/v1/bookings/${id}/cancel`, { reason }),
}
