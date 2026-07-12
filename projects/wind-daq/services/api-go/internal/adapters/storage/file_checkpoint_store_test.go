package storage

import (
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
)

func TestCheckpointAtomicReplaceRemainsReadable(t *testing.T) {
	store := NewFileCheckpointStore()
	path := filepath.Join(t.TempDir(), "checkpoint.json")
	type payload struct {
		Version int `json:"version"`
	}
	if err := store.Write(path, []byte(`{"version":0}`)); err != nil {
		t.Fatalf("initial Write returned error: %v", err)
	}

	var wait sync.WaitGroup
	wait.Add(1)
	errCh := make(chan error, 1)
	go func() {
		defer wait.Done()
		for index := 0; index < 200; index++ {
			data, err := store.Read(path)
			if err != nil {
				errCh <- err
				return
			}
			var got payload
			if err := json.Unmarshal(data, &got); err != nil {
				errCh <- err
				return
			}
		}
	}()
	for version := 1; version <= 200; version++ {
		data, err := json.Marshal(payload{Version: version})
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		if err := store.Write(path, data); err != nil {
			t.Fatalf("Write version %d returned error: %v", version, err)
		}
	}
	wait.Wait()
	select {
	case err := <-errCh:
		t.Fatalf("concurrent Read observed invalid checkpoint: %v", err)
	default:
	}
}
