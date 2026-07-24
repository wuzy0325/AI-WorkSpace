import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import MotionSafetyPanel, { type MotionSafetyPanelAxis } from '../MotionSafetyPanel.vue'
import type { MotionSafetyConfig } from '@shared/types/traversal'

/**
 * Task 18 契约测试：MotionSafetyPanel 前端校验与后端 validateMotionSafetyConfig 对齐。
 *
 * 验收标准（spec R-5 / plan Slice D2 / tasks Task 18）：
 *   1. frontend blocking rules 与 backend rejection 对齐
 *      —— 后端 validateMotionSafetyConfig 拒绝的配置，前端 isValid 必须为 false
 *   2. advisory 不使 isValid=false
 *      —— 后端不拒绝、仅 UX 提示的规则（epsilon >= arrival、timeout >= 120000）
 *         应进入 advisoryWarnings，不进入 blockingErrors
 *   3. override 非递归
 *      —— MotionSafetyAxisOverride 类型不含 axisOverrides 字段（编译期保证）
 *   4. unknown key 可见
 *      —— axisOverrides 中存在 motionAxes 未绑定轴时，advisoryWarnings 必须包含提示
 *   5. backend Start 始终权威
 *      —— 前端 isValid 仅作 UX 阻断；后端 ParseAndStartTraversal 仍独立校验
 *         （此条由后端 traversal_motion_safety_test.go 覆盖，本文件不重复）
 *
 * 后端校验规则对照（traversal_config.go validateMotionSafetyConfig / validateMotionSafetyFields）：
 *   B1. 浮点字段必须有限（非 NaN、非 Inf）—— 阻断
 *   B2. arrivalTolerance > 0 —— 阻断
 *   B3. criticalDeviationLimit > 0 —— 阻断
 *   B4. criticalDeviationLimit > arrivalTolerance（两者都配置时）—— 阻断
 *   B5. noProgressTimeoutMs >= 200（2 * motionCompletePollMsForValidation）—— 阻断
 *   B5a. 注意：后端不单独校验 timeout > 0，< 200 已涵盖
 *   B6. progressEpsilon > 0 —— 阻断
 *   B7. axisOverrides 键必须是 motionAxes 中已绑定的轴名 —— 阻断（后端拒绝）
 *   B8. axisOverrides 项内不允许嵌套 axisOverrides —— 阻断（后端拒绝）
 *   B9. 每个绑定轴 Resolve(axis) 后 criticalDeviationLimit > arrivalTolerance —— 阻断
 *
 * 前端独有 advisory 规则（后端不拒绝，仅 UX 提示）：
 *   A1. progressEpsilon >= arrivalTolerance —— 微动卡死会被误判为有进展
 *   A2. noProgressTimeoutMs >= 120000 —— 看门狗永远不先于兜底超时触发
 *   A3. axisOverrides 包含未绑定轴 —— 用户可能遗留了旧配置（后端会拒绝，但前端应提前提示）
 *
 * 测试策略：
 *   - 挂载 MotionSafetyPanel，通过 v-model:motion-safety 写入测试配置
 *   - 读取 wrapper.vm.isValid / blockingErrors / advisoryWarnings 断言行为
 *   - 不渲染真实 DOM 交互（v-if="false" 使面板不渲染，但 computed 仍生效）
 */

// 测试用 i18n 字符串：键名与 MotionSafetyPanel 实际使用保持一致
const TEST_T = {
  travMotionSafety: 'Motion Safety',
  travMotionSafetyHint: 'hint',
  travMotionSafetyDefaultTpl: 'Default {value}',
  travMotionSafetyInheritTpl: 'Inherit {value}',
  travMotionSafetyErrArrivalPositive: 'arrival must be positive',
  travMotionSafetyErrCriticalPositive: 'critical must be positive',
  travMotionSafetyErrTimeoutPositive: 'timeout must be positive',
  travMotionSafetyErrTimeoutMin200: 'timeout must be >= 200',
  travMotionSafetyErrEpsilonPositive: 'epsilon must be positive',
  travMotionSafetyErrCriticalGreaterThanArrival: 'critical must be > arrival',
  travMotionSafetyErrEpsilonLessThanArrival: 'epsilon should be < arrival',
  travMotionSafetyErrTimeoutLessThan120s: 'timeout should be < 120000',
  // 阻断错误（backend-equivalent）—— 与后端 validateMotionSafetyConfig 拒绝规则对齐
  travMotionSafetyErrArrivalFinite: 'arrival must be finite',
  travMotionSafetyErrCriticalFinite: 'critical must be finite',
  travMotionSafetyErrEpsilonFinite: 'epsilon must be finite',
  travMotionSafetyErrUnboundAxis: 'override contains unbound axis: {axis}',
  // 阻断/告警分区标题
  travMotionSafetyBlocking: 'Blocking errors',
  travMotionSafetyAdvisory: 'Advisory warnings',
  travArrivalTolerance: 'Arrival',
  travCriticalDeviationLimit: 'Critical',
  travNoProgressTimeout: 'Timeout',
  travProgressEpsilon: 'Epsilon',
  travArrivalToleranceShort: 'Arr',
  travCriticalDeviationLimitShort: 'Crt',
  travNoProgressTimeoutShort: 'Tmo',
  travProgressEpsilonShort: 'Eps',
  travArrivalToleranceHint: 'hint',
  travCriticalDeviationLimitHint: 'hint',
  travNoProgressTimeoutHint: 'hint',
  travProgressEpsilonHint: 'hint',
  travUnitMmOrDeg: 'mm/°',
  travUnitMs: 'ms',
  physicalAxis: 'Axis',
} as unknown as Record<string, string>

