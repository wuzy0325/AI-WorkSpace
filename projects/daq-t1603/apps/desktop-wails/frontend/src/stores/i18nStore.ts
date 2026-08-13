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
  'common.clear': '清除',
  'common.save': '保存',
  'common.saving': '保存中...',
  'common.saved': '所有更改已保存',
  'common.unsaved': '未保存',
  'common.unsavedChanges': '有未保存的更改',
  'common.selectAll': '全选',
  'common.unselectAll': '全取消',
  'common.on': '开',
  'common.off': '关',
  'common.enabled': '开启',
  'common.disabled': '关闭',
  'common.rescan': '重新扫描',

  // ---- 应用级 ----
  // Wails 原生窗口标题（由 App.vue onMounted + watch(locale) 调用 Window.SetTitle 同步）
  'app.windowTitle': 'DAQ-T-1603 温度采集',
  // 退出应用确认框文案（MainTopBar 退出按钮触发，确认后调用后端 ExitApplication）
  'app.confirmExitTitle': '退出应用',
  'app.confirmExitText': '确认退出 DAQ-T-1603 吗？未保存的录制将被正常关闭并落盘。',
  'app.exit': '退出',

  // ---- 设备状态 ----
  'status.acquiring': '采集中',
  'status.starting': '启动中',
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
  'topbar.subtitle': 'Temperature Acquisition',
  'topbar.realtimeMonitor': '实时监控',
  'topbar.startAcquisition': '开始采集',
  'topbar.stopAcquisition': '停止采集',
  'topbar.startSave': '开始保存',
  'topbar.stopSave': '停止保存',
  'topbar.pleaseAddDevice': '请先添加设备',
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
  'bottombar.recordingError': '录制错误',
  'bottombar.deviceError': '设备错误',
  'bottombar.deviceErrorCount': '{n} 台异常',
  'bottombar.currentDevice': '当前设备',
  'bottombar.runtime': '运行时间',
  'bottombar.systemTime': '系统时间',
  'bottombar.saveDir': '保存目录',

  // ---- 设备侧栏 ----
  'sidebar.deviceList': '设备列表',
  'sidebar.scanDevices': '扫描设备',
  'sidebar.addDevice': '添加设备',
  'sidebar.searchDevices': '搜索设备（名称 / 地址）',
  'sidebar.noSearchMatch': '未找到匹配设备',
  'sidebar.clearSearch': '清除搜索',
  'sidebar.noDevices': '暂无设备',
  'sidebar.addHint': '点击上方 + 添加 T1603',
  'sidebar.unnamed': '未命名',
  'sidebar.unnamedDevice': '未命名设备',
  'sidebar.deleteDevice': '删除设备',
  'sidebar.confirmDeleteTitle': '确认删除设备',
  'sidebar.confirmDeleteSubtitle': '删除后将移除该设备配置与当前选择状态。',
  'sidebar.confirmDeleteText': '确认删除设备 “{name}” 吗？',

  // ---- 监控主视图 ----
  'monitor.selectDevice': '选择一个设备开始监控',
  'monitor.selectDeviceHint': '从左侧设备列表中选择一台 T1603，或者点击顶栏 + 添加新设备',
  'monitor.realtimeWaveform': '实时波形',
  'monitor.multiChannel': '多通道并行',
  'monitor.dataExport': '数据导出',
  'monitor.unnamedDevice': '未命名设备',
  'monitor.thermocoupleType': '{type} 型热电偶',
  'monitor.disconnect': '断开',
  'monitor.connect': '连接',
  'monitor.connecting': '连接中',
  'monitor.config': '配置',
  'monitor.curveCount': '{n} 条曲线',
  'monitor.selectChannelHint': '· 点击右侧按钮选择通道',
  'monitor.channelSelection': '通道选择',
  'monitor.clearWaveform': '清除波形',
  'monitor.clearWaveformTitle': '清除当前波形',
  'monitor.channelMonitor': '通道监控',
  'monitor.channelMonitorHint': '通过上方“通道选择”按钮可以控制波形图中显示的通道',

  // ---- 通道卡片 ----
  'channel.pickColor': '选择波形颜色',

  // ---- 配置面板 ----
  'config.deviceConfig': '设备配置',
  'config.deviceName': '设备名称',
  'config.deviceNamePlaceholder': '例如: 温度采集器 1',
  'config.hardwareParams': '硬件参数',
  'config.channelConfig': '通道配置',
  'config.samplingRate': '采样频率',
  'config.hardwareTimestamp': '硬件时间戳',
  'config.autoConnect': '自动连接',
  'config.thermocoupleType': '热电偶类型（全部通道）',
  'config.thermocoupleTypeSuffix': ' 型',
  'config.acquireLockHint': '采集中不允许变更配置，请先停止采集。',
  'config.acquireLock': '采集中不允许变更配置',
  'config.channelEnabledCount': '{n}/16 启用',
  'config.disableChannel': '禁用通道',
  'config.enableChannel': '启用通道',
  'config.defaultChannelName': '通道 {n}',
  'config.savedAndApplied': '配置已保存并应用到设备',
  'config.savedLocalOfflineHint': '配置已保存到本地，但设备离线未应用到硬件，请重新连接后再次保存',
  'config.hardwareApplyFailed': '配置已保存，但硬件应用失败：{reason}',
  'config.saved': '配置已保存',
  'config.saveFailed': '保存失败',
  'config.error.nameExists': '设备名称已被其他设备占用，请修改后重试',

  // ---- 扫描结果列表 ----
  'scan.scanning': '正在扫描...',
  'scan.noDevices': '未发现设备',
  'scan.noDevicesHint': '请确保设备已开机并连接在同一网络',
  'scan.added': '已添加',
  'scan.deviceAddedTitle': '该设备已添加',
  'scan.addThisDevice': '添加此设备',
  'scan.discovered': '发现 {n} 台设备',

  // ---- 实时图表 ----
  'chart.waitingData': '等待实时数据...',
  'chart.noChannelSelected': '未选择通道',
  'chart.willShowWhenAcquiring': '设备开始采集后将自动显示波形',
  'chart.pleaseSelectChannel': '请在上方通道选择中勾选需要显示的通道',

  // ---- 对话框 ----
  'dialog.scanTitle': '扫描设备',
  'dialog.scanSubtitle': '局域网中发现 DAQ-T-1603 设备',
  'dialog.addDeviceTitle': '添加 T1603 设备',
  'dialog.addDeviceSubtitle': '通过 IP 端口接入温度采集器',
  'dialog.deviceName': '设备名称',
  'dialog.deviceNamePlaceholder': '例如: 温度采集器 1',
  'dialog.ipAddress': 'IP 地址',
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
  'log.levelAndAbove': '（及以上）',
  'log.levelDebug': '调试',
  'log.levelInfo': '信息',
  'log.levelWarn': '警告',
  'log.levelError': '错误',
  'log.category': '分类',
  'log.allGroups': '全部',
  'log.empty': '暂无日志',
  'log.searchPlaceholder': '检索日志（消息/标签/详情/设备）',
  'log.clearSearch': '清除检索',
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
  'error.duplicateDevice': '该设备已添加，请勿重复添加',
  'error.recordingDirNotFound': '保存目录不存在：{dir}，请重新选择有效目录',
  'error.recordingPermissionDenied': '无权限写入目录：{dir}，请检查目录权限',
  'error.recordingStartFailed': '启动录制失败：{reason}',

  // ---- 日志消息（写入 logStore 的运行时消息） ----
  'logMessage.autoConnectFailed': '部分设备自动连接失败: {message}',
  'logMessage.syncRefreshRateFailed': '同步后端刷新率失败: {message}',
  'logMessage.recordingFatal': '录制不可恢复错误 [{deviceId}]: {error}',
  'logMessage.recordingBackpressure': '录制队列背压丢帧 [{deviceId}]: 队列 {queueLen}/{queueCap}',
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
  'common.clear': 'Clear',
  'common.save': 'Save',
  'common.saving': 'Saving...',
  'common.saved': 'All changes saved',
  'common.unsaved': 'Unsaved',
  'common.unsavedChanges': 'You have unsaved changes',
  'common.selectAll': 'Select All',
  'common.unselectAll': 'Clear All',
  'common.on': 'On',
  'common.off': 'Off',
  'common.enabled': 'Enabled',
  'common.disabled': 'Disabled',
  'common.rescan': 'Rescan',

  // ---- app-level ----
  'app.windowTitle': 'DAQ-T-1603 Temperature Acquisition',
  'app.confirmExitTitle': 'Exit Application',
  'app.confirmExitText': 'Exit DAQ-T-1603? Any in-progress recording will be flushed and closed.',
  'app.exit': 'Exit',

  // ---- status ----
  'status.acquiring': 'Acquiring',
  'status.starting': 'Starting',
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
  'topbar.subtitle': 'Temperature Acquisition',
  'topbar.realtimeMonitor': 'Real-time Monitor',
  'topbar.startAcquisition': 'Start Acquisition',
  'topbar.stopAcquisition': 'Stop Acquisition',
  'topbar.startSave': 'Start Save',
  'topbar.stopSave': 'Stop Save',
  'topbar.pleaseAddDevice': 'Please add a device first',
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
  'bottombar.recordingError': 'Recording Error',
  'bottombar.deviceError': 'Device Error',
  'bottombar.deviceErrorCount': '{n} error(s)',
  'bottombar.currentDevice': 'Current',
  'bottombar.runtime': 'Runtime',
  'bottombar.systemTime': 'System Time',
  'bottombar.saveDir': 'Directory',

  // ---- sidebar ----
  'sidebar.deviceList': 'Devices',
  'sidebar.scanDevices': 'Scan Devices',
  'sidebar.addDevice': 'Add Device',
  'sidebar.searchDevices': 'Search devices (name / address)',
  'sidebar.noSearchMatch': 'No matching devices',
  'sidebar.clearSearch': 'Clear search',
  'sidebar.noDevices': 'No devices',
  'sidebar.addHint': 'Click + above to add a T1603',
  'sidebar.unnamed': 'Unnamed',
  'sidebar.unnamedDevice': 'Unnamed Device',
  'sidebar.deleteDevice': 'Delete Device',
  'sidebar.confirmDeleteTitle': 'Confirm Device Deletion',
  'sidebar.confirmDeleteSubtitle': 'The device configuration and current selection will be removed.',
  'sidebar.confirmDeleteText': 'Delete device "{name}"?',

  // ---- monitor ----
  'monitor.selectDevice': 'Select a device to start monitoring',
  'monitor.selectDeviceHint': 'Pick a T1603 from the device list, or click + in the top bar to add a new one',
  'monitor.realtimeWaveform': 'Real-time Waveform',
  'monitor.multiChannel': 'Multi-channel',
  'monitor.dataExport': 'Data Export',
  'monitor.unnamedDevice': 'Unnamed Device',
  'monitor.thermocoupleType': 'Type {type} thermocouple',
  'monitor.disconnect': 'Disconnect',
  'monitor.connect': 'Connect',
  'monitor.connecting': 'Connecting',
  'monitor.config': 'Config',
  'monitor.curveCount': '{n} curves',
  'monitor.selectChannelHint': '· Click the right button to select channels',
  'monitor.channelSelection': 'Channel Selection',
  'monitor.clearWaveform': 'Clear Waveform',
  'monitor.clearWaveformTitle': 'Clear current waveform',
  'monitor.channelMonitor': 'Channel Monitoring',
  'monitor.channelMonitorHint': 'Use the "Channel Selection" button above to control which channels appear in the chart',

  // ---- channel ----
  'channel.pickColor': 'Pick waveform color',

  // ---- config ----
  'config.deviceConfig': 'Device Config',
  'config.deviceName': 'Device Name',
  'config.deviceNamePlaceholder': 'e.g., Temperature DAQ 1',
  'config.hardwareParams': 'Hardware',
  'config.channelConfig': 'Channels',
  'config.samplingRate': 'Sampling Rate',
  'config.hardwareTimestamp': 'Hardware Timestamp',
  'config.autoConnect': 'Auto Connect',
  'config.thermocoupleType': 'Thermocouple Type (All Channels)',
  'config.thermocoupleTypeSuffix': '',
  'config.acquireLockHint': 'Configuration cannot be changed during acquisition. Please stop acquisition first.',
  'config.acquireLock': 'Locked during acquisition',
  'config.channelEnabledCount': '{n}/16 Enabled',
  'config.disableChannel': 'Disable Channel',
  'config.enableChannel': 'Enable Channel',
  'config.defaultChannelName': 'Channel {n}',
  'config.savedAndApplied': 'Config saved and applied to device',
  'config.savedLocalOfflineHint': 'Config saved locally, but device is offline. Reconnect and save again to apply',
  'config.hardwareApplyFailed': 'Config saved, but failed to apply to hardware: {reason}',
  'config.saved': 'Config saved',
  'config.saveFailed': 'Save failed',
  'config.error.nameExists': 'Device name is already used by another device. Please rename and retry.',

  // ---- scan ----
  'scan.scanning': 'Scanning...',
  'scan.noDevices': 'No devices found',
  'scan.noDevicesHint': 'Make sure the device is powered on and connected to the same network',
  'scan.added': 'Added',
  'scan.deviceAddedTitle': 'Device already added',
  'scan.addThisDevice': 'Add this device',
  'scan.discovered': '{n} devices found',

  // ---- chart ----
  'chart.waitingData': 'Waiting for real-time data...',
  'chart.noChannelSelected': 'No channel selected',
  'chart.willShowWhenAcquiring': 'Waveform will appear once acquisition starts',
  'chart.pleaseSelectChannel': 'Please select channels in the channel selection above',

  // ---- dialog ----
  'dialog.scanTitle': 'Scan Devices',
  'dialog.scanSubtitle': 'Discover DAQ-T-1603 devices on the LAN',
  'dialog.addDeviceTitle': 'Add T1603 Device',
  'dialog.addDeviceSubtitle': 'Connect to a temperature acquisition device via IP and port',
  'dialog.deviceName': 'Device Name',
  'dialog.deviceNamePlaceholder': 'e.g., Temperature DAQ 1',
  'dialog.ipAddress': 'IP Address',
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
  'log.levelAndAbove': '(and above)',
  'log.levelDebug': 'Debug',
  'log.levelInfo': 'Info',
  'log.levelWarn': 'Warn',
  'log.levelError': 'Error',
  'log.category': 'Category',
  'log.allGroups': 'All',
  'log.empty': 'No logs',
  'log.searchPlaceholder': 'Search logs (message/tag/detail/device)',
  'log.clearSearch': 'Clear search',
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
  'error.duplicateDevice': 'This device has already been added',
  'error.recordingDirNotFound': 'Save directory does not exist: {dir}, please choose a valid one',
  'error.recordingPermissionDenied': 'No permission to write to directory: {dir}, please check permissions',
  'error.recordingStartFailed': 'Failed to start recording: {reason}',

  // ---- logMessage ----
  'logMessage.autoConnectFailed': 'Some devices failed to auto-connect: {message}',
  'logMessage.syncRefreshRateFailed': 'Failed to sync refresh rate to backend: {message}',
  'logMessage.recordingFatal': 'Recording fatal error [{deviceId}]: {error}',
  'logMessage.recordingBackpressure': 'Recording backpressure dropped [{deviceId}]: queue {queueLen}/{queueCap}',
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
  storageKey: 'daq-t1603.locale',
})
