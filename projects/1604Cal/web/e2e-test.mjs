import { chromium } from 'playwright';

const BASE = 'http://localhost:5173';
const API  = 'http://localhost:8080/api/v1';

async function main() {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1400, height: 900 } });

  page.on('response', res => {
    if (res.url().includes('/api/v1/')) {
      console.log(`  API: ${res.request().method()} ${res.url().replace(API, '')} -> ${res.status()}`);
    }
  });

  // ========== 1. 计量页面 ==========
  console.log('=== 1. 计量页面 ===');
  await page.goto(`${BASE}/#/measurement`, { waitUntil: 'load', timeout: 15000 });
  await page.waitForTimeout(1000);
  await page.screenshot({ path: 'e2e-measurement.png', fullPage: true });

  const measText = await page.locator('body').innerText();
  console.log(`  页面文字(前500): ${measText.substring(0, 500)}`);

  // 查找按钮和选择器
  const buttons = await page.locator('button').allTextContents();
  console.log(`  按钮: ${JSON.stringify(buttons)}`);

  // 查找el-select下拉框
  const selectCount = await page.locator('.el-select').count();
  console.log(`  下拉框: ${selectCount}`);
  for (let i = 0; i < selectCount; i++) {
    const ph = await page.locator('.el-select').nth(i).locator('input').getAttribute('placeholder').catch(() => '');
    console.log(`    select[${i}]: placeholder="${ph}"`);
  }

  // ========== 2. 尝试选择打压设备 ==========
  console.log('\n=== 2. 选择打压设备下拉框 ===');
  // 查找包含"打压"或"压力"文字的select
  const labels = await page.locator('label, .label, .el-form-item__label, [class*="label"]').allTextContents();
  console.log(`  表单标签: ${JSON.stringify(labels)}`);

  // 尝试点击第一个下拉框（如果有的话）
  if (selectCount > 0) {
    console.log('  尝试点击第一个下拉框...');
    await page.locator('.el-select').first().click().catch(() => {});
    await page.waitForTimeout(500);
    await page.screenshot({ path: 'e2e-dropdown-open.png', fullPage: true });

    // 查看下拉选项
    const options = await page.locator('.el-select-dropdown__item, .el-option').allTextContents();
    console.log(`  下拉选项: ${JSON.stringify(options)}`);

    // 点击包含811A的选项
    const opt811a = page.locator('.el-select-dropdown__item, .el-option').filter({ hasText: '811A' });
    if (await opt811a.count() > 0) {
      console.log('  选择811A选项...');
      await opt811a.first().click();
      await page.waitForTimeout(1000);
      await page.screenshot({ path: 'e2e-selected-811a.png', fullPage: true });
    }

    // 按Escape关闭下拉
    await page.keyboard.press('Escape');
    await page.waitForTimeout(300);
  }

  // ========== 3. 标定页面 ==========
  console.log('\n=== 3. 标定页面 ===');
  await page.goto(`${BASE}/#/calibration`, { waitUntil: 'load', timeout: 15000 });
  await page.waitForTimeout(1000);
  await page.screenshot({ path: 'e2e-calibration.png', fullPage: true });

  const calText = await page.locator('body').innerText();
  console.log(`  页面文字(前500): ${calText.substring(0, 500)}`);

  const calButtons = await page.locator('button').allTextContents();
  console.log(`  按钮: ${JSON.stringify(calButtons)}`);

  // ========== 4. 设备管理页面 ==========
  console.log('\n=== 4. 设备管理页面 ===');
  await page.goto(`${BASE}/#/device-mgmt`, { waitUntil: 'load', timeout: 15000 });
  await page.waitForTimeout(1000);
  await page.screenshot({ path: 'e2e-devmgmt.png', fullPage: true });

  const devText = await page.locator('body').innerText();
  console.log(`  页面文字(前500): ${devText.substring(0, 500)}`);

  // ========== 5. 在设备管理页面尝试连接811A ==========
  console.log('\n=== 5. 尝试连接设备 ===');
  // 查找连接按钮
  const connectBtns = await page.locator('button').all();
  for (let i = 0; i < connectBtns.length; i++) {
    const text = await connectBtns[i].textContent();
    const disabled = await connectBtns[i].isDisabled();
    console.log(`  button[${i}]: "${text?.trim()}" disabled=${disabled}`);
  }

  // 查找包含"连接"的按钮并点击
  const connectBtn = page.locator('button').filter({ hasText: '连接' }).first();
  if (await connectBtn.count() > 0) {
    console.log('  找到连接按钮，点击...');
    await connectBtn.click();
    await page.waitForTimeout(3000);
    await page.screenshot({ path: 'e2e-after-connect.png', fullPage: true });

    // 检查连接后的状态
    const afterText = await page.locator('body').innerText();
    console.log(`  连接后状态: ${afterText.includes('connected') || afterText.includes('已连接') ? '已连接' : '未知'}`);
  }

  // ========== 6. 回到计量页面查看 ==========
  console.log('\n=== 6. 连接后回到计量页面 ===');
  await page.goto(`${BASE}/#/measurement`, { waitUntil: 'load', timeout: 15000 });
  await page.waitForTimeout(2000);
  await page.screenshot({ path: 'e2e-after-connect-measurement.png', fullPage: true });

  const finalText = await page.locator('body').innerText();
  console.log(`  最终页面文字(前800): ${finalText.substring(0, 800)}`);

  // 查找压力显示
  if (finalText.includes('MPa') || finalText.includes('kPa') || finalText.includes('压力')) {
    console.log('  ✓ 页面包含压力相关文字');
  } else {
    console.log('  ✗ 页面不包含压力相关文字');
  }

  if (finalText.includes('-0.0') || finalText.includes('0.0')) {
    console.log('  ✓ 页面包含压力数值');
  } else {
    console.log('  ✗ 页面不包含压力数值');
  }

  console.log('\n=== 测试完成 ===');
  await browser.close();
}

main().catch(err => {
  console.error('测试失败:', err);
  process.exit(1);
});
