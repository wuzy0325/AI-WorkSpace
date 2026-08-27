package dto

// Response 定义统一 API 响应结构。
type Response[T any] struct {
	Success bool   `json:"success"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Data    T      `json:"data"`
}
