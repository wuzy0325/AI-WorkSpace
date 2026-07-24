package backend

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// prbutil.go 抽取 5 / 3 / 7 孔 .prb 文件读取的公共逻辑。
//
// 3 个探针 service 原本各自实现了 readPrbLines / readThreeHolePrbLines / readSevenHolePrbLines，
// 函数体完全一致：打开文件、按行扫描、跳过空行、返回非空行列表。
// 抽到此处后 service 仅保留算法包专属的解析逻辑（LoadInnerPrbLines / LoadOuterPrbLines 等）。

// ReadPrbLines 读取 .prb 文件的全部非空行（已 TrimSpace），供算法包解析。
// 空行（含纯空白）会被跳过；不以 # 开头的注释行不在此处过滤——.prb 文件无注释规范。
func ReadPrbLines(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open PRB file: %w", err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read PRB file: %w", err)
	}
	return lines, nil
}
