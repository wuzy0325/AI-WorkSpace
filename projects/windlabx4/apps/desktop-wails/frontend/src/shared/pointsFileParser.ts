/**
 * TXT/CSV 点位文件解析器
 *
 * 用于遍历测试·自定义点模式批量导入点位列表。前端纯解析，不依赖后端文件 IO。
 *
 * 支持格式：
 * - CSV（逗号分隔）：首行可含表头（X/Y/Z/U 或 pos_x 等别名）+ 可选 per-point 配置列
 *   （dwellMs/samples/test）
 * - TSV（Tab / 空格分隔）：同上，但**仅识别 X/Y/Z/U**，per-point 配置列只在 CSV 中支持
 * - 无表头纯数据：列顺序固定为 X,Y,Z,U
 *
 * 缺列按 0 填充（坐标轴）或 undefined（per-point 配置），不抛错。
 *
 * per-point 配置列取值约定（spec §3）：
 * - dwellMs: 整数 ms（如 5000），留空 → undefined（用全局 dwellTimeMs）
 * - samples: 整数（如 50），留空 → undefined（用全局 samplesPerPoint）
 * - test: 1/true → true；0/false → false；留空 → undefined（视为 true）
 *   test 列别名 "skip"（值取反：skip=1 → test=false），便于"跳过此点"语义自然书写
 */

/** 标准轴名 */
export type AxisKey = 'x' | 'y' | 'z' | 'u'

/** per-point 配置键名 */
export type PointConfigKey = 'dwellMs' | 'samples' | 'test'

/**
 * 解析出的点位结构
 *
 * 坐标 x/y/z/u 为必填（缺列填 0，与后端 float64(0) 语义一致）。
 * per-point 配置字段为可选：undefined 表示"用全局默认值"，与后端 *int/*bool 的 nil 语义对齐。
 * 不用 0/false 作为"未设置"信号：samples 合法最小值是 1，无法用 0 区分"未设置"与"显式 1"。
 */
export interface ParsedPoint {
  x: number
  y: number
  z: number
  u: number
  /** per-point 稳定时间（ms），undefined 表示用全局 dwellTimeMs */
  dwellMs?: number
  /** per-point 采样点数，undefined 表示用全局 samplesPerPoint */
  samples?: number
  /** per-point 是否测试，undefined 表示用全局默认 true；false 时跳过采集保存 */
  test?: boolean
}

/**
 * 剥离中英文括号注释（非贪婪 + 全局，支持多段与半角/全角混排）
 *
 * 如 "x(mm)" → "x"、"x(mm)(deg)" → "x"、"x(mm)（deg）" → "x"
 */
function stripBracketComments(text: string): string {
  return text.replace(/\(.*?\)|（.*?）/g, '').trim()
}

/**
 * 列名 → 标准轴名归一化映射
 *
 * 算法：trim → lowercase → 移除括号注释（含全角、多段混排）→ 逐轴正则匹配
 *
 * 匹配的别名（大小写不敏感）：
 * - X: "x", "posx", "pos_x"
 * - Y: "y", "posy", "pos_y"
 * - Z: "z", "posz", "pos_z"
 * - U: "u", "posu", "pos_u"
 *
 * U 轴与其他三轴对称，不接受 α/alpha 别名。
 * 不匹配返回 null
 */
export function normalizeAxisName(raw: string): AxisKey | null {
  const cleaned = raw.trim().toLowerCase()
  // 移除括号注释（如 "X(mm)" → "x", "X(mm)（deg）" → "x"），含全角与多段注释
  const base = stripBracketComments(cleaned)
  if (/^(x|pos[_]?x)$/.test(base)) return 'x'
  if (/^(y|pos[_]?y)$/.test(base)) return 'y'
  if (/^(z|pos[_]?z)$/.test(base)) return 'z'
  // U 轴与其他三轴一视同仁，仅识别 u/posu/pos_u，不接受 α/alpha 别名
  if (/^(u|pos[_]?u)$/.test(base)) return 'u'
  return null
}

