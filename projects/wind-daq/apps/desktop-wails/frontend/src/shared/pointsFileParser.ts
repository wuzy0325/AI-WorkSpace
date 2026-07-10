/**
 * TXT/CSV 点位文件解析器
 *
 * 用于遍历测试·自定义点模式批量导入点位列表。前端纯解析，不依赖后端文件 IO。
 *
 * 支持格式：
 * - CSV（逗号分隔）：首行可含表头（X/Y/Z/U 或 pos_x 等别名）
 * - TSV（Tab / 空格分隔）：同上
 * - 无表头纯数据：列顺序固定为 X,Y,Z,U
 *
 * 缺列按 0 填充，不抛错。
 */

/** 标准轴名 */
export type AxisKey = 'x' | 'y' | 'z' | 'u'

/** 解析出的点位结构 */
export interface ParsedPoint {
  x: number
  y: number
  z: number
  u: number
}

/**
 * 列名 → 标准轴名归一化映射
 *
 * 算法：trim → lowercase → 移除括号注释（含全角）→ 逐轴正则匹配
 *
 * 匹配的别名（大小写不敏感）：
 * - X: "x", "posx", "pos_x"
 * - Y: "y", "posy", "pos_y"
 * - Z: "z", "posz", "pos_z"
 * - U: "u", "posu", "pos_u", "α", "alpha"
 *
 * 不匹配返回 null
 */
export function normalizeAxisName(raw: string): AxisKey | null {
  const cleaned = raw.trim().toLowerCase()
  // 移除括号注释（如 "X(mm)" → "x", "U(°)" → "u"），含全角括号
  const base = cleaned.replace(/\(.*\)|（.*）/, '').trim()
  if (/^(x|pos[_]?x)$/.test(base)) return 'x'
  if (/^(y|pos[_]?y)$/.test(base)) return 'y'
  if (/^(z|pos[_]?z)$/.test(base)) return 'z'
  // U 轴别名包含希腊字母 α 和英文 alpha
  if (/^(u|pos[_]?u|[α]|alpha)$/.test(base)) return 'u'
  return null
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
 * 说明：格式错误的行会被跳过，不抛错
 */
export function parsePointsFile(text: string): ParsedPoint[] {
  if (!text || !text.trim()) return []

  const delimiter = detectDelimiter(text)
  const lines = text.split(/\r?\n/).filter((line) => line.trim().length > 0)
  if (lines.length === 0) return []

  // 检测首行是否为表头：任一字段含字母或为已知列名别名 → 视为表头
  const firstFields = splitLine(lines[0], delimiter)
  const hasHeader = firstFields.some((f) => /[a-zA-Zα]/.test(f))

  // 列映射：column index → AxisKey
  let columnMap: Map<number, AxisKey> = new Map()
  let dataStartIndex = 0

  if (hasHeader) {
    // 解析表头列映射
    firstFields.forEach((field, idx) => {
      const axis = normalizeAxisName(field)
      if (axis) columnMap.set(idx, axis)
    })
    dataStartIndex = 1
    // 表头无任何已知列名 → 视为无效，回退默认 X,Y,Z,U 顺序
    if (columnMap.size === 0) {
      columnMap = new Map([[0, 'x'], [1, 'y'], [2, 'z'], [3, 'u']])
    }
  } else {
    // 无表头：默认列顺序 X,Y,Z,U
    columnMap = new Map([[0, 'x'], [1, 'y'], [2, 'z'], [3, 'u']])
  }

  const result: ParsedPoint[] = []
  for (let i = dataStartIndex; i < lines.length; i++) {
    const fields = splitLine(lines[i], delimiter)
    // 缺列按 0 填充
    const point: ParsedPoint = { x: 0, y: 0, z: 0, u: 0 }
    let hasValidValue = false
    for (const [idx, axis] of columnMap) {
      const raw = fields[idx]
      if (raw === undefined) continue
      const value = parseFloat(raw)
      if (!Number.isNaN(value)) {
        point[axis] = value
        hasValidValue = true
      }
    }
    // 全行无有效数字（如空行 / 仅含分隔符）跳过
    if (hasValidValue) {
      result.push(point)
    }
  }

  return result
}
