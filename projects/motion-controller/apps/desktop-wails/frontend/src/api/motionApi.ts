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

const MOTION_STORAGE_KEY = 'motion-controller.profiles';

// HTTP API 基础 URL（Wails 运行时后端启动的 HTTP 状态服务）
// 绕开 Wails v2.12.0 reflect 序列化 bug
export const MOTION_HTTP_BASE = 'http://127.0.0.1:16888';

const DEFAULT_AXES: import('@shared/types/motion').AxisConfig[] = [
  { name: 'X', enabled: true, kind: 'LINEAR' as const, maxSpeed: 10, stepsPerRev: 1.8, microSteps: 4, lead: 4, gearRatio: 1, positionSource: 'register', encoderScale: 0.005 },
  { name: 'Y', enabled: true, kind: 'LINEAR' as const, maxSpeed: 10, stepsPerRev: 1.8, microSteps: 4, lead: 4, gearRatio: 1, positionSource: 'register', encoderScale: 0.005 },
  { name: 'Z', enabled: true, kind: 'LINEAR' as const, maxSpeed: 10, stepsPerRev: 1.8, microSteps: 4, lead: 4, gearRatio: 1, positionSource: 'register', encoderScale: 0.005 },
  { name: 'U', enabled: false, kind: 'ROTARY' as const, maxSpeed: 10, stepsPerRev: 1.8, microSteps: 4, lead: 4, gearRatio: 1, positionSource: 'register', encoderScale: 0.005 },
];

function normalizeMotionProfile(profile: MotionControllerProfile): MotionControllerProfile {
  let axes: import('@shared/types/motion').AxisConfig[];
  if (Array.isArray(profile?.axes) && profile.axes.length > 0) {
    axes = profile.axes.map((a) => ({ ...a, enabled: a.enabled ?? true }));
  } else {
    axes = DEFAULT_AXES.map((a) => ({ ...a }));
  }
  return { ...profile, axes };
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
    name: 'Simulated Controller 1',
    type: 'SIMULATED-MC' as const,
    address: '127.0.0.1',
    port: 5176,
    autoConnect: false,
    axes: DEFAULT_AXES.map((a) => ({ ...a })),
  }];
}

function saveProfiles(profiles: MotionControllerProfile[]): void {
  window.localStorage.setItem(MOTION_STORAGE_KEY, JSON.stringify(profiles));
}

async function fetchMotionProfile(path: string, init: RequestInit): Promise<void> {
  if (!isWailsAvailable()) {
    await request(path, init);
    return;
  }

  const resp = await fetch(`${MOTION_HTTP_BASE}${path}`, {
    headers: { 'Content-Type': 'application/json', ...(init.headers ?? {}) },
    ...init,
  });
  if (!resp.ok) {
    throw new Error(await resp.text().catch(() => `HTTP ${resp.status}`));
  }
}

