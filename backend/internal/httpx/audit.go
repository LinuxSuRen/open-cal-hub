// 审计日志 helper：管理端操作与安全事件统一落 audit_logs 表。
// detail 为业务键值（JSON 对象入库）；失败只记错误不阻断请求（审计尽力而为，
// 由调用方决定是否依赖结果）。
package httpx

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

type Audit struct{ DB *sql.DB }

// Log 写一条审计记录。actorID<=0 表示无登录主体（如匿名安全事件）；ip 可为空。
func (a *Audit) Log(ctx context.Context, actorID int64, action, targetType, targetID, ip string, detail map[string]any) error {
	if detail == nil {
		detail = map[string]any{}
	}
	d, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	var actor any
	if actorID > 0 {
		actor = actorID
	}
	_, err = a.DB.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_id, action, target_type, target_id, detail, ip, created_at) VALUES (?,?,?,?,?,?,?)`,
		actor, action, targetType, targetID, string(d), ip, time.Now().UTC().Format(time.RFC3339))
	return err
}
