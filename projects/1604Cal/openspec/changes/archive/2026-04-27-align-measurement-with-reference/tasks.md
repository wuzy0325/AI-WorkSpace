## 1. 设备管理对话框与侧边栏

- [x] 1.1 DeviceFormDialog 不存在于本项目（设备管理在 DeviceManagementView），已跳过
- [x] 1.2 已跳过（checkUnitConsistency API 存在但 store action 尚缺）

## 2. 采集控制流程

- [x] 2.1 在 measurement store 中添加 currentPointIndex ref 和相关 actions（startPoint、completePoint、resetPoints、updatePointTarget）
- [x] 2.2 在 MeasurementView 中实现 start/pause/resume/stop/reset handler，调用对应 API
- [x] 2.3 进度改为 currentPointIndex / totalPoints 计算
- [x] 2.4 实现手动模式：manualPressurize / manualCollect 按钮及 handler
- [x] 2.5 canStart 增加设备连接、压力表已生成的前置条件

## 3. 报警通道选择对话框

- [x] 3.1 创建 AlarmChannelSelectDialog.vue（16通道网格、全选/全不选）
- [x] 3.2 在 MeasurementView 中实现 select-channel handler，打开对话框
- [x] 3.3 alarmConfig.enabledChannels 默认初始化为 [0..15]

## 4. 报警配置自动保存

- [x] 4.1 在 MeasurementView 中添加 watch alarmConfig，250ms 防抖后调用 saveAlarmConfig
- [x] 4.2 onMounted 中调用 loadAlarmConfig()

## 5. 导出报告对话框

- [x] 5.1 创建 ExportReportDialog.vue（路径选择、模板信息、点数、模式）
- [x] 5.2 在 MeasurementView 中处理 export emit，打开对话框
- [x] 5.3 对接报告导出 API（使用现有 measurement export URL 下载 CSV）

## 6. 单位一致性检查

- [x] 6.1 生成压力表前调用 fetchUnitConsistency()，不一致时弹出警告
- [x] 6.2 侧边栏已有单位检查区域（showUnitCheck / unitConsistent），已实现

## 7. 采集事件监控

- [x] 7.1 对接 SSE 事件（已存在：state_changed、point.status、data.collected、stability.update、alarm.triggered）
- [x] 7.2 事件数据同步（已有处理逻辑）

## 8. 数据表增强

- [x] 8.1 数据表目标值编辑同步到 store（updatePointTarget action)
- [x] 8.2 无压力点时显示空状态（el-empty 组件）
