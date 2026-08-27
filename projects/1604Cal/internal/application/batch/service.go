// Package batch 实现多量程分批计量的业务逻辑。
//
// 本包负责：
//  1. 验证前端提交的分批配置（量程分组是否一致、批次间通道不重叠等）
//  2. 核对码校验（操作员输入与批次量程值是否匹配）
//  3. 批次会话状态管理（记录当前进度，支持回退重跑）
//
// 加压序列执行本身复用 calibration.Service，本包不重复实现加压/稳定/采集逻辑。
package batch

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"

	apperrors "cal1604/internal/errors"
)

// 通道编号范围常量。一期固定 16 通道，通过常量便于后续扩展。
const (
	MinChannelID = 1
	MaxChannelID = 16
)

// rangeMatchEpsilon 核对码数值匹配的浮点容差。
// 量程值通常是整数或一位小数（1/10/100），1e-9 容差足以避免 IEEE 754 表示差异。
const rangeMatchEpsilon = 1e-9

// RangeUnit 量程单位类型。
type RangeUnit string

const (
	RangeUnitMPa RangeUnit = "MPa"
	RangeUnitKPa RangeUnit = "kPa"
	RangeUnitBar RangeUnit = "bar"
	RangeUnitPsi RangeUnit = "psi"
)

// ChannelRange 单通道量程配置。
//
// 量程用 [RangeMin, RangeMax] 闭区间表示：
//   - RangeMin：量程下限（支持负值，用于负压量程，如 -0.1 MPa）
//   - RangeMax：量程上限（必须 > RangeMin，作为分批标识与核对码）
//
// 分批依据：相同 (RangeMin, RangeMax, RangeUnit) 三元组的通道归为一批。
// 核对码：操作员输入与批次 RangeMax 数值匹配（标准器标识通常标满量程上限）。
type ChannelRange struct {
	ChannelID int       `json:"channelId"`
	RangeMin  float64   `json:"rangeMin"`
	RangeMax  float64   `json:"rangeMax"`
	RangeUnit RangeUnit `json:"rangeUnit"`
	Skipped   bool      `json:"skipped"`
}

// BatchStatus 批次状态。
type BatchStatus string

const (
	BatchStatusPending   BatchStatus = "pending"
	BatchStatusRunning   BatchStatus = "running"
	BatchStatusCompleted BatchStatus = "completed"
)

// BatchGroup 批次分组信息。
//
// 批次内所有通道的 (RangeMin, RangeMax, RangeUnit) 必须一致，由 validateBatches 保证。
type BatchGroup struct {
	BatchID        string             `json:"batchId"`
	BatchIndex     int                `json:"batchIndex"`
	RangeMin       float64            `json:"rangeMin"`
	RangeMax       float64            `json:"rangeMax"`
	RangeUnit      RangeUnit          `json:"rangeUnit"`
	Channels       []ChannelRange     `json:"channels"`
	Status         BatchStatus        `json:"status"`
	CollectedData  map[int][]float64  `json:"collectedData,omitempty"` // channelId -> 采集数据
	PressurePoints []float64          `json:"pressurePoints,omitempty"`
}

// BatchConfig 批次配置请求（由前端传入）。
type BatchConfig struct {
	ChannelRanges []ChannelRange `json:"channelRanges"`
	Batches       []BatchGroup   `json:"batches"`
}

// VerifyRequest 核对码校验请求。
type VerifyRequest struct {
	BatchID          string `json:"batchId"`
	VerificationCode string `json:"verificationCode"`
}

// VerifyResult 核对码校验结果。
type VerifyResult struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
}

// Session 分批计量会话。内存中维护，不持久化（断点不保留）。
type Session struct {
	ID                string         `json:"sessionId"`
	Config            BatchConfig    `json:"config"`
	CurrentBatchIndex int            `json:"currentBatchIndex"` // -1 表示未开始
	VerifiedBatchIDs  map[string]bool `json:"-"`                // 已通过核对码的批次集合
}

