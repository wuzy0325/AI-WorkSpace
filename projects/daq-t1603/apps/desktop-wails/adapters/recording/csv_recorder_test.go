package recording

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"daq-t1603/core"
)

// makeSnapshot 构造测试用快照，16 通道填充 0.1*i。
func makeSnapshot(deviceID string, ts int64) core.TemperatureSnapshot {
	values := make([]float64, 16)
	for i := range values {
		values[i] = float64(i) * 0.1
	}
	return core.TemperatureSnapshot{
		DeviceID:  deviceID,
		Timestamp: ts,
		Values:    values,
		Unit:      "°C",
	}
}

func TestStartStop(t *testing.T) {
	dir := t.TempDir()
	rec := NewCSVRecorder()

	if err := rec.Start(dir, "test"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	session := rec.Status()
	if session.Status != core.RecordingActive {
		t.Fatalf("expected active recording")
	}

	if err := rec.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	session = rec.Status()
	if session.Status != core.RecordingIdle {
		t.Fatalf("expected idle after stop")
	}
}

func TestDoubleStart(t *testing.T) {
	dir := t.TempDir()
	rec := NewCSVRecorder()

	if err := rec.Start(dir, "test"); err != nil {
		t.Fatal(err)
	}
	if err := rec.Start(dir, "test2"); err == nil {
		t.Fatalf("expected error on double start")
	}
	rec.Stop()
}

func TestWriteToClosedRecorder(t *testing.T) {
	rec := NewCSVRecorder()
	snapshot := core.TemperatureSnapshot{DeviceID: "d1", Timestamp: 1, Values: make([]float64, 16)}
	// Write without starting should not error (no-op)
	if err := rec.Write(snapshot); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestStopIdempotent(t *testing.T) {
	rec := NewCSVRecorder()
	if err := rec.Stop(); err != nil {
		t.Fatalf("Stop on idle should not error: %v", err)
	}
	if err := rec.Stop(); err != nil {
		t.Fatalf("double Stop should not error: %v", err)
	}
}

func TestCreateDirIfNotExist(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "subdir")
	rec := NewCSVRecorder()
	if err := rec.Start(dir, "nested"); err != nil {
		t.Fatalf("Start with new nested dir: %v", err)
	}
	rec.Stop()

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatalf("expected directory to be created")
	}
}

// TestWriteAndVerifyFile 验证单设备写入后文件存在且包含表头与数据行。
func TestWriteAndVerifyFile(t *testing.T) {
	dir := t.TempDir()
	rec := NewCSVRecorder()

	if err := rec.Start(dir, "verify"); err != nil {
		t.Fatal(err)
	}

	snapshot := makeSnapshot("dev1", 1700000000000)
	if err := rec.Write(snapshot); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if err := rec.Stop(); err != nil {
		t.Fatal(err)
	}

	session := rec.Status()
	if session.SnapshotCount != 1 {
		t.Fatalf("expected 1 snapshot, got %d", session.SnapshotCount)
	}

	files, err := filepath.Glob(filepath.Join(dir, "verify_dev1_*.csv"))
	if err != nil || len(files) != 1 {
		t.Fatalf("expected 1 CSV file for dev1, got %v %v", files, err)
	}

	content, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "DeviceID,Timestamp,Unit") {
		t.Fatalf("missing header in file: %s", text)
	}
	if !strings.Contains(text, "dev1") {
		t.Fatalf("missing deviceID in file: %s", text)
	}
	if !strings.Contains(text, "°C") {
		t.Fatalf("missing Unit in file: %s", text)
	}
}

// TestMultiDeviceIndependentFiles 验证多设备写入产生独立文件。
// 这是 10 台目标的核心保证：不同设备的帧不会混写到一个文件。
func TestMultiDeviceIndependentFiles(t *testing.T) {
	dir := t.TempDir()
	rec := NewCSVRecorder()
	if err := rec.Start(dir, "multi"); err != nil {
		t.Fatal(err)
	}

	devices := []string{"devA", "devB", "devC"}
	for _, id := range devices {
		for i := 0; i < 5; i++ {
			if err := rec.Write(makeSnapshot(id, int64(i)*1000)); err != nil {
				t.Fatalf("Write %s: %v", id, err)
			}
		}
	}

	// 等待 deviceWriter 消费 + flush
	if err := rec.Stop(); err != nil {
		t.Fatal(err)
	}

	for _, id := range devices {
		files, err := filepath.Glob(filepath.Join(dir, "multi_"+id+"_*.csv"))
		if err != nil || len(files) != 1 {
			t.Fatalf("expected 1 file for %s, got %v", id, files)
		}
		content, err := os.ReadFile(files[0])
		if err != nil {
			t.Fatalf("read %s: %v", id, err)
		}
		text := string(content)
		// 表头 1 行 + 数据 5 行
		if got := strings.Count(text, "\n"); got != 6 {
			t.Fatalf("%s: expected 6 lines (1 header + 5 data), got %d", id, got)
		}
		// 数据行行首为 deviceID，表头不含具体 deviceID 值，故 5 次出现
		if got := strings.Count(text, id); got != 5 {
			t.Fatalf("%s: expected 5 occurrences of deviceID in data rows, got %d", id, got)
		}
	}

	session := rec.Status()
	if session.SnapshotCount != 15 {
		t.Fatalf("expected total 15 snapshots, got %d", session.SnapshotCount)
	}
}

