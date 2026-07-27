// 小程序侧 .prb 文件选择封装
// 微信会话内选择文件（wx.chooseMessageFile），读取文本后按行拆分。
// 注意：微信小程序不支持直接读任意本地路径，需用户从聊天里发送 .prb 文件后在此选择。

// 从 wx.chooseMessageFile 的 tempFilePath 读取文本并拆成行
function readPrbFile(tempFilePath) {
  return new Promise((resolve, reject) => {
    const fs = wx.getFileSystemManager();
    fs.readFile({
      filePath: tempFilePath,
      encoding: 'utf-8',
      success: (res) => {
        const text = typeof res.data === 'string' ? res.data : '';
        const lines = text.split(/\r?\n/);
        resolve(lines);
      },
      fail: (err) => reject(err),
    });
  });
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
            const lines = await readPrbFile(f.path);
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

// 读取文件文本（用于 .csv / .txt 校准文件）
function readTextFile(tempFilePath) {
  return new Promise((resolve, reject) => {
    const fs = wx.getFileSystemManager();
    fs.readFile({
      filePath: tempFilePath,
      encoding: 'utf-8',
      success: (res) => resolve(typeof res.data === 'string' ? res.data : ''),
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
              const lines = await readPrbFile(f.path);
              out.push({ name, ext: 'prb', lines });
            } else {
              const text = await readTextFile(f.path);
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

module.exports = { choosePrbFiles, readPrbFile, chooseCalibrationFiles };
