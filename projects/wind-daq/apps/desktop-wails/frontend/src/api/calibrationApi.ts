import type { CalibrationConfig, CalibrationErrorCode, CalibrationType, MotionSafetyFailure, SevenHoleConfig, SevenHolePreviewResult, SphereTankGateConfig } from '@shared/types/calibration';
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

  /**
   * 七孔点位预览（spec Task 18）
   *
   * 在"配置向导"调整 α/β/θ/φ 范围与步长时实时调用后端，获取真实点位列表 + 内/外区点数聚合，
   * 用于侧边栏点阵预览与总点数显示。
   *
   * 实现策略（与 generateFiveHoleSnakePoints 形成对照）：
   *   - Wails 模式：调用 wailsApi.calibration.previewSevenHole(config) binding
   *     禁止像五孔那样 `return []`——必须真实调用后端 API（spec Task 18 Acceptance criteria）
   *   - HTTP 模式：POST /api/calibration/sevenhole-preview
   *   - 离线场景（Wails 不可用 + HTTP 失败）：抛错，不 fallback 到本地点位生成
   *
   * 返回 SevenHolePreviewResult：
   *   - points: 完整点位列表（含内区+外区，按蛇形/数据集顺序）
   *   - totalCount / innerCount / outerCount: 点数聚合统计
   *
   * 错误处理：
   *   - 配置非法（步长 ≤ 0、范围 min > max）：后端返回 Success=false + Error 透传
   *   - 离线场景：抛 Error，调用方应显示"请先连接后端"提示
   */
  previewSevenHolePoints: async (config: SevenHoleConfig): Promise<SevenHolePreviewResult> => {
    if (isWailsAvailable()) {
      // Wails 模式：调用 CalibrationPreviewSevenHole binding（GenericResponse 包装）
      // 禁止 `return { points: [], totalCount: 0, ... }`——必须真实调用后端 API（spec 强约束）
      const res = await wailsApi.calibration.previewSevenHole(config);
      if (!res.Success) {
        throw new Error(res.Error || '七孔点位预览失败：后端返回 Success=false');
      }
      // GenericResponse.Data 字段为 SevenHolePreviewResult（后端 JSON 序列化后字段名小驼峰）
      // 若 Data 为 null/undefined（binding 未同步或后端未填充），视为配置非法
      if (!res.Data) {
        throw new Error('七孔点位预览失败：后端未返回 Data 字段');
      }
      return res.Data as SevenHolePreviewResult;
    } else {
      // HTTP 模式：POST /api/calibration/sevenhole-preview
      // 后端 handleSevenHolePreview 直接返回 SevenHolePreviewResult JSON
      // 离线场景（HTTP 失败）：request 抛错，由调用方捕获显示"请先连接后端"
      return await request<SevenHolePreviewResult>('/api/calibration/sevenhole-preview', {
        method: 'POST',
        body: JSON.stringify(config),
      });
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
