// Package storage 提供 CSV 文件持久化的字节 I/O 实现。
//
// 本文件 atomic_replace.go 实现跨平台原子文件替换：写临时文件 → Sync → Close → Rename。
// Windows 使用 windows.MoveFileEx(MOVEFILE_REPLACE_EXISTING|MOVEFILE_WRITE_THROUGH)，
// 其他平台使用 os.Rename（POSIX 保证原子性）。
//
// 适用场景：checkpoint 落盘、CSV 截断重写等需要在崩溃时保持文件完整性的场景。
package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/sys/windows"
)

// atomicReplaceFile 原子性地将 data 写入 path：
//  1. 在 path 同目录创建临时文件（确保同一卷，rename 才能原子）
//  2. 写入数据 → Chmod → Sync → Close
//  3. 用 MoveFileEx (Windows) 或 os.Rename (Unix) 原子替换 path
//
// 任何步骤失败都会清理临时文件；返回错误时 path 不会被破坏。
//
// 注意：调用方必须确保 path 所在文件未被自己持有打开句柄（Windows 上替换文件需要独占）。
func atomicReplaceFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create parent directory: %w", err)
		}
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
			// 仅在失败路径清理临时文件；成功路径已 Close 且将被 Rename 消费
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(mode); err != nil {
		return fmt.Errorf("set temp file permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	closed = true

	if runtime.GOOS == "windows" {
		from, err := windows.UTF16PtrFromString(tmpPath)
		if err != nil {
			return fmt.Errorf("encode temp path: %w", err)
		}
		to, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return fmt.Errorf("encode target path: %w", err)
		}
		if err := windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
			return fmt.Errorf("replace target file: %w", err)
		}
		return nil
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace target file: %w", err)
	}
	return nil
}
