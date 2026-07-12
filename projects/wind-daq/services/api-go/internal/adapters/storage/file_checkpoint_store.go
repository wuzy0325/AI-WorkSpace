package storage

import (
	"os"
	"sync"
)

// FileCheckpointStore 基于 os 文件系统的断点存储实现
// 实现 ports.CheckpointStore，封装断点文件的字节 I/O。
// usecase 通过 port 接口调用，避免直接 import os。
type FileCheckpointStore struct {
	mu sync.RWMutex
}

// NewFileCheckpointStore 创建文件系统断点存储
func NewFileCheckpointStore() *FileCheckpointStore {
	return &FileCheckpointStore{}
}

// Stat 返回路径是否存在
func (s *FileCheckpointStore) Stat(path string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
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
func (s *FileCheckpointStore) Read(path string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return os.ReadFile(path)
}

// Write 原子写入：同目录临时文件完成写入和同步后再替换权威文件
// 实际逻辑委托给 atomicReplaceFile，与 CSV 截断重写共享同一原子替换实现。
func (s *FileCheckpointStore) Write(path string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return atomicReplaceFile(path, data, 0o644)
}

// Remove 删除文件（不存在时返回 nil）
func (s *FileCheckpointStore) Remove(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(path)
	if err == nil {
		return nil
	}
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
