/**
 * 依赖注入容器 - 类型定义
 * 
 * 为依赖注入系统提供完整的类型支持。
 * 
 * 注意：此文件复用共享模块中的类型定义，以确保类型一致性。
 */

// 复用共享类型，避免重复定义
export type {
  AxisName,
  AxisKind,
  AxisConfig,
  MotionControllerType,
  MotionControllerProfile,
  AxisStatus,
  MotionControllerStatus,
} from '@shared/types/motion';

// ========================================
// 本地扩展类型（仅用于容器内部）
// ========================================

/**
 * 运动轴配置（本地扩展版本）
 * 
 * 用于运动控制服务中的轴配置。
 */
export interface MotionAxisConfig {
  name: string;
  enabled: boolean;
  kind: 'LINEAR' | 'ROTARY';
  maxSpeed: number;
  stepsPerRev: number;
  microSteps: number;
  lead: number;
  gearRatio: number;
  positionSource: 'register' | 'encoder';
  encoderScale: number;
}

// ========================================
// 反馈相关类型
// ========================================

export type ToastLevel = 'info' | 'success' | 'warning' | 'error';

export interface ToastMessage {
  id: number;
  level: ToastLevel;
  message: string;
  durationMs: number;
}

export interface ConfirmState {
  open: boolean;
  message: string;
  title: string;
  confirmText: string;
  cancelText: string;
}

// ========================================
// 服务接口定义
// ========================================

/**
 * 运动控制 API 接口
 * 
 * 原始 API 接口，所有方法返回 Promise<boolean>。
 */
export interface IMotionApi {
  getProfiles(): Promise<import('@shared/types/motion').MotionControllerProfile[]>;
  getStatusAll(): Promise<import('@shared/types/motion').MotionControllerStatus[]>;
  upsertProfile(profile: import('@shared/types/motion').MotionControllerProfile): Promise<void>;
  deleteProfile(id: string): Promise<void>;
  connect(id: string): Promise<boolean>;
  disconnect(id: string): Promise<void>;
  moveTo(id: string, axis: import('@shared/types/motion').AxisName, position: number): Promise<boolean>;
  moveBy(id: string, axis: import('@shared/types/motion').AxisName, delta: number): Promise<boolean>;
  jog(id: string, axis: import('@shared/types/motion').AxisName, direction: 'forward' | 'reverse', speed?: number): Promise<boolean>;
  home(id: string, axis: import('@shared/types/motion').AxisName): Promise<boolean>;
  stop(id: string, axis?: import('@shared/types/motion').AxisName): Promise<boolean>;
  emergencyStop(id: string): Promise<boolean>;
  definePosition(id: string, axis: import('@shared/types/motion').AxisName, position: number): Promise<boolean>;
  resetEmergencyStop(id: string): Promise<boolean>;
  onStatusUpdated(callback: (status: import('@shared/types/motion').MotionControllerStatus[]) => void): () => void;
}

/**
 * 运动控制 Store 接口
 * 
 * Pinia Store 接口，所有方法返回 Promise<void>。
 */
export interface IMotionStore {
  profiles: import('@shared/types/motion').MotionControllerProfile[];
  statusList: import('@shared/types/motion').MotionControllerStatus[];
  statusById(id: string): import('@shared/types/motion').MotionControllerStatus | undefined;
  refreshProfiles(): Promise<void>;
  refreshStatus(): Promise<void>;
  attachStatusListener(): () => void;
  upsertProfile(profile: import('@shared/types/motion').MotionControllerProfile): Promise<void>;
  deleteProfile(id: string): Promise<void>;
  connect(id: string): Promise<void>;
  disconnect(id: string): Promise<void>;
  moveTo(id: string, axis: import('@shared/types/motion').AxisName, position: number): Promise<void>;
  moveBy(id: string, axis: import('@shared/types/motion').AxisName, delta: number): Promise<void>;
  jog(id: string, axis: import('@shared/types/motion').AxisName, direction: 'forward' | 'reverse', speed?: number): Promise<void>;
  home(id: string, axis: import('@shared/types/motion').AxisName): Promise<void>;
  stop(id: string, axis?: import('@shared/types/motion').AxisName): Promise<void>;
  emergencyStop(id: string): Promise<void>;
  definePosition(id: string, axis: import('@shared/types/motion').AxisName, position: number): Promise<void>;
  resetEmergencyStop(id: string): Promise<void>;
}

/**
 * 运动控制适配器接口
 * 
 * 类型适配器接口，确保所有方法返回 Promise<void>。
 */
export interface IMotionAdapter {
  connect(id: string): Promise<void>;
  disconnect(id: string): Promise<void>;
  moveTo(id: string, axis: import('@shared/types/motion').AxisName, position: number): Promise<void>;
  moveBy(id: string, axis: import('@shared/types/motion').AxisName, delta: number): Promise<void>;
  jog(id: string, axis: import('@shared/types/motion').AxisName, direction: 'forward' | 'reverse', speed?: number): Promise<void>;
  home(id: string, axis: import('@shared/types/motion').AxisName): Promise<void>;
  stop(id: string, axis?: import('@shared/types/motion').AxisName): Promise<void>;
  emergencyStop(id: string): Promise<void>;
  definePosition(id: string, axis: import('@shared/types/motion').AxisName, position: number): Promise<void>;
  resetEmergencyStop(id: string): Promise<void>;
}

/**
 * 运动控制服务接口
 * 
 * 完整的服务接口，包含 API、Store 和适配器。
 */
export interface IMotionService {
  api: IMotionApi;
  store: IMotionStore;
  adapter: IMotionAdapter;
}

/**
 * 反馈 Store 接口
 */
export interface IFeedbackStore {
  toasts: ToastMessage[];
  confirmState: ConfirmState;
  pushToast(message: string, level: ToastLevel, durationMs?: number): number;
  removeToast(id: number): void;
  confirm(
    message: string,
    options?: Partial<Pick<ConfirmState, 'title' | 'confirmText' | 'cancelText'>>
  ): Promise<boolean>;
  resolveConfirm(accepted: boolean): void;
}

/**
 * Toast 服务接口
 */
export interface IToastService {
  info(message: string, durationMs?: number): number;
  success(message: string, durationMs?: number): number;
  warning(message: string, durationMs?: number): number;
  error(message: string, durationMs?: number): number;
}

/**
 * 反馈服务接口
 * 
 * 完整的服务接口，包含 Store 和便捷方法。
 */
export interface IFeedbackService {
  store: IFeedbackStore;
  toast: IToastService;
  confirm(
    message: string,
    options?: { title?: string; confirmText?: string; cancelText?: string }
  ): Promise<boolean>;
}

// ========================================
// 容器接口定义
// ========================================

/**
 * 应用容器接口
 */
export interface IAppContainer {
  motion: IMotionService;
  feedback: IFeedbackService;
  reset(): void;
}
