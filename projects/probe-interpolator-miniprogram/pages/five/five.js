const { MultiPrbInterpolator } = require('../../utils/algorithms/index.js');
const { parseMachFromFileName } = require('../../utils/algorithms/fivehole/multi-prb-interpolator.js');
const { choosePrbFiles } = require('../../utils/prb-file.js');
const { runBatch, toCsv, parseNumber } = require('../../utils/csv-batch.js');
const { chooseCsvText, exportCsvFile, batchFileName } = require('../../utils/csv-file.js');
const { formatValue, unitLabelShort, OPTIONS } = require('../../utils/units.js');
const { buildCardModel } = require('../../utils/share-card.js');
const { shareCardImage } = require('../../utils/share-canvas.js');

const PROBE_TYPE = '五孔';

// 五孔结果列（顺序即 CSV 输出列顺序）
const RESULT_COLUMNS = ['alpha', 'beta', 'machNumber', 'v', 'vx', 'vy', 'vz', 'cas', 'sat', 'dynamicPressure', 'density', 'P0', 'Ps'];
// 每列的物理量类型（供单位换算与表头后缀）
const COLUMN_UNIT = {
  alpha: 'angle', beta: 'angle', machNumber: 'none',
  v: 'velocity', vx: 'velocity', vy: 'velocity', vz: 'velocity', cas: 'velocity',
  sat: 'temp', dynamicPressure: 'pressure', density: 'none', P0: 'pressure', Ps: 'pressure',
};
// 单点结果展示定义（label + 单位类型）
const FIELD_DEFS = [
  { key: 'alpha', label: '攻角 α', unitType: 'angle' },
  { key: 'beta', label: '侧滑角 β', unitType: 'angle' },
  { key: 'machNumber', label: '马赫数 Ma', unitType: 'none' },
  { key: 'v', label: '速度 V (TAS)', unitType: 'velocity' },
  { key: 'vx', label: 'Vx', unitType: 'velocity' },
  { key: 'vy', label: 'Vy', unitType: 'velocity' },
  { key: 'vz', label: 'Vz', unitType: 'velocity' },
  { key: 'cas', label: 'CAS', unitType: 'velocity' },
  { key: 'sat', label: 'SAT', unitType: 'temp' },
  { key: 'dynamicPressure', label: '动压', unitType: 'pressure' },
  { key: 'density', label: '密度', unitType: 'none' },
  { key: 'P0', label: '总压 P0', unitType: 'pressure' },
  { key: 'Ps', label: '静压 Ps', unitType: 'pressure' },
];

// 把算法基准结果按当前显示单位整理成展示行
function buildDispRows(result, fieldDefs, units) {
  return fieldDefs.map((fd) => ({
    label: fd.label,
    value: formatValue(fd.unitType, units[fd.unitType], result[fd.key]),
  }));
}

