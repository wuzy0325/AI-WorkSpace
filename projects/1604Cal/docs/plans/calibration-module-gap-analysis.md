# 校准/计量模块差距分析报告

> 参考模块: `1605MeassureApp` (Electron + Vue 3)
> 当前工程: `Cal1604` (Go + Wails + Vue 3)
> 生成日期: 2026-04-22

---

## 一、总览对比

| 维度 | 参考模块 (1605MeassureApp) | 当前工程 (Cal1604) | 差距等级 |
|------|---------------------------|-------------------|----------|
| 采集流程 | 完整 auto/manual 双模式 + 多点循环 | 基本框架已有，manual 模式薄弱 | 中 |
| 稳定性监控 | StabilityMonitor (事件驱动 + 进度推送) | StabilityAccumulator (纯计算，无事件) | 高 |
| 报警系统 | 可配置阈值 + 多通道 + 声音 + 确认弹窗 | 硬编码 5% 阈值，无通道选择 | 高 |
| 报告生成 | ExcelJS 完整报告 + 模板检测 + 通道块填充 | 仅模板文件名选择，无实际生成 | 极高 |
| 压力点生成 | 单程/回程 + 自定义点 + 前端可编辑 | 仅单程等间距生成 | 高 |
| 会话记录 | CollectionSession 完整记录 (ID/时间/配置/设备/状态) | 无持久化会话记录 | 中 |
| 多采样平均 | averageCount 可配置，逐设备逐次采集后取均值 | 未实现多采样平均 | 高 |
| 手动模式 | 手动打压 + 手动采集 + 稳定后触发 | controlMode 字段存在但 manual 流程未完善 | 中 |
| 设备模拟器 | MeasureSimulator + PressureSimulator | 无模拟器 | 低 |
| 配置持久化 | ConfigService 持久化校准参数/报警/最近设备 | 基础 appConfig，校准参数不持久化 | 中 |

---

## 二、逐项详细差距与代码改善指导

### 差距 #1：报告生成 (极高级)

**参考实现** (`src/main/services/ReportService.ts`):
- 基于 ExcelJS 读取 `.xlsx` 模板文件
- 自动检测模板中的通道块 (`findChannelBlockStarts` -- 搜索"通道"/"Channel"标记行)
- 按通道块填充标准压力值、正向测量值、回程测量值
- 计算示值误差、不确定度公式
- 单位解析优先级: 打压设备实时单位 > 缓存单位 > 采集数据单位 > 默认 kPa
- 无模板时动态生成 (`createTemplateLikeWorkbook`)
- 原始数据追溯页 (`fillRawDataWorksheet`)
- 进度推送 (preparing -> reading_template -> filling_data -> saving -> completed)
- 导出对话框: 路径选择 + 模板信息 + 点数/模式确认

**当前状态**:
- `internal/report/report_service.go`: 仅 `ResolveTemplatePath(points, mode)` -- 拼接模板文件路径
- `internal/report/template_selector.go`: 模板文件名选择逻辑 (`5s.xlsx`, `5m.xlsx`)
- **无实际 Excel 读写、无数据填充、无进度推送、无导出 UI**

**改善计划**:

```
步骤1: 后端 -- Excel 报告生成引擎
文件: internal/report/excel_generator.go (新建)

需要引入 Go 的 Excel 库 (推荐 excelize v2):
  go get github.com/xuri/excelize/v2

核心功能:
  - LoadTemplate(templatePath string) (*excelize.File, error)
  - FindChannelBlocks(f *excelize.File) ([]ChannelBlock, error)
    // 搜索 A 列中包含 "通道" 或 "Channel" 的行号
  - FillStandardValues(f *excelize.File, blocks []ChannelBlock, points []PressurePoint)
  - FillMeasureData(f *excelize.File, blocks []ChannelBlock, data map[int][]float64)
  - FillRoundTripData(f *excelize.File, blocks []ChannelBlock, forwardData, backwardData)
  - ResolveReportUnit(session *CalibrationSession) string
  - SaveWithProgress(ctx context.Context, f *excelize.File, path string, onProgress func(stage string, pct int)) error

步骤2: 后端 -- 报告 HTTP Handler
文件: internal/api/http/report_handler.go (新建)

路由:
  POST /api/v1/report/export
  Body: { sessionId, outputPath, template }
  Response: SSE 进度流或同步返回

  GET /api/v1/report/templates
  Response: 可用模板列表

步骤3: 后端 -- 报告服务增强
文件: internal/report/report_service.go (修改)

新增方法:
  - ExportReport(ctx, session, outputPath) error
  - GetTemplates() []ReportTemplate
  - CreateFallbackWorkbook(session) (*excelize.File, error)

步骤4: 前端 -- 导出对话框
文件: web/src/components/calibration/ReportExportDialog.vue (新建)

包含:
  - 路径选择 (Wails 下使用 runtime.SaveFileDialog)
  - 模板信息展示
  - 导出进度条
  - 完成确认
```

