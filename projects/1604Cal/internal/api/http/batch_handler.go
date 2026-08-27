package http

import (
	"fmt"
	"net/http"
	"strings"

	"cal1604/internal/application/batch"
	apperrors "cal1604/internal/errors"
	"cal1604/internal/report"
)

// batchCreateSessionRequest 创建分批计量会话请求。
type batchCreateSessionRequest struct {
	ChannelRanges []batch.ChannelRange `json:"channelRanges"`
	Batches       []batch.BatchGroup   `json:"batches"`
}

// batchVerifyRequest 核对码校验请求。
type batchVerifyRequest struct {
	VerificationCode string `json:"verificationCode"`
}

// batchSessionResponse 创建会话响应。
type batchSessionResponse struct {
	SessionID string `json:"sessionId"`
}

// batchCreateSessionHandler 创建分批计量会话。
//
// 前端完成 16 通道量程录入和自动分组后，提交配置到此端点。
// 后端验证配置合法性（通道数量、量程一致性），返回会话 ID。
// 后续操作（核对码校验、批次启动、状态查询）通过会话 ID 引用。
func (s *apiServer) batchCreateSessionHandler(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[batchCreateSessionRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}

	config := batch.BatchConfig{
		ChannelRanges: req.ChannelRanges,
		Batches:       req.Batches,
	}
	sessionID, err := s.batchService.CreateSession(config)
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusCreated, batchSessionResponse{SessionID: sessionID})
}

// batchVerifyHandler 核对码校验。
//
// 操作员在物理切换标准器后输入核对码，前端提交到此端点。
// 后端将输入字符串解析为数值，与批次量程值比对（数值匹配，10 == 10.0）。
// 校验通过后批次标记为已验证，后续 start 时不再需要重复验证。
func (s *apiServer) batchVerifyHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}
	batchID := r.PathValue("batchId")
	if batchID == "" {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	req, err := decodeJSON[batchVerifyRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}

	// 前置校验：核对码非空，避免落到 strconv.ParseFloat 才报错
	if strings.TrimSpace(req.VerificationCode) == "" {
		writeError(w, fmt.Errorf("%w: verificationCode 不能为空", apperrors.ErrInvalidArgument))
		return
	}

	result, err := s.batchService.VerifyBatch(sessionID, batchID, req.VerificationCode)
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, result)
}

// batchStartHandler 启动指定批次。
//
// 前置条件：批次必须已通过核对码校验。
// 本方法只更新批次状态为 running，实际加压执行复用现有 calibration 流程。
func (s *apiServer) batchStartHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}
	batchID := r.PathValue("batchId")
	if batchID == "" {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	if err := s.batchService.StartBatch(sessionID, batchID); err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]string{"status": "running"})
}

// batchCompleteHandler 标记批次完成。
//
// 前端在加压序列完成后调用，更新批次状态为 completed。
func (s *apiServer) batchCompleteHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}
	batchID := r.PathValue("batchId")
	if batchID == "" {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	if err := s.batchService.CompleteBatch(sessionID, batchID); err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]string{"status": "completed"})
}

// batchResetHandler 回退重跑批次。
//
// 将已完成批次重置为 pending，清空采集数据。
// 重置后需要重新通过核对码校验才能 start。
func (s *apiServer) batchResetHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}
	batchID := r.PathValue("batchId")
	if batchID == "" {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	if err := s.batchService.ResetBatch(sessionID, batchID); err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]string{"status": "reset"})
}

// batchGetSessionHandler 查询分批计量会话状态。
//
// 返回当前会话的所有批次信息、当前执行索引、已验证批次集合。
func (s *apiServer) batchGetSessionHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	session, err := s.batchService.GetSession(sessionID)
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, session)
}

// batchDeleteSessionHandler 删除分批计量会话，释放后端内存。
//
// 前端"重新开始"或退出分批模式时调用。删除不存在的会话也返回 200（幂等），
// 便于前端无需关心会话是否已存在。
func (s *apiServer) batchDeleteSessionHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		writeError(w, apperrors.ErrInvalidArgument)
		return
	}

	s.batchService.DeleteSession(sessionID)
	writeSuccess(w, http.StatusOK, map[string]bool{"deleted": true})
}

// batchReportRequest 合并报告请求。
type batchReportRequest struct {
	Batches []batch.BatchGroup `json:"batches"`
	Points  int                `json:"points"`  // 加压点数（用于模板选择）
	Mode    string             `json:"mode"`    // 压力模式：single / roundTrip
}

// validReportModes 报告模式白名单。
// 一期复用现有标定报告模板，仅支持 single / roundTrip 两种。
var validReportModes = map[string]bool{
	"single":    true,
	"roundTrip": true,
}

// batchReportHandler 生成合并报告。
//
// 所有批次完成后，前端提交所有批次数据到此端点。
// 一期仅返回模板选择信息 + 回显批次数据，不生成实际报告文件。
// 后续迭代中可在此处增加分批次段落的合并逻辑。
func (s *apiServer) batchReportHandler(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[batchReportRequest](r)
	if err != nil {
		writeError(w, err)
		return
	}

	// 前置校验：批次列表非空、点数合法、模式合法
	if err := validateBatchReportRequest(req); err != nil {
		writeError(w, fmt.Errorf("%w: %v", apperrors.ErrInvalidArgument, err))
		return
	}

	// 一期直接复用现有报告模板，返回模板选择信息
	// 后续迭代中可在此处增加分批次段落的合并逻辑
	filename, err := report.SelectTemplate(req.Points, req.Mode)
	if err != nil {
		// 保留原始诊断信息，便于前端分辨是点数越界还是模式非法
		writeError(w, fmt.Errorf("%w: %v", apperrors.ErrInvalidArgument, err))
		return
	}

	writeSuccess(w, http.StatusOK, map[string]any{
		"reportTemplate": reportTemplateSelection{Filename: filename},
		"batches":        req.Batches,
	})
}

// validateBatchReportRequest 校验合并报告请求的合法性。
func validateBatchReportRequest(req batchReportRequest) error {
	if len(req.Batches) == 0 {
		return fmt.Errorf("批次列表不能为空")
	}
	if req.Points <= 0 {
		return fmt.Errorf("加压点数必须 > 0，实际 %d", req.Points)
	}
	mode := strings.TrimSpace(req.Mode)
	if !validReportModes[mode] {
		return fmt.Errorf("压力模式非法: %s（仅支持 single / roundTrip）", req.Mode)
	}
	return nil
}