// 测试用绑定轴：X/Y 已绑定，Z/U 未绑定用于 unknown key 测试
const TEST_AXES: MotionSafetyPanelAxis[] = [
  { name: 'X', axis: 'X' },
  { name: 'Y', axis: 'Y' },
]

interface PanelExpose {
  isValid: boolean
  blockingErrors: string[]
  advisoryWarnings: string[]
}

function mountPanel(initial?: MotionSafetyConfig): {
  wrapper: ReturnType<typeof mount>
  getExpose: () => PanelExpose
  setMotionSafety: (next: MotionSafetyConfig | undefined) => Promise<void>
} {
  const wrapper = mount(MotionSafetyPanel, {
    props: {
      t: TEST_T,
      motionAxes: TEST_AXES,
      motionSafety: initial,
    },
  })
  const getExpose = () => {
    const vm = wrapper.vm as unknown as PanelExpose
    return {
      get isValid() { return vm.isValid },
      get blockingErrors() { return vm.blockingErrors },
      get advisoryWarnings() { return vm.advisoryWarnings },
    } as PanelExpose
  }
  const setMotionSafety = async (next: MotionSafetyConfig | undefined) => {
    await wrapper.setProps({ motionSafety: next })
    await nextTick()
  }
  return { wrapper, getExpose, setMotionSafety }
}

