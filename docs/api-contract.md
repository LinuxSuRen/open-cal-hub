# Open Cal Hub API 契约（v1）

本文件是前后端与测试的**唯一事实源**。字段 snake_case；实现与本文冲突时以本文为准，需变更先改本文。

## 0. 通用约定

- 基础路径 `/api/v1`；内容 `application/json; charset=utf-8`。
- 时间：一律 UTC RFC3339 秒级（`2026-11-13T08:00:00Z`）；日期 `YYYY-MM-DD`；分钟数 0..1439 整数；weekday 0=周日..6=周六；时区为 IANA 名称。
- 成功响应：`200/201`，包络 `{"data": ...}`；列表 `{"data": [...], "page": 1, "page_size": 20, "total": 0}`。
- 错误响应：`{"error": {"code": "auth/invalid_credentials", "message": "邮箱或密码不正确"}, "details": {"字段": "原因"}}`；`message` 为服务端默认中文兜底文案，前端按 `code` 映射本地文案，未知 code 用 message。
- 认证：`Authorization: Bearer <access_token>`（15 分钟）。刷新令牌在 httpOnly Cookie `calhub_rt`（30 天，每次刷新轮换；检测重用则吊销整个 family）。
- 分页：`page`（≥1）`page_size`（默认 20，上限 100）。
- `id` 一律为数据库数值主键。

### 错误码段位表

| 段 | HTTP | 含义 |
|---|---|---|
| common/validation_failed | 400 | 参数校验失败（details 给字段原因） |
| common/rate_limited | 429 | 请求过于频繁，请稍后再试 |
| common/unauthorized | 401 | 未登录或登录已过期 |
| common/forbidden | 403 | 无权执行此操作 |
| common/not_found | 404 | 资源不存在 |
| common/internal | 500 | 服务暂时不可用，请稍后重试 |
| auth/invalid_credentials | 401 | 邮箱或密码不正确 |
| auth/email_taken | 409 | 邮箱已被注册 |
| auth/username_taken | 409 | 用户名已被使用 |
| auth/invalid_token | 400 | 链接无效或已使用 |
| auth/token_expired | 400 | 链接已过期，请重新发起 |
| auth/email_not_verified | 403 | 请先完成邮箱验证 |
| auth/account_disabled | 403 | 账号已被停用 |
| auth/weak_password | 400 | 密码强度不足（≥8 位且含字母与数字） |
| auth/reserved_username | 400 | 该用户名不可用 |
| auth/invite_required | 403 | 当前仅支持邀请注册 |
| auth/invalid_invite | 403 | 邀请码无效或已使用 |
| cal/calendar_not_found | 404 | 日历不存在 |
| cal/event_not_found | 404 | 事件不存在 |
| cal/invalid_rrule | 400 | 重复规则无效 |
| cal/invalid_range | 400 | 时间范围无效（跨度上限 62 天） |
| cal/import_failed | 400 | 导入失败：文件不是有效的日历数据 |
| cal/share_not_found | 404 | 分享不存在或已撤销 |
| cal/share_expired | 410 | 分享已过期 |
| cal/member_exists | 409 | 该用户已在共享名单中 |
| cal/member_not_found | 404 | 共享成员不存在 |
| booking/event_type_not_found | 404 | 预约类型不存在 |
| booking/inactive | 403 | 该预约类型已停用 |
| booking/host_not_found | 404 | 用户不存在 |
| booking/slot_unavailable | 409 | 该时间段已被占用，请选择其他时间 |
| booking/outside_range | 400 | 超出可预约日期范围 |
| booking/insufficient_notice | 400 | 距开始时间太近，无法预约 |
| booking/invalid_slot | 400 | 时间段无效 |
| booking/not_found | 404 | 预约不存在 |
| booking/already_cancelled | 409 | 该预约已取消 |
| admin/invalid_setting | 400 | 设置项无效 |

## 1. 元信息（公开）

`GET /meta` → `{"data": {"site_name": "CalHub", "version": "1.0.0", "registration_mode": "open", "features": {"booking": true}, "default_locale": "zh-CN"}}`

## 2. 认证与账号

### POST /auth/register
```json
{"email": "a@b.c", "password": "passw0rd!", "username": "rick", "display_name": "Rick", "timezone": "Asia/Shanghai", "locale": "zh-CN", "invite_code": "可选"}
```
→ 201 `{"data": {"user": {...User}, "verification_required": true}}`。首个注册用户自动成为 admin（可由配置关闭）。发送验证邮件。username 可省略，由邮箱前缀生成并在冲突时加后缀；规则 `^[a-z0-9][a-z0-9_-]{1,30}$`，保留字：admin、api、u、s、b、dav、login、logout、register、me、pub、static、assets、healthz、readyz、metrics、support、help。

