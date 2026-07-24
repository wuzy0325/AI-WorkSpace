// Package httpserver helpers：HTTP 响应包装与请求体解析工具。
//
// 设计动机：
//   - 前端 fetch 调用统一从响应 JSON 中读取 { ok, data, error } 信封，
//     避免依赖 HTTP 状态码（fetch 不会因 4xx/5xx 抛异常，调用方仍需手动判断）。
//   - 把 JSON 解析 + 参数校验样板封装为 helper，让 handler 主体保持线性可读。
package httpserver

import (
	"encoding/json"
	"net/http"
)

// apiOK 是成功响应的信封结构。
// data 字段省略时 JSON 中不会出现，前端 void 调用可直接读 ok:true。
type apiOK struct {
	OK   bool `json:"ok"`
	Data any  `json:"data,omitempty"`
}

// apiErr 是失败响应的信封结构。
type apiErr struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

// writeOK 返回 200 + 成功信封。
//   - data 为 nil 时只输出 {"ok":true}，适用于无返回值的 RPC 调用；
//   - data 为切片/结构体时由 encoding/json 自动序列化。
func writeOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(apiOK{OK: true, Data: data})
}

// writeError 返回指定 HTTP 状态码 + 失败信封。
// 同时写 HTTP 状态码便于中间件/代理识别，又写 ok:false 便于前端统一处理。
func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiErr{OK: false, Error: msg})
}

// decodeJSON 把请求体解析到 dst，失败时自动写 400 响应。
// 返回 false 表示已写错误响应，调用方应直接 return。
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return false
	}
	return true
}
