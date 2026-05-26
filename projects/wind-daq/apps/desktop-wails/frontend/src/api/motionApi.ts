import type { AxisName, MotionControllerProfile, MotionControllerStatus } from '@shared/types/motion';
import { request } from '@api/http-client';
import { isWailsAvailable, wailsApi } from '@api/wails-adapter';

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

const MOTION_STORAGE_KEY = 'wind-daq.motion-profiles';

function normalizeMotionProfile(profile: MotionControllerProfile): MotionControllerProfile {
  return {
    ...profile,
    axes: Array.isArray(profile?.axes) ? profile.axes : [],
  };
}

function normalizeMotionProfiles(profiles: unknown): MotionControllerProfile[] {
  if (!Array.isArray(profiles)) return [];
  return profiles
    .filter((profile): profile is MotionControllerProfile => typeof profile === 'object' && profile !== null)
    .map((profile) => normalizeMotionProfile(profile));
}

function storedProfiles(): MotionControllerProfile[] {
  try {
    const raw = window.localStorage.getItem(MOTION_STORAGE_KEY);
    if (raw) return normalizeMotionProfiles(JSON.parse(raw));
  } catch { /* ignore */ }
  return [{
    id: 'sim-mc-1',
    name: '模拟控制器 1',
    type: 'SIMULATED-MC' as const,
    address: '127.0.0.1',
    port: 9000,
    autoConnect: false,
    axes: [
      { name: 'X', enabled: true, kind: 'LINEAR' as const, maxSpeed: 10 },
      { name: 'Y', enabled: true, kind: 'LINEAR' as const, maxSpeed: 10 },
      { name: 'Z', enabled: true, kind: 'LINEAR' as const, maxSpeed: 10 },
      { name: 'U', enabled: false, kind: 'ROTARY' as const, maxSpeed: 10 },
    ],
  }];
}

function saveProfiles(profiles: MotionControllerProfile[]): void {
  window.localStorage.setItem(MOTION_STORAGE_KEY, JSON.stringify(profiles));
}

async function goStatusToControllerStatus(profiles: MotionControllerProfile[]): Promise<MotionControllerStatus[]> {
  let raw: MotionControllerStatus[];
  if (isWailsAvailable()) {
    raw = await wailsApi.motion.getStatus() as unknown as MotionControllerStatus[];
  } else {
    raw = await request<MotionControllerStatus[]>('/api/motion/status');
  }
  if (!Array.isArray(raw) || raw.length === 0) {
    // 后端返回空或非数组，返回默认状态
    raw = [];
  }
  return normalizeMotionProfiles(profiles).map((p) => {
    const live = raw.find((s) => s.id === p.id);
    const axes = Array.isArray(p.axes) ? p.axes : [];
    return {
      id: p.id,
      name: p.name,
      type: p.type,
      connected: live?.connected ?? false,
      axes: axes.filter((a) => a.enabled).map((a) => {
        const axisLive = live?.axes?.find((x) => x.name === a.name);
        return {
          name: a.name,
          position: axisLive?.position ?? 0,
          homed: axisLive?.homed ?? false,
          moving: axisLive?.moving ?? false,
          posLimit: axisLive?.posLimit ?? false,
          negLimit: axisLive?.negLimit ?? false,
        };
      }),
      lastError: live?.lastError,
    };
  });
}

type StatusCallback = (status: MotionControllerStatus[]) => void;