### POST /auth/login
`{"email": "a@b.c", "password": "..."}`
→ 200 `{"data": {"access_token": "...", "expires_in": 900, "user": {...User}}}` + Set-Cookie。email 可用 username 代替。若未验证邮箱且系统要求验证 → `auth/email_not_verified`。

### POST /auth/refresh
凭 Cookie；→ 同 login（轮换 Cookie）。

### POST /auth/logout → 200 `{"data": null}`（吊销会话 + 清 Cookie）

### POST /auth/verify-email `{"token": "..."}` / POST /auth/resend-verification `{"email": "..."}` / POST /auth/forgot-password `{"email": "..."}` / POST /auth/reset-password `{"token": "...", "new_password": "..."}` → `{"data": null}`（忘记密码无论邮箱是否存在都返回成功）

### GET /auth/sessions → `{"data": [{"id": 1, "user_agent": "", "ip": "", "created_at": "", "last_used_at": "", "current": true}]}`
### DELETE /auth/sessions/{id} → `{"data": null}`

### GET /me → `{"data": {...User}}`；PATCH /me `{display_name?, timezone?, locale?, bio?}` → User
### POST /me/password `{current_password, new_password}` → null（吊销其他会话）

### 应用密码（CalDAV 用）
GET /me/app-passwords → `{"data": [{"id":1,"label":"iPhone","created_at":"","last_used_at":""}]}`
POST /me/app-passwords `{label}` → 201 `{"data": {"id":1, "password": "仅此一次返回"}}`
DELETE /me/app-passwords/{id} → null

**User 对象**：`{"id":1, "email":"a@b.c", "username":"rick", "display_name":"Rick", "bio":"", "timezone":"Asia/Shanghai", "locale":"zh-CN", "role":"user", "status":"active", "email_verified": true, "created_at":"", "last_login_at":""}`

## 3. 日历与事件

### GET /calendars
→ `{"data": [{"id":1, "name":"默认日历", "description":"", "color":"#4F6EF7", "timezone":"Asia/Shanghai", "is_default": true, "owner": {"id":1, "username":"rick", "display_name":"Rick"}, "role": "owner|editor|viewer", "created_at":""}]}`（含共享给我的，role 区分）

### POST /calendars `{name, color?, description?, timezone?}` → 201 Calendar；PATCH /calendars/{id}（owner only）→ Calendar；DELETE /calendars/{id} → null（软删除；不允许删除默认日历 → common/validation_failed）。首个日历自动设为默认。

### 事件实例（展开后）
`{"id": 12, "uid": "abc", "calendar_id": 1, "summary": "", "location": "", "description": "", "start": "...Z", "end": "...Z", "all_day": false, "tz": "Asia/Shanghai", "status": "confirmed", "rrule": "FREQ=WEEKLY;BYDAY=MO", "recurring": true, "recurrence_id": "2026-11-16T01:00:00Z", "reminders": [10], "master_id": 12}`。`recurrence_id` 为实例时刻（重复系列的合成实例同样返回该值，前端据此做 `scope=single` 编辑/删除定位）。编辑/删除单实例时传 `scope=single` 生成 override；`scope=all` 操作整个系列。取消状态的事件 status=cancelled（默认查询返回全部，`?include_cancelled=false` 可过滤）。

### GET /calendars/{id}/events?from&to&include_cancelled
跨度 ≤ 62 天 → `{"data": [EventInstance...]}`（重复事件已展开；override 生效；EXDATE 生效）

### POST /calendars/{id}/events
`{summary, start, end, all_day?, tz?, location?, description?, status?, reminders?: [10], rrule?: {freq: "DAILY|WEEKLY|MONTHLY|YEARLY", interval?: 1, byday?: ["MO"], until?: "...Z", count?: 10}}` → 201 EventInstance(master)。校验：end>start；all_day 时 start/end 为日期，按 iCalendar 排他结束语义存储与返回（`start=当日 00:00Z`、`end=结束次日 00:00Z`；end 省略或不晚于 start 时自动取 start 次日）。`until` 接受 RFC3339（`2026-12-31T23:59:59Z`）与 iCalendar 紧凑格式（`20261231T235959Z`）两种写法，输出统一为紧凑格式。

权限语义：事件写操作（POST/PATCH/DELETE）要求 owner/editor，viewer 越权写返回 `403 common/forbidden`；无任何成员关系的用户读取返回 `404 cal/calendar_not_found`（不泄漏日历存在性）。

### PATCH /events/{id}?scope=all `{...同上部分字段}` → EventInstance
### DELETE /events/{id}?scope=single|all → null

### POST /calendars/{id}/import `{ics: "BEGIN:VCALENDAR..."}` → `{"data": {"imported": 3, "skipped": 1}}`（按 UID 去重）

