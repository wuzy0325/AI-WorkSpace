import { request } from '@api/http-client';
import { isWailsAvailable, wailsApi } from '@api/wails-adapter';
import type { GenericResponse, StorageRecordingConfig, StorageRecordingStatus } from '@api/wails-adapter';

function apiBase(): string {
  if (import.meta.env.VITE_API_BASE) return import.meta.env.VITE_API_BASE;
  if (import.meta.env.DEV) return '';
  return isWailsAvailable() ? 'http://127.0.0.1:8900' : '';
}

function apiRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const fullPath = path.startsWith('http') ? path : `${apiBase()}${path}`;
  return request<T>(fullPath, init);
}

async function wailsSimpleAction(
  wailsFn: () => Promise<GenericResponse>,
  httpPath: string,
  httpInit?: RequestInit,
): Promise<{ success: boolean }> {
  if (isWailsAvailable()) {
    const result = await wailsFn();
    if (!result.Success) throw new Error(result.Error);
    return { success: true };
  }
  return apiRequest<{ success: boolean }>(httpPath, httpInit);
}

export interface TraversalPoint {
  x: number;
  y: number;
  z: number;
}

export const traversalApi = {
  generateGrid: (cfg: { xStart: number; xEnd: number; xStep: number; yStart: number; yEnd: number; yStep: number; zStart: number }) =>
    apiRequest<TraversalPoint[]>('/api/traversal/generateGrid', {
      method: 'POST',
      body: JSON.stringify(cfg),
    }),
  start: (taskId: string, deviceId: string, channels: number[], path: TraversalPoint[]) =>
    apiRequest<{ success: boolean }>('/api/traversal/start', {
      method: 'POST',
      body: JSON.stringify({ taskId, deviceId, channels, path }),
    }),
  status: () => apiRequest<{ state: string; currentPoint: number; totalPoints: number }>('/api/traversal/status'),
  runPoint: () => apiRequest<{ success: boolean }>('/api/traversal/runPoint', { method: 'POST' }),
  pause: () => apiRequest<{ success: boolean }>('/api/traversal/pause', { method: 'POST' }),
  resume: () => apiRequest<{ success: boolean }>('/api/traversal/resume', { method: 'POST' }),
  stop: () => apiRequest<{ success: boolean }>('/api/traversal/stop', { method: 'POST' }),
};

export const storageApi = {
  // status 返回扩展后的 RecordingStatus（含 currentFile/fileSize/recordCount/lastError 等）
  status: async (): Promise<StorageRecordingStatus> => {
    if (isWailsAvailable()) {
      return await wailsApi.storage.getStatus();
    }
    return apiRequest<StorageRecordingStatus>('/api/storage/status');
  },
  // start 接收完整 RecordingConfig，路径解析由后端统一完成。
  // config 中的 stopConditions/fileRotation/format 直接透传给 sink writerLoop 评估。
  start: (config: StorageRecordingConfig) =>
    wailsSimpleAction(
      () => wailsApi.storage.startRecording(config),
      '/api/storage/start',
      { method: 'POST', body: JSON.stringify(config) },
    ),
  stop: () =>
    wailsSimpleAction(
      () => wailsApi.storage.stopRecording(),
      '/api/storage/stop',
      { method: 'POST' },
    ),
};

export const reportApi = {
  generate: (outputDir: string, filePrefix: string, deviceId: string) =>
    apiRequest<{ path: string; size: number; records: number }>('/api/report/generate', {
      method: 'POST',
      body: JSON.stringify({ outputDir, filePrefix, deviceId }),
    }),
  status: async () => (isWailsAvailable() ? wailsApi.report.getStatus() : apiRequest<{ generating: boolean }>('/api/report/status')),
};

export interface MotionAxisStatus {
  name: string;
  position: number;
  homed: boolean;
  moving: boolean;
}

export interface MotionStatus {
  connected: boolean;
  axes: MotionAxisStatus[];
}
