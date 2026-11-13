// 公开页文案（owner: frontend-b）+ 错误码文案注册表（契约 §0 错误码段位表的前端映射）。
import { getLocale } from './index'

export const messages = {
  // 通用公开页
  'pub.hostNotFound.title': { 'zh-CN': '找不到该用户', en: 'User not found' },
  'pub.hostNotFound.desc': { 'zh-CN': '链接可能有误，或对方已关闭公开预约。', en: 'The link may be wrong, or public booking is closed.' },
  'pub.typeNotFound.title': { 'zh-CN': '预约类型不存在', en: 'Event type not found' },
  'pub.typeNotFound.desc': { 'zh-CN': '该预约项目可能已被删除或停用。', en: 'This booking item may have been deleted or disabled.' },
  'pub.shareNotFound.title': { 'zh-CN': '分享链接不存在或已撤销', en: 'Share link not found or revoked' },
  'pub.shareNotFound.desc': { 'zh-CN': '请向分享者确认链接是否仍然有效。', en: 'Please confirm with the owner whether the link is still valid.' },
  'pub.shareExpired.title': { 'zh-CN': '分享链接已过期', en: 'Share link expired' },
  'pub.shareExpired.desc': { 'zh-CN': '请联系分享者重新生成分享链接。', en: 'Please ask the owner to create a new share link.' },
  'pub.backHome': { 'zh-CN': '返回首页', en: 'Back to home' },
  'pub.poweredBy': { 'zh-CN': '由 CalHub 提供日程服务', en: 'Powered by CalHub' },

  // 预约主页 /u/{username}
  'pub.profile.bookWith': { 'zh-CN': '预约', en: 'Book with' },
  'pub.profile.noTypes': { 'zh-CN': '对方暂未开放可预约项目', en: 'No bookable items yet' },
  'pub.profile.hostTz': { 'zh-CN': '时区', en: 'Timezone' },
  'pub.minutes': { 'zh-CN': '分钟', en: 'min' },
  'pub.duration.m': { 'zh-CN': '{n} 分钟', en: '{n} min' },

  // 预约页 /u/{username}/{slug}
  'pub.tzLabel': { 'zh-CN': '时区', en: 'Timezone' },
  'pub.tzLocal': { 'zh-CN': '浏览器时区', en: 'Browser timezone' },
  'pub.step.date': { 'zh-CN': '选择日期', en: 'Pick a date' },
  'pub.step.slot': { 'zh-CN': '选择时间', en: 'Pick a time' },
  'pub.step.form': { 'zh-CN': '确认信息', en: 'Your details' },
  'pub.slot.morning': { 'zh-CN': '上午', en: 'Morning' },
  'pub.slot.afternoon': { 'zh-CN': '下午', en: 'Afternoon' },
  'pub.slot.evening': { 'zh-CN': '晚间', en: 'Evening' },
  'pub.noSlots': { 'zh-CN': '该日期暂无可预约时间', en: 'No available times on this date' },
  'pub.noSlotsInRange': { 'zh-CN': '这一段时间内没有可预约的日期', en: 'No bookable dates in this range' },
  'pub.prevRange': { 'zh-CN': '往前', en: 'Earlier' },
  'pub.nextRange': { 'zh-CN': '往后', en: 'Later' },
  'pub.form.name': { 'zh-CN': '姓名', en: 'Your name' },
  'pub.form.namePh': { 'zh-CN': '请输入姓名', en: 'Enter your name' },
  'pub.form.email': { 'zh-CN': '邮箱', en: 'Email' },
  'pub.form.emailPh': { 'zh-CN': '用于接收预约确认邮件', en: 'For booking confirmation' },
  'pub.form.note': { 'zh-CN': '备注', en: 'Note' },
  'pub.form.notePh': { 'zh-CN': '有什么需要提前告知的（可选）', en: 'Anything to share beforehand (optional)' },
  'pub.form.invalidEmail': { 'zh-CN': '邮箱格式不正确', en: 'Invalid email address' },
  'pub.form.required': { 'zh-CN': '此项必填', en: 'Required' },
  'pub.form.back': { 'zh-CN': '返回', en: 'Back' },
  'pub.form.submit': { 'zh-CN': '确认预约', en: 'Confirm booking' },
  'pub.form.submitting': { 'zh-CN': '提交中…', en: 'Submitting…' },
  'pub.booked.title': { 'zh-CN': '预约成功', en: 'Booking confirmed' },
  'pub.booked.desc': { 'zh-CN': '确认邮件已发送，请在赴约前留意时间。', en: 'A confirmation email has been sent.' },
  'pub.booked.manage': { 'zh-CN': '管理此预约', en: 'Manage this booking' },
  'pub.booked.again': { 'zh-CN': '再约一个时间', en: 'Book another time' },
  'pub.slotTaken': { 'zh-CN': '这个时间段刚被占用，已为你刷新，请另选时间', en: 'That slot was just taken. Times refreshed, please pick another.' },

  // 预约管理页 /b/{token}
  'pub.manage.title': { 'zh-CN': '预约详情', en: 'Booking details' },
  'pub.manage.host': { 'zh-CN': '主人', en: 'Host' },
  'pub.manage.status': { 'zh-CN': '状态', en: 'Status' },
  'pub.manage.guest': { 'zh-CN': '来客', en: 'Guest' },
  'pub.manage.note': { 'zh-CN': '备注', en: 'Note' },
  'pub.manage.time': { 'zh-CN': '时间', en: 'Time' },
  'pub.manage.reschedule': { 'zh-CN': '改期', en: 'Reschedule' },
  'pub.manage.reschedPick': { 'zh-CN': '选择新的时间', en: 'Pick a new time' },
  'pub.manage.reschedOk': { 'zh-CN': '已改期', en: 'Rescheduled' },
  'pub.manage.reschedConfirm': { 'zh-CN': '确认改期', en: 'Confirm reschedule' },
  'pub.manage.cancel': { 'zh-CN': '取消预约', en: 'Cancel booking' },
  'pub.manage.cancelReason': { 'zh-CN': '取消原因', en: 'Reason' },
  'pub.manage.cancelReasonPh': { 'zh-CN': '可选，会告知对方', en: 'Optional, will be shared with the host' },
  'pub.manage.cancelConfirm': { 'zh-CN': '确认取消', en: 'Confirm cancellation' },
  'pub.manage.cancelled': { 'zh-CN': '该预约已取消', en: 'This booking is cancelled' },
  'pub.manage.fallbackTitle': { 'zh-CN': '手动指定时间', en: 'Specify a time' },
  'pub.manage.fallbackDate': { 'zh-CN': '日期', en: 'Date' },
  'pub.manage.fallbackTime': { 'zh-CN': '时间', en: 'Time' },

  // 分享视图 /s/{token}
  'pub.share.viewMonth': { 'zh-CN': '月视图', en: 'Month' },
  'pub.share.viewList': { 'zh-CN': '事件列表', en: 'List' },
  'pub.share.today': { 'zh-CN': '今天', en: 'Today' },
  'pub.share.noEvents': { 'zh-CN': '本月暂无事件', en: 'No events this month' },
  'pub.share.dayEvents': { 'zh-CN': '当日事件', en: 'Events on this day' },
  'pub.share.ics': { 'zh-CN': '订阅 ICS 日历', en: 'Subscribe via ICS' },
  'pub.share.icsHint': { 'zh-CN': '添加到你的日历应用', en: 'Add to your calendar app' },
  'pub.share.allDay': { 'zh-CN': '全天', en: 'All day' },
  'pub.wd.0': { 'zh-CN': '日', en: 'Sun' },
  'pub.wd.1': { 'zh-CN': '一', en: 'Mon' },
  'pub.wd.2': { 'zh-CN': '二', en: 'Tue' },
  'pub.wd.3': { 'zh-CN': '三', en: 'Wed' },
  'pub.wd.4': { 'zh-CN': '四', en: 'Thu' },
  'pub.wd.5': { 'zh-CN': '五', en: 'Fri' },
  'pub.wd.6': { 'zh-CN': '六', en: 'Sat' },
} as const

