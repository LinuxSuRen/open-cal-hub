# 更新日志（Changelog）

本项目的所有显著变更都记录在本文件中。

格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循[语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

- 无。

## [1.0.0] - 待发布

首个正式版本：开箱即用的自托管日历与预约平台，单二进制、零外部依赖。

### 新增

**账号与认证**

- 邮箱注册与登录，支持用户名或邮箱登录；密码使用 argon2id 哈希存储。
- JWT 访问令牌（默认 15 分钟）+ httpOnly Cookie 刷新令牌（默认 30 天，每次刷新轮换，重用检测吊销整个令牌族）。
- 邮箱验证、忘记密码与重置密码、会话管理（查看/吊销）。
- 应用密码：为 CalDAV 客户端签发独立密码，支持备注、最后使用时间与吊销。
- 首个注册用户自动成为管理员（可配置关闭）；支持邀请码注册模式。

**日历与事件**

- 多日历管理：创建、编辑、软删除、颜色与独立时区；首个日历自动设为默认。
- 事件增删改查：全天/定时、时区感知、提醒（分钟提前量）。
- 重复事件：按日/周/月/年与间隔重复，支持 BYDAY、UNTIL、COUNT；单实例例外（override）与取消。
- ICS 导入：按 UID 去重，报告导入与跳过数量。
- 公开分享链接：只读 HTML 视图与 ICS 订阅源，可撤销、可过期。
- 成员共享：按用户授予 viewer（只读）或 editor（可编辑事件）角色。

**CalDAV**

- 基于 CalDAV 协议的标准读写访问：`/.well-known/caldav` 发现、PROPFIND、REPORT（calendar-query、calendar-multiget，含 time-range 与展开）、GET/PUT（ETag 与 If-Match/If-None-Match）、DELETE、MKCOL。
- Basic 认证（登录密码或应用密码）；VTIMEZONE 尊重 TZID。

**预约链接**

- 公开预约主页 `/u/{username}` 与预约类型页 `/u/{username}/{slug}`，无需登录，带限流与蜜罐防护。
- 预约类型：时长、颜色、地点类型、前后缓冲、槽位间隔、最小提前通知、可预约范围（滚动天数或固定区间）。
- 可用时间集合：按周规则 + 日期例外（不可用/仅当日可用），注册时自动创建"工作时间"默认集。
- 槽位计算：合并主人全部日历事件、已确认预约与外部日历源的忙碌时间。
- 访客预约、取消与改期（凭签名管理链接，无需注册）；确认/取消/改期邮件双方通知并附 ICS。
- 外部日历源（ICS URL）订阅用于忙碌检测，支持手动同步与错误展示。

**通知与运维**

- 邮件：SMTP 与 outbox（落库调试）双模式；验证、重置、预约确认/取消/改期等模板。
- 提醒扫描 worker：按事件提醒配置投递通知。
- 管理台：仪表盘统计、用户管理（停用/角色/重置密码）、全平台预约、审计日志、系统设置、outbox 邮件查看与重发、邀请码管理。
- 可观测性：slog 结构化日志（text/json）、request id、Prometheus `/metrics`、`/healthz` 与 `/readyz`。
- 安全基线：登录/注册/忘记密码/公开预约限流、安全响应头、CORS 默认同源、管理员操作与安全事件审计。

**交付与部署**

- 单二进制（前端构建产物内嵌）、SQLite（WAL）零外部依赖。
- 多阶段 Dockerfile（非 root 运行）、docker-compose、systemd 与 nginx 反代示例。
- 多平台发布：linux/darwin（amd64/arm64）、windows/amd64 二进制与 sha256 校验和，多架构容器镜像发布至 ghcr.io。
- CI（构建与测试）、CodeQL（go + javascript）与 Dependabot 自动化。

### 安全

- 密码与令牌只存哈希；验证/重置/分享/预约管理令牌均为一次性且有期限。
- 用户可见错误统一为稳定错误码，技术根因只进日志。

### 兼容性

- 首个版本，无兼容性约束。

[Unreleased]: https://github.com/linuxsuren/open-cal-hub/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/linuxsuren/open-cal-hub/releases/tag/v1.0.0
