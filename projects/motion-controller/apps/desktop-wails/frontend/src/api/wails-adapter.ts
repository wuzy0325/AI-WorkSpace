// Wails API 适配器 - 运行时动态加载绑定，避免 Vite 构建时模块解析问题
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { MOTION_HTTP_BASE } from './motionApi'

// 类型定义（与 Wails 生成的 models.ts 保持一致）
export interface GenericResponse {
  Success: boolean;
  Error?: string;
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

// 运行时获取 Wails API 绑定
function getWailsBinding(): any {
  if (typeof window !== 'undefined' && (window as any).go && (window as any).go.backend && (window as any).go.backend.App) {
    return (window as any).go.backend.App;
  }
  return null;
}

// 检查 Wails 环境是否可用
export const isWailsAvailable = (): boolean => {
  return getWailsBinding() !== null;
};

// 通用调用包装器
async function callBinding<T>(methodName: string, ...args: any[]): Promise<T> {
  const binding = getWailsBinding();
  if (!binding || !binding[methodName]) {
    throw new Error(`Wails binding '${methodName}' not available`);
  }
  try {
    // Wails v2 绑定函数期望参数直接传递（运行时会自动序列化）
    const result = await binding[methodName](...args);
    return result;
  } catch (e) {
    console.error(`[wails-adapter] callBinding('${methodName}') failed`, args, e);
    throw e;
  }
}

// Wails API 适配器
export const wailsApi = {
  // 设备管理
  device: {
    getProfiles: async (): Promise<DeviceProfile[]> => {
      return await callBinding('DeviceGetProfiles');
    },
    upsertProfile: async (profile: DeviceProfile): Promise<GenericResponse> => {
      return await callBinding('DeviceUpsertProfile', profile);
    },
    deleteProfile: async (profileId: string): Promise<GenericResponse> => {
      return await callBinding('DeviceDeleteProfile', profileId);
    },
    scanDevices: async (): Promise<DeviceScanResult[]> => {
      return (await callBinding('DeviceScanDevices')) as DeviceScanResult[];
    },
    connect: async (deviceId: string): Promise<GenericResponse> => {
      return await callBinding('DeviceConnect', deviceId);
    },
    disconnect: async (deviceId: string): Promise<GenericResponse> => {
      return await callBinding('DeviceDisconnect', deviceId);
    },
    getStatus: async (deviceId: string): Promise<DeviceStatus | boolean> => {
      return await callBinding('DeviceGetStatus', deviceId);
    },
    startAcquisition: async (deviceId: string): Promise<GenericResponse> => {
      return await callBinding('DeviceStartAcquisition', deviceId);
    },
    stopAcquisition: async (deviceId: string): Promise<GenericResponse> => {
      return await callBinding('DeviceStopAcquisition', deviceId);
    },
    getLatestData: async (deviceId: string): Promise<DeviceDataPayload | boolean> => {
      return await callBinding('DeviceGetLatestData', deviceId);
    },
    setPublishRate: async (rate: number): Promise<GenericResponse> => {
      return await callBinding('DeviceSetPublishRate', rate);
    },
    getPublishRate: async (): Promise<number> => {
      return await callBinding('DeviceGetPublishRate');
    },
    subscribeStream: async (deviceId: string, subscribe: boolean): Promise<GenericResponse> => {
      return await callBinding('DeviceSubscribeStream', deviceId, subscribe);
    },
    onPayload: (callback: (payload: DeviceDataPayload) => void): (() => void) => {
      const cleanup = EventsOn('daq:payload', (data: unknown) => {
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
      return () => {
        cleanup();
      };
    },
    getDsa3217ScanConfig: async (deviceId: string): Promise<DSA3217ScanConfigWailsResponse> => {
      return await callBinding('DeviceGetDsa3217ScanConfig', deviceId);
    },
    applyDsa3217ScanConfig: async (deviceId: string, avg: number, period: number): Promise<DSA3217ScanConfigWailsResponse> => {
      return await callBinding('DeviceApplyDsa3217ScanConfig', deviceId, avg, period);
    }
  },

  // 运动控制（Wails 绑定返回 Promise<void>，error 时 reject）
  // 注意：MotionGetProfiles 和 MotionGetStatus 返回 JSON 字符串（绕开 Wails reflect bug），
  // 需要在前端手动 JSON.parse
  motion: {
    getProfiles: async (): Promise<MotionProfile[]> => {
      const raw = await callBinding<string>('MotionGetProfiles');
      try {
        return JSON.parse(raw) as MotionProfile[];
      } catch {
        return [];
      }
    },
    upsertProfile: async (profile: MotionProfile): Promise<void> => {
      await callBinding('MotionUpsertProfile', profile);
    },
    deleteProfile: async (profileId: string): Promise<void> => {
      await callBinding('MotionDeleteProfile', profileId);
    },
    connect: async (controllerId: string): Promise<void> => {
      await callBinding('MotionConnect', controllerId);
    },
    disconnect: async (controllerId: string): Promise<void> => {
      await callBinding('MotionDisconnect', controllerId);
    },
    getStatus: async (): Promise<MotionStatus[]> => {
      const raw = await callBinding<string>('MotionGetStatus');
      try {
        return JSON.parse(raw) as MotionStatus[];
      } catch {
        return [];
      }
    },
    home: async (controllerId: string, axisName: string): Promise<void> => {
      await callBinding('MotionHome', controllerId, axisName);
    },
    moveTo: async (controllerId: string, axisName: string, position: number): Promise<void> => {
      await callBinding('MotionMoveTo', controllerId, axisName, position);
    },
    moveBy: async (controllerId: string, axisName: string, distance: number): Promise<void> => {
      await callBinding('MotionMoveBy', controllerId, axisName, distance);
    },
    jog: async (controllerId: string, axisName: string, velocity: number): Promise<void> => {
      await callBinding('MotionJog', controllerId, axisName, velocity);
    },
    stop: async (controllerId: string, axisName: string): Promise<void> => {
      await callBinding('MotionStop', controllerId, axisName);
    },
    emergencyStop: async (controllerId: string): Promise<void> => {
      await callBinding('MotionEmergencyStop', controllerId);
    },
    resetEmergencyStop: async (controllerId: string): Promise<void> => {
      await callBinding('MotionResetEmergencyStop', controllerId);
    },
    definePosition: async (controllerId: string, axisName: string, position: number): Promise<void> => {
      await callBinding('MotionDefinePosition', controllerId, axisName, position);
    },

    onStatusEvent: (callback: (data: unknown) => void): (() => void) => {
      // 绕开 Wails v2.12.0 reflect 序列化 bug，通过 HTTP API 轮询状态
      let active = true;
      let errorCount = 0;
      const poll = async () => {
        if (!active) return;
        try {
          const resp = await fetch(`${MOTION_HTTP_BASE}/api/motion/status`);
          if (resp.ok) {
            const statuses = await resp.json();
            if (active) callback(statuses);
          }
        } catch (e) {
          errorCount++;
          if (errorCount <= 10 || errorCount % 20 === 0) {
            console.error('[wails-adapter] onStatusEvent poll error', { errorCount, error: e })
          }
        }
        if (active) {
          setTimeout(poll, 200);
        }
      };
      poll();
      return () => {
        active = false;
      };
    }
  },

  // 校准管理
  calibration: {
    start: async (config: CalibrationConfig): Promise<GenericResponse> => {
      return await callBinding('CalibrationStart', config);
    },
    stop: async (): Promise<GenericResponse> => {
      return await callBinding('CalibrationStop');
    },
    pause: async (): Promise<GenericResponse> => {
      return await callBinding('CalibrationPause');
    },
    resume: async (): Promise<GenericResponse> => {
      return await callBinding('CalibrationResume');
    },
    collect: async (): Promise<GenericResponse> => {
      return await callBinding('CalibrationCollect');
    },
    status: async (): Promise<CalibrationStatus> => {
      return await callBinding('CalibrationStatus');
    },
    getResult: async (taskId: string): Promise<CalibrationStatus | boolean> => {
      return await callBinding('CalibrationGetResult', taskId);
    }
  },

  // 存储管理
  storage: {
    startRecording: async (outputDir: string, filePrefix: string): Promise<GenericResponse> => {
      return await callBinding('StorageStartRecording', outputDir, filePrefix);
    },
    stopRecording: async (): Promise<GenericResponse> => {
      return await callBinding('StorageStopRecording');
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

  // 应用通用
  app: {
    getVersion: async (): Promise<VersionInfo> => {
      return await callBinding('GetVersion');
    },
    pickDirectory: async (): Promise<string> => {
      return await callBinding('PickDirectory');
    },
    pickFile: async (title: string, filters: Array<{ displayName: string; pattern: string }>): Promise<string> => {
      return await callBinding('PickFile', title, filters);
    },
    pickFiles: async (title: string, filters: Array<{ displayName: string; pattern: string }>): Promise<string[]> => {
      return await callBinding('PickFiles', title, filters);
    }
  }
};
