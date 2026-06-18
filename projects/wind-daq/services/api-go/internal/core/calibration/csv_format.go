package calibration

import "strconv"

// csv_format.go — CSV 单元格格式化辅助函数（纯函数，无 I/O）
//
// 这些函数将数值格式化为 CSV 单元格字符串。
// 与 csv_schema.go 一起构成 core 层的格式描述，不涉及任何字节 I/O。

// formatFloat 将 float64 格式化为 CSV 单元格字符串
func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// formatInt 将 int 格式化为 CSV 单元格字符串
func formatInt(v int) string {
	return strconv.Itoa(v)
}

// formatInt64 将 int64 格式化为 CSV 单元格字符串
func formatInt64(v int64) string {
	return strconv.FormatInt(v, 10)
}
