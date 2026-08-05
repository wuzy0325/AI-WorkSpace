// 小程序侧 .prb / .csv 文件选择与读取封装
//
// 关键兼容性问题：
// wx.chooseMessageFile 在不同客户端返回的 tempFiles[i].path 格式不一致：
//   - 手机微信：wxfile://tmp_xxx  → FileSystemManager.readFile 可读 ✅
//   - 开发者工具模拟器：http://tmp/xxx → readFile 可读 ✅
//   - PC 微信客户端：http://tmpxxx.ext（无 /）→ readFile 报 "not found" ❌
//
// 兜底策略（readFileWithFallback）：
//   1. 直接 readFile（手机端/模拟器主路径）
//   2. 失败 → copyFile 到 wx.env.USER_DATA_PATH 再读（绕过 PC 微信 URL 解析）
//   3. 还失败 → saveFile 到本地后读（最后一道兜底）
//   4. 都失败 → 抛出含原始 path + 三种客户端建议的错误

// 生成不冲突的本地存储文件名（避免同次选多个同名文件覆盖）
function genLocalName(originalName) {
  // 用时间戳 + 随机串 + 原文件名，避免冲突；USER_DATA_PATH 单文件名最长 255 字符够用
  const stamp = Date.now().toString(36);
  const rand = Math.random().toString(36).slice(2, 8);
  return `${stamp}_${rand}_${originalName || 'unknown'}`;
}

// 多级 fallback 读取文件文本
// tempFilePath: wx.chooseMessageFile 返回的 path
// 返回 string（utf-8 文本）
function readTextWithFallback(tempFilePath, originalName) {
  return new Promise((resolve, reject) => {
    const fs = wx.getFileSystemManager();
    const userDir = wx.env.USER_DATA_PATH;
    const localPath = `${userDir}/${genLocalName(originalName)}`;

    // 第 1 级：直接读临时文件（手机端/模拟器主路径）
    fs.readFile({
      filePath: tempFilePath,
      encoding: 'utf-8',
      success: (res) => resolve(typeof res.data === 'string' ? res.data : ''),
      fail: (err1) => {
        // 第 2 级：copyFile 到 USER_DATA_PATH 再读（PC 微信 http://tmpxxx 兜底）
        fs.copyFile({
          srcPath: tempFilePath,
          destPath: localPath,
          success: () => {
            fs.readFile({
              filePath: localPath,
              encoding: 'utf-8',
              success: (res) => {
                // 读完成功后立即清理本地副本，避免占用 USER_DATA_PATH 配额
                fs.unlink({ filePath: localPath, fail: () => {} });
                resolve(typeof res.data === 'string' ? res.data : '');
              },
              fail: (err3) => {
                fs.unlink({ filePath: localPath, fail: () => {} });
                reject(buildReadError(tempFilePath, [err1, err3]));
              },
            });
          },
          fail: (err2) => {
            // 第 3 级：saveFile 兜底（部分客户端 saveFile 比 copyFile 宽容）
            fs.saveFile({
              tempFilePath: tempFilePath,
              success: (saveRes) => {
                fs.readFile({
                  filePath: saveRes.savedFilePath,
                  encoding: 'utf-8',
                  success: (res) => {
                    fs.unlink({ filePath: saveRes.savedFilePath, fail: () => {} });
                    resolve(typeof res.data === 'string' ? res.data : '');
                  },
                  fail: (err4) => {
                    fs.unlink({ filePath: saveRes.savedFilePath, fail: () => {} });
                    reject(buildReadError(tempFilePath, [err1, err2, err4]));
                  },
                });
              },
              fail: (err5) => {
                reject(buildReadError(tempFilePath, [err1, err2, err5]));
              },
            });
          },
        });
      },
    });
  });
}

// 构造友好错误信息（含原始 path + 客户端建议）
function buildReadError(tempFilePath, errs) {
  const reasons = errs.map((e) => e.errMsg || e.message || String(e)).filter(Boolean).join(' | ');
  const err = new Error(
    '文件读取失败（path=' + tempFilePath + '）：' + reasons +
    '。建议：1) 用手机微信扫码预览后从聊天选文件；2) 把 .prb 转发到「文件传输助手」后重试；3) PC 微信客户端兼容性较差，请优先用手机端。'
  );
  return err;
}

// 从 wx.chooseMessageFile 的 tempFilePath 读取文本并拆成行
function readPrbFile(tempFilePath, originalName) {
  return readTextWithFallback(tempFilePath, originalName).then((text) => text.split(/\r?\n/));
}

// 读取文件文本（用于 .csv / .txt 校准文件）
function readTextFile(tempFilePath, originalName) {
  return readTextWithFallback(tempFilePath, originalName);
}

// 选择多个 .prb 文件，返回 [{ filePath, name, lines }]
function choosePrbFiles() {
  return new Promise((resolve, reject) => {
    wx.chooseMessageFile({
      count: 10,
      type: 'file',
      extension: ['prb', 'txt'],
      success: async (res) => {
        try {
          const out = [];
          for (const f of res.tempFiles) {
            const lines = await readPrbFile(f.path, f.name);
            out.push({ filePath: f.path, name: f.name, lines });
          }
          resolve(out);
        } catch (e) {
          reject(e);
        }
      },
      fail: (err) => reject(err),
    });
  });
}

// 统一校准文件选择：同时允许 .prb 与 .csv（对齐桌面端「加载 PRB/CSV 文件」）。
// 返回 [{ name, ext:'prb'|'csv', lines? , text? }]，调用方按 ext 分流。
function chooseCalibrationFiles() {
  return new Promise((resolve, reject) => {
    wx.chooseMessageFile({
      count: 10,
      type: 'file',
      extension: ['prb', 'csv', 'txt'],
      success: async (res) => {
        try {
          const out = [];
          for (const f of res.tempFiles) {
            const name = f.name;
            const ext = (name.split('.').pop() || '').toLowerCase();
            if (ext === 'prb') {
              const lines = await readPrbFile(f.path, f.name);
              out.push({ name, ext: 'prb', lines });
            } else {
              const text = await readTextFile(f.path, f.name);
              out.push({ name, ext: 'csv', text });
            }
          }
          resolve(out);
        } catch (e) {
          reject(e);
        }
      },
      fail: (err) => reject(err),
    });
  });
}

module.exports = { choosePrbFiles, readPrbFile, readTextFile, chooseCalibrationFiles, readTextWithFallback };