// Service 分批计量业务服务。
//
// 职责边界：
//   - 只做配置验证、核对码校验、状态管理
//   - 加压执行委托给 calibration.Service（通过 batch handler 协调）
//   - 会话仅内存维护，不持久化
//
// 并发安全：所有公开方法均通过 s.mu 串行化。GetSession 返回深拷贝，
// 避免外部在外修改内部状态造成数据竞争。
type Service struct {
	mu       sync.Mutex
	sessions map[string]*Session // sessionId -> Session
}

// NewService 创建分批计量服务。
func NewService() *Service {
	return &Service{
		sessions: make(map[string]*Session),
	}
}

// 常见错误。使用 apperrors 保持与现有错误码体系一致。
var (
	ErrSessionNotFound      = errors.New("batch session not found")
	ErrBatchNotFound        = errors.New("batch not found")
	ErrInvalidRangeValue    = errors.New("invalid range value")
	ErrInvalidChannelID     = errors.New("invalid channel id")
	ErrChannelCountMismatch = errors.New("channel count mismatch")
	ErrInvalidBatchConfig   = errors.New("invalid batch config")
	ErrBatchRangeInconsistent = errors.New("batch range inconsistent")
)

// CreateSession 根据前端提交的分批配置创建会话。
//
// 创建时做以下验证：
//  1. 通道数量必须为 16
//  2. 每个通道编号在 1-16 范围内且不重复
//  3. 量程值必须 > 0
//  4. 批次列表非空、BatchID/BatchIndex 合法且唯一
//  5. 批次内通道量程一致、批次间通道不重叠、批次内通道不重复
//
// 返回会话 ID。后续操作通过会话 ID 引用
func (s *Service) CreateSession(config BatchConfig) (string, error) {
	if err := validateConfig(config); err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sessionID := generateSessionID()
	s.sessions[sessionID] = &Session{
		ID:                sessionID,
		Config:            cloneConfig(config),
		CurrentBatchIndex: -1,
		VerifiedBatchIDs:  make(map[string]bool),
	}
	return sessionID, nil
}

// VerifyBatch 核对码校验。
//
// 校验逻辑：将操作员输入的字符串解析为 float64，与批次量程上限（RangeMax）数值比对。
// 采用数值匹配而非字符串匹配：`10` 与 `10.0` 视为相同。
// 使用容差比较（rangeMatchEpsilon）避免 IEEE 754 浮点表示差异。
//
// 选 RangeMax 而非 RangeMin 作为核对码：标准器物理标识通常标注满量程上限，
// 操作员与现场标识比对时直接读上限值最直观。
//
// 校验通过后，该批次标记为已验证，后续 start 时不再需要重复验证。
// 若批次已验证且仍为 pending，直接返回通过（幂等）。
//
// 状态约束：仅 pending 状态允许校验；running/completed 状态拒绝校验，
// 避免在加压进行中或已完成时被重新校验导致状态混乱。
// 状态校验先于幂等校验，确保 running 批次即使带已验证标记也会被拒绝。
func (s *Service) VerifyBatch(sessionID, batchID, code string) (VerifyResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return VerifyResult{}, ErrSessionNotFound
	}

	batch, err := findBatch(session, batchID)
	if err != nil {
		return VerifyResult{}, err
	}

	// 状态约束：running/completed 不允许重新校验
	// 注意：必须先于幂等校验，避免 running 批次因已验证标记被静默放过
	if batch.Status == BatchStatusRunning || batch.Status == BatchStatusCompleted {
		return VerifyResult{}, apperrors.ErrInvalidStateTransition
	}

	// 幂等：已验证的直接通过（pending 状态下重复校验）
	if session.VerifiedBatchIDs[batchID] {
		return VerifyResult{Valid: true, Message: "已验证"}, nil
	}

	// 将输入字符串解析为数值
	inputValue, err := strconv.ParseFloat(code, 64)
	if err != nil {
		return VerifyResult{
			Valid:   false,
			Message: "请输入有效的量程数值",
		}, nil
	}

	// 数值匹配（容差比较）：10 == 10.0
	if math.Abs(inputValue-batch.RangeMax) > rangeMatchEpsilon {
		return VerifyResult{
			Valid:   false,
			Message: fmt.Sprintf("核对码不匹配，请确认标准器量程标识（应为 %g）", batch.RangeMax),
		}, nil
	}

	session.VerifiedBatchIDs[batchID] = true
	return VerifyResult{Valid: true, Message: "验证通过"}, nil
}

