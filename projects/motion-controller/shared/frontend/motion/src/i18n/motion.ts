// 运动控制模块翻译文本

/**
 * 运动控制模块中文翻译
 */
export const motionZh: Record<string, string> = {
  motionController: '运动控制器',
  config: '配置',
  noControllerConfig: '暂无控制器配置',
  clickConfigToAdd: '点击配置按钮添加',
  connected: '已连接',
  disconnected: '未连接',
  connectBtn: '连接',
  disconnectBtn: '断开',
  stopAll: '全部停止',
  eStop: '急停',
  eStopShortcut: '紧急停止 (快捷键: Esc)',
  controllerAlarm: '控制器报警',
  selectController: '选择控制器',
  selectControllerHint: '请选择一个控制器查看详情',

  axisNode: '轴节点',
  axisMotionControl: '轴运动控制',
  moving: '运动中',
  idle: '空闲',
  jogStep: '步长',
  targetPosition: '目标位置',
  move: '目标',
  setZero: '置零',
  stop: '停止',
  negLimit: '负限位',
  posLimit: '正限位',
  status: '状态',

  monitor: '监控',
  jog: '步进',
  homeStatus: '回零状态',
  notHomed: '未回零',
  homed: '已回零',

  jogHint: '点击 −/+ 按步长移动，或修改步长值后点击。',
  moveHint: '输入目标位置（相对于零点偏移），然后点击移动。',

  systemOnline: '系统在线',
  systemOffline: '系统离线',

  noAxesConfigured: '未配置运动轴',
  checkProfileAxes: '请在配置中启用至少一个轴',
  openConfig: '打开配置',

  axisControlAndMonitor: '轴控制与监控',
  standaloneWindowAxisControlAndMonitor: '独立窗口 • 轴控制与监控',
  currentPosition: '当前位置',
};

/**
 * 运动控制模块英文翻译
 */
export const motionEn: Record<string, string> = {
  motionController: 'Motion Controller',
  config: 'Config',
  noControllerConfig: 'No controller config',
  clickConfigToAdd: 'Click config to add',
  connected: 'Connected',
  disconnected: 'Disconnected',
  connectBtn: 'Connect',
  disconnectBtn: 'Disconnect',
  stopAll: 'Stop All',
  eStop: 'E-Stop',
  eStopShortcut: 'Emergency Stop (Shortcut: Esc)',
  controllerAlarm: 'Controller Alarm',
  selectController: 'Select Controller',
  selectControllerHint: 'Please select a controller to view details',

  axisNode: 'Axis Node',
  axisMotionControl: 'Axis Motion Control',
  moving: 'Moving',
  idle: 'Idle',
  jogStep: 'Jog Step',
  targetPosition: 'Target Position',
  move: 'Move',
  setZero: 'Set Zero',
  stop: 'Stop',
  negLimit: 'Neg Limit',
  posLimit: 'Pos Limit',
  status: 'Status',

  monitor: 'Monitor',
  jog: 'Jog',
  homeStatus: 'Home Status',
  notHomed: 'Not Homed',
  homed: 'Homed',

  jogHint: 'Click −/+ to move by step, or modify step value and click.',
  moveHint: 'Enter target position (relative to zero offset), then click move.',

  systemOnline: 'Online',
  systemOffline: 'Offline',

  noAxesConfigured: 'No axes configured',
  checkProfileAxes: 'Please enable at least one axis in config',
  openConfig: 'Open Config',

  axisControlAndMonitor: 'Axis Control & Monitor',
  standaloneWindowAxisControlAndMonitor: 'Standalone Window • Axis Control & Monitor',
  currentPosition: 'Current Position',
};
