package batch

import (
	"errors"
	"testing"

	apperrors "cal1604/internal/errors"
)

// 测试三段式格式：
//   测试前置：构造输入
//   测试步骤：调用被测函数
//   期待结果：断言输出

// mustCreateSession 在测试中创建会话，失败即 t.Fatalf，避免忽略错误。
func mustCreateSession(t *testing.T, svc *Service, config BatchConfig) string {
	t.Helper()
	sessionID, err := svc.CreateSession(config)
	if err != nil {
		t.Fatalf("CreateSession 失败: %v", err)
	}
	return sessionID
}

// mustVerifyBatch 在测试中校验批次，失败即 t.Fatalf。
func mustVerifyBatch(t *testing.T, svc *Service, sessionID, batchID, code string) {
	t.Helper()
	if _, err := svc.VerifyBatch(sessionID, batchID, code); err != nil {
		t.Fatalf("VerifyBatch(%s, %s) 失败: %v", batchID, code, err)
	}
}

func TestValidateConfig_Valid(t *testing.T) {
	// 测试前置：16 通道，分 2 个批次，每批内量程一致
	config := BatchConfig{
		ChannelRanges: buildChannelRanges(16, 0, 1.0, RangeUnitMPa),
		Batches: []BatchGroup{
			{
				BatchID:    "batch-1",
				BatchIndex: 1,
				RangeMin:   0,
				RangeMax:   1.0,
				RangeUnit:  RangeUnitMPa,
				Channels:   buildChannelRanges(8, 0, 1.0, RangeUnitMPa),
				Status:     BatchStatusPending,
			},
			{
				BatchID:    "batch-2",
				BatchIndex: 2,
				RangeMin:   0,
				RangeMax:   10.0,
				RangeUnit:  RangeUnitMPa,
				Channels:   buildChannelRangesFromTo(9, 16, 0, 10.0, RangeUnitMPa),
				Status:     BatchStatusPending,
			},
		},
	}

	// 测试步骤：验证配置
	err := validateConfig(config)

	// 期待结果：无错误
	if err != nil {
		t.Fatalf("期待验证通过，实际错误: %v", err)
	}
}

func TestValidateConfig_WrongChannelCount(t *testing.T) {
	// 测试前置：只有 15 通道
	config := BatchConfig{
		ChannelRanges: buildChannelRanges(15, 0, 1.0, RangeUnitMPa),
	}

	// 测试步骤
	err := validateConfig(config)

	// 期待结果：通道数不匹配错误
	if err == nil {
		t.Fatal("期待通道数不匹配错误，实际无错误")
	}
	if !errors.Is(err, ErrChannelCountMismatch) {
		t.Fatalf("期待 ErrChannelCountMismatch，实际: %v", err)
	}
}

func TestValidateConfig_DuplicateChannelID(t *testing.T) {
	// 测试前置：通道 1 出现两次
	ranges := buildChannelRanges(16, 0, 1.0, RangeUnitMPa)
	ranges[15].ChannelID = 1 // 最后一个通道改为 1，与第一个重复

	config := BatchConfig{ChannelRanges: ranges}

	// 测试步骤
	err := validateConfig(config)

	// 期待结果
	if err == nil {
		t.Fatal("期待通道重复错误，实际无错误")
	}
}

func TestValidateConfig_InvalidRangeValue(t *testing.T) {
	// 测试前置：量程上限等于下限（max <= min 不满足严格大于约束）
	ranges := buildChannelRanges(16, 0, 1.0, RangeUnitMPa)
	ranges[0].RangeMin = 5
	ranges[0].RangeMax = 5 // max == min，违反 max > min

	config := BatchConfig{ChannelRanges: ranges}

	// 测试步骤
	err := validateConfig(config)

	// 期待结果
	if err == nil {
		t.Fatal("期待量程值无效错误，实际无错误")
	}
}