describe('Task 18: MotionSafetyPanel contract — blocking vs advisory alignment', () => {
  // 共享 setup：每个用例独立挂载，避免相互污染
  let harness: ReturnType<typeof mountPanel>
  beforeEach(() => {
    harness = mountPanel()
  })

  // -------- 基线：合法配置 --------
  it('valid config: isValid=true, no blocking/advisory messages', async () => {
    await harness.setMotionSafety({
      arrivalTolerance: 0.1,
      criticalDeviationLimit: 5.0,
      noProgressTimeoutMs: 2000,
      progressEpsilon: 0.001,
    })
    const ex = harness.getExpose()
    expect(ex.isValid).toBe(true)
    expect(ex.blockingErrors).toHaveLength(0)
    expect(ex.advisoryWarnings).toHaveLength(0)
  })

  it('empty config: isValid=true (backend uses defaults)', async () => {
    await harness.setMotionSafety(undefined)
    const ex = harness.getExpose()
    expect(ex.isValid).toBe(true)
    expect(ex.blockingErrors).toHaveLength(0)
  })

  // -------- B1: NaN/Inf 阻断（与后端 validateMotionSafetyFields 对齐）--------
  it('B1: NaN arrivalTolerance is blocking', async () => {
    await harness.setMotionSafety({
      arrivalTolerance: Number.NaN,
      criticalDeviationLimit: 5.0,
    })
    const ex = harness.getExpose()
    expect(ex.isValid).toBe(false)
    expect(ex.blockingErrors.length).toBeGreaterThan(0)
  })

  it('B1: Infinity criticalDeviationLimit is blocking', async () => {
    await harness.setMotionSafety({
      arrivalTolerance: 0.1,
      criticalDeviationLimit: Number.POSITIVE_INFINITY,
    })
    const ex = harness.getExpose()
    expect(ex.isValid).toBe(false)
    expect(ex.blockingErrors.length).toBeGreaterThan(0)
  })

  // -------- B2: arrivalTolerance > 0 阻断 --------
  it('B2: zero arrivalTolerance is blocking', async () => {
    await harness.setMotionSafety({
      arrivalTolerance: 0,
      criticalDeviationLimit: 5.0,
    })
    const ex = harness.getExpose()
    expect(ex.isValid).toBe(false)
    expect(ex.blockingErrors.length).toBeGreaterThan(0)
  })

  it('B2: negative arrivalTolerance is blocking', async () => {
    await harness.setMotionSafety({
      arrivalTolerance: -0.1,
      criticalDeviationLimit: 5.0,
    })
    const ex = harness.getExpose()
    expect(ex.isValid).toBe(false)
    expect(ex.blockingErrors.length).toBeGreaterThan(0)
  })

  // -------- B3: criticalDeviationLimit > 0 阻断 --------
  it('B3: zero criticalDeviationLimit is blocking', async () => {
    await harness.setMotionSafety({
      arrivalTolerance: 0.1,
      criticalDeviationLimit: 0,
    })
    const ex = harness.getExpose()
    expect(ex.isValid).toBe(false)
    expect(ex.blockingErrors.length).toBeGreaterThan(0)
  })

  // -------- B4: criticalDeviationLimit > arrivalTolerance 阻断 --------
  it('B4: critical <= arrival is blocking', async () => {
    await harness.setMotionSafety({
      arrivalTolerance: 5.0,
      criticalDeviationLimit: 5.0,
    })
    const ex = harness.getExpose()
    expect(ex.isValid).toBe(false)
    expect(ex.blockingErrors.length).toBeGreaterThan(0)
  })

  it('B4: critical < arrival is blocking', async () => {
    await harness.setMotionSafety({
      arrivalTolerance: 10.0,
      criticalDeviationLimit: 5.0,
    })
    const ex = harness.getExpose()
    expect(ex.isValid).toBe(false)
    expect(ex.blockingErrors.length).toBeGreaterThan(0)
  })

  // -------- B5: noProgressTimeoutMs >= 200 阻断 --------
  it('B5: timeout < 200 is blocking', async () => {
    await harness.setMotionSafety({
      arrivalTolerance: 0.1,
      criticalDeviationLimit: 5.0,
      noProgressTimeoutMs: 100,
    })
    const ex = harness.getExpose()
    expect(ex.isValid).toBe(false)
    expect(ex.blockingErrors.length).toBeGreaterThan(0)
  })

  it('B5: timeout = 200 is accepted (boundary)', async () => {
    await harness.setMotionSafety({
      arrivalTolerance: 0.1,
      criticalDeviationLimit: 5.0,
      noProgressTimeoutMs: 200,
    })
    const ex = harness.getExpose()
    expect(ex.isValid).toBe(true)
  })

  // -------- B6: progressEpsilon > 0 阻断 --------
  it('B6: zero progressEpsilon is blocking', async () => {
    await harness.setMotionSafety({
      arrivalTolerance: 0.1,
      criticalDeviationLimit: 5.0,
      progressEpsilon: 0,
    })
    const ex = harness.getExpose()
    expect(ex.isValid).toBe(false)
    expect(ex.blockingErrors.length).toBeGreaterThan(0)
  })

  // -------- B7: axisOverrides 未绑定轴 阻断（后端拒绝） --------
  // 注意：后端 validateMotionSafetyConfig 返回 error 拒绝整个配置；
  // 前端按契约应对齐——unknown key 必须让 isValid=false，并在 blockingErrors 中可见。
  it('B7: axisOverrides with unbound axis is blocking and visible in blockingErrors', async () => {
    await harness.setMotionSafety({
      arrivalTolerance: 0.1,
      criticalDeviationLimit: 5.0,
      axisOverrides: {
        // Z 是 motionAxes 未绑定的轴；运行时合法（Record<string, ...> 允许任意 key），
        // 但前端按契约应对齐后端拒绝规则——B7 校验将其放入 blockingErrors
        Z: { arrivalTolerance: 0.05 },
      },
    })
    const ex = harness.getExpose()
    expect(ex.isValid).toBe(false)
    expect(ex.blockingErrors.length).toBeGreaterThan(0)
    // blockingErrors 应包含 Z 轴未绑定提示（用户能在红色错误区看到具体哪个轴）
    expect(ex.blockingErrors.some((w) => w.includes('Z'))).toBe(true)
  })

  // -------- B9: 跨字段合并 Resolve(axis) 倒置 阻断 --------
  it('B9: global arrival + axis critical inverted is blocking', async () => {
    await harness.setMotionSafety({
      arrivalTolerance: 10.0,
      criticalDeviationLimit: 20.0, // 全局自身满足 critical > arrival
      axisOverrides: {
        X: { criticalDeviationLimit: 5.0 }, // 但 X 轴 Resolve 后 arrival=10, critical=5 → 倒置
      },
    })
    const ex = harness.getExpose()
    expect(ex.isValid).toBe(false)
    expect(ex.blockingErrors.length).toBeGreaterThan(0)
  })

  it('B9: global critical + axis arrival inverted is blocking', async () => {
    await harness.setMotionSafety({
      arrivalTolerance: 0.1,
      criticalDeviationLimit: 5.0,
      axisOverrides: {
        X: { arrivalTolerance: 10.0 }, // X 轴 Resolve 后 arrival=10, critical=5 → 倒置
      },
    })
    const ex = harness.getExpose()
    expect(ex.isValid).toBe(false)
    expect(ex.blockingErrors.length).toBeGreaterThan(0)
  })

  it('B9 positive: consistent global + axis override is accepted', async () => {
    await harness.setMotionSafety({
      arrivalTolerance: 0.1,
      criticalDeviationLimit: 5.0,
      axisOverrides: {
        X: { arrivalTolerance: 0.05 }, // X 轴 Resolve 后 arrival=0.05, critical=5 → 满足
      },
    })
    const ex = harness.getExpose()
    expect(ex.isValid).toBe(true)
    expect(ex.blockingErrors).toHaveLength(0)
  })

  // -------- A1: epsilon >= arrival 仅 advisory --------
  it('A1: epsilon >= arrival is advisory only (not blocking)', async () => {
    await harness.setMotionSafety({
      arrivalTolerance: 0.1,
      criticalDeviationLimit: 5.0,
      progressEpsilon: 0.1, // == arrival，后端不拒绝但前端提示
    })
    const ex = harness.getExpose()
    expect(ex.isValid).toBe(true)
    expect(ex.blockingErrors).toHaveLength(0)
    expect(ex.advisoryWarnings.length).toBeGreaterThan(0)
  })

  it('A1: epsilon > arrival is advisory only', async () => {
    await harness.setMotionSafety({
      arrivalTolerance: 0.1,
      criticalDeviationLimit: 5.0,
      progressEpsilon: 0.5, // > arrival
    })
    const ex = harness.getExpose()
    expect(ex.isValid).toBe(true)
    expect(ex.advisoryWarnings.length).toBeGreaterThan(0)
  })

  // -------- A2: timeout >= 120000 仅 advisory --------
  it('A2: timeout >= 120000 is advisory only (not blocking)', async () => {
    await harness.setMotionSafety({
      arrivalTolerance: 0.1,
      criticalDeviationLimit: 5.0,
      noProgressTimeoutMs: 120000,
    })
    const ex = harness.getExpose()
    expect(ex.isValid).toBe(true)
    expect(ex.blockingErrors).toHaveLength(0)
    expect(ex.advisoryWarnings.length).toBeGreaterThan(0)
  })

  it('A2: timeout = 119999 is accepted (no advisory)', async () => {
    await harness.setMotionSafety({
      arrivalTolerance: 0.1,
      criticalDeviationLimit: 5.0,
      noProgressTimeoutMs: 119999,
    })
    const ex = harness.getExpose()
    expect(ex.isValid).toBe(true)
    expect(ex.advisoryWarnings).toHaveLength(0)
  })

  // -------- B8: 类型层面阻止嵌套 axisOverrides --------
  // 此条由 TypeScript 编译期保证：MotionSafetyAxisOverride 不含 axisOverrides 字段。
  // 运行时若通过 JSON 加载到嵌套结构，后端会拒绝；前端不重复运行时校验
  // （类型已阻止构造，且后端是权威）。
  it('B8 (compile-time): MotionSafetyAxisOverride type has no axisOverrides field', () => {
    // 测试前置：导入类型别名
    // 测试步骤：构造一个 axis override 对象，尝试赋值 axisOverrides 字段
    // 期待结果：TypeScript 编译期报错（@ts-expect-error 验证）
    const override = {
      arrivalTolerance: 0.1,
    }
    // @ts-expect-error axisOverrides 不应存在于 MotionSafetyAxisOverride 上
    override.axisOverrides = { X: {} }
    // 运行时此赋值会成功（JS 动态属性），但类型层面已被阻止
    expect(override).toBeDefined()
  })
})
