// 批量 CSV 解析与插值逻辑（纯 JS，不依赖 wx，可在 Node 校验端复用）。
//
// 设计目标：
//  - parseCsv/toCsv：RFC4180 风格（支持引号、"" 转义、逗号/换行在引号内、BOM 剥离）。
//  - detectColumns：容错匹配输入列（P1..Pn、Patm、TAtm，支持括号单位/中文/空格）。
//  - runBatch：逐行构建 input 调用 calculateRow(interp, input)，收集统一表头的结果行。
//
// 输出表头约定（顺序固定）：
//   默认模式（5/7 孔）：P1..Pn, Patm, TAtm, <resultColumns 各列>, isValid, warning
//     - isValid = 1/0，warning 为该行警告文本；解析/计算失败行 isValid=ERROR。
//   statusColumn 模式（3 孔，对齐桌面端 0.2.1「参考」语义）：
//     P1..Pn, Patm, TAtm, <resultColumns 各列>, 状态
//     - 状态 = 「参考」|「参考: 原因」|「计算失败: 原因」；解析/计算异常行状态=ERROR。
//     - calculateRow 返回的 result 须含 calculated/isValid/warning 三字段。

// ---------- 通用数值解析 ----------
// 解析单个数值输入：空串/空白/非数字 -> NaN（区别于合法的 0）。
// 用于防止空孔压被 Number('') 静默转成 0 Pa（见代码审查 #1）。
function parseNumber(s) {
  if (s == null) return NaN;
  const t = String(s).trim();
  if (t === '') return NaN;
  const v = Number(t);
  return isFinite(v) ? v : NaN;
}

// ---------- 基础 CSV 解析 ----------

function parseCsv(text) {
  if (typeof text !== 'string') return { header: [], rows: [] };
  if (text.charCodeAt(0) === 0xfeff) text = text.slice(1); // 去 UTF-8 BOM
  const records = [];
  let cur = [];
  let field = '';
  let inQuotes = false;
  const n = text.length;
  let i = 0;
  while (i < n) {
    const ch = text[i];
    if (inQuotes) {
      if (ch === '"') {
        if (text[i + 1] === '"') { field += '"'; i += 2; continue; }
        inQuotes = false; i++; continue;
      }
      field += ch; i++; continue;
    }
    if (ch === '"') { inQuotes = true; i++; continue; }
    if (ch === ',') { cur.push(field); field = ''; i++; continue; }
    if (ch === '\r') {
      if (text[i + 1] === '\n') i++;
      cur.push(field); field = '';
      records.push(cur); cur = [];
      i++; continue;
    }
    if (ch === '\n') {
      cur.push(field); field = '';
      records.push(cur); cur = [];
      i++; continue;
    }
    field += ch; i++; continue;
  }
  // 收尾：最后一个字段可能非空，或整行仅一个空字段（文件以换行结尾）
  if (field.length > 0 || cur.length > 0) {
    cur.push(field);
    records.push(cur);
  }
  // 去掉结尾的空记录
  if (records.length && records[records.length - 1].length === 1 && records[records.length - 1][0] === '') {
    records.pop();
  }
  if (records.length === 0) return { header: [], rows: [] };
  const header = records[0];
  const rows = records.slice(1).filter((r) => !(r.length === 1 && r[0].trim() === ''));
  return { header, rows };
}