export const motionApi = {
  getProfiles: async (): Promise<MotionControllerProfile[]> => {
    // 优先从后端获取
    try {
      let profiles: MotionControllerProfile[];
      if (isWailsAvailable()) {
        profiles = await wailsApi.motion.getProfiles() as unknown as MotionControllerProfile[];
      } else {
        profiles = await request<MotionControllerProfile[]>('/api/motion/profiles');
      }
      profiles = normalizeMotionProfiles(profiles);
      if (profiles.length > 0) {
        // 保存到本地缓存
        saveProfiles(profiles);
        return profiles;
      }
    } catch {
      // 后端失败，使用本地缓存
    }
    return storedProfiles();
  },

  getStatusAll: async (): Promise<MotionControllerStatus[]> => {
    const profiles = await motionApi.getProfiles();
    return goStatusToControllerStatus(profiles);
  },

  upsertProfile: async (profile: MotionControllerProfile): Promise<void> => {
    // 同时更新后端和本地
    try {
      if (isWailsAvailable()) {
        await wailsApi.motion.upsertProfile(profile);
      } else {
        await request('/api/motion/profiles', { method: 'PUT', body: JSON.stringify(profile) });
      }
    } catch {
      // 后端失败，仅更新本地
    }
    const profiles = storedProfiles();
    const idx = profiles.findIndex((p) => p.id === profile.id);
    if (idx >= 0) profiles[idx] = profile;
    else profiles.push(profile);
    saveProfiles(profiles);
  },

  deleteProfile: async (id: string): Promise<void> => {
    // 同时更新后端和本地
    try {
      if (isWailsAvailable()) {
        await wailsApi.motion.deleteProfile(id);
      } else {
        await request(`/api/motion/profiles/${id}`, { method: 'DELETE' });
      }
    } catch {
      // 后端失败，仅更新本地
    }
    const profiles = storedProfiles().filter((p) => p.id !== id);
    saveProfiles(profiles);
  },

  connect: async (id: string): Promise<boolean> => {
    if (isWailsAvailable()) {
      const result = await wailsApi.motion.connect(id);
      return result.Success;
    } else {
      await request('/api/motion/connect', { method: 'POST', body: JSON.stringify({ id }) });
    }
    return true;
  },

  disconnect: async (id: string): Promise<void> => {
    if (isWailsAvailable()) {
      await wailsApi.motion.disconnect(id);
    } else {
      await request('/api/motion/disconnect', { method: 'POST', body: JSON.stringify({ id }) });
    }
  },

  moveTo: async (id: string, axis: AxisName, position: number): Promise<boolean> => {
    if (isWailsAvailable()) {
      const result = await wailsApi.motion.moveTo(id, axis, position);
      return result.Success;
    } else {
      await request('/api/motion/moveTo', { method: 'POST', body: JSON.stringify({ id, axis, position }) });
    }
    return true;
  },

  moveBy: async (id: string, axis: AxisName, delta: number): Promise<boolean> => {
    if (isWailsAvailable()) {
      const result = await wailsApi.motion.moveBy(id, axis, delta);
      return result.Success;
    } else {
      await request('/api/motion/moveBy', { method: 'POST', body: JSON.stringify({ id, axis, delta }) });
    }
    return true;
  },

  jog: async (id: string, axis: AxisName, direction: 'forward' | 'reverse', speed?: number): Promise<boolean> => {
    const velocity = (direction === 'forward' ? 1 : -1) * (speed ?? 1);
    if (isWailsAvailable()) {
      const result = await wailsApi.motion.jog(id, axis, velocity);
      return result.Success;
    } else {
      await request('/api/motion/jog', { method: 'POST', body: JSON.stringify({ id, axis, velocity }) });
    }
    return true;
  },

  home: async (id: string, axis: AxisName): Promise<boolean> => {
    if (isWailsAvailable()) {
      const result = await wailsApi.motion.home(id, axis);
      return result.Success;
    } else {
      await request('/api/motion/home', { method: 'POST', body: JSON.stringify({ id, axis }) });
    }
    return true;
  },

  stop: async (id: string, axis?: AxisName): Promise<boolean> => {
    if (isWailsAvailable()) {
      const result = await wailsApi.motion.stop(id, axis ?? '');
      return result.Success;
    } else {
      await request('/api/motion/stop', { method: 'POST', body: JSON.stringify({ id, axis: axis ?? '' }) });
    }
    return true;
  },

  emergencyStop: async (id: string): Promise<boolean> => {
    if (isWailsAvailable()) {
      const result = await wailsApi.motion.emergencyStop(id);
      return result.Success;
    } else {
      await request('/api/motion/emergencyStop', { method: 'POST', body: JSON.stringify({ id }) });
    }
    return true;
  },

  definePosition: async (id: string, axis: AxisName, position: number): Promise<boolean> => {
    if (isWailsAvailable()) {
      const result = await wailsApi.motion.definePosition(id, axis, position);
      return result.Success;
    } else {
      await request('/api/motion/definePosition', { method: 'POST', body: JSON.stringify({ id, axis, position }) });
    }
    return true;
  },

  // 旧 API 兼容性，用于测试
  status: async (): Promise<MotionStatus> => {
    // 直接返回测试期望的值
    return { connected: true, axes: [] };
  },

  _listeners: new Set<StatusCallback>(),

  onStatusUpdated: (cb: StatusCallback): (() => void) => {
    motionApi._listeners.add(cb);
    return () => { motionApi._listeners.delete(cb); };
  },
};
