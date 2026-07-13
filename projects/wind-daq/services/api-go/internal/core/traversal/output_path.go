package traversal

import (
	"fmt"
	"path/filepath"
	"strings"
)

// filepath 使用边界：本文件仅使用 filepath.Join/Ext/Clean 三个纯字符串函数。
// 禁止使用 filepath.Abs（调 os.Getwd）、filepath.Glob（读目录）、filepath.Walk
// 等带 I/O 副作用的函数，以保持 core/ 层"零 I/O"硬约束（AGENTS.md）。
//
// 引入 path/filepath 是 core/ 层的新先例，但 Join/Ext/Clean 是纯字符串操作，
// 不构成文件系统访问。后续维护者必须遵守上述边界。

// ResolveOutputPath 根据 SavePath / SaveFileName / TaskID 计算最终 CSV 文件路径。
//
// 拼接规则（与前端"输出目录 + 输出文件名"语义一致）：
//   - SavePath 为空 → 返回相对文件名，最终落盘位置由调用方进程工作目录决定
//   - SavePath 已带 .csv 后缀（大小写不敏感）→ 视为完整文件路径，直接使用
//   - 其余情况 → filepath.Join(SavePath, SaveFileName)，并保证文件名带 .csv 后缀
//
// 路径分隔符一致性：所有返回值统一过 filepath.Clean，确保 Windows 上为反斜杠、
// Unix 上为正斜杠。下游若做字符串比较，必须使用 filepath.Clean 后的值，或用
// filepath.Join 重新派生路径，禁止直接字符串相等比较。
//
// 大小写：Windows 文件系统不区分扩展名大小写，用户输入 .CSV/.Csv 是常见场景，
// 故扩展名比较一律使用 strings.EqualFold，避免误判为目录触发 "is a directory"。
//
// 注意：SavePath 是"目录"，SaveFileName 才是"文件名"。早期 v2 存储初始化错误地把
// SavePath（目录）直接当文件创建路径，导致 O_EXCL 在目录上报 "is a directory"。
// 所有需要完整 CSV 路径的调用方都应通过本函数解析，禁止再用 SavePath 直接当文件路径。
func ResolveOutputPath(cfg Config) string {
	savePath := cfg.SavePath
	saveName := cfg.SaveFileName
	if saveName == "" {
		saveName = fmt.Sprintf("traversal_%s", cfg.TaskID)
	}
	if savePath == "" {
		// 空路径：返回相对文件名，最终落盘位置由调用方进程工作目录决定
		if !strings.EqualFold(filepath.Ext(saveName), ".csv") {
			saveName += ".csv"
		}
		return saveName
	}
	if strings.EqualFold(filepath.Ext(savePath), ".csv") {
		// 完整文件路径：过 Clean 统一分隔符，与 Join 分支保持一致
		return filepath.Clean(savePath)
	}
	if !strings.EqualFold(filepath.Ext(saveName), ".csv") {
		saveName += ".csv"
	}
	return filepath.Join(savePath, saveName)
}

// ResolveResultLogPath 返回与 CSV 同目录、同 stem 的结果日志路径（.results.jsonl）。
// 保证 CSV 与结果日志落在同一目录下，避免结果日志被错放到 SavePath 的父目录。
//
// 前置条件：ResolveOutputPath 返回值必带 .csv 后缀（大小写不敏感）。
// ext 直接取自 csvPath，TrimSuffix 大小写敏感但 ext 与 csvPath 末尾子串完全相同，
// 故无需 EqualFold。
func ResolveResultLogPath(cfg Config) string {
	csvPath := ResolveOutputPath(cfg)
	ext := filepath.Ext(csvPath)
	base := strings.TrimSuffix(csvPath, ext)
	return base + ".results.jsonl"
}
