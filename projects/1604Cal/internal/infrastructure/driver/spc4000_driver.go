package driver

import (
	"context"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// SPC4000 打压设备驱动 (Mensor 兼容命令集)
//
// SPC4000 是 Scanivalve 公司的压力校准器，底层基于 Mensor CPC6000 平台，
// 默认远程命令集为 Mensor 指令集（非标准 SCPI）。命令规范见
// docs/spc4000_v1_web.pdf 第 4 章「Remote Operations」Table 4.1。
//
// 关键命令语义（与标准 SCPI 打压设备不同，务必注意）：
//   - GP <value> / GN <value>：设定正/负目标压力，同时隐含切入控制模式。
//   - Measure：切入 Measure（陷阱）模式，即停止打压并隔离气路。
//   - ZO：泄压（Vents any pressure）并气路短接，等价于本地 [Vent] 键。
//   - RP：读取当前压力（可带 1~4 量程前缀，如 1RP）。
//   - Units <code>：单位使用数字代码（见 helpers.go 的 SPC4000 映射表）。
//   - RangeMin? / RangeMax?：查询当前传感器的压力量程上下限。
//
// 注意：该命令集没有独立的「控制模式」「稳定标志」命令。
//   - 控制模式只能通过 GP/GN 下发目标压力触发（见 StartControl 说明）。
//   - 稳定判定无硬件标志可查，需软件侧采样判稳（见 ReadStability 说明）。
// ---------------------------------------------------------------------------

type SPC4000Driver struct {
	base *tcpConnectionDriver
}

func newSPC4000DriverWithLocalAddr(host string, port int, localAddr string) *SPC4000Driver {
	return &SPC4000Driver{base: newTCPConnectionDriverWithLocalAddr("SPC4000", host, port, localAddr)}
}

func (d *SPC4000Driver) Connect(ctx context.Context) error    { return d.base.Connect(ctx) }
func (d *SPC4000Driver) Disconnect(ctx context.Context) error { return d.base.Disconnect(ctx) }

func (d *SPC4000Driver) SetTargetPressure(ctx context.Context, target float64) error {
	var cmd string
	if target >= 0 {
		cmd = fmt.Sprintf("GP %.4f", target)
	} else {
		cmd = fmt.Sprintf("GN %.4f", math.Abs(target))
	}
	_, err := d.base.sendSCPICommand(ctx, cmd, 3*time.Second)
	if err != nil {
		return fmt.Errorf("set target pressure: %w", err)
	}
	return nil
}

func (d *SPC4000Driver) Stop(ctx context.Context) error {
	_, err := d.base.sendSCPICommand(ctx, "Measure", 3*time.Second)
	if err != nil {
		return fmt.Errorf("stop pressure: %w", err)
	}
	return nil
}

// Exhaust 泄压并气路短接。
//
// 说明：SPC4000 的 Mensor 命令集没有独立的 "Vent" 远程命令，
// 手册 Table 4.1 中对应本地 [Vent] 键的远程命令是 ZO：
//
//	ZO  "Vents any pressure in the system and pneumatically shorts
//	     the positive and negative side of any transducers together..."
//
// 注意：ZO 会把正/负两侧短接，适合泄压到大气；若仅需停止打压并
// 保持当前压力（陷阱模式），应使用 Stop（Measure），而非本方法。
func (d *SPC4000Driver) Exhaust(ctx context.Context) error {
	_, err := d.base.sendSCPICommand(ctx, "ZO", 3*time.Second)
	if err != nil {
		return fmt.Errorf("exhaust: %w", err)
	}
	return nil
}

func (d *SPC4000Driver) ReadCurrentPressure(ctx context.Context) (float64, error) {
	resp, err := d.base.sendSCPICommand(ctx, "RP", 3*time.Second)
	if err != nil {
		return 0, fmt.Errorf("read current pressure: %w", err)
	}
	return parseSCPIPressure(resp)
}

func (d *SPC4000Driver) ReadUnit(ctx context.Context) (string, error) {
	resp, err := d.base.sendSCPICommand(ctx, "Units?", 3*time.Second)
	if err != nil {
		log.Printf("[SPC4000] ReadUnit error: %v", err)
		return "", fmt.Errorf("read unit: %w", err)
	}
	// 手册 Table 4.1：Units? 返回 "<sp>{string}"，即文本单位字符串（如 "psi"），
	// 而非数字代码，故直接按文本单位规范化。
	unit := NormalizePressureUnit(resp)
	log.Printf("[SPC4000] ReadUnit raw=%q parsed=%q", resp, unit)
	return unit, nil
}

func (d *SPC4000Driver) SetUnit(ctx context.Context, unit string) error {
	code, ok := pressureUnitToCodeSPC4000(unit)
	if !ok {
		return fmt.Errorf("unsupported pressure unit: %s", unit)
	}
	cmd := fmt.Sprintf("Units %s", code)
	log.Printf("[SPC4000] SetUnit %q → cmd=%q", unit, cmd)
	_, err := d.base.sendSCPICommand(ctx, cmd, 3*time.Second)
	if err != nil {
		return fmt.Errorf("set unit: %w", err)
	}
	return nil
}

// ReadStability 判断当前压力是否稳定。
//
// 说明：SPC4000 的 Mensor 命令集没有直接的「是否稳定」查询命令
// （ConST 811A 的 :STABle? 在此不适用）。稳定状态由设备的
// Stabletime / StableWin 参数定义，但无只读标志可查。
// 因此本方法采用软件判稳：连续采样若干次当前压力（RP），
// 若采样区间内压力极差不超过判稳阈值，则认为稳定。
//
// 判稳阈值取固定的小量（默认 0.05% 满量程），结合采样间隔
// 近似设备稳定时间。若设备量程可用（ReadTargetRange），
// 阈值会按满量程自适应。
func (d *SPC4000Driver) ReadStability(ctx context.Context) (bool, error) {
	const (
		sampleCount = 5           // 采样次数
		sampleDelay = 200 * time.Millisecond // 采样间隔
		defaultFS   = 100.0       // 无法获取量程时的默认满量程（单位：当前单位）
		fracThreshold = 0.0005    // 判稳阈值：0.05% 满量程
	)

	// 尽量获取满量程以自适应阈值；失败则退回默认满量程。
	span := defaultFS
	if _, max, err := d.ReadTargetRange(ctx); err == nil && max > 0 {
		span = max
	}

	var first, last float64
	for i := 0; i < sampleCount; i++ {
		p, err := d.ReadCurrentPressure(ctx)
		if err != nil {
			return false, fmt.Errorf("read stability: %w", err)
		}
		if i == 0 {
			first = p
		}
		last = p
		if i < sampleCount-1 {
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case <-time.After(sampleDelay):
			}
		}
	}

	stable := math.Abs(last-first) <= span*fracThreshold
	log.Printf("[SPC4000] ReadStability first=%.6f last=%.6f span=%.6f stable=%v", first, last, span, stable)
	return stable, nil
}