**参考代码映射**:

| 参考模块功能 | 目标文件 | 说明 |
|-------------|---------|------|
| `ReportService.loadTemplateWorkbook()` | `excel_generator.go` | 模板加载 + 回退逻辑 |
| `ReportService.findChannelBlockStarts()` | `excel_generator.go` | 通道块检测 |
| `ReportService.fillTemplateWorksheetData()` | `excel_generator.go` | 数据填充 |
| `ReportService.createTemplateLikeWorkbook()` | `excel_generator.go` | 动态模板生成 |
| `ReportService.resolveReportUnit()` | `report_service.go` | 单位解析 |
| `CalibrationView.vue` 导出对话框部分 | `ReportExportDialog.vue` | 导出 UI |

---

### 差距 #2：回程压力点生成 (高级)

**参考实现** (`CollectionService.ts:286-337`):
```typescript
// 单程
for (let i = 0; i < pointCount; i++) {
  points.push({ id: `point-up-${i}`, value: minPressure + step * i, ... });
}
// 回程：追加下降段（不含零点重复）
if (pressureMode === "roundTrip") {
  for (let i = pointCount - 1; i > 0; i--) {
    points.push({ id: `point-down-${i}`, value: minPressure + step * i, ... });
  }
}
```
- 前端表格显示"正"/"回"标记
- 报告中回程值写入第 3 列

**当前状态**:
- `CalibrationConfig` 无 `PressureMode` 字段
- `GeneratePoints()` 仅生成单程等间距点

**改善计划**:

```
步骤1: 后端 -- 配置扩展
文件: internal/application/calibration/service.go

CalibrationConfig 新增:
  PressureMode string `json:"pressureMode"` // "single" | "roundTrip"

修改 GeneratePoints (或新建方法):
  - 单程: points = [min, min+step, ..., max]
  - 回程: points = [min, ..., max, max-step, ..., min+step] (不含首尾重复)

步骤2: 前端 -- Store 扩展
文件: web/src/stores/calibration/index.ts

config 新增 pressureMode 字段
generatePressurePoints 方法适配回程生成

步骤3: 前端 -- UI 扩展
文件: web/src/views/CalibrationView.vue 或相关组件

添加"单程/回程"切换按钮组
压力点表格增加"正"/"回"标记
回程分割线样式
```

---

### 差距 #3：报警系统增强 (高级)

**参考实现** (`CollectionService.ts:618-703` + `CalibrationView.vue:77-460`):
- `AlarmConfig`: `{ enabled, precisionThreshold, soundEnabled, confirmOnAlarm, enabledChannels[] }`
- 满量程判定: `threshold = |maxPressure| * precisionThreshold`
- 多通道独立检测: 只检查 `enabledChannels` 中的通道
- 超差时推送详细事件 (通道明细: channel, value, deviation, unit)
- 声音报警: `process.stdout.write("\x07")`
- 前端弹窗: 超差明细 + "继续/重采" 选择
- 通道选择对话框: 16 通道可独立勾选
- 配置持久化 + 自动保存 (250ms debounce)

**当前状态**:
- `internal/workflow/alarm_service.go`: 仅 `Evaluate(target, actual, levelPercent)` + `ValidateDecision(decision)`
- `calibration/service.go checkAlarm()`: 硬编码 5% 阈值，只看第一个通道
- 前端无报警配置 UI、无通道选择、无报警弹窗

**改善计划**:

