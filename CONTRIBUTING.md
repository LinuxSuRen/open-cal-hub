# 贡献指南

感谢你关注 Open Cal Hub！本文说明如何搭建开发环境、验证改动以及提交 Pull Request。

## 行为准则

- 讨论对事不对人；review 意见给出理由与依据，不确定的表述请说明"不阻碍合并"。
- 公开透明：问题优先开 issue 讨论，避免私聊技术决策。

## 开发环境

| 依赖 | 版本要求 | 说明 |
|---|---|---|
| Go | ≥ 1.27 | 后端；`go 1.27.0` 见 `backend/go.mod` |
| Node.js | ≥ 22 | 前端构建 |
| pnpm | ≥ 9（锁定文件 lockfileVersion 9.0） | `corepack enable` 后使用 |
| SQLite | 无需安装 | 后端使用纯 Go 驱动（modernc.org/sqlite） |

获取代码并安装依赖：

```bash
git clone https://github.com/linuxsuren/open-cal-hub.git
cd open-cal-hub

# 前端依赖
pnpm --dir frontend install --frozen-lockfile

# 后端依赖
cd backend && go mod download && cd ..
```

## 目录结构

```
backend/                 Go 模块（单二进制 calhub）
  cmd/calhub/            程序入口与路由挂载
  internal/config/       配置加载（默认值 < YAML < 环境变量）
  internal/db/           SQLite 打开与迁移（migrations/*.sql）
  internal/httpx/        响应包络、错误码、中间件
  internal/auth/         JWT、会话、密码、令牌
  internal/user/         用户与应用密码
  internal/admin/        管理端 API
  internal/mail/         SMTP / outbox 双实现
  internal/cal/          日历、事件、重复规则、分享
  internal/dav/          CalDAV 适配
  internal/booking/      预约类型、可用时间、槽位、预约
  internal/remind/       提醒扫描 worker
  web/dist/              前端构建产物（构建脚本拷入，go:embed 内嵌）
frontend/                Vue 3 + TypeScript + Vite
  src/api/               API 客户端与类型
  src/views/             页面（auth / app / admin / pub）
  src/i18n/              文案（中文为主）
e2e/                     Playwright 端到端测试
docs/                    架构、API 契约、部署与配置文档
deploy/                  systemd、nginx、示例配置
scripts/                 构建与验证脚本
```

## 验证命令

提交前请在本地通过以下命令（也以同样内容运行在 CI 上）：

```bash
# 后端
cd backend && go build ./... && go vet ./... && go test ./... && cd ..

# 前端
cd frontend && pnpm run build && cd ..

# 全量集成（前端产物拷入 backend/web/dist 后构建单一二进制）
./scripts/build-frontend.sh && cd backend && go build -o calhub ./cmd/calhub

# 冒烟：启动后健康检查
./backend/calhub &          # 默认监听 :8080
curl -fsS http://localhost:8080/healthz
```

## 硬性约定

1. **API 契约是唯一事实源**：字段、错误码、状态码以 `docs/api-contract.md` 为准；需要变更先开 issue/PR 更新契约，再改实现。
2. **时间**：后端与数据库一律 UTC（RFC3339，`Z` 后缀）；weekday 0 = 周日；分钟数 0..1439。
3. **错误**：用户可见错误使用统一错误码与文案兜底，技术根因只进日志；新增错误码先登记契约与 `httpx` 注册表。
4. **文案**：前端中文为主，操作动词全流程一致；禁止后置括号补充说明。
5. **依赖**：新增 Go/npm 依赖需在 PR 中说明用途与维护状态；能 30 行内自研的不引依赖。
6. **迁移**：数据库结构变更通过新增 `backend/internal/db/migrations/NNNN_*.sql` 实现，不修改已有迁移文件。

## 提交 Pull Request

1. **先搜索 issue**：确认没有重复讨论；预计变更较大时，先开 issue 描述方案征得反馈。
2. **一个 PR 只包含一类修改**：从 `main` 新建分支（如 `fix/booking-slot`、`feat/ics-export`）；禁止直接在 `main` 上开发。
3. **commit 记录整洁**：只保留希望合并进上游的提交，必要时 squash；提交信息使用中文或英文的"类型: 摘要"格式（如 `fix: 预约槽位未应用前后缓冲`）。
4. **PR 描述完整**：给出问题背景（关联 issue，用 `close #123`）、改动点、自测过程与输出；涉及 UI 的改动附前后截图。
5. **自测通过再提交**：跑完上述验证命令；未准备好接受 review 时标题加 `WIP:` 前缀。
6. **不用 force push**：review 期间追加提交即可，方便 reviewer 增量查看。

维护者会在 2～7 天内 review；每个 PR 至少需要一名维护者批准后由维护者执行合并。

## 安全问题

请勿通过公开 issue 报告安全漏洞，参见 [SECURITY.md](SECURITY.md)。

## 许可

提交即表示你同意贡献内容以 [MIT 协议](LICENSE)授权本项目使用。
