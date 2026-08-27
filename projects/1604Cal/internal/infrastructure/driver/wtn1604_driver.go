package driver

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"cal1604/internal/domain"
)

// ---------------------------------------------------------------------------
// WTN1604 计量设备驱动
// ---------------------------------------------------------------------------

type WTN1604Driver struct {
	base *tcpConnectionDriver
}

func newWTN1604DriverWithLocalAddr(host string, port int, localAddr string) *WTN1604Driver {
	return &WTN1604Driver{base: newTCPConnectionDriverWithLocalAddr("WTN1604", host, port, localAddr)}
}

func (d *WTN1604Driver) Connect(ctx context.Context) error    { return d.base.Connect(ctx) }
func (d *WTN1604Driver) Disconnect(ctx context.Context) error { return d.base.Disconnect(ctx) }

// 阀门状态语义化常量：driver 层 re-export domain 常量，
// 调用方可直接 driver.ValveStateCalibration 使用，避免散落字面量。
const (
	ValveStateCalibration = string(domain.ValveStateCalibration)
	ValveStateMeasurement = string(domain.ValveStateMeasurement)
	ValveStateUnknown     = string(domain.ValveStateUnknown)
)

// ReadValveStatus 读取计量设备阀门状态。
// 返回 ValveStateCalibration / ValveStateMeasurement / ValveStateUnknown 三态之一。
// 设备拒绝（Nxx）会作为错误抛出，避免上层误把错误码当成阀门状态。
func (d *WTN1604Driver) ReadValveStatus(ctx context.Context) (string, error) {
	resp, err := d.base.sendWTN1604Command(ctx, "@01  0", 3*time.Second)
	if err != nil {
		return "", fmt.Errorf("read valve status: %w", err)
	}
	return parseValveReadResponse(resp)
}

// parseValveReadResponse 把硬件读阀响应映射为统一的阀门状态三态。
// 抽离为纯函数便于单测覆盖所有分支（NACK / 数字 0~3 / 文本同义词 / 未识别）。
func parseValveReadResponse(resp string) (string, error) {
	raw := strings.TrimSpace(resp)
	// 设备拒绝命令：以 N 开头并跟数字，直接当错误上抛
	if isWTN1604NACK(raw) {
		return "", fmt.Errorf("device rejected read valve: %s", raw)
	}

	val := strings.TrimSpace(strings.TrimPrefix(raw, "A"))
	if val == "" {
		val = raw
	}

	if num, parseErr := strconv.Atoi(strings.TrimSpace(val)); parseErr == nil {
		switch num {
		case 1:
			return ValveStateCalibration, nil
		case 2, 3:
			// 现场兼容：部分 1604 固件在 RUN/测量态返回 3。
			return ValveStateMeasurement, nil
		case 0:
			// 0 在不同固件下可能表示「测量态」或「未初始化」，
			// 没有现场固件文档前不再武断归类为 measurement，
			// 改为 unknown，让门禁/UI 自行决定如何处理。
			return ValveStateUnknown, nil
		}
	}

	switch strings.ToLower(strings.TrimSpace(val)) {
	case "calibration", "calibrate":
		return ValveStateCalibration, nil
	case "measurement", "measure":
		return ValveStateMeasurement, nil
	default:
		return ValveStateUnknown, nil
	}
}

// SetValveStatus 切换阀门状态。
// 校准 → w0C01；测量 → w0C00。
// 设备拒绝（Nxx）作为「设备拒绝」专属错误返回，便于上层向用户给出可读提示。
func (d *WTN1604Driver) SetValveStatus(ctx context.Context, status string) error {
	cmd, err := valveSetCommandFor(status)
	if err != nil {
		return err
	}
	resp, err := d.base.sendWTN1604Command(ctx, cmd, 3*time.Second)
	if err != nil {
		return fmt.Errorf("set valve status: %w", err)
	}
	return interpretValveSetResponse(cmd, resp)
}

// valveSetCommandFor 把业务态映射到具体协议命令字。
func valveSetCommandFor(status string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "calibration", "calibrate", "1":
		return "w0C01", nil
	case "measurement", "measure", "2":
		return "w0C00", nil
	default:
		return "", fmt.Errorf("invalid valve status: %s", status)
	}
}

