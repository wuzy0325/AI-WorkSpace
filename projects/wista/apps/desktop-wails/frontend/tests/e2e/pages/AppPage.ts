import { Page, Locator, expect } from '@playwright/test'

export class AppPage {
  readonly page: Page

  readonly topBar: TopBarSection
  readonly sidebar: SidebarSection
  readonly monitorView: MonitorViewSection
  readonly bottomBar: BottomBarSection

  constructor(page: Page) {
    this.page = page
    this.topBar = new TopBarSection(page)
    this.sidebar = new SidebarSection(page)
    this.monitorView = new MonitorViewSection(page)
    this.bottomBar = new BottomBarSection(page)
  }

  async goto() {
    await this.page.goto('/')
    await this.page.waitForLoadState('networkidle')
  }

  /** 选中设备并连接，断言状态从"未连接"变为"已连接" */
  async connectDevice(deviceName: string) {
    await this.sidebar.selectDevice(deviceName)
    await expect(this.monitorView.statusTag).toContainText('未连接')
    await this.monitorView.clickConnect()
    await expect(this.monitorView.statusTag).toContainText('已连接')
  }
}

export class TopBarSection {
  readonly page: Page

  readonly brandTitle: Locator
  readonly acquisitionButton: Locator
  readonly recordButton: Locator
  readonly addDeviceButton: Locator
  readonly configButton: Locator
  readonly themeToggleButton: Locator
  readonly refreshRateButton: Locator
  readonly versionLabel: Locator

  constructor(page: Page) {
    this.page = page
    this.brandTitle = page.getByTestId('topbar-title')
    this.acquisitionButton = page.getByTestId('btn-acquisition')
    this.recordButton = page.getByTestId('btn-record')
    this.addDeviceButton = page.getByTestId('btn-add-device')
    this.configButton = page.getByTestId('btn-config')
    this.themeToggleButton = page.getByTestId('btn-theme-toggle')
    this.refreshRateButton = page.locator('button[title="界面刷新率"]')
    this.versionLabel = page.getByTestId('topbar-version')
  }

  async clickAcquisitionToggle() {
    await this.acquisitionButton.click()
    await this.page.waitForLoadState('networkidle')
  }

  async clickRecordToggle() {
    await this.recordButton.click()
    await this.page.waitForLoadState('networkidle')
  }

  async clickThemeToggle() {
    await this.themeToggleButton.click()
    await this.page.waitForTimeout(300)
  }

  async isAcquiring(): Promise<boolean> {
    const cls = await this.acquisitionButton.getAttribute('class')
    return cls?.includes('--stop') ?? false
  }

  async isRecording(): Promise<boolean> {
    const cls = await this.recordButton.getAttribute('class')
    return cls?.includes('--recording') ?? false
  }
}

export class SidebarSection {
  readonly page: Page
  readonly title: Locator
  readonly deviceCount: Locator
  readonly scanButton: Locator
  readonly emptyState: Locator
  readonly deviceList: Locator
  readonly deviceItems: Locator

  constructor(page: Page) {
    this.page = page
    this.title = page.getByTestId('sidebar-title')
    this.deviceCount = page.getByTestId('sidebar-count')
    this.scanButton = page.getByTestId('btn-scan')
    this.emptyState = page.getByTestId('sidebar-empty')
    this.deviceList = page.getByTestId('sidebar-list')
    this.deviceItems = page.getByTestId('sidebar-item')
  }

  getDeviceItem(deviceName: string): Locator {
    return this.deviceItems.filter({ hasText: deviceName })
  }

  async getDeviceCount(): Promise<number> {
    return await this.deviceItems.count()
  }

  async selectDevice(deviceName: string) {
    await this.getDeviceItem(deviceName).click()
    await this.page.waitForLoadState('networkidle')
  }

  async isEmpty(): Promise<boolean> {
    return await this.emptyState.isVisible()
  }
}

