package storage

import (
	"fmt"
	"os"
)

// FileCheckpointStore 基于 os 文件系统的断点存储实现
// 实现 ports.CheckpointStore，封装断点文件的字节 I/O。
// usecase 通过 port 接口调用，避免直接 import os。
type FileCheckpointStore struct{}

// NewFileCheckpointStore 创建文件系统断点存储
func NewFileCheckpointStore() *FileCheckpointStore {
	return &FileCheckpointStore{}
}

// Stat 返回路径是否存在
func (FileCheckpointStore) Stat(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// Read 读取文件全部内容
func (FileCheckpointStore) Read(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// Write 原子写入：先写临时文件再 rename，避免半写入状态
func (FileCheckpointStore) Write(path string, data []byte) error {
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write tmp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		// rename 失败时清理临时文件
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename tmp to target: %w", err)
	}
	return nil
}

// Remove 删除文件（不存在时返回 nil）
func (FileCheckpointStore) Remove(path string) error {
	err := os.Remove(path)
	if err == nil {
		return nil
	}
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