```
步骤1: 后端 -- 报警配置结构
文件: internal/domain/alarm.go (新建)

type AlarmConfig struct {
    Enabled           bool    `json:"enabled"`
    PrecisionThreshold float64 `json:"precisionThreshold"` // 满量程百分比
    SoundEnabled      bool    `json:"soundEnabled"`
    ConfirmOnAlarm    bool    `json:"confirmOnAlarm"`
    EnabledChannels   []int   `json:"enabledChannels"`
}

步骤2: 后端 -- 多通道报警检查
文件: internal/workflow/alarm_service.go (修改)

新增方法:
  EvaluateMultiChannel(config AlarmConfig, target float64, maxPressure float64, channelData map[int]float64) *AlarmResult
  返回: overLimitChannels, maxDeviation, triggered

修改 calibration/service.go checkAlarm():
  使用 AlarmConfig 替代硬编码阈值
  检查所有已启用通道
  推送详细报警事件

步骤3: 后端 -- 报警配置 API
文件: internal/api/http/router.go

新增路由:
  GET  /api/v1/alarm/config
  POST /api/v1/alarm/config

步骤4: 前端 -- 报警配置 UI
文件: web/src/components/calibration/AlarmConfigPanel.vue (新建)

包含:
  - 启用/禁用开关
  - 精度阈值选择 (预设 0.01%~0.20% + 自定义)
  - 声音开关
  - 报警确认开关
  - 通道选择对话框 (4x4 网格, 全选/全不选)

步骤5: 前端 -- 报警弹窗
文件: web/src/components/calibration/AlarmConfirmDialog.vue (新建)

包含:
  - 目标压力 / 允许偏差 / 最大偏差
  - 超差通道明细表格
  - "继续下一步" / "重新采集当前点" 按钮
```

---

### 差距 #4：多采样平均 (高级)

**参考实现** (`CollectionService.ts:593-616`):
```typescript
for (let i = 0; i < this.m_config!.averageCount; i++) {
  const data = await measureDevice.collectData();
  samples.push(...data);
  if (i < this.m_config!.averageCount - 1) {
    await this.delay(100); // 采样间隔 100ms
  }
}
```
采集 averageCount 次后，报告服务中计算均值 (`getPointChannelAverageValue`):
```typescript
const average = sum / channelValues.length;
return parseFloat(average.toFixed(6));
```

**当前状态**:
- `CalibrationConfig.AverageCount` 字段已存在
- `calibration/service.go` 的 `Collect()` 方法只采集一次
- 报告填充未实现均值计算

**改善计划**:

```
步骤1: 后端 -- 多采样采集
文件: internal/application/calibration/service.go

修改 Collect() 方法:
  1. 循环 averageCount 次调用 session.ReadMeasureData(ctx)
  2. 每次间隔 100ms (time.Sleep 或 context timer)
  3. 按通道累加值，最后除以次数得到均值
  4. 或保存原始多采样数据，在报告生成时计算均值

步骤2: 数据结构适配
  PressurePoint.CollectedData 从 []float64 改为 [][]float64 (采样次 x 通道数)
  或新增 CollectedAverages []float64 字段存储均值
```

---

### 差距 #5：稳定性监控增强 (高级)

**参考实现** (`StabilityMonitor.ts`):
- `StabilityMonitor` 类: EventEmitter 子类
- 配置: `{ threshold, checkIntervalMs }`
- 事件: `stabilityChanged`, `stabilityAchieved`, `stabilityLost`, `progressUpdate`
- `StabilityStatus`: `{ isStable, isInRange, currentValue, targetValue, deviation, stableDuration, requiredDuration, progress }`
- SCPI 设备通过 `PRESsure:STABle?` 硬件查询
- 非 SCPI 设备通过软件侧采样判断

**当前状态**:
- `StabilityAccumulator`: 纯计算结构，`AddSample()` 返回 `(stable bool, accumulated time.Duration)`
- 无事件推送、无进度百分比、无稳定丢失事件
- 校准服务未在自动采集中集成稳定等待

**改善计划**:

