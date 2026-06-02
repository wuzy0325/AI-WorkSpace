import type { AxisName } from '@shared/types/motion';
import { container } from '@/core/container';
import type { IMotionService, IFeedbackService } from '@/core/types';

/**
 * 运动控制服务 Hook
 * 
 * 提供类型安全的运动控制服务访问接口。
 * 使用 composable 模式，确保在 Vue 组件中正确使用。
 * 
 * @example
 * ```typescript
 * const motion = useMotionService();
 * 
 * // 使用适配器（返回 void）
 * await motion.adapter.connect('controller-1');
 * await motion.adapter.moveTo('controller-1', 'X', 100);
 * 
 * // 使用 Store（返回 void）
 * await motion.store.connect('controller-1');
 * 
 * // 使用原始 API（返回 boolean）
 * const success = await motion.api.connect('controller-1');
 * ```
 */
export function useMotionService(): IMotionService {
  return container.motion;
}

/**
 * 反馈服务 Hook
 * 
 * 提供类型安全的用户反馈服务访问接口。
 * 包括 Toast 消息和确认对话框功能。
 * 
 * @example
 * ```typescript
 * const feedback = useFeedbackService();
 * 
 * // Toast 消息
 * feedback.toast.success('操作成功！');
 * feedback.toast.error('发生错误', 5000);
 * 
 * // 确认对话框
 * const confirmed = await feedback.confirm('确定要删除吗？', {
 *   title: '删除确认',
 *   confirmText: '删除',
 *   cancelText: '取消'
 * });
 * ```
 */
export function useFeedbackService(): IFeedbackService {
  return container.feedback;
}

/**
 * 组合式函数：使用运动服务并进行错误处理
 * 
 * 封装常见的运动控制操作，自动处理错误和用户反馈。
 * 
 * @param options 配置选项
 * @param options.showError 是否在错误时显示 Toast，默认为 true
 * @param options.errorPrefix 错误消息前缀，默认为 '运动控制失败'
 */
export function useMotionWithFeedback(options: {
  showError?: boolean;
  errorPrefix?: string;
} = {}): IMotionService & {
  /**
   * 安全执行运动操作，自动处理错误
   */
  safeMoveTo: (id: string, axis: AxisName, position: number) => Promise<boolean>;
  safeHome: (id: string, axis: AxisName) => Promise<boolean>;
  safeConnect: (id: string) => Promise<boolean>;
} {
  const { showError = true, errorPrefix = '运动控制失败' } = options;
  const motion = container.motion;
  const feedback = container.feedback;
  
  return {
    ...motion,
    
    safeMoveTo: async (id: string, axis: AxisName, position: number): Promise<boolean> => {
      try {
        await motion.adapter.moveTo(id, axis, position);
        return true;
      } catch (error) {
        if (showError) {
          const message = error instanceof Error ? error.message : String(error);
          feedback.toast.error(`${errorPrefix}: ${message}`);
        }
        return false;
      }
    },
    
    safeHome: async (id: string, axis: AxisName): Promise<boolean> => {
      try {
        await motion.adapter.home(id, axis);
        return true;
      } catch (error) {
        if (showError) {
          const message = error instanceof Error ? error.message : String(error);
          feedback.toast.error(`${errorPrefix}: ${message}`);
        }
        return false;
      }
    },
    
    safeConnect: async (id: string): Promise<boolean> => {
      try {
        await motion.adapter.connect(id);
        return true;
      } catch (error) {
        if (showError) {
          const message = error instanceof Error ? error.message : String(error);
          feedback.toast.error(`${errorPrefix}: ${message}`);
        }
        return false;
      }
    },
  };
}