### 分享链接（owner）
POST /calendars/{id}/share-links `{label?, expires_at?}` → 201 `{"id":1, "token":"x7K...", "url": "/s/x7K...", "label":"", "created_at":"", "expires_at":null}`
GET /calendars/{id}/share-links → 列表；DELETE /share-links/{id} → null（撤销）

### 成员共享（owner）
GET /calendars/{id}/members → `{"data":[{"user_id":2,"username":"","display_name":"","role":"viewer","created_at":""}]}`
POST /calendars/{id}/members `{username_or_email: "", role: "viewer|editor"}` → 201；DELETE /calendars/{id}/members/{user_id} → null

## 4. 公开访问（无认证，限流）

### GET /pub/share/{token}
→ `{"data": {"site_name":"", "calendar": {"name":"", "description":"", "color":""}, "owner_display_name": "", "timezone": ""}}`；失效 → cal/share_not_found / cal/share_expired

### GET /pub/share/{token}/events?from&to → `{"data": [EventInstance...]}`（只读摘要）
### GET /pub/share/{token}/ics → `text/calendar`（过去 30 天至未来 365 天展开；`Content-Disposition: filename=calendar.ics`；`Cache-Control: public, max-age=300`）

### 公开预约页
GET /pub/u/{username} → `{"data": {"display_name":"", "bio":"", "timezone":"", "event_types":[{"slug":"intro", "name":"30 分钟会谈", "duration_minutes":30, "color":"#4F6EF7"}]}}`（仅 active；不存在 → booking/host_not_found）
GET /pub/u/{username}/{slug} → `{"data": {...EventTypePublic: slug,name,description,duration_minutes,color,location_type,range_type,range_days,start_date,end_date,availability:{timezone, rules:[{weekday,start_minute,end_minute}], dates:[...]}, buffers:{before,after}, slot_interval, min_notice_minutes}}`
GET /pub/u/{username}/{slug}/slots?start=YYYY-MM-DD&days=7&tz=Asia/Shanghai → `{"data": {"days": [{"date": "2026-11-16", "slots": [{"start": "...Z", "end": "...Z", "available": true}]}]}}`（busy = 主人全部日历事件 + 外部源忙碌 + 该类型已确认预约；应用前后缓冲；days ≤ 31）
POST /pub/u/{username}/{slug}/bookings `{"start": "...Z", "name": "", "email": "", "note": "", "timezone": "Asia/Shanghai", "company": ""}`（company 为蜜罐，非空即静默成功但不入库）→ 201 `{"data": {"booking": {...BookingGuestView}, "manage_url": "/b/..."}}`；冲突 → booking/slot_unavailable

### 预约管理（凭签名链接令牌）
GET /pub/b/{token} → `{"data": {"event_type": {"name":"", "slug":"", "duration_minutes":30}, "host_username": "", "host_display_name": "", "start": "", "end": "", "status": "confirmed", "guest_name": "", "guest_note": "", "timezone": ""}}`（host_username + slug 供改期页复用槽位接口）
POST /pub/b/{token}/cancel `{reason?}` → null（事件置 STATUS:CANCELLED、邮件双方）
POST /pub/b/{token}/reschedule `{"start": "...Z"}` → BookingGuestView（重校验槽位）

**BookingGuestView**：`{"id":1, "event_type_name":"", "host_display_name":"", "start":"", "end":"", "status":"confirmed", "created_at":""}`

## 5. 预约配置（认证）

### 可用时间
GET /availability-sets → `{"data":[{"id":1,"name":"工作时间","timezone":"Asia/Shanghai","is_default":true,"rules":[{"weekday":1,"start_minute":540,"end_minute":1020}],"dates":[{"date":"2026-11-20","kind":"unavailable","start_minute":0,"end_minute":1440}]}]}`
POST /availability-sets `{name, timezone, is_default?, rules: [...], dates?: [...]}` → 201；PATCH /availability-sets/{id}（rules/dates 整体替换）→ 更新后对象；DELETE /availability-sets/{id} → null（被引用时 → common/validation_failed）。注册时自动创建"工作时间"默认集（周一至周五 09:00-17:00）。

### 预约类型
GET /event-types → `{"data": [EventTypeFull...]}`；POST /event-types（字段同公开视图 + `active`, `availability_id`, `calendar_id`；slug `^[a-z0-9-]{1,64}$` 每用户唯一）→ 201；PATCH /event-types/{id}；DELETE /event-types/{id} → null（已有预约保留、公开页 404）
**EventTypeFull**：`{id, slug, name, description, duration_minutes, location_type, location_value, color, buffer_before, buffer_after, slot_interval, min_notice_minutes, range_type, range_days, start_date, end_date, availability_id, calendar_id, active, booking_url: "/u/rick/intro", created_at, updated_at}`