// 错误码 → 本地文案。契约 §0；未知 code 由 errorText 回退到服务端 message。
export const errorMessages: Record<string, { 'zh-CN': string; en: string }> = {
  'common/validation_failed': { 'zh-CN': '信息有误，请检查后重试', en: 'Invalid input, please check and retry' },
  'common/rate_limited': { 'zh-CN': '请求过于频繁，请稍后再试', en: 'Too many requests, please retry later' },
  'common/unauthorized': { 'zh-CN': '登录已过期，请重新登录', en: 'Session expired, please sign in again' },
  'common/forbidden': { 'zh-CN': '没有权限执行此操作', en: 'You are not allowed to do this' },
  'common/not_found': { 'zh-CN': '内容不存在或已被删除', en: 'Not found or deleted' },
  'common/internal': { 'zh-CN': '服务暂时不可用，请稍后重试', en: 'Service temporarily unavailable' },
  'timeout': { 'zh-CN': '请求超时，请检查网络后重试', en: 'Request timed out' },
  'network': { 'zh-CN': '无法连接服务器，请检查网络', en: 'Cannot reach the server' },
  'auth/weak_password': { 'zh-CN': '密码强度不足：至少 8 位且同时包含字母与数字', en: 'Weak password: at least 8 chars with letters and digits' },
  'auth/invalid_credentials': { 'zh-CN': '邮箱或密码不正确', en: 'Incorrect email or password' },
  'auth/email_not_verified': { 'zh-CN': '请先完成邮箱验证', en: 'Please verify your email first' },
  'cal/share_not_found': { 'zh-CN': '分享链接不存在或已撤销', en: 'Share link not found or revoked' },
  'cal/share_expired': { 'zh-CN': '分享链接已过期', en: 'Share link expired' },
  'booking/event_type_not_found': { 'zh-CN': '预约类型不存在', en: 'Event type not found' },
  'booking/inactive': { 'zh-CN': '该预约类型已停用', en: 'This booking type is disabled' },
  'booking/host_not_found': { 'zh-CN': '用户不存在', en: 'User not found' },
  'booking/slot_unavailable': { 'zh-CN': '该时间段已被占用，请选择其他时间', en: 'That time is taken, please pick another' },
  'booking/outside_range': { 'zh-CN': '超出可预约日期范围', en: 'Outside the bookable date range' },
  'booking/insufficient_notice': { 'zh-CN': '距开始时间太近，请选择更晚的时间', en: 'Too close to the start time, pick a later one' },
  'booking/invalid_slot': { 'zh-CN': '时间段无效，请重新选择', en: 'Invalid time slot, please pick again' },
  'booking/not_found': { 'zh-CN': '预约不存在', en: 'Booking not found' },
  'booking/already_cancelled': { 'zh-CN': '该预约已取消', en: 'This booking is already cancelled' },
  'admin/invalid_setting': { 'zh-CN': '设置项无效', en: 'Invalid setting' },
}

/** 按 code 取本地文案；未登记的 code 回退 fallback（服务端 message）。 */
export function errorText(code: string, fallback: string): string {
  const entry = errorMessages[code]
  if (entry) return entry[getLocale()] ?? entry['zh-CN']
  return fallback || errorText('common/internal', '')
}

/** 适配 ApiError 形状的便捷入口。 */
export function apiErrorText(err: { code?: string; message?: string } | null | undefined): string {
  const code = err?.code ?? 'network'
  return errorText(code, err?.message ?? '')
}
