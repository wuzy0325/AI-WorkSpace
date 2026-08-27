const API_BASE = 'http://localhost:8080/api/v1';
const DEVICE_ID = '811a-test-01';

/**
 * 创建并连接 811A 测试设备。
 * 若设备已存在（409），则忽略；其余异常直接抛出。
 */
async function setupDevice(): Promise<void> {
  // 1. 创建设备配置
  const createRes = await fetch(`${API_BASE}/devices`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      id: DEVICE_ID,
      name: 'ConST 811A 测试设备',
      type: 'pressure',
      model: '811A',
      host: '192.168.3.131',
      port: 8000,
      unit: 'kPa',
    }),
  });

  if (!createRes.ok && createRes.status !== 409) {
    throw new Error(`setupDevice: 创建设备失败，状态码 ${createRes.status}`);
  }

  // 2. 连接设备
  const connectRes = await fetch(`${API_BASE}/devices/connect`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id: DEVICE_ID }),
  });

  if (!connectRes.ok) {
    throw new Error(`setupDevice: 连接设备失败，状态码 ${connectRes.status}`);
  }
}

/**
 * 清理 811A 测试设备：停止多点任务、注销设备、断开连接。
 * 所有步骤的异常均被忽略。
 */
async function teardownDevice(): Promise<void> {
  try {
    await fetch(`${API_BASE}/multipress/stop-all`, { method: 'POST' });
  } catch {
    // 忽略错误
  }

  try {
    await fetch(`${API_BASE}/multipress/unregister`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ deviceId: DEVICE_ID }),
    });
  } catch {
    // 忽略错误
  }

  try {
    await fetch(`${API_BASE}/devices/disconnect`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id: DEVICE_ID }),
    });
  } catch {
    // 忽略错误
  }
}

export { API_BASE, DEVICE_ID, setupDevice, teardownDevice };
