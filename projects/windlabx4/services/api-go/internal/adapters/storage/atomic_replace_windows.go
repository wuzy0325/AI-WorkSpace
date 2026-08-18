//go:build windows

// Package storage 提供 CSV 文件持久化的字节 I/O 实现。
//
// 本文件 atomic_replace_windows.go 实现 Windows 平台的原子文件替换：
// 使用 windows.MoveFileEx(MOVEFILE_REPLACE_EXISTING|MOVEFILE_WRITE_THROUGH)。
//
// Windows 上 os.Rename 目标已存在会失败，必须用 MoveFileEx 才能原子替换。
// MOVEFILE_WRITE_THROUGH 确保目录元数据修改落盘，断电后 rename 不丢失。
package storage

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// replaceTarget 在 Windows 上用 MoveFileEx 原子替换目标文件。
// MOVEFILE_REPLACE_EXISTING 允许目标已存在时替换（os.Rename 在 Windows 上做不到）。
// MOVEFILE_WRITE_THROUGH 确保操作完成前数据落盘，断电安全。
func replaceTarget(tmpPath, targetPath string) error {
	from, err := windows.UTF16PtrFromString(tmpPath)
	if err != nil {
		return fmt.Errorf("encode temp path: %w", err)
	}
	to, err := windows.UTF16PtrFromString(targetPath)
	if err != nil {
		return fmt.Errorf("encode target path: %w", err)
	}
	if err := windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return fmt.Errorf("replace target file: %w", err)
	}
	return nil
}