func TestValidateConfig_BatchRangeInconsistent(t *testing.T) {
	// 测试前置：批次 1 声明 0~1 MPa，但包含一个 0~10 MPa 的通道
	config := BatchConfig{
		ChannelRanges: buildChannelRanges(16, 0, 1.0, RangeUnitMPa),
		Batches: []BatchGroup{
			{
				BatchID:    "batch-1",
				BatchIndex: 1,
				RangeMin:   0,
				RangeMax:   1.0,
				RangeUnit:  RangeUnitMPa,
				Channels: []ChannelRange{
					{ChannelID: 1, RangeMin: 0, RangeMax: 1.0, RangeUnit: RangeUnitMPa, Skipped: false},
					{ChannelID: 2, RangeMin: 0, RangeMax: 10.0, RangeUnit: RangeUnitMPa, Skipped: false}, // 不一致
				},
				Status: BatchStatusPending,
			},
		},
	}

	// 测试步骤
	err := validateConfig(config)

	// 期待结果：批次内量程不一致错误
	if err == nil {
		t.Fatal("期待批次内量程不一致错误，实际无错误")
	}
	if !errors.Is(err, ErrBatchRangeInconsistent) {
		t.Fatalf("期待 ErrBatchRangeInconsistent，实际: %v", err)
	}
}

func TestValidateConfig_EmptyBatches(t *testing.T) {
	// 测试前置：批次列表为空
	config := BatchConfig{
		ChannelRanges: buildChannelRanges(16, 0, 1.0, RangeUnitMPa),
		Batches:       []BatchGroup{},
	}

	// 测试步骤
	err := validateConfig(config)

	// 期待结果
	if err == nil {
		t.Fatal("期待批次列表为空错误，实际无错误")
	}
	if !errors.Is(err, ErrInvalidBatchConfig) {
		t.Fatalf("期待 ErrInvalidBatchConfig，实际: %v", err)
	}
}

func TestValidateConfig_DuplicateBatchID(t *testing.T) {
	// 测试前置：两个批次同 BatchID
	config := BatchConfig{
		ChannelRanges: buildChannelRanges(16, 0, 1.0, RangeUnitMPa),
		Batches: []BatchGroup{
			{BatchID: "batch-1", BatchIndex: 1, RangeMin: 0, RangeMax: 1.0, RangeUnit: RangeUnitMPa,
				Channels: buildChannelRanges(8, 0, 1.0, RangeUnitMPa), Status: BatchStatusPending},
			{BatchID: "batch-1", BatchIndex: 2, RangeMin: 0, RangeMax: 10.0, RangeUnit: RangeUnitMPa,
				Channels: buildChannelRangesFromTo(9, 16, 0, 10.0, RangeUnitMPa), Status: BatchStatusPending},
		},
	}

	err := validateConfig(config)

	if err == nil {
		t.Fatal("期待批次 ID 重复错误，实际无错误")
	}
	if !errors.Is(err, ErrInvalidBatchConfig) {
		t.Fatalf("期待 ErrInvalidBatchConfig，实际: %v", err)
	}
}

func TestValidateConfig_OverlappingChannels(t *testing.T) {
	// 测试前置：通道 1 同时属于批次 1 和批次 2
	config := BatchConfig{
		ChannelRanges: buildChannelRanges(16, 0, 1.0, RangeUnitMPa),
		Batches: []BatchGroup{
			{BatchID: "batch-1", BatchIndex: 1, RangeMin: 0, RangeMax: 1.0, RangeUnit: RangeUnitMPa,
				Channels: []ChannelRange{
					{ChannelID: 1, RangeMin: 0, RangeMax: 1.0, RangeUnit: RangeUnitMPa, Skipped: false},
					{ChannelID: 2, RangeMin: 0, RangeMax: 1.0, RangeUnit: RangeUnitMPa, Skipped: false},
				}, Status: BatchStatusPending},
			{BatchID: "batch-2", BatchIndex: 2, RangeMin: 0, RangeMax: 1.0, RangeUnit: RangeUnitMPa,
				Channels: []ChannelRange{
					{ChannelID: 1, RangeMin: 0, RangeMax: 1.0, RangeUnit: RangeUnitMPa, Skipped: false}, // 与批次 1 重叠
					{ChannelID: 3, RangeMin: 0, RangeMax: 1.0, RangeUnit: RangeUnitMPa, Skipped: false},
				}, Status: BatchStatusPending},
		},
	}

	err := validateConfig(config)

	if err == nil {
		t.Fatal("期待通道跨批次重叠错误，实际无错误")
	}
	if !errors.Is(err, ErrInvalidBatchConfig) {
		t.Fatalf("期待 ErrInvalidBatchConfig，实际: %v", err)
	}
}

