import { motionApi } from '@api/motionApi';
import { useFeedbackStore } from '@stores/feedbackStore';
import { useMotionStore } from '@stores/motionStore';
import type { AxisName } from '@shared/types/motion';

export interface MotionService {
  api: typeof motionApi;
  store: ReturnType<typeof useMotionStore>;
  adapter: {
    connect: (id: string) => Promise<void>;
    disconnect: (id: string) => Promise<void>;
    moveTo: (id: string, axis: AxisName, position: number) => Promise<void>;
    moveBy: (id: string, axis: AxisName, delta: number) => Promise<void>;
    jog: (id: string, axis: AxisName, direction: 'forward' | 'reverse', speed?: number) => Promise<void>;
    home: (id: string, axis: AxisName) => Promise<void>;
    stop: (id: string, axis?: AxisName) => Promise<void>;
    emergencyStop: (id: string) => Promise<void>;
    definePosition: (id: string, axis: AxisName, position: number) => Promise<void>;
    resetEmergencyStop: (id: string) => Promise<void>;
  };
}

export interface FeedbackService {
  store: ReturnType<typeof useFeedbackStore>;
  toast: {
    info: (message: string, durationMs?: number) => number;
    success: (message: string, durationMs?: number) => number;
    warning: (message: string, durationMs?: number) => number;
    error: (message: string, durationMs?: number) => number;
  };
  confirm: (
    message: string,
    options?: { title?: string; confirmText?: string; cancelText?: string }
  ) => Promise<boolean>;
}

export class AppContainer {
  private static instance: AppContainer;
  private _motionService: MotionService | null = null;
  private _feedbackService: FeedbackService | null = null;

  private constructor() {}

  public static getInstance(): AppContainer {
    if (!AppContainer.instance) {
      AppContainer.instance = new AppContainer();
    }
    return AppContainer.instance;
  }

  public get motion(): MotionService {
    if (!this._motionService) {
      const motionStore = useMotionStore();
      this._motionService = {
        api: motionApi,
        store: motionStore,
        adapter: {
          connect: (id: string): Promise<void> => motionApi.connect(id).then(() => {}),
          disconnect: (id: string): Promise<void> => motionApi.disconnect(id).then(() => {}),
          moveTo: (id: string, axis: AxisName, position: number): Promise<void> => motionApi.moveTo(id, axis, position).then(() => {}),
          moveBy: (id: string, axis: AxisName, delta: number): Promise<void> => motionApi.moveBy(id, axis, delta).then(() => {}),
          jog: (id: string, axis: AxisName, direction: 'forward' | 'reverse', speed?: number): Promise<void> => motionApi.jog(id, axis, direction, speed).then(() => {}),
          home: (id: string, axis: AxisName): Promise<void> => motionApi.home(id, axis).then(() => {}),
          stop: (id: string, axis?: AxisName): Promise<void> => motionApi.stop(id, axis).then(() => {}),
          emergencyStop: (id: string): Promise<void> => motionApi.emergencyStop(id).then(() => {}),
          definePosition: (id: string, axis: AxisName, position: number): Promise<void> => motionApi.definePosition(id, axis, position).then(() => {}),
          resetEmergencyStop: (id: string): Promise<void> => motionApi.resetEmergencyStop(id).then(() => {}),
        },
      };
    }
    return this._motionService;
  }

  public get feedback(): FeedbackService {
    if (!this._feedbackService) {
      const feedbackStore = useFeedbackStore();
      this._feedbackService = {
        store: feedbackStore,
        toast: {
          info: (message: string, durationMs = 2800): number => feedbackStore.pushToast(message, 'info', durationMs),
          success: (message: string, durationMs = 2800): number => feedbackStore.pushToast(message, 'success', durationMs),
          warning: (message: string, durationMs = 3500): number => feedbackStore.pushToast(message, 'warning', durationMs),
          error: (message: string, durationMs = 4000): number => feedbackStore.pushToast(message, 'error', durationMs),
        },
        confirm: (
          message: string,
          options?: { title?: string; confirmText?: string; cancelText?: string }
        ): Promise<boolean> => feedbackStore.confirm(message, options),
      };
    }
    return this._feedbackService;
  }

  public reset(): void {
    this._motionService = null;
    this._feedbackService = null;
  }
}

export const container = AppContainer.getInstance();