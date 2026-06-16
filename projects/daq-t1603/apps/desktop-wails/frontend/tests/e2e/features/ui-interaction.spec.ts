import { test, expect } from '@playwright/test'
import { AppPage } from '../pages/AppPage'
import { setupMockBridge, defaultMockState } from '../mock-bridge'
import { SINGLE_DEVICE, createMultiProfiles } from '../fixtures/deviceFixtures'

test.describe('UI Interaction', () => {
  test('should toggle theme between dark and light', async ({ page }) => {
    await setupMockBridge(page, defaultMockState())
    const app = new AppPage(page)
    await app.goto()

    const initialTheme = await page.evaluate(() =>
      document.documentElement.getAttribute('data-theme')
    )

    await app.topBar.clickThemeToggle()

    const newTheme = await page.evaluate(() =>
      document.documentElement.getAttribute('data-theme')
    )
    expect(newTheme).not.toBe(initialTheme)
  })

  test('should show channel cards when device is selected', async ({ page }) => {
    const state = defaultMockState()
    state.profiles = [SINGLE_DEVICE]
    state.statusMap['test_dev_1'] = 'Connected'
    await setupMockBridge(page, state)

    const app = new AppPage(page)
    await app.goto()

    await app.sidebar.selectDevice('测试设备')

    expect(await app.monitorView.isShowingEmptyState()).toBe(false)
    await expect(app.monitorView.chartPanel).toBeVisible()

    const cardCount = await app.monitorView.channelCards.count()
    expect(cardCount).toBe(16)
  })

  test('should toggle channel selection for chart', async ({ page }) => {
    const state = defaultMockState()
    state.profiles = [SINGLE_DEVICE]
    state.statusMap['test_dev_1'] = 'Connected'
    await setupMockBridge(page, state)

    const app = new AppPage(page)
    await app.goto()

    await app.sidebar.selectDevice('测试设备')

    await app.monitorView.toggleChannelSelector()

    const popover = page.getByTestId('channel-popover')
    await expect(popover).toBeVisible()

    await expect(popover.locator('.n-checkbox').first()).toBeVisible()
  })

  test('should show device count in bottom bar', async ({ page }) => {
    const state = defaultMockState()
    state.profiles = createMultiProfiles(2)
    await setupMockBridge(page, state)

    const app = new AppPage(page)
    await app.goto()

    await expect(app.bottomBar.deviceCount).toContainText('2')
  })

  test('should scan for devices', async ({ page }) => {
    await setupMockBridge(page, defaultMockState())
    const app = new AppPage(page)
    await app.goto()

    await app.sidebar.scanButton.click()

    const scanDialog = page.locator('.dialog').filter({ hasText: '扫描设备' })
    await expect(scanDialog).toBeVisible()

    const scanResults = scanDialog.locator('.scan-result, .dialog__body')
    await expect(scanResults).toBeVisible()
  })

  test('should show config dialog when clicking config button', async ({ page }) => {
    const state = defaultMockState()
    state.profiles = [SINGLE_DEVICE]
    await setupMockBridge(page, state)

    const app = new AppPage(page)
    await app.goto()

    await app.sidebar.selectDevice('测试设备')
    await app.monitorView.clickConfig()

    const modal = page.locator('.modal-overlay')
    await expect(modal).toBeVisible()
  })

  test('should display version number', async ({ page }) => {
    await setupMockBridge(page, defaultMockState())
    const app = new AppPage(page)
    await app.goto()

    await expect(app.topBar.versionLabel).toContainText(/v\d+\.\d+\.\d+/)
  })
})
