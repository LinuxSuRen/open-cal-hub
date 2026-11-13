// 基础冒烟（API 级，不依赖浏览器）：验证 webServer 启动的真实二进制、
// 静态资源 embed 与 SPA fallback。业务端点用例在阶段二补齐。
import { test, expect } from '@playwright/test';

test.describe('服务冒烟', () => {
  test('GET /healthz 返回 200', async ({ request }) => {
    const res = await request.get('/healthz');
    expect(res.status()).toBe(200);
  });

  test('GET /readyz 返回 200（DB 可用）', async ({ request }) => {
    const res = await request.get('/readyz');
    expect(res.status()).toBe(200);
  });

  test('GET / 返回 SPA 壳（index.html）', async ({ request }) => {
    const res = await request.get('/');
    expect(res.status()).toBe(200);
    const body = await res.text();
    expect(body).toContain('<div id="app">');
    expect(body).toContain('CalHub');
  });

  test('SPA fallback：未知前端路由返回 index.html', async ({ request }) => {
    const res = await request.get('/app/calendar');
    expect(res.status()).toBe(200);
    expect(await res.text()).toContain('<div id="app">');
  });

  test('未知 /api 路径不回退 SPA，返回 404', async ({ request }) => {
    const res = await request.get('/api/v1/__definitely_missing__');
    expect(res.status()).toBe(404);
  });
});
