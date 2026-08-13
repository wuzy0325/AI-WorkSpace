import { createI18nStore } from '@shared-frontend/i18n'

/**
 * 简体中文文案字典。
 *
 * 设计原则：
 * - 按"模块.功能"扁平化命名（如 topbar.startAcquisition），避免深层嵌套
 * - 含变量的文案用 {name} 占位符，由 t() 函数替换
 * - 用户可见的所有字符串必须由此字典统一管理
 */
const zh = {
  // ---- 通用 ----
  'common.cancel': '取消',
  'common.add': '添加',
  'common.delete': '删除',
  'common.close': '关闭',
  'common.reset': '重置',
  'common.save': '保存',
  'common.saving': '保存中...',
  'common.saved': '所有更改已保存',
  'common.unsaved': '未保存',
  'common.unsavedChanges': '有未保存的更改',
  'common.applyToAll': '应用到全部',
  'common.selectAll': '全选',
  'common.unselectAll': '全取消',
  'common.on': '开',
  'common.off': '关',
  'common.enabled': '开启',
  'common.disabled': '关闭',
  'common.precision': '精度',
  'common.precisionDecimals': '{n} 位小数',

  // ---- 应用级 ----
  // Wails 原生窗口标题（由 App.vue onMounted + watch(locale) 调用 Window.SetTitle 同步）
  'app.windowTitle': 'DAQ-P-1604 压力采集',
  // 退出应用确认框文案（MainTopBar 退出按钮触发，确认后调用后端 ExitApplication）
  'app.confirmExitTitle': '退出应用',
  'app.confirmExitText': '确认退出 DAQ-P-1604 吗？未保存的录制将被正常关闭并落盘。',
  'app.exit': '退出',

  // ---- 设备状态 ----
  'status.acquiring': '采集中',
  'status.stopping': '停止中',
  'status.connected': '已连接',
  'status.connecting': '连接中',
  'status.error': '错误',
  'status.disconnected': '未连接',
  'status.running': '运行中',
  'status.stopped': '已停止',
  'status.recording': '保存中',
  'status.notRecording': '未保存',

  // ---- 顶栏 ----
  'topbar.subtitle': 'Pressure Acquisition',
  'topbar.realtimeMonitor': '实时监控',
  'topbar.startAcquisition': '开始采集',
  'topbar.stopAcquisition': '停止采集',
  'topbar.startSave': '开始保存',
  'topbar.stopSave': '停止保存',
  'topbar.addDevice': '添加设备',
  'topbar.zeroCalibration': '校零',
  'topbar.zeroing': '校零中...',
  'topbar.connectBeforeZero': '请先连接设备再校零',
  'topbar.noDeviceAvailable': '没有可用的设备',
  'topbar.operating': '操作中...',
  'topbar.uiRefreshRate': '界面刷新率',
  'topbar.toggleLightTheme': '切换为浅色模式',
  'topbar.toggleDarkTheme': '切换为深色模式',
  'topbar.toggleLanguage': '切换语言',
  'topbar.exitApp': '退出应用',
  // switchToZh/switchToEn 故意用目标语言描述：屏幕阅读器读出"切换到中文" / "Switch to English"
  // 让用户在切换前听到目标语言发音，便于识别将切到哪种语言。
  'topbar.switchToZh': '切换到中文',
  'topbar.switchToEn': 'Switch to English',

  // ---- 底部状态栏 ----
  'bottombar.acquisitionStatus': '采集状态',
  'bottombar.recordingStatus': '记录状态',
  'bottombar.devices': '设备',
  'bottombar.online': '在线',
  'bottombar.recorded': '已记录',
  'bottombar.dropped': '丢弃',
  'bottombar.fileCount': '文件数',
  'bottombar.recordingError': '录制错误',
  'bottombar.runtime': '运行时间',
  'bottombar.systemTime': '系统时间',
  'bottombar.saveFile': '保存文件',
  'bottombar.saveDir': '保存目录',

  // ---- 设备侧栏 ----
  'sidebar.deviceList': '设备列表',
  'sidebar.scanDevices': '扫描设备',
  'sidebar.addDevice': '添加设备',
  'sidebar.noDevices': '暂无设备',
  'sidebar.addHint': '点击顶栏 + 添加 P1604',
  'sidebar.unnamed': '未命名',
  'sidebar.unnamedDevice': '未命名设备',
  'sidebar.deleteDevice': '删除设备',
  'sidebar.confirmDeleteTitle': '确认删除设备',
  'sidebar.confirmDeleteSubtitle': '删除后将移除该设备配置与当前选择状态。',
  'sidebar.confirmDeleteText': '确认删除设备 "{name}" 吗？',

  // ---- 监控主视图 ----
  'monitor.selectDevice': '选择一个设备开始监控',
  'monitor.selectDeviceHint': '从左侧设备列表中选择一台 P1604，或者点击顶栏 + 添加新设备',
  'monitor.realtimeWaveform': '实时波形',
  'monitor.multiChannel': '多通道并行',
  'monitor.dataExport': '数据导出',
  'monitor.unnamedDevice': '未命名设备',
  'monitor.disconnect': '断开',
  'monitor.connect': '连接',
  'monitor.connecting': '连接中',
  'monitor.config': '配置',
  'monitor.curveCount': '{n} 条曲线',
  'monitor.selectChannelHint': '· 点击右侧按钮选择通道',
  'monitor.channelSelection': '通道选择',
  'monitor.channelMonitor': '通道监控',
  'monitor.channelMonitorHint': '通过上方"通道选择"按钮可以控制波形图中显示的通道',

  // ---- 通道卡片 ----
  'channel.pickColor': '选择波形颜色',

  // ---- 配置面板 ----
  'config.deviceConfig': '设备配置',
  'config.deviceName': '设备名称',
  'config.deviceNamePlaceholder': '输入设备名称',
  'config.deviceNameRequired': '请输入设备名称',
  'config.hardwareParams': '硬件参数',
  'config.channelConfig': '通道配置',
  'config.samplingRate': '采样频率',
  'config.pressureUnit': '压力单位（全部通道）',
  'config.globalPrecision': '全局精度（全部通道）',
  'config.applyGlobalPrecisionTitle': '将全局精度应用到所有通道',
  'config.autoConnect': '自动连接',
  'config.deviceTimestamp': '设备硬件时间戳',
  'config.acquireLockHint': '采集中不允许变更配置，请先停止采集。',
  'config.acquireLock': '采集中不允许变更配置',
  'config.channelEnabledCount': '{n}/18 启用',
  'config.atmosphericPressure': '大气压力',
  'config.atmosphericTemperature': '大气温度',
  'config.disableChannel': '禁用通道',
  'config.enableChannel': '启用通道',
  'config.savedAndApplied': '配置已保存并应用到设备',
  'config.hardwareApplyFailed': '硬件配置应用失败',
  'config.saved': '配置已保存',
  'config.saveFailed': '保存失败',
  'config.defaultChannelName': '通道 {n}',

  // ---- 扫描结果列表 ----
  'scan.scanning': '正在扫描...',
  'scan.noDevices': '未发现设备',
  'scan.noDevicesHint': '请确保设备已开机并连接在同一网络',
  'scan.selectAllTitle': '全部选中',
  'scan.unselectAllTitle': '全部取消',
  'scan.selectAll': '全选',
  'scan.unselectAll': '取消全选',
  'scan.selectionSummary': '已选 {checked} / 可添加 {selectable} · 共 {total} 台',
  'scan.editNameChecked': '可修改设备名称',
  'scan.editNameUnchecked': '勾选后可修改设备名称',
  'scan.added': '已添加',

  // ---- 实时图表 ----
  'chart.waitingData': '等待实时数据...',
  'chart.noChannelSelected': '未选择通道',
  'chart.willShowWhenAcquiring': '设备开始采集后将自动显示波形',
  'chart.pleaseSelectChannel': '请在上方通道选择中勾选需要显示的通道',

  // ---- 对话框 ----
  'dialog.scanTitle': '扫描设备',
  'dialog.scanSubtitle': '勾选发现的设备后一次性添加',
  'dialog.rescan': '重新扫描',
  'dialog.scanInProgress': '扫描进行中，请稍候',
  'dialog.addingDevices': '添加中...',
  'dialog.addSelected': '添加所选 ({n})',
  'dialog.defaultAutoConnect': '添加的设备默认启用开机自动连接（本次新加会立即尝试连接）',
  'dialog.addDeviceTitle': '添加 P1604 设备',
  'dialog.addDeviceSubtitle': '通过 IP 端口接入压力采集器',
  'dialog.deviceName': '设备名称',
  'dialog.deviceNamePlaceholder': '例如: 压力采集器 1',
  'dialog.ipAddress': 'IP 地址',
  'dialog.localIpAddress': '绑定本地 IP（可选，多网卡使用）',
  'dialog.port': '端口',
  'dialog.inputDeviceName': '请输入设备名称',
  'dialog.inputIpAddress': '请输入 IP 地址',
  'dialog.addDeviceFailed': '添加设备失败',

  // ---- 日志面板 ----
  'log.title': '日志',
  'log.entries': '{n} 条',
  'log.collapse': '收起',
  'log.copyAll': '拷贝全部日志',
  'log.copyEntry': '拷贝此条日志',
  'log.clear': '清空日志',
  'log.saveToFile': '保存日志到文件',
  'log.savingTo': '日志正在保存至: {dir}\n点击停止',
  'log.level': '级别',
  'log.levelDebug': '调试',
  'log.levelInfo': '信息',
  'log.levelWarn': '警告',
  'log.levelError': '错误',
  'log.category': '分类',
  'log.allGroups': '全部',
  'log.empty': '暂无日志',
  'log.throttled': '… 及 {n} 条同类日志',

  // ---- 日志分组/分类标签 ----
  'logGroup.system': '系统',
  'logGroup.communication': '通信',
  'logGroup.acquisition': '采集',
  'logCategory.system': '系统',
  'logCategory.hardwareSend': '发送',
  'logCategory.hardwareRecv': '接收',
  'logCategory.acquisition': '采集',

  // ---- 错误消息（throw / 状态栏显示） ----
  'error.saveConfigFailed': '保存配置失败',
  'error.applyConfigTimeout': '应用配置超时，设备可能无响应',
  'error.zeroCalibrationTimeout': '校零超时，设备可能无响应',
  'error.scanTimeout': '设备扫描超时，请检查网络或防火墙后重试',
  'error.connectTimeout': '连接设备超时，请检查设备响应或网络后重试',
  'error.duplicateDevice': '该设备已添加，请勿重复添加',
  'error.duplicateName': '设备名已存在，请更换名称',

  // ---- 日志消息（写入 logStore 的运行时消息） ----
  'logMessage.autoConnectFailed': '自动连接失败: {message}',
  'logMessage.deviceUnitSynced': '设备 [{id}] 单位已从硬件同步: {prev} -> {next}',
  'logMessage.deviceStateError': '设备 [{id}] 状态异常: {error}',
  'logMessage.deviceDisconnected': '设备 [{id}] 已断开（后端推送，前一状态: {prev}）',
  'logMessage.scanSkipped': '扫描添加：{count} 台设备因地址重复被跳过 ({details})',
  'logMessage.scanFailed': '扫描添加失败：{name} - {error}',
  'logMessage.scanDiscoveryFailed': '设备扫描失败：{error}',
  'logMessage.scanAdded': '扫描添加：成功新增 {count} 台设备',
  'logMessage.autoConnectDeviceFailed': '自动连接失败：{name} - {reason}',
} as const

