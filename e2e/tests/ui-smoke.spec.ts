// UI 冒烟（依赖 chromium）：验证前端产物真实可加载。
// 浏览器不可用时自动跳过（降级为 API 级 e2e，见 utils/env.ts）。
import { test, expect } from '@playwright/test';
import { apiOnlyMode } from '../utils/env';

test.skip(apiOnlyMode, 'chromium 不可用：已降级为 API 级 e2e');

test('SPA 壳在浏览器中渲染 #app', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('#app')).toBeAttached();
});
