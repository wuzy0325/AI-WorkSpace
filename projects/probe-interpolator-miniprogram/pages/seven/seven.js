const { SevenHolePrbInterpolator } = require('../../utils/algorithms/index.js');
const { chooseCalibrationFiles } = require('../../utils/prb-file.js');
const { runBatch, toCsv, parseNumber } = require('../../utils/csv-batch.js');
const { chooseCsvText, exportCsvFile, batchFileName } = require('../../utils/csv-file.js');
const { loadCalibrationCSV } = require('../../utils/algorithms/sevenhole/csv-loader.js');
const { formatValue, convertNumber, unitLabelShort, OPTIONS } = require('../../utils/units.js');
const { buildCardModel } = require('../../utils/share-card.js');
const { shareCardImage } = require('../../utils/share-canvas.js');

const PROBE_TYPE = '七孔';

// 七孔结果列（顺序即 CSV 输出列顺序）
const RESULT_COLUMNS = ['alpha', 'beta', 'theta', 'phi', 'machNumber', 'velocity', 'totalPressure', 'staticPressure', 'dynamicPressure'];
// 每列的物理量类型（供单位换算与表头后缀）
const COLUMN_UNIT = {
  alpha: 'angle', beta: 'angle', theta: 'angle', phi: 'angle', machNumber: 'none',
  velocity: 'velocity', totalPressure: 'pressure', staticPressure: 'pressure', dynamicPressure: 'pressure',
};
// CSV 表格各列固定精度（小数位）—— 列宽统一 + 右对齐 + 固定精度 = 视觉对齐
const COLUMN_DECIMALS = {
  alpha: 3, beta: 3, theta: 3, phi: 3, machNumber: 4,
  velocity: 2, totalPressure: 2, staticPressure: 2, dynamicPressure: 2,
};
// CSV 表头中文标签（替代英文 key + 后缀，提升可读性）
const COLUMN_LABEL = {
  alpha: '侧滑角α', beta: '迎角β', theta: 'θ俯仰', phi: 'φ方位',
  machNumber: '马赫数', velocity: '速度V',
  totalPressure: '总压Pt', staticPressure: '静压Ps', dynamicPressure: '动压',
};
// 单点结果展示定义（label + 单位类型）
const FIELD_DEFS = [
  { key: 'alpha', label: '侧滑角 α', unitType: 'angle' },
  { key: 'beta', label: '迎角 β', unitType: 'angle' },
  { key: 'theta', label: 'θ 网格俯仰', unitType: 'angle' },
  { key: 'phi', label: 'φ 网格方位', unitType: 'angle' },
  { key: 'machNumber', label: '马赫数 Ma', unitType: 'none' },
  { key: 'velocity', label: '速度 V', unitType: 'velocity' },
  { key: 'totalPressure', label: '总压 Pt', unitType: 'pressure' },
  { key: 'staticPressure', label: '静压 Ps', unitType: 'pressure' },
  { key: 'dynamicPressure', label: '动压', unitType: 'pressure' },
];

// 把算法基准结果按当前显示单位整理成展示行
function buildDispRows(result, fieldDefs, units) {
  return fieldDefs.map((fd) => ({
    label: fd.label,
    value: formatValue(fd.unitType, units[fd.unitType], result[fd.key]),
  }));
}

// 七孔校准 CSV 路由：从 7 个已选文件中识别「1 个内区 + 6 个外区扇区」。
// 优先按 basename 识别（inner/内区/7 → 内区；1~6 → 对应扇区），识别不全则回退顺序
// （首文件=内区，其后 6 个依序=扇区 1~6）。返回 { innerIdx, inner, outer:[6] }。
function basenameOf(n) {
  return (n || '').split(/[\\/]/).pop().toLowerCase().replace(/\.(csv|txt)$/i, '');
}
function routeCalibrationCsv(chosen) {
  let innerIdx = chosen.findIndex((c) => /inner|内区|^7$/.test(basenameOf(c.name)));
  if (innerIdx < 0) innerIdx = 0; // 回退：首文件为内区
  const rest = chosen.filter((_, i) => i !== innerIdx);
  const mapped = new Array(6).fill(null);
  for (const c of rest) {
    const m = basenameOf(c.name).match(/([1-6])/);
    if (m) mapped[parseInt(m[1], 10) - 1] = c;
  }
  let pos = 0;
  for (let s = 0; s < 6; s++) if (!mapped[s]) mapped[s] = rest[pos++];
  return {
    innerIdx,
    inner: { name: chosen[innerIdx].name, text: chosen[innerIdx].text },
    outer: mapped.map((c) => ({ name: c.name, text: c.text })),
  };
}

