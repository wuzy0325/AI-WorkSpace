import { describe, expect, it } from 'vitest'

import type { ProbeChannelConfig } from '../types/calibration'
import {
  assignSevenHoleCsvFilesByName,
  assignSevenHoleFilesByName,
  createSevenHoleTraversalProbeChannels,
  detectSevenHoleBatchFormat,
  isTraversalConfigurableProbeChannel,
  isTraversalRequiredProbeChannel,
  normalizeTraversalProbeConfig,
  SEVEN_HOLE_TRAVERSAL_PROBE_CHANNEL_PRESETS,
  TRAVERSAL_PROBE_PRESENTATION,
  type SevenHolePrbFileInfo,
  type SevenHoleTraversalInterpolationConfig,
  type TraversalProbeConfig
} from '../types/traversal'

const innerFile: SevenHolePrbFileInfo = { filePath: 'D:/cal/7.prb', fileName: '7.prb', sector: 7, pointCount: 169 }
const outerFiles = [1, 2, 3, 4, 5, 6].map((n) => ({
  filePath: `D:/cal/${n}.prb`,
  fileName: `${n}.prb`,
  sector: n,
  pointCount: 52
})) as SevenHoleTraversalInterpolationConfig['outerFiles']

const validSevenHoleRaw = {
  probeType: 'seven-hole',
  channels: { probeChannels: [] },
  sevenHolePrb: { kind: 'seven-hole-prb-set', innerFile, outerFiles }
}

describe('七孔通道预设', () => {
  it('9 通道且 CH 映射与后端 roleToLabel 键逐字一致', () => {
    expect(SEVEN_HOLE_TRAVERSAL_PROBE_CHANNEL_PRESETS).toHaveLength(9)
    const roles = SEVEN_HOLE_TRAVERSAL_PROBE_CHANNEL_PRESETS.map((p) => p.role)
    expect(roles).toEqual([
      'sevenHole.p1', 'sevenHole.p2', 'sevenHole.p3', 'sevenHole.p4', 'sevenHole.p5',
      'sevenHole.p6', 'sevenHole.p7', 'sevenHole.pAtm', 'sevenHole.tAtm'
    ])
    const indices = SEVEN_HOLE_TRAVERSAL_PROBE_CHANNEL_PRESETS.map((p) => p.defaultChannelIndex)
    expect(indices).toEqual([0, 1, 2, 3, 4, 5, 6, 16, 17])
  })

  it('createSevenHoleTraversalProbeChannels 生成 9 条默认通道', () => {
    const channels = createSevenHoleTraversalProbeChannels()
    expect(channels).toHaveLength(9)
    expect(channels[6]).toMatchObject({ name: 'P7', role: 'sevenHole.p7' })
  })

  it('七孔 P6/P7 与五孔通道同享必需/可配置判定（含 role 与 name 双路径）', () => {
    // 七孔全部 9 通道均为必需且可配置（含 P6/P7，防回归：曾只查五孔预设导致 P6/P7 被判非必需且保存被过滤）
    for (const preset of SEVEN_HOLE_TRAVERSAL_PROBE_CHANNEL_PRESETS) {
      expect(isTraversalRequiredProbeChannel(preset.role, preset.name), `${preset.role}`).toBe(true)
      expect(isTraversalConfigurableProbeChannel(preset.role, preset.name), `${preset.role}`).toBe(true)
    }
    // name 路径（保存配置按 name 匹配旧行为）
    expect(isTraversalRequiredProbeChannel(undefined, 'P6')).toBe(true)
    expect(isTraversalConfigurableProbeChannel(undefined, 'P7')).toBe(true)
  })
})

describe('assignSevenHoleFilesByName 批量导入文件名分配', () => {
  it('规范命名全套 7 份：7.prb→内区，1~6.prb→扇区 1..6', () => {
    const paths = ['D:/cal/7.prb', 'D:/cal/1.prb', 'D:/cal/2.prb', 'D:/cal/3.prb', 'D:/cal/4.prb', 'D:/cal/5.prb', 'D:/cal/6.prb']
    const r = assignSevenHoleFilesByName(paths)
    expect(r.innerFile).toMatchObject({ filePath: 'D:/cal/7.prb', sector: 7 })
    for (let n = 1; n <= 6; n++) {
      expect(r.outerFiles.get(n)).toMatchObject({ filePath: `D:/cal/${n}.prb`, sector: n })
    }
    expect(r.unmatched).toEqual([])
  })

  it('大小写不敏感 + 仅认规范命名：非常规命名进 unmatched', () => {
    const r = assignSevenHoleFilesByName(['D:/cal/7.PRB', 'D:/cal/probe7.prb', 'D:/cal/3.prb', 'D:/cal/8.prb', 'D:/cal/0.prb'])
    expect(r.innerFile).toMatchObject({ filePath: 'D:/cal/7.PRB' })
    expect(r.outerFiles.get(3)).toMatchObject({ filePath: 'D:/cal/3.prb' })
    expect(r.unmatched).toEqual(['D:/cal/probe7.prb', 'D:/cal/8.prb', 'D:/cal/0.prb'])
  })

  it('同槽位重复命中后来者覆盖', () => {
    const r = assignSevenHoleFilesByName(['D:/a/2.prb', 'D:/b/2.prb'])
    expect(r.outerFiles.get(2)).toMatchObject({ filePath: 'D:/b/2.prb' })
  })
})

