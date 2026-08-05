// 结果分享卡片 —— Canvas 2D 绘制与导出分享（依赖 wx 运行时）。
// 纯数据模型见 share-card.js（Node 可校验）。本模块无法在 Node 端测试，
// 绘制逻辑力求健壮并带三级回退：wx.shareFileMessage → saveImageToPhotosAlbum → previewImage。
//
// 状态语义对齐桌面端 0.2.1「参考」体系：
//   model.statusKind = 'ok'（绿）| 'warn'（琥珀）| 'error'（红）
//   model.statusText = 「参考」|「参考: 原因」|「计算失败: 原因」
// 旧字段 model.isValid/model.warning 仅作 5/7 孔兼容回退，3 孔已切到 statusText/statusKind。

const CARD_W = 600;        // 逻辑宽度（px）
const PAD = 28;
// 主色与 app.wxss --accent / --accent-ink 一致（科学蓝降饱和）
const ACCENT = '#0f5cad';
const ACCENT_INK = '#0a4a8c';
const ACCENT_BAR_H = 64;
const LINE_GAP = 30;
const TITLE_FS = 22;
const SUB_FS = 13;
const LABEL_FS = 15;
const VAL_FS = 15;
const FOOTER = '探针插值计算器';

// 状态档位 → 颜色（与 app.wxss --ok/--warn/--err 一致，降饱和版）
const STATUS_COLORS = {
  ok: '#15803d',     // 绿（参考-范围内）
  warn: '#a45a00',   // 琥珀（参考-超范围）
  error: '#b91c1c',  // 红（计算失败）
};

function roundRect(ctx, x, y, w, h, r) {
  ctx.beginPath();
  ctx.moveTo(x + r, y);
  ctx.arcTo(x + w, y, x + w, y + h, r);
  ctx.arcTo(x + w, y + h, x, y + h, r);
  ctx.arcTo(x, y + h, x, y, r);
  ctx.arcTo(x, y, x + w, y, r);
  ctx.closePath();
}

// 仅顶部圆角、底部平直的色条路径
function accentBar(ctx, w, h, r) {
  ctx.beginPath();
  ctx.moveTo(0, r);
  ctx.arcTo(0, 0, r, 0, r);
  ctx.arcTo(w, 0, w, r, r);
  ctx.lineTo(w, h);
  ctx.lineTo(0, h);
  ctx.closePath();
}

// 依据内容计算卡片逻辑高度（含充裕底部留白）
function computeHeight(model) {
  let h = ACCENT_BAR_H + PAD;
  if (model.inputs && model.inputs.length) {
    h += LABEL_FS;                       // “输入”标题
    h += model.inputs.length * LINE_GAP;
    h += 10;
  }
  h += PAD;                              // 分隔线空间
  h += LABEL_FS;                         // “结果”标题
  h += model.results.length * LINE_GAP;
  h += 10;
  h += LABEL_FS + LINE_GAP;              // 状态行
  // 兼容旧 model.warning：3 孔已把原因并入 statusText，warning 通常为空；5/7 孔旧路径仍可能携带
  if (model.warning) h += LINE_GAP;
  h += PAD + 20;                         // 底部留白 + 页脚
  return Math.ceil(h);
}