// 七孔探针工作区。
// 校准：从微信会话选 7 个 .prb（7.prb 内区 + 1~6.prb 外区扇区），按 basename 路由后加载。
// 计算：输入 7 个孔压（P1..P6 外围环，P7 中心，Pa 表压）+ 大气，输出 α/β/θ/φ/Ma/V/总静压。
// 语义：α=侧滑角，β=迎角（与五孔相反）；大小角度模式自动判定。
Page({
  data: {
    p1: '', p2: '', p3: '', p4: '', p5: '', p6: '', p7: '',
    patm: '101325', tatm: '20',
    files: [],          // [{ name, lines }]
    innerName: '',
    outerNames: ['', '', '', '', '', ''],
    validRange: null,
    result: null,
    dispRows: [],       // 按单位格式化后的展示行
    status: '请加载校准文件（7 个 .prb 或 7 份校准 .csv）',
    statusType: '',     // '' | 'ok' | 'error'
    units: { pressure: 'Pa', velocity: 'm/s', angle: 'deg', temp: '°C' },
    pressureOptions: OPTIONS.pressure, angleOptions: OPTIONS.angle,
    tempOptions: OPTIONS.temp, velocityOptions: OPTIONS.velocity,
    pressureIndex: 0, angleIndex: 0, tempIndex: 0, velocityIndex: 0,
    // 批量 CSV
    batchHeader: [],
    batchRows: [],
    batchPreview: [],
    batchSummary: null,
    batchStatus: '',
    batchStatusType: '',
    // 数据输入方式 tab：'manual' | 'csv'，两种并列，计算按钮只在 manual tab 内
    activeInputTab: 'manual',
  },

  // 跳转使用说明页并预选七孔 tab
  goHelp() {
    wx.navigateTo({ url: '/pages/help/help?tab=seven' });
  },

  onLoad() {
    const app = getApp();
    const u = app.globalData.units || this.data.units;
    this.setData({
      units: Object.assign({}, u),
      pressureIndex: OPTIONS.pressure.indexOf(u.pressure),
      angleIndex: OPTIONS.angle.indexOf(u.angle),
      tempIndex: OPTIONS.temp.indexOf(u.temp),
      velocityIndex: OPTIONS.velocity.indexOf(u.velocity),
    });
  },

  onInput(e) {
    const field = e.currentTarget.dataset.field;
    this.setData({ [field]: e.detail.value });
  },

  // 切换数据输入方式 tab：manual（手动输入） / csv（CSV 批量）
  onSwitchTab(e) {
    const tab = e.currentTarget.dataset.tab;
    if (tab && tab !== this.data.activeInputTab) {
      this.setData({ activeInputTab: tab });
    }
  },

  // 统一校准入口：一次选择既可选 .prb 也可选 .csv（对齐桌面端「加载 PRB/CSV 文件」）
  async onLoadCalibration() {
    try {
      const chosen = await chooseCalibrationFiles();
      if (!chosen.length) return; // 用户取消
      const prb = chosen.filter((c) => c.ext === 'prb');
      const csv = chosen.filter((c) => c.ext !== 'prb');
      if (prb.length && csv.length) {
        this.setData({ status: '请勿混用 .prb 与 .csv，请只选一种校准文件', statusType: 'error' });
        return;
      }
      if (csv.length) {
        if (csv.length !== 7) {
          this.setData({ status: '校准 CSV 需选 7 个文件（1 内区 + 6 外区扇区），实际 ' + csv.length + ' 个', statusType: 'error' });
          return;
        }
        const route = routeCalibrationCsv(csv);
        const { interpolator, warnings } = loadCalibrationCSV(route.inner.text, route.outer.map((o) => o.text));
        this._interp = interpolator;
        const vr = interpolator.getValidRange();
        const outerNames = route.outer.map((o) => o.name);
        const warnMsg = warnings.length ? '；' + warnings.join('；') : '';
        this.setData({
          files: csv.map((c) => ({ name: c.name })),
          innerName: route.inner.name,
          outerNames,
          validRange: vr,
          result: null,
          dispRows: [],
          status: '校准 CSV 加载完成（内区 169 + 6 扇区）' + warnMsg,
          statusType: 'ok',
        });
        return;
      }
      // .prb 分支：按 basename 路由（7.prb 内区 + 1~6.prb 外区扇区）
      const files = prb.map((c) => ({ name: c.name, lines: c.lines }));
      this.setData({ files });
      this._loadCalibration(files);
    } catch (err) {
      const msg = (err && err.errMsg) ? err.errMsg : (err && err.message) ? err.message : String(err);
      if (msg.indexOf('cancel') >= 0) return;
      this.setData({ status: '加载失败: ' + msg, statusType: 'error' });
    }
  },

  clearFiles() {
    this._interp = null;
    this._lastCsvText = null;
    this.setData({
      files: [], innerName: '', outerNames: ['', '', '', '', '', ''],
      validRange: null, result: null, dispRows: [], status: '已清空校准文件',
    });
  },

  _loadCalibration(files) {
    if (files.length === 0) {
      this.setData({ status: '请先选择 .prb 文件', statusType: 'error' });
      return;
    }
    // 按 basename 路由：7.prb → 内区；1..6.prb → 外区扇区 n。
    const byName = {};
    for (const f of files) {
      const base = (f.name || '').split(/[\\/]/).pop().toLowerCase().replace(/\.prb$/i, '');
      byName[base] = f;
    }
    const interp = new SevenHolePrbInterpolator();
    try {
      const inner = byName['7'];
      if (!inner) throw new Error('未找到 7.prb（内区文件）');
      interp.loadInnerPrbLines(inner.lines, inner.name);
      const outerNames = ['', '', '', '', '', ''];
      for (let s = 1; s <= 6; s++) {
        const f = byName[String(s)];
        if (!f) throw new Error('未找到 ' + s + '.prb（外区扇区 ' + s + '）');
        interp.loadOuterPrbLines(s, f.lines, f.name);
        outerNames[s - 1] = f.name;
      }
      const vr = interp.getValidRange();
      this._interp = interp;
      this.setData({
        innerName: inner.name,
        outerNames,
        validRange: vr,
        result: null,
        dispRows: [],
        status: '校准加载完成（内区 169 + 6 扇区）',
        statusType: 'ok',
      });
    } catch (e) {
      this._interp = null;
      this.setData({ status: '加载失败: ' + e.message, statusType: 'error', validRange: null });
    }
  },

  onCalculate() {
    const d = this.data;
    const interp = this._interp;
    if (!interp || !interp.isLoaded()) {
      this.setData({ status: '请先加载校准（7 个 .prb 或 7 份校准 CSV）', statusType: 'error' });
      return;
    }
    const ph = [d.p1, d.p2, d.p3, d.p4, d.p5, d.p6, d.p7].map(parseNumber);
    if (!ph.every(isFinite)) {
      this.setData({ status: '请填写完整的 7 个孔压（Pa）', statusType: 'error' });
      return;
    }
    const patm = parseNumber(d.patm), tatm = parseNumber(d.tatm);
    if (!isFinite(patm) || !isFinite(tatm)) {
      this.setData({ status: '请填写大气压 Patm（Pa）与气温 TAtm（°C）', statusType: 'error' });
      return;
    }
    const input = { P1: ph[0], P2: ph[1], P3: ph[2], P4: ph[3], P5: ph[4], P6: ph[5], P7: ph[6], PAtm: patm, TAtm: tatm };
    try {
      const result = interp.calculate(input);
      const warn = result.warning ? [result.warning] : [];
      // 5/7 孔算法无 calculated 字段，回退到 isValid 决定「参考」语义：
      //   isValid=true → ok（参考）；isValid=false & 有 warning → warn（参考: 原因）；其余 → error（计算失败）
      // 与 share-card 旧路径回退逻辑一致，保证 UI 与分享卡片语义统一。
      const statusKind = result.isValid ? 'ok' : (result.warning ? 'warn' : 'error');
      const statusText = result.isValid ? '参考' : (result.warning ? '参考: ' + result.warning : '计算失败');
      this.setData({
        result,
        dispRows: buildDispRows(result, FIELD_DEFS, d.units),
        status: result.isValid ? (warn.length ? warn.join('；') : '计算完成') : (warn.join('；') || '超出校准网格，不支持外推'),
        statusType: result.isValid ? 'ok' : 'error',
        statusKind,
        statusText,
      });
    } catch (e) {
      this.setData({ status: '计算失败: ' + e.message, statusType: 'error', result: null, dispRows: [], statusKind: '', statusText: '' });
    }
  },

  onCopyResult() {
    const rows = this.data.dispRows || [];
    if (!rows.length) {
      this.setData({ status: '无结果可复制', statusType: 'error' });
      return;
    }
    const lines = rows.map((r) => r.label + '\t' + r.value);
    const text = '七孔探针结果\r\n' + lines.join('\r\n');
    wx.setClipboardData({
      data: text,
      success: () => this.setData({ status: '结果已复制到剪贴板', statusType: 'ok' }),
      fail: (e) => this.setData({ status: '复制失败: ' + (e.errMsg || e), statusType: 'error' }),
    });
  },

  onShareCard() {
    const d = this.data;
    if (!d.result) {
      this.setData({ status: '无结果可分享', statusType: 'error' });
      return;
    }
    const ps = [d.p1, d.p2, d.p3, d.p4, d.p5, d.p6, d.p7];
    const inputs = [
      ps.map((v, i) => 'P' + (i + 1) + '=' + v + ' Pa').join('  '),
      'Patm=' + d.patm + ' Pa',
      'Tatm=' + d.tatm + ' °C',
    ];
    const model = buildCardModel({
      probeType: PROBE_TYPE,
      inputs,
      dispRows: d.dispRows,
      isValid: d.result.isValid,
      warning: d.result.warning,
      // 直接复用页面派生的 statusText/statusKind，避免 share-card 二次派生导致文案不一致
      statusText: d.statusText,
      statusKind: d.statusKind,
    });
    this.setData({ status: '正在生成分享卡片…', statusType: '' });
    shareCardImage(model, 'shareCanvas')
      .then((r) => {
        const how = r.method === 'share' ? '已调起分享' : (r.method === 'album' ? '已保存到相册' : '已打开预览（可长按保存/分享）');
        this.setData({ status: '分享卡片生成成功 → ' + how, statusType: 'ok' });
      })
      .catch((e) => {
        const msg = (e && e.errMsg) ? e.errMsg : (e && e.message) ? e.message : String(e);
        this.setData({ status: '生成分享卡片失败: ' + msg, statusType: 'error' });
      });
  },

  back() {
    wx.navigateBack();
  },

  // 单位切换：更新全局偏好 + 重算单点展示 + 若已导入则按新单位重跑批量
  onUnitChange(e) {
    const kind = e.currentTarget.dataset.unit;
    const idx = e.detail.value;
    const val = OPTIONS[kind][idx];
    const units = Object.assign({}, this.data.units, { [kind]: val });
    const app = getApp();
    if (app.globalData) app.globalData.units = units;
    const idxKey = { pressure: 'pressureIndex', angle: 'angleIndex', temp: 'tempIndex', velocity: 'velocityIndex' }[kind];
    const patch = { units, [idxKey]: idx };
    if (this.data.result) patch.dispRows = buildDispRows(this.data.result, FIELD_DEFS, units);
    this.setData(patch);
    if (this._lastCsvText) this.runBatchWith(this._lastCsvText, units);
  },

  // CSV 表格结果列格式化：换算到当前单位 + 固定精度纯数值（不带单位后缀，单位在表头）
  _fmtVal(units) {
    return (col, raw) => {
      const t = COLUMN_UNIT[col] || 'none';
      if (raw === undefined || raw === null || raw === '') return '';
      if (t === 'none') {
        const n = Number(raw);
        const d = COLUMN_DECIMALS[col] ?? 3;
        return isFinite(n) ? n.toFixed(d) : '';
      }
      const v = convertNumber(t, units[t], Number(raw));
      if (typeof v !== 'number' || !isFinite(v)) return '';
      return v.toFixed(COLUMN_DECIMALS[col] ?? 3);
    };
  },
  // CSV 表格输入列格式化：P1~P7/Patm/TAtm 统一 2 位。
  // 输入列不换算单位（CSV 原始数据就是 Pa / °C），只统一精度；
  // 单位切换只作用于结果列（_fmtVal/_colHdr），units 参数保留仅为签名一致。
  _fmtInput(k, v, units) {
    if (v === undefined || v === null || v === '') return '';
    const n = Number(v);
    if (!isFinite(n)) return String(v);
    return n.toFixed(2);
  },
  _colHdr(units) {
    return (col) => {
      const label = COLUMN_LABEL[col] || col;
      const t = COLUMN_UNIT[col] || 'none';
      if (t === 'none') return label;
      return label + '(' + unitLabelShort(t, units[t]) + ')';
    };
  },

  // 批量运行（含单位换算），供导入与单位切换复用（复用已加载插值器）
  runBatchWith(text, units) {
    const d = this.data;
    const interp = this._interp;
    if (!interp || !interp.isLoaded()) {
      this.setData({ batchStatus: '请先加载校准（7 个 .prb）', batchStatusType: 'error' });
      return;
    }
    let out;
    try {
      out = runBatch(text, {
        holeCount: 7,
        defaults: { PAtm: Number(d.patm), TAtm: Number(d.tatm) },
        interp,
        calculateRow: (it, input) => it.calculate(input),
        resultColumns: RESULT_COLUMNS,
        formatResultValue: this._fmtVal(units),
        resultColumnHeader: this._colHdr(units),
        // 输入列固定精度：P1~P7/Patm 压力 2 位、TAtm 温度 2 位
        inputFormat: (k, v) => this._fmtInput(k, v, units),
        // 输入列表头中文标签
        inputHeaderMap: { P1: 'P1', P2: 'P2', P3: 'P3', P4: 'P4', P5: 'P5', P6: 'P6', P7: 'P7', Patm: '大气压', TAtm: '温度' },
      });
    } catch (e) {
      this.setData({ batchStatus: '批量计算失败: ' + (e.message || e), batchStatusType: 'error' });
      return;
    }
    const { summary, missing } = out;
    let msg = '批量完成: 共 ' + summary.total + ' 行，有效 ' + summary.ok + '，无效 ' + summary.invalid + '，错误 ' + summary.errors;
    if (missing && missing.length) msg += '；缺列 ' + missing.join(',') + '（缺失列的行已标记为 ERROR）';
    this.setData({
      batchHeader: out.header,
      batchRows: out.rows,
      batchPreview: out.rows.slice(0, 200),
      batchSummary: summary,
      batchStatus: msg,
      batchStatusType: summary.errors > 0 || missing.length ? 'error' : 'ok',
    });
  },

  async onImportCsv() {
    const interp = this._interp;
    if (!interp || !interp.isLoaded()) {
      this.setData({ batchStatus: '请先加载校准（7 个 .prb）', batchStatusType: 'error' });
      return;
    }
    let text;
    try {
      text = await chooseCsvText();
    } catch (err) {
      const msg = (err && err.errMsg) ? err.errMsg : (err && err.message) ? err.message : String(err);
      if (msg.indexOf('cancel') >= 0) return;
      this.setData({ batchStatus: '选择 CSV 失败: ' + msg, batchStatusType: 'error' });
      return;
    }
    this._lastCsvText = text;
    this.runBatchWith(text, this.data.units);
  },

  async onExportCsv() {
    if (!this.data.batchRows || this.data.batchRows.length === 0) {
      this.setData({ batchStatus: '请先导入并运行 CSV 再导出', batchStatusType: 'error' });
      return;
    }
    const csv = toCsv(this.data.batchHeader, this.data.batchRows);
    const name = batchFileName('seven');
    try {
      const r = await exportCsvFile(csv, name);
      let how;
      if (r.method === 'share') {
        how = '已调起分享到会话，请选择聊天或文件传输助手转发';
      } else if (r.method === 'disk') {
        how = '已保存到本地磁盘';
      } else {
        how = '已写出到 ' + r.path;
      }
      this.setData({ batchStatus: '导出成功（' + this.data.batchRows.length + ' 行）→ ' + how, batchStatusType: 'ok' });
    } catch (e) {
      // 用户在分享面板点返回 —— 静默提示，不算错误
      if (e && e.canceled) {
        this.setData({ batchStatus: '已取消分享，未导出', batchStatusType: '' });
        return;
      }
      const msg = (e && e.errMsg) ? e.errMsg : (e && e.message) ? e.message : String(e);
      this.setData({ batchStatus: '导出失败: ' + msg, batchStatusType: 'error' });
    }
  },
});
