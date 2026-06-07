import {
  GetProfiles,
  UpsertProfile,
  DeleteProfile,
  Connect,
  Disconnect,
  StartAcquisition,
  StopAcquisition,
  ApplyConfig,
  GetStatus,
  ScanDevices,
} from '../../wailsjs/go/backend/App'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'

export interface ChannelConfig {
  index: number
  name: string
  enabled: boolean
  unit: string
  color: string
  precision: number
  rangeMin?: number
  rangeMax?: number
  thermocoupleType: string
}

export interface T1603Config {
  thermocoupleTypes: string
  channelMask: string
  samplingRate: number
  averageCount: number
  showTimestamp: boolean
  showSequence: boolean
  autoConnect: boolean
}

export const TC_RANGES: Record<string, { min: number; max: number }> = {
  K: { min: -200, max: 1372 },
  J: { min: -210, max: 1200 },
  T: { min: -270, max: 400 },
  E: { min: -270, max: 1000 },
  N: { min: -270, max: 1300 },
  R: { min: -50, max: 1768 },
  S: { min: -50, max: 1768 },
  B: { min: 0, max: 1820 },
}

export interface TemperatureProfile {
  id: string
  name: string
  address: string
  port: number
  samplingRate: number
  channels: ChannelConfig[]
  t1603Config: T1603Config
  createdAt: number
}

export interface DeviceState {
  profile: TemperatureProfile
  status: number
  error: string
  connectedAt: number
  acquiringAt: number
  samplingRate: number
}

export interface ScanResult {
  id: string
  name: string
  address: string
  port: number
  macAddress?: string
  serialNumber?: string
  firmwareVersion?: string
}

export interface TemperatureSnapshot {
  deviceId: string
  timestamp: number
  hardwareTimestamp: number
  values: number[]
  unit: string
}

export type LogLevel = 'debug' | 'info' | 'warn' | 'error'
export type LogCategory = 'system' | 'hardware-send' | 'hardware-recv' | 'acquisition'

export interface DeviceLogEvent {
  level: LogLevel
  category: LogCategory
  deviceId?: string
  source: string
  message: string
  detail?: string
  timestamp: number
}

function toPlain<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

export function getProfiles(): Promise<TemperatureProfile[]> {
  return GetProfiles() as any
}

export function upsertProfile(profile: TemperatureProfile): Promise<void> {
  return UpsertProfile(toPlain(profile) as any) as Promise<void>
}

export function deleteProfile(id: string): Promise<void> {
  return DeleteProfile(id) as Promise<void>
}

export function connect(id: string): Promise<void> {
  return Connect(id) as Promise<void>
}

export function disconnect(id: string): Promise<void> {
  return Disconnect(id) as Promise<void>
}

export function startAcquisition(id: string): Promise<void> {
  return StartAcquisition(id) as Promise<void>
}

export function stopAcquisition(id: string): Promise<void> {
  return StopAcquisition(id) as Promise<void>
}

export function getStatus(id: string): Promise<DeviceState | boolean> {
  return GetStatus(id) as any
}

export function applyConfig(id: string, cfg: T1603Config): Promise<void> {
  return ApplyConfig(id, toPlain(cfg) as any) as Promise<void>
}

export function scanDevices(): Promise<ScanResult[]> {
  return ScanDevices() as any
}

export function onPayload(handler: (snapshot: TemperatureSnapshot) => void): void {
  EventsOn('daq:payload', handler)
}

export function offPayload(): void {
  EventsOff('daq:payload')
}

export function onLog(handler: (entry: DeviceLogEvent) => void): void {
  EventsOn('daq:log', handler)
}

export function offLog(): void {
  EventsOff('daq:log')
}
