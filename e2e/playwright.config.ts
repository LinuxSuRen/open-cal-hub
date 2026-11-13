import os from 'node:os';
import path from 'node:path';
import fs from 'node:fs';
import { execSync } from 'node:child_process';
import { defineConfig, devices } from '@playwright/test';

// ---------------------------------------------------------------------------
// 运行环境：随机端口 + 每轮独立临时数据目录（保证 e2e 可重复运行）。
// 配置文件必须同步求值（defineConfig 不支持 async 工厂），这里用子进程申请
// 空闲端口：输出用正则提取纯数字，任何异常退回随机端口兜底，绝不在配置
// 加载期抛错。端口真被抢占时 webServer 就绪探测会失败并报 address in use，
// 重跑即可换端口。
// ---------------------------------------------------------------------------
function pickFreePort(): number {
  try {
    // 输出带 PORT= 标记：即使 stdout 混入 ANSI 转义序列等杂音也不会误读端口。
    const out = execSync(
      `node -e "const s=require('node:net').createServer();s.listen(0,'127.0.0.1',function(){console.log('PORT='+s.address().port);s.close()})"`,
      { encoding: 'utf8', timeout: 10_000, env: { ...process.env, NO_COLOR: '1', FORCE_COLOR: '0' } },
    );
    const m = out.match(/PORT=(\d+)/);
    const port = m ? Number(m[1]) : 0;
    if (Number.isInteger(port) && port > 0) return port;
  } catch {
    /* 落入随机兜底 */
  }
  return 20000 + Math.floor(Math.random() * 20000);
}

function freshDataDir(): string {
  // 清理 6 小时前遗留的数据目录，控制 /tmp 占用；当前轮目录保留用于排障。
  const tmp = os.tmpdir();
  const cutoff = Date.now() - 6 * 60 * 60 * 1000;
  for (const name of fs.readdirSync(tmp)) {
    if (!name.startsWith('calhub-e2e-')) continue;
    const dir = path.join(tmp, name);
    try {
      if (fs.statSync(dir).mtimeMs < cutoff) fs.rmSync(dir, { recursive: true, force: true });
    } catch {
      /* 并发清理竞争忽略 */
    }
  }
  return fs.mkdtempSync(path.join(tmp, 'calhub-e2e-'));
}

// 主进程与 worker 进程都会加载本配置：通过 process.env 保证一轮运行内
// 端口/数据目录一致（worker 继承主进程 fork 时的环境变量）。也可由外部
// 显式注入 CALHUB_E2E_PORT / CALHUB_E2E_DATA_DIR 覆盖。
function runScoped(key: string, make: () => string): string {
  const envKey = `CALHUB_E2E_${key}`;
  if (!process.env[envKey]) process.env[envKey] = make();
  return process.env[envKey] as string;
}

const port = Number(runScoped('PORT', () => String(pickFreePort())));
const dataDir = runScoped('DATA_DIR', () => freshDataDir());
const baseURL = `http://127.0.0.1:${port}`;

export default defineConfig({
  testDir: './tests',
  timeout: 30_000,
  expect: { timeout: 5_000 },
  // 单服务工作目录共享一份 SQLite 数据；且登录/注册有 IP 限流（10/min），
  // 并行 worker 会互相触发限流，故严格串行。
  fullyParallel: false,
  workers: 1,
  retries: 0,
  forbidOnly: !!process.env.CI,
  reporter: [
    ['list'],
    ['html', { open: 'never', outputFolder: 'playwright-report' }],
  ],
  use: {
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: {
    // 组装并启动真实二进制（前端产物 embed）；服务器日志在 $E2E_DATA_DIR/server.log
    command: 'bash start-server.sh',
    url: `${baseURL}/healthz`,
    reuseExistingServer: false,
    timeout: 180_000, // 首轮可能触发前端组装 + go build，放宽
    stdout: 'ignore',
    stderr: 'pipe',
    env: {
      E2E_PORT: String(port),
      E2E_DATA_DIR: dataDir,
      E2E_BASE_URL: baseURL,
    },
  },
});
