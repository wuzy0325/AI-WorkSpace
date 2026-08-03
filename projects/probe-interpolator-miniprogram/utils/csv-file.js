// 微信小程序侧 CSV 文件选择 / 导出封装。
//  - chooseCsvText(): 从微信会话选一个 .csv/.txt，读取为 UTF-8 文本。
//  - exportCsvFile(csvText, fileName): 写入 USER_DATA_PATH 后分享/保存。
//    优先 wx.shareFileMessage（分享到会话，最通用，工程师可把结果直接发回聊天），
//    回退 wx.saveFileToDisk（PC 端存盘），再回退仅本地写出。

function chooseCsvText() {
  return new Promise((resolve, reject) => {
    wx.chooseMessageFile({
      count: 1,
      type: 'file',
      extension: ['csv', 'txt'],
      success: (res) => {
        const f = (res.tempFiles && res.tempFiles[0]) || null;
        if (!f) { reject(new Error('未选择文件')); return; }
        const fs = wx.getFileSystemManager();
        fs.readFile({
          filePath: f.path,
          encoding: 'utf-8',
          success: (r) => resolve(typeof r.data === 'string' ? r.data : ''),
          fail: (e) => reject(e),
        });
      },
      fail: (e) => reject(e),
    });
  });
}

function exportCsvFile(csvText, fileName) {
  return new Promise((resolve, reject) => {
    const fs = wx.getFileSystemManager();
    // USER_DATA_PATH 下文件名需唯一，避免覆盖
    const base = wx.env.USER_DATA_PATH;
    const path = base + '/' + fileName;
    fs.writeFile({
      filePath: path,
      data: csvText,
      encoding: 'utf-8',
      success: () => {
        // 1) 分享到会话（最通用）
        if (typeof wx.shareFileMessage === 'function') {
          wx.shareFileMessage({
            filePath: path,
            fileName: fileName,
            success: () => resolve({ path, method: 'share' }),
            fail: (e) => {
              // 2) 回退 PC 存盘
              if (typeof wx.saveFileToDisk === 'function') {
                wx.saveFileToDisk({
                  filePath: path,
                  success: () => resolve({ path, method: 'disk' }),
                  fail: (e2) => reject(e2 || e),
                });
              } else {
                reject(e);
              }
            },
          });
          return;
        }
        // 2) 无 shareFileMessage：直接存盘
        if (typeof wx.saveFileToDisk === 'function') {
          wx.saveFileToDisk({
            filePath: path,
            success: () => resolve({ path, method: 'disk' }),
            fail: (e) => reject(e),
          });
          return;
        }
        // 3) 均无：仅本地写出，告知路径
        resolve({ path, method: 'written' });
      },
      fail: (e) => reject(e),
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
            const text = await new Promise((res2, rej2) => {
              const fs = wx.getFileSystemManager();
              fs.readFile({
                filePath: f.path,
                encoding: 'utf-8',
                success: (r) => res2(typeof r.data === 'string' ? r.data : ''),
                fail: (e) => rej2(e),
              });
            });
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
