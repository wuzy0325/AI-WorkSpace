import { describe, expect, it } from 'vitest'
import {
  filterTraversalAxisOptions,
  findDuplicateChannelBindings,
  findDuplicateMotionAxisBindings,
  findOccupiedMotionAxisDirections,
  findTraversalAxisKindIssues,
  getTraversalLayoutPoints,
  hasDuplicateChannel,
  hasDuplicateMotionAxis,
  normalizeTraversalLayoutRanges,
  requiredTraversalAxisKind,
  type ProbeChannelConfig,
  type TraversalLayout,
  type TraversalMotionAxisConfig
} from '@shared/types/traversal'

describe('normalizeTraversalLayoutRanges', () => {
  it('uses rectangle segment ranges instead of stale persisted bounds', () => {
    const layout: TraversalLayout = {
      pattern: 'rectangle',
      primaryAxis: 'x',
      rectangle: {
        xMin: -30,
        xMax: 30,
        xStepSegments: [{ start: 30, end: 50, step: 10 }],
        yMin: -30,
        yMax: 30,
        yStepSegments: [{ start: -30, end: 30, step: 30 }]
      }
    }

    const directPoints = getTraversalLayoutPoints(layout)
    const normalized = normalizeTraversalLayoutRanges(layout)
    const points = getTraversalLayoutPoints(normalized)

    expect(normalized.rectangle).toMatchObject({
      xMin: 30,
      xMax: 50,
      yMin: -30,
      yMax: 30
    })
    expect(directPoints).toEqual(points)
    expect(points).toHaveLength(9)
    expect([...new Set(points.map((point) => point.x))]).toEqual([30, 40, 50])
    expect([...new Set(points.map((point) => point.y))]).toEqual([-30, 0, 30])
  })

  it('leaves non-rectangle layouts unchanged', () => {
    const layout: TraversalLayout = {
      pattern: 'custom',
      custom: { points: [{ x: 1, y: 2, z: 3, u: 4 }] }
    }

    expect(normalizeTraversalLayoutRanges(layout)).toBe(layout)
  })

  it('repairs a persisted rectangle that collapsed to one X column', () => {
    const layout: TraversalLayout = {
      pattern: 'rectangle',
      rectangle: {
        xMin: 30,
        xMax: 30,
        xStepSegments: [{ start: 30, end: 30, step: 5 }],
        yMin: -30,
        yMax: 30,
        yStepSegments: [{ start: -30, end: 30, step: 5 }]
      }
    }

    const normalized = normalizeTraversalLayoutRanges(layout)
    const points = getTraversalLayoutPoints(layout)

    expect(normalized.rectangle?.xStepSegments).toEqual([{ start: -30, end: 30, step: 5 }])
    expect(points).toHaveLength(169)
    expect([...new Set(points.map((point) => point.x))]).toHaveLength(13)
    expect([...new Set(points.map((point) => point.y))]).toHaveLength(13)
  })
})

describe('sector traversal preview', () => {
  it('keeps Cartesian geometry in the preview while motion uses radius and angle targets', () => {
    const layout: TraversalLayout = {
      pattern: 'sector',
      sector: {
        centerX: 0,
        centerY: 0,
        radiusMin: 100,
        radiusMax: 100,
        radialStepSegments: [{ start: 100, end: 100, step: 10 }],
        angleStart: 0,
        angleEnd: 90,
        angularStepSegments: [{ start: 0, end: 90, step: 90 }]
      }
    }

    const points = getTraversalLayoutPoints(layout)

    expect(points).toHaveLength(2)
    expect(points[0]).toMatchObject({ x: 100, y: 0 })
    expect(points[1].x).toBeCloseTo(0)
    expect(points[1].y).toBeCloseTo(100)
  })
})