// StartBatch 标记批次为运行中，更新当前批次索引。
//
// 前置条件：
//   - 批次必须已通过核对码校验
//   - 批次状态必须为 pending（不允许从 completed/running 直接 start）
//   - 同一会话内无其它批次处于 running（批次必须串行执行）
//
// 实际加压执行由 calibration.Service 负责，本方法只更新状态。
func (s *Service) StartBatch(sessionID, batchID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return ErrSessionNotFound
	}

	batch, err := findBatch(session, batchID)
	if err != nil {
		return err
	}

	if !session.VerifiedBatchIDs[batchID] {
		return apperrors.ErrPrerequisiteNotMet
	}

	// 状态机校验：仅 pending 允许 start
	if batch.Status != BatchStatusPending {
		return apperrors.ErrInvalidStateTransition
	}

	// 串行约束：同一会话内不允许并发执行多个批次。
	// 用 BatchID 比较而非 slice 元素指针比较，避免未来 Batches 改用 map/slice 重新分配时静默失效。
	for i := range session.Config.Batches {
		other := &session.Config.Batches[i]
		if other.BatchID != batch.BatchID && other.Status == BatchStatusRunning {
			return apperrors.ErrWorkflowConflict
		}
	}

	batch.Status = BatchStatusRunning
	session.CurrentBatchIndex = batch.BatchIndex - 1 // BatchIndex 为 1-based，转 0-based
	return nil
}

// CompleteBatch 标记批次完成。
//
// 状态约束：仅 running 状态允许完成，避免跳过加压直接 complete。
func (s *Service) CompleteBatch(sessionID, batchID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return ErrSessionNotFound
	}

	batch, err := findBatch(session, batchID)
	if err != nil {
		return err
	}

	// 状态机校验：仅 running 允许 complete
	if batch.Status != BatchStatusRunning {
		return apperrors.ErrInvalidStateTransition
	}

	batch.Status = BatchStatusCompleted
	return nil
}

// ResetBatch 回退重跑：将已完成批次重置为 pending，清空采集数据。
//
// 状态约束：仅 completed 状态允许回退（running 批次应先 stop 而非 reset）。
// 重置后需要重新通过核对码校验才能 start。
func (s *Service) ResetBatch(sessionID, batchID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return ErrSessionNotFound
	}

	batch, err := findBatch(session, batchID)
	if err != nil {
		return err
	}

	// 状态机校验：仅 completed 允许 reset
	if batch.Status != BatchStatusCompleted {
		return apperrors.ErrInvalidStateTransition
	}

	batch.Status = BatchStatusPending
	batch.CollectedData = nil
	batch.PressurePoints = nil
	delete(session.VerifiedBatchIDs, batchID)
	return nil
}

// GetSession 查询会话（用于状态查询）。
//
// 返回会话的深拷贝，避免外部在锁外修改内部状态造成数据竞争。
// 调用方可安全序列化返回值或读取其字段，但修改不会影响 Service 内部状态。
func (s *Service) GetSession(sessionID string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return cloneSession(session), nil
}

// DeleteSession 删除会话，释放内存。
//
// 用于前端"重新开始"或退出分批模式时清理后端状态。
// 删除不存在的会话不报错（幂等），便于前端容错调用。
func (s *Service) DeleteSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

// findBatch 在会话中查找指定批次。调用方需持有锁。
// 返回的是 slice 元素指针，仅用于 Service 内部持锁修改，不对外暴露。
func findBatch(session *Session, batchID string) (*BatchGroup, error) {
	for i := range session.Config.Batches {
		if session.Config.Batches[i].BatchID == batchID {
			return &session.Config.Batches[i], nil
		}
	}
	return nil, ErrBatchNotFound
}

