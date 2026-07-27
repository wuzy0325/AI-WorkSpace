// useSevenHoleCalibration.ts 抽取 SevenHoleWorkspace.vue 的校准文件加载逻辑：
//   - 数据源切换（PRB / 校准 CSV）
//   - 7 槽位状态管理（内区 + 6 外区）
//   - 批量导入 + 单槽位选择 + 移除
//   - 7 份齐备后自动调用后端加载
//   - 后端已加载状态的恢复（onMounted 路径）
//
// 抽出原因：SevenHoleWorkspace.vue 单文件超 1400 行，校准逻辑约 280 行独立成块，
// 与数据输入/计算/导出逻辑耦合度低。抽出后父组件专注于 UI 展示，加载状态机独立维护。
//
// 设计原则：本 composable 持有全部校准状态（loaded / dataSource / innerFile / outerFiles 等），
// 父组件通过返回值访问。setStatus 回调由父组件注入，便于将加载进度同步到状态栏。

import { ref, computed, onMounted, type Ref } from 'vue'
import {
  api,
  isWailsAvailable,
  type SevenHolePrbFileInfo,
  type SevenHolePrbValidRange,
  type SevenHoleDataSource,
  type SevenHoleLoadPrbResult,
  type GenericResponse,
} from '../adapters/seven-hole'
import {
  detectSevenHoleBatchFormat,
  assignSevenHoleFilesByName,
  assignSevenHoleCsvFilesByName,
  fileNameOf,
} from '../adapters/seven-hole-helpers'

/** 状态栏支持的消息类型，与父组件 statusType 对齐 */
type StatusType = 'info' | 'success' | 'error' | 'warning'

/** useSevenHoleCalibration 入参：父组件注入状态栏回调 */
export interface UseSevenHoleCalibrationOptions {
  /** setStatus 由父组件提供，加载进度通过此回调同步到状态栏 */
  setStatus: (msg: string, type?: StatusType) => void
}

/** useSevenHoleCalibration 返回值：父组件透传到模板 */
export interface UseSevenHoleCalibrationReturn {
  // 状态
  loaded: Ref<boolean>
  dataSource: Ref<SevenHoleDataSource>
  innerFile: Ref<SevenHolePrbFileInfo | null>
  outerFiles: Ref<(SevenHolePrbFileInfo | null)[]>
  validRange: Ref<SevenHolePrbValidRange | null>
  innerPointCount: Ref<number>
  outerPointCounts: Ref<number[]>
  isImporting: Ref<boolean>
  // 计算属性
  isComplete: Ref<boolean>
  isCsvSource: Ref<boolean>
  validRangeText: Ref<string>
  dataSourceText: Ref<string>
  // 操作
  setSource: (source: 'prb' | 'calibration-csv') => void
  batchImport: () => Promise<void>
  pickInner: () => Promise<void>
  pickOuter: (sector: number) => Promise<void>
  removeInner: () => void
  removeOuter: (sector: number) => void
}

/**
 * useSevenHoleCalibration 封装 7 孔校准文件加载的全状态机。
 *
 * 状态流转：
 *   未加载 → 选择数据源 → 选择 7 份文件 → 自动加载 → loaded=true
 *                              ↑                    ↓
 *                              └── 移除文件 ←──── 重新选择
 *
 * 数据源切换时清空旧槽位，避免 PRB / CSV 两种解析入口混用。
 */
