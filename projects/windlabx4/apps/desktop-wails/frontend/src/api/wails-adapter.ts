// Wails API 适配器 - 统一封装 Wails v3 生成绑定，避免业务层感知绑定路径变化

// 修复（2026-08-01）：静态导入 Wails runtime 事件模块。
// 历史实现动态 import('@wailsio/runtime')，若 chunk 加载失败会导致
// onExitRequested 监听永不注册，用户点 X 后确认框不弹出、应用无法退出。
import { Events } from '@wailsio/runtime';

// 类型定义（与 Wails 生成的 models.ts 保持一致）
// 注意：Wails 实际通过 JSON tag 序列化为小写 success/error，
// 但历史代码中存在大量 `result.Success` / `res.Error` 的大写读法（来自此文件之前的错误手写类型）。
// 为了同时兼容新旧调用点，这里同时声明大小写两套字段，并由 callBindingGeneric() 在运行时镜像填充。
export interface GenericResponse {
  success: boolean;
  error?: string;
  /** @deprecated Wails 实际返回小写 success；保留 Success 仅用于兼容旧调用点 */
  Success: boolean;
  /** @deprecated Wails 实际返回小写 error；保留 Error 仅用于兼容旧调用点 */
  Error?: string;
  /**
   * 返回数据载荷（与后端 GenericResponse.Data 对齐，spec Task 13 起新增）。
   *
   * 仅在需要返回数据的 binding（如 CalibrationPreviewSevenHole 返回点位预览结果）中填充；
   * 简单成功/失败响应不填此字段，运行时 omitempty 省略。
   *
   * 类型由调用方按需断言（如 `res.Data as SevenHolePreviewResult`），
   * 此处保留 unknown 以保持 GenericResponse 通用性。
   */
  data?: unknown;
  /** @deprecated 与 data 同义，保留 Data 仅为兼容大写字段旧调用点 */
  Data?: unknown;
}

export interface FileResponse extends GenericResponse {
  filepath?: string;
  Filepath?: string;
}

export interface VersionInfo {
  name: string;
  version: string;
}

export interface DeviceProfile {
  id: string;
  name: string;
  type: string;
  samplingRate: number;
  channels: any[];
  address?: string;
  localAddress?: string;
}

export interface DeviceStatus {
  id: string;
  name: string;
  type: string;
  connection: string;
  acquiring: boolean;
  lastError?: string;
}

type BoolResult<T> = T | [T, boolean] | boolean;

function unwrapBoolResult<T>(result: BoolResult<T>): T | boolean {
  if (Array.isArray(result)) {
    const [value, ok] = result;
    return ok ? value : false;
  }
  return result;
}

export interface DeviceScanResult {
  id: string;
  name: string;
  type: string;
  available: boolean;
  address?: string;
  port?: number;
  macAddress?: string;
  serialNumber?: string;
  firmwareVersion?: string;
}

export interface DeviceDataPayload {
  deviceId: string;
  timestamp: number;
  channels: number[];
  channelIndices: number[];
}

export interface MotionProfile {
  id: string;
  name: string;
  type: string;
  address: string;
  port: number;
  autoConnect: boolean;
  axes: any[];
}

export interface MotionStatus {
  id: string;
  name: string;
  type: string;
  connected: boolean;
  emergencyStopped: boolean;
  axes: any[];
  lastError?: string;
}

export interface CalibrationConfig {
  taskId?: string;
  deviceId?: string;
  type: string;
  channels?: number[];
  pressurePoints?: number[];
  averageSamples?: number;
  probeChannels?: any[];
  points?: any[];
  samplesPerPoint?: number;
  dwellTimeMs?: number;
  stopOnError?: boolean;
}

