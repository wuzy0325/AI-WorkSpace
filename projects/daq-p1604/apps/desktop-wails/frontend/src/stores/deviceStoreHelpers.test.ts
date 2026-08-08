import { describe, it, expect } from 'vitest'
import {
  makeProfileId,
  makeDefaultName,
  dedupeName,
  hostKey,
  computeExistingKeys,
  computeExistingNames,
  planScannedAdditions,
} from './deviceStoreHelpers'

// ---------- makeProfileId ----------
describe('makeProfileId', () => {
  it('优先使用 MAC 生成小写去分隔符的 ID', () => {
    expect(makeProfileId({ macAddress: '00:1A:2B:3A:5F:1B', address: '192.168.3.101', port: 9000 }))
      .toBe('p1604-001a2b3a5f1b')
  })

  it('MAC 含短横线时也能正确规范化', () => {
    expect(makeProfileId({ macAddress: '00-1a-2b-3a-5f-1b', address: '192.168.3.101', port: 9000 }))
      .toBe('p1604-001a2b3a5f1b')
  })

  it('无 MAC 时回退到 host-port 规则（点转短横线）', () => {
    expect(makeProfileId({ macAddress: '', address: '192.168.3.101', port: 9000 }))
      .toBe('p1604-192-168-3-101-9000')
  })

  it('MAC 为 undefined 时也回退到 host-port', () => {
    expect(makeProfileId({ address: '10.0.0.5', port: 7000 } as any))
      .toBe('p1604-10-0-0-5-7000')
  })
})

// ---------- makeDefaultName ----------
describe('makeDefaultName', () => {
  it('优先使用 MAC 末 6 位大写作为默认名后缀', () => {
    expect(makeDefaultName({ macAddress: '00:1A:2B:3A:5F:1B', address: '192.168.3.101', port: 9000 }))
      .toBe('DAQ-P-1604-3A5F1B')
  })

  it('无 MAC 时使用 IP 末段 + 端口', () => {
    expect(makeDefaultName({ macAddress: '', address: '192.168.3.101', port: 9000 }))
      .toBe('DAQ-P-1604-101-9000')
  })

  it('IP 只有单段的极端场景仍能兜底', () => {
    expect(makeDefaultName({ macAddress: '', address: 'localhost', port: 7000 }))
      .toBe('DAQ-P-1604-localhost-7000')
  })
})

// ---------- dedupeName ----------
describe('dedupeName', () => {
  it('目标名字不冲突时原样返回', () => {
    expect(dedupeName('DAQ-P-1604-3A5F1B', new Set())).toBe('DAQ-P-1604-3A5F1B')
  })

  it('冲突一次追加 (2)', () => {
    expect(dedupeName('DAQ-P-1604-3A5F1B', new Set(['DAQ-P-1604-3A5F1B'])))
      .toBe('DAQ-P-1604-3A5F1B (2)')
  })

  it('冲突到 (2) 时继续升到 (3)', () => {
    const taken = new Set(['DAQ-P-1604-3A5F1B', 'DAQ-P-1604-3A5F1B (2)'])
    expect(dedupeName('DAQ-P-1604-3A5F1B', taken)).toBe('DAQ-P-1604-3A5F1B (3)')
  })
})

// ---------- hostKey ----------
describe('hostKey', () => {
  it('生成小写 trim 后的 host:addr:port', () => {
    expect(hostKey('  192.168.3.101 ', 9000)).toBe('host:192.168.3.101:9000')
  })
})

// ---------- computeExistingKeys ----------
describe('computeExistingKeys', () => {
  it('从 profiles 生成 host key 集合', () => {
    const keys = computeExistingKeys([
      { id: 'a', address: '192.168.3.101', port: 9000 } as any,
      { id: 'b', address: '10.0.0.5', port: 7000 } as any,
    ])
    expect(keys.has('host:192.168.3.101:9000')).toBe(true)
    expect(keys.has('host:10.0.0.5:7000')).toBe(true)
    expect(keys.size).toBe(2)
  })
})

// ---------- computeExistingNames ----------
describe('computeExistingNames', () => {
  it('从 profiles 收集非空设备名', () => {
    const names = computeExistingNames([
      { id: 'a', name: '工位A', address: '192.168.3.101', port: 9000 } as any,
      { id: 'b', name: '', address: '10.0.0.5', port: 7000 } as any,
      { id: 'c', name: '工位A', address: '10.0.0.6', port: 7000 } as any,
    ])
    expect(names.has('工位A')).toBe(true)
    expect(names.size).toBe(1)
  })

  it('空列表返回空集合', () => {
    expect(computeExistingNames([]).size).toBe(0)
  })
})

