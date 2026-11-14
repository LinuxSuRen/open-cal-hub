# 外部日历接入指南

Open Cal Hub 的「外部日历源」（`/app/booking` 页面）支持订阅任意 ICS URL，把其中的忙碌时段纳入预约槽位计算（只读、单向）。本文给出常见邮件日历的 ICS 地址获取方式与注意事项。

## 工作方式

- 添加源：`POST /external-feeds {name, url}`（或界面操作），手动/按需点「同步」
- 同步时拉取 ICS、展开重复规则至未来 120 天，写入本地缓存（`feed_events`）
- 槽位计算自动避开：事件 `TRANSPARENT`（透明）与 `STATUS:CANCELLED` 不算忙碌
- 单次同步上限 2000 个实例（防重复规则炸弹）；失败会记录在源的 `last_error`

## Google Calendar

1. 打开 [calendar.google.com](https://calendar.google.com) → 左侧对应日历的「设置与共享」
2. 「整合日历」→「私密地址」区块 → 复制 **iCal 格式地址**（形如 `https://calendar.google.com/calendar/ical/<邮箱>/private-<token>/basic.ics`）
3. 粘贴到外部日历源 URL

注意：私密地址即隐式凭证——泄露给他人等于公开你的忙碌时段。怀疑泄露时在同一页面点「重置」，再在 CalHub 更新源地址。

## Outlook / Microsoft 365 / 腾讯企业邮箱

Outlook Web（含腾讯企业邮箱使用的 OWA 界面）：

1. 网页版日历 → 设置（齿轮）→ 「日历」→ 「共享日历」（或旧版：日历右键 → 「共享」→「发布日历」）
2. 选择只读的 **ICS 发布链接**（秘密性选「非公开」仅自己可见）
3. 复制 `https://outlook.office365.com/owa/calendar/.../publish.ics` 形式的地址

注意：发布日历是**只读快照链接**，取消发布即失效；企业租户可能禁用此功能（请联系管理员，或使用 Microsoft Graph 订阅——列入后续路线）。

## 腾讯 QQ 邮箱日历

1. QQ 邮箱网页版 → 「日历」→ 左侧「我的日历」→ 目标日历「设置」
2. 「订阅」或「导出」区块 → 复制 iCal 订阅链接

注意：部分版本仅在移动端 App 提供「订阅链接」，网页端如无该入口可从手机端复制。

## Apple Calendar（iCloud）

1. [icloud.com](https://icloud.com) 日历 → 左侧日历「分享」勾选「公共日历」
2. 复制 `webcal://calendar.icloud.com/...` 链接，将 `webcal://` 改为 `https://` 后填入

注意：公共日历同样为隐式凭证；取消分享即失效。

## 安全与运维建议

- ICS 订阅地址都是**隐式令牌**，请像密码一样保管；定期轮换（提供方侧重置后同步更新源）
- CalHub 侧的同步仅做忙碌检测，不拉取事件详情（标题仅用于本地缓存排查）
- 源失效（401/404）会在 `last_error` 提示，槽位计算自动退化为「无该源忙碌」
- 双向同步与 Google OAuth 自动拉取见路线图（依赖 OAuth 基建，issue #2）
