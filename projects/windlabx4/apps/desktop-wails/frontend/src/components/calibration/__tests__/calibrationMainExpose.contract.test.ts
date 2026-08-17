import { describe, it, expect } from 'vitest'
import type { ComponentExposed } from 'vue-component-type-helpers'
import type { CalibrationMainExpose } from '@shared/types/calibration'
import FiveHoleMain from '../five-hole/FiveHoleMain.vue'
import ThreeHoleMain from '../three-hole/ThreeHoleMain.vue'
import TotalPressureMain from '../total-pressure/TotalPressureMain.vue'
import TotalTemperatureMain from '../total-temperature/TotalTemperatureMain.vue'

/**
 * 编译期契约：4 个校准 Main 组件必须通过 defineExpose 暴露 CalibrationMainExpose。
 *
 * 触发场景：CalibrationWindow.handleSettingsSaved 通过 currentMainRef.reloadSavedConfig()
 *   在 Settings 保存后触发对应 Main 重新加载配置。缺暴露会导致 currentConfig 保留挂载时
 *   旧值，canStartCalibration 不刷新，UI 一直提示"未配置"，必须切走再切回才生效
 *   （曾发生在 ThreeHoleMain / TotalPressureMain / TotalTemperatureMain）。
 *
 * 防回归原理：AssertExposes<T extends CalibrationMainExpose> 强制 ComponentExposed<Component>
 *   满足 CalibrationMainExpose；任何 Main 缺 defineExpose 时 vue-tsc 报错，npm run typecheck 红。
 */
type AssertExposes<T extends CalibrationMainExpose> = T
// eslint-disable-next-line @typescript-eslint/no-unused-vars
type _CheckFiveHole = AssertExposes<ComponentExposed<typeof FiveHoleMain>>
// eslint-disable-next-line @typescript-eslint/no-unused-vars
type _CheckThreeHole = AssertExposes<ComponentExposed<typeof ThreeHoleMain>>
// eslint-disable-next-line @typescript-eslint/no-unused-vars
type _CheckTotalPressure = AssertExposes<ComponentExposed<typeof TotalPressureMain>>
// eslint-disable-next-line @typescript-eslint/no-unused-vars
type _CheckTotalTemperature = AssertExposes<ComponentExposed<typeof TotalTemperatureMain>>

describe('CalibrationMainExpose contract', () => {
  // 测试前置：4 个 Main 组件类型导入 + 4 条类型断言已通过 vue-tsc 编译
  // 测试步骤：运行时占位 it（让 vitest collect 该文件，避免 no-test-found 警告）
  // 期待结果：编译期断言生效——任何 Main 缺 defineExpose({ reloadSavedConfig }) 时
  //           npm run typecheck 失败，PR 阶段就拦截回归
  it('all four calibration Main components expose reloadSavedConfig (enforced at compile time)', () => {
    expect(true).toBe(true)
  })
})
