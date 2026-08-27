package device

import (
	"context"
	"fmt"
	"sort"

	"cal1604/internal/domain"
	apperrors "cal1604/internal/errors"
)

// CheckValveCalibrationGate 校验所有计量设备的阀门均处于校准模式。
// 多设备场景任一设备未达标即整体门禁失败（错误携带 deviceId 便于定位）；
// 按设备 ID 排序遍历，保证多台设备同时不达标时报错可复现。
// 标定与计量两个应用包共用本实现，避免同一门禁逻辑跨包重复维护。
func CheckValveCalibrationGate(ctx context.Context, drivers map[string]MeasureDriver) error {
	devIDs := make([]string, 0, len(drivers))
	for devID := range drivers {
		devIDs = append(devIDs, devID)
	}
	sort.Strings(devIDs)

	for _, devID := range devIDs {
		valveStatus, err := drivers[devID].ReadValveStatus(ctx)
		if err != nil {
			return fmt.Errorf("%w: read valve status on %s: %v", apperrors.ErrPrerequisiteNotMet, devID, err)
		}
		if valveStatus != string(domain.ValveStateCalibration) {
			return fmt.Errorf("%w: valve must be in calibration state on %s, current: %s",
				apperrors.ErrPrerequisiteNotMet, devID, valveStatus)
		}
	}
	return nil
}
