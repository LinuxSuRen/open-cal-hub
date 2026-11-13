// Package httpx 提供响应包络、稳定错误码与通用中间件。
package httpx

import (
	"errors"
	"log/slog"
	"net/http"
)

// Error 是面向用户的稳定错误：Code 稳定不变，Message 为中文兜底文案，
// Cause 为技术根因（只进日志，绝不进响应）。
type Error struct {
	Code    string
	Message string
	Details map[string]string
	Cause   error
}

func (e *Error) Error() string { return "[" + e.Code + "] " + e.Message }
func (e *Error) Unwrap() error { return e.Cause }

// codeStatus 错误码到 HTTP 状态的注册表；新增错误码必须同时登记（与 docs/api-contract.md 对齐）。
var codeStatus = map[string]int{
	"common/validation_failed":     http.StatusBadRequest,
	"common/rate_limited":          http.StatusTooManyRequests,
	"common/unauthorized":          http.StatusUnauthorized,
	"common/forbidden":             http.StatusForbidden,
	"common/not_found":             http.StatusNotFound,
	"common/internal":              http.StatusInternalServerError,
	"auth/invalid_credentials":     http.StatusUnauthorized,
	"auth/email_taken":             http.StatusConflict,
	"auth/username_taken":          http.StatusConflict,
	"auth/invalid_token":           http.StatusBadRequest,
	"auth/token_expired":           http.StatusBadRequest,
	"auth/email_not_verified":      http.StatusForbidden,
	"auth/account_disabled":        http.StatusForbidden,
	"auth/weak_password":           http.StatusBadRequest,
	"auth/reserved_username":       http.StatusBadRequest,
	"auth/invite_required":         http.StatusForbidden,
	"auth/invalid_invite":          http.StatusForbidden,
	"cal/calendar_not_found":       http.StatusNotFound,
	"cal/event_not_found":          http.StatusNotFound,
	"cal/invalid_rrule":            http.StatusBadRequest,
	"cal/invalid_range":            http.StatusBadRequest,
	"cal/import_failed":            http.StatusBadRequest,
	"cal/share_not_found":          http.StatusNotFound,
	"cal/share_expired":            http.StatusGone,
	"cal/member_exists":            http.StatusConflict,
	"cal/member_not_found":         http.StatusNotFound,
	"booking/event_type_not_found": http.StatusNotFound,
	"booking/inactive":             http.StatusForbidden,
	"booking/host_not_found":       http.StatusNotFound,
	"booking/slot_unavailable":     http.StatusConflict,
	"booking/outside_range":        http.StatusBadRequest,
	"booking/insufficient_notice":  http.StatusBadRequest,
	"booking/invalid_slot":         http.StatusBadRequest,
	"booking/not_found":            http.StatusNotFound,
	"booking/already_cancelled":    http.StatusConflict,
	"admin/invalid_setting":        http.StatusBadRequest,
}

func E(code, message string) *Error { return &Error{Code: code, Message: message} }

func (e *Error) WithDetails(d map[string]string) *Error { e.Details = d; return e }
func (e *Error) WithCause(cause error) *Error           { e.Cause = cause; return e }

func Validation(details map[string]string) *Error {
	return &Error{Code: "common/validation_failed", Message: "请检查输入内容", Details: details}
}

func Unauthorized() *Error { return &Error{Code: "common/unauthorized", Message: "请先登录"} }
func Forbidden() *Error    { return &Error{Code: "common/forbidden", Message: "无权执行此操作"} }
func NotFound() *Error     { return &Error{Code: "common/not_found", Message: "资源不存在"} }

// WriteError 统一错误出口：*Error 按注册表状态输出；其余错误一律 500 且不泄漏细节。
func WriteError(w http.ResponseWriter, log *slog.Logger, err error) {
	var he *Error
	if !errors.As(err, &he) {
		he = &Error{Code: "common/internal", Message: "服务暂时不可用，请稍后重试", Cause: err}
	}
	status, ok := codeStatus[he.Code]
	if !ok {
		status = http.StatusInternalServerError
	}
	if log != nil {
		log.Error("user_error", "code", he.Code, "status", status, slog.Any("cause", he.Cause))
	}
	body := map[string]any{"error": map[string]any{"code": he.Code, "message": he.Message}}
	if he.Details != nil {
		body["details"] = he.Details
	}
	writeJSON(w, status, body)
}