export class MonitorViewSection {
  readonly page: Page
  readonly emptyState: Locator
  readonly deviceName: Locator
  readonly statusTag: Locator
  readonly connectButton: Locator
  readonly configButton: Locator
  readonly chartPanel: Locator
  readonly channelSelectButton: Locator
  readonly channelGrid: Locator
  readonly channelCards: Locator
  readonly deviceAddressLabel: Locator

  constructor(page: Page) {
    this.page = page
    this.emptyState = page.getByTestId('detail-empty')
    this.deviceName = page.getByTestId('detail-device-info').locator('h2')
    this.statusTag = page.getByTestId('detail-header-right').locator('.n-tag')
    this.connectButton = page.getByTestId('detail-header-right').locator('.n-button').first()
    this.configButton = page.getByTestId('detail-header-right').locator('.n-button').last()
    this.chartPanel = page.getByTestId('detail-chart')
    this.channelSelectButton = page.getByTestId('detail-chart').locator('.detail__chart-tools .n-button')
    this.channelGrid = page.locator('.grid')
    this.channelCards = page.locator('.card')
    this.deviceAddressLabel = page.getByTestId('detail-device-info').locator('.n-text').first()
  }

  async isShowingEmptyState(): Promise<boolean> {
    return await this.emptyState.isVisible()
  }

  async getStatusText(): Promise<string> {
    return await this.statusTag.textContent() ?? ''
  }

  async clickConnect() {
    await this.connectButton.click()
    await this.page.waitForTimeout(500)
  }

  async clickConfig() {
    await this.configButton.click()
    await this.page.waitForLoadState('networkidle')
  }

  async getChannelCardValue(index: number): Promise<string> {
    return await this.channelCards.nth(index).locator('.card__value').textContent() ?? ''
  }

  async toggleChannelSelector() {
    await this.channelSelectButton.click()
    await this.page.waitForTimeout(200)
  }
}

export class BottomBarSection {
  readonly page: Page
  readonly acquisitionStatus: Locator
  readonly recordStatus: Locator
  readonly deviceCount: Locator
  readonly onlineCount: Locator
  readonly recordedCount: Locator

  constructor(page: Page) {
    this.page = page
    this.acquisitionStatus = page.getByTestId('status-acquisition').locator('.bottombar__status-value')
    this.recordStatus = page.getByTestId('status-recording').locator('.bottombar__status-value')
    this.deviceCount = page.getByTestId('status-devices').locator('.bottombar__status-value')
    this.onlineCount = page.getByTestId('status-online').locator('.bottombar__status-value')
    this.recordedCount = page.getByTestId('status-recorded').locator('.bottombar__status-value')
  }

  async getAcquisitionStatus(): Promise<string> {
    return await this.acquisitionStatus.textContent() ?? ''
  }

  async getRecordStatus(): Promise<string> {
    return await this.recordStatus.textContent() ?? ''
  }
}

export class AddDeviceDialog {
  readonly page: Page
  readonly dialog: Locator
  readonly nameInput: Locator
  readonly addressInput: Locator
  readonly portInput: Locator
  readonly confirmButton: Locator
  readonly cancelButton: Locator
  readonly errorText: Locator

  constructor(page: Page) {
    this.page = page
    this.dialog = page.locator('.dialog').filter({ hasText: '添加 T1603 设备' })
    this.nameInput = page.locator('input').first()
    this.addressInput = page.locator('input').nth(1)
    this.portInput = page.locator('input[type="number"]')
    this.confirmButton = this.dialog.locator('.dialog__btn--primary')
    this.cancelButton = this.dialog.locator('.dialog__btn--secondary')
    this.errorText = this.dialog.locator('.dialog__error')
  }

  async waitForVisible() {
    await this.dialog.waitFor({ state: 'visible', timeout: 5000 })
    await this.page.waitForTimeout(300)
  }

  async fill(data: { name: string; address: string; port: number }) {
    await this.nameInput.waitFor({ state: 'visible', timeout: 5000 })
    await this.nameInput.fill(data.name)
    await this.addressInput.fill(data.address)
    await this.portInput.fill(String(data.port))
  }

  async isVisible(): Promise<boolean> {
    return await this.dialog.isVisible()
  }
}
