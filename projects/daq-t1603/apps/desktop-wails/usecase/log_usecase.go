package usecase

import "daq-t1603/ports"

// LogUsecase 日志持久化业务逻辑
type LogUsecase struct {
	logPort ports.LogPort
}

// NewLogUsecase 创建日志 usecase
func NewLogUsecase(logPort ports.LogPort) *LogUsecase {
	return &LogUsecase{logPort: logPort}
}

// Start 开始将日志写入文件
func (uc *LogUsecase) Start(outputDir string, prefix string) error {
	return uc.logPort.Start(outputDir, prefix)
}

// Write 写入一条日志到文件
func (uc *LogUsecase) Write(timestamp int64, level string, category string, deviceID string, source string, message string, detail string) error {
	return uc.logPort.Write(timestamp, level, category, deviceID, source, message, detail)
}

// Stop 停止写入并关闭文件
func (uc *LogUsecase) Stop() error {
	return uc.logPort.Stop()
}

// IsActive 日志文件写入是否激活
func (uc *LogUsecase) IsActive() bool {
	return uc.logPort.IsActive()
}

// GetOutputDir 获取当前输出目录，未激活时返回空字符串
func (uc *LogUsecase) GetOutputDir() string {
	return uc.logPort.GetOutputDir()
}