/**
 * 列名 → per-point 配置键归一化映射
 *
 * 算法：trim → lowercase → 移除括号注释（含全角、多段混排）→ 正则匹配
 *
 * 匹配的别名（大小写不敏感）：
 * - dwellMs: "dwell", "dwellms", "dwell_time_ms", "dwelltimems", "stabilization", "stabilizationms"
 * - samples: "samples", "samplesperpoint", "samples_per_point"
 * - test: "test", "enable", "skip"（skip 值取反：skip=1 → test=false）
 *
 * 与 normalizeAxisName 一致，支持剥离括号注释（如 "Dwell(ms)" → "dwell"），
 * 便于用户在表头里同时标注单位。仅 CSV 表头识别 per-point 配置列；
 * TXT（空白分隔）不识别，保持纯坐标语义。不匹配返回 null。
 */
export function normalizeConfigKey(raw: string): PointConfigKey | null {
  const cleaned = raw.trim().toLowerCase()
  // 移除括号注释（如 "Dwell(ms)" → "dwell"，含多段混排），与 normalizeAxisName 保持一致
  const base = stripBracketComments(cleaned)
  if (/^(dwell|dwellms|dwell_time_ms|dwelltimems|stabilization|stabilizationms)$/.test(base)) return 'dwellMs'
  if (/^(samples|samplesperpoint|samples_per_point)$/.test(base)) return 'samples'
  if (/^(test|enable|skip)$/.test(base)) return 'test'
  return null
}

/**
 * 把表头单元格值归一化为 test 字段的 boolean。
 *
 * 取值约定（spec §3）：
 * - "1" / "true" / "y" / "yes" → true
 * - "0" / "false" / "n" / "no" → false
 * - 空字符串 → undefined（用全局默认 true）
 *
 * skip 列别名取反（skip=1 → test=false）由调用方在解析列名时识别，
 * 解析单元格值时统一通过本函数处理，再由调用方根据列名决定是否取反。
 *
 * 不识别的值返回 undefined（与"留空"同语义），避免 CSV 中混入单位注释
 * （如 "1 (test)"）导致整行被丢弃。
 */
function parseTestCell(raw: string): boolean | undefined {
  const cleaned = raw.trim().toLowerCase()
  if (cleaned === '') return undefined
  if (/^(1|true|y|yes)$/.test(cleaned)) return true
  if (/^(0|false|n|no)$/.test(cleaned)) return false
  return undefined
}

/** 检测分隔符：第一行含逗号 → CSV，否则视为 TSV（Tab / 空格） */
function detectDelimiter(text: string): ',' | 'whitespace' {
  const firstLine = text.split(/\r?\n/, 1)[0] ?? ''
  return firstLine.includes(',') ? ',' : 'whitespace'
}

/** 按分隔符切分行字段 */
function splitLine(line: string, delimiter: ',' | 'whitespace'): string[] {
  if (delimiter === ',') {
    return line.split(',').map((s) => s.trim())
  }
  // 空白分隔：Tab / 多空格 / 混合
  return line.trim().split(/\s+/)
}

/**
 * 解析 TXT/CSV 点位文件文本
 *
 * @param text 文件文本内容
 * @returns 解析出的点位数组，空文件返回 []
 *
 * 说明：
 * - 格式错误的行会被跳过，不抛错
 * - TXT（空白分隔）仅识别 X/Y/Z/U 4 列坐标，忽略任何 per-point 配置列
 *   （spec §3 方案 A：TXT 不支持新字段，保持纯坐标语义）
 * - CSV（逗号分隔）首行表头可包含 dwellMs/samples/test per-point 配置列
 */
