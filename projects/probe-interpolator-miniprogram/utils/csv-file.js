// 微信小程序侧 CSV 文件选择 / 导出封装。
//  - chooseCsvText(): 从微信会话选一个 .csv/.txt，读取为 UTF-8 文本。
//  - exportCsvFile(csvText, fileName): 两级导出 CSV（无剪贴板兜底，按用户要求）。
//    1) wx.shareFileMessage —— 分享到会话（手机端主路径，工程师可直接发回聊天或文件传输助手）
//    2) wx.saveFileToDisk —— PC 端存盘（仅 PC 微信支持）
//    3) 都失败 → reject 错误，由调用方提示「导出失败」
//
// 常见失败原因：
//  - PC 微信：shareFileMessage 不弹分享面板 → 走 saveFileToDisk
//  - 开发者工具：shareFileMessage 调起失败 → 走 saveFileToDisk（可用）
//  - 手机微信：shareFileMessage 用户取消 → 走 saveFileToDisk（不可用）→ reject

// chooseCsvText 同步 prb-file.js 的多级 fallback 读取逻辑，避免 PC 微信 readFile 失败
function _readTextWithFallback(tempFilePath, originalName) {
  return new Promise((resolve, reject) => {
    const fs = wx.getFileSystemManager();
    const userDir = wx.env.USER_DATA_PATH;
    const stamp = Date.now().toString(36);
    const rand = Math.random().toString(36).slice(2, 8);
    const localPath = userDir + '/' + stamp + '_' + rand + '_' + (originalName || 'unknown');

    fs.readFile({
      filePath: tempFilePath,
      encoding: 'utf-8',
      success: (res) => resolve(typeof res.data === 'string' ? res.data : ''),
      fail: (err1) => {
        // copyFile 兜底（PC 微信 http://tmpxxx 路径）
        fs.copyFile({
          srcPath: tempFilePath,
          destPath: localPath,
          success: () => {
            fs.readFile({
              filePath: localPath,
              encoding: 'utf-8',
              success: (res) => {
                fs.unlink({ filePath: localPath, fail: () => {} });
                resolve(typeof res.data === 'string' ? res.data : '');
              },
              fail: (err3) => {
                fs.unlink({ filePath: localPath, fail: () => {} });
                reject(err3);
              },
            });
          },
          fail: () => {
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
                  fail: (err5) => {
                    fs.unlink({ filePath: saveRes.savedFilePath, fail: () => {} });
                    reject(err5);
                  },
                });
              },
              fail: (err5) => reject(err5),
            });
          },
        });
      },
    });
  });
}

function chooseCsvText() {
  return new Promise((resolve, reject) => {
    wx.chooseMessageFile({
      count: 1,
      type: 'file',
      extension: ['csv', 'txt'],
      success: async (res) => {
        const f = (res.tempFiles && res.tempFiles[0]) || null;
        if (!f) { reject(new Error('未选择文件')); return; }
        try {
          const text = await _readTextWithFallback(f.path, f.name);
          resolve(text);
        } catch (e) {
          reject(e);
        }
      },
      fail: (e) => reject(e),
    });
  });
}

// 两级导出：shareFileMessage（手机主路径）→ saveFileToDisk（PC 端）→ reject
// 注：按用户要求不再走剪贴板兜底；分享被取消或两个 API 都不可用时直接 reject。
// 用户取消分享（在原生面板点返回）会被识别为 canceled=true，调用方可静默处理，
// 不会被误导成「导出失败」。
function exportCsvFile(csvText, fileName) {
  return new Promise((resolve, reject) => {
    const fs = wx.getFileSystemManager();
    const base = wx.env.USER_DATA_PATH;
    const path = base + '/' + fileName;

    // 把错误统一包成 { errMsg, canceled } 形式，方便调用方区分
    const failErr = (raw, reason, canceled) => {
      const msg = (raw && raw.errMsg) ? raw.errMsg : (reason || '导出失败');
      return { errMsg: msg, path, fileName, canceled: !!canceled };
    };

    // 识别用户取消：errMsg 含 cancel（如 "shareFileMessage:fail cancel"）
    const isCancel = (e) => {
      const msg = (e && e.errMsg) ? String(e.errMsg) : '';
      return msg.indexOf('cancel') >= 0;
    };

    fs.writeFile({
      filePath: path,
      data: csvText,
      encoding: 'utf-8',
      success: () => {
        // 1) 分享到会话（手机端主路径，最通用）
        if (typeof wx.shareFileMessage === 'function') {
          wx.shareFileMessage({
            filePath: path,
            fileName: fileName,
            success: () => resolve({ path, method: 'share' }),
            fail: (e) => {
              // 用户在分享面板点返回 —— 静默 reject（canceled=true）
              if (isCancel(e)) {
                reject(failErr(e, '已取消分享', true));
                return;
              }
              // 其他失败（PC 端不弹面板 / 限频）→ 尝试 PC 存盘兜底
              if (typeof wx.saveFileToDisk === 'function') {
                wx.saveFileToDisk({
                  filePath: path,
                  success: () => resolve({ path, method: 'disk' }),
                  fail: (e2) => reject(failErr(e2, '分享与存盘均失败')),
                });
              } else {
                reject(failErr(e, '分享不可用'));
              }
            },
          });
          return;
        }
        // 2) 无 shareFileMessage：直接尝试 PC 存盘
        if (typeof wx.saveFileToDisk === 'function') {
          wx.saveFileToDisk({
            filePath: path,
            success: () => resolve({ path, method: 'disk' }),
            fail: (e) => reject(failErr(e, '存盘失败')),
          });
          return;
        }
        // 3) 两个 API 都不可用（极少数环境）→ reject
        reject(failErr(null, '当前环境不支持导出'));
      },
      fail: (e) => reject(failErr(e, '本地写入失败')),
    });
  });
}

// 生成带时间戳的文件名，避免覆盖
function batchFileName(probeType) {
  const d = new Date();
  const p = (x) => String(x).padStart(2, '0');
  const ts = '' + d.getFullYear() + p(d.getMonth() + 1) + p(d.getDate()) + '_' + p(d.getHours()) + p(d.getMinutes()) + p(d.getSeconds());
  return 'probe_batch_' + probeType + '_' + ts + '.csv';
}

// 选择多个 .csv/.txt 并读取为 UTF-8 文本，返回 [{ name, text }]。
// count 为最多选择数量（七孔校准需 7 个）。
function chooseCsvFiles(count) {
  return new Promise((resolve, reject) => {
    wx.chooseMessageFile({
      count: count || 7,
      type: 'file',
      extension: ['csv', 'txt'],
      success: async (res) => {
        try {
          const out = [];
          for (const f of res.tempFiles) {
            const text = await _readTextWithFallback(f.path, f.name);
            out.push({ name: f.name, text });
          }
          resolve(out);
        } catch (e) {
          reject(e);
        }
      },
      fail: (e) => reject(e),
    });
  });
}

module.exports = { chooseCsvText, chooseCsvFiles, exportCsvFile, batchFileName };
