-- Open Cal Hub initial schema (SQLite)
-- 约定：时间戳一律 UTC RFC3339 文本；日期 YYYY-MM-DD；分钟数 0..1439；weekday 0=周日..6=周六

CREATE TABLE users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  email TEXT NOT NULL UNIQUE,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  display_name TEXT NOT NULL,
  bio TEXT NOT NULL DEFAULT '',
  timezone TEXT NOT NULL DEFAULT 'UTC',
  locale TEXT NOT NULL DEFAULT 'zh-CN',
  role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user','admin')),
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled','deleted')),
  email_verified_at TEXT,
  last_login_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE auth_tokens (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('verify_email','reset_password','invite')),
  token_hash TEXT NOT NULL UNIQUE,
  payload TEXT NOT NULL DEFAULT '',
  expires_at TEXT NOT NULL,
  used_at TEXT,
  created_at TEXT NOT NULL
);
CREATE INDEX idx_auth_tokens_user ON auth_tokens(user_id, kind);

CREATE TABLE sessions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  refresh_hash TEXT NOT NULL UNIQUE,
  family_id TEXT NOT NULL,
  user_agent TEXT NOT NULL DEFAULT '',
  ip TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  last_used_at TEXT,
  expires_at TEXT NOT NULL,
  revoked_at TEXT
);
CREATE INDEX idx_sessions_user ON sessions(user_id);

CREATE TABLE app_passwords (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  label TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  created_at TEXT NOT NULL,
  last_used_at TEXT
);

CREATE TABLE calendars (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  color TEXT NOT NULL DEFAULT '#4F6EF7',
  timezone TEXT NOT NULL DEFAULT 'UTC',
  is_default INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT
);
CREATE INDEX idx_calendars_owner ON calendars(owner_id) WHERE deleted_at IS NULL;

CREATE TABLE events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  calendar_id INTEGER NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
  uid TEXT NOT NULL,
  recurrence_id TEXT,                -- NULL=主事件/单次事件；非 NULL=该实例的覆盖
  data TEXT NOT NULL,                -- 完整 VCALENDAR 文本
  summary TEXT NOT NULL DEFAULT '',
  location TEXT NOT NULL DEFAULT '',
  dtstart TEXT NOT NULL,             -- UTC（all_day 时为该日期 00:00 UTC）
  dtend TEXT NOT NULL,
  tzid TEXT NOT NULL DEFAULT 'UTC',
  all_day INTEGER NOT NULL DEFAULT 0,
  rrule TEXT NOT NULL DEFAULT '',
  reminders TEXT NOT NULL DEFAULT '[]',  -- 分钟数数组 JSON，如 [10,1440]
  status TEXT NOT NULL DEFAULT 'confirmed' CHECK (status IN ('confirmed','tentative','cancelled')),
  sequence INTEGER NOT NULL DEFAULT 0,
  etag TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (calendar_id, uid, recurrence_id)
);
CREATE INDEX idx_events_calendar_time ON events(calendar_id, dtstart, dtend);
CREATE INDEX idx_events_uid ON events(uid);

CREATE TABLE share_links (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  calendar_id INTEGER NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
  token TEXT NOT NULL UNIQUE,
  label TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  expires_at TEXT,
  revoked_at TEXT
);
CREATE INDEX idx_share_links_cal ON share_links(calendar_id);

CREATE TABLE calendar_members (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  calendar_id INTEGER NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'viewer' CHECK (role IN ('viewer','editor')),
  created_at TEXT NOT NULL,
  UNIQUE (calendar_id, user_id)
);

