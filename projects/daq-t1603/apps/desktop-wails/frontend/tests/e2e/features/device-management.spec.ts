import { test, expect } from '@playwright/test'
import { AppPage, AddDeviceDialog } from '../pages/AppPage'
import { setupMockBridge, defaultMockState } from '../mock-bridge'
import { SINGLE_DEVICE, createMultiProfiles } from '../fixtures/deviceFixtures'

async function connectDevice(app: AppPage, deviceName: string) {
  await app.sidebar.selectDevice(deviceName)
  await expect(app.monitorView.statusTag).toContainText('未连接')
  await app.monitorView.clickConnect()
  await expect(app.monitorView.statusTag).toContainText('已连接')
}

test.describe('Device Management', () => {
  test('should show empty state when no device selected', async ({ page }) => {
    await setupMockBridge(page, defaultMockState())
    const app = new AppPage(page)
    await app.goto()

    expect(await app.monitorView.isShowingEmptyState()).toBe(true)
    expect(await app.sidebar.isEmpty()).toBe(true)
    expect(await app.sidebar.getDeviceCount()).toBe(0)
  })

  test('should add a device via dialog', async ({ page }) => {
    await setupMockBridge(page, defaultMockState())
    const app = new AppPage(page)
    await app.goto()

    await app.topBar.addDeviceButton.click()

    const dialog = new AddDeviceDialog(page)
    await dialog.waitForVisible()

    await dialog.fill({ name: '测试设备', address: '192.168.1.10', port: 9000 })
    await dialog.confirmButton.click()
    await page.waitForTimeout(500)

    await expect(app.sidebar.deviceItems.first()).toBeVisible()
    expect(await app.sidebar.getDeviceCount()).toBe(1)
    await expect(app.sidebar.deviceItems.first()).toContainText('测试设备')
  })

  test('should select a device and show details', async ({ page }) => {
    const state = defaultMockState()
    state.profiles = [SINGLE_DEVICE]
    await setupMockBridge(page, state)

    const app = new AppPage(page)
    await app.goto()

    await app.sidebar.selectDevice('测试设备')

    expect(await app.monitorView.isShowingEmptyState()).toBe(false)
    await expect(app.monitorView.deviceName).toContainText('测试设备')
    await expect(app.monitorView.statusTag).toContainText('未连接')
  })

  test('should delete a device', async ({ page }) => {
    const state = defaultMockState()
    state.profiles = [SINGLE_DEVICE]
    await setupMockBridge(page, state)

    const app = new AppPage(page)
    await app.goto()

    expect(await app.sidebar.getDeviceCount()).toBe(1)

    const deviceItem = app.sidebar.getDeviceItem('测试设备')
    await deviceItem.hover()
    await deviceItem.locator('.device__delete').click()

    await expect(page.locator('.dialog__title').last()).toContainText('确认删除')
    await page.locator('.dialog__btn--danger').click()
    await page.waitForLoadState('networkidle')

    expect(await app.sidebar.getDeviceCount()).toBe(0)
    expect(await app.sidebar.isEmpty()).toBe(true)
    expect(await app.monitorView.isShowingEmptyState()).toBe(true)
  })

  test('should show multiple devices in sidebar', async ({ page }) => {
    const state = defaultMockState()
    state.profiles = createMultiProfiles(3)
    await setupMockBridge(page, state)

    const app = new AppPage(page)
    await app.goto()

    expect(await app.sidebar.getDeviceCount()).toBe(3)
  })

  test('should connect to a device', async ({ page }) => {
    const state = defaultMockState()
    state.profiles = [SINGLE_DEVICE]
    await setupMockBridge(page, state)

    const app = new AppPage(page)
    await app.goto()

    await connectDevice(app, '测试设备')
  })

  test('should disconnect a device', async ({ page }) => {
    const state = defaultMockState()
    state.profiles = [SINGLE_DEVICE]
    await setupMockBridge(page, state)

    const app = new AppPage(page)
    await app.goto()

    await connectDevice(app, '测试设备')

    await app.monitorView.clickConnect()
    await expect(app.monitorView.statusTag).toContainText('未连接')
  })
})