export function parsePointsFile(text: string): ParsedPoint[] {
  if (!text || !text.trim()) return []

  // 剥离 UTF-8 BOM（\uFEFF）：Windows 记事本/Excel 另存为 CSV 默认带 BOM，
  // trim() 不会剥离 BOM（不属于默认空白字符集），残留会让首列名（如 "X"）
  // 变成 "\uFEFFX"，normalizeAxisName 正则不匹配 → X 列静默丢失。
  // 起点剥离一次即可，行内 \uFEFF 不存在合法语义。
  const stripped = text.replace(/^\uFEFF/, '')

  const delimiter = detectDelimiter(stripped)
  const lines = stripped.split(/\r?\n/).filter((line) => line.trim().length > 0)
  if (lines.length === 0) return []

  // 检测首行是否为表头：任一字段含字母或为已知列名别名 → 视为表头
  const firstFields = splitLine(lines[0], delimiter)
  const hasHeader = firstFields.some((f) => /[a-zA-Zα]/.test(f))

  // 列映射：column index → AxisKey / PointConfigKey
  // axisMap 与 configMap 分开存储，避免同一列被双重消费
  let axisMap: Map<number, AxisKey> = new Map()
  let configMap: Map<number, { key: PointConfigKey; invert: boolean }> = new Map()
  let dataStartIndex = 0

  if (hasHeader) {
    // 解析表头列映射
    firstFields.forEach((field, idx) => {
      const axis = normalizeAxisName(field)
      if (axis) {
        axisMap.set(idx, axis)
        return
      }
      const cfgKey = normalizeConfigKey(field)
      if (cfgKey) {
        // skip 列别名取反：skip=1 → test=false（更符合"跳过此点"的直觉书写）
        // 其他 test 别名（test/enable）值不取反。
        // 判定必须基于剥离括号注释后的 normalized base（与 normalizeConfigKey 内部一致），
        // 否则 "skip(1=skip)(flag)" 这类带注释的 skip 列会被识别为 test 配置键，
        // 但 invert 误判为 false → skip=1 被解析为 test=true，违反 skip 语义。
        const normalizedBase = stripBracketComments(field.trim().toLowerCase())
        configMap.set(idx, { key: cfgKey, invert: /^skip$/.test(normalizedBase) })
      }
    })
    dataStartIndex = 1
    // 表头无任何已知列名 → 视为无效，回退默认 X,Y,Z,U 顺序
    if (axisMap.size === 0 && configMap.size === 0) {
      axisMap = new Map([[0, 'x'], [1, 'y'], [2, 'z'], [3, 'u']])
    }
    // TXT 格式忽略 per-point 配置列（spec §3 方案 A）：
    // CSV 表头同时含 dwellMs 等列时，TXT 分隔符不会出现（detectDelimiter 已切到 whitespace），
    // 这里清空 configMap 是防御性兜底——若 CSV 表头错把 Tab 当分隔符导致 detectDelimiter 误判，
    // 也避免 TXT 解析路径消费 per-point 字段产生不一致行为。
    if (delimiter === 'whitespace') {
      configMap = new Map()
    }
  } else {
    // 无表头：默认列顺序 X,Y,Z,U
    axisMap = new Map([[0, 'x'], [1, 'y'], [2, 'z'], [3, 'u']])
  }

  const result: ParsedPoint[] = []
  for (let i = dataStartIndex; i < lines.length; i++) {
    const fields = splitLine(lines[i], delimiter)
    // 缺列按 0 填充（坐标轴）或 undefined（per-point 配置）
    const point: ParsedPoint = { x: 0, y: 0, z: 0, u: 0 }
    let hasValidValue = false
    for (const [idx, axis] of axisMap) {
      const raw = fields[idx]
      if (raw === undefined) continue
      const value = parseFloat(raw)
      if (!Number.isNaN(value)) {
        point[axis] = value
        hasValidValue = true
      }
    }
    // per-point 配置列：仅 CSV 路径会消费（TXT 已清空 configMap）
    for (const [idx, { key, invert }] of configMap) {
      const raw = fields[idx]
      if (raw === undefined) continue
      if (key === 'test') {
        const parsed = parseTestCell(raw)
        if (parsed !== undefined) {
          // skip 列别名取反：skip=1 → test=false
          point.test = invert ? !parsed : parsed
        }
        // 留空（parsed === undefined）时不赋值，保持 undefined 表示"用全局默认 true"
      } else {
        // dwellMs / samples：parseFloat 解析数字，非数字留空保持 undefined
        const value = parseFloat(raw)
        if (!Number.isNaN(value) && raw.trim() !== '') {
          // 整数化：dwellMs/samples 不接受小数（ms 与采样次数均为整数语义）
          point[key] = Math.trunc(value)
        }
      }
    }
    // 全行无有效数字（如空行 / 仅含分隔符）跳过
    if (hasValidValue) {
      result.push(point)
    }
  }

  return result
}

