package storage

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
)

const traversalResultLogVersion = 1

type traversalResultLogRecord struct {
	Version int                   `json:"version"`
	Phase   string                `json:"phase"`
	Result  traversal.PointResult `json:"result"`
}

type TraversalResultLog struct {
	mu     sync.Mutex
	file   *os.File
	writer *bufio.Writer
	path   string
	taskID string
}

// 编译期接口断言哨兵：确保 TraversalResultLog 始终满足 ports.TraversalResultLogPort 接口契约。
var _ ports.TraversalResultLogPort = (*TraversalResultLog)(nil)

func NewTraversalResultLog() *TraversalResultLog {
	return &TraversalResultLog{}
}

func (l *TraversalResultLog) Open(ctx context.Context, session ports.TraversalOutputSession) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return errors.New("遍历结果日志会话已打开")
	}
	if session.Path == "" || session.TaskID == "" {
		return errors.New("遍历结果日志路径和任务标识不能为空")
	}
	var file *os.File
	var path string
	var err error
	if session.Mode == ports.TraversalOutputCreate {
		// 创建模式：若目标已存在（同一天、同名重跑），自动追加 -2/-3 另存，
		// 与遍历 CSV 保持一致，避免报错拒绝启动、也不覆盖历史数据。
		file, path, err = openCreateUnique(session.Path)
	} else {
		file, err = os.OpenFile(session.Path, os.O_RDWR, 0o644)
		path = session.Path
	}
	if err != nil {
		return fmt.Errorf("打开遍历结果日志失败: %w", err)
	}
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		_ = file.Close()
		return fmt.Errorf("定位遍历结果日志末尾失败: %w", err)
	}
	l.file, l.writer, l.path, l.taskID = file, bufio.NewWriter(file), path, session.TaskID
	return nil
}

func (l *TraversalResultLog) AppendPrepared(ctx context.Context, result traversal.PointResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.writer == nil || l.file == nil {
		return errors.New("遍历结果日志未初始化")
	}
	if result.TaskID != l.taskID || result.CommitSeq == 0 {
		return errors.New("遍历结果日志记录任务标识或提交序号无效")
	}
	data, err := json.Marshal(traversalResultLogRecord{Version: traversalResultLogVersion, Phase: "prepared", Result: result})
	if err != nil {
		return fmt.Errorf("编码遍历结果日志失败: %w", err)
	}
	if _, err := l.writer.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("写入遍历结果日志失败: %w", err)
	}
	return nil
}

func (l *TraversalResultLog) Sync(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.syncLocked()
}

func (l *TraversalResultLog) ReadCommitted(ctx context.Context, commitSeq uint64) ([]traversal.PointResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	records, _, err := l.readRecordsLocked(commitSeq)
	return records, err
}

func (l *TraversalResultLog) ValidateTail(ctx context.Context, commitSeq uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _, err := l.readRecordsLocked(commitSeq)
	return err
}

func (l *TraversalResultLog) TruncateAfter(ctx context.Context, commitSeq uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, offset, err := l.readRecordsLocked(commitSeq)
	if err != nil {
		return err
	}
	if err := l.file.Truncate(offset); err != nil {
		return fmt.Errorf("截断遍历结果日志失败: %w", err)
	}
	if _, err := l.file.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("定位遍历结果日志末尾失败: %w", err)
	}
	l.writer = bufio.NewWriter(l.file)
	return l.file.Sync()
}

func (l *TraversalResultLog) Close(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	syncErr := l.syncLocked()
	closeErr := l.file.Close()
	l.file, l.writer = nil, nil
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func (l *TraversalResultLog) syncLocked() error {
	if l.writer == nil || l.file == nil {
		return errors.New("遍历结果日志未初始化")
	}
	if err := l.writer.Flush(); err != nil {
		return fmt.Errorf("刷新遍历结果日志失败: %w", err)
	}
	if err := l.file.Sync(); err != nil {
		return fmt.Errorf("同步遍历结果日志失败: %w", err)
	}
	return nil
}

func (l *TraversalResultLog) readRecordsLocked(commitSeq uint64) ([]traversal.PointResult, int64, error) {
	if l.file == nil {
		return nil, 0, errors.New("遍历结果日志未初始化")
	}
	if err := l.writer.Flush(); err != nil {
		return nil, 0, fmt.Errorf("刷新遍历结果日志失败: %w", err)
	}
	data, err := os.ReadFile(l.path)
	if err != nil {
		return nil, 0, fmt.Errorf("读取遍历结果日志失败: %w", err)
	}
	lines := bytes.Split(data, []byte{'\n'})
	results := make([]traversal.PointResult, 0, commitSeq)
	seen := make(map[uint64][]byte)
	var committedOffset int64
	var offset int64
	for index, line := range lines {
		if len(line) == 0 {
			if index == len(lines)-1 {
				break
			}
			offset++
			continue
		}
		lineEnd := offset + int64(len(line))
		if index < len(lines)-1 {
			lineEnd++
		}
		var record traversalResultLogRecord
		if err := json.Unmarshal(line, &record); err != nil {
			if uint64(len(results)) >= commitSeq && index == len(lines)-1 {
				break
			}
			return nil, 0, fmt.Errorf("解析遍历结果日志第 %d 行失败: %w", index+1, err)
		}
		if record.Version != traversalResultLogVersion || record.Phase != "prepared" || record.Result.TaskID != l.taskID || record.Result.CommitSeq == 0 {
			return nil, 0, fmt.Errorf("遍历结果日志第 %d 行记录无效", index+1)
		}
		encoded, _ := json.Marshal(record.Result)
		if previous, ok := seen[record.Result.CommitSeq]; ok {
			if !bytes.Equal(previous, encoded) {
				return nil, 0, fmt.Errorf("遍历结果日志提交序号 %d 存在冲突重复", record.Result.CommitSeq)
			}
			return nil, 0, fmt.Errorf("遍历结果日志提交序号 %d 重复", record.Result.CommitSeq)
		}
		seen[record.Result.CommitSeq] = encoded
		if record.Result.CommitSeq <= commitSeq {
			expected := uint64(len(results) + 1)
			if record.Result.CommitSeq != expected {
				return nil, 0, fmt.Errorf("遍历结果日志提交序号缺口: got=%d want=%d", record.Result.CommitSeq, expected)
			}
			results = append(results, record.Result)
			committedOffset = lineEnd
		}
		offset = lineEnd
	}
	if uint64(len(results)) != commitSeq {
		return nil, 0, fmt.Errorf("遍历结果日志提交记录不足: got=%d want=%d", len(results), commitSeq)
	}
	return results, committedOffset, nil
}