// StartControl 切入控制（打压）模式。
//
// 说明：SPC4000 的 Mensor 命令集没有独立的「仅进入控制模式」命令，
// 手册 Table 4.1 明确 GP/GN "Sends the SPC4000 to control mode and a
// desired pressure"。即控制模式只能通过下发目标压力触发。
//
// 因此本方法实现为：若当前已处于控制模式（Mode? 返回 CONTROL）则直接
// 返回；否则读取当前压力，并用 GP/GN 重发该压力以触发切入控制模式，
// 使设备由 Measure 模式转回控制而不改变目标压力。
func (d *SPC4000Driver) StartControl(ctx context.Context) error {
	mode, err := d.base.sendSCPICommand(ctx, "Mode?", 3*time.Second)
	if err != nil {
		return fmt.Errorf("query mode: %w", err)
	}
	if strings.EqualFold(strings.TrimSpace(mode), "CONTROL") {
		return nil
	}

	// 不在控制模式：读当前压力并以之重发 GP/GN，触发切入控制模式。
	current, err := d.ReadCurrentPressure(ctx)
	if err != nil {
		return fmt.Errorf("read current pressure: %w", err)
	}
	return d.SetTargetPressure(ctx, current)
}

// ReadTargetRange 读取当前激活传感器的压力量程上下限。
//
// 手册 Table 4.1：RangeMin? / RangeMax? 分别返回当前单位下的
// 量程最小/最大值（单值）。返回 (min, max)。
func (d *SPC4000Driver) ReadTargetRange(ctx context.Context) (min, max float64, err error) {
	maxResp, err := d.base.sendSCPICommand(ctx, "RangeMax?", 3*time.Second)
	if err != nil {
		return 0, 0, fmt.Errorf("query range max: %w", err)
	}
	max, err = strconv.ParseFloat(strings.TrimSpace(maxResp), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse range max %q: %w", maxResp, err)
	}

	minResp, err := d.base.sendSCPICommand(ctx, "RangeMin?", 3*time.Second)
	if err != nil {
		return 0, 0, fmt.Errorf("query range min: %w", err)
	}
	min, err = strconv.ParseFloat(strings.TrimSpace(minResp), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse range min %q: %w", minResp, err)
	}
	return min, max, nil
}