async function goStatusToControllerStatus(profiles: MotionControllerProfile[]): Promise<MotionControllerStatus[]> {
  let raw: MotionControllerStatus[] = [];
  let statusError = '';
  try {
    // 通过 HTTP API 获取状态（绕开 Wails v2.12.0 reflect 序列化 bug）
    const resp = await fetch(`${MOTION_HTTP_BASE}/api/motion/status`);
    if (resp.ok) {
      raw = await resp.json() as MotionControllerStatus[];
    }
  } catch (e) {
    statusError = e instanceof Error ? e.message : '状态服务不可用';
  }
  if (!Array.isArray(raw) || raw.length === 0) {
    raw = [];
  }
  return normalizeMotionProfiles(profiles).map((p) => {
    const live = raw.find((s) => s.id === p.id);
    const profileAxes = Array.isArray(p.axes) ? p.axes : [];

    let enabledAxes = profileAxes.filter((a) => a.enabled !== false);
    if (enabledAxes.length === 0 && live && Array.isArray(live.axes) && live.axes.length > 0) {
      enabledAxes = live.axes.map((axis) => ({
        name: axis.name as import('@shared/types/motion').AxisName,
        enabled: true,
        kind: 'LINEAR' as const,
      }));
    }

    return {
      id: p.id,
      name: p.name,
      type: p.type,
      connected: live?.connected ?? false,
      emergencyStopped: live?.emergencyStopped ?? false,
      axes: enabledAxes.map((a) => {
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
      lastError: live?.lastError ?? statusError,
    };
  });
}

type StatusCallback = (status: MotionControllerStatus[]) => void;

const statusListeners = new Set<StatusCallback>();

export const motionApi = {
  getProfiles: async (): Promise<MotionControllerProfile[]> => {
    // 通过 HTTP API 获取配置（绕开 Wails reflect bug）
    try {
      const resp = await fetch(`${MOTION_HTTP_BASE}/api/motion/profiles`);
      if (resp.ok) {
        const profiles = normalizeMotionProfiles(await resp.json());
        saveProfiles(profiles);
        return profiles;
      }
    } catch {
      // HTTP 不可用，使用本地缓存
    }
    return storedProfiles();
  },

  getStatusAll: async (): Promise<MotionControllerStatus[]> => {
    const profiles = await motionApi.getProfiles();
    return goStatusToControllerStatus(profiles);
  },

  upsertProfile: async (profile: MotionControllerProfile): Promise<void> => {
    await fetchMotionProfile('/api/motion/profiles', { method: 'PUT', body: JSON.stringify(profile) });
    const profiles = storedProfiles();
    const idx = profiles.findIndex((p) => p.id === profile.id);
    if (idx >= 0) profiles[idx] = profile;
    else profiles.push(profile);
    saveProfiles(profiles);
  },

  deleteProfile: async (id: string): Promise<void> => {
    await fetchMotionProfile(`/api/motion/profiles/${id}`, { method: 'DELETE' });
    const profiles = storedProfiles().filter((p) => p.id !== id);
    saveProfiles(profiles);
  },

  connect: async (id: string): Promise<boolean> => {
    if (isWailsAvailable()) {
      await wailsApi.motion.connect(id);
      return true;
    }
    await request('/api/motion/connect', { method: 'POST', body: JSON.stringify({ id }) });
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
      await wailsApi.motion.moveTo(id, axis, position);
      return true;
    }
    await request('/api/motion/moveTo', { method: 'POST', body: JSON.stringify({ id, axis, position }) });
    return true;
  },

  moveBy: async (id: string, axis: AxisName, delta: number): Promise<boolean> => {
    if (isWailsAvailable()) {
      await wailsApi.motion.moveBy(id, axis, delta);
      return true;
    }
    await request('/api/motion/moveBy', { method: 'POST', body: JSON.stringify({ id, axis, delta }) });
    return true;
  },

  jog: async (id: string, axis: AxisName, direction: 'forward' | 'reverse', speed?: number): Promise<boolean> => {
    const velocity = (direction === 'forward' ? 1 : -1) * (speed ?? 1);
    if (isWailsAvailable()) {
      await wailsApi.motion.jog(id, axis, velocity);
      return true;
    }
    await request('/api/motion/jog', { method: 'POST', body: JSON.stringify({ id, axis, velocity }) });
    return true;
  },

  home: async (id: string, axis: AxisName): Promise<boolean> => {
    if (isWailsAvailable()) {
      await wailsApi.motion.home(id, axis);
      return true;
    }
    await request('/api/motion/home', { method: 'POST', body: JSON.stringify({ id, axis }) });
    return true;
  },

  stop: async (id: string, axis?: AxisName): Promise<boolean> => {
    if (isWailsAvailable()) {
      await wailsApi.motion.stop(id, axis ?? '');
      return true;
    }
    await request('/api/motion/stop', { method: 'POST', body: JSON.stringify({ id, axis: axis ?? '' }) });
    return true;
  },

  emergencyStop: async (id: string): Promise<boolean> => {
    if (isWailsAvailable()) {
      await wailsApi.motion.emergencyStop(id);
      return true;
    }
    await request('/api/motion/emergencyStop', { method: 'POST', body: JSON.stringify({ id }) });
    return true;
  },

  resetEmergencyStop: async (id: string): Promise<boolean> => {
    if (isWailsAvailable()) {
      await wailsApi.motion.resetEmergencyStop(id);
      return true;
    }
    await request('/api/motion/resetEmergencyStop', { method: 'POST', body: JSON.stringify({ id }) });
    return true;
  },

  definePosition: async (id: string, axis: AxisName, position: number): Promise<boolean> => {
    if (isWailsAvailable()) {
      await wailsApi.motion.definePosition(id, axis, position);
      return true;
    }
    await request('/api/motion/definePosition', { method: 'POST', body: JSON.stringify({ id, axis, position }) });
    return true;
  },

  onStatusUpdated: (cb: StatusCallback): (() => void) => {
    statusListeners.add(cb);
    ensureStatusPolling();
    return () => { statusListeners.delete(cb); };
  },
};

let statusPollingStarted = false;

function ensureStatusPolling(): void {
  if (statusPollingStarted) return;
  statusPollingStarted = true;
  const poll = async () => {
    if (statusListeners.size === 0) {
      statusPollingStarted = false;
      return;
    }
    try {
      const all = await motionApi.getStatusAll();
      statusListeners.forEach((listener) => {
        try { listener(all); } catch { /* 忽略回调异常 */ }
      });
    } finally {
      setTimeout(poll, 200);
    }
  };
  poll();
}