func TestService_CreateSession_Success(t *testing.T) {
	// 测试前置：合法配置
	svc := NewService()
	config := BatchConfig{
		ChannelRanges: buildChannelRanges(16, 0, 1.0, RangeUnitMPa),
		Batches: []BatchGroup{
			{
				BatchID:    "batch-1",
				BatchIndex: 1,
				RangeMin:   0,
				RangeMax:   1.0,
				RangeUnit:  RangeUnitMPa,
				Channels:   buildChannelRanges(16, 0, 1.0, RangeUnitMPa),
				Status:     BatchStatusPending,
			},
		},
	}

	// 测步骤
	sessionID, err := svc.CreateSession(config)

	// 期待结果
	if err != nil {
		t.Fatalf("期待成功，实际错误: %v", err)
	}
	if sessionID == "" {
		t.Fatal("期待非空 sessionID")
	}
}

func TestService_VerifyBatch_NumericMatch(t *testing.T) {
	// 测试前置：批次量程值为 10.0，操作员输入 "10"
	svc := NewService()
	config := newSingleBatchConfig(10.0, RangeUnitMPa)
	sessionID := mustCreateSession(t, svc, config)

	// 测试步骤：输入 "10"（不带小数）
	result, err := svc.VerifyBatch(sessionID, "batch-1", "10")

	// 期待结果：数值匹配，校验通过
	if err != nil {
		t.Fatalf("期待无错误，实际: %v", err)
	}
	if !result.Valid {
		t.Fatalf("期待校验通过（数值匹配 10 == 10.0），实际: %s", result.Message)
	}
}

func TestService_VerifyBatch_Mismatch(t *testing.T) {
	// 测试前置：批次量程值为 10.0，操作员输入 "1"
	svc := NewService()
	config := newSingleBatchConfig(10.0, RangeUnitMPa)
	sessionID := mustCreateSession(t, svc, config)

	// 测试步骤
	result, err := svc.VerifyBatch(sessionID, "batch-1", "1")

	// 期待结果
	if err != nil {
		t.Fatalf("期待无错误，实际: %v", err)
	}
	if result.Valid {
		t.Fatal("期待校验失败（1 != 10）")
	}
}

func TestService_VerifyBatch_InvalidNumber(t *testing.T) {
	// 测试前置：输入非数字字符串
	svc := NewService()
	config := newSingleBatchConfig(10.0, RangeUnitMPa)
	sessionID := mustCreateSession(t, svc, config)

	// 测试步骤
	result, err := svc.VerifyBatch(sessionID, "batch-1", "abc")

	// 期待结果：返回校验失败（不是 error），提示输入有效数值
	if err != nil {
		t.Fatalf("期待无错误，实际: %v", err)
	}
	if result.Valid {
		t.Fatal("期待校验失败")
	}
}

func TestService_VerifyBatch_Idempotent(t *testing.T) {
	// 测试前置：批次已验证过
	svc := NewService()
	config := newSingleBatchConfig(10.0, RangeUnitMPa)
	sessionID := mustCreateSession(t, svc, config)
	mustVerifyBatch(t, svc, sessionID, "batch-1", "10")

	// 测试步骤：再次验证，输入错误码
	result, err := svc.VerifyBatch(sessionID, "batch-1", "999")

	// 期待结果：幂等，返回通过
	if err != nil {
		t.Fatalf("期待无错误，实际: %v", err)
	}
	if !result.Valid {
		t.Fatal("期待幂等过，实际失败")
	}
}

func TestService_VerifyBatch_OnRunningRejected(t *testing.T) {
	// 测试前置：批次已 start 处于 running
	svc := NewService()
	config := newSingleBatchConfig(10.0, RangeUnitMPa)
	sessionID := mustCreateSession(t, svc, config)
	mustVerifyBatch(t, svc, sessionID, "batch-1", "10")
	if err := svc.StartBatch(sessionID, "batch-1"); err != nil {
		t.Fatalf("StartBatch 失败: %v", err)
	}

	// 测试步骤：running 状态下尝试重新校验
	_, err := svc.VerifyBatch(sessionID, "batch-1", "10")

	// 期待结果：状态迁移非法
	if !errors.Is(err, apperrors.ErrInvalidStateTransition) {
		t.Fatalf("期待 ErrInvalidStateTransition，实际: %v", err)
	}
}

