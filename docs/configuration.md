# 配置参考

本文与 `backend/internal/config/config.go` 一一对应，覆盖全部配置项。

## 加载顺序

优先级从低到高：**内置默认值 < YAML 配置文件 < 环境变量**。

- YAML：启动参数 `-config <path>` 指定；逐项注释的完整示例见 [deploy/config.example.yaml](../deploy/config.example.yaml)；
- 环境变量：统一前缀 `CALHUB_` + 小节 + 键名，例如 `CALHUB_SERVER_ADDRESS`、`CALHUB_MAIL_HOST`，逐项对应关系见下表（布尔值为 `true`/`false`；时长为 Go duration 格式，如 `15m`、`720h`；整数直接写数字）。

## server — 服务

| YAML 键 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| `server.address` | `CALHUB_SERVER_ADDRESS` | `:8080` | HTTP 监听地址 |
| `server.base_url` | `CALHUB_SERVER_BASE_URL` | `http://localhost:8080` | 对外基础 URL；邮件验证/重置链接、分享与预约链接基于它生成。生产环境必改，末尾斜杠会被自动去除 |
| `server.data_dir` | `CALHUB_SERVER_DATA_DIR` | `data` | 数据目录：SQLite 数据库（`db.path` 为空时）与自动生成的 JWT 密钥 `secret.key` 存放处 |

## db — 存储

| YAML 键 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| `db.path` | `CALHUB_DB_PATH` | 空（即 `<data_dir>/calhub.db`） | SQLite 数据库文件路径；父目录不存在时启动会自动创建 |

## auth — 认证与会话

| YAML 键 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| `auth.jwt_secret` | `CALHUB_AUTH_JWT_SECRET` | 空 | JWT 签名密钥。留空时首次启动自动生成 32 字节随机密钥并持久化到 `<data_dir>/secret.key`（权限 0600）。多副本部署、或容器重建后需要保持登录态时，请显式设置并妥善保管 |
| `auth.access_ttl` | `CALHUB_AUTH_ACCESS_TTL` | `15m` | 访问令牌有效期；到期后凭刷新 Cookie 换新 |
| `auth.refresh_ttl` | `CALHUB_AUTH_REFRESH_TTL` | `720h`（30 天） | 刷新令牌有效期；每次刷新轮换，检测到重用会吊销整个令牌族 |
| `auth.first_user_admin` | `CALHUB_AUTH_FIRST_USER_ADMIN` | `true` | 首个注册用户自动成为管理员；对外开站建议关闭，由管理员手动提权 |
| `auth.require_email_validation` | `CALHUB_AUTH_REQUIRE_EMAIL_VALIDATION` | `true` | 注册后必须完成邮箱验证才能登录。本地试用时可设为 `false`，或保持 `mail.mode: outbox` 在管理台「邮件」页取验证链接 |
| `auth.bootstrap_admin_email` | `CALHUB_AUTH_BOOTSTRAP_ADMIN_EMAIL` | 空 | 初始管理员引导：空库首次启动时自动创建 admin 账号，适合脚本化部署。需与 bootstrap 密码同时配置；账号视为已验证。创建后建议立即改密并移除这两个变量（已有用户的库幂等跳过） |
| `auth.bootstrap_admin_password` | `CALHUB_AUTH_BOOTSTRAP_ADMIN_PASSWORD` | 空 | 初始管理员引导密码（至少 8 位且含字母与数字）。只配置其一会导致启动失败，避免半配置静默失效 |
| `auth.oauth.github.client_id` | `CALHUB_AUTH_OAUTH_GITHUB_CLIENT_ID` | 空 | GitHub OAuth 登录。client_id 与 secret 同时配置时启用；回调地址为 `<base_url>/api/v1/auth/oauth/github/callback`，请在 GitHub OAuth App 中登记 |
| `auth.oauth.github.client_secret` | `CALHUB_AUTH_OAUTH_GITHUB_CLIENT_SECRET` | 空 | GitHub OAuth 客户端密钥（仅服务端使用，不出后端） |
| `auth.oauth.google.client_id` | `CALHUB_AUTH_OAUTH_GOOGLE_CLIENT_ID` | 空 | Google OAuth 登录（scope：openid email profile）。需同时在 Google Cloud 控制台登记回调地址 `<base_url>/api/v1/auth/oauth/google/callback` |
| `auth.oauth.google.client_secret` | `CALHUB_AUTH_OAUTH_GOOGLE_CLIENT_SECRET` | 空 | Google OAuth 客户端密钥 |
| `auth.oauth.redirect_base` | `CALHUB_AUTH_OAUTH_REDIRECT_BASE` | 同 `server.base_url` | OAuth 回调地址前缀（base_url 与实际对外域名不一致的反代场景使用） |
| `auth.oauth.*.authorize_url` 等 | `CALHUB_AUTH_OAUTH_{GITHUB,GOOGLE}_{AUTHORIZE,TOKEN,USERINFO,EMAILS}_URL` | 官方端点 | 端点覆盖（联调/测试环境指向本地模拟服务用） |

## mail — 邮件

| YAML 键 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| `mail.mode` | `CALHUB_MAIL_MODE` | `outbox` | `outbox`：邮件只写入数据库不外发（管理台「邮件」页可查看，适合开发与试用）；`smtp`：真实发送 |
| `mail.from` | `CALHUB_MAIL_FROM` | `calhub@localhost` | 发件人地址 |
| `mail.host` | `CALHUB_MAIL_HOST` | 空 | SMTP 服务器主机；`mode: outbox` 时无需配置 |
| `mail.port` | `CALHUB_MAIL_PORT` | `0` | SMTP 端口；常用 587（STARTTLS）、465（TLS）、25（明文） |
| `mail.username` | `CALHUB_MAIL_USERNAME` | 空 | SMTP 认证用户名 |
| `mail.password` | `CALHUB_MAIL_PASSWORD` | 空 | SMTP 认证密码；建议经环境变量或密钥管理注入，勿写入仓库 |
| `mail.tls` | `CALHUB_MAIL_TLS` | `starttls` | `starttls` / `tls` / `none` |

## log — 日志

| YAML 键 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| `log.level` | `CALHUB_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `log.format` | `CALHUB_LOG_FORMAT` | `text` | `text` / `json`（容器与采集环境建议 `json`） |

## metrics — 指标

| YAML 键 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| `metrics.enabled` | —（当前版本仅支持 YAML） | `true` | Prometheus `/metrics` 端点开关 |

## 环境变量示例

```bash
export CALHUB_SERVER_BASE_URL="https://cal.example.com"
export CALHUB_SERVER_DATA_DIR="/var/lib/calhub"
export CALHUB_AUTH_JWT_SECRET="$(openssl rand -hex 32)"
export CALHUB_MAIL_MODE="smtp"
export CALHUB_MAIL_HOST="smtp.example.com"
export CALHUB_MAIL_PORT="587"
export CALHUB_MAIL_USERNAME="calhub@example.com"
export CALHUB_MAIL_PASSWORD="******"
export CALHUB_LOG_FORMAT="json"
```

## 配置校验行为

- YAML 解析失败、配置文件不可读：启动直接失败并输出 `load_config_failed`；
- `db.path` 父目录不存在：自动创建（无权限则失败）；
- 非法的 `mail.mode` 一律按 `outbox` 处理；`base_url` 末尾斜杠自动去除；
- `auth.access_ttl` / `auth.refresh_ttl` 设为非正值时回退默认值（15m / 720h）。