// TestBackpressureDropsFrames 验证队列满时丢帧并触发回调。
//
// 测试 setup：
//   - 用一只读不消费的"卡死" writer 模拟 deviceWriter 阻塞，
//     保证 recCh 必然填满；
//   - 直接构造 deviceWriter 不调 start()，run goroutine 不运行，
//     recCh 内的帧无人消费；
//   - 此 setup 比依赖 deviceWriter 消费速度的旧版本更稳定，
//     不受调度抖动影响。
func TestBackpressureDropsFrames(t *testing.T) {
	dir := t.TempDir()
	rec := NewCSVRecorder()

	var bpCount atomic.Int64
	var lastBp atomic.Pointer[core.BackpressureEvent]
	rec.SetBackpressureHandler(func(e core.BackpressureEvent) {
		bpCount.Add(1)
		lastBp.Store(&e)
	})

	if err := rec.Start(dir, "bp"); err != nil {
		t.Fatal(err)
	}

	// 注入一只"卡死"的 deviceWriter：不 start，run goroutine 不运行
	deadW, err := newDeviceWriter("devBp", dir, "bp")
	if err != nil {
		t.Fatal(err)
	}
	// 用 done channel 阻止 close() 等待（close 会等 closed，但 deadW 从未 start）
	// 这里直接注册到 writers map，跳过 start
	rec.mu.Lock()
	rec.writers["devBp"] = deadW
	rec.mu.Unlock()
	// deadW 从未 start，Stop 时不可调 close（会死锁），
	// 用 goroutine 主动关闭 file + close(done) 模拟资源回收
	defer func() {
		close(deadW.done)
		_ = deadW.file.Close()
	}()

	// deviceRecChCap=8192，灌入 9000 帧必填满队列并触发背压
	totalFrames := deviceRecChCap + 1000
	for i := 0; i < totalFrames; i++ {
		_ = rec.Write(makeSnapshot("devBp", int64(i)))
	}

	// active 翻 false 后 Stop 不再尝试关 deadW（已从 writers 取出）
	// 这里手动 Stop 清理其他状态
	rec.active.Store(false)
	rec.mu.Lock()
	delete(rec.writers, "devBp")
	rec.mu.Unlock()
	_ = rec.Stop()

	dropped := int(rec.droppedTotal.Load())
	if dropped < 1000 {
		t.Fatalf("expected >=1000 frames dropped under pressure, got %d", dropped)
	}
	if bpCount.Load() == 0 {
		t.Fatalf("expected backpressure callback fired, got 0")
	}
	if lastBp.Load() == nil || lastBp.Load().DeviceID != "devBp" {
		t.Fatalf("backpressure event deviceID mismatch")
	}
}

// TestConcurrentWriteAcrossDevices 验证多设备并发写入无竞态、无 panic。
// 用 race detector 跑此用例可捕获底层竞争。
func TestConcurrentWriteAcrossDevices(t *testing.T) {
	dir := t.TempDir()
	rec := NewCSVRecorder()
	if err := rec.Start(dir, "conc"); err != nil {
		t.Fatal(err)
	}

	const deviceCount = 8
	const framesPerDevice = 500
	var wg sync.WaitGroup
	for d := 0; d < deviceCount; d++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := "dev" + string(rune('A'+idx))
			for i := 0; i < framesPerDevice; i++ {
				_ = rec.Write(makeSnapshot(id, int64(i)))
			}
		}(d)
	}
	wg.Wait()

	if err := rec.Stop(); err != nil {
		t.Fatal(err)
	}

	expected := deviceCount * framesPerDevice
	written := rec.Status().SnapshotCount
	dropped := int(rec.droppedTotal.Load())
	if written+dropped != expected {
		t.Fatalf("written(%d)+dropped(%d) != expected(%d)", written, dropped, expected)
	}

	// 每个设备应有独立文件
	for d := 0; d < deviceCount; d++ {
		id := "dev" + string(rune('A'+d))
		files, err := filepath.Glob(filepath.Join(dir, "conc_"+id+"_*.csv"))
		if err != nil || len(files) != 1 {
			t.Fatalf("expected 1 file for %s, got %v err=%v", id, files, err)
		}
	}
}