func TestService_StartBatch_WithoutVerify(t *testing.T) {
	// 测试前置：未验证的批次
	svc := NewService()
	config := newSingleBatchConfig(10.0, RangeUnitMPa)
	sessionID := mustCreateSession(t, svc, config)

	// 测试步骤：未验证直接 start
	err := svc.StartBatch(sessionID, "batch-1")

	// 期待结果：前置条件不满足
	if err == nil {
		t.Fatal("期待前置条件不满足错误，实际无错误")
	}
	if !errors.Is(err, apperrors.ErrPrerequisiteNotMet) {
		t.Fatalf("期待 ErrPrerequisiteNotMet，实际: %v", err)
	}
}

func TestService_StartBatch_AfterVerify(t *testing.T) {
	// 测试前置：已验证的批次
	svc := NewService()
	config := newSingleBatchConfig(10.0, RangeUnitMPa)
	sessionID := mustCreateSession(t, svc, config)
	mustVerifyBatch(t, svc, sessionID, "batch-1", "10")

	// 测试步骤
	err := svc.StartBatch(sessionID, "batch-1")

	// 期待结果
	if err != nil {
		t.Fatalf("期待成功，实际: %v", err)
	}
}

func TestService_StartBatch_OnCompletedRejected(t *testing.T) {
	// 测试前置：批次已完成
	svc := NewService()
	config := newSingleBatchConfig(10.0, RangeUnitMPa)
	sessionID := mustCreateSession(t, svc, config)
	mustVerifyBatch(t, svc, sessionID, "batch-1", "10")
	if err := svc.StartBatch(sessionID, "batch-1"); err != nil {
		t.Fatalf("StartBatch 失败: %v", err)
	}
	if err := svc.CompleteBatch(sessionID, "batch-1"); err != nil {
		t.Fatalf("CompleteBatch 失败: %v", err)
	}

	// 测试步骤：completed 状态下尝试 start（已验证标记仍在）
	err := svc.StartBatch(sessionID, "batch-1")

	// 期待结果：状态迁移非法
	if !errors.Is(err, apperrors.ErrInvalidStateTransition) {
		t.Fatalf("期待 ErrInvalidStateTransition，实际: %v", err)
	}
}

func TestService_StartBatch_ConflictWithRunning(t *testing.T) {
	// 测试前置：两个批次，批次 1 已 running，批次 2 已验证
	svc := NewService()
	config := BatchConfig{
		ChannelRanges: buildChannelRanges(16, 0, 1.0, RangeUnitMPa),
		Batches: []BatchGroup{
			{BatchID: "batch-1", BatchIndex: 1, RangeMin: 0, RangeMax: 1.0, RangeUnit: RangeUnitMPa,
				Channels: buildChannelRanges(8, 0, 1.0, RangeUnitMPa), Status: BatchStatusPending},
			{BatchID: "batch-2", BatchIndex: 2, RangeMin: 0, RangeMax: 10.0, RangeUnit: RangeUnitMPa,
				Channels: buildChannelRangesFromTo(9, 16, 0, 10.0, RangeUnitMPa), Status: BatchStatusPending},
		},
	}
	sessionID := mustCreateSession(t, svc, config)
	mustVerifyBatch(t, svc, sessionID, "batch-1", "1")
	mustVerifyBatch(t, svc, sessionID, "batch-2", "10")
	if err := svc.StartBatch(sessionID, "batch-1"); err != nil {
		t.Fatalf("StartBatch(batch-1) 失败: %v", err)
	}

	// 测试步骤：批次 1 running 时尝试 start 批次 2
	err := svc.StartBatch(sessionID, "batch-2")

	// 期待结果：工作流冲突
	if !errors.Is(err, apperrors.ErrWorkflowConflict) {
		t.Fatalf("期待 ErrWorkflowConflict，实际: %v", err)
	}
}