```
步骤1: 后端 -- 稳定性监控器增强
文件: internal/workflow/stability_service.go (修改)

新增:
  type StabilityMonitor struct {
      accumulator   *StabilityAccumulator
      publish       StatusPublisher
      lastInRange   bool
      lastProgress  float64
  }

  func NewStabilityMonitor(tolerance float64, required time.Duration, publish StatusPublisher) *StabilityMonitor
  func (m *StabilityMonitor) FeedSample(ctx context.Context, target, actual float64) StabilityStatus
  // 内部判断并推送事件:
  //   - calibration.stability.changed (进入/离开稳定范围)
  //   - calibration.stability.progress (进度百分比)
  //   - calibration.stability.achieved (达到要求时长)
  //   - calibration.stability.lost (稳定中断)

  type StabilityStatus struct {
      IsStable        bool    `json:"isStable"`
      IsInRange       bool    `json:"isInRange"`
      CurrentValue    float64 `json:"currentValue"`
      TargetValue     float64 `json:"targetValue"`
      Deviation       float64 `json:"deviation"`
      StableDurationMs int64  `json:"stableDurationMs"`
      RequiredDurationMs int64 `json:"requiredDurationMs"`
      Progress        float64 `json:"progress"` // 0-100
  }

步骤2: 后端 -- 集成到自动采集循环
文件: internal/application/calibration/service.go

在 collectPoint() 的 Pressurize 后增加稳定等待:
  1. 创建 StabilityMonitor(tolerance, requiredDuration, publish)
  2. 循环读取压力 + FeedSample 直到 stable
  3. 通过 SSE 推送稳定进度

步骤3: 前端 -- 稳定进度展示
文件: web/src/views/CalibrationView.vue 或相关组件

监听 calibration.stability.* SSE 事件
显示: 稳定状态徽章 + 累计时间 + 进度条
```

---

### 差距 #6：手动采集模式 (中级)

**参考实现**:
- `manualPressurize(targetPressure)`: 手动设置目标压力
- `manualCollect()`: 手动触发一次数据采集
- 前端: "采集"按钮仅在 `controlMode=manual && state=stabilizing && pressureStable` 时启用
- 侧边栏显示打压设备控制面板 (`PressureControlCard`)

**当前状态**:
- `CalibrationConfig.ControlMode` 已有 `"auto"/"manual"` 字段
- 后端 `StartCalibration()` 对 manual 模式仅跳过 `RunAutoCollection()`
- 无 `ManualPressurize()` 和 `ManualCollect()` API
- 前端无手动模式专用控件

**改善计划**:

```
步骤1: 后端 -- 手动模式 API
文件: internal/api/http/calibration_handler.go (修改)

新增路由:
  POST /api/v1/calibration/manual-pressurize
  Body: { targetPressure: number }

  POST /api/v1/calibration/manual-collect
  (无 Body，触发一次采集)

步骤2: 后端 -- 服务方法
文件: internal/application/calibration/service.go (修改)

新增:
  func (s *Service) ManualPressurize(ctx context.Context, target float64) error
  func (s *Service) ManualCollect(ctx context.Context, pointIndex int) ([]float64, error)

步骤3: 前端 -- 手动模式 UI
文件: web/src/components/calibration/ManualControlPanel.vue (新建)

包含:
  - 目标压力输入 + "打压"按钮
  - 实时压力/稳定状态显示
  - "采集"按钮 (仅稳定后启用)
  - 当前点选择器
```

---

### 差距 #7：会话记录 (中级)

**参考实现** (`CollectionService.ts:88-97`):
```typescript
interface CollectionSession {
  id: string;
  startTime: string;
  endTime?: string;
  config: CalibrationConfig;
  pressurePoints: PressurePointData[];
  pressureDeviceId: string;
  measureDeviceIds: string[];
  status: CollectionState;
}
```
会话在 `initializeCollection()` 时创建，完成/停止时更新 `endTime` 和 `status`。

**当前状态**:
- 采集数据仅保存在内存中的 `rows []CollectedRow` (measurement) 或 `pressurePoints []PressurePoint` (calibration)
- 无会话 ID、无起止时间记录、无完整快照
- 无法从历史会话恢复或生成报告

**改善计划**:

