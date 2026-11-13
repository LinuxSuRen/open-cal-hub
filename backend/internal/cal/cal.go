// Package cal 提供日历、事件（含重复规则展开）、分享链接与成员共享的存储与 REST API。
//
// 存储模型：每个事件行是主事件（recurrence_id IS NULL，可能带 RRULE）或
// 某个实例的覆盖（recurrence_id 非空）。VEVENT 原文完整保存在 events.data，
// 索引列（dtstart/dtend/rrule/...）由写入路径同步生成，读取展开时以原文为准。
// 时间一律 UTC RFC3339 秒级文本存取；weekday 0=周日；分钟数 0..1439。
package cal

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"time"
)

// DBTX 是存储层接受的最小数据库接口；*sql.DB 与 *sql.Tx 均满足，
// 使预约下单等事务可以复用本包的写入函数。
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// 时间格式约定：DB 与 API 全部使用 UTC RFC3339 秒级（Z 后缀）。
func fmtTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func nowUTC() time.Time { return time.Now().UTC() }

// NowUTCString 当前 UTC RFC3339 文本（供 dav 等包复用）。
func NowUTCString() string { return fmtTime(nowUTC()) }

// UTCString 任意时间转 UTC RFC3339 文本。
func UTCString(t time.Time) string { return fmtTime(t) }

// 展开安全上限：单个主事件在一次查询中的展开实例数（RRULE 炸弹防护）。
const MaxExpandInstances = 2000

// 事件状态。
const (
	StatusConfirmed = "confirmed"
	StatusTentative = "tentative"
	StatusCancelled = "cancelled"
)

// ValidStatus 校验事件状态取值。
func ValidStatus(s string) bool {
	return s == StatusConfirmed || s == StatusTentative || s == StatusCancelled
}

var (
	hostnameOnce sync.Once
	hostnameVal  string
)

// hostname 返回本机名（去掉点），用于事件 UID 域部分。
func hostname() string {
	hostnameOnce.Do(func() {
		h := os.Getenv("CALHUB_UID_HOSTNAME")
		if h == "" {
			var err error
			h, err = os.Hostname()
			if err != nil || h == "" {
				h = "calhub"
			}
		}
		buf := make([]byte, 0, len(h))
		for i := 0; i < len(h); i++ {
			if h[i] != '.' {
				buf = append(buf, h[i])
			}
		}
		hostnameVal = string(buf)
	})
	return hostnameVal
}

// NewUID 生成形如 calhub-<random16>@<hostname 无点> 的事件 UID。
func NewUID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand 失败极罕见；退化为时间熵保证可用性
		now := nowUTC().UnixNano()
		for i := range b {
			b[i] = byte(now >> (i * 8))
		}
	}
	return fmt.Sprintf("calhub-%s@%s", hex.EncodeToString(b[:]), hostname())
}

// NewToken 生成 URL 安全的随机令牌（明文存储，用于 URL 匹配）。
func NewToken() string {
	var b [18]byte
	if _, err := rand.Read(b[:]); err != nil {
		for i := range b {
			b[i] = byte(nowUTC().UnixNano() >> (i * 8))
		}
	}
	return hex.EncodeToString(b[:])
}

// HashToken 计算令牌哈希（预约管理令牌只存哈希）。
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// calcETag 依据对象内容生成强 ETag（不带引号；协议层输出时自行加引号）。
func calcETag(data, updated string) string {
	sum := sha256.Sum256([]byte(data + "|" + updated))
	return hex.EncodeToString(sum[:16])
}
