import type { CalibrationConfig, CalibrationErrorCode, CalibrationType, LivePhysics, MotionSafetyFailure, SevenHoleConfig, SevenHolePreviewResult, SphereTankGateConfig } from '@shared/types/calibration';
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
  /**
   * 实时物理量快照（spec Task 13/14，与后端 core/calibration.LivePhysics 对齐）。
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
  livePhysics?: LivePhysics;
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
        return { success: true };
      }
      // HTTP 模式（spec Task 14）：调用既有 POST /api/calibration/pause route，
      // 不再返回 synthetic success。后端返回 { success: true } 或 4xx + { error: "..." }；
      // request 在 !response.ok 时抛 ApiError，由 catch 转为 { success: false, error }。
      // _taskId 参数保留兼容签名——后端 route 不需要 taskId（与 Wails binding 一致）。
      await request('/api/calibration/pause', { method: 'POST' });
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
        return { success: true };
      }
      // HTTP 模式（spec Task 14）：调用既有 POST /api/calibration/resume route。
      await request('/api/calibration/resume', { method: 'POST' });
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
        return { success: true };
      }
      // HTTP 模式（spec Task 14）：调用既有 POST /api/calibration/stop route。
      await request('/api/calibration/stop', { method: 'POST' });
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
    // HTTP 模式（spec Task 14）：调用既有 POST /api/calibration/pause route，
    // 不再返回 synthetic success。request 在 !response.ok 时抛 ApiError，
    // 由调用方捕获（与 Wails 模式 throw new Error 行为一致）。
    await request('/api/calibration/pause', { method: 'POST' });
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
    // HTTP 模式（spec Task 14）：调用既有 POST /api/calibration/resume route。
    await request('/api/calibration/resume', { method: 'POST' });
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
    // HTTP 模式（spec Task 14）：调用既有 POST /api/calibration/stop route。
    await request('/api/calibration/stop', { method: 'POST' });
    return { success: true };
  },

  getResult: async (taskId: string): Promise<CalibrationStatus> => {
    if (isWailsAvailable()) {
      const result = await wailsApi.calibration.getResult(taskId);
      // Wails binding 偶尔返回 boolean（旧版签名遗留），用 typeof 而非 === true/false
      // 避免 TS 严格模式判定 CalibrationStatus 与 boolean 无交集报错
      if (typeof result === 'boolean') {
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

  /**
   * 五孔蛇形点位预览（spec Task 11 重写）
   *
   * 在"配置向导"调整 α/β 范围与步长时实时调用后端，获取真实点位列表。
   *
   * 实现策略（spec Task 11 acceptance：HTTP/Wails 都调用同一 usecase，后端错误传到 UI）：
   *   - Wails 模式：调用 wailsApi.calibration.previewFiveHole(layout) binding
   *     旧实现 `return []` 在 Wails 模式下不调后端，违反"HTTP/Wails 共用同一 usecase"约束
   *   - HTTP 模式：POST /api/calibration/fivehole（bare array 响应）
   *   - 离线/错误场景：抛 Error，不 fallback 到本地蛇形算法
   *
   * 返回 CalibrationPoint[]（bare array，与既有前端契约一致）：
   *   - id: 点位序号（1-based）
   *   - coordinates: { α: number, β: number }
   *
   * 错误处理：
   *   - 配置非法（步长 ≤ 0）：后端返回 Success=false + Error 透传（Wails）或 400 + error 字段（HTTP）
   *   - 离线场景（Wails 不可用 + HTTP 失败）：抛 Error，调用方应显示"请先连接后端"提示
   */
  generateFiveHoleSnakePoints: async (layout: {
    alphaMin: number; alphaMax: number; alphaStep: number
    betaMin: number; betaMax: number; betaStep: number
    serpentine?: boolean
    angleConvert?: boolean
  }): Promise<Array<{ id: number; coordinates: Record<string, number> }>> => {
    if (isWailsAvailable()) {
      // Wails 模式：调用 CalibrationPreviewFiveHole binding（GenericResponse 包装）
      // 旧实现 `return []` 让 Wails 模式静默失败，Task 11 后必须真实调用后端 API
      const res = await wailsApi.calibration.previewFiveHole(layout);
      if (!res.Success) {
        throw new Error(res.Error || '五孔点位预览失败：后端返回 Success=false');
      }
      // GenericResponse.Data 字段为 []FiveHoleSnakePoint（后端 JSON 序列化为 bare array）
      // 若 Data 为 null/undefined（binding 未同步或后端未填充），视为配置非法或内部错误
      if (!res.Data) {
        throw new Error('五孔点位预览失败：后端未返回 Data 字段');
      }
      return res.Data as Array<{ id: number; coordinates: Record<string, number> }>;
    }
    // HTTP 模式：POST /api/calibration/fivehole
    // 后端 handleFiveHolePreview 直接返回 bare array JSON
    // 离线场景（HTTP 失败）：request 抛错，由调用方捕获显示"请先连接后端"
    return await request('/api/calibration/fivehole', {
      method: 'POST',
      body: JSON.stringify(layout),
    });
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