CREATE TABLE availability_sets (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  timezone TEXT NOT NULL,
  is_default INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE availability_rules (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  availability_id INTEGER NOT NULL REFERENCES availability_sets(id) ON DELETE CASCADE,
  weekday INTEGER NOT NULL CHECK (weekday BETWEEN 0 AND 6),
  start_minute INTEGER NOT NULL CHECK (start_minute BETWEEN 0 AND 1439),
  end_minute INTEGER NOT NULL CHECK (end_minute BETWEEN 1 AND 1440),
  UNIQUE (availability_id, weekday, start_minute)
);

CREATE TABLE availability_dates (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  availability_id INTEGER NOT NULL REFERENCES availability_sets(id) ON DELETE CASCADE,
  date TEXT NOT NULL,                -- YYYY-MM-DD（availability 时区）
  kind TEXT NOT NULL CHECK (kind IN ('available','unavailable')),
  start_minute INTEGER NOT NULL,
  end_minute INTEGER NOT NULL,
  UNIQUE (availability_id, date, start_minute)
);

CREATE TABLE event_types (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  slug TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  duration_minutes INTEGER NOT NULL,
  location_type TEXT NOT NULL DEFAULT 'none' CHECK (location_type IN ('none','online','in_person','phone')),
  location_value TEXT NOT NULL DEFAULT '',
  color TEXT NOT NULL DEFAULT '#4F6EF7',
  buffer_before INTEGER NOT NULL DEFAULT 0,
  buffer_after INTEGER NOT NULL DEFAULT 0,
  slot_interval INTEGER NOT NULL DEFAULT 0,   -- 0 = 与时长一致
  min_notice_minutes INTEGER NOT NULL DEFAULT 0,
  range_type TEXT NOT NULL DEFAULT 'rolling' CHECK (range_type IN ('rolling','fixed')),
  range_days INTEGER NOT NULL DEFAULT 60,
  start_date TEXT,
  end_date TEXT,
  availability_id INTEGER REFERENCES availability_sets(id) ON DELETE SET NULL,
  calendar_id INTEGER NOT NULL REFERENCES calendars(id),
  active INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (user_id, slug)
);

CREATE TABLE external_feeds (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  url TEXT NOT NULL,
  color TEXT NOT NULL DEFAULT '#8A8F98',
  last_synced_at TEXT,
  last_error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE feed_events (             -- 外部源忙碌时段缓存（已展开到未来 120 天）
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  feed_id INTEGER NOT NULL REFERENCES external_feeds(id) ON DELETE CASCADE,
  uid TEXT NOT NULL,
  summary TEXT NOT NULL DEFAULT '',
  busy INTEGER NOT NULL DEFAULT 1,
  dtstart TEXT NOT NULL,
  dtend TEXT NOT NULL,
  UNIQUE (feed_id, uid, dtstart)
);
CREATE INDEX idx_feed_events_time ON feed_events(feed_id, dtstart, dtend);

CREATE TABLE bookings (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  event_type_id INTEGER NOT NULL REFERENCES event_types(id),
  host_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  calendar_id INTEGER NOT NULL REFERENCES calendars(id),
  event_id INTEGER REFERENCES events(id) ON DELETE SET NULL,
  guest_name TEXT NOT NULL,
  guest_email TEXT NOT NULL,
  guest_note TEXT NOT NULL DEFAULT '',
  guest_timezone TEXT NOT NULL DEFAULT 'UTC',
  start_utc TEXT NOT NULL,
  end_utc TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'confirmed' CHECK (status IN ('confirmed','cancelled')),
  cancelled_at TEXT,
  cancelled_by TEXT CHECK (cancelled_by IN ('host','guest')),
  cancel_reason TEXT NOT NULL DEFAULT '',
  manage_token_hash TEXT NOT NULL UNIQUE,
  ics_uid TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX idx_bookings_slot ON bookings(event_type_id, start_utc) WHERE status='confirmed';
CREATE INDEX idx_bookings_host_time ON bookings(host_id, start_utc);

CREATE TABLE reminders (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  event_row_id INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  fire_at TEXT NOT NULL,
  sent_at TEXT,
  dedup_key TEXT NOT NULL UNIQUE,     -- event_row_id:start:minutes
  created_at TEXT NOT NULL
);
CREATE INDEX idx_reminders_due ON reminders(sent_at, fire_at);

CREATE TABLE audit_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  actor_id INTEGER,
  action TEXT NOT NULL,
  target_type TEXT NOT NULL DEFAULT '',
  target_id TEXT NOT NULL DEFAULT '',
  detail TEXT NOT NULL DEFAULT '{}',
  ip TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX idx_audit_time ON audit_logs(created_at);

CREATE TABLE settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE mail_outbox (            -- 开发模式邮件出箱；生产 SMTP 模式也留档最近 N 条用于排查
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  to_addr TEXT NOT NULL,
  subject TEXT NOT NULL,
  body_text TEXT NOT NULL,
  body_html TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','sent','failed')),
  error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  sent_at TEXT
);
CREATE INDEX idx_outbox_time ON mail_outbox(created_at);

CREATE TABLE invite_codes (
  code TEXT PRIMARY KEY,
  created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  used_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  expires_at TEXT,
  used_at TEXT,
  created_at TEXT NOT NULL
);