// validateConfig 验证分批配置的合法性。
//
// 校验项：
//  1. 通道数量必须为 16
//  2. 通道编号在 1-16 范围内且不重复
//  3. 量程区间合法：RangeMax > RangeMin（支持负压量程，如 -0.1~0.1 MPa）
//  4. 批次列表非空、BatchID/BatchIndex 合法且唯一
//  5. 批次内通道 ID 在该批次内唯一
//  6. 批次间通道不重叠（一个通道只能属于一个批次）
//  7. 批次内通道量程与批次声明一致（三元组 RangeMin/RangeMax/RangeUnit）
func validateConfig(config BatchConfig) error {
	// 1. 通道数量校验
	if len(config.ChannelRanges) != MaxChannelID {
		return fmt.Errorf("%w: 期望 %d 通道，实际 %d", ErrChannelCountMismatch, MaxChannelID, len(config.ChannelRanges))
	}

	// 2. 通道编号唯一性 + 范围校验 + 3. 量程区间合法性
	seen := make(map[int]bool, MaxChannelID)
	for _, ch := range config.ChannelRanges {
		if ch.ChannelID < MinChannelID || ch.ChannelID > MaxChannelID {
			return fmt.Errorf("%w: 通道 %d 不在有效范围 [%d, %d]",
				ErrInvalidChannelID, ch.ChannelID, MinChannelID, MaxChannelID)
		}
		if seen[ch.ChannelID] {
			return fmt.Errorf("%w: 通道 %d 重复", ErrInvalidChannelID, ch.ChannelID)
		}
		seen[ch.ChannelID] = true

		// 量程上限必须严格大于下限（支持负压：min 可为负，max 可为 0）
		if ch.RangeMax <= ch.RangeMin {
			return fmt.Errorf("%w: 通道 %d 量程上限必须大于下限（min=%g, max=%g）",
				ErrInvalidRangeValue, ch.ChannelID, ch.RangeMin, ch.RangeMax)
		}
	}

	return validateBatches(config)
}

// validateBatches 验证批次列表的合法性。
// 单独拆分以保持 validateConfig 的单一职责。
func validateBatches(config BatchConfig) error {
	if len(config.Batches) == 0 {
		return fmt.Errorf("%w: 批次列表不能为空", ErrInvalidBatchConfig)
	}

	seenBatchIDs := make(map[string]bool, len(config.Batches))
	seenBatchIndices := make(map[int]bool, len(config.Batches))
	// channelToBatch 记录通道所属批次，用于检测跨批次重叠
	channelToBatch := make(map[int]string, MaxChannelID)

	for i := range config.Batches {
		b := &config.Batches[i]

		// 4a. BatchID 非空且唯一
		if b.BatchID == "" {
			return fmt.Errorf("%w: 批次 %d 的 batchId 为空", ErrInvalidBatchConfig, i+1)
		}
		if seenBatchIDs[b.BatchID] {
			return fmt.Errorf("%w: 批次 ID 重复: %s", ErrInvalidBatchConfig, b.BatchID)
		}
		seenBatchIDs[b.BatchID] = true

		// 4b. BatchIndex >= 1 且唯一
		if b.BatchIndex < 1 {
			return fmt.Errorf("%w: 批次 %s 的 batchIndex 必须 >= 1", ErrInvalidBatchConfig, b.BatchID)
		}
		if seenBatchIndices[b.BatchIndex] {
			return fmt.Errorf("%w: 批次索引重复: %d", ErrInvalidBatchConfig, b.BatchIndex)
		}
		seenBatchIndices[b.BatchIndex] = true

		// 批次内通道不能为空
		if len(b.Channels) == 0 {
			return fmt.Errorf("%w: 批次 %s 的通道列表为空", ErrInvalidBatchConfig, b.BatchID)
		}

		// 5. 批次内通道唯一性 + 6. 批次间不重叠 + 7. 量程一致性
		seenInBatch := make(map[int]bool, len(b.Channels))
		for _, ch := range b.Channels {
			if ch.ChannelID < MinChannelID || ch.ChannelID > MaxChannelID {
				return fmt.Errorf("%w: 批次 %s 通道 %d 越界",
					ErrInvalidBatchConfig, b.BatchID, ch.ChannelID)
			}
			if seenInBatch[ch.ChannelID] {
				return fmt.Errorf("%w: 批次 %s 内通道 %d 重复",
					ErrInvalidBatchConfig, b.BatchID, ch.ChannelID)
			}
			seenInBatch[ch.ChannelID] = true

			if other, ok := channelToBatch[ch.ChannelID]; ok {
				return fmt.Errorf("%w: 通道 %d 同时属于批次 %s 和 %s",
					ErrInvalidBatchConfig, ch.ChannelID, other, b.BatchID)
			}
			channelToBatch[ch.ChannelID] = b.BatchID

			// 量程一致性：三元组 (RangeMin, RangeMax, RangeUnit) 必须全等
			minMismatch := math.Abs(ch.RangeMin-b.RangeMin) > rangeMatchEpsilon
			maxMismatch := math.Abs(ch.RangeMax-b.RangeMax) > rangeMatchEpsilon
			if minMismatch || maxMismatch || ch.RangeUnit != b.RangeUnit {
				return fmt.Errorf("%w: 批次 %s 内通道 %d 量程不一致（期望 %g~%g %s，实际 %g~%g %s）",
					ErrBatchRangeInconsistent, b.BatchID, ch.ChannelID,
					b.RangeMin, b.RangeMax, b.RangeUnit,
					ch.RangeMin, ch.RangeMax, ch.RangeUnit)
			}
		}
	}

	return nil
}

