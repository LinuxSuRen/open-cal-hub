// 组 7：并发预约——同槽位两并发请求恰好一 201 一 409。
import { test, expect } from '@playwright/test';
import { api, setupHost, firstSlot, BASE } from './utils/api';

test('并发下单同槽位：恰好 1×201 + 1×409', async () => {
  const host = 'hoster'; // 复用 booking 组的主人配置（intro 类型）
  await setupHost(host);
  const slot = await firstSlot(host, 'intro', new Set([process.env.E2E_BOOKED_SLOT ?? '']));
  const body = JSON.stringify({
    start: slot, name: '并发访客', email: 'race@example.com',
  });

  const results = await Promise.all([0, 1].map((i) =>
    fetch(`${BASE()}/api/v1/pub/u/${host}/intro/bookings`, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: body.replace('并发访客', `并发访客${i}`),
    }).then(async (r) => ({ status: r.status, body: await r.json() })),
  ));

  const statuses = results.map((r) => r.status).sort();
  expect(statuses).toEqual([201, 409]);
  const conflict = results.find((r) => r.status === 409)!;
  expect(conflict.body.error.code).toBe('booking/slot_unavailable');

  // 库中恰好一条已确认预约（booking 组留下的那条 + 本组一条 = 2）
  const me = await setupHost(host);
  const list = await api('GET', '/api/v1/me/bookings?status=confirmed', undefined, me);
  expect(list.data.total).toBe(2);
});
