import { describe, it, expect } from 'vitest'
import {
  normalizeAxisName,
  normalizeConfigKey,
  parsePointsFile,
  parsePointsFileWithWarnings,
  PER_POINT_DWELL_MS_MAX,
  PER_POINT_DWELL_MS_MIN,
  PER_POINT_SAMPLES_MAX,
  PER_POINT_SAMPLES_MIN,
} from '@shared/pointsFileParser'

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

describe('normalizeConfigKey', () => {
  // 测试前置：per-point 配置列名变体
  // 测试步骤：逐项调用 normalizeConfigKey
  // 期待结果：均返回对应 PointConfigKey（'dwellMs' / 'samples' / 'test'）

  it('识别 dwellMs 别名（dwell / dwellms / dwell_time_ms / stabilization 等）', () => {
    expect(normalizeConfigKey('dwellMs')).toBe('dwellMs')
    expect(normalizeConfigKey('DwellMs')).toBe('dwellMs')
    expect(normalizeConfigKey('dwell')).toBe('dwellMs')
    expect(normalizeConfigKey('dwellms')).toBe('dwellMs')
    expect(normalizeConfigKey('dwell_time_ms')).toBe('dwellMs')
    expect(normalizeConfigKey('dwelltimems')).toBe('dwellMs')
    expect(normalizeConfigKey('stabilization')).toBe('dwellMs')
    expect(normalizeConfigKey('stabilizationms')).toBe('dwellMs')
    // 表头带单位注释也应识别（如 dwellMs(ms)）
    expect(normalizeConfigKey('Dwell(ms)')).toBe('dwellMs')
  })

  it('识别 samples 别名（samples / samplesperpoint / samples_per_point）', () => {
    expect(normalizeConfigKey('samples')).toBe('samples')
    expect(normalizeConfigKey('Samples')).toBe('samples')
    expect(normalizeConfigKey('samplesperpoint')).toBe('samples')
    expect(normalizeConfigKey('samples_per_point')).toBe('samples')
  })

  it('识别 test 别名（test / enable / skip）', () => {
    expect(normalizeConfigKey('test')).toBe('test')
    expect(normalizeConfigKey('Test')).toBe('test')
    expect(normalizeConfigKey('enable')).toBe('test')
    expect(normalizeConfigKey('Enable')).toBe('test')
    expect(normalizeConfigKey('skip')).toBe('test')
  })

  it('未识别返回 null', () => {
    expect(normalizeConfigKey('foo')).toBeNull()
    expect(normalizeConfigKey('1')).toBeNull()
    expect(normalizeConfigKey('')).toBeNull()
    expect(normalizeConfigKey('dwellTime')).toBeNull()
    expect(normalizeConfigKey('sample')).toBeNull()
    expect(normalizeConfigKey('enabled')).toBeNull()
    expect(normalizeConfigKey('testing')).toBeNull()
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

  // ============ per-point 配置列测试（spec §3） ============

  it('用例 9：旧格式 CSV（仅 X,Y,Z,U）→ 三个新字段全为 undefined', () => {
    // 测试前置：旧版本 CSV，无 per-point 配置列
    // 测试步骤：调用 parsePointsFile
    // 期待结果：2 点，dwellMs/samples/test 字段均不存在（toEqual 视 undefined 与缺失等价）
    const text = 'X,Y,Z,U\n1,2,3,4\n5,6,7,8'
    const result = parsePointsFile(text)
    expect(result).toEqual([
      { x: 1, y: 2, z: 3, u: 4 },
      { x: 5, y: 6, z: 7, u: 8 }
    ])
    // 显式断言三字段 undefined，确保向后兼容（旧 CSV 升级后默认走全局配置）
    expect(result[0].dwellMs).toBeUndefined()
    expect(result[0].samples).toBeUndefined()
    expect(result[0].test).toBeUndefined()
  })

  it('用例 10：新格式 CSV 完整列（dwellMs/samples/test 均有值）', () => {
    // 测试前置：CSV 表头 X,Y,Z,U,dwellMs,samples,test
    // 期待结果：3 个新字段正确解析为对应类型
    const text = 'X,Y,Z,U,dwellMs,samples,test\n1,2,3,4,5000,50,1\n5,6,7,8,10000,100,0'
    const result = parsePointsFile(text)
    expect(result).toEqual([
      { x: 1, y: 2, z: 3, u: 4, dwellMs: 5000, samples: 50, test: true },
      { x: 5, y: 6, z: 7, u: 8, dwellMs: 10000, samples: 100, test: false }
    ])
  })

  it('用例 11：新格式 CSV 部分列（仅 dwellMs）→ samples / test 为 undefined', () => {
    // 测试前置：表头只新增 dwellMs 列，samples/test 列缺失
    // 期待结果：dwellMs 解析为数值；samples/test 字段不存在（走全局默认）
    const text = 'X,Y,Z,U,dwellMs\n1,2,3,4,3000'
    const result = parsePointsFile(text)
    expect(result).toEqual([{ x: 1, y: 2, z: 3, u: 4, dwellMs: 3000 }])
    expect(result[0].samples).toBeUndefined()
    expect(result[0].test).toBeUndefined()
  })

  it('用例 12：单行某列留空（dwellMs,test 留空，samples 有值）', () => {
    // 测试前置：表头完整 7 列，数据行 dwellMs 留空、test 留空、samples=20
    // 期待结果：dwellMs/test 字段不存在；samples=20；test 视为 true（用全局默认）
    const text = 'X,Y,Z,U,dwellMs,samples,test\n12.5,-30,0,0,,20,1'
    const result = parsePointsFile(text)
    expect(result).toEqual([
      { x: 12.5, y: -30, z: 0, u: 0, samples: 20, test: true }
    ])
    expect(result[0].dwellMs).toBeUndefined()
  })

  it('用例 13：test 列取值约定（1/true/y/yes → true；0/false/n/no → false；空 → undefined）', () => {
    // 测试前置：6 行 CSV，test 列依次为 1/true/0/false/yes/no
    // 期待结果：前 2 行 true；中间 2 行 false；最后 2 行 yes→true, no→false
    const text = 'X,Y,Z,U,test\n1,0,0,0,1\n2,0,0,0,true\n3,0,0,0,0\n4,0,0,0,false\n5,0,0,0,yes\n6,0,0,0,no'
    const result = parsePointsFile(text)
    expect(result.map((p) => p.test)).toEqual([true, true, false, false, true, false])
  })

  it('用例 14：test 列留空 → undefined（视为用全局默认 true）', () => {
    // 测试前置：表头含 test 列，数据行 test 留空
    // 期待结果：test 字段不存在（undefined），由后端按"用全局默认 true"处理
    const text = 'X,Y,Z,U,test\n1,0,0,0,'
    const result = parsePointsFile(text)
    expect(result).toEqual([{ x: 1, y: 0, z: 0, u: 0 }])
    expect(result[0].test).toBeUndefined()
  })

  it('用例 15：TXT 格式（空白分隔）→ 仅解析 X/Y/Z/U，忽略 per-point 配置列', () => {
    // 测试前置：TXT 表头含 dwellMs/samples/test 列，但分隔符是 Tab/空格
    // 期待结果：仅解析 4 轴坐标，per-point 字段全部 undefined（spec §3 方案 A）
    // 验证：即便用户在 TXT 表头写了 dwellMs，解析器也不消费该列
    const text = 'X\tY\tZ\tU\tdwellMs\tsamples\ttest\n1\t2\t3\t4\t5000\t50\t1'
    const result = parsePointsFile(text)
    expect(result).toEqual([{ x: 1, y: 2, z: 3, u: 4 }])
    expect(result[0].dwellMs).toBeUndefined()
    expect(result[0].samples).toBeUndefined()
    expect(result[0].test).toBeUndefined()
  })

  it('用例 16：skip 列别名取反（skip=1 → test=false；skip=0 → test=true）', () => {
    // 测试前置：表头用 skip 列名代替 test，语义取反
    // 期待结果：skip=1 → test=false（跳过此点）；skip=0 → test=true（执行测试）
    const text = 'X,Y,Z,U,skip\n1,0,0,0,1\n2,0,0,0,0'
    const result = parsePointsFile(text)
    expect(result[0].test).toBe(false)
    expect(result[1].test).toBe(true)
  })

  it('用例 17：skip 列留空 → test=undefined（视为用全局默认 true）', () => {
    // 测试前置：表头用 skip 列，数据行 skip 留空
    // 期待结果：test 字段不存在，由后端按"用全局默认 true"处理
    const text = 'X,Y,Z,U,skip\n1,0,0,0,'
    const result = parsePointsFile(text)
    expect(result[0].test).toBeUndefined()
  })

  it('用例 18：dwellMs/samples 小数自动截断为整数', () => {
    // 测试前置：CSV 中 dwellMs=5000.7、samples=50.9（小数）
    // 期待结果：dwellMs=5000、samples=50（Math.trunc 截断，与 ms/采样次数整数语义一致）
    const text = 'X,Y,Z,U,dwellMs,samples\n1,0,0,0,5000.7,50.9'
    const result = parsePointsFile(text)
    expect(result[0].dwellMs).toBe(5000)
    expect(result[0].samples).toBe(50)
  })

  it('用例 19：表头列顺序打乱（dwellMs 在 X 之前）也能正确解析', () => {
    // 测试前置：表头顺序为 dwellMs,X,Y,Z,U,samples,test（per-point 字段在前）
    // 期待结果：按列名匹配而非位置匹配，全部字段正确解析
    const text = 'dwellMs,X,Y,Z,U,samples,test\n5000,1,2,3,4,50,1'
    const result = parsePointsFile(text)
    expect(result).toEqual([
      { x: 1, y: 2, z: 3, u: 4, dwellMs: 5000, samples: 50, test: true }
    ])
  })

  it('用例 20：dwellMs/samples 非数字留空 → undefined（不抛错）', () => {
    // 测试前置：dwellMs 列填 "abc"、samples 列填 "xyz"
    // 期待结果：字段不存在（视为用全局默认），不抛错，坐标仍正确解析
    const text = 'X,Y,Z,U,dwellMs,samples\n1,2,3,4,abc,xyz'
    const result = parsePointsFile(text)
    expect(result).toEqual([{ x: 1, y: 2, z: 3, u: 4 }])
    expect(result[0].dwellMs).toBeUndefined()
    expect(result[0].samples).toBeUndefined()
  })

  it('用例 21：未知列名宽容忽略（不抛错）', () => {
    // 测试前置：表头含 unknown/comment 等未识别列
    // 期待结果：未知列被忽略，已知列正确解析（保持向后兼容策略）
    const text = 'X,Y,Z,U,unknown,dwellMs,comment\n1,2,3,4,foo,5000,bar'
    const result = parsePointsFile(text)
    expect(result).toEqual([
      { x: 1, y: 2, z: 3, u: 4, dwellMs: 5000 }
    ])
  })

  it('用例 22：UTF-8 BOM 头剥离（Windows 记事本/Excel 另存为 CSV 默认带 BOM）', () => {
    // 测试前置：CSV 文本以 \uFEFF 开头（UTF-8 BOM），表头为标准 X,Y,Z,U
    // 期待结果：BOM 被剥离，首列 X 正常识别，4 轴数据完整解析
    // 回归保护：此前 BOM 让 normalizeAxisName 把首列识别为 "\uFEFFX" 不匹配 → X 列静默丢失
    const text = '\uFEFFX,Y,Z,U\n1,2,3,4\n5,6,7,8'
    const result = parsePointsFile(text)
    expect(result).toEqual([
      { x: 1, y: 2, z: 3, u: 4 },
      { x: 5, y: 6, z: 7, u: 8 }
    ])
  })

  it('用例 23：BOM + per-point 配置列组合（BOM 不影响 dwellMs 列识别）', () => {
    // 测试前置：CSV 含 BOM + 完整 7 列表头（X,Y,Z,U,dwellMs,samples,test）
    // 期待结果：BOM 剥离后所有列正常识别，per-point 字段正确解析
    const text = '\uFEFFX,Y,Z,U,dwellMs,samples,test\n1,2,3,4,5000,50,1'
    const result = parsePointsFile(text)
    expect(result).toEqual([
      { x: 1, y: 2, z: 3, u: 4, dwellMs: 5000, samples: 50, test: true }
    ])
  })
})

describe('parsePointsFileWithWarnings', () => {
  // 测试前置：所有用例基于 per-point 字段范围校验 spec
  // 范围：dwellMs ∈ [100, 60000]、samples ∈ [1, 1000]
  // 越界 → clamp 到边界 + warnings 收集，避免外部 CSV 绕过 UI 约束

  it('用例 24：dwellMs 超上限 → clamp 到 60000 并写入 warning', () => {
    // 测试前置：dwellMs=99999（超过 60000）
    // 期待结果：dwellMs 修正为 60000，warnings 含 1 条提示
    const text = 'X,Y,Z,U,dwellMs\n1,2,3,4,99999'
    const { points, warnings } = parsePointsFileWithWarnings(text)
    expect(points[0].dwellMs).toBe(PER_POINT_DWELL_MS_MAX)
    expect(warnings).toHaveLength(1)
    expect(warnings[0]).toContain('dwellMs')
    expect(warnings[0]).toContain(String(PER_POINT_DWELL_MS_MAX))
  })

  it('用例 25：samples 负数 → clamp 到 1 并写入 warning', () => {
    // 测试前置：samples=-5（小于最小值 1）
    // 期待结果：samples 修正为 1，warnings 含 1 条提示
    const text = 'X,Y,Z,U,samples\n1,2,3,4,-5'
    const { points, warnings } = parsePointsFileWithWarnings(text)
    expect(points[0].samples).toBe(PER_POINT_SAMPLES_MIN)
    expect(warnings).toHaveLength(1)
    expect(warnings[0]).toContain('samples')
  })

  it('用例 26：所有字段正常 → warnings 为空数组', () => {
    // 测试前置：dwellMs=5000（在范围内）、samples=50（在范围内）
    // 期待结果：字段值保持不变，warnings 为空
    const text = 'X,Y,Z,U,dwellMs,samples\n1,2,3,4,5000,50'
    const { points, warnings } = parsePointsFileWithWarnings(text)
    expect(points[0].dwellMs).toBe(5000)
    expect(points[0].samples).toBe(50)
    expect(warnings).toEqual([])
  })

  it('用例 27：未设置 per-point 字段 → 不触发 clamp，warnings 为空', () => {
    // 测试前置：仅 X/Y/Z/U 4 列，无 per-point 配置列
    // 期待结果：dwellMs/samples 字段不存在，warnings 为空（undefined 不参与范围校验）
    const text = 'X,Y,Z,U\n1,2,3,4'
    const { points, warnings } = parsePointsFileWithWarnings(text)
    expect(points[0].dwellMs).toBeUndefined()
    expect(points[0].samples).toBeUndefined()
    expect(warnings).toEqual([])
  })

  it('用例 28：多行多字段超界 → 每行每字段独立 clamp 与 warning', () => {
    // 测试前置：2 行点位，第 1 行 dwellMs 超上限 + samples 超下限；第 2 行 dwellMs 超下限
    // 期待结果：3 条 warnings，对应 3 个被 clamp 的字段
    const text = 'X,Y,Z,U,dwellMs,samples\n1,2,3,4,99999,-5\n5,6,7,8,-100,50'
    const { points, warnings } = parsePointsFileWithWarnings(text)
    expect(points[0].dwellMs).toBe(PER_POINT_DWELL_MS_MAX)
    expect(points[0].samples).toBe(PER_POINT_SAMPLES_MIN)
    expect(points[1].dwellMs).toBe(PER_POINT_DWELL_MS_MIN)
    expect(points[1].samples).toBe(50)
    expect(warnings).toHaveLength(3)
  })
})
