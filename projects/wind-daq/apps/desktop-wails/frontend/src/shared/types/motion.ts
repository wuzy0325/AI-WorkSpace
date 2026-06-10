export type AxisName = 'X' | 'Y' | 'Z' | 'U';

export type AxisKind = 'LINEAR' | 'ROTARY';

export type MotionControllerType = 'SIMULATED-MC' | 'B140-MC' | 'WTNMC4A-MC';

export interface AxisConfig {
  name: AxisName;
  enabled: boolean;
  kind: AxisKind;
  maxSpeed?: number;
  minLimit?: number;
  maxLimit?: number;
  inverted?: boolean;
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
