/**
 * 核心模块 - 统一导出
 * 
 * 导出所有核心功能模块，提供统一的访问入口。
 */

// 依赖注入容器
export { AppContainer, container, type AppContainer as AppContainerClass } from './container';
export type { MotionService, FeedbackService } from './container';

// 类型定义
export * from './types';

// Store ID 管理
export * from './storeIds';
