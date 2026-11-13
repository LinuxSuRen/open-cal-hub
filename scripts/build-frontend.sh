#!/usr/bin/env bash
# 构建前端并拷贝产物到 backend/web/dist（embed 需要）。
set -euo pipefail
cd "$(dirname "$0")/.."
pnpm --dir frontend install --frozen-lockfile
pnpm --dir frontend run build
rm -rf backend/web/dist
cp -r frontend/dist backend/web/dist
echo "frontend dist -> backend/web/dist"
