import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'

// =====================================================================
// spec Task 11 测试：generateFiveHoleSnakePoints 不再有本地 fallback
// =====================================================================
//
// 验收标准（plan Slice B4 / spec R-4、R-6）：
//   1. HTTP/Wails 都调用同一 usecase（通过 calibrationApi 透传）
//   2. 后端错误传到 UI（不静默 fallback 到本地蛇形算法）
//   3. 前端无蛇形算法或 catch fallback
//
// 测试策略：
//   - mock @api/calibrationApi：观察调用次数与透传行为
//   - mock @api/wails-adapter：控制 isWailsAvailable() 切换模式
//   - 不验证点位算法本身（那是后端 usecase 测试的职责）

// Mock calibrationApi：默认 generateFiveHoleSnakePoints 返回后端点位
// 调用方按需通过 mockResolvedValueOnce/mockRejectedValueOnce 覆盖
vi.mock('@api/calibrationApi', () => ({
  calibrationApi: {
    generateFiveHoleSnakePoints: vi.fn(async () => [
      { id: 1, coordinates: { α: 0, β: 0 } },
      { id: 2, coordinates: { α: 5, β: 0 } },
    ]),
  },
}))

// Mock wails-adapter：默认 isWailsAvailable=false（HTTP 模式）
// 各测试用例通过 vi.mocked(isWailsAvailable).mockReturnValue 切换
vi.mock('@api/wails-adapter', () => ({
  isWailsAvailable: vi.fn(() => false),
  wailsApi: {},
}))

import { generateFiveHoleSnakePoints } from '../motionCalibrationUtils'
import { calibrationApi } from '@api/calibrationApi'
import { isWailsAvailable } from '@api/wails-adapter'
import type { FiveHolePointLayout } from '@shared/types/calibration'

const layout: FiveHolePointLayout = {
  alphaMin: 0, alphaMax: 10, alphaStep: 5,
  betaMin: 0, betaMax: 10, betaStep: 5,
  serpentine: true,
}

