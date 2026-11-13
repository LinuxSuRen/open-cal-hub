// Package auth 中的密码模块：argon2id 哈希与校验（PHC 字符串格式）。
// 使用 alexedwards/argon2id（底层为 golang.org/x/crypto/argon2），参数为库默认：
// m=64MiB, t=1, p=2（RFC 9106 第一推荐档）。
package auth

import (
	"github.com/alexedwards/argon2id"
)

// HashPassword 生成 PHC 格式的 argon2id 哈希。
func HashPassword(password string) (string, error) {
	return argon2id.CreateHash(password, argon2id.DefaultParams)
}

// CheckPassword 校验密码与 PHC 哈希；格式非法视为不匹配（不区分错误原因，避免侧信道）。
func CheckPassword(password, encoded string) bool {
	ok, err := argon2id.ComparePasswordAndHash(password, encoded)
	return err == nil && ok
}