```
步骤1: 后端 -- 会话模型
文件: internal/domain/session.go (新建或修改)

type CalibrationSession struct {
    ID               string                `json:"id"`
    StartTime        time.Time             `json:"startTime"`
    EndTime          *time.Time            `json:"endTime,omitempty"`
    Config           CalibrationConfig     `json:"config"`
    PressurePoints   []PressurePoint       `json:"pressurePoints"`
    MeasureDeviceID  string                `json:"measureDeviceId"`
    PressureDeviceID string                `json:"pressureDeviceId"`
    Status           string                `json:"status"`
}

步骤2: 后端 -- 会话管理
文件: internal/application/calibration/service.go (修改)

在 StartCalibration 时创建 session
在 EndCalibration / StopAutoCollection 时关闭 session
保存当前 session 供报告生成使用

步骤3: 后端 -- 会话 API
  GET /api/v1/calibration/session  (获取当前会话)
```

---

### 差距 #8：自定义压力点 (中级)

**参考实现** (`CollectionService.ts:243-337`):
- `initializeCollection(config, pressureDeviceId, measureDeviceIds, customPoints?)`
- `customPoints`: 可传入自定义压力值数组，覆盖等间距生成
- 前端压力表每个点的目标值可直接编辑 (`updatePressurePoint(id, value)`)
- 支持 `addPressurePoint`, `removePressurePoint`, `movePointUp`, `movePointDown`

**当前状态**:
- 后端 `GeneratePoints()` 仅支持等间距
- 前端校准视图中压力点可编辑性未确认

**改善计划**:

```
步骤1: 后端 -- 自定义点生成
文件: internal/application/calibration/service.go

修改 GeneratePoints 或新增:
  func (s *Service) SetCustomPoints(points []float64)
  func (s *Service) UpdatePoint(index int, value float64) error
  func (s *Service) AddPoint(value float64) error
  func (s *Service) RemovePoint(index int) error
  func (s *Service) MovePoint(index int, direction int) error

步骤2: 后端 -- API
  POST /api/v1/calibration/points/custom
  PUT  /api/v1/calibration/points/{index}
  POST /api/v1/calibration/points
  DELETE /api/v1/calibration/points/{index}
  POST /api/v1/calibration/points/{index}/move

步骤3: 前端 -- 压力点编辑
  表格中目标值列改为可编辑 input
  增加"添加"/"删除"/"上移"/"下移"操作按钮
```

---

### 差距 #9：校准参数配置持久化 (中级)

**参考实现** (`ConfigService.ts`):
- 持久化: `calibration` (minPressure, maxPressure, pointCount, precision, averageCount, stableDuration, pressureMode, controlMode)
- 持久化: `alarm` (AlarmConfig)
- 持久化: `lastPressureDeviceId`, `lastMeasureDeviceIds`
- 自动迁移: `precisionLevelScaleVersion`
- JSON 文件存储在 Electron userData 目录

**当前状态**:
- `internal/config/app_config.go`: 仅连接重试参数
- 校准参数不持久化，每次重启恢复默认值
- 无报警配置持久化

**改善计划**:

```
步骤1: 后端 -- 配置扩展
文件: internal/config/app_config.go (修改)

AppConfig 新增:
  Calibration CalibrationParams `json:"calibration"`
  Alarm       AlarmConfig      `json:"alarm"`
  LastDevices LastDevicesConfig `json:"lastDevices"`

type CalibrationParams struct {
    MinPressure    float64 `json:"minPressure"`
    MaxPressure    float64 `json:"maxPressure"`
    PointCount     int     `json:"pointCount"`
    Precision      int     `json:"precision"`
    AverageCount   int     `json:"averageCount"`
    StableDurationMs int   `json:"stableDurationMs"`
    PrecisionLevel float64 `json:"precisionLevel"`
    PressureMode   string  `json:"pressureMode"`
    ControlMode    string  `json:"controlMode"`
}

type LastDevicesConfig struct {
    PressureDeviceID string   `json:"pressureDeviceId"`
    MeasureDeviceIDs []string `json:"measureDeviceIds"`
}

步骤2: 后端 -- 配置 API
  GET  /api/v1/config/calibration
  POST /api/v1/config/calibration
  GET  /api/v1/config/alarm
  POST /api/v1/config/alarm

步骤3: 前端 -- 配置自动加载/保存
  页面加载时从后端读取上次配置
  参数变更时 debounce 250ms 自动保存
  设备连接时记住上次使用的设备
```

