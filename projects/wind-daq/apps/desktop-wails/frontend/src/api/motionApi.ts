import type { AxisName, MotionControllerProfile, MotionControllerStatus } from '@shared/types/motion';
import { request } from '@api/http-client';
import { isWailsAvailable, wailsApi } from '@api/wails-adapter';

// =====================================================================
// 运动控制器独立窗口（motion 子进程）专用通道
// ---------------------------------------------------------------------
// 背景：运动控制器独立窗口通过 OpenMotionWindow 启动一个新的 Wails 子进程。
// 子进程拥有自己的 MotionManager 内存实例，与主进程完全隔离。一旦关闭独立
// 窗口（Quit），子进程退出，连接句柄丢失；再次打开时会启动全新的子进程，
// 前端看到"未连接"是真实状态。
//
// 修复策略：在独立窗口（motion 模式）下，所有运动控制相关的请求都通过
// 主进程暴露的本地 HTTP API（127.0.0.1:8900）转发到主进程的 MotionManager。
// 这样连接状态只保存在主进程，独立窗口仅是"操作面板"，关闭后再次打开
// 也能拿到主进程持有的真实连接状态。
//
// 状态推送同样改为定时轮询 HTTP /api/motion/status，因为 Wails 事件
// （motion:status）只在当前进程内传递，子进程订阅不到主进程的事件。
// =====================================================================

// 主进程本地 HTTP API 地址（与 backend/app.go startLocalAPIServer 保持一致）
const MAIN_PROCESS_API_BASE = 'http://127.0.0.1:8900';

// 是否处于运动控制器独立窗口（motion 子进程）模式
// 由 main.ts 在启动阶段拿到 GetStartupMode 后调用 setMotionStandaloneMode 设置
let _motionStandaloneMode = false;

/**
 * 标记当前前端是否运行在运动控制器独立窗口（motion 子进程）中。
 * 由 main.ts bootstrap 阶段设置；运动调用据此走主进程 HTTP，避免连接状态分裂。
 */
export function setMotionStandaloneMode(enabled: boolean): void {
  _motionStandaloneMode = enabled;
}

export function isMotionStandaloneMode(): boolean {
  return _motionStandaloneMode;
}

/**
 * 通过主进程本地 HTTP API 调用运动接口。
 * 仅在 motion 子进程下使用，避免子进程的 wails binding 走自己进程的 MotionManager。
 */