// generateSessionID 生成会话 ID。
// 格式：batch-session-{unix毫秒}-{单调计数器}
// 进程内唯一；会话仅内存维护，进程重启后会话本身丢失，ID 复用不影响正确性。
func generateSessionID() string {
	return fmt.Sprintf("batch-session-%d-%d", time.Now().UnixMilli(), nextSessionCounter())
}

var (
	sessionCounterMu sync.Mutex
	sessionCounter   int64
)

func nextSessionCounter() int64 {
	sessionCounterMu.Lock()
	defer sessionCounterMu.Unlock()
	sessionCounter++
	return sessionCounter
}

// ---- 深拷贝辅助 ----
// 用于 GetSession 返回副本，避免外部修改内部状态。

func cloneSession(src *Session) *Session {
	if src == nil {
		return nil
	}
	dst := &Session{
		ID:                src.ID,
		Config:            cloneConfig(src.Config),
		CurrentBatchIndex: src.CurrentBatchIndex,
		VerifiedBatchIDs:  make(map[string]bool, len(src.VerifiedBatchIDs)),
	}
	for k, v := range src.VerifiedBatchIDs {
		dst.VerifiedBatchIDs[k] = v
	}
	return dst
}

func cloneConfig(src BatchConfig) BatchConfig {
	dst := BatchConfig{
		ChannelRanges: make([]ChannelRange, len(src.ChannelRanges)),
		Batches:       make([]BatchGroup, len(src.Batches)),
	}
	copy(dst.ChannelRanges, src.ChannelRanges)
	for i := range src.Batches {
		dst.Batches[i] = cloneBatchGroup(src.Batches[i])
	}
	return dst
}

func cloneBatchGroup(src BatchGroup) BatchGroup {
	dst := BatchGroup{
		BatchID:        src.BatchID,
		BatchIndex:     src.BatchIndex,
		RangeMin:       src.RangeMin,
		RangeMax:       src.RangeMax,
		RangeUnit:      src.RangeUnit,
		Status:         src.Status,
		Channels:       make([]ChannelRange, len(src.Channels)),
		PressurePoints: make([]float64, len(src.PressurePoints)),
	}
	copy(dst.Channels, src.Channels)
	copy(dst.PressurePoints, src.PressurePoints)
	if src.CollectedData != nil {
		dst.CollectedData = make(map[int][]float64, len(src.CollectedData))
		for k, v := range src.CollectedData {
			cp := make([]float64, len(v))
			copy(cp, v)
			dst.CollectedData[k] = cp
		}
	}
	return dst
}