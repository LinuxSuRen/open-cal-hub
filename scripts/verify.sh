#!/usr/bin/env bash
# 本地一键验证：后端 build/vet/test + 前端 build/test + 组装单二进制产物并冒烟启动。
#
# 用法：
#   ./scripts/verify.sh                # 全量
#   ./scripts/verify.sh backend        # 只跑后端组（build/vet/test）
#   ./scripts/verify.sh frontend       # 只跑前端组（install/build/test）
#   ./scripts/verify.sh assemble       # 只跑组装组（build-frontend.sh + go build + 启动冒烟）
#
# 全部步骤跑完后输出汇总；任一失败退出码非 0。
# 各步骤完整日志保存在临时目录（汇总中给出路径）。
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOG_DIR="$(mktemp -d "${TMPDIR:-/tmp}/calhub-verify.XXXXXX")"
BIN_OUT="$LOG_DIR/calhub"

# ---------------------------------------------------------------- 运行框架
declare -a STEP_NAMES=()
declare -a STEP_CMDS=()
declare -a STEP_RC=()

add_step() { # add_step <组> <名称> <命令（在仓库根执行）>
  STEP_NAMES+=("$1.$2")
  STEP_CMDS+=("$3")
}

run_all() {
  local total=${#STEP_NAMES[@]}
  local i=0
  for idx in "${!STEP_NAMES[@]}"; do
    local name="${STEP_NAMES[$idx]}" cmd="${STEP_CMDS[$idx]}"
    local log="$LOG_DIR/${name//./_}.log"
    i=$((idx + 1))
    printf '[%d/%d] %-24s' "$i" "$total" "$name"
    local t0=$SECONDS
    bash -c "$cmd" >"$log" 2>&1
    local rc=$?
    STEP_RC+=("$rc")
    local dur=$((SECONDS - t0))s
    if [ "$rc" -eq 0 ]; then
      printf 'ok   (%s)\n' "$dur"
    else
      printf 'FAIL (rc=%d, %s)\n' "$rc" "$dur"
      echo "------ 错误日志（$log 末尾 30 行） ------"
      tail -n 30 "$log" | sed 's/^/  | /'
      echo "------------------------------------------"
    fi
  done
}

summary() {
  local failed=0
  echo
  echo "================ 验证汇总 ================"
  for idx in "${!STEP_NAMES[@]}"; do
    local name="${STEP_NAMES[$idx]}" rc="${STEP_RC[$idx]}"
    if [ "$rc" -eq 0 ]; then
      printf 'PASS  %s\n' "$name"
    else
      printf 'FAIL  %s\n' "$name"
      failed=$((failed + 1))
    fi
  done
  echo "完整日志目录：$LOG_DIR"
  if [ "$failed" -gt 0 ]; then
    echo "结果：$failed 个步骤失败"
    return 1
  fi
  echo "结果：全部通过 ✓"
  return 0
}

# ---------------------------------------------------------------- 步骤定义
GROUP="${1:-all}"

if [[ "$GROUP" == all || "$GROUP" == backend ]]; then
  add_step backend build 'cd backend && go build ./...'
  add_step backend vet   'cd backend && go vet ./...'
  add_step backend test  'cd backend && go test ./... -race -count=1'
fi

if [[ "$GROUP" == all || "$GROUP" == frontend ]]; then
  add_step frontend install 'cd frontend && pnpm install --frozen-lockfile'
  add_step frontend build   'cd frontend && pnpm run build'
  add_step frontend test    'cd frontend && pnpm test'
fi

if [[ "$GROUP" == all || "$GROUP" == assemble ]]; then
  # 组装：前端产物 embed 进二进制；随后用独立临时数据目录启动并探活 /healthz。
  add_step assemble build-frontend './scripts/build-frontend.sh'
  add_step assemble build-binary "cd backend && go build -o '$BIN_OUT' ./cmd/calhub"
  add_step assemble boot-smoke "
    set -e
    smoke_dir=\"\$(mktemp -d '$LOG_DIR'/smoke.XXXXXX)\"
    smoke_port=\$((18080 + RANDOM % 2000)) # 并行 verify.sh 时降低撞端口概率
    CALHUB_SERVER_ADDRESS=127.0.0.1:\$smoke_port CALHUB_SERVER_BASE_URL=http://127.0.0.1:\$smoke_port \
      CALHUB_SERVER_DATA_DIR=\"\$smoke_dir\" CALHUB_DB_PATH=\"\$smoke_dir/calhub.db\" \
      CALHUB_MAIL_MODE=outbox '$BIN_OUT' >\"\$smoke_dir/server.log\" 2>&1 &
    pid=\$!
    trap \"kill \$pid 2>/dev/null || true\" EXIT
    for _ in \$(seq 1 50); do
      if curl -fsS http://127.0.0.1:\$smoke_port/healthz >/dev/null 2>&1; then
        echo 'healthz ok'; exit 0
      fi
      if ! kill -0 \$pid 2>/dev/null; then break; fi
      sleep 0.2
    done
    echo 'boot smoke failed; server log:'; tail -n 20 \"\$smoke_dir/server.log\"; exit 1
  "
fi

if [ ${#STEP_NAMES[@]} -eq 0 ]; then
  echo "未知分组：$GROUP（可用：backend | frontend | assemble | all）" >&2
  exit 2
fi

cd "$ROOT"
run_all
summary
