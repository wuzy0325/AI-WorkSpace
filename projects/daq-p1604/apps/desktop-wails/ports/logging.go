package ports

// LogPort 日志持久化端口接口
type LogPort interface {
	// Start 开始将日志写入文件
	Start(outputDir string, prefix string) error
	// Write 写入一条日志到文件
	Write(timestamp int64, level string, category string, deviceID string, source string, message string, detail string) error
	// Stop 停止写入并关闭文件
	Stop() error
	// IsActive 日志文件写入是否激活
	IsActive() bool
	// GetOutputDir 获取当前输出目录，未激活时返回空字符串
	GetOutputDir() string
}
