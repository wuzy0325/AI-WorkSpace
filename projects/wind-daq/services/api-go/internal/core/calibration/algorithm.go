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
	// checkAbort：可选中止检查闭包，由 AutomaticCalibration 注入；
	// 返回 true 时算法应立即返回 ErrPointAborted，使主循环回退索引重跑该点。
	// 传 nil 时等同于不检查（向后兼容）。
	// onSampleProgress：可选采样进度回调，每次采样后由算法调用，
	// 传入当前采样序号（从 1 开始）和总采样数，用于 UI 显示"当前点采样 3/10"。
	// 传 nil 时等同于不推送（向后兼容）。
	AcquireDataWithConfig(point CalPoint, channelReader ChannelValueReader, config Config, checkAbort func() bool, onSampleProgress func(current, total int)) (DataPoint, error)

	// ValidateConfig 验证校准配置是否有效
	ValidateConfig(config Config) error
}

// ChannelValueReader 通道数据读取函数类型
// 输入设备ID和通道索引，返回当前通道值
type ChannelValueReader func(deviceID string, channelIndex int) (float64, bool)

// TimestampReader 设备时间戳读取函数类型
// 输入设备ID，返回该设备最新数据帧的时间戳（毫秒）和是否可用。
// 校准算法通过此函数判断设备是否产出新帧，避免重复读取缓存旧数据。
type TimestampReader func(deviceID string) (int64, bool)

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

// ==================== 七孔探针数据点适配 ====================

// 确保 SevenHoleDataPoint 实现 DataPoint 接口
var _ DataPoint = (*SevenHoleDataPoint)(nil)

// GetPointID 返回七孔校准点 ID
func (d *SevenHoleDataPoint) GetPointID() int { return d.PointID }

// GetCoordinates 返回逻辑坐标（业务语义，CSV 落盘用）
// 内区返回 (α,β)，外区返回 (θ,φ)；运动坐标通过 MotionCoordinates 字段单独访问。
func (d *SevenHoleDataPoint) GetCoordinates() map[string]float64 { return d.Coordinates }

// ==================== 通用点位结果适配 ====================

// 确保 PointResult 实现 DataPoint 接口
var _ DataPoint = (*PointResult)(nil)

func (r *PointResult) GetPointID() int                    { return r.PointIndex }
func (r *PointResult) GetCoordinates() map[string]float64 { return nil }
