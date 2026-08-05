// 结果分享卡片 —— 纯数据模型（与 wx 无关，可在 Node 端校验）。
// 真正的 Canvas 绘制在 share-canvas.js（依赖 wx 运行时）。
//
// 状态语义对齐桌面端 0.2.1「参考」体系：
//   - calculated=true & isValid=true  → statusText="参考"，statusKind="ok"（绿色）
//   - calculated=true & isValid=false → statusText="参考: 原因"，statusKind="warn"（琥珀色）
//   - calculated=false                → statusText="计算失败: 原因"，statusKind="error"（红色）
// 5/7 孔旧路径无 calculated 字段时回退到旧 isValid 语义（兼容期保留）。

function pad2(n) {
  return n < 10 ? '0' + n : '' + n;
}

// 生成 "YYYY-MM-DD HH:mm" 时间戳
function nowStamp() {
  const d = new Date();
  return d.getFullYear() + '-' + pad2(d.getMonth() + 1) + '-' + pad2(d.getDate()) +
    ' ' + pad2(d.getHours()) + ':' + pad2(d.getMinutes());
}

// 由 calculated/isValid/warning 三字段归一为 statusText（与页面 statusTextOf 共用语义）。
// calculated === undefined 时回退到 5/7 孔旧路径：仅按 isValid 决定「参考」/「参考: 原因」。
function statusTextOf(calculated, isValid, warning) {
  const warn = (warning || '').trim();
  if (calculated === false) return '计算失败: ' + (warn || '未知原因');
  return isValid ? '参考' : '参考: ' + (warn || '超出校准范围');
}

// 颜色档位：'' | 'ok' | 'warn' | 'error'，供 share-canvas 取色。
function statusKindOf(calculated, isValid) {
  if (calculated === false) return 'error';
  return isValid ? 'ok' : 'warn';
}

// 由页面当前结果构建卡片模型。
// opts:
//   probeType: '三孔' | '五孔' | '七孔'
//   inputs:    string[]  —— 输入摘要行（如 ['P1=.. Pa, P2=.. Pa', 'Patm=.. Pa', 'Tatm=.. °C']）
//   dispRows:  { label, value }[] —— 已按单位格式化的结果展示行
//   calculated: boolean（可选，3 孔提供；5/7 孔省略以走旧兼容路径）
//   isValid:   boolean
//   warning:   string（可选）
//   statusText/statusKind: 可选，若调用方已计算可直接传入覆盖
// 返回 { probeType, inputs, results, statusText, statusKind, warning, generatedAt }
function buildCardModel(opts) {
  const probeType = opts && opts.probeType ? String(opts.probeType) : '';
  const inputs = Array.isArray(opts && opts.inputs) ? opts.inputs.slice() : [];
  const dispRows = Array.isArray(opts && opts.dispRows) ? opts.dispRows : [];
  const results = dispRows.map((r) => ({ label: String(r.label), value: String(r.value) }));
  const calculated = (opts && typeof opts.calculated === 'boolean') ? opts.calculated : undefined;
  const isValid = !!(opts && opts.isValid);
  const warning = (opts && opts.warning) ? String(opts.warning) : '';
  // 优先用调用方传入的 statusText/statusKind（与页面 UI 完全一致），否则现场计算
  const statusText = (opts && opts.statusText)
    ? String(opts.statusText)
    : statusTextOf(calculated, isValid, warning);
  const statusKind = (opts && opts.statusKind)
    ? String(opts.statusKind)
    : statusKindOf(calculated, isValid);
  return {
    probeType,
    inputs,
    results,
    statusText,
    statusKind,
    warning,
    generatedAt: nowStamp(),
  };
}

module.exports = { buildCardModel, nowStamp, statusTextOf, statusKindOf };
