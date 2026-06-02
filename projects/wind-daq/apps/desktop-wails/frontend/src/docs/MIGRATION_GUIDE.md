/**
 * 依赖注入迁移指南
 * 
 * 本文档指导如何将现有代码迁移到新的依赖注入系统。
 * 
 * ## 迁移步骤
 * 
 * ### 1. 从直接导入改为使用 Composables
 * 
 * **旧代码**:
 * ```typescript
 * import { motionApi } from '@/api/motionApi';
 * import { useMotionStore } from '@/stores/motionStore';
 * 
 * const motionStore = useMotionStore();
 * await motionStore.connect('controller-1');
 * ```
 * 
 * **新代码**:
 * ```typescript
 * import { useMotionService } from '@/composables/useServices';
 * 
 * const motion = useMotionService();
 * await motion.store.connect('controller-1');
 * // 或者使用适配器
 * await motion.adapter.connect('controller-1');
 * ```
 * 
 * ### 2. 处理返回值类型差异
 * 
 * **motionApi** (返回 `Promise<boolean>`):
 * ```typescript
 * const success = await motion.api.connect('controller-1');
 * if (success) {
 *   console.log('连接成功');
 * }
 * ```
 * 
 * **motionStore** (返回 `Promise<void>`):
 * ```typescript
 * await motion.store.connect('controller-1');
 * // 无需检查返回值
 * ```
 * 
 * **motionAdapter** (返回 `Promise<void>`):
 * ```typescript
 * await motion.adapter.connect('controller-1');
 * // 适配器自动处理类型转换
 * ```
 * 
 * ### 3. 使用带错误处理的 Composable
 * 
 * ```typescript
 * import { useMotionWithFeedback } from '@/composables/useServices';
 * 
 * const motion = useMotionWithFeedback({
 *   showError: true,
 *   errorPrefix: '移动失败'
 * });
 * 
 * const success = await motion.safeMoveTo('controller-1', 'X', 100);
 * if (success) {
 *   console.log('移动成功');
 * }
 * ```
 * 
 * ### 4. 反馈服务使用
 * 
 * **旧代码**:
 * ```typescript
 * import { useFeedbackStore } from '@/stores/feedbackStore';
 * 
 * const feedback = useFeedbackStore();
 * feedback.pushToast('操作成功', 'success');
 * const confirmed = await feedback.confirm('确定删除？');
 * ```
 * 
 * **新代码**:
 * ```typescript
 * import { useFeedbackService } from '@/composables/useServices';
 * 
 * const feedback = useFeedbackService();
 * feedback.toast.success('操作成功');
 * const confirmed = await feedback.confirm('确定删除？', {
 *   title: '删除确认',
 *   confirmText: '删除',
 *   cancelText: '取消'
 * });
 * ```
 * 
 * ## 迁移进度追踪
 * 
 * | 组件/文件 | 当前状态 | 目标状态 | 完成度 |
 * |---------|---------|---------|--------|
 * | motionStore | 使用中 | 使用 useMotionService | 0% |
 * | feedbackStore | 使用中 | 使用 useFeedbackService | 0% |
 * | motionApi | 使用中 | 通过容器访问 | 0% |
 * 
 * ## 注意事项
 * 
 * 1. **向后兼容**: 在完全迁移前，保持对旧 API 的支持
 * 2. **类型安全**: 使用 TypeScript 类型定义确保类型正确
 * 3. **测试覆盖**: 为每个迁移编写单元测试
 * 4. **渐进式迁移**: 逐个模块迁移，避免大规模重构
 */
