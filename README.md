# Open Cal Hub

<p align="left"><img src="assets/logo.svg" width="96" alt="Open Cal Hub Logo" /></p>

[![Release](https://github.com/linuxsuren/open-cal-hub/actions/workflows/release.yml/badge.svg)](https://github.com/linuxsuren/open-cal-hub/actions/workflows/release.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Open Cal Hub 是一个自托管的开源日历与预约平台**：一个二进制文件加一个 SQLite 数据文件，即可为你的团队或个人提供网页日历、CalDAV 同步和公开预约链接的全部能力，数据完全归你自己掌控。

## 解决什么问题

如果你在使用托管 SaaS 的类预约链接平台，可能遇到过这些限制：数据不在自己手里、按席位订阅收费、功能受制于服务商、内网环境无法使用。Open Cal Hub 把同样的能力搬到你自己的服务器上：

- **自托管、零外部依赖**：单二进制（前端内嵌）+ SQLite，没有消息队列、对象存储和专用数据库要运维；
- **标准协议优先**：CalDAV 读写，macOS/Thunderbird/iOS 等客户端直接同步；分享链接提供 ICS 订阅源；
- **预约链接开箱即用**：公开主页 + 预约类型 + 可用时间规则 + 槽位自动计算，访客无需注册即可预约、改期、取消；
- **为生产而设计**：JWT 会话轮换、argon2id 密码哈希、限流、审计日志、Prometheus 指标、健康检查。

## 特性

- **日历与事件**：多日历、全天/定时事件、时区感知、重复规则（按日/周/月/年、BYDAY、UNTIL、COUNT）与单实例例外、ICS 导入；
- **分享与协作**：公开分享链接（只读网页 + ICS 订阅，可撤销/过期）；按成员授予只读或可编辑角色；
- **CalDAV 同步**：PROPFIND / REPORT / GET / PUT / DELETE / MKCOL，ETag 与条件请求，支持登录密码或独立应用密码；
- **预约链接**：`/u/<用户名>` 公开主页；预约类型支持时长、地点、前后缓冲、最小提前通知、可预约范围；可用时间按周规则 + 日期例外定义；
- **忙碌感知**：预约槽位自动避开你全部日历的事件、已确认预约，以及订阅的外部日历源（ICS URL）；
- **通知**：预约确认/取消/改期邮件双向通知并附 ICS；事件提醒扫描；SMTP 或落库（outbox）双模式；
- **管理台**：用户与角色管理、全平台预约、审计日志、站点设置、outbox 邮件查阅、邀请码；
- **可观测**：`/healthz`、`/readyz`、Prometheus `/metrics`、结构化日志（text/json）。

## 快速开始

访问 [http://localhost:8080](http://localhost:8080) 即可使用。默认监听 `:8080`，数据写入 `./data`（SQLite + 自动生成的密钥）。

### 方式一：二进制

从 [Releases](https://github.com/linuxsuren/open-cal-hub/releases) 下载对应平台压缩包（内含 `calhub` 二进制、`config.example.yaml`、`LICENSE`、`README.md`）：

```bash
# 以 linux/amd64 为例；macOS 为 darwin/amd64、darwin/arm64，windows 为 windows/amd64
curl -LO https://github.com/linuxsuren/open-cal-hub/releases/download/v1.0.0/calhub_1.0.0_linux_amd64.tar.gz
curl -LO https://github.com/linuxsuren/open-cal-hub/releases/download/v1.0.0/checksums.txt
shasum -a 256 -c --ignore-missing checksums.txt

tar xzf calhub_1.0.0_linux_amd64.tar.gz
./calhub -config config.example.yaml
```

### 方式二：Docker

```bash
docker run -d --name calhub \
  -p 8080:8080 \
  -v "$PWD/data:/data" \
  -e CALHUB_SERVER_BASE_URL=http://localhost:8080 \
  ghcr.io/linuxsuren/open-cal-hub:1.0.0

curl -fsS http://localhost:8080/healthz   # => ok
```

或使用 docker compose（推荐，含健康检查与数据卷）：

```bash
mkdir calhub && cd calhub
curl -LO https://raw.githubusercontent.com/linuxsuren/open-cal-hub/main/docker-compose.yml
docker compose up -d
docker compose logs -f calhub
```

### 方式三：源码构建

要求 Go ≥ 1.27、Node.js ≥ 22 与 pnpm（`corepack enable`）。

```bash
git clone https://github.com/linuxsuren/open-cal-hub.git
cd open-cal-hub

./scripts/build-frontend.sh        # 构建前端并拷贝到 backend/web/dist
cd backend && go build -o calhub ./cmd/calhub
./calhub
```

### 首次运行

1. 打开 `http://localhost:8080/register` 注册——**首个注册用户自动成为管理员**（可用 `auth.first_user_admin: false` 关闭）；
2. 默认开启邮箱验证且邮件模式为 `outbox`（邮件只落库不外发）：管理员登录后进入 **管理台 → 邮件** 查看验证链接；生产环境请改用 SMTP（见下）；
3. 备份只需保存数据目录（默认 `./data`，含 SQLite 文件与 `secret.key`）。

## 配置要点

配置优先级：**默认值 < YAML 文件（`-config`）< 环境变量（`CALHUB_*`）**。生产部署至少关注：

| 配置项 | 环境变量 | 说明 |
|---|---|---|
| `server.base_url` | `CALHUB_SERVER_BASE_URL` | 对外访问地址（邮件链接、分享链接基于它生成），必改 |
| `server.data_dir` | `CALHUB_SERVER_DATA_DIR` | 数据目录（SQLite、`secret.key`），容器内为 `/data` |
| `auth.jwt_secret` | `CALHUB_AUTH_JWT_SECRET` | 留空则自动生成并持久化；多副本/重建容器时请显式固定 |
| `mail.mode` | `CALHUB_MAIL_MODE` | `smtp`（生产）或 `outbox`（调试） |
| `mail.host` / `mail.port` / `mail.username` / `mail.password` | `CALHUB_MAIL_*` | SMTP 参数，`mail.tls` 支持 `starttls` / `tls` / `none` |
| `log.format` | `CALHUB_LOG_FORMAT` | `json` 便于采集 |

完整配置项（含默认值）见[配置参考](docs/configuration.md)，逐项注释的示例文件见 [deploy/config.example.yaml](deploy/config.example.yaml)。

## CalDAV 客户端接入

以 macOS「日历」为例：

1. **日历 → 添加账户 → 其他 CalDAV 账户**，账户类型选「高级设置」；
2. 用户名填 `rick`（或邮箱），密码用登录密码；**推荐**先在 网页 **设置 → 应用密码** 创建一个专用密码，泄露可单独吊销；
3. 服务器地址填 `https://cal.example.com/dav/users/rick/calendars/`。

Thunderbird（日历 → 新建日历 → CalDAV）与其他标准客户端同理，日历合集地址为：

```
https://<你的域名>/dav/users/<用户名>/calendars/
```

客户端发现入口 `/.well-known/caldav` 会 301 到 `/dav/`。用 curl 验证连通性：

```bash
curl -u rick:<应用密码> -X PROPFIND -H "Depth: 1" \
  https://cal.example.com/dav/users/rick/calendars/
```

经 nginx 反代部署时无需特殊模块，任意 HTTP 方法会默认透传（注意不要使用 `limit_except` 限制方法），完整示例见 [docs/deployment.md](docs/deployment.md#反向代理与-tls)。

## 文档

- [配置参考](docs/configuration.md)——全部配置项、环境变量与默认值
- [部署指南](docs/deployment.md)——二进制 / Docker / compose / systemd / 反向代理与 TLS
- [外部日历接入](docs/integrations.md)——Google/Outlook/QQ/iCloud 日历的 ICS 订阅地址获取与安全注意
- [架构设计](docs/architecture.md)——拓扑、数据模型与关键决策
- [API 契约](docs/api-contract.md)——REST 与 CalDAV 接口的唯一事实源

## 参与贡献

Issue 与 PR 的提交要求、开发环境与验证命令见[贡献指南](CONTRIBUTING.md)。发现问题先看 [Issues](https://github.com/linuxsuren/open-cal-hub/issues) 是否已有讨论。

## 安全

漏洞请走私有通道报告，勿开公开 issue，见[安全策略](SECURITY.md)。

## 协议

[MIT](LICENSE)。第三方依赖的协议声明见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。