export interface CalibrationStatus {
  taskId: string;
  state: string;
  currentPoint: number;
  totalPoints: number;
  pausedDurationMs?: number;
  results: any[];
  lastError?: string;
  /**
   * 实时物理量快照（spec Task 13/14，与后端 core/calibration.LivePhysics 对齐）。
   *
   * Wails binding 重新生成后会自动包含此字段；旧 binding 未同步时为 undefined。
   *
   * 三态语义（与 *float64 指针语义一致）：
   *   - 字段省略（undefined）：缺失（必需通道未配置/读取失败/物理非法如 Pt < Ps）
   *   - 字段值为 0：有效零（Pt == Ps 即零流量，Task 12）
   *   - 字段值为正数：正常计算值
   *
   * 整体 livePhysics 省略（undefined）：类型不支持（总温）或未启动校准。
   * 整体存在但字段省略：类型支持但运行期读取失败。
   *
   * 关键不变量：前端不得用 truthiness 判断 `livePhysics.machNumber`——0 是有效零。
   */
  livePhysics?: {
    machNumber?: number;
    velocity?: number;
  };
}

// SevenHoleConfig 与 shared/types/calibration.SevenHoleConfig 对齐，
// 用于 CalibrationPreviewSevenHole binding 调用——透传到后端 GenerateSevenHolePoints。
// 字段命名遵循后端 json tag（SevenHoleConfigDTO 直接是 calibration.SevenHoleConfig 别名）。
export interface SevenHoleConfig {
  mode: string; // 'full' | 'dataset'
  innerAlphaMin: number;
  innerAlphaMax: number;
  innerAlphaStep: number;
  innerBetaMin: number;
  innerBetaMax: number;
  innerBetaStep: number;
  outerThetaMin: number;
  outerThetaMax: number;
  outerThetaStep: number;
  outerPhiMin: number;
  outerPhiMax: number;
  outerPhiStep: number;
  serpentine: boolean;
}

// FiveHolePointLayout 与 shared/types/calibration.FiveHolePointLayout 对齐，
// 用于 CalibrationPreviewFiveHole binding 调用（spec Task 11）——
// 透传到后端 usecase.PreviewFiveHolePoints（再调 core.GenerateFiveHoleSnakePoints）。
// 字段命名遵循后端 json tag（FiveHolePointLayoutDTO 直接是 calibration.FiveHolePointLayout 别名）。
export interface FiveHolePointLayout {
  alphaMin: number;
  alphaMax: number;
  alphaStep: number;
  betaMin: number;
  betaMax: number;
  betaStep: number;
  /** 蛇形走位：奇数行反向遍历 α；默认 false 为逐行 raster 扫描 */
  serpentine?: boolean;
}

// StorageRecordingConfig 与后端 storage.RecordingConfig 对齐，
// 由 UI 在 Start 时传入，贯穿 usecase -> sink。
// 字段命名遵循后端 json tag（与 api/server.go 直接 Decode 进 storage.RecordingConfig 一致）。
export interface StorageRecordingConfig {
  outputDir: string;
  filePrefix: string;
  /** 存储格式："csv"（默认）或 "binary"；为空时由装配层默认值决定 */
  format?: string;
  /** 自动停止条件，全部为零值表示不限制 */
  stopConditions?: {
    maxDurationMs?: number;
    maxFileSizeBytes?: number;
    maxRecordCount?: number;
  };
  /** 文件滚动配置 */
  fileRotation?: {
    enabled: boolean;
    maxFileSizeBytes: number;
    maxDurationMs: number;
  };
  /** 是否在采集启动时自动开始录制（仅由编排层消费，sink 不消费） */
  autoStartOnAcquisition?: boolean;
}

// StorageRecordingStatus 与后端 storage.RecordingStatus 对齐，
// 包含运行时统计字段（文件、大小、记录数、丢弃数、错误等）。
export interface StorageRecordingStatus {
  recording: boolean;
  outputDir?: string;
  /** 当前写入的文件名（不含目录） */
  currentFile?: string;
  /** 当前文件累计字节数 */
  fileSize?: number;
  /** 本会话已滚动的文件数（含当前文件，从 1 开始） */
  fileCount?: number;
  /** 本会话累计写入的记录条数 */
  recordCount?: number;
  /** 本会话累计录制时长（毫秒） */
  durationMs?: number;
  /** 因队列满被丢弃的 payload 数 */
  droppedCount?: number;
  /** 最近一次 I/O 错误描述（空表示无错误） */
  lastError?: string;
}

