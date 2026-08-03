package traversal

import "path/filepath"

// ResolveProbePrefixedPath 为输出文件路径添加 probe ID 文件名前缀（spec FR8）。
//
// 例：D:/out/traversal_t1.csv + "probe1" → D:/out/probe1-traversal_t1.csv。
// 前缀加在文件名 stem 前（目录与扩展名不变），-2/-3 防覆盖机制在带前缀的
// 文件名上照常工作（csvPort.Open 撞名处理后以 OutputPath() 实际路径为准）。
// probeID 为空或文件名已带该前缀时原样返回（幂等）。
func ResolveProbePrefixedPath(csvPath, probeID string) string {
	if probeID == "" {
		return csvPath
	}
	dir := filepath.Dir(csvPath)
	stem := filepath.Base(csvPath)
	prefix := probeID + "-"
	if len(stem) >= len(prefix) && stem[:len(prefix)] == prefix {
		return csvPath
	}
	return filepath.Join(dir, prefix+stem)
}
