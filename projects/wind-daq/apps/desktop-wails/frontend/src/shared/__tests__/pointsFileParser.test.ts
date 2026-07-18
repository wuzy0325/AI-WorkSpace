import { describe, it, expect } from 'vitest'
import { normalizeAxisName, parsePointsFile } from '@shared/pointsFileParser'

// 测试前置：所有用例基于 plan-traversal-custom-4axis.md Task 8 三段式格式
// - 测试前置：准备文件文本
// - 测试步骤：调用 parsePointsFile / normalizeAxisName
// - 期待结果：返回值断言

describe('normalizeAxisName', () => {
  it('识别 X/X(mm)/pos_x/PosX', () => {
    // 测试前置：列名变体列表
    // 测试步骤：逐项调用 normalizeAxisName
    // 期待结果：均返回 'x'
    expect(normalizeAxisName('X')).toBe('x')
    expect(normalizeAxisName('x')).toBe('x')
    expect(normalizeAxisName('X(mm)')).toBe('x')
    expect(normalizeAxisName('pos_x')).toBe('x')
    expect(normalizeAxisName('PosX')).toBe('x')
  })

  it('识别 U/pos_u/U(°)，但不识别 α/alpha（四轴一视同仁）', () => {
    // 测试前置：U 轴列名变体
    // 期待结果：U/pos_u/U(°) 返回 'u'；α/alpha 不再映射到 U 轴，返回 null
    expect(normalizeAxisName('U')).toBe('u')
    expect(normalizeAxisName('U(°)')).toBe('u')
    expect(normalizeAxisName('pos_u')).toBe('u')
    // 四轴一视同仁：U 轴不特殊识别 α/alpha 别名
    expect(normalizeAxisName('α')).toBeNull()
    expect(normalizeAxisName('alpha')).toBeNull()
  })

  it('未识别返回 null', () => {
    expect(normalizeAxisName('foo')).toBeNull()
    expect(normalizeAxisName('1')).toBeNull()
    expect(normalizeAxisName('')).toBeNull()
    expect(normalizeAxisName('α')).toBeNull()
    expect(normalizeAxisName('alpha')).toBeNull()
  })
})

describe('parsePointsFile', () => {
  it('用例 1：CSV 全 4 列 + 表头大小写混合', () => {
    // 测试前置：4 列 CSV，表头大小写混合
    // 测试步骤：调用 parsePointsFile
    // 期待结果：2 点，4 轴数据完整
    const text = 'X,Y,Z,U\n1,2,3,4\n5,6,7,8'
    const result = parsePointsFile(text)
    expect(result).toEqual([
      { x: 1, y: 2, z: 3, u: 4 },
      { x: 5, y: 6, z: 7, u: 8 }
    ])
  })

  it('用例 2：CSV 表头含括号注释', () => {
    // 测试前置：表头为 X(mm),Y(mm),Z(mm),U(°)
    // 期待结果：4 列正确解析
    const text = 'X(mm),Y(mm),Z(mm),U(°)\n1,2,3,4'
    const result = parsePointsFile(text)
    expect(result).toEqual([{ x: 1, y: 2, z: 3, u: 4 }])
  })

  it('用例 3：TSV 缺 U 列（验证 0 填充）', () => {
    // 测试前置：3 列 TSV（缺 U 列），用 Tab 分隔
    // 期待结果：返回 1 点，u=0
    const text = 'X\tY\tZ\n1\t2\t3'
    const result = parsePointsFile(text)
    expect(result).toEqual([{ x: 1, y: 2, z: 3, u: 0 }])
  })

  it('用例 4：无表头纯数据（默认 X,Y,Z,U 顺序）', () => {
    // 测试前置：纯数字 CSV，无表头
    // 期待结果：按 X,Y,Z,U 顺序解析
    const text = '1,2,3,4\n5,6,7,8'
    const result = parsePointsFile(text)
    expect(result).toEqual([
      { x: 1, y: 2, z: 3, u: 4 },
      { x: 5, y: 6, z: 7, u: 8 }
    ])
  })

  it('用例 5：同轴多列后列覆盖前列（pos_u,U 都映射到 U 轴）', () => {
    // 测试前置：表头 pos_u,U — 两列都映射到 U 轴
    // 期待结果：同轴多列时后列覆盖前列，最终 u=2，其余轴 0
    const text = 'pos_u,U\n1,2'
    const result = parsePointsFile(text)
    expect(result).toEqual([{ x: 0, y: 0, z: 0, u: 2 }])
  })

  it('用例 6：空文件返回 []', () => {
    expect(parsePointsFile('')).toEqual([])
    expect(parsePointsFile('   \n  \n')).toEqual([])
  })

  it('用例 7：跳过空行和仅含分隔符的行', () => {
    const text = 'X,Y\n1,2\n\n3,4\n'
    const result = parsePointsFile(text)
    expect(result).toEqual([
      { x: 1, y: 2, z: 0, u: 0 },
      { x: 3, y: 4, z: 0, u: 0 }
    ])
  })

  it('用例 8：负数和小数', () => {
    const text = 'X,Y,Z,U\n-1.5,2.3,-3.14,4.0'
    const result = parsePointsFile(text)
    expect(result).toEqual([{ x: -1.5, y: 2.3, z: -3.14, u: 4.0 }])
  })
})
