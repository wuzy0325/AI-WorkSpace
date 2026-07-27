// 结果分享卡片 —— 纯数据模型（与 wx 无关，可在 Node 端校验）。
// 真正的 Canvas 绘制在 share-canvas.js（依赖 wx 运行时）。

function pad2(n) {
  return n < 10 ? '0' + n : '' + n;
}

// 生成 "YYYY-MM-DD HH:mm" 时间戳
function nowStamp() {
  const d = new Date();
  return d.getFullYear() + '-' + pad2(d.getMonth() + 1) + '-' + pad2(d.getDate()) +
    ' ' + pad2(d.getHours()) + ':' + pad2(d.getMinutes());
}

// 由页面当前结果构建卡片模型。
// opts:
//   probeType: '三孔' | '五孔' | '七孔'
//   inputs:    string[]  —— 输入摘要行（如 ['P1=.. Pa, P2=.. Pa', 'Patm=.. Pa', 'Tatm=.. °C']）
//   dispRows:  { label, value }[] —— 已按单位格式化的结果展示行
//   isValid:  boolean
//   warning:   string（可选）
// 返回 { probeType, inputs, results, isValid, warning, generatedAt }
function buildCardModel(opts) {
  const probeType = opts && opts.probeType ? String(opts.probeType) : '';
  const inputs = Array.isArray(opts && opts.inputs) ? opts.inputs.slice() : [];
  const dispRows = Array.isArray(opts && opts.dispRows) ? opts.dispRows : [];
  const results = dispRows.map((r) => ({ label: String(r.label), value: String(r.value) }));
  const isValid = !!(opts && opts.isValid);
  const warning = (opts && opts.warning) ? String(opts.warning) : '';
  return {
    probeType,
    inputs,
    results,
    isValid,
    warning,
    generatedAt: nowStamp(),
  };
}

module.exports = { buildCardModel, nowStamp };
