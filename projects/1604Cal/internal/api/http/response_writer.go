package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"cal1604/internal/api/dto"
	apperrors "cal1604/internal/errors"
)

func writeSuccess[T any](w http.ResponseWriter, status int, data T) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(dto.Response[T]{
		Success: true,
		Data:    data,
	})
}

// decodeJSON 从请求体解码 JSON 到 T，未知字段报错返回 ErrInvalidArgument。
func decodeJSON[T any](r *http.Request) (T, error) {
	var v T
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&v); err != nil {
		return v, errors.Join(apperrors.ErrInvalidArgument, err)
	}
	return v, nil
}

// decodeJSONOptional 解码"请求体可省略"的端点：空请求体返回零值不报错，
// 其余校验行为与 decodeJSON 一致（未知字段仍报错）。
func decodeJSONOptional[T any](r *http.Request) (T, error) {
	if r.ContentLength == 0 {
		var zero T
		return zero, nil
	}
	return decodeJSON[T](r)
}
