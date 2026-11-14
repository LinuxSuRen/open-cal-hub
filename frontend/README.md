# CalHub 前端

Vue 3 + Vite + TypeScript 单页应用；构建产物 `dist/` 由后端 embed 成单一二进制（`scripts/build-frontend.sh`）。

## 开发

```bash
pnpm install
pnpm run dev        # 开发服务器
pnpm run build      # 产物 dist/
pnpm test           # vitest（时区/DST 边界、槽位分组、错误码映射等）
```

## 结构

- `src/api/`：fetch 封装的 API 层（契约见 `docs/api-contract.md`）
- `src/views/{auth,app,admin,pub}/`：按路由域组织的页面
- `src/components/{calendar,booking,admin,ds}/`：领域组件与设计系统
- `src/i18n/`：语言包（按域拆分的合并字典）

## 已知限制

- **语言切换需重新导航**：i18n 基座为静态合并字典，切换语言后已渲染的页面文案需重新进入路由才刷新。若需即时生效需改造为响应式字典注入（涉及 i18n 模块整体重构，见联调 issue #8 第 5 项裁决记录）。

## 约定

- 中文为主文案；操作动词全流程一致；新增用户可见错误直接使用后端稳定错误码映射
- 时间展示一律用户时区；与后端交互一律 UTC `Z` 后缀
