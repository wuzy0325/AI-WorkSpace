package calibration

// Algorithm 校准算法接口，定义所有校准类型的通用行为
// 每种校准类型（五孔、三孔、总压、总温）实现此接口
type Algorithm interface {
	// Type 返回校准类型
	Type() CalibrationType

	// AcquireData 采集单个点位数据
	// point: 当前校准测点
	// channelReader: 通道数据读取函数
	// samplesPerPoint: 每个点位的采样次数
	AcquireData(point CalPoint, channelReader ChannelValueReader, samplesPerPoint int) (DataPoint, error)

	// AcquireDataWithConfig 使用完整校准配置采集单个点位数据。
	// 自动校准流程使用该方法，确保通道映射、采样策略等配置参与采集。
	AcquireDataWithConfig(point CalPoint, channelReader ChannelValueReader, config Config) (DataPoint, error)

	// ValidateConfig 验证校准配置是否有效
	ValidateConfig(config Config) error
}

// ChannelValueReader 通道数据读取函数类型
// 输入设备ID和通道索引，返回当前通道值
type ChannelValueReader func(deviceID string, channelIndex int) (float64, bool)

// DataPointSink 单个数据点采集完成后的回调类型，用于实时持久化（如逐点写 CSV）。
type DataPointSink func(DataPoint)

// DataPoint 通用校准数据点接口
type DataPoint interface {
	// GetPointID 返回点位ID
	GetPointID() int
	// GetCoordinates 返回坐标
	GetCoordinates() map[string]float64
}

// ==================== 五孔探针数据点适配 ====================

// 确保 FiveHoleDataPoint 实现 DataPoint 接口
var _ DataPoint = (*FiveHoleDataPoint)(nil)

func (d *FiveHoleDataPoint) GetPointID() int                    { return d.PointID }
func (d *FiveHoleDataPoint) GetCoordinates() map[string]float64 { return d.Coordinates }

// ==================== 三孔探针数据点适配 ====================

var _ DataPoint = (*ThreeHoleDataPoint)(nil)

func (d *ThreeHoleDataPoint) GetPointID() int                    { return d.PointID }
func (d *ThreeHoleDataPoint) GetCoordinates() map[string]float64 { return d.Coordinates }

// ==================== 总压探针数据点适配 ====================

var _ DataPoint = (*TotalPressureDataPoint)(nil)

func (d *TotalPressureDataPoint) GetPointID() int { return d.PointID }
func (d *TotalPressureDataPoint) GetCoordinates() map[string]float64 {
	return map[string]float64{"α": d.Alpha}
}

// ==================== 总温探针数据点适配 ====================

var _ DataPoint = (*TotalTemperatureDataPoint)(nil)

func (d *TotalTemperatureDataPoint) GetPointID() int { return d.ID }
func (d *TotalTemperatureDataPoint) GetCoordinates() map[string]float64 {
	return map[string]float64{"Ma": d.TargetMachNumber}
}

// ==================== 通用点位结果适配 ====================

// 确保 PointResult 实现 DataPoint 接口
var _ DataPoint = (*PointResult)(nil)

func (r *PointResult) GetPointID() int                    { return r.PointIndex }
func (r *PointResult) GetCoordinates() map[string]float64 { return nil }