export interface ReportStatus {
  generating: boolean;
  lastResult?: string;
}

export interface TraversalPoint {
  x: number;
  y: number;
  z: number;
}

export interface TraversalStatus {
  state: string;
  currentPoint: number;
  totalPoints: number;
}

// DSA3217 扫描配置响应
export interface DSA3217ScanConfigWailsResponse {
  Success: boolean;
  Data?: {
    Avg: number;
    Period: number;
    Fps: string;
    Unit: string;
  };
  Error?: string;
}

// 配置加载结果
export interface ConfigLoadResult<T = unknown> {
  success: boolean;
  data?: T;
  error?: string;
}

// 运行时获取 Wails API 绑定
async function getWailsBinding(): Promise<any> {
  // 仅在真实 Wails 环境下加载 v3 runtime/bindings，避免浏览器预览和单测被 runtime fetch 副作用污染。
  if (!isWailsAvailable()) return null;
  // @ts-expect-error Wails v3 alpha generates JS bindings without TypeScript declarations.
  return await import('../../bindings/windlabx4/apps/desktop-wails/backend/app.js');
}

// 检查 Wails 环境是否可用
export const isWailsAvailable = (): boolean => {
  if (typeof window === 'undefined') return false;
  const w = window as any;
  return Boolean(
    w.chrome?.webview?.postMessage ||
    w.webkit?.messageHandlers?.external?.postMessage ||
    w.wails?.invoke,
  );
};

// 通用调用包装器
async function callBinding<T>(methodName: string, ...args: any[]): Promise<T> {
  const binding = await getWailsBinding();
  if (!binding || !binding[methodName]) {
    throw new Error(`Wails binding '${methodName}' not available`);
  }
  return await binding[methodName](...args);
}

/**
 * 归一化 Wails GenericResponse 的大小写差异。
 *
 * Wails 自动生成的 TypeScript model 字段是小写 success/error（来自 Go 的 json:"success" tag），
 * 但项目内不少调用点写成了 `res.Success`/`res.Error`（大写）。如果不归一化，这些旧调用点
 * 永远拿到 undefined，例如打开运动控制器独立窗口时，后端 success=true 但前端
 * `!res.Success` 永远为真，导致"启动独立窗口失败"误报。
 *
 * 这里统一在 callBindingGeneric 出口把两套字段同时填充，保持向后兼容。
 */
function normalizeGenericResponse(raw: unknown): GenericResponse {
  const obj = (raw ?? {}) as Record<string, unknown>;
  const success = (obj.success ?? obj.Success ?? false) as boolean;
  const error = (obj.error ?? obj.Error) as string | undefined;
  // Data 字段双写：Wails JSON 反序列化后是小写 data，这里同时填充 data/Data
  // 以便调用方按需使用任意大小写读法（与 success/Success 双写策略一致）
  const data = (obj.data ?? obj.Data) as unknown;
  return { success, error, Success: success, Error: error, data, Data: data };
}

async function callBindingGeneric(methodName: string, ...args: any[]): Promise<GenericResponse> {
  const raw = await callBinding<unknown>(methodName, ...args);
  return normalizeGenericResponse(raw);
}