### 外部日历源（忙碌检测）
GET /external-feeds → `{"data":[{"id":1,"name":"公司日历","url":"https://.../basic.ics","color":"#8A8F98","last_synced_at":"","last_error":""}]}`
POST /external-feeds `{name, url, color?}` → 201；DELETE /external-feeds/{id} → null；POST /external-feeds/{id}/sync → `{"data": {"events": 42, "last_error": ""}}`

### 我的预约（主人视角）
GET /me/bookings?from&to&status&page&page_size → `{"data":[{"id":1,"event_type":{"id":1,"name":"","slug":""},"guest_name":"","guest_email":"","guest_note":"","start":"","end":"","status":"confirmed","created_at":""}], "page":1,"page_size":20,"total":1}`
POST /bookings/{id}/cancel `{reason?}` → null

## 6. 管理端（role=admin）

GET /admin/stats → `{"data":{"users":100,"active_users_30d":60,"calendars":210,"events":5200,"bookings_30d":340,"pending_outbox":0}}`
GET /admin/users?page&page_size&query&status&role → 用户列表（含 calendars_count）
PATCH /admin/users/{id} `{status?, role?}` → User；不可对自己降级/停用 → common/validation_failed
DELETE /admin/users/{id} → null（status=deleted，邮箱改写为 `deleted+{id}@invalid`）
POST /admin/users/{id}/send-reset → null（触发重置密码邮件）
GET /admin/bookings?page&page_size&query&status → 全平台预约
GET /admin/audit-logs?page&page_size → `{"data":[{"id":1,"actor_id":1,"action":"user.disable","target_type":"user","target_id":"2","detail":{},"ip":"","created_at":""}]}`
GET /admin/settings → `{"data":{"site_name":"CalHub","registration_mode":"open|invite","require_email_verification":true,"booking_horizon_days":365}}`
PUT /admin/settings `{...}` → 更新后对象（未知键 → admin/invalid_setting）
GET /admin/emails?page&page_size → outbox（含正文，便于联调查验邮件）；
POST /admin/emails/{id}/retry → null
GET /admin/invite-codes → `{"data":[{"code":"...","created_at":"","expires_at":"","used_at":""}]}`（裸数组）；POST /admin/invite-codes `{count?: 1, expires_in_hours?: 168}` → 201 `{"data":{"codes":["7XKQ-4MHB-9WZT", ...]}}`（对象包裹，便于后续扩展字段）；DELETE /admin/invite-codes/{code} → null

## 7. CalDAV（/dav/，非 REST）

- 发现：`/.well-known/caldav` 301 → `/dav/`；OPTIONS 返回 DAV: 1, calendar-access。
- principal：`/dav/users/{username}/`；home set：`/dav/users/{username}/calendars/`；日历：`/dav/users/{username}/calendars/{calendar_id}/`；对象：`.../{uid}.ics`。
- 认证：Basic（username 或 email + 登录密码或应用密码）；测试也接受 Bearer。
- 支持：PROPFIND（深度 0/1）、REPORT（calendar-query、calendar-multiget，含 time-range 与 expand）、GET/PUT（ETag、If-Match/If-None-Match）、DELETE、MKCOL（displayname 创建日历）。PUT 的 VCALENDAR 由 go-webdav 校验（单 VEVENT、必有 UID）。
- 时区：VCALENDAR 含 VTIMEZONE 时尊重 TZID；否则按 UTC。

## 8. 前端路由契约

| 路径 | 页面 | 鉴权 |
|---|---|---|
| /login /register /forgot /reset /verify | 认证流程 | 游客 |
| /app | 工作台首页（今日与近期日程） | 用户 |
| /app/calendar | 日历视图（月/周/日/列表 + 事件编辑） | 用户 |
| /app/calendars | 日历管理（含分享、成员、导入） | 用户 |
| /app/booking | 预约类型与可用时间、外部源配置 | 用户 |
| /app/bookings | 我的预约 | 用户 |
| /app/settings | 个人资料/密码/会话/应用密码 | 用户 |
| /admin | 管理台（仪表盘/用户/预约/审计/设置/邮件） | admin |
| /u/{username}, /u/{username}/{slug} | 公开预约页 | 游客 |
| /s/{token} | 公开日历视图 | 游客 |
| /b/{token} | 预约管理页（取消/改期） | 游客（令牌） |

SPA 路由 fallback 由后端静态服务处理（非 /api、/dav、静态资源前缀一律回 index.html）。

## 9. 邮件（内部接口）

kind：verify_email（含验证链接）、reset_password、booking_confirmed（主人与客人各一封，附 ICS）、booking_cancelled、booking_rescheduled、email_changed、event_reminder（事件提醒 worker 发送，收件人为日历属主）。链接基于配置的 `base_url`。开发模式（mail.mode=outbox）只写 mail_outbox，管理员页与 e2e 用 GET /admin/emails 读取链接；纯文本附件（如 ICS）追加在 body_text 尾部留档。
