package backend

import (
	"fmt"
	"os"

	seveninterp "ai-workspace/shared/algorithms/go/sevenhole/interpolation"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// loadSevenHoleCalibrationCsvFiles 读取 7 份 GBK 编码的校准 CSV 文件并构建七孔插值器。
//
// 文件 I/O 与 GB18030 解码在 adapter 层完成——共享算法包只接收 UTF-8 字节，
// 不依赖文件系统与编码库（避免纯插值包被外部依赖污染，便于内存/远程/沙箱场景复用）。
//
// 返回插值器、抖动/表头警告与错误。
func loadSevenHoleCalibrationCsvFiles(innerPath string, outerPaths []string) (*seveninterp.SevenHolePrbInterpolator, []string, error) {
	if len(outerPaths) != 6 {
		return nil, nil, fmt.Errorf("seven-hole outer calibration csv count must be 6, got %d", len(outerPaths))
	}
	innerSrc, err := readSevenHoleCsvSource(innerPath)
	if err != nil {
		return nil, nil, err
	}
	outerSrcs := make([]seveninterp.SevenHoleCSVSource, 6)
	for i, p := range outerPaths {
		src, err := readSevenHoleCsvSource(p)
		if err != nil {
			return nil, nil, err
		}
		outerSrcs[i] = src
	}
	return seveninterp.LoadCalibrationCSVFromUTF8(innerSrc, outerSrcs)
}

// readSevenHoleCsvSource 读取单个 CSV 文件并解码 GB18030→UTF-8。
//
// 使用 GB18030 而非纯 GBK：GB18030 是 GBK 的超集，纯 GBK 内容向后兼容，
// 同时兼容校准软件偶尔输出的扩展汉字（如生僻计量单位）。
func readSevenHoleCsvSource(path string) (seveninterp.SevenHoleCSVSource, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return seveninterp.SevenHoleCSVSource{}, fmt.Errorf("read seven-hole calibration csv %s: %w", path, err)
	}
	utf8, err := simplifiedchinese.GB18030.NewDecoder().Bytes(raw)
	if err != nil {
		return seveninterp.SevenHoleCSVSource{}, fmt.Errorf("decode GB18030 csv %s: %w", path, err)
	}
	return seveninterp.SevenHoleCSVSource{Label: path, Data: utf8}, nil
}