describe('assignSevenHoleCsvFilesByName 校准 CSV 文件名分配', () => {
  it('校准导出规范命名全套：小角度区→内区，大角度N区→扇区 N', () => {
    const stem = 'D:/cal/W532.202608.P.7H.1-01-85米每秒（0.242Ma）'
    const paths = [
      `${stem}(大角度3区).csv`,
      `${stem}(小角度区).csv`,
      `${stem}(大角度1区).csv`,
      `${stem}(大角度6区).csv`,
      `${stem}(大角度2区).csv`,
      `${stem}(大角度4区).csv`,
      `${stem}(大角度5区).csv`
    ]
    const r = assignSevenHoleCsvFilesByName(paths)
    expect(r.innerFile).toMatchObject({ sector: 7 })
    for (let n = 1; n <= 6; n++) {
      expect(r.outerFiles.get(n)?.fileName).toContain(`大角度${n}区`)
    }
    expect(r.unmatched).toEqual([])
  })

  it('非常规命名进 unmatched', () => {
    const r = assignSevenHoleCsvFilesByName(['D:/cal/inner.csv', 'D:/cal/大角度7区.csv', 'D:/cal/大角度2区.csv'])
    expect(r.innerFile).toBeNull()
    expect(r.outerFiles.get(2)?.fileName).toBe('大角度2区.csv')
    expect(r.unmatched).toEqual(['D:/cal/inner.csv', 'D:/cal/大角度7区.csv'])
  })

  it('calibration-csv kind 在 normalize 中保留', () => {
    const result = normalizeTraversalProbeConfig({
      probeType: 'seven-hole',
      channels: { probeChannels: [] },
      sevenHolePrb: { kind: 'seven-hole-calibration-csv', innerFile, outerFiles }
    })
    if (result.probeType === 'seven-hole') {
      expect(result.interpolation.kind).toBe('seven-hole-calibration-csv')
    } else {
      throw new Error('expected seven-hole variant')
    }
  })
})

describe('detectSevenHoleBatchFormat 批量导入格式探测', () => {
  it('全 .prb → prb；全 .csv → calibration-csv；混选 → mixed；空 → empty', () => {
    expect(detectSevenHoleBatchFormat(['a/1.prb', 'b/7.PRB'])).toBe('prb')
    expect(detectSevenHoleBatchFormat(['a/x.csv', 'b/y.CSV'])).toBe('calibration-csv')
    expect(detectSevenHoleBatchFormat(['a/1.prb', 'b/x.csv'])).toBe('mixed')
    expect(detectSevenHoleBatchFormat([])).toBe('empty')
  })
})

