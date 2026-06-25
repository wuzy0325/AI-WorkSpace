// Wails API 适配器 - 统一封装 Wails v3 生成绑定，避免业务层感知绑定路径变化

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
}

export interface DeviceStatus {
  id: string;
  name: string;
  type: string;
  connection: string;
  acquiring: boolean;
  lastError?: string;
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
  results: any[];
  lastError?: string;
}

export interface StorageRecordingStatus {
  recording: boolean;
  outputDir?: string;
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
  return await import('../../bindings/wind-daq/apps/desktop-wails/backend/app.js');
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
  return { success, error, Success: success, Error: error };
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
      return await callBinding('DeviceGetStatus', deviceId);
    },
    startAcquisition: async (deviceId: string): Promise<GenericResponse> => {
      return await callBindingGeneric('DeviceStartAcquisition', deviceId);
    },
    stopAcquisition: async (deviceId: string): Promise<GenericResponse> => {
      return await callBindingGeneric('DeviceStopAcquisition', deviceId);
    },
    getLatestData: async (deviceId: string): Promise<DeviceDataPayload | boolean> => {
      return await callBinding('DeviceGetLatestData', deviceId);
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
        try {
          const resp = await fetch(STATUS_API);
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
          // 网络错误（如主进程尚未启动 HTTP server）静默重试，避免日志噪音
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
      return await callBinding('CalibrationGetResult', taskId);
    },
    saveCsv: async (taskId: string, savePath: string): Promise<FileResponse> => {
      const raw = await callBinding<unknown>('CalibrationSaveCsv', taskId, savePath);
      const normalized = normalizeGenericResponse(raw);
      const obj = (raw ?? {}) as Record<string, unknown>;
      const filepath = (obj.filepath ?? obj.Filepath) as string | undefined;
      return { ...normalized, filepath, Filepath: filepath };
    }
  },

  // 存储管理
  storage: {
    startRecording: async (outputDir: string, filePrefix: string): Promise<GenericResponse> => {
      return await callBindingGeneric('StorageStartRecording', outputDir, filePrefix);
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
    // 获取启动模式："normal" 或 "motion"
    getStartupMode: async (): Promise<string> => {
      return await callBinding('GetStartupMode');
    },
    // 启动运动控制器独立窗口（独立进程）
    openMotionWindow: async (): Promise<GenericResponse> => {
      return await callBindingGeneric('OpenMotionWindow');
    }
  }
};
