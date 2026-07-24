import { defineStore } from 'pinia'
import { ref } from 'vue'

/**
 * 探针校准模块许可证 store。
 *
 * 业务背景：
 *   探针校准是付费模块，默认不对客户启用。客户首次点击「探针校准」入口时
 *   需要输入验证码解锁；解锁后状态持久化到 localStorage，后续直接放行。
 *
 * 安全边界（务必阅读）：
 *   1. 本实现为「君子防范」级别——验证码硬编码在前端 JS bundle 中，
 *      理论上可被逆向提取。适用于「普通客户不会翻源码」的场景。
 *   2. localStorage 只保存「已解锁」布尔标记，不保存验证码明文，
 *      避免客户直接从存储中读出密码。
 *   3. 如需更强保护（例如按机器码授权、联网激活），应将校验下沉到
 *      Go 后端，并通过机器指纹绑定。当前需求未达到此级别。
 *
 * 验证码来源：由产品负责人指定，固定为 `i@windtuner`。
 * 如需更换，仅修改下方常量即可，无需改其他文件。
 */
const CALIBRATION_LICENSE_CODE = 'i@windtuner'

/** localStorage 键名——只存布尔标记，不存验证码 */
const STORAGE_KEY = 'wind-daq.calibration.unlocked'

/** localStorage 中表示已解锁的值（用字符串 '1' 而非 'true'，略微混淆） */
const UNLOCKED_FLAG = '1'

export const useCalibrationLicenseStore = defineStore('calibrationLicense', () => {
  // 解锁状态：启动时从 localStorage 读取，避免每次刷新都要求重新输入
  const isUnlocked = ref<boolean>(loadUnlocked())

  /**
   * 从 localStorage 读取解锁标记。
   * 读取失败（隐私模式/存储禁用）视为未解锁，降级为每次要求输入。
   */
  function loadUnlocked(): boolean {
    try {
      return localStorage.getItem(STORAGE_KEY) === UNLOCKED_FLAG
    } catch {
      // localStorage 不可用时，保守视为未解锁
      return false
    }
  }

  /**
   * 将解锁标记写入 localStorage。
   * 写入失败不影响功能——当前会话内仍保持解锁，仅下次启动会重新要求输入。
   */
  function persistUnlocked(): void {
    try {
      localStorage.setItem(STORAGE_KEY, UNLOCKED_FLAG)
    } catch {
      // 静默忽略：隐私模式或存储已满，不阻塞业务流程
    }
  }

  /**
   * 校验验证码是否正确。
   *
   * 实现说明：
   *   - 长度不匹配时提前返回，不是严格意义上的恒定时间比较（会泄漏验证码长度）。
   *   - 但验证码本身硬编码在前端 bundle 中（见文件顶部安全边界说明），攻击者无需
   *     计时即可直接读出，此处比较仅为防止控制台随手输入被旁人偷看到的「君子防范」。
   *   - 浏览器 JS 引擎的 GC/JIT 噪声远大于字符异或耗时，计时攻击在客户端环境本就不可行。
   *   因此这里不追求严格的恒定时间语义，仅做基本的逐字符异或比较。
   */
  function verify(code: string): boolean {
    if (typeof code !== 'string') return false
    const a = code
    const b = CALIBRATION_LICENSE_CODE
    if (a.length !== b.length) return false
    let diff = 0
    for (let i = 0; i < a.length; i++) {
      diff |= a.charCodeAt(i) ^ b.charCodeAt(i)
    }
    return diff === 0
  }

  /**
   * 尝试用给定验证码解锁。
   * - 验证通过：置位 isUnlocked 并持久化，返回 true。
   * - 验证失败：保持原状态，返回 false。
   */
  function unlock(code: string): boolean {
    if (!verify(code)) return false
    isUnlocked.value = true
    persistUnlocked()
    return true
  }

  /** 重置解锁状态（当前无 UI 入口，预留供后续「重新锁定」功能使用） */
  function lock(): void {
    isUnlocked.value = false
    try {
      localStorage.removeItem(STORAGE_KEY)
    } catch {
      // 静默忽略
    }
  }

  return { isUnlocked, verify, unlock, lock }
})
