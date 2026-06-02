// 运动控制 API 接口定义
// 各项目需要实现此接口

import type { AxisName, MotionControllerProfile, MotionControllerStatus } from '../types/motion';

/**
 * 运动控制器 API 接口
 * 各项目（motion-controller、wind-daq）需要提供自己的实现
 */
export interface IMotionApi {
  getProfiles(): Promise<MotionControllerProfile[]>;
  getStatusAll(): Promise<MotionControllerStatus[]>;
  upsertProfile(profile: MotionControllerProfile): Promise<void>;
  deleteProfile(id: string): Promise<void>;
  connect(id: string): Promise<void>;
  disconnect(id: string): Promise<void>;
  moveTo(id: string, axis: AxisName, position: number): Promise<void>;
  moveBy(id: string, axis: AxisName, delta: number): Promise<void>;
  jog(id: string, axis: AxisName, direction: 'forward' | 'reverse', speed?: number): Promise<void>;
  home(id: string, axis: AxisName): Promise<void>;
  stop(id: string, axis?: AxisName): Promise<void>;
  emergencyStop(id: string): Promise<void>;
  resetEmergencyStop(id: string): Promise<void>;
  definePosition(id: string, axis: AxisName, position: number): Promise<void>;
  onStatusUpdated(callback: (status: MotionControllerStatus[]) => void): () => void;
}

/**
 * Toast 提示接口
 */
export interface IToastService {
  pushToast(message: string, type?: 'info' | 'warning' | 'error' | 'success'): void;
}

// 全局 API 实例（需要各项目初始化）
let motionApiInstance: IMotionApi | null = null;

// 全局 Toast 服务（需要各项目初始化）
let toastServiceInstance: IToastService | null = null;

/**
 * 设置运动控制器 API 实例
 */
export function setMotionApi(api: IMotionApi): void {
  motionApiInstance = api;
}

/**
 * 获取运动控制器 API 实例
 */
export function getMotionApi(): IMotionApi {
  if (!motionApiInstance) {
    throw new Error('Motion API 未初始化，请先调用 setMotionApi()');
  }
  return motionApiInstance;
}

/**
 * 设置 Toast 服务实例
 */
export function setToastService(service: IToastService): void {
  toastServiceInstance = service;
}

/**
 * 获取 Toast 服务实例
 */
export function getToastService(): IToastService {
  if (!toastServiceInstance) {
    // 提供一个空实现，避免崩溃
    return {
      pushToast: () => { /* 空实现 */ }
    };
  }
  return toastServiceInstance;
}
