// 运动控制 Pinia Store
// 依赖注入的 API，由各项目提供具体实现

import { defineStore } from 'pinia';
import { ref } from 'vue';
import type { MotionControllerProfile, MotionControllerStatus, AxisName } from '../types/motion';
import { getMotionApi, getToastService } from './motionApi';

export const useMotionStore = defineStore('motion', () => {
  const profiles = ref<MotionControllerProfile[]>([]);
  const statusList = ref<MotionControllerStatus[]>([]);

  function statusById(id: string): MotionControllerStatus | undefined {
    return statusList.value.find((s: MotionControllerStatus) => s.id === id);
  }

  async function refreshProfiles(): Promise<void> {
    try {
      const api = getMotionApi();
      const result = await api.getProfiles();
      profiles.value = Array.isArray(result) ? result : [];
    } catch (e) {
      const toast = getToastService();
      toast.pushToast(`加载配置失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error');
    }
  }

  async function refreshStatus(): Promise<void> {
    try {
      const api = getMotionApi();
      const result = await api.getStatusAll();
      statusList.value = Array.isArray(result) ? result : [];
    } catch {
      // 状态刷为非关键操作，失败时不干扰用户操作
    }
  }

  function attachStatusListener(): () => void {
    const api = getMotionApi();
    return api.onStatusUpdated((all) => {
      statusList.value = Array.isArray(all) ? all : [];
    });
  }

  async function upsertProfile(profile: MotionControllerProfile): Promise<void> {
    const toast = getToastService();
    try {
      const api = getMotionApi();
      await api.upsertProfile(profile);
      await refreshProfiles();
    } catch (e) {
      toast.pushToast(`保存配置失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error');
      throw e;
    }
  }

  async function deleteProfile(id: string): Promise<void> {
    const toast = getToastService();
    try {
      const api = getMotionApi();
      await api.deleteProfile(id);
      await refreshProfiles();
    } catch (e) {
      toast.pushToast(`删除配置失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error');
      throw e;
    }
  }

  async function connect(id: string): Promise<void> {
    const toast = getToastService();
    try {
      const api = getMotionApi();
      await api.connect(id);
    } catch (e) {
      toast.pushToast(`连接失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error');
      throw e;
    }
  }

  async function disconnect(id: string): Promise<void> {
    const toast = getToastService();
    try {
      const api = getMotionApi();
      await api.disconnect(id);
    } catch (e) {
      toast.pushToast(`断开失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error');
    }
  }

  async function moveTo(id: string, axis: AxisName, position: number): Promise<void> {
    const toast = getToastService();
    try {
      const api = getMotionApi();
      await api.moveTo(id, axis, position);
    } catch (e) {
      toast.pushToast(`移动失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error');
    }
  }

  async function moveBy(id: string, axis: AxisName, delta: number): Promise<void> {
    const toast = getToastService();
    try {
      const api = getMotionApi();
      await api.moveBy(id, axis, delta);
    } catch (e) {
      toast.pushToast(`移动失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error');
    }
  }

  async function jog(id: string, axis: AxisName, direction: 'forward' | 'reverse', speed?: number): Promise<void> {
    const toast = getToastService();
    try {
      const api = getMotionApi();
      await api.jog(id, axis, direction, speed);
    } catch (e) {
      toast.pushToast(`点动失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error');
    }
  }

  async function home(id: string, axis: AxisName): Promise<void> {
    const toast = getToastService();
    try {
      const api = getMotionApi();
      await api.home(id, axis);
    } catch (e) {
      toast.pushToast(`回零失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error');
    }
  }

  async function stop(id: string, axis?: AxisName): Promise<void> {
    const toast = getToastService();
    try {
      const api = getMotionApi();
      await api.stop(id, axis);
    } catch (e) {
      toast.pushToast(`停止失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error');
    }
  }

  async function emergencyStop(id: string): Promise<void> {
    const toast = getToastService();
    try {
      const api = getMotionApi();
      await api.emergencyStop(id);
    } catch (e) {
      toast.pushToast(`急停失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error');
    }
  }

  async function resetEmergencyStop(id: string): Promise<void> {
    const toast = getToastService();
    try {
      const api = getMotionApi();
      await api.resetEmergencyStop(id);
    } catch (e) {
      toast.pushToast(`解除急停失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error');
    }
  }

  async function definePosition(id: string, axis: AxisName, position: number): Promise<void> {
    const toast = getToastService();
    try {
      const api = getMotionApi();
      await api.definePosition(id, axis, position);
    } catch (e) {
      toast.pushToast(`定义位置失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error');
    }
  }

  function clearError(id: string): void {
    const idx = statusList.value.findIndex((s: MotionControllerStatus) => s.id === id);
    if (idx !== -1 && statusList.value[idx].lastError) {
      statusList.value[idx].lastError = '';
    }
  }

  return {
    profiles,
    statusList,
    statusById,
    refreshProfiles,
    refreshStatus,
    attachStatusListener,
    upsertProfile,
    deleteProfile,
    connect,
    disconnect,
    moveTo,
    moveBy,
    jog,
    home,
    stop,
    emergencyStop,
    resetEmergencyStop,
    definePosition,
    clearError,
  };
});
