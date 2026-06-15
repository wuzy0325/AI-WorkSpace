import { Page, Locator } from '@playwright/test'

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
    this.brandTitle = page.locator('.topbar__title')
    this.acquisitionButton = page.locator('.topbar__action-btn--start, .topbar__action-btn--stop')
    this.recordButton = page.locator('.topbar__action-btn--record')
    this.addDeviceButton = page.locator('button[title="添加设备"]')
    this.configButton = page.locator('button[title="打开配置"]')
    this.themeToggleButton = page.locator('.topbar__icon-btn').last()
    this.refreshRateButton = page.locator('button[title="界面刷新率"]')
    this.versionLabel = page.locator('.topbar__version')
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
    this.title = page.locator('.sidebar__title')
    this.deviceCount = page.locator('.sidebar__count')
    this.scanButton = page.locator('.sidebar__scan-btn')
    this.emptyState = page.locator('.sidebar__empty')
    this.deviceList = page.locator('.sidebar__list')
    this.deviceItems = page.locator('.sidebar__item')
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
    this.emptyState = page.locator('.detail__empty')
    this.deviceName = page.locator('.detail__device-info h2')
    this.statusTag = page.locator('.detail__header-right .n-tag')
    this.connectButton = page.locator('.detail__header-right .n-button').first()
    this.configButton = page.locator('.detail__header-right .n-button').last()
    this.chartPanel = page.locator('.detail__chart')
    this.channelSelectButton = page.locator('.detail__chart-tools .n-button')
    this.channelGrid = page.locator('.grid')
    this.channelCards = page.locator('.card')
    this.deviceAddressLabel = page.locator('.detail__device-info .n-space .n-text').first()
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
    this.acquisitionStatus = page.locator('.bottombar__status-item').nth(0).locator('.bottombar__status-value')
    this.recordStatus = page.locator('.bottombar__status-item').nth(1).locator('.bottombar__status-value')
    this.deviceCount = page.locator('.bottombar__status-item').nth(2).locator('.bottombar__status-value')
    this.onlineCount = page.locator('.bottombar__status-item').nth(3).locator('.bottombar__status-value')
    this.recordedCount = page.locator('.bottombar__status-item').nth(4).locator('.bottombar__status-value')
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