// TestLazyDeviceWriterCreation 验证设备首次出现时惰性创建 writer。
func TestLazyDeviceWriterCreation(t *testing.T) {
	dir := t.TempDir()
	rec := NewCSVRecorder()
	if err := rec.Start(dir, "lazy"); err != nil {
		t.Fatal(err)
	}

	// 录制开始时无 writer
	rec.mu.Lock()
	n := len(rec.writers)
	rec.mu.Unlock()
	if n != 0 {
		t.Fatalf("expected 0 writers before first Write, got %d", n)
	}

	_ = rec.Write(makeSnapshot("lazyDev", 1))

	// 写入后应创建 writer
	rec.mu.Lock()
	n = len(rec.writers)
	rec.mu.Unlock()
	if n != 1 {
		t.Fatalf("expected 1 writer after first Write, got %d", n)
	}

	rec.Stop()
}

// TestStopDrainsQueuedFrames 验证 Stop 时会 drain 已入队但未消费的帧。
// 这是数据完整性的关键保证：Stop 不应丢失缓冲内的帧。
func TestStopDrainsQueuedFrames(t *testing.T) {
	dir := t.TempDir()
	rec := NewCSVRecorder()
	if err := rec.Start(dir, "drain"); err != nil {
		t.Fatal(err)
	}

	// 快速写入 1000 帧，deviceWriter 可能还没消费完
	for i := 0; i < 1000; i++ {
		_ = rec.Write(makeSnapshot("drainDev", int64(i)))
	}

	// Stop 等待 drain
	start := time.Now()
	if err := rec.Stop(); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("Stop took too long: %v", elapsed)
	}

	// 所有 1000 帧应都落盘（无背压，队列足够大）
	session := rec.Status()
	dropped := int(rec.droppedTotal.Load())
	if session.SnapshotCount != 1000 {
		t.Fatalf("expected 1000 snapshots persisted, got %d (dropped=%d)", session.SnapshotCount, dropped)
	}

	files, _ := filepath.Glob(filepath.Join(dir, "drain_drainDev_*.csv"))
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	content, _ := os.ReadFile(files[0])
	// 表头 1 行 + 数据 1000 行
	if got := strings.Count(string(content), "\n"); got != 1001 {
		t.Fatalf("expected 1001 lines, got %d", got)
	}
}

// TestFatalErrorHandlerOnCreateFailure 验证 writer 创建失败时触发 fatal 回调。
//
// 通过把 outputDir 指向一个已存在的文件路径（而非目录），
// 让 newDeviceWriter 内 os.Create 失败（"not a directory"）。
func TestFatalErrorHandlerOnCreateFailure(t *testing.T) {
	baseDir := t.TempDir()
	// 在 baseDir 下创建一个名为 "blocker" 的文件，作为"假目录"
	blockerPath := filepath.Join(baseDir, "blocker")
	if err := os.WriteFile(blockerPath, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	rec := NewCSVRecorder()

	var fatalCount atomic.Int64
	var lastFatal atomic.Pointer[string]
	rec.SetFatalErrorHandler(func(deviceID string, err error) {
		fatalCount.Add(1)
		s := deviceID
		lastFatal.Store(&s)
	})

	// Start 用 blocker 文件路径当 outputDir：MkdirAll 会返回错误（not a directory）
	if err := rec.Start(blockerPath, "fatal"); err == nil {
		// 某些平台 MkdirAll 对已存在文件不报错，则手动构造失败场景
		rec.Stop()
		t.Skip("MkdirAll did not fail on file path; platform difference")
	}

	// 走到这里说明 Start 已失败，recorder 未启动。
	// 改用直接调用 newDeviceWriter 验证 fatal 路径：
	// 先正常 Start 到合法 baseDir
	rec2 := NewCSVRecorder()
	rec2.SetFatalErrorHandler(func(deviceID string, err error) {
		fatalCount.Add(1)
		s := deviceID
		lastFatal.Store(&s)
	})
	if err := rec2.Start(baseDir, "fatal"); err != nil {
		t.Fatal(err)
	}

	// 直接调用 newDeviceWriter（不通过 getOrCreateWriter）触发文件创建失败：
	// 用 baseDir 下的 "blocker" 文件当 outputDir
	_, err := newDeviceWriter("devFail", blockerPath, "fatal")
	if err == nil {
		t.Skip("newDeviceWriter did not fail; platform allows creating file under file path")
	}

	// 验证通过 getOrCreateWriter 路径也能触发 fatal：
	// 临时把 outputDir 改为 blockerPath（绕过 Start 的 MkdirAll 校验）
	rec2.mu.Lock()
	rec2.outputDir = blockerPath
	rec2.mu.Unlock()

	// 重置 fatalCount（前面的失败是手动调用 newDeviceWriter）
	fatalCount.Store(0)
	_ = rec2.Write(makeSnapshot("devFail", 1))

	if fatalCount.Load() == 0 {
		t.Fatalf("expected fatal handler fired on writer creation failure")
	}
	if lastFatal.Load() == nil || *lastFatal.Load() != "devFail" {
		t.Fatalf("fatal deviceID mismatch")
	}

	rec2.Stop()
}