describe('sector axis kind constraint', () => {
  const profiles = [
    {
      id: 'mc1',
      axes: [
        { name: 'X' as const, kind: 'LINEAR' as const, enabled: true },
        { name: 'Y' as const, kind: 'LINEAR' as const, enabled: true },
        { name: 'Z' as const, kind: 'LINEAR' as const, enabled: false },
        { name: 'U' as const, kind: 'ROTARY' as const, enabled: true }
      ]
    }
  ]

  it('requires LINEAR for X and ROTARY for Y only in sector pattern', () => {
    expect(requiredTraversalAxisKind('sector', 'X')).toBe('LINEAR')
    expect(requiredTraversalAxisKind('sector', 'Y')).toBe('ROTARY')
    expect(requiredTraversalAxisKind('line', 'X')).toBeNull()
    expect(requiredTraversalAxisKind('rectangle', 'Y')).toBeNull()
    expect(requiredTraversalAxisKind('custom', 'Y')).toBeNull()
  })

  it('flags bindings whose axis kind does not match the sector requirement', () => {
    const motionAxes = [
      { name: 'X' as const, controllerId: 'mc1', axis: 'U' as const },
      { name: 'Y' as const, controllerId: 'mc1', axis: 'Y' as const }
    ]

    const issues = findTraversalAxisKindIssues(motionAxes, profiles, 'sector')

    expect(issues).toEqual([
      { name: 'X', axis: 'U', requiredKind: 'LINEAR', actualKind: 'ROTARY' },
      { name: 'Y', axis: 'Y', requiredKind: 'ROTARY', actualKind: 'LINEAR' }
    ])
  })

  it('flags axis missing from the controller profile as actualKind null', () => {
    // Z 存在但 disabled：kind 可读（LINEAR）与要求匹配，仍因停用被判不合规，
    // 与 filterTraversalAxisOptions 排除停用轴的行为对齐（选不了的轴不能通过校验）。
    const disabledAxis = [{ name: 'X' as const, controllerId: 'mc1', axis: 'Z' as const }]
    expect(findTraversalAxisKindIssues(disabledAxis, profiles, 'sector')).toEqual([
      { name: 'X', axis: 'Z', requiredKind: 'LINEAR', actualKind: 'LINEAR', disabled: true }
    ])

    // 换成 profile 中不存在的轴名才报 actualKind null
    const missingAxis = [{ name: 'X' as const, controllerId: 'mc1', axis: 'U' as const }]
    const profileWithoutU = [{ id: 'mc1', axes: profiles[0].axes.filter((a) => a.name !== 'U') }]
    expect(findTraversalAxisKindIssues(missingAxis, profileWithoutU, 'sector')).toEqual([
      { name: 'X', axis: 'U', requiredKind: 'LINEAR', actualKind: null }
    ])
  })

  it('skips validation when controller is unbound or pattern is not sector', () => {
    const unbound = [{ name: 'X' as const, controllerId: '', axis: 'U' as const }]
    expect(findTraversalAxisKindIssues(unbound, profiles, 'sector')).toEqual([])

    const rotaryX = [{ name: 'X' as const, controllerId: 'mc1', axis: 'U' as const }]
    expect(findTraversalAxisKindIssues(rotaryX, profiles, 'rectangle')).toEqual([])
    expect(findTraversalAxisKindIssues(rotaryX, [], 'sector')).toEqual([])
  })

  it('filters axis options by required kind, excluding disabled axes', () => {
    expect(filterTraversalAxisOptions(profiles[0], 'sector', 'X')).toEqual(['X', 'Y'])
    expect(filterTraversalAxisOptions(profiles[0], 'sector', 'Y')).toEqual(['U'])
    expect(filterTraversalAxisOptions(profiles[0], 'line', 'X')).toBeNull()
    expect(filterTraversalAxisOptions(null, 'sector', 'X')).toBeNull()
  })
})


describe('findDuplicateChannelBindings（设备+通道号粒度）', () => {
  function probe(name: string, deviceId: string, channelIndex: number, enabled = true): ProbeChannelConfig {
    return {
      name,
      role: 'fiveHole.p1',
      channel: { deviceId, channelIndex },
      enabled,
      precision: 3
    }
  }

  it('不同设备相同通道号不算冲突（五孔在设备1的1通道、大气压在设备2的1通道）', () => {
    const channels = [
      probe('P1', 'dev-1', 1),
      probe('Patm', 'dev-2', 1),
      probe('Tatm', 'dev-2', 2)
    ]
    expect(findDuplicateChannelBindings(channels).size).toBe(0)
    expect(hasDuplicateChannel(channels)).toBe(false)
  })

  it('同一设备同一通道被两个探针绑定才报冲突', () => {
    const channels = [
      probe('P1', 'dev-1', 1),
      probe('Patm', 'dev-1', 1)
    ]
    const dupes = findDuplicateChannelBindings(channels)
    expect(dupes.size).toBe(1)
    expect(hasDuplicateChannel(channels)).toBe(true)
  })

  it('未启用通道不参与检测；channelIndex < 0 视为未分配', () => {
    const channels = [
      probe('P1', 'dev-1', 1),
      probe('Patm', 'dev-1', 1, false),
      probe('Tatm', 'dev-1', -1)
    ]
    expect(hasDuplicateChannel(channels)).toBe(false)
  })
})