func TestService_CompleteBatch_OnlyRunningAllowed(t *testing.T) {
	// 测试前置：pending 状态的批次
	svc := NewService()
	config := newSingleBatchConfig(10.0, RangeUnitMPa)
	sessionID := mustCreateSession(t, svc, config)
	mustVerifyBatch(t, svc, sessionID, "batch-1", "10")

	// 测试步骤：未 start 直接 complete
	err := svc.CompleteBatch(sessionID, "batch-1")

	// 期待结果：状态迁移非法
	if !errors.Is(err, apperrors.ErrInvalidStateTransition) {
		t.Fatalf("期待 ErrInvalidStateTransition，实际: %v", err)
	}
}

func TestService_CompleteBatch_Success(t *testing.T) {
	// 测试前置：running 状态的批次
	svc := NewService()
	config := newSingleBatchConfig(10.0, RangeUnitMPa)
	sessionID := mustCreateSession(t, svc, config)
	mustVerifyBatch(t, svc, sessionID, "batch-1", "10")
	if err := svc.StartBatch(sessionID, "batch-1"); err != nil {
		t.Fatalf("StartBatch 失败: %v", err)
	}

	// 测试步骤
	err := svc.CompleteBatch(sessionID, "batch-1")

	// 期待结果
	if err != nil {
		t.Fatalf("期待成功，实际: %v", err)
	}
}

func TestService_ResetBatch_OnlyCompletedAllowed(t *testing.T) {
	// 测试前置：pending 状态的批次（未 start）
	svc := NewService()
	config := newSingleBatchConfig(10.0, RangeUnitMPa)
	sessionID := mustCreateSession(t, svc, config)

	// 测试步骤：pending 状态下 reset
	err := svc.ResetBatch(sessionID, "batch-1")

	// 期待结果：状态迁移非法
	if !errors.Is(err, apperrors.ErrInvalidStateTransition) {
		t.Fatalf("期待 ErrInvalidStateTransition，实际: %v", err)
	}
}

func TestService_ResetBatch_AllowsReverify(t *testing.T) {
	// 测试前置：已完成且已验证的批次
	svc := NewService()
	config := newSingleBatchConfig(10.0, RangeUnitMPa)
	sessionID := mustCreateSession(t, svc, config)
	mustVerifyBatch(t, svc, sessionID, "batch-1", "10")
	if err := svc.StartBatch(sessionID, "batch-1"); err != nil {
		t.Fatalf("StartBatch 失败: %v", err)
	}
	if err := svc.CompleteBatch(sessionID, "batch-1"); err != nil {
		t.Fatalf("CompleteBatch 失败: %v", err)
	}

	// 测试步骤：重置批次
	err := svc.ResetBatch(sessionID, "batch-1")

	// 期待结果：重置成功，需要重新验证
	if err != nil {
		t.Fatalf("期待成功，实际: %v", err)
	}

	// 再次 start 应该失败（需重新验证）
	err = svc.StartBatch(sessionID, "batch-1")
	if err == nil {
		t.Fatal("期待重置后 start 失败（需重新验证）")
	}
}

func TestService_ResetBatch_FullRerunPath(t *testing.T) {
	// 测试前置：完成 → 重置 → 重新验证 → start → complete
	svc := NewService()
	config := newSingleBatchConfig(10.0, RangeUnitMPa)
	sessionID := mustCreateSession(t, svc, config)
	mustVerifyBatch(t, svc, sessionID, "batch-1", "10")
	if err := svc.StartBatch(sessionID, "batch-1"); err != nil {
		t.Fatalf("StartBatch 失败: %v", err)
	}
	if err := svc.CompleteBatch(sessionID, "batch-1"); err != nil {
		t.Fatalf("CompleteBatch 失败: %v", err)
	}

	// 测试步骤：reset → reverify → start → complete
	if err := svc.ResetBatch(sessionID, "batch-1"); err != nil {
		t.Fatalf("ResetBatch 失败: %v", err)
	}
	mustVerifyBatch(t, svc, sessionID, "batch-1", "10")
	if err := svc.StartBatch(sessionID, "batch-1"); err != nil {
		t.Fatalf("重新 StartBatch 失败: %v", err)
	}
	if err := svc.CompleteBatch(sessionID, "batch-1"); err != nil {
		t.Fatalf("重新 CompleteBatch 失败: %v", err)
	}

	// 期待结果：批次状态为 completed
	session, err := svc.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession 失败: %v", err)
	}
	if session.Config.Batches[0].Status != BatchStatusCompleted {
		t.Fatalf("期待 completed，实际: %s", session.Config.Batches[0].Status)
	}
}