export function useSevenHoleCalibration(
  options: UseSevenHoleCalibrationOptions,
): UseSevenHoleCalibrationReturn {
  const { setStatus } = options

  // ==================== 状态管理 ====================
  const loaded = ref(false)
  const dataSource = ref<SevenHoleDataSource>('') // 'prb' | 'calibration-csv' | ''
  const innerFile = ref<SevenHolePrbFileInfo | null>(null)
  const outerFiles = ref<(SevenHolePrbFileInfo | null)[]>([null, null, null, null, null, null])
  const validRange = ref<SevenHolePrbValidRange | null>(null)
  const innerPointCount = ref(0)
  const outerPointCounts = ref<number[]>([0, 0, 0, 0, 0, 0])
  const isImporting = ref(false)

  // ==================== 计算属性 ====================
  const isComplete = computed(
    () => innerFile.value !== null && outerFiles.value.every((f) => f !== null),
  )
  const isCsvSource = computed(() => dataSource.value === 'calibration-csv')

  const validRangeText = computed(() => {
    if (!validRange.value) return ''
    const v = validRange.value
    return `α: ${v.alphaMin.toFixed(0)}°~${v.alphaMax.toFixed(0)}°, β: ${v.betaMin.toFixed(0)}°~${v.betaMax.toFixed(0)}°`
  })

  const dataSourceText = computed(() => {
    if (!loaded.value) return '未加载'
    return isCsvSource.value ? '校准 CSV' : 'PRB 文件集'
  })

  // ==================== 数据源切换 ====================
  /**
   * setSource 切换数据源（PRB / 校准 CSV）。
   * 切换时清空旧格式槽位，避免混用两种解析入口导致后端解析失败。
   */
  function setSource(source: 'prb' | 'calibration-csv'): void {
    if (isImporting.value || source === dataSource.value) return
    dataSource.value = source
    innerFile.value = null
    outerFiles.value = [null, null, null, null, null, null]
    validRange.value = null
    innerPointCount.value = 0
    outerPointCounts.value = [0, 0, 0, 0, 0, 0]
    loaded.value = false
    setStatus(`已切换到 ${source === 'calibration-csv' ? '校准 CSV' : 'PRB'} 数据源，请选择 7 份文件`, 'info')
  }

  // ==================== 批量导入 ====================
  /**
   * batchImport 批量导入：一次多选全部文件，按所选文件自动识别格式并分配槽位。
   * 7 份齐备后自动触发后端加载。
   */
  async function batchImport(): Promise<void> {
    if (!isWailsAvailable()) {
      setStatus('当前不在 Wails 环境中运行', 'error')
      return
    }
    if (isImporting.value) return
    isImporting.value = true
    try {
      const [resp, paths] = await api.pickFiles()
      if (!resp.success) {
        setStatus('选择文件失败: ' + (resp.error || '未知错误'), 'error')
        return
      }
      if (paths.length === 0) return // 用户取消
      if (paths.length !== 7) {
        setStatus(`请选择完整的 7 份同格式校准文件，当前选择 ${paths.length} 份`, 'error')
        return
      }

      const format = detectSevenHoleBatchFormat(paths)
      if (format === 'mixed' || format === 'empty') {
        setStatus('文件格式混合或为空，请统一选择 .prb 或 .csv 文件', 'warning')
        return
      }

      // 按 basename 分配槽位
      const assignment = format === 'calibration-csv'
        ? assignSevenHoleCsvFilesByName(paths)
        : assignSevenHoleFilesByName(paths)

      if (assignment.unmatched.length > 0 || !assignment.innerFile || assignment.outerFiles.size !== 6) {
        setStatus(
          format === 'calibration-csv'
            ? 'CSV 文件不完整：需要 1 份小角度区和大角度 1~6 区文件'
            : 'PRB 文件不完整：需要 1.prb~7.prb',
          'error',
        )
        return
      }

      dataSource.value = format
      innerFile.value = assignment.innerFile
      const nextOuter: (SevenHolePrbFileInfo | null)[] = [null, null, null, null, null, null]
      for (const [sector, info] of assignment.outerFiles) {
        nextOuter[sector - 1] = info
      }
      outerFiles.value = nextOuter

      await importIfComplete()
    } finally {
      isImporting.value = false
    }
  }

  // ==================== 单槽位选择 ====================
  async function pickInner(): Promise<void> {
    if (!isWailsAvailable() || isImporting.value) return
    isImporting.value = true
    try {
      const [resp, paths] = await api.pickFiles()
      if (!resp.success) {
        setStatus('选择文件失败: ' + (resp.error || '未知错误'), 'error')
        return
      }
      if (paths.length === 0) return
      const path = paths[0]
      innerFile.value = {
        filePath: path,
        fileName: fileNameOf(path),
        sector: 7,
        pointCount: 0, // 后端加载完成后回填
      }
      await importIfComplete()
    } finally {
      isImporting.value = false
    }
  }

  async function pickOuter(sector: number): Promise<void> {
    if (!isWailsAvailable() || isImporting.value) return
    isImporting.value = true
    try {
      const [resp, paths] = await api.pickFiles()
      if (!resp.success) {
        setStatus('选择文件失败: ' + (resp.error || '未知错误'), 'error')
        return
      }
      if (paths.length === 0) return
      const path = paths[0]
      const next = [...outerFiles.value]
      next[sector - 1] = {
        filePath: path,
        fileName: fileNameOf(path),
        sector,
        pointCount: 0, // 后端加载完成后回填
      }
      outerFiles.value = next
      await importIfComplete()
    } finally {
      isImporting.value = false
    }
  }

  function removeInner(): void {
    innerFile.value = null
    validRange.value = null
    innerPointCount.value = 0
    loaded.value = false
    setStatus('已移除内区文件，请重新选择', 'info')
  }

  function removeOuter(sector: number): void {
    // 不可变写法：用新数组替换 ref.value，与 pickOuter / batchImport 风格一致。
    const nextOuter = [...outerFiles.value]
    nextOuter[sector - 1] = null
    outerFiles.value = nextOuter
    const nextCounts = [...outerPointCounts.value]
    nextCounts[sector - 1] = 0
    outerPointCounts.value = nextCounts
    loaded.value = false
    setStatus(`已移除外区 ${sector} 文件，请重新选择`, 'info')
  }

  // ==================== 自动加载 ====================
  /**
   * importIfComplete 在 7 份文件齐备后按数据源自动调用后端加载。
   * 失败保留槽位由用户修正——后端错误信息含文件路径便于定位。
   */
  async function importIfComplete(): Promise<void> {
    if (innerFile.value === null || outerFiles.value.some((f) => f === null)) return
    const inner = innerFile.value
    // 用类型守卫替代 as 断言：让 TS 编译器在编译期保证 outer 无 null，
    // 而非依赖注释 + 人脑推理。若未来某次重构打破"some 后必无 null"不变量，
    // 此处的 filter 会在运行时把 null 重新过滤掉，避免悄悄传入 null 给后端。
    const outer = outerFiles.value.filter(
      (f): f is SevenHolePrbFileInfo => f !== null,
    )
    // 7 槽位 some 已校验非空，filter 后长度必为 6；防御性断言以早失败。
    if (outer.length !== 6) {
      setStatus('加载失败: 外区文件槽位异常', 'error')
      return
    }
    const outerPaths = outer.map((f) => f.filePath)

    isImporting.value = true
    let resp: GenericResponse
    let result: SevenHoleLoadPrbResult | null = null
    try {
      ;[resp, result] = dataSource.value === 'calibration-csv'
        ? await api.loadCalibrationCsvFiles(inner.filePath, outerPaths)
        : await api.loadPrbFiles(inner.filePath, outerPaths)
    } finally {
      isImporting.value = false
    }

    if (!resp.success || !result || !Array.isArray(result.files)) {
      setStatus('加载失败: ' + (resp.error || 'PRB 文件信息为空'), 'error')
      return
    }

    // 用服务端返回的逐文件信息回填槽位（含 pointCount 等）
    const innerRet = result.files.find((f) => f.sector === 7)
    if (innerRet) {
      innerFile.value = { ...inner, ...innerRet }
    }
    const nextOuter = outer.map((slot, i) => {
      const ret = result.files.find((f) => f.sector === i + 1)
      return ret ? { ...slot, ...ret } : slot
    })
    outerFiles.value = nextOuter

    validRange.value = result.validRange ?? null
    innerPointCount.value = result.innerPointCount ?? 0
    outerPointCounts.value = [...result.outerPointCounts]
    loaded.value = true

    const sourceLabel = dataSource.value === 'calibration-csv' ? '校准 CSV' : 'PRB'
    const totalPoints = result.innerPointCount + result.outerPointCounts.reduce((a, b) => a + b, 0)
    if (result.warnings && result.warnings.length > 0) {
      setStatus(
        `已加载 ${result.files.length} 个 ${sourceLabel} 文件（${totalPoints} 点，含 ${result.warnings.length} 条警告）`,
        'warning',
      )
    } else {
      setStatus(`已加载 ${result.files.length} 个 ${sourceLabel} 文件，共 ${totalPoints} 个网格点`, 'success')
    }
  }

  // ==================== 初始化：恢复后端已加载状态 ====================
  // 用户从其他探针工作区切回 7 孔时，后端 sevenHoleState 可能仍保留上次加载的文件。
  // 此处查询 dataSource + prbFiles，若已加载则回填槽位 UI。
  //
  // pointCount 派生：后端 GetSevenHolePrbFiles 返回的 SevenHolePrbFileInfo 已包含
  // 每份文件的 pointCount（LoadPrbFiles / LoadCalibrationCsvFiles 成功路径回填）。
  // 直接从 file 元信息派生 innerPointCount / outerPointCounts，避免重新加载或新增
  // 后端 API；与 importIfComplete 路径填充的字段保持一致，防止"切换工作区后已加载
  // 卡片显示 0 个网格点"的回归。
  onMounted(async () => {
    if (!isWailsAvailable()) return
    try {
      const [dsResp, ds] = await api.getDataSource()
      if (!dsResp.success || !ds) return
      dataSource.value = ds

      const files = await api.getPrbFiles()
      if (!files || files.length === 0) return
      const inner = files.find((f) => f.sector === 7) ?? null
      const outer: (SevenHolePrbFileInfo | null)[] = [null, null, null, null, null, null]
      for (let i = 0; i < 6; i++) {
        outer[i] = files.find((f) => f.sector === i + 1) ?? null
      }
      innerFile.value = inner
      outerFiles.value = outer

      // 从恢复的 file.pointCount 派生网格点计数，避免显示"0 个网格点"的回归。
      // inner 缺失时记 0；outer 任意槽位缺失时该扇区记 0（与未加载状态保持一致）。
      innerPointCount.value = inner?.pointCount ?? 0
      const nextOuterCounts = [0, 0, 0, 0, 0, 0]
      for (let i = 0; i < 6; i++) {
        nextOuterCounts[i] = outer[i]?.pointCount ?? 0
      }
      outerPointCounts.value = nextOuterCounts

      const isLoaded = await api.isPrbLoaded()
      if (isLoaded) {
        loaded.value = true
        const [vrResp, vr] = await api.getValidRange()
        if (vrResp.success && vr) {
          validRange.value = vr
        }
      }
    } catch (e) {
      // 静默失败：恢复状态失败不阻塞 UI，用户可重新选文件
      // 仍记录到 console 便于开发态排查"为什么状态没恢复"。
      console.warn('[useSevenHoleCalibration] 恢复状态失败:', e)
    }
  })

  return {
    loaded,
    dataSource,
    innerFile,
    outerFiles,
    validRange,
    innerPointCount,
    outerPointCounts,
    isImporting,
    isComplete,
    isCsvSource,
    validRangeText,
    dataSourceText,
    setSource,
    batchImport,
    pickInner,
    pickOuter,
    removeInner,
    removeOuter,
  }
}
