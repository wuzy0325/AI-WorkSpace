package backend

import (
	"bufio"
	"strings"
)

// csvutil.go 集中 5 / 3 / 7 孔 CSV 读取中完全相同的工具函数：
//   - 剥离 UTF-8 BOM
//   - 检测分隔符（tab vs 逗号）
//
// 历史背景：3 个探针 service 各自实现了一份相同逻辑，code-review P3 抽出。

// utf8BOM 是 Windows Excel "另存为 CSV UTF-8" 在首字节加的 BOM。
// 不剥离会让首列名变成 "\uFEFFP1"，colMap["P1"] 查找失败。
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// StripUTF8BOM 检测并丢弃 *bufio.Reader 头部的 UTF-8 BOM（如有）。
// 调用方应在 Peek 之后再交给 csv.NewReader 或 Scanner 使用。
func StripUTF8BOM(br *bufio.Reader) {
	if b, err := br.Peek(3); err == nil && len(b) >= 3 && b[0] == utf8BOM[0] && b[1] == utf8BOM[1] && b[2] == utf8BOM[2] {
		br.Discard(3)
	}
}

// DetectDelimiter 通过比较 Tab 与逗号出现次数判定分隔符。
// Tab 多于逗号则用 Tab（常见于 .txt/.dat 导出），否则用逗号（标准 CSV）。
func DetectDelimiter(line string) rune {
	tabCount := strings.Count(line, "\t")
	commaCount := strings.Count(line, ",")
	if tabCount > commaCount {
		return '\t'
	}
	return ','
}