---

### 差距 #10：校准工作台 UI 对齐 (中级)

**参考实现** (`CalibrationView.vue`, ~1540 行):
- 紧凑双控制条布局: 上条(参数) + 下条(模式/操作/状态)
- 侧边栏: 设备卡片 + 单位一致性指示器
- 压力表: 序号 + 状态标签 + 可编辑目标值 + 16 通道值 (good/warn/error 色码) + 时间
- 进度条: 当前点/总数 + 百分比
- 稳定状态: 是/否 徽章 + 累计时间
- 报警区: 启用/声音/确认 开关 + 通道选择按钮

**当前状态**:
- `web/src/views/CalibrationView.vue`: 使用 Element Plus 组件，布局较为基础
- 侧边栏有设备面板，但控制面板和状态显示与参考差距较大
- 压力点列表组件 (`PressurePointList.vue`) 较简单

**改善计划**:

```
步骤1: 布局重构
参考 CalibrationView.vue 的双控制条布局:
  - 第一行 (config-bar): 参数输入 + "生成压力表"按钮
  - 第二行 (secondary): 模式切换 + 进度 + 稳定状态 + 报警设置 + 操作按钮

步骤2: 数据表格增强
参考 CalibrationView.vue 的 data-table:
  - 16 通道固定列
  - 值色码: good(绿)/warn(黄)/error(红) 基于 |value - target| / (maxPressure * precisionLevel)
  - 行高亮: current-row(蓝色左边框) + completed-row(浅绿底) + error-row(浅红底)
  - 回程分割行 (roundtrip-split-row)
  - 自动滚动到当前采集行

步骤3: 控制按钮组
  - 自动模式: [开始采集] [暂停] [恢复] [停止] [重置] [导出报告]
  - 手动模式: [采集] [暂停] [恢复] [停止] [重置] [导出报告]
  - 按钮状态联动: 采集状态 + 设备连接 + 稳定状态
```

---

## 三、实施优先级建议

| 优先级 | 差距项 | 原因 |
|--------|--------|------|
| P0 | #1 报告生成 | 核心交付物，没有报告=无法交付 |
| P1 | #2 回程模式 | 校准规范要求 |
| P1 | #3 报警系统 | 质量控制关键功能 |
| P1 | #4 多采样平均 | 测量精度核心 |
| P2 | #5 稳定性监控 | 自动采集流程体验 |
| P2 | #7 会话记录 | 报告生成的前置依赖 |
| P2 | #6 手动模式 | 特定场景需求 |
| P3 | #8 自定义压力点 | 用户体验优化 |
| P3 | #9 配置持久化 | 便利性提升 |
| P3 | #10 UI 对齐 | 视觉一致性 |

---

## 四、参考代码文件索引

| 功能领域 | 参考模块文件 | 对应当前文件 |
|---------|-------------|-------------|
| 采集引擎 | `src/main/services/CollectionService.ts` | `internal/application/calibration/service.go` |
| 报告生成 | `src/main/services/ReportService.ts` | `internal/report/report_service.go` |
| 稳定性监控 | `src/main/device/monitor/StabilityMonitor.ts` | `internal/workflow/stability_service.go` |
| 报警检测 | `CollectionService.ts checkAndHandleAlarm()` | `internal/workflow/alarm_service.go` |
| 校准配置 | `src/renderer/stores/calibrationStore.ts` | `web/src/stores/calibration/index.ts` |
| 设备管理 | `src/renderer/stores/deviceStore.ts` | `web/src/stores/measurement/deviceStore.ts` |
| 校准工作台 UI | `src/renderer/views/CalibrationView.vue` | `web/src/views/CalibrationView.vue` |
| 设备协议 | `src/main/device/adapters/`, `src/main/device/scpi/` | `internal/infrastructure/driver/` |
| 设备管理 | `src/main/device/manager/DeviceManager.ts` | `internal/application/deviceconnect/service.go` |
| 配置持久化 | `src/main/services/ConfigService.ts` | `internal/config/app_config.go` |
| 事件系统 | IPC events + EventEmitter | `internal/events/bus.go` (SSE) |
