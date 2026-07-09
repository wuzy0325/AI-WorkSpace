/**
 * 校准 CSV 默认文件名与路径拼接工具。
 *
 * 用于四个校准 Settings 组件（五孔/三孔/总压/总温）的"选择 CSV 保存目录"按钮
 * 以及 useCalibrationWorkflow 的"导出 CSV"按钮，统一文件名清洗规则，
 * 避免同一逻辑在 5 处重复维护。
 */

/**
 * 构建校准 CSV 默认文件名：清洗非法字符 + 去重日期后缀。
 *
 * 清洗规则：
 *   - 配置名可能含日期斜杠（如"三孔探针校准-2026/7/9"）或其他文件名非法字符
 *     （/ \ : * ? " < > |），直接拼进文件名会被文件系统/原生对话框解析为
 *     路径分隔符，导致默认名非法或落到错误子目录
 *   - 配置名末尾若已含日期（YYYY-M-D 或 YYYY-MM-DD，含单位数月日写法），
 *     不再追加当天日期避免重复。早期正则只认 ISO 两位数 \d{4}-\d{2}-\d{2}，
 *     导致用户手写的"五孔探针-2026-6-24"被识别为无日期，再追加成
 *     "五孔探针-2026-6-24-2026-07-09"。放宽到 \d{1,2} 覆盖单位数月日。
 *
 * @param rawName 用户输入的配置名（可能含非法字符或日期）
 * @param fallback rawName 清洗后为空时的回退名（如 'three-hole'）
 * @returns 形如 "三孔探针校准-2026-07-09.csv" 的文件名
 */
export function buildCalibrationCsvName(rawName: string, fallback: string): string {
  const safeName = rawName.replace(/[\\/:*?"<>|]/g, '-').trim() || fallback
  // 末尾日期检测：放宽到单位数月日，覆盖 YYYY-M-D 与 YYYY-MM-DD 两种写法。
  const hasDateSuffix = /\d{4}-\d{1,2}-\d{1,2}$/.test(safeName)
  const dateSuffix = hasDateSuffix ? '' : `-${new Date().toISOString().slice(0, 10)}`
  return `${safeName}${dateSuffix}.csv`
}

/**
 * 拼接目录与文件名为完整路径，归一化为 POSIX 风格。
 *
 * Windows 下 pickDirectory 返回的路径可能含反斜杠（如 "D:\data"），
 * 末尾也可能带分隔符。直接拼接会与正斜杠混用，导致后端解析异常。
 * 统一去除末尾分隔符 + 反斜杠转正斜杠。
 */
export function joinCalibrationPath(dir: string, fileName: string): string {
  const normalizedDir = dir.replace(/[\\/]+$/, '').replace(/\\/g, '/')
  return `${normalizedDir}/${fileName}`
}