describe('Task 11: generateFiveHoleSnakePoints — backend-driven, no local fallback', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // 默认 HTTP 模式
    vi.mocked(isWailsAvailable).mockReturnValue(false)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  // 测试前置：mock calibrationApi 返回后端点位
  // 测试步骤：HTTP 模式下调 generateFiveHoleSnakePoints(layout)
  // 期待结果：返回后端点位；calibrationApi.generateFiveHoleSnakePoints 被调用一次
  it('HTTP mode: returns backend points via calibrationApi', async () => {
    const backendPoints = [
      { id: 1, coordinates: { α: 0, β: 0 } },
      { id: 2, coordinates: { α: 5, β: 0 } },
      { id: 3, coordinates: { α: 10, β: 0 } },
    ]
    vi.mocked(calibrationApi.generateFiveHoleSnakePoints).mockResolvedValueOnce(backendPoints)

    const result = await generateFiveHoleSnakePoints(layout)

    expect(calibrationApi.generateFiveHoleSnakePoints).toHaveBeenCalledTimes(1)
    expect(calibrationApi.generateFiveHoleSnakePoints).toHaveBeenCalledWith(layout)
    expect(result).toEqual(backendPoints)
  })

  // 测试前置：mock calibrationApi 抛错（模拟后端 400 步长非法）
  // 测试步骤：HTTP 模式下调 generateFiveHoleSnakePoints(layout)
  // 期待结果：错误透传，不 fallback 到本地蛇形算法
  //           spec Task 11 acceptance：后端错误传到 UI，不静默生成本地点位
  it('HTTP mode: propagates backend error, does not fallback to local snake algorithm', async () => {
    const backendErr = new Error('生成五孔点位失败: step must be positive')
    vi.mocked(calibrationApi.generateFiveHoleSnakePoints).mockRejectedValueOnce(backendErr)

    await expect(generateFiveHoleSnakePoints(layout)).rejects.toThrow(backendErr)

    // 仅尝试一次——没有 fallback 第二次调用
    expect(calibrationApi.generateFiveHoleSnakePoints).toHaveBeenCalledTimes(1)
  })

  // 测试前置：mock calibrationApi 返回空数组（边界场景：后端返回空点位）
  // 测试步骤：HTTP 模式下调 generateFiveHoleSnakePoints(layout)
  // 期待结果：返回空数组——不 fallback 到本地算法生成点位
  //           旧实现 `if (result && result.length > 0) return result` 会 fallback 到本地，
  //           Task 11 后空数组是合法后端响应，直接透传
  it('HTTP mode: returns empty array as-is, does not fallback when backend returns 0 points', async () => {
    vi.mocked(calibrationApi.generateFiveHoleSnakePoints).mockResolvedValueOnce([])

    const result = await generateFiveHoleSnakePoints(layout)

    expect(result).toEqual([])
    expect(calibrationApi.generateFiveHoleSnakePoints).toHaveBeenCalledTimes(1)
  })

  // 测试前置：mock isWailsAvailable=true（Wails 模式），calibrationApi 内部会调 wailsApi.calibration.previewFiveHole
  //           这里直接 mock calibrationApi 返回 binding 数据（Wails 模式下 calibrationApi 内部已处理 binding 调用）
  // 测试步骤：Wails 模式下调 generateFiveHoleSnakePoints(layout)
  // 期待结果：返回后端点位；calibrationApi.generateFiveHoleSnakePoints 被调用（统一入口）
  it('Wails mode: calls calibrationApi (which internally routes to binding)', async () => {
    vi.mocked(isWailsAvailable).mockReturnValue(true)
    const bindingPoints = [
      { id: 1, coordinates: { α: 0, β: 0 } },
      { id: 2, coordinates: { α: 5, β: 5 } },
    ]
    vi.mocked(calibrationApi.generateFiveHoleSnakePoints).mockResolvedValueOnce(bindingPoints)

    const result = await generateFiveHoleSnakePoints(layout)

    expect(calibrationApi.generateFiveHoleSnakePoints).toHaveBeenCalledTimes(1)
    expect(result).toEqual(bindingPoints)
  })

  // 测试前置：Wails 模式，mock calibrationApi 抛错（binding Success=false）
  // 测试步骤：Wails 模式下调 generateFiveHoleSnakePoints(layout)
  // 期待结果：错误透传，不 fallback 到本地
  it('Wails mode: propagates binding error, does not fallback', async () => {
    vi.mocked(isWailsAvailable).mockReturnValue(true)
    const bindingErr = new Error('校准管理器未初始化')
    vi.mocked(calibrationApi.generateFiveHoleSnakePoints).mockRejectedValueOnce(bindingErr)

    await expect(generateFiveHoleSnakePoints(layout)).rejects.toThrow(bindingErr)
    expect(calibrationApi.generateFiveHoleSnakePoints).toHaveBeenCalledTimes(1)
  })

  // 测试前置：源码静态扫描——motionCalibrationUtils.ts 不应包含本地蛇形算法
  // 测试步骤：读取源文件内容，断言无关键本地算法标识
  // 期待结果：无 "generateFiveHoleSnakePointsLocal"、
  //           "alphaValues"/"betaValues" 循环等本地算法痕迹；
  //           generateFiveHoleSnakePoints 函数体内无 try/catch 包裹
  //           spec Task 11 acceptance：前端无蛇形算法或 catch fallback
  it('source file has no local snake algorithm or catch fallback', async () => {
    const fs = await import('fs')
    const path = await import('path')
    const src = fs.readFileSync(
      path.resolve(__dirname, '../motionCalibrationUtils.ts'),
      'utf-8'
    )

    // 删除本地算法函数
    expect(src).not.toContain('generateFiveHoleSnakePointsLocal')
    // 删除本地蛇形循环（alphaValues/betaValues push）
    expect(src).not.toMatch(/alphaValues\.push/)
    expect(src).not.toMatch(/betaValues\.push/)
    // generateFiveHoleSnakePoints 函数体内不应有 try/catch 包裹
    // 提取函数体后断言无 catch 关键字（旧实现 `try { api } catch { fallback }`）
    const fnMatch = src.match(/export async function generateFiveHoleSnakePoints[\s\S]*?\n\}/)
    expect(fnMatch, 'generateFiveHoleSnakePoints function should exist').not.toBeNull()
    if (fnMatch) {
      const fnBody = fnMatch[0]
      expect(fnBody).not.toMatch(/\bcatch\b/)
      expect(fnBody).not.toMatch(/\bfallback\b/i)
    }
  })
})
