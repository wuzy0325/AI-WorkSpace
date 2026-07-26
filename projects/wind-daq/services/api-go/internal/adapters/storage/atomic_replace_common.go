// Package storage 提供 CSV 文件持久化的字节 I/O 实现。
//
// 本文件 atomic_replace_common.go 实现跨平台原子文件替换的共享逻辑：
// 写临时文件 → Sync → Close → 平台特定的 rename。
//
// 平台特定的 rename 实现见 atomic_replace_windows.go / atomic_replace_unix.go。
// 共享函数 atomicReplaceFile 是入口，平台 rename 函数为 replaceTarget。
package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// atomicReplaceFile 原子性地将 data 写入 path：
//  1. 在 path 同目录创建临时文件（确保同一卷，rename 才能原子）
//  2. 写入数据 → Chmod → Sync → Close
//  3. 调用平台特定的 replaceTarget 原子替换 path
//
// 临时文件清理策略（关键：避免 rename 失败时残留临时文件）：
//   - consumed 标志仅在 rename 真正成功后置 true
//   - defer 在 !consumed 时 Close + Remove 临时文件
//   - rename 成功后临时文件已被目标路径"消费"，不能再 Remove
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
	// 双标志语义（关键：避免 double close + 避免 rename 失败时残留 .tmp）：
	//   - closed：tmp.Close() 成功后置 true，defer 不再 Close（防止 os.ErrClosed 掩盖真实错误）
	//   - consumed：replaceTarget 成功后置 true，defer 不再 Remove（rename 已消费临时文件）
	// 之前的实现只有 closed 标志，rename 失败时跳过 os.Remove 残留 .tmp；
	// 替换为 consumed 标志后修复了残留，却丢失了 closed 语义导致 defer double close。
	// 双标志同时解决两个问题。
	closed := false
	consumed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		if !consumed {
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
	// 注意：此处不置 consumed=true，rename 失败时仍需清理临时文件

	if err := replaceTarget(tmpPath, path); err != nil {
		return err
	}
	// rename 成功：临时文件已被目标路径消费，defer 不再清理
	consumed = true
	return nil
}