// ---------- planScannedAdditions ----------
describe('planScannedAdditions', () => {
  it('全新设备全部通过，名字互不相同', () => {
    const plan = planScannedAdditions({
      inputs: [
        { address: '192.168.3.101', port: 9000, macAddress: '00:1A:2B:3A:5F:1B', serialNumber: 'SN1' },
        { address: '192.168.3.102', port: 9000, macAddress: '00:1A:2B:9C:82:AA', serialNumber: 'SN2' },
      ],
      existingProfiles: [],
      defaultAutoConnect: true,
    })
    expect(plan.toAdd).toHaveLength(2)
    expect(plan.skipped).toHaveLength(0)
    expect(plan.toAdd[0]!.name).toBe('DAQ-P-1604-3A5F1B')
    expect(plan.toAdd[1]!.name).toBe('DAQ-P-1604-9C82AA')
    expect(plan.toAdd[0]!.id).toBe('p1604-001a2b3a5f1b')
    expect(plan.toAdd[0]!.autoConnect).toBe(true)
  })

  it('已存在的 host:port 会被 skip 并标记原因', () => {
    const plan = planScannedAdditions({
      inputs: [
        { address: '192.168.3.101', port: 9000, macAddress: '00:1A:2B:3A:5F:1B', serialNumber: 'SN1' },
      ],
      existingProfiles: [
        { id: 'p1604-old', address: '192.168.3.101', port: 9000 } as any,
      ],
      defaultAutoConnect: true,
    })
    expect(plan.toAdd).toHaveLength(0)
    expect(plan.skipped).toHaveLength(1)
    expect(plan.skipped[0]!.reason).toBe('duplicate-address')
  })

  it('两台 MAC 末6 相同时第二台名字自动追加 (2)', () => {
    const plan = planScannedAdditions({
      inputs: [
        { address: '192.168.3.101', port: 9000, macAddress: '00:1A:2B:3A:5F:1B', serialNumber: 'SN1' },
        { address: '192.168.3.102', port: 9000, macAddress: '00:00:00:3A:5F:1B', serialNumber: 'SN2' },
      ],
      existingProfiles: [],
      defaultAutoConnect: false,
    })
    expect(plan.toAdd[0]!.name).toBe('DAQ-P-1604-3A5F1B')
    expect(plan.toAdd[1]!.name).toBe('DAQ-P-1604-3A5F1B (2)')
    expect(plan.toAdd[0]!.autoConnect).toBe(false)
    expect(plan.toAdd[1]!.autoConnect).toBe(false)
  })

  it('新名字与已有 profile 名字冲突时也会追加 (2)', () => {
    const plan = planScannedAdditions({
      inputs: [
        { address: '192.168.3.102', port: 9000, macAddress: '00:1A:2B:3A:5F:1B', serialNumber: 'SN1' },
      ],
      existingProfiles: [
        { id: 'other', name: 'DAQ-P-1604-3A5F1B', address: '192.168.3.99', port: 9000 } as any,
      ],
      defaultAutoConnect: true,
    })
    expect(plan.toAdd[0]!.name).toBe('DAQ-P-1604-3A5F1B (2)')
  })

  it('用户内联覆盖名字后不再走默认名逻辑', () => {
    const plan = planScannedAdditions({
      inputs: [
        {
          address: '192.168.3.101',
          port: 9000,
          macAddress: '00:1A:2B:3A:5F:1B',
          serialNumber: 'SN1',
          name: '工位A',
        },
      ],
      existingProfiles: [],
      defaultAutoConnect: true,
    })
    expect(plan.toAdd[0]!.name).toBe('工位A')
  })

  it('用户覆盖名字与已有 profile 冲突时同样加 (2) 兜底', () => {
    const plan = planScannedAdditions({
      inputs: [
        {
          address: '192.168.3.101',
          port: 9000,
          macAddress: '00:1A:2B:3A:5F:1B',
          serialNumber: 'SN1',
          name: '工位A',
        },
      ],
      existingProfiles: [
        { id: 'other', name: '工位A', address: '192.168.3.99', port: 9000 } as any,
      ],
      defaultAutoConnect: true,
    })
    expect(plan.toAdd[0]!.name).toBe('工位A (2)')
  })

  /**
   * 回归测试：ScanResultList 的 ScanSelectionItem 通过 v-model 传到 AppShell，
   * 再原样传入 addScannedProfiles → planScannedAdditions。
   * ScanSelectionItem.name 字段必须被识别为用户覆盖名，不能因字段名不匹配
   * 而静默回退到默认名（曾因 overrideName 与 name 不一致导致改名不生效）。
   */
  it('ScanSelectionItem 结构（含 id + name）直接传入时名字生效', () => {
    const plan = planScannedAdditions({
      inputs: [
        {
          id: 'p1604-001a2b3a5f1b',
          name: '风洞北侧',
          address: '192.168.3.101',
          port: 9000,
          macAddress: '00:1A:2B:3A:5F:1B',
          serialNumber: 'SN1',
        } as any,
      ],
      existingProfiles: [],
      defaultAutoConnect: true,
    })
    expect(plan.toAdd[0]!.name).toBe('风洞北侧')
  })

  it('ScanSelectionItem.name 为空字符串时回退到默认名', () => {
    const plan = planScannedAdditions({
      inputs: [
        {
          id: 'p1604-001a2b3a5f1b',
          name: '',
          address: '192.168.3.101',
          port: 9000,
          macAddress: '00:1A:2B:3A:5F:1B',
          serialNumber: 'SN1',
        } as any,
      ],
      existingProfiles: [],
      defaultAutoConnect: true,
    })
    expect(plan.toAdd[0]!.name).toBe('DAQ-P-1604-3A5F1B')
  })
})