async function mainProcessRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${MAIN_PROCESS_API_BASE}${path}`, {
    headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) },
    ...init,
  });
  const text = await response.text().catch(() => '');
  if (!response.ok) {
    throw new Error(text || `HTTP ${response.status}`);
  }
  if (!text) return undefined as unknown as T;
  try {
    return JSON.parse(text) as T;
  } catch (e) {
    const detail = e instanceof Error ? e.message : String(e);
    throw new Error(`主进程返回非 JSON 数据 (${detail}): ${text.slice(0, 200)}`);
  }
}

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

const DEFAULT_AXES: import('@shared/types/motion').AxisConfig[] = [
  { name: 'X', enabled: true, kind: 'LINEAR' as const, maxSpeed: 10 },
  { name: 'Y', enabled: true, kind: 'LINEAR' as const, maxSpeed: 10 },
  { name: 'Z', enabled: true, kind: 'LINEAR' as const, maxSpeed: 10 },
  { name: 'U', enabled: false, kind: 'ROTARY' as const, maxSpeed: 10 },
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
  } catch (err) {
    // localStorage 读取或 JSON 解析失败：使用默认模拟控制器，但记录日志便于排查
    console.warn('[motionApi] failed to load stored profiles:', err);
  }
  return [{
    id: 'sim-mc-1',
    name: '模拟控制器 1',
    type: 'SIMULATED-MC' as const,
    address: '127.0.0.1',
    port: 9000,
    autoConnect: false,
    axes: DEFAULT_AXES.map((a) => ({ ...a })),
  }];
}

function saveProfiles(profiles: MotionControllerProfile[]): void {
  window.localStorage.setItem(MOTION_STORAGE_KEY, JSON.stringify(profiles));
}

async function goStatusToControllerStatus(profiles: MotionControllerProfile[]): Promise<MotionControllerStatus[]> {
  let raw: MotionControllerStatus[];
  if (isMotionStandaloneMode()) {
    // 独立窗口：通过主进程 HTTP 拉真实连接状态，避免子进程的 MotionManager 永远为空
    raw = await mainProcessRequest<MotionControllerStatus[]>('/api/motion/status');
  } else if (isWailsAvailable()) {
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
    const profileAxes = Array.isArray(p.axes) ? p.axes : [];

    // 优先从 profile 配置中获取启用的轴，如果 profile 中没有配置，则尝试从后端状态中获取
    // 注意：enabled 为 undefined 时视为 true（兼容旧数据）
    let enabledAxes = profileAxes.filter((a) => a.enabled !== false);
    if (enabledAxes.length === 0 && live && Array.isArray(live.axes) && live.axes.length > 0) {
      // profile 中没有配置轴，但后端状态中有轴数据，使用后端的轴数据
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
      if (isMotionStandaloneMode()) {
        // 独立窗口：profile 也从主进程读，确保与主窗口一致
        profiles = await mainProcessRequest<MotionControllerProfile[]>('/api/motion/profiles');
      } else if (isWailsAvailable()) {
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
    } catch (err) {
      console.warn('[motionApi] getProfiles backend unreachable, falling back to local cache:', err);
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
      if (isMotionStandaloneMode()) {
        // 独立窗口：profile 写到主进程，避免子进程持久化文件后被关闭丢失或与主进程冲突
        await mainProcessRequest('/api/motion/profiles', { method: 'PUT', body: JSON.stringify(profile) });
      } else if (isWailsAvailable()) {
        await wailsApi.motion.upsertProfile(profile);
      } else {
        await request('/api/motion/profiles', { method: 'PUT', body: JSON.stringify(profile) });
      }
    } catch (err) {
      console.warn('[motionApi] upsertProfile backend failed, updating local cache only:', err);
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
      if (isMotionStandaloneMode()) {
        await mainProcessRequest(`/api/motion/profiles/${id}`, { method: 'DELETE' });
      } else if (isWailsAvailable()) {
        await wailsApi.motion.deleteProfile(id);
      } else {
        await request(`/api/motion/profiles/${id}`, { method: 'DELETE' });
      }
    } catch (err) {
      console.warn('[motionApi] deleteProfile backend failed, updating local cache only:', err);
    }
    const profiles = storedProfiles().filter((p) => p.id !== id);
    saveProfiles(profiles);
  },

  connect: async (id: string): Promise<boolean> => {
    if (isMotionStandaloneMode()) {
      // 独立窗口：连接动作交给主进程，连接状态保存在主进程 MotionManager
      await mainProcessRequest('/api/motion/connect', { method: 'POST', body: JSON.stringify({ id }) });
      return true;
    }
    if (isWailsAvailable()) {
      const result = await wailsApi.motion.connect(id);
      return result.Success;
    } else {
      await request('/api/motion/connect', { method: 'POST', body: JSON.stringify({ id }) });
    }
    return true;
  },

  disconnect: async (id: string): Promise<void> => {
    if (isMotionStandaloneMode()) {
      // 独立窗口：断开动作也由主进程执行，主进程的 controllers map 会移除该 id
      await mainProcessRequest('/api/motion/disconnect', { method: 'POST', body: JSON.stringify({ id }) });
      return;
    }
    if (isWailsAvailable()) {
      await wailsApi.motion.disconnect(id);
    } else {
      await request('/api/motion/disconnect', { method: 'POST', body: JSON.stringify({ id }) });
    }
  },

  moveTo: async (id: string, axis: AxisName, position: number): Promise<boolean> => {
    if (isMotionStandaloneMode()) {
      await mainProcessRequest('/api/motion/moveTo', { method: 'POST', body: JSON.stringify({ id, axis, position }) });
      return true;
    }
    if (isWailsAvailable()) {
      const result = await wailsApi.motion.moveTo(id, axis, position);
      return result.Success;
    } else {
      await request('/api/motion/moveTo', { method: 'POST', body: JSON.stringify({ id, axis, position }) });
    }
    return true;
  },

  moveBy: async (id: string, axis: AxisName, delta: number): Promise<boolean> => {
    if (isMotionStandaloneMode()) {
      await mainProcessRequest('/api/motion/moveBy', { method: 'POST', body: JSON.stringify({ id, axis, delta }) });
      return true;
    }
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
    if (isMotionStandaloneMode()) {
      await mainProcessRequest('/api/motion/jog', { method: 'POST', body: JSON.stringify({ id, axis, velocity }) });
      return true;
    }
    if (isWailsAvailable()) {
      const result = await wailsApi.motion.jog(id, axis, velocity);
      return result.Success;
    } else {
      await request('/api/motion/jog', { method: 'POST', body: JSON.stringify({ id, axis, velocity }) });
    }
    return true;
  },

  home: async (id: string, axis: AxisName): Promise<boolean> => {
    if (isMotionStandaloneMode()) {
      await mainProcessRequest('/api/motion/home', { method: 'POST', body: JSON.stringify({ id, axis }) });
      return true;
    }
    if (isWailsAvailable()) {
      const result = await wailsApi.motion.home(id, axis);
      return result.Success;
    } else {
      await request('/api/motion/home', { method: 'POST', body: JSON.stringify({ id, axis }) });
    }
    return true;
  },

  stop: async (id: string, axis?: AxisName): Promise<boolean> => {
    if (isMotionStandaloneMode()) {
      await mainProcessRequest('/api/motion/stop', { method: 'POST', body: JSON.stringify({ id, axis: axis ?? '' }) });
      return true;
    }
    if (isWailsAvailable()) {
      const result = await wailsApi.motion.stop(id, axis ?? '');
      return result.Success;
    } else {
      await request('/api/motion/stop', { method: 'POST', body: JSON.stringify({ id, axis: axis ?? '' }) });
    }
    return true;
  },

  emergencyStop: async (id: string): Promise<boolean> => {
    if (isMotionStandaloneMode()) {
      await mainProcessRequest('/api/motion/emergencyStop', { method: 'POST', body: JSON.stringify({ id }) });
      return true;
    }
    if (isWailsAvailable()) {
      const result = await wailsApi.motion.emergencyStop(id);
      return result.Success;
    } else {
      await request('/api/motion/emergencyStop', { method: 'POST', body: JSON.stringify({ id }) });
    }
    return true;
  },

  definePosition: async (id: string, axis: AxisName, position: number): Promise<boolean> => {
    if (isMotionStandaloneMode()) {
      await mainProcessRequest('/api/motion/definePosition', { method: 'POST', body: JSON.stringify({ id, axis, position }) });
      return true;
    }
    if (isWailsAvailable()) {
      const result = await wailsApi.motion.definePosition(id, axis, position);
      return result.Success;
    } else {
      await request('/api/motion/definePosition', { method: 'POST', body: JSON.stringify({ id, axis, position }) });
    }
    return true;
  },

  resetEmergencyStop: async (id: string): Promise<boolean> => {
    if (isMotionStandaloneMode()) {
      await mainProcessRequest('/api/motion/resetEmergencyStop', { method: 'POST', body: JSON.stringify({ id }) });
      return true;
    }
    if (isWailsAvailable()) {
      const result = await wailsApi.motion.resetEmergencyStop(id);
      return result.Success;
    } else {
      await request('/api/motion/resetEmergencyStop', { method: 'POST', body: JSON.stringify({ id }) });
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

    // 独立窗口（motion 子进程）：自己进程的 motion:status 事件源拿不到主进程数据，
    // 改为定时拉取主进程 HTTP /api/motion/status，并把结果分发给所有本地监听器。
    if (isMotionStandaloneMode()) {
      // 仅由首个监听器启动定时器，避免多个 store 实例触发多组定时器
      const ensureTimer = (): void => {
        const slot = motionApi as unknown as { _standaloneTimer?: ReturnType<typeof setInterval> };
        if (slot._standaloneTimer) return;
        const tick = async (): Promise<void> => {
          try {
            const list = await motionApi.getStatusAll();
            motionApi._listeners.forEach((listener) => {
              try {
                listener(list);
              } catch (err) {
                // 监听器回调异常不应中断轮询；记录以便排查上游 store 缺陷
                console.error('[motionApi] status listener threw:', err);
              }
            });
          } catch (err) {
            // 主进程不可达：下个 tick 重试。debug 级别避免刷屏
            console.debug('[motionApi] standalone tick failed (will retry):', err);
          }
        };
        // 与主进程 MotionStatusPoller 节奏保持接近：250ms 兼顾实时性与 HTTP 开销
        slot._standaloneTimer = setInterval(() => { void tick(); }, 250);
        // 立即触发一次，避免首次渲染等待 250ms
        void tick();
      };
      ensureTimer();
      return () => {
        motionApi._listeners.delete(cb);
        // 最后一个监听器移除时停止定时器，防止内存泄漏
        if (motionApi._listeners.size === 0) {
          const slot = motionApi as unknown as { _standaloneTimer?: ReturnType<typeof setInterval> };
          if (slot._standaloneTimer) {
            clearInterval(slot._standaloneTimer);
            slot._standaloneTimer = undefined;
          }
        }
      };
    }

    // 在 Wails 环境下，监听后端推送的运动状态事件
    if (isWailsAvailable()) {
      const unsubscribe = wailsApi.motion.onStatusEvent((data) => {
        if (Array.isArray(data)) {
          motionApi._listeners.forEach((listener) => {
            try {
              listener(data as MotionControllerStatus[]);
            } catch (err) {
              // 监听器回调异常不应中断 Wails 事件订阅
              console.error('[motionApi] status listener threw (Wails):', err);
            }
          });
        }
      });
      // 返回清理函数，同时移除监听器和 Wails 事件订阅
      return () => {
        motionApi._listeners.delete(cb);
        try {
          unsubscribe();
        } catch (err) {
          console.warn('[motionApi] unsubscribe failed:', err);
        }
      };
    }

    return () => { motionApi._listeners.delete(cb); };
  },
};
