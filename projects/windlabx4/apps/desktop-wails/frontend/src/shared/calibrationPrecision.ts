import type { CalibrationConfig } from '@shared/types/calibration'
import { calibrationApi } from '@api/calibrationApi'

const FALLBACK_PROBE_PRECISION = 3 as const
const FALLBACK_MACH_PRECISION = 4 as const
const FALLBACK_VELOCITY_PRECISION = 3 as const

export const DEFAULT_CALIBRATION_PROBE_PRECISION = FALLBACK_PROBE_PRECISION
export const DEFAULT_CALIBRATION_MACH_PRECISION = FALLBACK_MACH_PRECISION
export const DEFAULT_CALIBRATION_VELOCITY_PRECISION = FALLBACK_VELOCITY_PRECISION

let defaultsCache: {
  probePrecision: number
  machPrecision: number
  velocityPrecision: number
} | null = null

let fetchPromise: Promise<void> | null = null

async function ensureDefaults(): Promise<void> {
  if (defaultsCache !== null) return
  if (fetchPromise !== null) return fetchPromise
  fetchPromise = (async () => {
    try {
      const result = await calibrationApi.getPrecisionDefaults()
      if (result) {
        defaultsCache = result
      }
    } catch {
      // use fallback
    }
    if (!defaultsCache) {
      defaultsCache = {
        probePrecision: FALLBACK_PROBE_PRECISION,
        machPrecision: FALLBACK_MACH_PRECISION,
        velocityPrecision: FALLBACK_VELOCITY_PRECISION,
      }
    }
  })()
  return fetchPromise
}

function getDefaults(): { probePrecision: number; machPrecision: number; velocityPrecision: number } {
  return defaultsCache ?? {
    probePrecision: FALLBACK_PROBE_PRECISION,
    machPrecision: FALLBACK_MACH_PRECISION,
    velocityPrecision: FALLBACK_VELOCITY_PRECISION,
  }
}

export async function initPrecisionDefaults(): Promise<void> {
  await ensureDefaults()
}

export function applyCalibrationPrecisionDefaults(config: CalibrationConfig): CalibrationConfig {
  const d = getDefaults()
  const result = { ...config }
  if (!result.derivedValuePrecision) {
    result.derivedValuePrecision = {
      machNumber: d.machPrecision,
      velocity: d.velocityPrecision,
    }
  }
  if (result.probeChannels) {
    result.probeChannels = result.probeChannels.map((ch) => ({
      ...ch,
      precision: ch.precision ?? d.probePrecision,
    }))
  }
  return result
}

export function getProbeChannelPrecision(config: CalibrationConfig | null | undefined, role: string): number {
  const d = getDefaults()
  if (!config?.probeChannels) return d.probePrecision
  const ch = config.probeChannels.find((c) => c.role === role)
  return ch?.precision ?? d.probePrecision
}

export function getDerivedValuePrecision(
  config: CalibrationConfig | null | undefined,
  key: 'machNumber' | 'velocity'
): number {
  const d = getDefaults()
  if (!config?.derivedValuePrecision) {
    return key === 'machNumber' ? d.machPrecision : d.velocityPrecision
  }
  return config.derivedValuePrecision[key] ?? (key === 'machNumber' ? d.machPrecision : d.velocityPrecision)
}
