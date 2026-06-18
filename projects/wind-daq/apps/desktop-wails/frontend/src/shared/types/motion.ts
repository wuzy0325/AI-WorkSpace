export type AxisName = 'X' | 'Y' | 'Z' | 'U';

export type AxisKind = 'LINEAR' | 'ROTARY';

export type PositionSource = 'register' | 'encoder';

export type MotionControllerType = 'SIMULATED-MC' | 'B140-MC' | 'WTNMC4A-MC';

/**
 * 编码器补偿参数
 *
 * 当位置来源选择编码器时启用，用于消除编码器反馈与指令位置之间的静态偏差。
 */
export interface AxisEncoderCompensationConfig {
  enabled: boolean;
  tolerance: number;
  maxCycles: number;
  settleMs: number;
  minStep: number;
  timeoutMs: number;
}

/**
 * 单轴配置
 *
 * 包含机械、电气及运动限制参数。stepsPerRev / microSteps / lead / gearRatio
 * 用于把电机步数换算为工程单位（mm 或 °）。
 */
export interface AxisConfig {
  name: AxisName;
  enabled: boolean;
  kind: AxisKind;
  maxSpeed?: number;
  minLimit?: number;
  maxLimit?: number;
  inverted?: boolean;
  encoderInverted?: boolean;
  stepsPerRev?: number;
  microSteps?: number;
  lead?: number;
  gearRatio?: number;
  positionSource?: PositionSource;
  encoderScale?: number;
  encoderCompensation?: AxisEncoderCompensationConfig;
}

export interface MotionControllerProfile {
  id: string;
  name: string;
  type: MotionControllerType;
  address: string;
  port: number;
  autoConnect: boolean;
  axes: AxisConfig[];
}

export interface AxisStatus {
  name: AxisName;
  position: number;
  moving: boolean;
  homed: boolean;
  posLimit?: boolean;
  negLimit?: boolean;
  compensating?: boolean;
  compensationError?: string;
  positionError?: number;
}

export interface MotionControllerStatus {
  id: string;
  name: string;
  type: MotionControllerType;
  connected: boolean;
  emergencyStopped?: boolean;
  axes: AxisStatus[];
  lastError?: string;
}
