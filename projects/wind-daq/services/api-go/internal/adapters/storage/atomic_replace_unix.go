//go:build !windows

// Package storage 提供 CSV 文件持久化的字节 I/O 实现。
//
// 本文件 atomic_replace_unix.go 实现 Unix 平台的原子文件替换：
// 使用 os.Rename（POSIX 保证原子性）。
//
// POSIX 下 os.Rename 原子，但目录元数据修改未 fsync，断电后 rename 可能丢失
// （文件还在但以 tmp 名存在）。因此在 rename 成功后 fsync 父目录，确保目录
// 条目落盘。Windows 路径已通过 MOVEFILE_WRITE_THROUGH 处理，无需此步骤。
package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// replaceTarget 在 Unix 上用 os.Rename 原子替换目标文件。
// POSIX 保证 rename 原子性；之后 fsync 父目录确保目录条目落盘。
func replaceTarget(tmpPath, targetPath string) error {
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("replace target file: %w", err)
	}
	// fsync 父目录：确保目录元数据修改落盘，断电后 rename 不丢失。
	// 失败仅记录不返回错误：rename 本身已成功，目录 fsync 失败只影响极端断电场景。
	if dir, err := os.Open(filepath.Dir(targetPath)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}