function csvCell(v) {
  const s = (v === undefined || v === null) ? '' : String(v);
  if (/[",\r\n]/.test(s)) return '"' + s.replace(/"/g, '""') + '"';
  return s;
}

function toCsv(header, rows) {
  const lines = [];
  lines.push(header.map(csvCell).join(','));
  for (const r of rows) lines.push(r.map(csvCell).join(','));
  return lines.join('\r\n');
}

// ---------- 列匹配 ----------

// 把表头单元归一化为可识别的「canonical key」。
function canonHeader(h) {
  if (h == null) return '';
  let s = String(h).trim();
  // 去掉包裹的引号
  if (s.length >= 2 && s[0] === '"' && s[s.length - 1] === '"') s = s.slice(1, -1);
  // 去掉括号内的单位/说明，如 (Pa) (kPa) （度） 等
  s = s.replace(/[（(][^（）()]*[）)]/g, '');
  s = s.toLowerCase();
  // 只保留字母、数字、中文，去掉空格/下划线/标点
  s = s.replace(/[^a-z0-9一-龥]/g, '');
  return s;
}

function detectColumns(header, holeCount) {
  const pCols = new Array(holeCount).fill(-1);
  let patmIdx = -1;
  let tatmIdx = -1;
  header.forEach((h, i) => {
    const c = canonHeader(h);
    const m = c.match(/^p(\d+)$/);
    if (m) {
      const num = parseInt(m[1], 10);
      if (num >= 1 && num <= holeCount) pCols[num - 1] = i;
      return;
    }
    if (c === 'patm' || c.indexOf('大气压') >= 0 || c.indexOf('环境压') >= 0) {
      patmIdx = i; return;
    }
    if (c === 'tatm' || c.indexOf('气温') >= 0 || c.indexOf('温度') >= 0 || c.indexOf('环境温') >= 0) {
      tatmIdx = i; return;
    }
  });
  return { pCols, patmIdx, tatmIdx };
}

// ---------- 批量运行 ----------

// opts:
//   holeCount: 3 | 5 | 7
//   defaults: { PAtm, TAtm }  —— CSV 缺 Patm/TAtm 时回退值
//   interp: 已加载的插值器实例
//   calculateRow(interp, input): 返回 result 对象（含 isValid/warning[/calculated]）；可抛出异常
//   resultColumns: string[] —— 结果列顺序（如 ['alpha','machNumber',...]）
//   formatResultValue(col, raw): 可选，把 result 某列基准值格式化为输出文本（含单位）。
//        默认 (col, raw) => String(raw)。用于按当前显示单位换算。
//   resultColumnHeader(col): 可选，返回该结果列的表头名（如带单位后缀 totalPressure_kPa）。
//        默认 col => col。
//   statusColumn: 可选 bool，true 时启用「状态」文本列（3 孔对齐桌面端 0.2.1「参考」语义），
//        替代默认的 isValid(1/0)+warning 两列。calculateRow 返回的 result 须含 calculated 字段。
function runBatch(text, opts) {
  const { holeCount, defaults, interp, calculateRow, resultColumns } = opts;
  const formatResultValue = opts.formatResultValue || ((col, raw) => (raw === undefined || raw === null ? '' : String(raw)));
  const resultColumnHeader = opts.resultColumnHeader || ((col) => col);
  const useStatusColumn = !!opts.statusColumn;
  const parsed = parseCsv(text);
  if (!parsed.header.length) {
    return { header: [], rows: [], summary: { total: 0, ok: 0, invalid: 0, errors: 0 }, missing: [], note: '空文件或无法解析表头' };
  }
  const mapping = detectColumns(parsed.header, holeCount);
  const missing = [];
  for (let k = 1; k <= holeCount; k++) if (mapping.pCols[k - 1] < 0) missing.push('P' + k);

  const inCols = [];
  for (let k = 1; k <= holeCount; k++) inCols.push('P' + k);
  // 输入列表头：调用方可传 inputHeaderMap 自定义中文标签；默认回退到原 key（保持旧路径兼容）
  const inputHeaderMap = (opts.inputHeaderMap || {});
  const inColLabels = inCols.map((k) => inputHeaderMap[k] || k);
  const patmLabel = inputHeaderMap['Patm'] || 'Patm';
  const tatmLabel = inputHeaderMap['TAtm'] || 'TAtm';
  // 表头尾部：默认 isValid+warning 两列；statusColumn 模式下合并为单「状态」列。
  const tailHeader = useStatusColumn ? ['状态'] : ['isValid', 'warning'];
  const header = inColLabels.concat([patmLabel, tatmLabel]).concat(resultColumns.map(resultColumnHeader)).concat(tailHeader);
  // 输入列格式化：调用方可传 opts.inputFormat(key, value) 自定义精度；默认 String 保持兼容
  // 这样 3/5/7 孔可统一传固定精度函数，让 CSV 表格数值列视觉对齐
  const inputFormat = opts.inputFormat || ((k, v) => (v === undefined || v === null) ? '' : String(v));
  const fmt = (k, v) => inputFormat(k, v);

  // 把 result 的 calculated/isValid/warning 三字段归一为「状态」文本（对齐桌面端 0.2.1）。
  // - calculated=false → "计算失败: 原因"
  // - calculated=true & isValid=true → "参考"
  // - calculated=true & isValid=false → "参考: 原因"
  // 5/7 孔旧路径无 calculated 字段时回退到旧语义（isValid 决定参考/参考-超范围）。
  function statusTextOf(result) {
    const warn = (result.warning || '').trim();
    if (result.calculated === false) return '计算失败: ' + (warn || '未知原因');
    // calculated === true 或 undefined（5/7 孔旧路径）
    return result.isValid ? '参考' : '参考: ' + (warn || '超出校准范围');
  }

  const rows = [];
  let ok = 0, invalid = 0, errors = 0, total = 0;

  for (const raw of parsed.rows) {
    total++;
    const input = { PAtm: defaults.PAtm, TAtm: defaults.TAtm };
    let bad = false;
    let badMsg = '';
    for (let k = 1; k <= holeCount; k++) {
      const idx = mapping.pCols[k - 1];
      const cell = idx >= 0 ? String(raw[idx] == null ? '' : raw[idx]).trim() : '';
      const v = cell === '' ? NaN : Number(cell);
      if (!isFinite(v)) { bad = true; badMsg = '缺列或非法数值: P' + k; break; }
      input['P' + k] = v;
    }
    if (!bad) {
      if (mapping.patmIdx >= 0) {
        const c = String(raw[mapping.patmIdx] == null ? '' : raw[mapping.patmIdx]).trim();
        if (c !== '') {
          if (isFinite(Number(c))) input.PAtm = Number(c);
          else { bad = true; badMsg = 'Patm 非法数值: ' + c; }
        }
      }
      if (!bad && mapping.tatmIdx >= 0) {
        const c = String(raw[mapping.tatmIdx] == null ? '' : raw[mapping.tatmIdx]).trim();
        if (c !== '') {
          if (isFinite(Number(c))) input.TAtm = Number(c);
          else { bad = true; badMsg = 'TAtm 非法数值: ' + c; }
        }
      }
    }

    if (bad) {
      errors++;
      const tail = useStatusColumn ? ['ERROR'] : ['ERROR', badMsg];
      const out = inCols.map(() => '').concat([fmt('Patm', input.PAtm), fmt('TAtm', input.TAtm)])
        .concat(resultColumns.map(() => '')).concat(tail);
      rows.push(out);
      continue;
    }

    let result = null;
    let errMsg = null;
    try {
      result = calculateRow(interp, input);
    } catch (e) {
      errMsg = (e && e.message) ? e.message : String(e);
    }
    if (errMsg) {
      errors++;
      const tail = useStatusColumn ? ['ERROR'] : ['ERROR', errMsg];
      const out = inCols.map((k) => fmt(k, input[k]))
        .concat([fmt('Patm', input.PAtm), fmt('TAtm', input.TAtm)])
        .concat(resultColumns.map(() => '')).concat(tail);
      rows.push(out);
      continue;
    }

    if (result.isValid) ok++; else invalid++;
    const resVals = resultColumns.map((k) => {
      const raw = result[k];
      if (raw === undefined || raw === null || raw === '') return '';
      return formatResultValue(k, raw);
    });
    // statusColumn 模式：result.calculated=false 时数值列应已置空（由调用方 calculateRow 保证），
    // 这里仅按「状态」文本输出；非 statusColumn 模式保留 isValid(1/0)+warning 两列旧格式。
    const tail = useStatusColumn
      ? [statusTextOf(result)]
      : [result.isValid ? '1' : '0', result.warning || ''];
    const out = inCols.map((k) => fmt(k, input[k]))
      .concat([fmt('Patm', input.PAtm), fmt('TAtm', input.TAtm)])
      .concat(resVals)
      .concat(tail);
    rows.push(out);
  }

  return {
    header,
    rows,
    summary: { total, ok, invalid, errors },
    missing,
  };
}

module.exports = { parseCsv, toCsv, detectColumns, runBatch, parseNumber };
