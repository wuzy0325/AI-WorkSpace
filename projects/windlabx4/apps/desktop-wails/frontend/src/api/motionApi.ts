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
async function mainProcessRequest<T>(path: string, init?: RequestInit, timeoutMs?: number): Promise<T> {
  // timeoutMs 可选：仅对状态查询等需要快速失败的请求设置（如 1500ms），
  // 避免某控制器卡顿时 fetch 长时间挂起拖停轮询链。命令类请求不传，使用默认无超时。
  const controller = timeoutMs ? new AbortController() : undefined;
  const timeoutId = controller ? setTimeout(() => controller.abort(), timeoutMs) : undefined;
  let response: Response;
  try {
    response = await fetch(`${MAIN_PROCESS_API_BASE}${path}`, {
      headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) },
      ...init,
      signal: controller?.signal,
    });
  } finally {
    if (timeoutId) clearTimeout(timeoutId);
  }
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

async function motionHttpRequest<T>(path: string, init?: RequestInit): Promise<T> {
  if (isWailsAvailable()) {
    return await mainProcessRequest<T>(path, init);
  }
  return await request<T>(path, init);
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

const MOTION_STORAGE_KEY = 'WindLabX4.motion-profiles';

const DEFAULT_AXES: import('@shared/types/motion').AxisConfig[] = [
  { name: 'X', enabled: true, kind: 'LINEAR' as const, maxSpeed: 10 },
  { name: 'Y', enabled: true, kind: 'LINEAR' as const, maxSpeed: 10 },
  { name: 'Z', enabled: true, kind: 'LINEAR' as const, maxSpeed: 10 },
  { name: 'U', enabled: true, kind: 'ROTARY' as const, maxSpeed: 10 },
];

function normalizeMotionProfile(profile: MotionControllerProfile): MotionControllerProfile {
  let axes: import('@shared/types/motion').AxisConfig[];
  if (Array.isArray(profile?.axes) && profile.axes.length > 0) {
    axes = profile.axes.map((a) => ({ ...a, enabled: true }));
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

// 内存缓存的 profiles，轮询 tick 不再重复请求后端
let _cachedProfiles: MotionControllerProfile[] | null = null;

// 轮询速率控制：运动中 100ms 高频刷新，全部空闲时 500ms 监控心跳
// 实测 WTNMC4A Status 耗时 ~132ms（4轴 readLP 串行），setTimeout 串联模式会自动
// 适应 Status 耗时：实际间隔 = max(FAST_POLL_MS, Status耗时) ≈ 132ms，
// 1mm 步长运动期间能看到 1-2 次中间位置更新。
const FAST_POLL_MS = 100;
const SLOW_POLL_MS = 500;
const BROWSER_POLL_MS = 3000;
const COMMAND_FAST_POLL_GRACE_MS = 2000;
// 当前期望轮询间隔。setTimeout 串联模式下由 scheduleNextStandaloneTick 读取。
// 修改此值后下一个 tick 周期立即生效，无需重启定时器。
let _currentPollIntervalMs = FAST_POLL_MS;
let _fastPollingUntil = 0;
// 防止 tick 重叠执行的串行锁。前一次 tick 未完成时，跳过本次触发。
let _standaloneTickInFlight = false;
let _standaloneActive = false;

/**
 * 仅拉取原始状态数据，不依赖 profiles。
 * 与 goStatusToControllerStatus 的"拉取+合并"二合一逻辑解耦，供 tick 复用。
 */
async function fetchRawStatus(): Promise<MotionControllerStatus[]> {
  if (isMotionStandaloneMode()) {
    // 超时 2500ms：B140 四轴编码器场景单次 Status 需 14 次串行 TCP 往返，
    // 网络抖动时可能逼近 2s；留足余量避免误超时，同时仍远低于原先无超时的 5s 级联阻塞。
    return await mainProcessRequest<MotionControllerStatus[]>('/api/motion/status', undefined, 2500);
  }
  if (isWailsAvailable()) {
    return await wailsApi.motion.getStatus() as unknown as MotionControllerStatus[];
  }
  return await request<MotionControllerStatus[]>('/api/motion/status');
}

/**
 * 从原始 status 数据和 profiles 合成归一化的 MotionControllerStatus 列表。
 * 纯函数，不发起任何网络请求。
 */
function mergeStatusWithProfiles(
  raw: MotionControllerStatus[],
  profiles: MotionControllerProfile[],
  statusError = ''
): MotionControllerStatus[] {
  if (!Array.isArray(raw)) raw = [];
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

async function goStatusToControllerStatus(profiles: MotionControllerProfile[]): Promise<MotionControllerStatus[]> {
  let raw: MotionControllerStatus[] = [];
  let statusError = '';
  try {
    raw = await fetchRawStatus();
  } catch (e) {
    statusError = e instanceof Error ? e.message : '状态服务不可用';
  }
  return mergeStatusWithProfiles(raw, profiles, statusError);
}

/**
 * 根据是否有轴正在运动，调整轮询间隔。
 * - 运动中或命令刚发出后 → 100ms 高频，实时跟踪位置变化
 * - 全部空闲 → 500ms 监控心跳，避免独立画面位置长时间不刷新
 *
 * 仅更新期望间隔，不重启定时器：setTimeout 串联模式下由 tickStandalone 末尾的
 * scheduleNextStandaloneTick 读取最新值。这与原来 setInterval 模式有本质区别——
 * 不再因 clearInterval + setInterval 重置计数器，也不会因 tick 耗时长导致
 * setInterval 强制触发新 tick 造成请求堆积。
 */
function adjustPollingRate(hasMoving: boolean): void {
  _currentPollIntervalMs = hasMoving || Date.now() < _fastPollingUntil ? FAST_POLL_MS : SLOW_POLL_MS;
}

/**
 * 调度下一次 tickStandalone。必须在 tick 完成后调用，保证串行执行。
 */
function scheduleNextStandaloneTick(): void {
  if (!_standaloneActive) return;
  const slot = motionApi as unknown as { _standaloneTimer?: ReturnType<typeof setTimeout> };
  if (slot._standaloneTimer) {
    clearTimeout(slot._standaloneTimer);
  }
  slot._standaloneTimer = setTimeout(() => { void tickStandalone(); }, _currentPollIntervalMs);
}

/**
 * 运动指令发出后调用：立即切换到高频轮询并触发一次 tick，
 * 确保 UI 快速响应电机位置变化。
 */
function requestFastPolling(): void {
  if (!isMotionStandaloneMode()) return;
  _currentPollIntervalMs = FAST_POLL_MS;
  _fastPollingUntil = Date.now() + COMMAND_FAST_POLL_GRACE_MS;
  // 上一次 tick 仍在进行时，不重复触发，等其完成后用新间隔调度
  if (_standaloneTickInFlight) return;
  // 取消尚未触发的 setTimeout，立即执行一次
  const slot = motionApi as unknown as { _standaloneTimer?: ReturnType<typeof setTimeout> };
  if (slot._standaloneTimer) {
    clearTimeout(slot._standaloneTimer);
    slot._standaloneTimer = undefined;
  }
  void tickStandalone();
}

/**
 * 独立窗口（motion 子进程）的轮询 tick。
 * 只拉取 /api/motion/status，用缓存的 profiles 做归一化，
 * 并根据运动状态自动调速。
 *
 * 串行执行：tickStandalone 完成后才调度下一次。
 * 如果 tick 本身耗时超过轮询间隔（WTNMC4A 4 轴状态查询可能超过 100ms），不会发生请求堆积——
 * 下一次 tick 在前一次返回后才开始，避免主进程 WTNMC4A.Status 因 c.mu.Lock
 * 排队堆积导致 2500ms 超时，UI 位置更新延迟。
 */
async function tickStandalone(): Promise<void> {
  if (!_standaloneActive) return;
  _standaloneTickInFlight = true;
  try {
    let raw: MotionControllerStatus[] = [];
    let statusError = '';
    try {
      raw = await mainProcessRequest<MotionControllerStatus[]>('/api/motion/status', undefined, 2500);
      if (!Array.isArray(raw)) raw = [];
    } catch (err) {
      statusError = err instanceof Error ? err.message : '主进程状态服务不可用';
    }

    try {
      if (!_cachedProfiles) {
        _cachedProfiles = await motionApi.getProfiles();
      }
      const list = mergeStatusWithProfiles(raw, _cachedProfiles, statusError);

      const hasMoving = list.some((s) => s.axes?.some((a) => a.moving));
      adjustPollingRate(hasMoving);

      motionApi._listeners.forEach((listener) => {
        try {
          listener(list);
        } catch (err) {
          console.error('[motionApi] status listener threw:', err);
        }
      });
    } catch (err) {
      console.debug('[motionApi] standalone tick failed (will retry):', err);
    }
  } finally {
    _standaloneTickInFlight = false;
    scheduleNextStandaloneTick();
  }
}

type StatusCallback = (status: MotionControllerStatus[]) => void;

export const motionApi = {
  getProfiles: async (): Promise<MotionControllerProfile[]> => {
    // profile 结构体包含嵌套数组和具名 string 类型，统一走 HTTP 避开 Wails 反射桥。
    try {
      let profiles = await motionHttpRequest<MotionControllerProfile[]>('/api/motion/profiles');
      profiles = normalizeMotionProfiles(profiles);
      _cachedProfiles = profiles;
      saveProfiles(profiles);
      return profiles;
    } catch (err) {
      console.warn('[motionApi] getProfiles backend unreachable, falling back to local cache:', err);
    }
    const fallback = storedProfiles();
    _cachedProfiles = fallback;
    return fallback;
  },

  getStatusAll: async (): Promise<MotionControllerStatus[]> => {
    const [raw, profiles] = await Promise.all([
      fetchRawStatus(),
      motionApi.getProfiles(),
    ]);
    return mergeStatusWithProfiles(raw, profiles);
  },

  upsertProfile: async (profile: MotionControllerProfile): Promise<void> => {
    // 后端保存失败时抛出，避免本地缓存与后端不一致让用户误以为保存成功
    await motionHttpRequest('/api/motion/profiles', { method: 'PUT', body: JSON.stringify(profile) });
    const profiles = storedProfiles();
    const idx = profiles.findIndex((p) => p.id === profile.id);
    if (idx >= 0) profiles[idx] = profile;
    else profiles.push(profile);
    saveProfiles(profiles);
  },

  deleteProfile: async (id: string): Promise<void> => {
    await motionHttpRequest(`/api/motion/profiles/${id}`, { method: 'DELETE' });
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
      requestFastPolling();
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
      requestFastPolling();
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
      requestFastPolling();
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
      requestFastPolling();
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
      requestFastPolling();
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
      requestFastPolling();
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
      // 与 emergencyStop 行为对齐：复位后立即触发一次高频轮询，
      // 让独立窗口 UI 立刻反映急停解除状态，避免依赖 2s 慢速心跳。
      requestFastPolling();
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

    // 独立窗口（motion 子进程）：通过 HTTP 轮询主进程获取状态，启动时慢速心跳，
    // 电机运动时自动切换到高频轮询，全部停止后降回监控心跳。
    // 使用 setTimeout 串联：前一次 tick 完成后才调度下一次，避免 setInterval
    // 在 tick 耗时较长时强制触发新 tick，造成请求堆积与 WTNMC4A.Status 锁竞争。
    if (isMotionStandaloneMode()) {
      const slot = motionApi as unknown as { _standaloneTimer?: ReturnType<typeof setTimeout> };
      if (!_standaloneActive) {
        _standaloneActive = true;
        _currentPollIntervalMs = SLOW_POLL_MS;
        void tickStandalone();
      }
      return () => {
        motionApi._listeners.delete(cb);
        if (motionApi._listeners.size === 0) {
          _standaloneActive = false;
          if (slot._standaloneTimer) {
            clearTimeout(slot._standaloneTimer);
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

    const slot = motionApi as unknown as { _browserTimer?: ReturnType<typeof setInterval> };
    if (!slot._browserTimer) {
      const tickBrowser = async () => {
        try {
          const list = await motionApi.getStatusAll();
          motionApi._listeners.forEach((listener) => {
            try {
              listener(list);
            } catch (err) {
              console.error('[motionApi] status listener threw (browser):', err);
            }
          });
        } catch (err) {
          console.debug('[motionApi] browser status polling failed (will retry):', err);
        }
      };
      slot._browserTimer = setInterval(() => { void tickBrowser(); }, BROWSER_POLL_MS);
      void tickBrowser();
    }

    return () => {
      motionApi._listeners.delete(cb);
      if (motionApi._listeners.size === 0 && slot._browserTimer) {
        clearInterval(slot._browserTimer);
        slot._browserTimer = undefined;
      }
    };
  },
};
