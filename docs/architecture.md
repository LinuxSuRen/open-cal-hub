# Open Cal Hub 架构设计

Open Cal Hub 是一个自托管的开源日历与预约平台：用户以邮箱注册、拥有独立日历（CalDAV 协议读写）、可将日历公开分享，并提供 Calendly 风格（独立实现的预约链接）能力。项目为原创实现，不包含任何 Calendly 的代码、文案、图片或界面抄袭。

## 1. 拓扑

```
浏览器 ──> 单一二进制 calhub-server (Go)
             ├── /api/v1/*      REST API（SPA + 公开页）
             ├── /dav/*         CalDAV（基于 emersion/go-webdav）
             ├── /healthz /readyz /metrics
             └── /*             内嵌前端静态资源（frontend 构建产物 embed）
存储: SQLite（WAL）    邮件: SMTP（可配置，开发模式落库 outbox）
```

- 单二进制 + 单 SQLite 文件，面向自托管生产环境；无外部中间件依赖。
- 前端构建产物在发布时通过 `go:embed` 内嵌，同时支持 `-config` 指定的本地目录覆盖。

## 2. 仓库布局

```
backend/                 Go 模块（module github.com/linuxsuren/open-cal-hub/backend）
  cmd/calhub/            入口
  internal/config/       配置加载（env + YAML）
  internal/db/           SQLite 打开、迁移
  internal/httpx/        envelope、错误码、中间件（日志/恢复/限流/安全头）
  internal/auth/         JWT、会话、密码、令牌
  internal/user/         用户、应用密码
  internal/admin/        管理端 API
  internal/mail/         SMTP/outbox 双实现
  internal/cal/          日历、事件、重复规则、分享
  internal/dav/          CalDAV 后端（适配 go-webdav）
  internal/booking/      预约类型、可用时间、槽位计算、预约
  internal/remind/       提醒扫描 worker
  web/dist/              前端构建产物（构建脚本拷入，embed）
frontend/                Vue 3 + TS + Vite（产品 UI 手写设计令牌，管理台用 Element Plus）
e2e/                     Playwright 端到端测试
docs/                    架构、API 契约、部署文档
.github/                 CI / Release / CodeQL / Dependabot
```

## 3. 关键决策

| 决策 | 选择 | 理由 |
|---|---|---|
| 存储 | SQLite（modernc 纯 Go 驱动，WAL） | 自托管零依赖；CGO-free 交叉编译 |
| CalDAV | emersion/go-webdav `caldav.Handler` + 自研 Backend | 需求指定；协议正确性由库保证 |
| 重复事件 | 存 iCalendar 原文 + 索引列，teambition/rrule-go 展开 | 与 CalDAV 存储模型一致 |
| 时间 | DB 一律 UTC RFC3339 文本；API 一律 UTC；前端按用户时区渲染 | 时区是日历产品第一正确性风险 |
| 认证 | argon2id 密码 + JWT access(15m) + 刷新令牌轮换(30d, httpOnly Cookie) | 生产标准 |
| 错误 | `{error:{code,message}}`，稳定错误码，技术根因只进日志 | 用户可见错误封闭 |
| 部署 | 单二进制 / Docker（多架构）/ compose / systemd | 生产交付 |
| 观测 | slog 结构化日志 + request id + Prometheus 指标 | 可运维 |

## 4. 权限模型

- 角色：`user` / `admin`（首个注册用户自动成为 admin；可用配置关闭）。
- 日历归属：owner 全权；`calendar_members` 授权 viewer（只读事件）/ editor（可增删改事件，不可改日历设置与分享）。
- 公开分享：`share_links` 令牌只读（HTML 视图 + ICS 订阅源），可撤销、可过期。
- CalDAV：Basic（主密码或应用密码），仅本人日历（v1 范围）。
- 预约公开页：无需认证，限流 + 蜜罐字段。

## 5. 安全基线

- 密码 argon2id；令牌（验证/重置/分享/预约管理）只存哈希、有期限、一次性。
- 限流：登录 10/min/IP，注册与忘记密码 5/h/IP，公开槽位 120/min/IP，公开预约提交 10/h/IP。
- Security headers（X-Content-Type-Options、Referrer-Policy、CSP 由前端资源形态决定）、CORS 默认同源。
- 审计日志记录全部 admin 操作与安全事件（登录失败、令牌重用）。

## 6. 观测

- 出入口日志带 request_id；每个对外错误码在日志可检索（`user_error code=...`）。
- 指标：HTTP 请求速率/时延/错误码、注册/登录、事件写入、预约创建/取消、提醒投递、DB 慢查询。
- on-call 问题基线：注册登录为何失败、预约槽位为何不可用、CalDAV 同步是否异常、邮件是否发出。

## 7. 知识产权

- 名称、界面、文案全部原创；不使用 Calendly 商标、截图与文案。
- 依赖均为 MIT/BSD/Apache 等宽松协议；发布物附 THIRD_PARTY_NOTICES。
- 本项目以 MIT 协议开源。
