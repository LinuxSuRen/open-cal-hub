#!/usr/bin/env bash
# 组装并启动 calhub 二进制（Playwright webServer 调用）。
# 每轮 e2e 使用独立临时数据目录；服务器日志写入 $E2E_DATA_DIR/server.log 便于排障。
#
# 环境变量（由 playwright.config.ts 注入）：
#   E2E_PORT      监听端口（必填）
#   E2E_DATA_DIR  临时数据目录（必填，每轮新建）
#   E2E_BASE_URL  对外 base_url，用于邮件内链接（默认 http://127.0.0.1:$E2E_PORT）
#   E2E_ASSEMBLE  =1 强制重新组装前端产物；默认仅当 backend/web/dist 缺失时组装
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
: "${E2E_PORT:?E2E_PORT required}"
: "${E2E_DATA_DIR:?E2E_DATA_DIR required}"
BASE_URL="${E2E_BASE_URL:-http://127.0.0.1:${E2E_PORT}}"

mkdir -p "$E2E_DATA_DIR" "$ROOT/e2e/.build"

# 前端产物缺失时组装（CI / 本地 verify.sh 通常已提前组装，此处为兜底）
if [ ! -f "$ROOT/backend/web/dist/index.html" ] || [ "${E2E_ASSEMBLE:-0}" = "1" ]; then
  echo "[start-server] assembling frontend dist..." >&2
  "$ROOT/scripts/build-frontend.sh" >&2
fi

echo "[start-server] building calhub binary..." >&2
(cd "$ROOT/backend" && go build -o "$ROOT/e2e/.build/calhub" ./cmd/calhub)

echo "[start-server] starting calhub on 127.0.0.1:${E2E_PORT} (data: ${E2E_DATA_DIR})" >&2
export CALHUB_SERVER_ADDRESS="127.0.0.1:${E2E_PORT}"
export CALHUB_SERVER_BASE_URL="$BASE_URL"
export CALHUB_SERVER_DATA_DIR="$E2E_DATA_DIR"
export CALHUB_DB_PATH="$E2E_DATA_DIR/calhub.db"
export CALHUB_MAIL_MODE=outbox
export CALHUB_LOG_LEVEL="${CALHUB_LOG_LEVEL:-info}"
exec "$ROOT/e2e/.build/calhub" >"$E2E_DATA_DIR/server.log" 2>&1
