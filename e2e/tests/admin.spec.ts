// 组 5：管理台——bootstrap 管理员、stats 非零、停用用户后登录被拒、邮件出箱分页。
// （文件名字母序保证本文件先跑：rick 在此完成 bootstrap 管理员断言）
import { test, expect } from '@playwright/test';
import { api, ensureAdmin, ensureUser, login, userID, PASSWORD } from './utils/api';

test.describe.serial('管理台', () => {
  let admin: string;

  test('bootstrap 管理员生效（空库首启创建，已验证）', async () => {
    admin = await ensureAdmin();
    const me = await api('GET', '/api/v1/me', undefined, admin);
    expect(me.status).toBe(200);
    expect(me.data.data.role).toBe('admin');
    expect(me.data.data.email_verified).toBe(true);
  });

  test('stats：注册与事件写入后计数非零', async () => {
    // 造数据：普通用户 bob 建日历与事件
    const bob = await ensureUser('bob');
    const cal = await api('POST', '/api/v1/calendars', { name: '工作', timezone: 'Asia/Shanghai' }, bob);
    expect(cal.status).toBe(201);
    const ev = await api('POST', `/api/v1/calendars/${cal.data.data.id}/events`, {
      summary: '既定日程', start: '2026-12-07T09:00:00Z', end: '2026-12-07T10:00:00Z', tz: 'UTC',
    }, bob);
    expect(ev.status).toBe(201);

    const stats = await api('GET', '/api/v1/admin/stats', undefined, admin);
    expect(stats.status).toBe(200);
    const d = stats.data.data;
    expect(d.users).toBeGreaterThanOrEqual(2);
    expect(d.calendars).toBeGreaterThanOrEqual(1);
    expect(d.events).toBeGreaterThanOrEqual(1);
    expect(d.pending_outbox).toBeGreaterThanOrEqual(0);
  });

  test('停用用户后登录被拒', async () => {
    const bob = await login('bob');
    const id = await userID(bob);
    const patch = await api('PATCH', `/api/v1/admin/users/${id}`, { status: 'disabled' }, admin);
    expect(patch.status).toBe(200);
    const r = await api('POST', '/api/v1/auth/login', { email: 'bob@example.com', password: PASSWORD });
    expect(r.status).toBe(403);
    expect(r.data.error.code).toBe('auth/account_disabled');
    // 恢复活跃，供后续组使用（bob 仅本组使用，此处保持整洁）
    await api('PATCH', `/api/v1/admin/users/${id}`, { status: 'active' }, admin);
  });

  test('邮件出箱分页', async () => {
    const page1 = await api('GET', '/api/v1/admin/emails?page=1&page_size=5', undefined, admin);
    expect(page1.status).toBe(200);
    expect(page1.data.total).toBeGreaterThanOrEqual(1);
    expect(Array.isArray(page1.data.data)).toBe(true);
    expect(page1.data.page).toBe(1);
    expect(page1.data.page_size).toBe(5);
    const first = page1.data.data[0];
    expect(first).toHaveProperty('kind');
    expect(first).toHaveProperty('body_text');
  });
});