/**
 * per-point 配置字段的有效范围（与 CustomPointsTable.vue 输入框 min/max 对齐）
 *
 * 用途：CSV 导入路径 clamp + warning 收集，避免外部工具导出的非法值
 * （dwellMs=-100 / samples=99999）进入运行时绕过 UI 约束。
 * 坐标 x/y/z/u 不在此约束——坐标范围由设备物理行程决定，应在 motionSafety 校验。
 */
export const PER_POINT_DWELL_MS_MIN = 100
export const PER_POINT_DWELL_MS_MAX = 60000
export const PER_POINT_SAMPLES_MIN = 1
export const PER_POINT_SAMPLES_MAX = 1000

/**
 * 解析结果 + 范围校验警告
 *
 * warnings：每条对应一个被 clamp 的字段，调用方据此向用户反馈（toast/状态栏），
 * 避免静默修正导致用户误以为原始值已生效。空数组表示无 clamp。
 */
export interface ParsePointsFileResult {
  points: ParsedPoint[]
  warnings: string[]
}

/**
 * 解析 TXT/CSV 点位文件文本，并对 per-point 配置字段做范围 clamp
 *
 * 与 parsePointsFile 的差异：
 * - dwellMs 超出 [100, 60000] → clamp 到边界并写入 warnings
 * - samples 超出 [1, 1000] → clamp 到边界并写入 warnings
 * - 负数 / 非有限数（NaN/Infinity）→ 视为未设置（undefined），写入 warnings
 *
 * @param text 文件文本内容
 * @returns { points, warnings }：解析出的点位数组（已 clamp）+ 范围修正警告列表
 */
export function parsePointsFileWithWarnings(text: string): ParsePointsFileResult {
  const raw = parsePointsFile(text)
  const warnings: string[] = []
  const points = raw.map((p, i) => {
    const point: ParsedPoint = { x: p.x, y: p.y, z: p.z, u: p.u }
    // dwellMs clamp + 负数/非有限数过滤
    if (p.dwellMs !== undefined) {
      if (!Number.isFinite(p.dwellMs) || p.dwellMs < PER_POINT_DWELL_MS_MIN) {
        warnings.push(
          `行 ${i + 1}: dwellMs=${p.dwellMs} 小于最小值 ${PER_POINT_DWELL_MS_MIN}，已修正为 ${PER_POINT_DWELL_MS_MIN}`,
        )
        point.dwellMs = PER_POINT_DWELL_MS_MIN
      } else if (p.dwellMs > PER_POINT_DWELL_MS_MAX) {
        warnings.push(
          `行 ${i + 1}: dwellMs=${p.dwellMs} 超过最大值 ${PER_POINT_DWELL_MS_MAX}，已修正为 ${PER_POINT_DWELL_MS_MAX}`,
        )
        point.dwellMs = PER_POINT_DWELL_MS_MAX
      } else {
        point.dwellMs = p.dwellMs
      }
    }
    // samples clamp + 负数/非有限数过滤
    if (p.samples !== undefined) {
      if (!Number.isFinite(p.samples) || p.samples < PER_POINT_SAMPLES_MIN) {
        warnings.push(
          `行 ${i + 1}: samples=${p.samples} 小于最小值 ${PER_POINT_SAMPLES_MIN}，已修正为 ${PER_POINT_SAMPLES_MIN}`,
        )
        point.samples = PER_POINT_SAMPLES_MIN
      } else if (p.samples > PER_POINT_SAMPLES_MAX) {
        warnings.push(
          `行 ${i + 1}: samples=${p.samples} 超过最大值 ${PER_POINT_SAMPLES_MAX}，已修正为 ${PER_POINT_SAMPLES_MAX}`,
        )
        point.samples = PER_POINT_SAMPLES_MAX
      } else {
        point.samples = p.samples
      }
    }
    // test 字段无需 clamp（boolean）
    if (p.test !== undefined) {
      point.test = p.test
    }
    return point
  })
  return { points, warnings }
}
