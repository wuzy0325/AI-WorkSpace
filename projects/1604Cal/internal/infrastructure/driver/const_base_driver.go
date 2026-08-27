package driver

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// ConST 系列公共基类（提取 811A/820/860 重复逻辑）
// ---------------------------------------------------------------------------

// constBaseDriver 封装 ConST 系列打压设备的公共行为。
// 各子型号仅通过 SCPI 命令差异（stableCmd / pressureCmd / unitCmd 等）来区分。
type constBaseDriver struct {
	base *tcpConnectionDriver
}

// constConnect 公共连接逻辑：先建立 TCP，再轮询稳定状态直到设备就绪。
// 若 10 次判稳查询全部失败，说明设备虽然 TCP 连通但 SCPI 无响应，返回错误。
func (d *constBaseDriver) constConnect(ctx context.Context, stableCmd string) error {
	if err := d.base.Connect(ctx); err != nil {
		return err
	}
	stableOk := false
	for i := 0; i < 10; i++ {
		resp, err := d.base.sendSCPICommand(ctx, stableCmd, 2*time.Second)
		if err == nil && (resp == "0" || resp == "1") {
			stableOk = true
			break
		}
		if err != nil {
			log.Printf("[constConnect] %s attempt %d: %v", d.base.model, i+1, err)
		}
	}
	if !stableOk {
		_ = d.base.Disconnect(ctx)
		return fmt.Errorf("%s: TCP connected but SCPI %q not responding after 10 attempts (check cable / IP / firewall)", d.base.model, stableCmd)
	}
	return nil
}

// Disconnect 断开 TCP 连接（实现 ConnectionDriver 接口）。
func (d *constBaseDriver) Disconnect(ctx context.Context) error {
	return d.base.Disconnect(ctx)
}

// constReadStability 公共读取稳定状态逻辑。
func (d *constBaseDriver) constReadStability(ctx context.Context, stableCmd string) (bool, error) {
	resp, err := d.base.sendSCPICommand(ctx, stableCmd, 3*time.Second)
	if err != nil {
		return false, fmt.Errorf("read stability: %w", err)
	}
	return strings.TrimSpace(resp) == "1", nil
}

// ReadTargetRange 读取目标压力可设范围。
func (d *constBaseDriver) ReadTargetRange(ctx context.Context) (min, max float64, err error) {
	resp, err := d.base.sendSCPICommand(ctx, "PRESsure:TARGet:RANGe?", 3*time.Second)
	if err != nil {
		return 0, 0, fmt.Errorf("read target range: %w", err)
	}
	return parseTargetRange(resp)
}
