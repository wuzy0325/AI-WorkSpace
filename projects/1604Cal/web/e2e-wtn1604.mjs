import { chromium } from 'playwright';

const BASE = 'http://localhost:4173';
const API  = 'http://localhost:8080/api/v1';

async function delay(ms) {
  return new Promise(r => setTimeout(r, ms));
}

async function main() {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1400, height: 900 } });

  page.on('response', res => {
    if (res.url().includes('/api/v1/')) {
      console.log(`  API: ${res.request().method()} ${res.url().replace(API, '')} -> ${res.status()}`);
    }
  });

  // 捕获页面 console
  page.on('console', msg => {
    console.log(`  [console.${msg.type()}] ${msg.text()}`);
  });

  // ========== 1. 进入设备管理页面，添加 WTN1604 设备 ==========
  console.log('=== 1. 进入设备管理页面 ===');
  // 先进入首页再点击导航，避免 hash 路由直接加载时组件未渲染
  await page.goto(`${BASE}/`, { waitUntil: 'load', timeout: 15000 });
  await delay(1500);
  await page.locator('a[href="#/device-management"]').click();
  await delay(3000);
  await page.screenshot({ path: 'e2e-01-device-mgmt.png', fullPage: true });

  // 点击"新增设备"
  console.log('点击 新增设备');
  await page.locator('[data-test="add-device"]').waitFor({ state: 'visible', timeout: 10000 });
  await page.locator('[data-test="add-device"]').click();
  await delay(800);

  // 填写表单
  console.log('填写设备表单');
  await page.locator('[data-test="form-name"]').fill('WTN1604-测试设备');
  await page.locator('[data-test="form-type"]').selectOption('measure');
  // 等待型号联动更新
  await delay(300);
  await page.locator('[data-test="form-model"]').selectOption('WTN1604');
  await page.locator('[data-test="form-host"]').fill('192.168.1.7');
  await page.locator('[data-test="form-port"]').fill('9000');
  await page.locator('[data-test="form-unit"]').selectOption('MPa');

  await page.screenshot({ path: 'e2e-02-form-filled.png', fullPage: true });

  // 保存
  console.log('点击 保存');
  await page.locator('[data-test="submit-form"]').click();
  await delay(1500);
  await page.screenshot({ path: 'e2e-03-after-save.png', fullPage: true });

  // 检查设备卡片是否出现
  const cardText = await page.locator('.device-card').first().innerText().catch(() => '');
  console.log(`设备卡片内容: ${cardText.slice(0, 200)}`);

  // ========== 2. 连接设备 ==========
  console.log('\n=== 2. 连接 WTN1604 设备 ===');
  // 找到包含 WTN1604 的设备卡片，点击其中的"连接"按钮
  const deviceCard = page.locator('.device-card').filter({ hasText: 'WTN1604-测试设备' });
  if (await deviceCard.count() === 0) {
    console.error('未找到刚添加的设备卡片');
    await browser.close();
    process.exit(1);
  }

  const connectBtn = deviceCard.locator('button').filter({ hasText: '连接' });
  if (await connectBtn.count() > 0) {
    console.log('点击 连接 按钮');
    await connectBtn.first().click();
    await delay(4000);
    await page.screenshot({ path: 'e2e-04-after-connect.png', fullPage: true });
  } else {
    console.log('连接按钮不存在，可能已连接');
  }

  // 读取连接后状态
  const afterConnectText = await deviceCard.first().innerText().catch(() => '');
  console.log(`连接后卡片状态: ${afterConnectText.slice(0, 300)}`);

  // ========== 3. 进入计量页面，检查单位显示 ==========
  console.log('\n=== 3. 进入计量页面，检查单位 ===');
  await page.goto(`${BASE}/#/measurement`, { waitUntil: 'networkidle', timeout: 15000 });
  await delay(2000);
  await page.screenshot({ path: 'e2e-05-measurement.png', fullPage: true });

  const measBody = await page.locator('body').innerText();
  console.log(`计量页面文字(前800): ${measBody.slice(0, 800)}`);

  // 检查单位类型
  if (measBody.includes('单位类型')) {
    const unitMatch = measBody.match(/单位类型[:\s]+(\S+)/);
    console.log(`读取到的单位: ${unitMatch ? unitMatch[1] : '未解析'}`);
  }

  // 检查阀门状态
  if (measBody.includes('阀门状态')) {
    const valveMatch = measBody.match(/阀门状态[:\s]+([\u4e00-\u9fa5(]+\))/);
    console.log(`读取到的阀门状态: ${valveMatch ? valveMatch[1] : '未解析'}`);
  }

  // ========== 4. 切换阀门状态（间接测试设置命令） ==========
  console.log('\n=== 4. 切换阀门状态（测试设置类命令）===');
  // 在计量页面左侧 Device1604Panel 中，如果已连接，会有"校准模式"和"测量模式"按钮
  const calibrateBtn = page.locator('button').filter({ hasText: '校准模式' });
  const measureBtn = page.locator('button').filter({ hasText: '测量模式' });

  if (await calibrateBtn.count() > 0) {
    console.log('点击 校准模式');
    await calibrateBtn.first().click();
    await delay(2000);
    await page.screenshot({ path: 'e2e-06-calibration-mode.png', fullPage: true });
  }

  if (await measureBtn.count() > 0) {
    console.log('点击 测量模式');
    await measureBtn.first().click();
    await delay(2000);
    await page.screenshot({ path: 'e2e-07-measurement-mode.png', fullPage: true });
  }

  // ========== 5. 回到设备管理页面断开设备 ==========
  console.log('\n=== 5. 断开设备 ===');
  await page.goto(`${BASE}/`, { waitUntil: 'load', timeout: 15000 });
  await delay(1500);
  await page.locator('a[href="#/device-management"]').click();
  await delay(2000);

  const deviceCard2 = page.locator('.device-card').filter({ hasText: 'WTN1604-测试设备' });
  const disconnectBtn = deviceCard2.locator('button').filter({ hasText: '断开' });
  if (await disconnectBtn.count() > 0) {
    console.log('点击 断开 按钮');
    await disconnectBtn.first().click();
    await delay(3000);
    await page.screenshot({ path: 'e2e-08-after-disconnect.png', fullPage: true });
  }

  // ========== 6. 最终状态检查 ==========
  console.log('\n=== 6. 最终状态 ===');
  const finalText = await deviceCard2.first().innerText().catch(() => '');
  console.log(`断开后卡片状态: ${finalText.slice(0, 300)}`);

  const success = finalText.includes('未连接') || finalText.includes('disconnected');
  console.log(`\n测试结果: ${success ? '✓ 流程跑通' : '✗ 流程异常'}`);

  await browser.close();
  process.exit(success ? 0 : 1);
}

main().catch(err => {
  console.error('测试失败:', err);
  process.exit(1);
});
