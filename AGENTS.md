# AGENTS.md — 仓库协作规范（所有 agent 必读）

本仓库由多 agent 并行开发，lead 负责唯一 git 提交入口。**任何 agent 不得执行 git commit / push / branch 操作。**

## 唯一事实源

- API：`docs/api-contract.md`（字段、错误码、状态码以它为准；要改先请 lead 更新契约）
- 架构与数据模型：`docs/architecture.md`、`backend/internal/db/migrations/*.sql`
- 前端路由：契约 §8

## 写作用域（严格遵守，跨域改动先消息 lead）

| 成员 | 可写范围 |
|---|---|
| backend-core | `backend/internal/{auth,user,admin,mail,httpx,app,config,server,db}`、`backend/cmd/`、`backend/go.mod`、`backend/go.sum`、迁移 `0001` |
| backend-cal | `backend/internal/{cal,booking,dav,remind}`、迁移 `0002+`（新增文件） |
| frontend-a | `frontend/src/{api/client.ts,api/types.ts,api/auth.ts,api/calendars.ts,api/index.ts,i18n/index.ts,i18n/core.ts,i18n/app.ts,App.vue,router.ts,main.ts,stores/,views/auth/,views/app/{AppHome,CalendarPage,CalendarsPage}.vue,components/calendar/,composables/}` |
| frontend-b | `frontend/src/{api/{booking,pub,admin}.ts,i18n/{booking,admin,pub}.ts,views/app/{BookingConfigPage,MyBookingsPage,SettingsPage}.vue,views/admin/,views/pub/,components/{booking,admin,ds}/}` |
| qa | `e2e/**`、`.github/workflows/ci.yml`、`scripts/verify.sh`（只读他人源码；bug 报给 lead） |
| release | 根目录门面文件（README、LICENSE、CHANGELOG、CONTRIBUTING、SECURITY、THIRD_PARTY_NOTICES）、`Dockerfile`、`docker-compose.yml`、`deploy/`、`docs/{deployment,configuration}.md`、`.github/workflows/{release,codeql}.yml`、`.github/dependabot.yml` |

## 验证命令（完成宣称必须附当次运行输出）

- 后端：`cd backend && go build ./... && go vet ./... && go test ./...`
- 前端：`cd frontend && pnpm run build`
- 全量集成：`./scripts/build-frontend.sh && cd backend && go build ./cmd/calhub`

## 硬性纪律

1. 时间：后端/DB 一律 UTC RFC3339；API 响应一律 UTC `Z` 后缀；weekday 0=周日；分钟数 0..1439。
2. 错误：用户可见错误必须用 `httpx.E(code, message)`，新错误码先登记契约与 `httpx` 注册表；技术根因只进日志。
3. 文案：前端中文为主（i18n 模块按 owner 归属添加），操作动词全流程一致；禁止后置括号补充。
4. 依赖：新增 Go/npm 依赖需在报告里说明用途与维护状态；能 30 行内自研的不引依赖。
5. 不确定就问 lead（send_message），不要自行改契约或他人文件。