// interpretValveSetResponse 把写阀响应分类：成功 / 设备拒绝 / 协议异常。
// 抽为纯函数便于单测覆盖各分支（A / Nxx / 其他）。
func interpretValveSetResponse(cmd, resp string) error {
	trimmed := strings.TrimSpace(resp)
	if isWTN1604NACK(trimmed) {
		// 现场常见：固件不支持该阀位（如 N09 拒绝校准），
		// 必须明确告知用户「设备拒绝」，否则前端只看到"按下去没反应"。
		return fmt.Errorf("device rejected valve command %s: %s", cmd, trimmed)
	}
	if trimmed != "A" {
		return fmt.Errorf("set valve status failed: response %q", trimmed)
	}
	return nil
}

// isWTN1604NACK 判定响应是否为设备拒绝错误码（Nxx）。
// 协议中 N 开头后跟两位数字（如 N09、N03）表示固件拒绝。
func isWTN1604NACK(resp string) bool {
	if len(resp) < 2 || resp[0] != 'N' {
		return false
	}
	for i := 1; i < len(resp); i++ {
		if resp[i] < '0' || resp[i] > '9' {
			return false
		}
	}
	return true
}

func (d *WTN1604Driver) ReadUnit(ctx context.Context) (string, error) {
	resp, err := d.base.sendWTN1604Command(ctx, "u01101", 3*time.Second)
	if err != nil {
		return "", fmt.Errorf("read unit: %w", err)
	}
	unit, ok := parseWTN1604Unit(resp)
	if !ok {
		return coefficientToUnit(strings.TrimSpace(resp)), nil
	}
	return unit, nil
}

func (d *WTN1604Driver) SetUnit(ctx context.Context, unit string) error {
	coef, ok := unitToCoefficient(unit)
	if !ok {
		return fmt.Errorf("unsupported unit: %s", unit)
	}
	cmd := fmt.Sprintf("v01101 %s", coef)
	resp, err := d.base.sendWTN1604Command(ctx, cmd, 3*time.Second)
	if err != nil {
		return fmt.Errorf("set unit: %w", err)
	}
	if resp != "A" {
		return fmt.Errorf("set unit failed: response %q", resp)
	}
	return nil
}

func (d *WTN1604Driver) CollectData(ctx context.Context, channels []int) ([]float64, error) {
	bitmap := channelsToBitmap(channels)
	cmd := fmt.Sprintf("r%s0", bitmap)
	resp, err := d.base.sendWTN1604Command(ctx, cmd, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("collect data: %w", err)
	}
	if strings.HasPrefix(resp, "N") {
		return nil, fmt.Errorf("device error: %s", resp)
	}
	parts := strings.Fields(resp)
	values := make([]float64, 0, len(parts))
	for _, p := range parts {
		v, err := strconv.ParseFloat(p, 64)
		if err != nil {
			continue
		}
		values = append(values, v)
	}
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
	return values, nil
}

// CalibrateZero 执行调零校准并返回各通道结果。
func (d *WTN1604Driver) CalibrateZero(ctx context.Context, channels []int) ([]float64, error) {
	bitmap := channelsToBitmap(channels)
	cmd := fmt.Sprintf("C 04 %s", bitmap)
	resp, err := d.base.sendWTN1604Command(ctx, cmd, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("calibrate zero: %w", err)
	}

	values, err := parseCalibrationValues(resp)
	if err != nil {
		return nil, fmt.Errorf("calibrate zero: %w", err)
	}

	return values, nil
}

// CalibrateFullScale 执行满量程校准并返回各通道结果。
func (d *WTN1604Driver) CalibrateFullScale(ctx context.Context, channels []int, fullScaleValue float64) ([]float64, error) {
	bitmap := channelsToBitmap(channels)
	cmd := fmt.Sprintf("C 05 %s %.6f", bitmap, fullScaleValue)
	resp, err := d.base.sendWTN1604Command(ctx, cmd, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("calibrate full scale: %w", err)
	}

	values, err := parseCalibrationValues(resp)
	if err != nil {
		return nil, fmt.Errorf("calibrate full scale: %w", err)
	}

	return values, nil
}

