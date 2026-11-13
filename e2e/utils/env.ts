import fs from 'node:fs';
import { chromium } from 'playwright';

// 浏览器可用性探测：chromium 未安装（网络受限无法 playwright install）时，
// UI 用例自动跳过，e2e 降级为 Node fetch 的 API 级测试（在报告中说明）。
// E2E_MODE=api 可强制 API 级模式。
function detectChromium(): boolean {
  try {
    return fs.existsSync(chromium.executablePath());
  } catch {
    return false;
  }
}

export const chromiumAvailable = detectChromium();
export const apiOnlyMode = process.env.E2E_MODE === 'api' || !chromiumAvailable;