describe('findDuplicateMotionAxisBindings（控制器+物理轴粒度）', () => {
  function axis(name: 'X' | 'Y' | 'Z' | 'U', controllerId: string, physicalAxis: 'X' | 'Y' | 'Z' | 'U'): TraversalMotionAxisConfig {
    return { name, controllerId, axis: physicalAxis }
  }

  it('不同控制器相同物理轴名不算冲突（X→ctrlA.X、Z→ctrlB.X 各自独立编号）', () => {
    const axes = [
      axis('X', 'ctrl-A', 'X'),
      axis('Y', 'ctrl-A', 'Y'),
      axis('Z', 'ctrl-B', 'X'),
      axis('U', 'ctrl-B', 'Y')
    ]
    expect(findDuplicateMotionAxisBindings(axes).size).toBe(0)
    expect(hasDuplicateMotionAxis(axes)).toBe(false)
  })

  it('同一控制器同一物理轴被两个方向绑定才报冲突（X→ctrlA.X + Z→ctrlA.X）', () => {
    const axes = [
      axis('X', 'ctrl-A', 'X'),
      axis('Y', 'ctrl-A', 'Y'),
      axis('Z', 'ctrl-A', 'X'),
      axis('U', 'ctrl-A', 'U')
    ]
    const dupes = findDuplicateMotionAxisBindings(axes)
    expect(dupes.size).toBe(1)
    expect(dupes.has('ctrl-A X')).toBe(true)
    expect(hasDuplicateMotionAxis(axes)).toBe(true)
  })

  it('未绑定控制器的行不参与检测（controllerId 为空时跳过）', () => {
    const axes = [
      axis('X', 'ctrl-A', 'X'),
      axis('Y', '', 'X'),
      axis('Z', '', 'X'),
      axis('U', 'ctrl-A', 'X')
    ]
    // Y/Z 行 controllerId 为空，只有 X 和 U 在 ctrl-A 上冲突
    const dupes = findDuplicateMotionAxisBindings(axes)
    expect(dupes.size).toBe(1)
    expect(dupes.has('ctrl-A X')).toBe(true)
  })

  it('空数组与全空绑定时无冲突', () => {
    expect(hasDuplicateMotionAxis([])).toBe(false)
    expect(hasDuplicateMotionAxis([
      axis('X', '', 'X'),
      axis('Y', '', 'Y')
    ])).toBe(false)
  })
})

describe('findOccupiedMotionAxisDirections（下拉选项占用预告）', () => {
  function axis(name: 'X' | 'Y' | 'Z' | 'U', controllerId: string, physicalAxis: 'X' | 'Y' | 'Z' | 'U'): TraversalMotionAxisConfig {
    return { name, controllerId, axis: physicalAxis }
  }

  it('同控制器其他方向已绑定的物理轴被报告为占用', () => {
    const axes = [
      axis('X', 'ctrl-A', 'X'),
      axis('Y', 'ctrl-A', 'Y')
    ]
    // 当前行为 Y：物理轴 X 被 X 方向占用；Y 轴是当前行自己绑的，不算占用
    const occupied = findOccupiedMotionAxisDirections(axes, axes[1])
    expect(occupied.size).toBe(1)
    expect(occupied.get('X')).toEqual(['X'])
  })

  it('同一物理轴被多个其他方向占用时按行顺序全部列出', () => {
    const axes = [
      axis('X', 'ctrl-A', 'X'),
      axis('Y', 'ctrl-A', 'Y'),
      axis('Z', 'ctrl-A', 'X'),
      axis('U', 'ctrl-A', 'U')
    ]
    // 当前行为 U：物理轴 X 同时被 X、Z 两个方向绑定
    const occupied = findOccupiedMotionAxisDirections(axes, axes[3])
    expect(occupied.get('X')).toEqual(['X', 'Z'])
    expect(occupied.get('Y')).toEqual(['Y'])
  })

  it('不同控制器的同名物理轴不算占用（各控制器独立编号）', () => {
    const axes = [
      axis('X', 'ctrl-A', 'X'),
      axis('Y', 'ctrl-B', 'X')
    ]
    // 当前行 Y 在 ctrl-B 上：ctrl-A 的 X 轴绑定与其无关
    expect(findOccupiedMotionAxisDirections(axes, axes[1]).size).toBe(0)
  })

  it('当前行自身的绑定被排除（按方向名识别）', () => {
    const axes = [
      axis('X', 'ctrl-A', 'X'),
      axis('Y', 'ctrl-A', 'Y')
    ]
    // 当前行为 X：Y 行占用了物理轴 Y 会如实报告，但当前行自己绑的物理轴 X 不自我标注
    const occupied = findOccupiedMotionAxisDirections(axes, axes[0])
    expect(occupied.has('X')).toBe(false)
    expect(occupied.get('Y')).toEqual(['Y'])
    // 冲突态下双方互见：X、Y 同绑 ctrl-A.X 时，X 行看物理轴 X 被 Y 方向占用
    const dupAxes = [
      axis('X', 'ctrl-A', 'X'),
      axis('Y', 'ctrl-A', 'X')
    ]
    expect(findOccupiedMotionAxisDirections(dupAxes, dupAxes[0]).get('X')).toEqual(['Y'])
  })

  it('未绑定控制器的行不参与占用；当前行未选控制器时无占用关系', () => {
    const axes = [
      axis('X', '', 'X'),
      axis('Y', '', 'X'),
      axis('Z', 'ctrl-A', 'X')
    ]
    // 当前行 Z：X/Y 未绑定控制器，跳过
    expect(findOccupiedMotionAxisDirections(axes, axes[2]).size).toBe(0)
    // 当前行 X 未选控制器：没有"同控制器"概念，恒为空
    expect(findOccupiedMotionAxisDirections(axes, axes[0]).size).toBe(0)
  })

  it('空数组返回空映射', () => {
    expect(findOccupiedMotionAxisDirections([], axis('X', 'ctrl-A', 'X')).size).toBe(0)
  })
})
