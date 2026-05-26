import { request } from '@api/http-client';
import { isWailsAvailable, wailsApi } from '@api/wails-adapter';

const API_BASE = import.meta.env.VITE_API_BASE || 'http://127.0.0.1:8900';

function apiRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const fullPath = path.startsWith('http') ? path : `${API_BASE}${path}`;
  return request<T>(fullPath, init);
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
  status: async () => {
    if (isWailsAvailable()) {
      return await wailsApi.storage.getStatus();
    }
    return apiRequest<{ recording: boolean; outputDir?: string }>('/api/storage/status');
  },
  start: async (outputDir: string, filePrefix: string) => {
    if (isWailsAvailable()) {
      const result = await wailsApi.storage.startRecording(outputDir, filePrefix);
      if (!result.Success) {
        throw new Error(result.Error);
      }
      return { success: true };
    }
    return apiRequest<{ success: boolean }>('/api/storage/start', {
      method: 'POST',
      body: JSON.stringify({ outputDir, filePrefix }),
    });
  },
  stop: async () => {
    if (isWailsAvailable()) {
      const result = await wailsApi.storage.stopRecording();
      if (!result.Success) {
        throw new Error(result.Error);
      }
      return { success: true };
    }
    return apiRequest<{ success: boolean }>('/api/storage/stop', { method: 'POST' });
  },
};

export const reportApi = {
  generate: (outputDir: string, filePrefix: string, deviceId: string) =>
    apiRequest<{ path: string; size: number; records: number }>('/api/report/generate', {
      method: 'POST',
      body: JSON.stringify({ outputDir, filePrefix, deviceId }),
    }),
  status: async () => {
    if (isWailsAvailable()) {
      return await wailsApi.report.getStatus();
    }
    return apiRequest<{ generating: boolean }>('/api/report/status');
  },
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
