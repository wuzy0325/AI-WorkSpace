// 共享运动控制模块入口

// 类型导出
export * from './types/motion';

// Store 导出
export { useMotionStore, setMotionApi, getMotionApi, setToastService, getToastService } from './stores';
export type { IMotionApi, IToastService } from './stores';

// i18n 导出
export { motionZh, motionEn } from './i18n';

// 组件导出
export { MotionControlPanel } from './components';
