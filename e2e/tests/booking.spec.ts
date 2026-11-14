// 组 4：预约全链——配置可用时间+类型 → 游客选槽提交 → 成功响应 →
// 主人可见 → 游客 manage 链接取消 → 槽位重新可用。
import { test, expect } from '@playwright/test';
import { api, setupHost, firstSlot } from './utils/api';

test.describe.serial('预约全链', () => {
  const host = 'hoster';
  let slot: string;

  test('主人配置可用时间与预约类型', async () => {
    const token = await setupHost(host);
    const profile = await api('GET', `/api/v1/pub/u/${host}`, undefined, undefined);
    expect(profile.status).toBe(200);
    const types = profile.data.data.event_types as any[];
    expect(types.some((t) => t.slug === 'intro')).toBe(true);
  });

  test('游客查看槽位并提交预约', async () => {
    slot = await firstSlot(host, 'intro');
    const r = await api('POST', `/api/v1/pub/u/${host}/intro/bookings`, {
      start: slot, name: '来访者', email: 'guest@example.com', note: 'e2e 预约', timezone: 'Asia/Shanghai',
    });
    expect(r.status).toBe(201);
    const d = r.data.data;
    expect(d.booking.start).toBe(slot);
    expect(d.booking.event_type_name).toBe('介绍会谈');
    expect(d.manage_url).toMatch(/^\/b\//);
    // 存到 process.env 供后续用例复用（单 worker 串行，无竞争）
    process.env.E2E_MANAGE_URL = d.manage_url as string;
  });

  test('已约槽位从列表消失', async () => {
    const again = await firstSlot(host, 'intro');
    expect(again).not.toBe(slot);
  });

  test('主人视角可见预约', async () => {
    const token = await setupHost(host); // 幂等：已存在则登录
    const r = await api('GET', '/api/v1/me/bookings?status=confirmed', undefined, token);
    expect(r.status).toBe(200);
    expect(r.data.total).toBe(1);
    expect(r.data.data[0].guest_name).toBe('来访者');
    expect(r.data.data[0].event_type.slug).toBe('intro');
  });

  test('游客凭管理链接查看预约', async () => {
    const url = process.env.E2E_MANAGE_URL as string;
    const r = await api('GET', `/api/v1/pub${url}`);
    expect(r.status).toBe(200);
    expect(r.data.data.host_username).toBe(host);
    expect(r.data.data.status).toBe('confirmed');
    expect(r.data.data.event_type.slug).toBe('intro');
  });

  test('乱令牌 → booking/not_found', async () => {
    const r = await api('GET', '/api/v1/pub/b/deadbeefdeadbeef');
    expect(r.status).toBe(404);
    expect(r.data.error.code).toBe('booking/not_found');
  });

  test('游客取消后槽位重新可用', async () => {
    const url = process.env.E2E_MANAGE_URL as string;
    const cancel = await api('POST', `/api/v1/pub${url}/cancel`, { reason: '计划有变' });
    expect(cancel.status).toBe(200);

    const again = await firstSlot(host, 'intro');
    expect(again).toBe(slot); // 取消后原槽恢复为第一个可用槽
  });

  test('蜜罐字段假成功不入库', async () => {
    const s = await firstSlot(host, 'intro');
    const r = await api('POST', `/api/v1/pub/u/${host}/intro/bookings`, {
      start: s, name: '机器人', email: 'bot@example.com', company: 'spam-inc',
    });
    expect(r.status).toBe(201);
    // 机器人预约不影响后续真实下单（同槽仍可约）
    const real = await api('POST', `/api/v1/pub/u/${host}/intro/bookings`, {
      start: s, name: '真人', email: 'real@example.com',
    });
    expect(real.status).toBe(201);
    // 记录已占槽位，供并发组跳过（单 worker 串行共享 env）
    process.env.E2E_BOOKED_SLOT = s;
  });
});
