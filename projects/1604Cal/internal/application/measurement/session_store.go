package measurement

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SessionStore 计量会话持久化存储。
type SessionStore struct {
	dir string
}

// NewSessionStore 创建会话存储实例。
func NewSessionStore(baseDir string) (*SessionStore, error) {
	dir := filepath.Join(baseDir, "measurement_sessions")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}
	return &SessionStore{dir: dir}, nil
}

// Save 将会话保存为 JSON 文件。
func (ss *SessionStore) Save(session *Session) error {
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	filename := fmt.Sprintf("%s.json", session.ID)
	path := filepath.Join(ss.dir, filename)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write session file: %w", err)
	}
	return nil
}

// List 返回所有已保存会话的摘要列表（倒序，最新的在前）。
func (ss *SessionStore) List() ([]*Session, error) {
	entries, err := os.ReadDir(ss.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read session dir: %w", err)
	}

	sessions := make([]*Session, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(ss.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("measurement session: read file %s: %v", entry.Name(), err)
			continue
		}
		var s Session
		if err := json.Unmarshal(data, &s); err != nil {
			log.Printf("measurement session: parse file %s: %v", entry.Name(), err)
			continue
		}
		sessions = append(sessions, &s)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartTime.After(sessions[j].StartTime)
	})

	return sessions, nil
}

// Get 根据会话 ID 加载完整会话。
func (ss *SessionStore) Get(id string) (*Session, error) {
	filename := fmt.Sprintf("%s.json", id)
	path := filepath.Join(ss.dir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("session not found: %s", id)
		}
		return nil, fmt.Errorf("read session file: %w", err)
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}
	return &s, nil
}

// Delete 删除指定会话文件。
func (ss *SessionStore) Delete(id string) error {
	filename := fmt.Sprintf("%s.json", id)
	path := filepath.Join(ss.dir, filename)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("delete session file: %w", err)
	}
	return nil
}

