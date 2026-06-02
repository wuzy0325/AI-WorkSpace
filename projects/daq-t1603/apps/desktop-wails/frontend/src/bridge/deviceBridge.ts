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
}

export interface T1603Config {
  thermocoupleType: string
  coldJunction: string
  filterHz: number
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

export const FILTER_HZ_OPTIONS = [50, 60, 400, 1000] as const

export function filterHzToLabel(hz: number): string {
  return `${hz}Hz`
}

export function filterLabelToHz(label: string): number {
  const num = parseInt(label, 10)
  return isNaN(num) ? 50 : num
}

export function coldJunctionToLabel(cj: string): string {
  return cj === 'internal' ? 'internal' : cj
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

export interface TemperatureSnapshot {
  deviceId: string
  timestamp: number
  values: number[]
  unit: string
}

export function getProfiles(): Promise<TemperatureProfile[]> {
  return GetProfiles() as any
}

export function upsertProfile(profile: TemperatureProfile): Promise<void> {
  return UpsertProfile(profile as any) as Promise<void>
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
  return ApplyConfig(id, cfg as any) as Promise<void>
}

export function onPayload(handler: (snapshot: TemperatureSnapshot) => void): void {
  EventsOn('daq:payload', handler)
}

export function offPayload(): void {
  EventsOff('daq:payload')
}
