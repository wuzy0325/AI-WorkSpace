## Why

当前计量工作台与参考模块（1605MeassureApp CalibrationView）在业务逻辑和UI上存在14项差异，包括设备管理、采集流程、报警配置持久化、导出报告、单位一致性检查等。这些问题导致计量模块功能不完整，用户体验不一致。

## What Changes

- 添加设备管理对话框（DeviceFormDialog），支持侧边栏添加/编辑/删除设备
- 设备卡片支持删除操作和扩展状态显示
- 生成压力表前调用单位一致性检查
- 实现完整的采集控制流程：自动采集、手动打压、手动采集、暂停/恢复/停止/重置
- 添加导出报告对话框（路径选择、模板信息、点数、模式）
- 添加报警通道选择对话框（16通道网格、全选/全不选）
- 报警配置变更时自动防抖保存到后端
- 报警通道默认初始化为全部16通道
- 数据表目标值编辑同步到 store
- 数据表无压力点时显示空状态提示
- 对接 useCollectionEvents 监控采集事件
- onMounted 加载报警配置
- 进度计算改为基于 currentPointIndex（与参考一致）

## Capabilities

### New Capabilities
- `device-management-dialog`: 设备添加/编辑对话框及删除操作
- `collection-control-flow`: 自动/手动采集控制流程（开始、打压、采集、暂停、恢复、停止、重置）
- `export-report-dialog`: 导出报告对话框（路径选择、模板信息）
- `alarm-channel-select-dialog`: 报警通道选择对话框（16通道网格）
- `alarm-config-persistence`: 报警配置自动防抖保存
- `unit-consistency-check`: 设备单位一致性检查与提示
- `collection-event-monitor`: 采集事件监听与状态同步

### Modified Capabilities
- `pressure-table`: 目标值编辑持久化到 store，空状态显示
- `progress-calculation`: 进度计算改为 currentPointIndex 方式

## Impact

- 前端：web/src/components/measurement/、web/src/stores/measurement/、web/src/views/MeasurementView.vue
- 后端：无影响（API 已存在）
- 新增组件：DeviceFormDialog 复用（已有）、ExportReportDialog、AlarmChannelSelectDialog
