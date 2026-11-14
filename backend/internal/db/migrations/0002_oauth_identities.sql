-- OAuth 身份绑定（issue #2）：一个账号可绑定多个外部身份。
CREATE TABLE oauth_identities (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider TEXT NOT NULL CHECK (provider IN ('github','google')),
  provider_uid TEXT NOT NULL,
  email_at_link TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  UNIQUE (provider, provider_uid)
);
CREATE INDEX idx_oauth_user ON oauth_identities(user_id);
