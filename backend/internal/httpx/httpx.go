package httpx

import (
	"encoding/json"
	"net/http"
)

// WriteData 输出成功包络 {"data": ...}。
func WriteData(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, map[string]any{"data": data})
}

// WriteList 输出列表包络。
func WriteList(w http.ResponseWriter, page, pageSize, total int, data any) {
	writeJSON(w, http.StatusOK, map[string]any{"data": data, "page": page, "page_size": pageSize, "total": total})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// ReadJSON 严格解析请求体（禁止未知字段之外的宽松行为，空体返回 nil map）。
func ReadJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if err.Error() == "EOF" {
			return Validation(map[string]string{"body": "请求体不能为空"})
		}
		return Validation(map[string]string{"body": "请求格式不正确"})
	}
	return nil
}

// PageParams 解析分页参数（page≥1，page_size 默认 20 上限 100）。
func PageParams(r *http.Request) (page, pageSize int) {
	page, pageSize = 1, 20
	if v := r.URL.Query().Get("page"); v != "" {
		if n := atoi(v); n > 0 {
			page = n
		}
	}
	if v := r.URL.Query().Get("page_size"); v != "" {
		if n := atoi(v); n > 0 {
			pageSize = n
		}
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
		if n > 1_000_000 {
			return n
		}
	}
	return n
}
