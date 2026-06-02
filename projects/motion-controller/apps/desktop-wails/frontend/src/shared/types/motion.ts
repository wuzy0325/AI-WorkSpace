export type AxisName = 'X' | 'Y' | 'Z' | 'U';

export type AxisKind = 'LINEAR' | 'ROTARY';

export type PositionSource = 'register' | 'encoder';

export type MotionControllerType = 'SIMULATED-MC' | 'B140-MC' | 'WTNMC4A-MC';

export interface AxisEncoderCompensationConfig {
  enabled?: boolean;
  tolerance?: number;
  maxCycles?: number;
  settleMs?: number;
  minStep?: number;
  timeoutMs?: number;
}

export interface AxisConfig {
  name: AxisName;
  enabled: boolean;
  kind: AxisKind;
  /** 步距角(°/step)，如 1.8° 对应 200 步/转 */
  stepsPerRev?: number;
  microSteps?: number;
  lead?: number;
  gearRatio?: number;
  maxSpeed?: number;
  minLimit?: number;
  maxLimit?: number;
  inverted?: boolean;
  encoderInverted?: boolean;
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