Page({
  data: {
    p1: '', p2: '', p3: '', p4: '', p5: '',
    patm: '101325', tatm: '20',
    files: [],          // [{ filePath, name, lines }]
    result: null,
    dispRows: [],       // 按单位格式化后的展示行
    status: '',
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

  async onChooseFile() {
    try {
      const chosen = await choosePrbFiles();
      const files = this.data.files.concat(chosen.map((c) => {
        const m = parseMachFromFileName(c.filePath);
        return { filePath: c.filePath, name: c.name, lines: c.lines, mach: m > 0 ? String(m) : '' };
      }));
      this.setData({ files, status: '已加载 ' + files.length + ' 个 PRB 文件', statusType: 'ok' });
    } catch (err) {
      const msg = (err && err.errMsg) ? err.errMsg : (err && err.message) ? err.message : String(err);
      if (msg.indexOf('cancel') >= 0) return; // 用户取消
      this.setData({ status: '选择文件失败: ' + msg, statusType: 'error' });
    }
  },

  onMachInput(e) {
    const idx = Number(e.currentTarget.dataset.idx);
    const files = this.data.files.slice();
    if (idx < 0 || idx >= files.length) return;
    files[idx] = Object.assign({}, files[idx], { mach: e.detail.value });
    this.setData({ files });
  },

  clearFiles() {
    this.setData({ files: [], status: '已清空校准文件', statusType: '' });
  },

  collectMachNumbers() {
    const nums = [];
    for (const f of this.data.files) {
      const m = parseNumber(f.mach);
      if (!isFinite(m) || m <= 0) {
        return { ok: false, error: '请为文件「' + f.name + '」填写有效的马赫数 Ma（文件名未包含马赫数，需手动输入）' };
      }
      nums.push(m);
    }
    return { ok: true, machNumbers: nums };
  },

  onCalculate() {
    const d = this.data;
    if (d.files.length === 0) {
      this.setData({ status: '请先上传至少一个 .prb 校准文件', statusType: 'error' });
      return;
    }
    const ph = [d.p1, d.p2, d.p3, d.p4, d.p5].map(parseNumber);
    if (!ph.every(isFinite)) {
      this.setData({ status: '请填写完整的 5 个孔压（Pa）', statusType: 'error' });
      return;
    }
    const patm = parseNumber(d.patm), tatm = parseNumber(d.tatm);
    if (!isFinite(patm) || !isFinite(tatm)) {
      this.setData({ status: '请填写大气压 Patm（Pa）与气温 TAtm（°C）', statusType: 'error' });
      return;
    }
    const input = { P1: ph[0], P2: ph[1], P3: ph[2], P4: ph[3], P5: ph[4], PAtm: patm, TAtm: tatm };

    const machInfo = this.collectMachNumbers();
    if (!machInfo.ok) {
      this.setData({ status: machInfo.error, statusType: 'error', result: null, dispRows: [] });
      return;
    }
    const interp = new MultiPrbInterpolator();
    const loadRes = interp.loadPrbData(d.files, machInfo.machNumbers);
    if (!loadRes.ok) {
      const warn = (loadRes.warnings || []).join('；');
      this.setData({ status: '加载失败: ' + (loadRes.error || '') + (warn ? '；' + warn : ''), statusType: 'error', result: null, dispRows: [] });
      return;
    }
    const { result, error } = interp.calculate(input);
    if (error) {
      this.setData({ status: '计算失败: ' + error, statusType: 'error', result: null, dispRows: [] });
      return;
    }
    const warn = (loadRes.warnings || []).concat(result.warning ? [result.warning] : []).filter(Boolean);
    this.setData({
      result,
      dispRows: buildDispRows(result, FIELD_DEFS, d.units),
      status: warn.length ? warn.join('；') : '计算完成',
      statusType: result.isValid ? 'ok' : 'error',
    });
  },

  onCopyResult() {
    const rows = this.data.dispRows || [];
    if (!rows.length) {
      this.setData({ status: '无结果可复制', statusType: 'error' });
      return;
    }
    const lines = rows.map((r) => r.label + '\t' + r.value);
    const text = '五孔探针结果\r\n' + lines.join('\r\n');
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
    const ps = [d.p1, d.p2, d.p3, d.p4, d.p5];
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

  _fmtVal(units) {
    return (col, raw) => formatValue(COLUMN_UNIT[col] || 'none', units[COLUMN_UNIT[col] || 'none'], raw);
  },
  _colHdr(units) {
    return (col) => {
      const t = COLUMN_UNIT[col] || 'none';
      if (t === 'none') return col;
      return col + '_' + unitLabelShort(t, units[t]);
    };
  },

  // 批量运行（含单位换算），供导入与单位切换复用
  runBatchWith(text, units) {
    const d = this.data;
    if (d.files.length === 0) {
      this.setData({ batchStatus: '请先上传至少一个 .prb 校准文件', batchStatusType: 'error' });
      return;
    }
    const machInfo = this.collectMachNumbers();
    if (!machInfo.ok) {
      this.setData({ batchStatus: machInfo.error, batchStatusType: 'error' });
      return;
    }
    const interp = new MultiPrbInterpolator();
    const loadRes = interp.loadPrbData(d.files, machInfo.machNumbers);
    if (!loadRes.ok) {
      const warn = (loadRes.warnings || []).join('；');
      this.setData({ batchStatus: '加载失败: ' + (loadRes.error || '') + (warn ? '；' + warn : ''), batchStatusType: 'error' });
      return;
    }
    let out;
    try {
      out = runBatch(text, {
        holeCount: 5,
        defaults: { PAtm: Number(d.patm), TAtm: Number(d.tatm) },
        interp,
        calculateRow: (it, input) => {
          const { result, error } = it.calculate(input);
          if (error) throw new Error(error);
          return result;
        },
        resultColumns: RESULT_COLUMNS,
        formatResultValue: this._fmtVal(units),
        resultColumnHeader: this._colHdr(units),
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
    const name = batchFileName('five');
    try {
      const r = await exportCsvFile(csv, name);
      const how = r.method === 'share' ? '已调起分享到会话' : (r.method === 'disk' ? '已保存到本地' : '已写出到 ' + r.path);
      this.setData({ batchStatus: '导出成功（' + this.data.batchRows.length + ' 行）→ ' + how, batchStatusType: 'ok' });
    } catch (e) {
      const msg = (e && e.errMsg) ? e.errMsg : (e && e.message) ? e.message : String(e);
      this.setData({ batchStatus: '导出失败: ' + msg, batchStatusType: 'error' });
    }
  },
});
