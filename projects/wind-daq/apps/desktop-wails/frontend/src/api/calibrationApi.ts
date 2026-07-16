import type { CalibrationConfig, CalibrationErrorCode, CalibrationType, MotionSafetyFailure, SphereTankGateConfig } from '@shared/types/calibration';
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
  pausedDurationMs?: number;
  lastError?: string;
  /**
   * 结构化错误码（与 shared/types/calibration.CalibrationErrorCode 对齐）。
   *
   * 旧 Wails binding 未自动同步此字段时为 undefined，前端需做 fallback；
   * 新 binding 重新生成后会与后端 LastErrorCode 同步。
   */
  lastErrorCode?: CalibrationErrorCode;
  /**
   * 运动安全故障现场快照（与 shared/types/calibration.MotionSafetyFailure 对齐）。
   *
   * 旧 Wails binding 未自动同步此字段时为 undefined；新 binding 重新生成后会与后端同步。
   * 前端据此展示故障现场告警卡片。
   */
  motionSafetyFailure?: MotionSafetyFailure | null;
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

  saveData: async (taskId: string, savePath: string): Promise<{ success: boolean; filepath?: string; error?: string }> => {
    if (!savePath) return { success: false, error: '请先在校准配置中设置保存路径' };
    try {
      if (isWailsAvailable()) {
        const result = await wailsApi.calibration.saveCsv(taskId, savePath);
        if (!result.Success) return { success: false, error: result.Error };
        return { success: true, filepath: result.filepath || result.Filepath };
      }
      return await request('/api/calibration/saveCsv', {
        method: 'POST',
        body: JSON.stringify({ taskId, savePath }),
      });
    } catch (e) {
      return { success: false, error: String(e) };
    }
  },

  exportReport: async (_taskId: string): Promise<{ success: boolean; filepath?: string; error?: string }> => {
    return { success: false, error: '校准报告导出接口尚未接入' };
  },

  // 旧 API 兼容性，用于测试
  start: async (_cfg: OldCalibrationConfig): Promise<{ success: boolean }> => {
    return { success: true };
  },

  status: async (): Promise<CalibrationStatus> => {
    if (isWailsAvailable()) {
      return await wailsApi.calibration.status();
    } else {
      return await request<CalibrationStatus>('/api/calibration/status');
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

  generateFiveHoleSnakePoints: async (layout: {
    alphaMin: number; alphaMax: number; alphaStep: number
    betaMin: number; betaMax: number; betaStep: number
  }): Promise<Array<{ id: number; coordinates: Record<string, number> }>> => {
    try {
      if (isWailsAvailable()) {
        return [] // Wails 模式下暂不实现
      }
      return await request('/api/calibration/fivehole', {
        method: 'POST',
        body: JSON.stringify(layout),
      })
    } catch {
      return []
    }
  },

  getPrecisionDefaults: async (): Promise<{ probePrecision: number; machPrecision: number; velocityPrecision: number } | null> => {
    try {
      if (isWailsAvailable()) {
        return null; // Wails 模式下暂不实现，使用前端 fallback
      } else {
        return await request('/api/calibration/precisionDefaults');
      }
    } catch {
      return null;
    }
  },
};
