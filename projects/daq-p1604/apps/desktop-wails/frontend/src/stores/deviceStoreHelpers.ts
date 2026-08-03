/**
 * deviceStore 纯工具函数集合。
 *
 * 抽出到独立文件的原因：
 * 1. 便于单元测试（无需 mock @bridge/deviceBridge 与 pinia）
 * 2. 与 store 状态解耦，输入输出纯函数化
 */

import type { PressureProfile } from '@bridge/deviceBridge'

/** 扫描到的原始设备信息（供 planScannedAdditions / makeProfileId / makeDefaultName 使用） */
export interface ScannedDeviceInput {
  address: string
  port: number
  macAddress?: string
  serialNumber?: string
  /** 用户在扫描弹窗中内联覆盖的名字；未填写时走默认名生成 */
  overrideName?: string
}

/** 落地为 profile 前的中间结果（不含 channels / p1604Config 等模板字段） */
export interface PlannedProfile {
  id: string
  name: string
  address: string
  port: number
  autoConnect: boolean
}

/** 被去重跳过的条目 */
export interface SkippedScanEntry {
  address: string
  port: number
  macAddress: string
  reason: 'duplicate-address'
}

/** planScannedAdditions 的输出 */
export interface ScanAdditionPlan {
  toAdd: PlannedProfile[]
  skipped: SkippedScanEntry[]
}

/**
 * addScannedProfiles 的最终返回值。
 * 抽出到 helpers 供外部消费者（composable / 测试）直接引用，
 * 避免只能通过 `ReturnType<typeof addScannedProfiles>` 反推类型。
 */
export interface AddScannedResult {
  added: import('@bridge/deviceBridge').PressureProfile[]
  skipped: SkippedScanEntry[]
  failed: Array<{ input: PlannedProfile; error: string }>
}

/** 规范化 MAC：去掉 :/-，转小写 */
function normalizeMac(mac: string | undefined): string {
  if (!mac) return ''
  return mac.replace(/[:\-]/g, '').toLowerCase()
}

/**
 * 生成可复现的 profile ID。
 * 有 MAC 用 MAC；无 MAC 用 host + port（点换短横线）。
 * 目的：同一台真机多次扫描落到同一 ID，配合 json_config 按 ID 去重即天然幂等。
 */
export function makeProfileId(input: { macAddress?: string; address: string; port: number }): string {
  const mac = normalizeMac(input.macAddress)
  if (mac) return `p1604-${mac}`
  const hostSlug = input.address.replace(/\./g, '-')
  return `p1604-${hostSlug}-${input.port}`
}

/**
 * 生成默认设备名。
 * 有 MAC → DAQ-P-1604-<末6位大写>；无 MAC → DAQ-P-1604-<IP末段>-<port>。
 */
export function makeDefaultName(input: { macAddress?: string; address: string; port: number }): string {
  const mac = (input.macAddress ?? '').replace(/[:\-]/g, '').toUpperCase()
  if (mac.length >= 6) {
    return `DAQ-P-1604-${mac.slice(-6)}`
  }
  const parts = input.address.split('.')
  const ipTail = parts.length > 1 ? parts[parts.length - 1]! : input.address
  return `DAQ-P-1604-${ipTail}-${input.port}`
}

/**
 * 冲突时自动追加 (2)/(3)/... 直到唯一。
 * @param base 目标名字
 * @param existing 已被占用的名字集合（含运行时 profiles + 本批已排布名字）
 */
export function dedupeName(base: string, existing: Set<string>): string {
  if (!existing.has(base)) return base
  let n = 2
  while (existing.has(`${base} (${n})`)) n += 1
  return `${base} (${n})`
}

/** 生成用于去重的 host key：格式 `host:<小写 trim addr>:<port>` */
export function hostKey(address: string, port: number): string {
  return `host:${address.trim().toLowerCase()}:${port}`
}

/** 从当前 profiles 生成 host key 集合，供扫描弹窗置灰使用 */
export function computeExistingKeys(profiles: PressureProfile[]): Set<string> {
  const keys = new Set<string>()
  for (const p of profiles) {
    keys.add(hostKey(p.address, p.port))
  }
  return keys
}

/**
 * 从当前 profiles 生成设备名集合（忽略空名）。
 * 供"名字全局唯一"校验使用：手动添加、配置面板改名、扫描批量添加共用同一语义。
 */
export function computeExistingNames(profiles: PressureProfile[]): Set<string> {
  const names = new Set<string>()
  for (const p of profiles) {
    if (p.name) names.add(p.name)
  }
  return names
}

/**
 * 计算一次批量添加计划。
 *
 * 输入：扫描到的原始输入 + 现有 profile 列表 + 默认自动连接开关值
 * 输出：可落库的 PlannedProfile[] + 因去重跳过的条目
 *
 * 规则：
 * 1. host:addr:port 命中现有 profile → skip（reason: duplicate-address）
 * 2. 名字冲突（含现有 profile 名字 + 本批已用名字）→ dedupeName 追加 (2)/(3)
 * 3. overrideName 优先于默认名，但同样受名字冲突兜底
 * 4. autoConnect 值由 defaultAutoConnect 决定，同批统一
 */
export function planScannedAdditions(params: {
  inputs: ScannedDeviceInput[]
  existingProfiles: PressureProfile[]
  defaultAutoConnect: boolean
}): ScanAdditionPlan {
  const { inputs, existingProfiles, defaultAutoConnect } = params

  const existingKeys = computeExistingKeys(existingProfiles)
  // 名字冲突集合：包含现有 profile 名字，用于 dedupeName 命中判断
  const usedNames = computeExistingNames(existingProfiles)

  const toAdd: PlannedProfile[] = []
  const skipped: SkippedScanEntry[] = []

  for (const input of inputs) {
    const key = hostKey(input.address, input.port)
    if (existingKeys.has(key)) {
      // 已存在相同地址：跳过，不影响其它条目
      skipped.push({
        address: input.address,
        port: input.port,
        macAddress: input.macAddress ?? '',
        reason: 'duplicate-address',
      })
      continue
    }

    // 生成候选名字：优先用户覆盖名，否则用默认名
    const rawName = (input.overrideName ?? '').trim() || makeDefaultName(input)
    const finalName = dedupeName(rawName, usedNames)
    usedNames.add(finalName)

    toAdd.push({
      id: makeProfileId(input),
      name: finalName,
      address: input.address,
      port: input.port,
      autoConnect: defaultAutoConnect,
    })
    // 本批地址也计入去重集，防止同批扫到两次同 IP 造成重复
    existingKeys.add(key)
  }

  return { toAdd, skipped }
}
