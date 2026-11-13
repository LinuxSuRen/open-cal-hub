// 认证与账号 API（owner: frontend-a）。契约 §2。
import { get, post, patch, del } from './client'
import type { User } from './types'

export const authApi = {
  meta: () => get<import('./types').Meta>('/api/v1/meta'),
  login: (email: string, password: string) =>
    post<{ access_token: string; expires_in: number; user: User }>('/api/v1/auth/login', { email, password }),
  register: (p: { email: string; password: string; username?: string; display_name?: string; timezone?: string; locale?: string; invite_code?: string }) =>
    post<{ user: User; verification_required: boolean }>('/api/v1/auth/register', p),
  logout: () => post<null>('/api/v1/auth/logout'),
  refreshHandoff: undefined,
  verifyEmail: (token: string) => post<null>('/api/v1/auth/verify-email', { token }),
  resendVerification: (email: string) => post<null>('/api/v1/auth/resend-verification', { email }),
  forgotPassword: (email: string) => post<null>('/api/v1/auth/forgot-password', { email }),
  resetPassword: (token: string, newPassword: string) => post<null>('/api/v1/auth/reset-password', { token, new_password: newPassword }),
  sessions: () => get<{ id: number; user_agent: string; ip: string; created_at: string; last_used_at: string | null; current: boolean }[]>('/api/v1/auth/sessions'),
  revokeSession: (id: number) => del<null>(`/api/v1/auth/sessions/${id}`),
  me: () => get<User>('/api/v1/me'),
  updateMe: (p: Partial<Pick<User, 'display_name' | 'timezone' | 'locale' | 'bio'>>) => patch<User>('/api/v1/me', p),
  changePassword: (currentPassword: string, newPassword: string) => post<null>('/api/v1/me/password', { current_password: currentPassword, new_password: newPassword }),
  appPasswords: () => get<{ id: number; label: string; created_at: string; last_used_at: string | null }[]>('/api/v1/me/app-passwords'),
  createAppPassword: (label: string) => post<{ id: number; password: string }>('/api/v1/me/app-passwords', { label }),
  deleteAppPassword: (id: number) => del<null>(`/api/v1/me/app-passwords/${id}`),
}