func (d *WTN1604Driver) ReadDeviceInfo(ctx context.Context) (map[string]string, error) {
	info := make(map[string]string)
	resp, err := d.base.sendWTN1604Command(ctx, "A", 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("device communication test: %w", err)
	}
	info["commTest"] = resp
	if resp, err = d.base.sendWTN1604Command(ctx, "q00", 3*time.Second); err == nil {
		info["model"] = resp
	}
	if resp, err = d.base.sendWTN1604Command(ctx, "q01", 3*time.Second); err == nil {
		info["version"] = resp
	}
	return info, nil
}

func (d *WTN1604Driver) Reset(ctx context.Context) error {
	resp, err := d.base.sendWTN1604Command(ctx, "B", 3*time.Second)
	if err != nil {
		return fmt.Errorf("reset: %w", err)
	}
	if resp != "A" {
		return fmt.Errorf("reset failed: response %q", resp)
	}
	return nil
}

func (d *WTN1604Driver) StartCalibration(ctx context.Context, channels []int, pressurePoints int, avgPoints int) error {
	bitmap := channelsToBitmap(channels)
	cmd := fmt.Sprintf("C 00 %s %d 1 %d", bitmap, pressurePoints, avgPoints)
	resp, err := d.base.sendWTN1604Command(ctx, cmd, 5*time.Second)
	if err != nil {
		return fmt.Errorf("start calibration: %w", err)
	}
	if resp != "A" {
		return fmt.Errorf("start calibration failed: response %q", resp)
	}
	return nil
}

func (d *WTN1604Driver) CollectCalibrationPoint(ctx context.Context, pointIndex int, targetPressure float64) ([]float64, error) {
	cmd := fmt.Sprintf("C 01 %d %.2f", pointIndex, targetPressure)
	resp, err := d.base.sendWTN1604Command(ctx, cmd, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("collect calibration point: %w", err)
	}
	if strings.HasPrefix(resp, "N") {
		return nil, fmt.Errorf("device error: %s", resp)
	}
	parts := strings.Fields(resp)
	values := make([]float64, 0, len(parts))
	for _, p := range parts {
		v, err := strconv.ParseFloat(p, 64)
		if err != nil {
			continue
		}
		values = append(values, v)
	}
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
	return values, nil
}

func (d *WTN1604Driver) PerformFitting(ctx context.Context) error {
	resp, err := d.base.sendWTN1604Command(ctx, "C 02", 10*time.Second)
	if err != nil {
		return fmt.Errorf("perform fitting: %w", err)
	}
	if resp != "A" {
		return fmt.Errorf("perform fitting failed: response %q", resp)
	}
	return nil
}

func (d *WTN1604Driver) EndCalibration(ctx context.Context) error {
	resp, err := d.base.sendWTN1604Command(ctx, "C 03", 5*time.Second)
	if err != nil {
		return fmt.Errorf("end calibration: %w", err)
	}
	if resp != "A" {
		return fmt.Errorf("end calibration failed: response %q", resp)
	}
	return nil
}

func (d *WTN1604Driver) SaveCoefficients(ctx context.Context) error {
	resp, err := d.base.sendWTN1604Command(ctx, "w08", 3*time.Second)
	if err != nil {
		return fmt.Errorf("save zero coefficient: %w", err)
	}
	if resp != "A" {
		return fmt.Errorf("save zero coefficient failed: response %q", resp)
	}
	resp, err = d.base.sendWTN1604Command(ctx, "w09", 3*time.Second)
	if err != nil {
		return fmt.Errorf("save gain coefficient: %w", err)
	}
	if resp != "A" {
		return fmt.Errorf("save gain coefficient failed: response %q", resp)
	}
	return nil
}

func parseCalibrationValues(resp string) ([]float64, error) {
	trimmed := strings.TrimSpace(resp)
	if trimmed == "A" {
		return []float64{}, nil
	}
	if strings.HasPrefix(trimmed, "N") {
		return nil, fmt.Errorf("device error: %s", trimmed)
	}

	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return nil, fmt.Errorf("unexpected calibration response: %q", resp)
	}

	values := make([]float64, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid calibration value %q: %w", part, err)
		}
		values = append(values, value)
	}

	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}

	return values, nil
}