function drawCard(ctx, model, W, H) {
  ctx.clearRect(0, 0, W, H);

  // 卡片白底（圆角）
  ctx.fillStyle = '#ffffff';
  roundRect(ctx, 0, 0, W, H, 14);
  ctx.fill();

  // 顶部色条（仅上圆角）—— 与首页 hero / probe-badge 一致的渐变
  const grad = ctx.createLinearGradient(0, 0, W, ACCENT_BAR_H);
  grad.addColorStop(0, ACCENT);
  grad.addColorStop(1, ACCENT_INK);
  ctx.fillStyle = grad;
  accentBar(ctx, W, ACCENT_BAR_H, 14);
  ctx.fill();

  // 标题（左）
  ctx.fillStyle = '#ffffff';
  ctx.textBaseline = 'middle';
  ctx.textAlign = 'left';
  ctx.font = '600 ' + TITLE_FS + 'px sans-serif';
  ctx.fillText((model.probeType || '探针') + ' 插值结果', PAD, ACCENT_BAR_H / 2);

  // 时间（右）
  ctx.font = SUB_FS + 'px sans-serif';
  ctx.textAlign = 'right';
  ctx.fillText(model.generatedAt, W - PAD, ACCENT_BAR_H / 2);

  let y = ACCENT_BAR_H + PAD;

  // 输入
  if (model.inputs && model.inputs.length) {
    ctx.textAlign = 'left';
    ctx.fillStyle = '#0f172a';
    ctx.font = '600 ' + LABEL_FS + 'px sans-serif';
    ctx.fillText('输入', PAD, y);
    y += LINE_GAP;
    ctx.font = SUB_FS + 'px sans-serif';
    ctx.fillStyle = '#334155';
    for (const line of model.inputs) {
      ctx.fillText(line, PAD, y);
      y += LINE_GAP;
    }
    y += 10;
  }

  // 分隔线
  ctx.strokeStyle = '#e2e8f0';
  ctx.lineWidth = 1;
  ctx.beginPath();
  ctx.moveTo(PAD, y);
  ctx.lineTo(W - PAD, y);
  ctx.stroke();
  y += PAD;

  // 结果
  ctx.textAlign = 'left';
  ctx.fillStyle = '#0f172a';
  ctx.font = '600 ' + LABEL_FS + 'px sans-serif';
  ctx.fillText('结果', PAD, y);
  y += LINE_GAP;
  for (const r of model.results) {
    ctx.font = SUB_FS + 'px sans-serif';
    ctx.fillStyle = '#475569';
    ctx.textAlign = 'left';
    ctx.fillText(r.label, PAD, y);
    ctx.fillStyle = '#0f172a';
    ctx.font = '600 ' + VAL_FS + 'px sans-serif';
    ctx.textAlign = 'right';
    ctx.fillText(r.value, W - PAD, y);
    y += LINE_GAP;
  }
  y += 10;

  // 状态行（「参考」/「参考: 原因」/「计算失败: 原因」），按 statusKind 取色
  // 5/7 孔旧路径若未提供 statusText，则回退到旧「有效/无效」语义以保兼容。
  const statusText = (model.statusText !== undefined && model.statusText !== '')
    ? String(model.statusText)
    : (model.isValid ? '有效' : '无效');
  const statusKind = model.statusKind || (model.isValid ? 'ok' : 'error');
  ctx.textAlign = 'left';
  ctx.font = '600 ' + LABEL_FS + 'px sans-serif';
  ctx.fillStyle = STATUS_COLORS[statusKind] || STATUS_COLORS.error;
  ctx.fillText(statusText, PAD, y);
  y += LINE_GAP;

  // 兼容旧 model.warning（3 孔已并入 statusText，通常为空；5/7 孔旧路径仍可能携带独立警告）
  if (model.warning) {
    ctx.font = SUB_FS + 'px sans-serif';
    ctx.fillStyle = STATUS_COLORS.error;
    const w = model.warning.length > 64 ? model.warning.slice(0, 61) + '...' : model.warning;
    ctx.fillText('⚠ ' + w, PAD, y);
    y += LINE_GAP;
  }

  // 页脚
  ctx.font = SUB_FS + 'px sans-serif';
  ctx.fillStyle = '#94a3b8';
  ctx.fillText(FOOTER, PAD, H - PAD / 2);
}

// ---- wx 运行时部分 ----

function getCanvasNode(canvasId) {
  return new Promise((resolve, reject) => {
    wx.createSelectorQuery().select('#' + canvasId).fields({ node: true, size: true }).exec((res) => {
      if (!res || !res[0] || !res[0].node) {
        reject(new Error('未找到 canvas 节点（需在真机/开发者工具中渲染）'));
        return;
      }
      resolve(res[0].node);
    });
  });
}

function exportToTemp(canvas) {
  return new Promise((resolve, reject) => {
    wx.canvasToTempFilePath({
      canvas,
      success: (r) => resolve(r.tempFilePath),
      fail: (e) => reject(e),
    });
  });
}

function shareFile(filePath) {
  return new Promise((resolve, reject) => {
    if (typeof wx.shareFileMessage !== 'function') return reject(new Error('环境不支持 shareFileMessage'));
    wx.shareFileMessage({
      filePath,
      fileName: 'probe_result.png',
      success: () => resolve('share'),
      fail: (e) => reject(e),
    });
  });
}

function saveAlbum(filePath) {
  return new Promise((resolve, reject) => {
    wx.saveImageToPhotosAlbum({
      filePath,
      success: () => resolve('album'),
      fail: (e) => reject(e),
    });
  });
}

function preview(filePath) {
  return new Promise((resolve) => {
    wx.previewImage({ urls: [filePath], current: filePath, complete: () => resolve('preview') });
  });
}

// 主入口：构建临时图并三级回退分享。
// 返回 Promise<{ method: 'share'|'album'|'preview', filePath }>
async function shareCardImage(model, canvasId) {
  const canvas = await getCanvasNode(canvasId);
  const ctx = canvas.getContext('2d');
  const info = (typeof wx.getWindowInfo === 'function') ? wx.getWindowInfo() : wx.getSystemInfoSync();
  const dpr = (info && info.pixelRatio) || 2;
  const W = CARD_W;
  const H = computeHeight(model);
  canvas.width = Math.round(W * dpr);
  canvas.height = Math.round(H * dpr);
  ctx.scale(dpr, dpr);
  drawCard(ctx, model, W, H);
  const filePath = await exportToTemp(canvas);

  try {
    const m = await shareFile(filePath);
    return { method: m, filePath };
  } catch (e) {
    try {
      const m = await saveAlbum(filePath);
      return { method: m, filePath };
    } catch (e2) {
      const m = await preview(filePath);
      return { method: m, filePath };
    }
  }
}

module.exports = { shareCardImage, drawCard, computeHeight };