// Wails API 适配器
export const wailsApi = {
  // 设备管理
  device: {
    getProfiles: async (): Promise<DeviceProfile[]> => {
      return await callBinding('DeviceGetProfiles');
    },
    upsertProfile: async (profile: DeviceProfile): Promise<GenericResponse> => {
      return await callBindingGeneric('DeviceUpsertProfile', profile);
    },
    deleteProfile: async (profileId: string): Promise<GenericResponse> => {
      return await callBindingGeneric('DeviceDeleteProfile', profileId);
    },
    scanDevices: async (): Promise<DeviceScanResult[]> => {
      return (await callBinding('DeviceScanDevices')) as DeviceScanResult[];
    },
    connect: async (deviceId: string): Promise<GenericResponse> => {
      return await callBindingGeneric('DeviceConnect', deviceId);
    },
    disconnect: async (deviceId: string): Promise<GenericResponse> => {
      return await callBindingGeneric('DeviceDisconnect', deviceId);
    },
    getStatus: async (deviceId: string): Promise<DeviceStatus | boolean> => {
      return unwrapBoolResult(await callBinding<BoolResult<DeviceStatus>>('DeviceGetStatus', deviceId));
    },
    startAcquisition: async (deviceId: string): Promise<GenericResponse> => {
      return await callBindingGeneric('DeviceStartAcquisition', deviceId);
    },
    stopAcquisition: async (deviceId: string): Promise<GenericResponse> => {
      return await callBindingGeneric('DeviceStopAcquisition', deviceId);
    },
    getLatestData: async (deviceId: string): Promise<DeviceDataPayload | boolean> => {
      return unwrapBoolResult(await callBinding<BoolResult<DeviceDataPayload>>('DeviceGetLatestData', deviceId));
    },
    setPublishRate: async (rate: number): Promise<GenericResponse> => {
      return await callBindingGeneric('DeviceSetPublishRate', rate);
    },
    getPublishRate: async (): Promise<number> => {
      return await callBinding('DeviceGetPublishRate');
    },
    subscribeStream: async (deviceId: string, subscribe: boolean): Promise<GenericResponse> => {
      return await callBindingGeneric('DeviceSubscribeStream', deviceId, subscribe);
    },
    onPayload: (callback: (payload: DeviceDataPayload) => void): (() => void) => {
      if (!isWailsAvailable()) return () => {};

      let cleanup: (() => void) | null = null;
      let active = true;

      void import('@wailsio/runtime').then(({ Events }) => {
        if (!active) return;
        cleanup = Events.On('daq:payload', (event: { data: unknown }) => {
          const data = event.data
          if (data == null) return
          const raw = data as Record<string, unknown>
          const normalized: DeviceDataPayload = {
            deviceId: typeof raw.deviceId === 'string' ? raw.deviceId : '',
            timestamp: typeof raw.timestamp === 'number' ? raw.timestamp : 0,
            channels: Array.isArray(raw.channels) ? raw.channels as number[] : [],
            channelIndices: Array.isArray(raw.channelIndices) ? raw.channelIndices as number[] : [],
          }
          callback(normalized)
        });
      });
      return () => {
        active = false;
        cleanup?.();
      };
    },
    getDsa3217ScanConfig: async (deviceId: string): Promise<DSA3217ScanConfigWailsResponse> => {
      return await callBinding('DeviceGetDsa3217ScanConfig', deviceId);
    },
    applyDsa3217ScanConfig: async (deviceId: string, avg: number, period: number): Promise<DSA3217ScanConfigWailsResponse> => {
      return await callBinding('DeviceApplyDsa3217ScanConfig', deviceId, avg, period);
    }
  },

  // 运动控制
  motion: {
    getProfiles: async (): Promise<MotionProfile[]> => {
      const raw = await callBinding<string>('MotionGetProfiles');
      try {
        return JSON.parse(raw) as MotionProfile[];
      } catch {
        return [];
      }
    },
    upsertProfile: async (profile: MotionProfile): Promise<GenericResponse> => {
      return await callBindingGeneric('MotionUpsertProfile', profile);
    },
    deleteProfile: async (profileId: string): Promise<GenericResponse> => {
      return await callBindingGeneric('MotionDeleteProfile', profileId);
    },
    connect: async (controllerId: string): Promise<GenericResponse> => {
      return await callBindingGeneric('MotionConnect', controllerId);
    },
    disconnect: async (controllerId: string): Promise<GenericResponse> => {
      return await callBindingGeneric('MotionDisconnect', controllerId);
    },
    getStatus: async (): Promise<MotionStatus[]> => {
      const raw = await callBinding<string>('MotionGetStatus');
      try {
        return JSON.parse(raw) as MotionStatus[];
      } catch {
        return [];
      }
    },
    home: async (controllerId: string, axisName: string): Promise<GenericResponse> => {
      return await callBindingGeneric('MotionHome', controllerId, axisName);
    },
    moveTo: async (controllerId: string, axisName: string, position: number): Promise<GenericResponse> => {
      return await callBindingGeneric('MotionMoveTo', controllerId, axisName, position);
    },
    moveBy: async (controllerId: string, axisName: string, distance: number): Promise<GenericResponse> => {
      return await callBindingGeneric('MotionMoveBy', controllerId, axisName, distance);
    },
    jog: async (controllerId: string, axisName: string, velocity: number): Promise<GenericResponse> => {
      return await callBindingGeneric('MotionJog', controllerId, axisName, velocity);
    },
    stop: async (controllerId: string, axisName: string): Promise<GenericResponse> => {
      return await callBindingGeneric('MotionStop', controllerId, axisName);
    },
    emergencyStop: async (controllerId: string): Promise<GenericResponse> => {
      return await callBindingGeneric('MotionEmergencyStop', controllerId);
    },
    definePosition: async (controllerId: string, axisName: string, position: number): Promise<GenericResponse> => {
      return await callBindingGeneric('MotionDefinePosition', controllerId, axisName, position);
    },
    resetEmergencyStop: async (controllerId: string): Promise<GenericResponse> => {
      return await callBindingGeneric('MotionResetEmergencyStop', controllerId);
    },

    onStatusEvent: (callback: (data: unknown) => void): (() => void) => {
      // 改为 HTTP 轮询主进程本地 API（127.0.0.1:8900/api/motion/status）。
      //
      // 背景：Wails v2.12.0 通过 EventsEmit 推送 MotionControllerStatus[] 时，
      // 内部对 interface{} 切片做反射序列化，遇到具名 string 类型（ControllerType /
      // AxisName）的切片会错误调用 reflect.Value.IsNil()，触发 panic：
      //   "reflect: call of reflect.Value.IsNil on string Value"
      // B140 点动场景下 100% 复现。
      //
      // 与 motion-controller 项目保持一致的修复策略：后端不再 EventsEmit，
      // 前端通过 HTTP 主动拉取，由 Go 标准库 encoding/json 直接编码到 ResponseWriter，
      // 完全绕开 Wails 的反射桥。
      //
      // 轮询节奏：运动中 250ms 高频，空闲 2000ms 慢速心跳，避免空载时的 CPU 浪费。
      const STATUS_API = 'http://127.0.0.1:8900/api/motion/status';
      const FAST_INTERVAL_MS = 250;
      const SLOW_INTERVAL_MS = 2000;
      let active = true;
      let currentInterval = FAST_INTERVAL_MS;
      let timer: ReturnType<typeof setTimeout> | null = null;

      const scheduleNext = (delay: number): void => {
        if (!active) return;
        timer = setTimeout(() => { void poll(); }, delay);
      };

      const poll = async (): Promise<void> => {
        if (!active) return;
        // 加 2.5s 超时：避免某个控制器卡顿（B140 单命令超时 5s）时 fetch 长时间挂起，
        // 拖停整个轮询链导致 UI 长时间不刷新。超时后立即进入下一轮重试。
        // 阈值 2500ms 容纳 B140 四轴 14 次 TCP 往返的正常耗时，避免误超时。
        const controller = new AbortController();
        const timeoutId = setTimeout(() => controller.abort(), 2500);
        try {
          const resp = await fetch(STATUS_API, { signal: controller.signal });
          if (resp.ok) {
            const statuses = await resp.json();
            if (active && Array.isArray(statuses)) {
              try {
                callback(statuses);
              } catch (err) {
                console.error('[wails-adapter] motion status callback threw:', err);
              }
              // 动态调速：任一轴在运动则保持高频，否则降为慢速心跳
              const hasMoving = (statuses as Array<{ axes?: Array<{ moving?: boolean }> }>)
                .some((s) => Array.isArray(s.axes) && s.axes.some((a) => a.moving === true));
              currentInterval = hasMoving ? FAST_INTERVAL_MS : SLOW_INTERVAL_MS;
            }
          }
        } catch {
          // 网络错误或超时（如主进程尚未启动 HTTP server、某控制器卡顿）静默重试，避免日志噪音
        } finally {
          clearTimeout(timeoutId);
        }
        scheduleNext(currentInterval);
      };

      // 立即触发首次拉取，无需等待轮询周期
      void poll();

      return () => {
        active = false;
        if (timer !== null) {
          clearTimeout(timer);
          timer = null;
        }
      };
    }
  },

  // 校准管理
  calibration: {
    start: async (config: CalibrationConfig): Promise<GenericResponse> => {
      return await callBindingGeneric('CalibrationStart', config);
    },
    stop: async (): Promise<GenericResponse> => {
      return await callBindingGeneric('CalibrationStop');
    },
    pause: async (): Promise<GenericResponse> => {
      return await callBindingGeneric('CalibrationPause');
    },
    resume: async (): Promise<GenericResponse> => {
      return await callBindingGeneric('CalibrationResume');
    },
    collect: async (): Promise<GenericResponse> => {
      return await callBindingGeneric('CalibrationCollect');
    },
    status: async (): Promise<CalibrationStatus> => {
      return await callBinding('CalibrationStatus');
    },
    getResult: async (taskId: string): Promise<CalibrationStatus | boolean> => {
      return unwrapBoolResult(await callBinding<BoolResult<CalibrationStatus>>('CalibrationGetResult', taskId));
    },
    saveCsv: async (taskId: string, savePath: string): Promise<FileResponse> => {
      const raw = await callBinding<unknown>('CalibrationSaveCsv', taskId, savePath);
      const normalized = normalizeGenericResponse(raw);
      const obj = (raw ?? {}) as Record<string, unknown>;
      const filepath = (obj.filepath ?? obj.Filepath) as string | undefined;
      return { ...normalized, filepath, Filepath: filepath };
    },
    /**
     * 七孔点位预览（spec Task 13/18）
     *
     * 调用后端 CalibrationPreviewSevenHole binding，纯计算不涉及 I/O：
     *   - 接收前端配置向导提交的 SevenHoleConfig（α/β/θ/φ 范围与步长）
     *   - 调用 usecase.PreviewSevenHolePoints 生成完整点位 + 内/外区聚合统计
     *   - 返回 SevenHolePreviewResult（points + totalCount + innerCount + outerCount）
     *
     * 返回 GenericResponse：Success=false 时 Error 透传 GenerateSevenHolePoints 错误
     * （如步长 ≤ 0、范围 min > max）；Success=true 时 Data 字段为 SevenHolePreviewResult。
     */
    previewSevenHole: async (config: SevenHoleConfig): Promise<GenericResponse> => {
      return await callBindingGeneric('CalibrationPreviewSevenHole', config);
    },
    /**
     * 五孔点位预览（spec Task 11）
     *
     * 调用后端 CalibrationPreviewFiveHole binding，纯计算不涉及 I/O：
     *   - 接收前端配置向导提交的 FiveHolePointLayout（α/β 范围与步长 + serpentine 开关）
     *   - 调用 usecase.PreviewFiveHolePoints（再调 core.GenerateFiveHoleSnakePoints）
     *   - 返回 []FiveHoleSnakePoint（bare array，与 HTTP /api/calibration/fivehole 契约一致）
     *
     * 返回 GenericResponse：Success=false 时 Error 透传 GenerateFiveHoleSnakePoints 错误
     * （如步长 ≤ 0）；Success=true 时 Data 字段为 []FiveHoleSnakePoint。
     *
     * 与 previewSevenHole 的区别：
     *   - 五孔 Data 是 bare array（[]FiveHoleSnakePoint），前端直接迭代
     *   - 七孔 Data 是包装对象（SevenHolePreviewResult，含 totalCount/innerCount/outerCount）
     */
    previewFiveHole: async (layout: FiveHolePointLayout): Promise<GenericResponse> => {
      return await callBindingGeneric('CalibrationPreviewFiveHole', layout);
    }
  },

  // 存储管理
  storage: {
    // startRecording 接收完整 RecordingConfig，路径解析由后端统一完成。
    // Wails v3 会按 Go 结构体 json tag 反序列化前端传入的对象。
    startRecording: async (config: StorageRecordingConfig): Promise<GenericResponse> => {
      return await callBindingGeneric('StorageStartRecording', config);
    },
    stopRecording: async (): Promise<GenericResponse> => {
      return await callBindingGeneric('StorageStopRecording');
    },
    getStatus: async (): Promise<StorageRecordingStatus> => {
      return await callBinding('StorageGetStatus');
    }
  },

  // 报告管理
  report: {
    getStatus: async (): Promise<ReportStatus> => {
      return await callBinding('ReportGetStatus');
    }
  },

  // 配置管理
  config: {
    load: async <T = unknown>(key: string): Promise<ConfigLoadResult<T>> => {
      try {
        const result = await callBinding<ConfigLoadResult<T>>('ConfigLoad', key);
        return result;
      } catch (e) {
        return { success: false, error: String(e) };
      }
    },
    save: async (key: string, config: unknown): Promise<GenericResponse> => {
      try {
        return await callBindingGeneric('ConfigSave', key, JSON.stringify(config));
      } catch (e) {
        return normalizeGenericResponse({ success: false, error: String(e) });
      }
    },
  },

  // 应用通用
  app: {
    getVersion: async (): Promise<VersionInfo> => {
      return await callBinding('GetVersion');
    },
    pickDirectory: async (): Promise<string> => {
      return await callBinding('PickDirectory');
    },
    resolvePath: async (p: string): Promise<string> => {
      return await callBinding('ResolvePath', p);
    },
    pickFile: async (title: string, filters: Array<{ displayName: string; pattern: string }>): Promise<string> => {
      return await callBinding('PickFile', title, filters);
    },
    pickFiles: async (title: string, filters: Array<{ displayName: string; pattern: string }>): Promise<string[]> => {
      return await callBinding('PickFiles', title, filters);
    },
    pickSaveFile: async (
      title: string,
      defaultFilename: string,
      filters: Array<{ displayName: string; pattern: string }>,
    ): Promise<string> => {
      return await callBinding('PickSaveFile', title, defaultFilename, filters);
    },
    fileExists: async (path: string): Promise<boolean> => {
      return await callBinding('FileExists', path);
    },
    removeFile: async (path: string): Promise<boolean> => {
      return await callBinding('RemoveFile', path);
    },
    // 获取启动模式："normal" 或 "motion"
    getStartupMode: async (): Promise<string> => {
      return await callBinding('GetStartupMode');
    },
    // 获取安装程序写入的语言偏好：返回 "zh"、"en" 或空字符串
    getInstallerLanguage: async (): Promise<string> => {
      return await callBinding('GetInstallerLanguage');
    },
    // 启动运动控制器独立窗口（独立进程）
    openMotionWindow: async (): Promise<GenericResponse> => {
      return await callBindingGeneric('OpenMotionWindow');
    },
    /**
     * 请求退出应用：用户在确认对话框中点击"退出"后调用。
     * 后端置 userConfirmedExit=true 后通过 application.Quit() 触发完整关闭流程
     * （cleanup → ServiceShutdown → 关闭所有窗口 → 最后一个窗口关闭后退出消息循环）。
     */
    requestExit: async (): Promise<GenericResponse> => {
      return await callBindingGeneric('RequestExit');
    },
    /**
     * 监听后端推送的退出请求事件（用户点 X 按钮触发 WindowClosing hook 时推送）。
     * 调用方应在回调中弹出确认对话框，根据用户选择决定是否调用 requestExit()。
     * 返回取消监听函数。
     */
    onExitRequested: (callback: () => void): (() => void) => {
      if (!isWailsAvailable()) return () => {};
      let cleanup: (() => void) | null = null;
      let active = true;
      try {
        cleanup = Events.On('app:exit-requested', () => {
          callback()
        })
      } catch (err) {
        console.error('[exit] register onExitRequested failed:', err)
      }
      return () => {
        active = false;
        cleanup?.();
      };
    },
  }
};
