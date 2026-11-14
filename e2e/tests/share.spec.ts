// 组 3：分享链接——建链接 → 匿名访问公开视图 → 过期 410 → 撤销 404。
import { test, expect } from '@playwright/test';
import { api, ensureUser } from './utils/api';
import { apiOnlyMode } from '../utils/env';

test.describe.serial('分享链接', () => {
  const user = 'dave'; // 全轮共享用户（限流 5/h/IP，见 utils/api.ts）
  let token: string;
  let calID: number;
  let shareToken: string;
  let shareID: number;

  test.beforeAll(async () => {
    token = await ensureUser(user);
    const cal = await api('POST', '/api/v1/calendars', { name: '公开日历', timezone: 'Asia/Shanghai' }, token);
    calID = cal.data.data.id;
    const ev = await api('POST', `/api/v1/calendars/${calID}/events`, {
      summary: '公开事件', start: '2026-12-09T09:00:00Z', end: '2026-12-09T10:00:00Z', tz: 'UTC',
    }, token);
    expect(ev.status).toBe(201);
  });

  test('创建分享链接（匿名可见）', async () => {
    const r = await api('POST', `/api/v1/calendars/${calID}/share-links`, { label: 'e2e' }, token);
    expect(r.status).toBe(201);
    shareToken = r.data.data.token;
    expect(shareToken).toBeTruthy();
    shareID = r.data.data.id;

    const info = await api('GET', `/api/v1/pub/share/${shareToken}`);
    expect(info.status).toBe(200);
    expect(info.data.data.calendar.name).toBe('公开日历');

    const events = await api('GET', `/api/v1/pub/share/${shareToken}/events?from=2026-12-01T00:00:00Z&to=2026-12-31T00:00:00Z`);
    expect(events.status).toBe(200);
    expect((events.data.data as any[]).some((i) => i.summary === '公开事件')).toBe(true);
  });

  test('公开分享页渲染（浏览器可用时）', async ({ browser }) => {
    test.skip(apiOnlyMode, 'chromium 不可用：API 级断言已覆盖');
    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    await page.goto(`/s/${shareToken}`);
    await expect(page.locator('#app')).toBeAttached();
    await ctx.close();
  });

  test('过期分享 → 410 share_expired', async () => {
    const past = new Date(Date.now() - 3600_000).toISOString().replace(/\.\d+Z$/, 'Z');
    const r = await api('POST', `/api/v1/calendars/${calID}/share-links`, {
      label: '过期', expires_at: past,
    }, token);
    expect(r.status).toBe(201);
    const expired = await api('GET', `/api/v1/pub/share/${r.data.data.token}`);
    expect(expired.status).toBe(410);
    expect(expired.data.error.code).toBe('cal/share_expired');
  });

  test('撤销后匿名访问 → 404 share_not_found', async () => {
    const revoke = await api('DELETE', `/api/v1/share-links/${shareID}`, undefined, token);
    expect(revoke.status).toBe(200);
    const r = await api('GET', `/api/v1/pub/share/${shareToken}/events?from=2026-12-01T00:00:00Z&to=2026-12-31T00:00:00Z`);
    expect(r.status).toBe(404);
    expect(r.data.error.code).toBe('cal/share_not_found');
  });
});
