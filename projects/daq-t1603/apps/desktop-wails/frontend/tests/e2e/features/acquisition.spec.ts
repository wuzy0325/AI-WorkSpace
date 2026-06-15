import { test, expect } from '@playwright/test'
import { AppPage } from '../pages/AppPage'
import { setupMockBridge, defaultMockState, triggerPayload } from '../mock-bridge'
import { SINGLE_DEVICE } from '../fixtures/deviceFixtures'

async function connectDevice(app: AppPage, deviceName: string) {
  await app.sidebar.selectDevice(deviceName)
  await expect(app.monitorView.statusTag).toContainText('未连接')
  await app.monitorView.clickConnect()
  await expect(app.monitorView.statusTag).toContainText('已连接')
}

test.describe('Acquisition', () => {
  test('should start acquisition and update status', async ({ page }) => {
    const state = defaultMockState()
    state.profiles = [SINGLE_DEVICE]
    await setupMockBridge(page, state)

    const app = new AppPage(page)
    await app.goto()

    await connectDevice(app, '测试设备')

    await app.topBar.clickAcquisitionToggle()
    await expect(app.monitorView.statusTag).toContainText('采集中')
  })

  test('should show channel card values when data arrives', async ({ page }) => {
    const state = defaultMockState()
    state.profiles = [SINGLE_DEVICE]
    await setupMockBridge(page, state)

    const app = new AppPage(page)
    await app.goto()

    await connectDevice(app, '测试设备')

    const cardCount = await app.monitorView.channelCards.count()
    expect(cardCount).toBeGreaterThan(0)

    await page.evaluate((deviceId) => {
      const push = (window as any).__mockPushSnapshot
      if (push) {
        for (let i = 0; i < 20; i++) push(deviceId)
      }
    }, SINGLE_DEVICE.id)
    await page.waitForTimeout(500)

    for (let i = 0; i < Math.min(4, cardCount); i++) {
      const val = await app.monitorView.getChannelCardValue(i)
      expect(val).not.toBe('---')
    }
  })

  test('should stop acquisition', async ({ page }) => {
    const state = defaultMockState()
    state.profiles = [SINGLE_DEVICE]
    await setupMockBridge(page, state)

    const app = new AppPage(page)
    await app.goto()

    await connectDevice(app, '测试设备')
    await app.topBar.clickAcquisitionToggle()
    await expect(app.monitorView.statusTag).toContainText('采集中')

    await app.topBar.clickAcquisitionToggle()
    await expect(app.monitorView.statusTag).toContainText('已连接')
  })

  test('should show acquisition status in bottom bar', async ({ page }) => {
    const state = defaultMockState()
    state.profiles = [SINGLE_DEVICE]
    await setupMockBridge(page, state)

    const app = new AppPage(page)
    await app.goto()

    expect(await app.bottomBar.getAcquisitionStatus()).toBe('已停止')

    await connectDevice(app, '测试设备')
    await app.topBar.clickAcquisitionToggle()

    await expect(async () => {
      const status = await app.bottomBar.getAcquisitionStatus()
      expect(status).toBe('运行中')
    }).toPass({ timeout: 3000 })
  })

  test('should handle connect error gracefully', async ({ page }) => {
    const state = defaultMockState()
    state.profiles = [SINGLE_DEVICE]
    state.connectError = 'Connection refused'
    await setupMockBridge(page, state)

    const app = new AppPage(page)
    await app.goto()

    await app.sidebar.selectDevice('测试设备')
    await expect(app.monitorView.statusTag).toContainText('未连接')

    await app.monitorView.clickConnect()
    await page.waitForTimeout(500)

    await expect(app.monitorView.statusTag).toContainText('未连接')
  })
})