func TestService_GetSession_NotFound(t *testing.T) {
	// 测试前置：空 Service
	svc := NewService()

	// 测试步骤
	_, err := svc.GetSession("nonexistent")

	// 期待结果
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("期待 ErrSessionNotFound，实际: %v", err)
	}
}

func TestService_GetSession_ReturnsDeepCopy(t *testing.T) {
	// 测试前置：创建会话
	svc := NewService()
	config := newSingleBatchConfig(10.0, RangeUnitMPa)
	sessionID := mustCreateSession(t, svc, config)

	// 测试步骤：GetSession 拿到副本，修改副本不应影响内部
	copy1, err := svc.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession 失败: %v", err)
	}
	copy1.Config.Batches[0].Status = BatchStatusCompleted
	copy1.CurrentBatchIndex = 999

	// 期待结果：内部状态未变
	internal, err := svc.GetSession(sessionID)
	if err != nil {
		t.Fatalf("二次 GetSession 失: %v", err)
	}
	if internal.Config.Batches[0].Status != BatchStatusPending {
		t.Fatalf("期待内部状态仍为 pending，实际: %s", internal.Config.Batches[0].Status)
	}
	if internal.CurrentBatchIndex != -1 {
		t.Fatalf("期待内部 CurrentBatchIndex 仍为 -1，实际: %d", internal.CurrentBatchIndex)
	}
}

func TestService_VerifyBatch_SessionNotFound(t *testing.T) {
	// 测试前置：空 Service
	svc := NewService()

	// 测试步骤
	_, err := svc.VerifyBatch("nonexistent", "batch-1", "10")

	// 期待结果
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("期待 ErrSessionNotFound，实际: %v", err)
	}
}

func TestService_StartBatch_BatchNotFound(t *testing.T) {
	// 测试前置：会话存在但批次不存在
	svc := NewService()
	config := newSingleBatchConfig(10.0, RangeUnitMPa)
	sessionID := mustCreateSession(t, svc, config)

	// 测试步骤
	err := svc.StartBatch(sessionID, "nonexistent-batch")

	// 期待结果
	if !errors.Is(err, ErrBatchNotFound) {
		t.Fatalf("期待 ErrBatchNotFound，实际: %v", err)
	}
}

// newSingleBatchConfig 构造单批次覆盖 16 通道的测试配置。
// min/max 默认 0~max，单位由调用方指定。
func newSingleBatchConfig(max float64, unit RangeUnit) BatchConfig {
	return BatchConfig{
		ChannelRanges: buildChannelRanges(16, 0, max, unit),
		Batches: []BatchGroup{
			{
				BatchID:    "batch-1",
				BatchIndex: 1,
				RangeMin:   0,
				RangeMax:   max,
				RangeUnit:  unit,
				Channels:   buildChannelRanges(16, 0, max, unit),
				Status:     BatchStatusPending,
			},
		},
	}
}

// buildChannelRanges 构造指定数量的通道量程配置（辅助函数）。
// 所有通道使用相同的 (min, max, unit) 三元组。
func buildChannelRanges(count int, min, max float64, unit RangeUnit) []ChannelRange {
	result := make([]ChannelRange, count)
	for i := 0; i < count; i++ {
		result[i] = ChannelRange{
			ChannelID: i + 1,
			RangeMin:  min,
			RangeMax:  max,
			RangeUnit: unit,
			Skipped:   false,
		}
	}
	return result
}

// buildChannelRangesFromTo 构造从 from 到 to 的通道量程配置（辅助函数）。
func buildChannelRangesFromTo(from, to int, min, max float64, unit RangeUnit) []ChannelRange {
	result := make([]ChannelRange, 0, to-from+1)
	for i := from; i <= to; i++ {
		result = append(result, ChannelRange{
			ChannelID: i,
			RangeMin:  min,
			RangeMax:  max,
			RangeUnit: unit,
			Skipped:   false,
		})
	}
	return result
}