import type { CalibrationConfig, CalibrationType, SphereTankGateConfig } from '@shared/types/calibration';
import { request } from '@api/http-client';
import { isWailsAvailable, wailsApi } from '@api/wails-adapter';

function getConfigKey(type: CalibrationType): string {
  return `calibration-config.${type}`;
}

// 兼容旧 API 的类型
export interface CalibrationPointResult {
  pointIndex: number;
  targetPressure: number;
  timestamp: number;
  values: Record<number, number>;
}

export interface CalibrationStatus {
  taskId: string;
  state: string;
  currentPoint: number;
  totalPoints: number;
  lastError?: string;
  results?: CalibrationPointResult[];
}

export interface OldCalibrationConfig {
  taskId: string;
  deviceId: string;
  type: string;
  channels: number[];
  pressurePoints: number[];
  averageSamples: number;
}

export const calibrationApi = {
  getConfig: async (type: CalibrationType): Promise<{ success: boolean; data?: CalibrationConfig; error?: string }> => {
    try {
      if (isWailsAvailable()) {
        const res = await wailsApi.config.load(getConfigKey(type));
        if (res.success && res.data) {
          return { success: true, data: res.data as CalibrationConfig };
        }
        return { success: false, error: 'No saved config' };
      } else {
        const res = await request<{ success: boolean; data?: CalibrationConfig }>(`/api/config/${getConfigKey(type)}`);
        if (res.success && res.data) {
          return { success: true, data: res.data };
        }
        return { success: false, error: 'No saved config' };
      }
    } catch (e) {
      return { success: false, error: String(e) };
    }
  },

  saveConfig: async (type: CalibrationType, config: CalibrationConfig): Promise<{ success: boolean; data?: CalibrationConfig; error?: string }> => {
    try {
      if (isWailsAvailable()) {
        await wailsApi.config.save(getConfigKey(type), config);
      } else {
        await request(`/api/config/${getConfigKey(type)}`, {
          method: 'PUT',
          body: JSON.stringify(config),
        });
      }
      return { success: true, data: config };
    } catch (e) {
      return { success: false, error: String(e) };
    }
  },

  startCalibration: async (config: CalibrationConfig): Promise<{ success: boolean; taskId?: string; error?: string }> => {
    try {
      if (isWailsAvailable()) {
        const result = await wailsApi.calibration.start(config);
        if (!result.Success) {
          return { success: false, error: result.Error };
        }
        return { success: true, taskId: config.taskId };
      } else {
        const result = await request<{ success: boolean; taskId?: string; error?: string }>('/api/calibration/start', {
          method: 'POST',
          body: JSON.stringify(config),
        });
        return result;
      }
    } catch (e) {
      return { success: false, error: String(e) };
    }
  },

  pauseCalibration: async (_taskId: string): Promise<{ success: boolean; error?: string }> => {
    try {
      if (isWailsAvailable()) {
        const result = await wailsApi.calibration.pause();
        if (!result.Success) {
          return { success: false, error: result.Error };
        }
      } else {
        // HTTP 模式下暂不实现
      }
      return { success: true };
    } catch (e) {
      return { success: false, error: String(e) };
    }
  },

  resumeCalibration: async (_taskId: string): Promise<{ success: boolean; error?: string }> => {
    try {
      if (isWailsAvailable()) {
        const result = await wailsApi.calibration.resume();
        if (!result.Success) {
          return { success: false, error: result.Error };
        }
      } else {
        // HTTP 模式下暂不实现
      }
      return { success: true };
    } catch (e) {
      return { success: false, error: String(e) };
    }
  },

  stopCalibration: async (_taskId: string): Promise<{ success: boolean; error?: string }> => {
    try {
      if (isWailsAvailable()) {
        const result = await wailsApi.calibration.stop();
        if (!result.Success) {
          return { success: false, error: result.Error };
        }
      } else {
        // HTTP 模式下暂不实现
      }
      return { success: true };
    } catch (e) {
      return { success: false, error: String(e) };
    }
  },

  saveData: async (_taskId: string): Promise<{ success: boolean; filepath?: string; error?: string }> => {
    return { success: true, filepath: '' };
  },

  exportReport: async (_taskId: string): Promise<{ success: boolean; filepath?: string; error?: string }> => {
    return { success: true, filepath: '' };
  },

  // 旧 API 兼容性，用于测试
  start: async (_cfg: OldCalibrationConfig): Promise<{ success: boolean }> => {
    return { success: true };
  },

  status: async (): Promise<CalibrationStatus> => {
    if (isWailsAvailable()) {
      return await wailsApi.calibration.status();
    } else {
      return {
        taskId: 'test',
        state: 'idle',
        currentPoint: 0,
        totalPoints: 0,
      };
    }
  },

  collect: async (): Promise<{ success: boolean }> => {
    if (isWailsAvailable()) {
      const result = await wailsApi.calibration.collect();
      if (!result.Success) {
        throw new Error(result.Error);
      }
      return { success: true };
    }
    return { success: true };
  },

  pause: async (): Promise<{ success: boolean }> => {
    if (isWailsAvailable()) {
      const result = await wailsApi.calibration.pause();
      if (!result.Success) {
        throw new Error(result.Error);
      }
      return { success: true };
    }
    return { success: true };
  },

  resume: async (): Promise<{ success: boolean }> => {
    if (isWailsAvailable()) {
      const result = await wailsApi.calibration.resume();
      if (!result.Success) {
        throw new Error(result.Error);
      }
      return { success: true };
    }
    return { success: true };
  },

  stop: async (): Promise<{ success: boolean }> => {
    if (isWailsAvailable()) {
      const result = await wailsApi.calibration.stop();
      if (!result.Success) {
        throw new Error(result.Error);
      }
      return { success: true };
    }
    return { success: true };
  },

  getResult: async (taskId: string): Promise<CalibrationStatus> => {
    if (isWailsAvailable()) {
      const result = await wailsApi.calibration.getResult(taskId);
      if (result === true || result === false) {
        return {
          taskId: taskId,
          state: 'idle',
          currentPoint: 0,
          totalPoints: 0,
        };
      }
      return result as CalibrationStatus;
    }
    return {
      taskId: taskId,
      state: 'idle',
      currentPoint: 0,
      totalPoints: 0,
    };
  },

  updateSphereTankGate: async (_gate: SphereTankGateConfig): Promise<{ success: boolean; error?: string }> => {
    return { success: true };
  },
};
