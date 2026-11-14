// 组 8：响应式适配冒烟（issue #25）——手机/平板视口下页面可渲染且无横向溢出。
import { test, expect } from '@playwright/test';
import { apiOnlyMode } from '../utils/env';

test.skip(apiOnlyMode, 'chromium 不可用');

const VIEWPORTS = [
  { name: '手机 390×844', width: 390, height: 844 },
  { name: '平板 820×1180', width: 820, height: 1180 },
];

// 无横向溢出：内容不撑破视口（+1px 容差）。
async function expectNoHorizontalOverflow(page: import('@playwright/test').Page) {
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - window.innerWidth,
  );
  expect(overflow).toBeLessThanOrEqual(1);
}

for (const vp of VIEWPORTS) {
  test.describe(`响应式 ${vp.name}`, () => {
    test.use({ viewport: { width: vp.width, height: vp.height } });

    test('登录页可渲染且无横向溢出', async ({ page }) => {
      await page.goto('/login');
      await expect(page.locator('#app')).toBeAttached();
      await expectNoHorizontalOverflow(page);
    });

    test('公开预约主页可渲染且无横向溢出', async ({ page }) => {
      await page.goto('/u/hoster');
      await expect(page.locator('#app')).toBeAttached();
      await expectNoHorizontalOverflow(page);
    });

    test('公开分享页可渲染且无横向溢出', async ({ page }) => {
      // 分享 token 由 share 组创建（字母序在其后；此处改用日历公开页兜底：
      // 访问不存在的 token 会进入错误态页面，同样验证壳与溢出约束）
      await page.goto('/s/__responsive_probe__');
      await expect(page.locator('#app')).toBeAttached();
      await expectNoHorizontalOverflow(page);
    });
  });
}
