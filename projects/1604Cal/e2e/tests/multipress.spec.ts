import { test, expect } from '@playwright/test';
import { setupDevice, teardownDevice } from '../fixtures/device-setup';

test.beforeAll(async () => {
  await setupDevice();
});

test.afterAll(async () => {
  await teardownDevice();
});

test.describe('多设备打压控制', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/#/multi-pressure');
    await page.waitForSelector('.multi-press-page', { timeout: 15000 });
  });

  test('TC01: 注册与注销设备', async ({ page }) => {
    const deviceName = 'ConST 811A 测试设备';

    // 1. 找到可用设备卡片并点击注册
    const availableCard = page.locator('.available-card').filter({ hasText: deviceName });
    await expect(availableCard).toBeVisible({ timeout: 10000 });
    await availableCard.getByRole('button', { name: '注册' }).click();

    // 2. 等待已注册卡片出现，并断言状态为“空闲”
    const pressureCard = page.locator('.pressure-card').filter({ hasText: deviceName });
    await expect(pressureCard).toBeVisible({ timeout: 15000 });
    const statusLabel = pressureCard.locator('.status-label');
    await expect(statusLabel).toHaveText('空闲', { timeout: 10000 });

    // 3. 点击注销
    await pressureCard.getByRole('button', { name: '注销' }).click();

    // 4. 等待可用卡片重新出现
    await expect(availableCard).toBeVisible({ timeout: 15000 });
  });
});