describe('normalizeTraversalProbeConfig', () => {
  it('旧扁平五孔 JSON（无 probeType）规范化为 five-hole 变体', () => {
    const result = normalizeTraversalProbeConfig({
      channels: { probeChannels: [{ name: 'P1' } as ProbeChannelConfig] },
      prbFile: { filePath: 'a.prb' }
    })
    expect(result.probeType).toBe('five-hole')
    expect(result.interpolation).toEqual({ kind: 'single-prb', file: { filePath: 'a.prb' } })
  })

  it('五孔插值配置优先级与后端恢复链一致：CSV > 多 PRB > 单 PRB > 未配置', () => {
    const base = { channels: { probeChannels: [] } }
    expect(normalizeTraversalProbeConfig(base).interpolation).toEqual({ kind: 'none' })
    expect(
      normalizeTraversalProbeConfig({
        ...base,
        prbFile: { filePath: 'a.prb' },
        useMultiPrb: true,
        multiPrb: { files: [], machNumbers: [], interpolationMode: 'nearest' }
      }).interpolation.kind
    ).toBe('multi-prb')
    expect(
      normalizeTraversalProbeConfig({
        ...base,
        prbFile: { filePath: 'a.prb' },
        useMultiPrb: true,
        multiPrb: { files: [], machNumbers: [], interpolationMode: 'nearest' },
        interpolationAlgorithm: 'new',
        calibrationCsvFile: { filePath: 'c.csv' }
      }).interpolation.kind
    ).toBe('calibration-csv')
  })

  it('七孔合法配置规范化为 seven-hole 变体', () => {
    const result = normalizeTraversalProbeConfig(validSevenHoleRaw)
    expect(result.probeType).toBe('seven-hole')
    if (result.probeType === 'seven-hole') {
      expect(result.interpolation.kind).toBe('seven-hole-prb-set')
      expect(result.interpolation.outerFiles).toHaveLength(6)
    }
  })

  it('未知 probeType 抛错（不静默降级）', () => {
    expect(() => normalizeTraversalProbeConfig({ probeType: 'nine-hole' })).toThrow(/未知探针类型/)
  })

  it('双变体并存合法：五孔激活时 sevenHolePrb 仅作持久化数据透传', () => {
    const result = normalizeTraversalProbeConfig({
      probeType: 'five-hole',
      prbFile: { filePath: 'a.prb' },
      sevenHolePrb: validSevenHoleRaw.sevenHolePrb
    })
    expect(result.probeType).toBe('five-hole')
    expect(result.interpolation).toEqual({ kind: 'single-prb', file: { filePath: 'a.prb' } })
    // 缺省 probeType（旧配置）+ sevenHolePrb 并存同样合法，归一化为五孔
    expect(normalizeTraversalProbeConfig({ sevenHolePrb: validSevenHoleRaw.sevenHolePrb }).probeType).toBe(
      'five-hole'
    )
  })

  it('双变体并存合法：七孔激活时五孔字段仅作持久化数据透传', () => {
    const result = normalizeTraversalProbeConfig({
      ...validSevenHoleRaw,
      prbFile: { filePath: 'a.prb' },
      calibrationCsvFile: { filePath: 'c.csv' }
    })
    expect(result.probeType).toBe('seven-hole')
    if (result.probeType === 'seven-hole') {
      expect(result.interpolation.kind).toBe('seven-hole-prb-set')
    }
  })

  it('七孔文件集不齐抛错', () => {
    expect(() =>
      normalizeTraversalProbeConfig({
        probeType: 'seven-hole',
        sevenHolePrb: { innerFile, outerFiles: outerFiles.slice(0, 5) }
      })
    ).toThrow(/6 份/)
    expect(() => normalizeTraversalProbeConfig({ probeType: 'seven-hole' })).toThrow(/内区文件|sevenHolePrb/)
    expect(() =>
      normalizeTraversalProbeConfig({
        probeType: 'seven-hole',
        sevenHolePrb: { kind: 'wrong-kind', innerFile, outerFiles }
      })
    ).toThrow(/kind/)
  })
})

describe('TRAVERSAL_PROBE_PRESENTATION 展示元数据', () => {
  it('五孔 Alpha=攻角/Beta=侧滑角；七孔 Alpha=侧滑角/Beta=迎角', () => {
    expect(TRAVERSAL_PROBE_PRESENTATION['five-hole']).toMatchObject({
      titleKey: 'fiveHoleTraversalTest',
      alphaLabelKey: 'angleOfAttack',
      betaLabelKey: 'sideslipAngle'
    })
    expect(TRAVERSAL_PROBE_PRESENTATION['seven-hole']).toMatchObject({
      titleKey: 'sevenHoleTraversalTest',
      alphaLabelKey: 'sideslipAngle',
      betaLabelKey: 'angleOfAttack'
    })
  })
})

// =====================================================================
// 类型级契约（vue-tsc 校验）：判别联合互斥性
// =====================================================================

// @ts-expect-error 七孔变体不能赋值五孔 interpolation（kind: 'none' 属五孔）
const badMixed: TraversalProbeConfig = {
  probeType: 'seven-hole',
  probeChannels: [],
  interpolation: { kind: 'none' }
}
void badMixed

// @ts-expect-error 完整七孔 outerFiles 必须恰为 6 项（5 项非法）
const badOuterCount: SevenHoleTraversalInterpolationConfig = { kind: 'seven-hole-prb-set', innerFile, outerFiles: outerFiles.slice(0, 5) }
void badOuterCount

// 合法赋值可被接受（对照组，确保 @ts-expect-error 上方行确为报错点）
const goodSeven: TraversalProbeConfig = {
  probeType: 'seven-hole',
  probeChannels: [],
  interpolation: { kind: 'seven-hole-prb-set', innerFile, outerFiles }
}
void goodSeven
