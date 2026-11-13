# 第三方声明（THIRD_PARTY_NOTICES）

Open Cal Hub（本项目）使用 [MIT 协议](LICENSE) 开源。本文件列出发布物中随附的第三方依赖及其协议，按后端（Go）与前端（npm）分组。

- 协议判定依据：Go 模块取本地模块缓存中各版本的 `LICENSE` 文件；npm 包取其 `package.json` 的 `license` 字段。
- 本文件与依赖清单同步维护：`backend/go.mod` 或 `frontend/package.json` 变更后，请一并更新本文件。
- 部分依赖内嵌了自身的第三方组件（已在备注中说明），其声明见各依赖仓库。

## 后端（backend/go.mod）

### 直接依赖

| 模块 | 版本 | 协议 | 备注 |
|---|---|---|---|
| github.com/alexedwards/argon2id | v1.0.0 | MIT | 密码哈希 |
| github.com/emersion/go-ical | v0.0.0-20261112170022-46c5b53a1723 | MIT | iCalendar 解析与序列化 |
| github.com/emersion/go-webdav | v0.7.0 | MIT | CalDAV 协议服务端 |
| github.com/golang-jwt/jwt/v5 | v5.3.1 | MIT | JWT 签发与校验 |
| github.com/teambition/rrule-go | v1.8.2 | MIT | 重复规则展开 |
| gopkg.in/yaml.v3 | v3.0.1 | MIT 或 Apache-2.0（双许可） | 配置文件解析 |
| modernc.org/sqlite | v1.62.0 | BSD-3-Clause | 纯 Go SQLite 驱动；内嵌 SQLite（公有领域，见其 `LICENSE-SQLITE`），另附 `LICENSE-3RD-PARTY.md` |

### 间接依赖

| 模块 | 版本 | 协议 | 备注 |
|---|---|---|---|
| github.com/dustin/go-humanize | v1.1.0 | MIT | sqlite 依赖 |
| github.com/google/uuid | v1.6.0 | BSD-3-Clause | sqlite 依赖 |
| github.com/mattn/go-isatty | v0.0.24 | MIT | sqlite 依赖 |
| github.com/ncruces/go-strftime | v1.1.0 | MIT | sqlite 依赖 |
| github.com/remyoudompheng/bigfft | v0.0.0-20230129092748-24d4a6f8daec | BSD-3-Clause | sqlite 依赖 |
| golang.org/x/crypto | v0.58.0 | BSD-3-Clause | Go 扩展库（含 `PATENTS` 附加专利授权） |
| golang.org/x/sys | v0.49.0 | BSD-3-Clause | Go 扩展库 |
| modernc.org/libc | v1.80.1 | BSD-3-Clause | sqlite 依赖，另附 `LICENSE-3RD-PARTY.md` |
| modernc.org/mathutil | v1.7.1 | BSD-3-Clause | sqlite 依赖 |
| modernc.org/memory | v1.12.1 | BSD-3-Clause | sqlite 依赖 |

## 前端（frontend/package.json）

### 运行时依赖

| 包 | 版本范围 | 协议 |
|---|---|---|
| @element-plus/icons-vue | ^2.3.1 | MIT |
| dayjs | ^1.11.13 | MIT |
| element-plus | ^2.9.1 | MIT |
| vue | ^3.5.13 | MIT |
| vue-router | ^4.5.0 | MIT |

### 开发依赖（不进入运行时发布物）

| 包 | 版本范围 | 协议 |
|---|---|---|
| @vitejs/plugin-vue | ^5.2.1 | MIT |
| typescript | ^5.7.2 | Apache-2.0 |
| vite | ^6.0.5 | MIT |
| vitest | ^2.1.8 | MIT |
| vue-tsc | ^2.2.0 | MIT |

## 再生成方式

协议探测可在本机复现（模块缓存与 `node_modules` 就绪时）：

```bash
# Go 模块：读取 go.mod 列出的模块在本地缓存中的 LICENSE 文件
go mod download -C backend 2>/dev/null || (cd backend && go mod download)
for m in $(cd backend && go list -m all | tail -n +2); do
  v=$(cd backend && go list -m -f '{{.Version}}' "$m")
  d=$(find "$(go env GOMODCACHE)" -maxdepth 4 -type d -path "*/$m@$v" | head -1)
  echo "$m@$v => $(head -2 "$d/LICENSE" | tr '\n' ' ')"
done

# npm 包：读取 node_modules 中各 package.json 的 license 字段
cd frontend && pnpm install --frozen-lockfile
for p in $(python3 -c "import json;d=json.load(open('package.json'));print(' '.join(list(d.get('dependencies',{}))+list(d.get('devDependencies',{}))))"); do
  echo "$p => $(python3 -c "import json;print(json.load(open('node_modules/$p/package.json')).get('license'))")"
done
```
