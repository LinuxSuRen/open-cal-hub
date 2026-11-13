# 部署指南

本文覆盖 Open Cal Hub 的生产部署方式：单二进制、Docker、docker compose、systemd、反向代理与 TLS。全部方式的默认监听端口为 `8080`，数据保存在数据目录（默认 `./data`，含 SQLite 数据库与 `secret.key`）。

部署前请确认：

- 一个可访问本服务的域名（用于 `server.base_url`，邮件与分享链接基于它生成）；
- 建议通过反向代理提供 HTTPS（CalDAV 客户端与预约页都需要）；
- 数据目录所在磁盘可写，并纳入备份计划。

## 方式一：单二进制

1. 从 [Releases](https://github.com/linuxsuren/open-cal-hub/releases) 下载对应平台压缩包并校验：

   ```bash
   curl -LO https://github.com/linuxsuren/open-cal-hub/releases/download/v1.0.0/calhub_1.0.0_linux_amd64.tar.gz
   curl -LO https://github.com/linuxsuren/open-cal-hub/releases/download/v1.0.0/checksums.txt
   shasum -a 256 -c --ignore-missing checksums.txt
   tar xzf calhub_1.0.0_linux_amd64.tar.gz
   ```

2. 使用示例配置启动（生产环境请修改 `base_url`、`jwt_secret` 与邮件配置，逐项说明见[配置参考](configuration.md)）：

   ```bash
   ./calhub -config config.example.yaml
   curl -fsS http://localhost:8080/healthz   # => ok
   ```

3. 注册首个账号（自动成为管理员），随后按需改为 systemd 常驻（见下文）。

## 方式二：Docker 单容器

```bash
docker run -d --name calhub \
  -p 8080:8080 \
  -v "$PWD/data:/data" \
  -e CALHUB_SERVER_BASE_URL=https://cal.example.com \
  -e CALHUB_AUTH_JWT_SECRET=<32字节随机串> \
  -e CALHUB_MAIL_MODE=smtp \
  -e CALHUB_MAIL_HOST=smtp.example.com \
  -e CALHUB_MAIL_PORT=587 \
  -e CALHUB_MAIL_USERNAME=calhub@example.com \
  -e CALHUB_MAIL_PASSWORD=****** \
  ghcr.io/linuxsuren/open-cal-hub:1.0.0
```

镜像特性：多阶段构建、非 root（uid 10001）运行、内置 `HEALTHCHECK`（`curl /healthz`）、数据全部落在卷 `/data`。生成随机密钥：`openssl rand -hex 32`。

## 方式三：docker compose

仓库根目录提供 [docker-compose.yml](../docker-compose.yml)：

```bash
mkdir calhub && cd calhub
curl -LO https://raw.githubusercontent.com/linuxsuren/open-cal-hub/main/docker-compose.yml
# 按需写入 .env 覆盖 CALHUB_* 默认值
docker compose up -d
docker compose ps          # healthcheck 变为 healthy 即就绪
```

升级与备份：

```bash
docker compose pull && docker compose up -d   # 升级
tar czf calhub-backup-$(date +%F).tar.gz data # 备份（停机或低峰执行更安全）
```

## systemd 常驻服务

[deploy/calhub.service](../deploy/calhub.service) 提供了带沙箱加固的完整单元，要点：

```bash
sudo useradd --system --home /var/lib/calhub --shell /usr/sbin/nologin calhub
sudo mkdir -p /var/lib/calhub /etc/calhub
sudo cp deploy/config.example.yaml /etc/calhub/config.yaml && sudoedit /etc/calhub/config.yaml
sudo cp calhub /usr/local/bin/
sudo cp deploy/calhub.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now calhub
journalctl -u calhub -f          # 跟踪日志
```

单元说明：

- 配置文件在 `/etc/calhub/config.yaml`，数据目录由 `Environment=CALHUB_SERVER_DATA_DIR=/var/lib/calhub` 指定（环境变量优先级高于 YAML）；
- `ProtectSystem=strict` + `ReadWritePaths=/var/lib/calhub`：文件系统只开放数据目录写入；
- `Restart=on-failure`：异常退出 5 秒后自动拉起；`systemctl stop calhub` 触发优雅停机（最长 10 秒）。

## 反向代理与 TLS

[deploy/nginx.conf](../deploy/nginx.conf) 提供完整片段。核心配置：

```nginx
server {
    listen 443 ssl;
    http2 on;                          # nginx >= 1.25.1；旧版写 "listen 443 ssl http2;"
    server_name cal.example.com;

    ssl_certificate     /etc/letsencrypt/live/cal.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/cal.example.com/privkey.pem;

    client_max_body_size 50m;          # CalDAV 整册同步 / ICS 导入

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 300s;
    }
}
```

CalDAV 相关注意：

1. **方法透传**：nginx 默认转发任意 HTTP 方法（`PROPFIND`、`REPORT`、`MKCOL` 等），无需额外模块；**切勿**用 `limit_except` 收窄方法集，否则 CalDAV 客户端会收到 405；
2. **发现入口**：`/.well-known/caldav` 由后端 301 到 `/dav/`，代理层无需特判；
3. **请求体**：整册同步与导入可能携带较大 body，`client_max_body_size` 不宜过小；
4. **不要缓冲打乱长连接**：保持 `proxy_http_version 1.1`；如需 WebSocket 不必特殊配置（本项目不使用）；
5. **MKCOL 创建日历的路径**：通过 CalDAV `MKCOL` 创建的日历以服务端生成的 `{calendar_id}` 作为资源路径段（如 `/dav/users/{username}/calendars/42/`），客户端请求中的路径段仅作为日历名称建议——下次 `PROPFIND` 以服务端路径为准。

TLS 证书推荐 [certbot](https://certbot.eff.org/) 自动续期：`sudo certbot --nginx -d cal.example.com`。配置完成后记得把 `CALHUB_SERVER_BASE_URL`（或 `server.base_url`）改为 `https://cal.example.com`。

## 健康检查与监控

| 端点 | 用途 |
|---|---|
| `GET /healthz` | 存活探针（无依赖检查，恒为 `ok`） |
| `GET /readyz` | 就绪探针（含数据库连通性） |
| `GET /metrics` | Prometheus 指标（HTTP 时延/错误码、注册登录、事件写入、预约创建/取消、提醒投递、DB 慢查询；`metrics.enabled: false` 可关闭） |

容器编排探针建议：存活用 `/healthz`，就绪用 `/readyz`。

## 升级与回滚

- 单二进制：替换二进制文件后 `systemctl restart calhub`；
- Docker：更新镜像 tag 后重建容器；
- 数据库结构变更由内置迁移在启动时自动执行，**不做降级迁移**——升级前请备份数据目录，回滚时恢复备份。

## 备份与恢复

全部状态都在数据目录中：

| 文件 | 作用 |
|---|---|
| `calhub.db`（及 WAL/SHM） | SQLite 数据库（用户、日历、预约、outbox 邮件） |
| `secret.key` | 自动生成的 JWT 密钥（未显式配置 `auth.jwt_secret` 时） |

```bash
# 备份（低峰执行；或先停服务保证一致性）
sqlite3 data/calhub.db ".backup 'calhub-backup.db'"   # 在线热备
cp data/secret.key secret.key.bak

# 恢复：停服务 → 覆盖数据目录 → 启动
```

## 故障排查

| 现象 | 排查方向 |
|---|---|
| 启动即退出，日志 `mkdir_data_failed` / `open_db_failed` | 数据目录权限：容器内 uid 10001、systemd 下 `calhub` 用户需可写 |
| 邮件未收到 | `mail.mode` 是否为 `smtp`；管理台「邮件」页看投递错误；SMTP 端口与 `tls` 是否匹配 |
| 邮件/分享链接指向 localhost | `server.base_url` 未配置为对外域名 |
| CalDAV 客户端 405 | 反代收窄了方法集（见上文），或中间设备拦截 PROPFIND/REPORT |
| 登录态频繁失效 / 容器重建后全部掉线 | 未固定 `auth.jwt_secret`，且 `secret.key` 未持久化 |
| 反代后限流异常 | 后端限流按 IP 统计，经反代需确认 `X-Forwarded-For` 传递正确 |
