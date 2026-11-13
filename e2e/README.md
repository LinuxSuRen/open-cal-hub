# Open Cal Hub E2E（Playwright）

走**真实二进制**（前端产物 embed 进 calhub）的端到端测试。每轮运行使用独立
临时数据目录与随机端口，可重复执行。

## 运行

```bash
cd e2e
pnpm install
pnpm exec playwright install chromium   # 首次需要（见下方网络受限说明）
pnpm exec playwright test
```

- webServer 由 `playwright.config.ts` 自动启动：调用 `start-server.sh` 组装
  （frontend dist 缺失时先跑 `scripts/build-frontend.sh`）并构建
  `backend/cmd/calhub`，以 `CALHUB_MAIL_MODE=outbox` 启动。
- 服务器日志：`$CALHUB_E2E_DATA_DIR/server.log`（数据目录在 tmp，测试输出中打印）。
- 手动指定端口/数据目录：`CALHUB_E2E_PORT=8080 CALHUB_E2E_DATA_DIR=/tmp/x pnpm exec playwright test`。

## 浏览器下载（网络受限环境）

默认 CDN 不可达时走镜像：

```bash
PLAYWRIGHT_DOWNLOAD_HOST=https://cdn.npmmirror.com/binaries/playwright \
  pnpm exec playwright install chromium
```

浏览器不可用时，UI 用例自动跳过并降级为 Node fetch 的 API 级 e2e
（`utils/env.ts` 检测；`E2E_MODE=api` 可强制降级模式）。

## 约定

- 单 worker 串行：服务共享一份 SQLite，且登录/注册有 IP 限流（10/min），
  并行会互相触发限流。
- 断言优先 API 状态码 + 页面关键元素（`#app` 等），避免脆弱选择器。
- 用例按契约 `docs/api-contract.md` 编写；实现与契约冲突记为缺陷报 lead。