/**
 * 英文文案字典。
 *
 * 用 `as const satisfies Record<LocaleKey, string>` 同时获得：
 * 1. satisfies 做双向 key 集合校验（zh 多/少 key 都会编译报错）
 * 2. as const 保留字面量类型供 IDE 智能提示
 */
const en = {
  // ---- common ----
  'common.cancel': 'Cancel',
  'common.add': 'Add',
  'common.delete': 'Delete',
  'common.close': 'Close',
  'common.reset': 'Reset',
  'common.save': 'Save',
  'common.saving': 'Saving...',
  'common.saved': 'All changes saved',
  'common.unsaved': 'Unsaved',
  'common.unsavedChanges': 'You have unsaved changes',
  'common.applyToAll': 'Apply to All',
  'common.selectAll': 'Select All',
  'common.unselectAll': 'Clear All',
  'common.on': 'On',
  'common.off': 'Off',
  'common.enabled': 'Enabled',
  'common.disabled': 'Disabled',
  'common.precision': 'Precision',
  'common.precisionDecimals': '{n} decimals',

  // ---- app-level ----
  'app.windowTitle': 'DAQ-P-1604 Pressure Acquisition',
  'app.confirmExitTitle': 'Exit Application',
  'app.confirmExitText': 'Exit DAQ-P-1604? Any in-progress recording will be flushed and closed.',
  'app.exit': 'Exit',

  // ---- status ----
  'status.acquiring': 'Acquiring',
  'status.stopping': 'Stopping',
  'status.connected': 'Connected',
  'status.connecting': 'Connecting',
  'status.error': 'Error',
  'status.disconnected': 'Disconnected',
  'status.running': 'Running',
  'status.stopped': 'Stopped',
  'status.recording': 'Saving',
  'status.notRecording': 'Not Saved',

  // ---- topbar ----
  'topbar.subtitle': 'Pressure Acquisition',
  'topbar.realtimeMonitor': 'Real-time Monitor',
  'topbar.startAcquisition': 'Start Acquisition',
  'topbar.stopAcquisition': 'Stop Acquisition',
  'topbar.startSave': 'Start Save',
  'topbar.stopSave': 'Stop Save',
  'topbar.addDevice': 'Add Device',
  'topbar.zeroCalibration': 'Zero Calibration',
  'topbar.zeroing': 'Zeroing...',
  'topbar.connectBeforeZero': 'Connect the device before zeroing',
  'topbar.noDeviceAvailable': 'No device available',
  'topbar.operating': 'Operating...',
  'topbar.uiRefreshRate': 'UI Refresh Rate',
  'topbar.toggleLightTheme': 'Switch to Light Theme',
  'topbar.toggleDarkTheme': 'Switch to Dark Theme',
  'topbar.toggleLanguage': 'Switch Language',
  'topbar.exitApp': 'Exit Application',
  'topbar.switchToZh': '切换到中文',
  'topbar.switchToEn': 'Switch to English',

  // ---- bottombar ----
  'bottombar.acquisitionStatus': 'Acquisition',
  'bottombar.recordingStatus': 'Recording',
  'bottombar.devices': 'Devices',
  'bottombar.online': 'Online',
  'bottombar.recorded': 'Recorded',
  'bottombar.dropped': 'Dropped',
  'bottombar.fileCount': 'Files',
  'bottombar.recordingError': 'Recording Error',
  'bottombar.runtime': 'Runtime',
  'bottombar.systemTime': 'System Time',
  'bottombar.saveFile': 'File',
  'bottombar.saveDir': 'Directory',

  // ---- sidebar ----
  'sidebar.deviceList': 'Devices',
  'sidebar.scanDevices': 'Scan Devices',
  'sidebar.addDevice': 'Add Device',
  'sidebar.noDevices': 'No devices',
  'sidebar.addHint': 'Click + in the top bar to add a P1604',
  'sidebar.unnamed': 'Unnamed',
  'sidebar.unnamedDevice': 'Unnamed Device',
  'sidebar.deleteDevice': 'Delete Device',
  'sidebar.confirmDeleteTitle': 'Confirm Device Deletion',
  'sidebar.confirmDeleteSubtitle': 'The device configuration and current selection will be removed.',
  'sidebar.confirmDeleteText': 'Delete device "{name}"?',

  // ---- monitor ----
  'monitor.selectDevice': 'Select a device to start monitoring',
  'monitor.selectDeviceHint': 'Pick a P1604 from the device list, or click + in the top bar to add a new one',
  'monitor.realtimeWaveform': 'Real-time Waveform',
  'monitor.multiChannel': 'Multi-channel',
  'monitor.dataExport': 'Data Export',
  'monitor.unnamedDevice': 'Unnamed Device',
  'monitor.disconnect': 'Disconnect',
  'monitor.connect': 'Connect',
  'monitor.connecting': 'Connecting',
  'monitor.config': 'Config',
  'monitor.curveCount': '{n} curves',
  'monitor.selectChannelHint': '· Click the right button to select channels',
  'monitor.channelSelection': 'Channel Selection',
  'monitor.channelMonitor': 'Channel Monitoring',
  'monitor.channelMonitorHint': 'Use the "Channel Selection" button above to control which channels appear in the chart',

  // ---- channel ----
  'channel.pickColor': 'Pick waveform color',

  // ---- config ----
  'config.deviceConfig': 'Device Config',
  'config.deviceName': 'Device Name',
  'config.deviceNamePlaceholder': 'Enter device name',
  'config.deviceNameRequired': 'Please enter a device name',
  'config.hardwareParams': 'Hardware',
  'config.channelConfig': 'Channels',
  'config.samplingRate': 'Sampling Rate',
  'config.pressureUnit': 'Pressure Unit (All Channels)',
  'config.globalPrecision': 'Global Precision (All Channels)',
  'config.applyGlobalPrecisionTitle': 'Apply global precision to all channels',
  'config.autoConnect': 'Auto Connect',
  'config.deviceTimestamp': 'Hardware Timestamp',
  'config.acquireLockHint': 'Configuration cannot be changed during acquisition. Please stop acquisition first.',
  'config.acquireLock': 'Locked during acquisition',
  'config.channelEnabledCount': '{n}/18 Enabled',
  'config.atmosphericPressure': 'Atmospheric Pressure',
  'config.atmosphericTemperature': 'Atmospheric Temperature',
  'config.disableChannel': 'Disable Channel',
  'config.enableChannel': 'Enable Channel',
  'config.savedAndApplied': 'Config saved and applied to device',
  'config.hardwareApplyFailed': 'Failed to apply config to hardware',
  'config.saved': 'Config saved',
  'config.saveFailed': 'Save failed',
  'config.defaultChannelName': 'Channel {n}',

  // ---- scan ----
  'scan.scanning': 'Scanning...',
  'scan.noDevices': 'No devices found',
  'scan.noDevicesHint': 'Make sure the device is powered on and connected to the same network',
  'scan.selectAllTitle': 'Select All',
  'scan.unselectAllTitle': 'Clear All',
  'scan.selectAll': 'Select All',
  'scan.unselectAll': 'Clear All',
  'scan.selectionSummary': 'Selected {checked} / Available {selectable} · Total {total}',
  'scan.editNameChecked': 'Edit device name',
  'scan.editNameUnchecked': 'Check the box to edit the device name',
  'scan.added': 'Added',

  // ---- chart ----
  'chart.waitingData': 'Waiting for real-time data...',
  'chart.noChannelSelected': 'No channel selected',
  'chart.willShowWhenAcquiring': 'Waveform will appear once acquisition starts',
  'chart.pleaseSelectChannel': 'Please select channels in the channel selection above',

  // ---- dialog ----
  'dialog.scanTitle': 'Scan Devices',
  'dialog.scanSubtitle': 'Check discovered devices and add them in one batch',
  'dialog.rescan': 'Rescan',
  'dialog.scanInProgress': 'Scan in progress, please wait',
  'dialog.addingDevices': 'Adding...',
  'dialog.addSelected': 'Add Selected ({n})',
  'dialog.defaultAutoConnect': 'New devices default to auto-connect on startup (will try to connect immediately)',
  'dialog.addDeviceTitle': 'Add P1604 Device',
  'dialog.addDeviceSubtitle': 'Connect to a pressure acquisition device via IP and port',
  'dialog.deviceName': 'Device Name',
  'dialog.deviceNamePlaceholder': 'e.g., Pressure DAQ 1',
  'dialog.ipAddress': 'IP Address',
  'dialog.localIpAddress': 'Local IP binding (optional)',
  'dialog.port': 'Port',
  'dialog.inputDeviceName': 'Please enter a device name',
  'dialog.inputIpAddress': 'Please enter an IP address',
  'dialog.addDeviceFailed': 'Failed to add device',

  // ---- log ----
  'log.title': 'Logs',
  'log.entries': '{n} entries',
  'log.collapse': 'Collapse',
  'log.copyAll': 'Copy All',
  'log.copyEntry': 'Copy This Entry',
  'log.clear': 'Clear Logs',
  'log.saveToFile': 'Save logs to file',
  'log.savingTo': 'Logs are being saved to: {dir}\nClick to stop',
  'log.level': 'Level',
  'log.levelDebug': 'Debug',
  'log.levelInfo': 'Info',
  'log.levelWarn': 'Warn',
  'log.levelError': 'Error',
  'log.category': 'Category',
  'log.allGroups': 'All',
  'log.empty': 'No logs',
  'log.throttled': '… and {n} similar logs',

  // ---- logGroup / logCategory ----
  'logGroup.system': 'System',
  'logGroup.communication': 'Comm',
  'logGroup.acquisition': 'Acquisition',
  'logCategory.system': 'System',
  'logCategory.hardwareSend': 'Send',
  'logCategory.hardwareRecv': 'Recv',
  'logCategory.acquisition': 'Acquisition',

  // ---- error ----
  'error.saveConfigFailed': 'Failed to save config',
  'error.applyConfigTimeout': 'Config apply timed out, device may be unresponsive',
  'error.zeroCalibrationTimeout': 'Zero calibration timed out, device may be unresponsive',
  'error.scanTimeout': 'Device scan timed out. Check the network or firewall and try again.',
  'error.connectTimeout': 'Device connection timed out. Check the device response or network and try again.',
  'error.duplicateDevice': 'This device has already been added',
  'error.duplicateName': 'Device name already exists. Please choose another.',

  // ---- logMessage ----
  'logMessage.autoConnectFailed': 'Auto-connect failed: {message}',
  'logMessage.deviceUnitSynced': 'Device [{id}] unit synced from hardware: {prev} -> {next}',
  'logMessage.deviceStateError': 'Device [{id}] state error: {error}',
  'logMessage.deviceDisconnected': 'Device [{id}] disconnected (backend push, previous: {prev})',
  'logMessage.scanSkipped': 'Scan add: {count} device(s) skipped due to duplicate address ({details})',
  'logMessage.scanFailed': 'Scan add failed: {name} - {error}',
  'logMessage.scanDiscoveryFailed': 'Device scan failed: {error}',
  'logMessage.scanAdded': 'Scan add: successfully added {count} device(s)',
  'logMessage.autoConnectDeviceFailed': 'Auto-connect failed: {name} - {reason}',
} as const satisfies Record<keyof typeof zh, string>

/** 翻译字典 key 类型。由 zh 字典推导，供组件代码获得类型安全的 t() 调用 */
export type LocaleKey = keyof typeof zh

/**
 * 项目级 i18n store。
 *
 * 由 shared/frontend/i18n 的工厂创建，基础设施（t、locale、timeLocale、localStorage 读写）
 * 来自工厂，本文件仅负责字典内容与 storageKey 隔离。
 */
export const useI18nStore = createI18nStore({
  zh,
  en,
  storageKey: 'daq-p1604.locale',
})
