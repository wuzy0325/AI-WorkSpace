package usecase

// This file centralizes acquisition state and readable device identity checks
// for every device referenced by a traversal configuration.
//
// spec-traversal-acquisition-stop：遍历只关心三种采集态——
//   - Acquiring：正在产帧，正常采样
//   - Stopped：操作员主动停采（可恢复，无限期等待重启采集）
//   - ReconnectRequired：掉线/未连接/已移除（需重连并启动采集）
//
// 异常设备（非 Acquiring）统一进入"类暂停"无限期等待，本文件只负责分类与列表，
// 等待行为在 waitForAcquisitionResume（traversal_acquisition.go）。

import (
	"sort"

	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
)

// acquisitionDeviceState 单个引用设备的采集态。
type acquisitionDeviceState struct {
	id    string
	name  string
	state ports.AcquisitionState
}

// referencedDeviceIDs 提取配置引用的去重设备 ID（字典序稳定）。
func referencedDeviceIDs(config traversal.Config) []string {
	ids := make(map[string]struct{})
	for _, ref := range config.ResolvedChannelRefs() {
		if ref.DeviceID != "" {
			ids[ref.DeviceID] = struct{}{}
		}
	}
	if len(ids) == 0 && config.DeviceID != "" {
		ids[config.DeviceID] = struct{}{}
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	return ordered
}

// referencedAcquisitionDeviceStates 返回全部引用设备的三态列表（含 Acquiring），
// 按 deviceID 字典序稳定排序。controller 为 nil 时返回 nil。
func referencedAcquisitionDeviceStates(controller ports.AcquisitionController, config traversal.Config) []acquisitionDeviceState {
	if controller == nil {
		return nil
	}
	ids := referencedDeviceIDs(config)
	states := make([]acquisitionDeviceState, 0, len(ids))
	for _, id := range ids {
		status := controller.AcquisitionStatus(id)
		states = append(states, acquisitionDeviceState{
			id:    id,
			name:  status.Name,
			state: status.State,
		})
	}
	return states
}

// abnormalAcquisitionDevices 返回引用设备中处于非 Acquiring 状态的设备列表
// （仅 Stopped / ReconnectRequired），按 deviceID 字典序稳定排序；
// 全部设备 Acquiring 时返回空列表。controller 为 nil 时返回 nil。
// "等 N 台设备"的 N = len(返回列表)。
func abnormalAcquisitionDevices(controller ports.AcquisitionController, config traversal.Config) []acquisitionDeviceState {
	return abnormalAcquisitionDevicesForIDs(controller, referencedDeviceIDs(config))
}

// abnormalAcquisitionDevicesForIDs 同 abnormalAcquisitionDevices，但直接接受设备 ID 列表
// （供采样循环按 groups 判定，语义与按 config 判定一致）。
func abnormalAcquisitionDevicesForIDs(controller ports.AcquisitionController, ids []string) []acquisitionDeviceState {
	if controller == nil {
		return nil
	}
	states := make([]acquisitionDeviceState, 0, len(ids))
	for _, id := range ids {
		status := controller.AcquisitionStatus(id)
		if status.State == ports.AcquisitionAcquiring {
			continue
		}
		states = append(states, acquisitionDeviceState{
			id:    id,
			name:  status.Name,
			state: status.State,
		})
	}
	return states
}

// firstAbnormalAcquisitionDevice 返回主展示异常设备（供错误/等待文案），
// 优先级 ReconnectRequired > Stopped；全部 Acquiring 时返回 (zero, false)。
func firstAbnormalAcquisitionDevice(controller ports.AcquisitionController, config traversal.Config) (acquisitionDeviceState, bool) {
	states := abnormalAcquisitionDevices(controller, config)
	var worst *acquisitionDeviceState
	for i := range states {
		s := &states[i]
		if worst == nil {
			worst = s
			continue
		}
		if s.state == ports.AcquisitionReconnectRequired && worst.state != ports.AcquisitionReconnectRequired {
			worst = s
		}
	}
	if worst == nil {
		return acquisitionDeviceState{}, false
	}
	return *worst, true
}
