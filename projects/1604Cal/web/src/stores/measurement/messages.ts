export const MeasurementMessages = {
  // Success
  START_OK: '计量采集已开始',
  MANUAL_READY: '手动模式已就绪',
  PAUSE_OK: '采集已暂停',
  STOP_OK: '采集已停止',
  POINTS_GENERATED: '压力点已生成',
  COLLECT_RESET: '采集数据已重置',

  // Warnings
  DEVICE_NOT_BOUND: '请先绑定计量设备',
  NO_PRESSURE_TABLE: '请先生成压力表',

  DEVICE_NOT_FOUND: '未找到指定设备',

  // Errors
  START_FAILED: (d: string) => `启动采集失败: ${d}`,
  MANUAL_START_FAILED: (d: string) => `启动手动模式失败: ${d}`,
  PAUSE_FAILED: '暂停采集失败',
  STOP_FAILED: '停止采集失败',
  GENERATE_FAILED: (d: string) => `生成压力点失败: ${d}`,
  AUTO_COLLECT_FAILED: (d: string) => `自动采集失败: ${d}`,
  MANUAL_PRESSURIZE_FAILED: (d: string) => `手动打压失败: ${d}`,
  MANUAL_COLLECT_FAILED: (d: string) => `手动采集失败: ${d}`,
  LOAD_FAILED: '加载设备列表失败',
  CONNECT_FAILED: (name: string) => `连接设备 ${name} 失败`,
  DISCONNECT_FAILED: (name: string) => `断开设备 ${name} 失败`,
  ADD_FAILED: '添加设备失败',
}
