import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

/**
 * Task 18 / Task 24 契约测试：遍历测试模式开关门禁。
 *
 * 模式开关入口已从 TraversalView 顶部按钮迁移到主导航「遍历测试」子菜单
 * （MainDashboardView + traversalModeStore）。本测试覆盖：
 *   1. mode 持久化到 localStorage（键 wind-daq.traversal.mode）
 *   2. 任一 session 活动时（running/moving/stabilizing/acquiring/saving/paused/isStarting）
 *      模式开关 disabled + toast 显示原因
 *   3. 全部终态后允许切换；切换前清理旧模式订阅与 timer
 *   4. single 模式渲染 TraversalMain，dual 模式渲染 DualTraversalMain
 *   5. 侧边栏点击「遍历测试」弹出子菜单（取代顶部开关行）
 *
 * 测试策略：源代码静态契约检查（避免复杂 mount 依赖）。
 * 后端 equivalent：traversal_manager_registry_test.go 已覆盖 registry 侧门禁。
 */
describe('Traversal mode switch gate (store + nav submenu)', () => {
  const storeSource = readFileSync(
    resolve(__dirname, '../../stores/traversalModeStore.ts'),
    'utf8',
  )
  const viewSource = readFileSync(resolve(__dirname, '../TraversalView.vue'), 'utf8')
  const dashSource = readFileSync(
    resolve(__dirname, '../main/MainDashboardView.vue'),
    'utf8',
  )

  it('mode 持久化到 localStorage 键 wind-daq.traversal.mode', () => {
    expect(storeSource).toContain("'wind-daq.traversal.mode'")
    expect(storeSource).toContain('localStorage.getItem(MODE_STORAGE_KEY)')
    expect(storeSource).toContain('localStorage.setItem(MODE_STORAGE_KEY')
  })

  it('single 模式活动判定包含 canStop 与 isStarting（覆盖 paused/running/启动中）', () => {
    const block = storeSource.slice(
      storeSource.indexOf('const singleActive'),
      storeSource.indexOf('const dualActive'),
    )
    expect(block).toContain('traversalStore.canStop')
    expect(block).toContain('traversalStore.isStarting')
  })

  it('dual 模式活动判定使用 dualTraversalStore.anyActive（覆盖两路所有活动状态）', () => {
    const block = storeSource.slice(
      storeSource.indexOf('const dualActive'),
      storeSource.indexOf('const modeSwitchDisabled'),
    )
    expect(block).toContain('dualTraversalStore.anyActive')
  })

  it('modeSwitchDisabled 根据 mode 选择对应判定', () => {
    const block = storeSource.slice(
      storeSource.indexOf('const modeSwitchDisabled = computed'),
      storeSource.indexOf('const modeSwitchDisabledReason'),
    )
    expect(block).toContain("mode.value === 'single'")
    expect(block).toContain('singleActive.value')
    expect(block).toContain('dualActive.value')
  })

  it('活动时切换被拒绝并提示原因（toast + 不调用 close/reset）', () => {
    const block = storeSource.slice(
      storeSource.indexOf('async function switchMode'),
      storeSource.indexOf('async function cleanupOnLeave'),
    )
    expect(block).toContain('if (modeSwitchDisabled.value)')
    expect(block).toContain('feedbackStore.pushToast')
    expect(block).toContain('modeSwitchDisabledReason.value')
  })

  it('切换前清理旧模式：dual→single 先通过 close gate，再 reset', () => {
    const block = storeSource.slice(
      storeSource.indexOf('async function switchMode'),
      storeSource.indexOf('async function cleanupOnLeave'),
    )
    expect(block).toContain("mode.value === 'dual'")
    expect(block).toContain('closeDualSessions()')
    expect(block).toContain('if (!closed) return false')
    expect(block).toContain("dualTraversalStore.reset('probe1')")
    expect(block).toContain("dualTraversalStore.reset('probe2')")
  })

  it('TraversalView 渲染分支：single→TraversalMain，dual→DualTraversalMain', () => {
    expect(viewSource).toContain("v-if=\"mode === 'single'\"")
    expect(viewSource).toContain('<TraversalMain')
    expect(viewSource).toContain('<DualTraversalMain')
  })

  it('TraversalView 从 traversalModeStore 获取 mode（不再持有本地状态）', () => {
    expect(viewSource).toContain('useTraversalModeStore')
    expect(viewSource).toContain('traversalModeStore.mode')
    // 不应再有本地 MODE_STORAGE_KEY / loadMode / persistMode
    expect(viewSource).not.toContain('MODE_STORAGE_KEY')
    expect(viewSource).not.toContain('function loadMode')
    expect(viewSource).not.toContain('function persistMode')
  })

  it('TraversalView 独占 unmount 本地清理，DualTraversalMain 不重复关闭 server', () => {
    const dualMainSource = readFileSync(
      resolve(__dirname, '../../components/traversal/dual/DualTraversalMain.vue'),
      'utf8',
    )
    const block = viewSource.slice(
      viewSource.indexOf('onBeforeUnmount('),
      viewSource.indexOf('function backFromTraversal'),
    )
    expect(block).toContain('traversalModeStore.cleanupOnLeave')
    expect(dualMainSource).not.toContain('dualStore.close(')
    expect(dualMainSource).not.toContain('onBeforeUnmount')
  })

  it('MainDashboardView 点击「遍历测试」弹出子菜单（不再直接切换页面）', () => {
    // handleRailSelect 对 traversal 项特殊处理：调用 openTraversalModeMenu
    const block = dashSource.slice(
      dashSource.indexOf('function handleRailSelect'),
      dashSource.indexOf('function handleLicenseUnlocked'),
    )
    expect(block).toContain("id === 'traversal'")
    expect(block).toContain('openTraversalModeMenu')
  })

  it('MainDashboardView 子菜单选项触发 switchMode + 进入 traversal 页面', () => {
    const block = dashSource.slice(
      dashSource.indexOf('async function selectTraversalMode'),
      dashSource.indexOf('// 许可证对话框'),
    )
    expect(block).toContain('traversalModeStore.switchMode')
    expect(block).toContain("activePage.value = 'traversal'")
  })

  it('MainDashboardView 子菜单面板渲染单/双探针选项 + 活动检测 disabled', () => {
    expect(dashSource).toContain('traversal-mode-menu')
    expect(dashSource).toContain("selectTraversalMode('single')")
    expect(dashSource).toContain("selectTraversalMode('dual')")
    expect(dashSource).toContain('traversalModeStore.modeSwitchDisabled')
  })

  it('TraversalView 顶部不再渲染模式开关行（已迁移到侧边栏子菜单）', () => {
    expect(viewSource).not.toContain('traversal-mode-switch')
    expect(viewSource).not.toContain('onModeChange')
  })
})
