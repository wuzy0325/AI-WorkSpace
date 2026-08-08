const { ThreeHoleInterpolator } = require('../../utils/algorithms/index.js');
const { choosePrbFiles } = require('../../utils/prb-file.js');
const { runBatch, toCsv, parseNumber } = require('../../utils/csv-batch.js');
const { chooseCsvText, exportCsvFile, batchFileName } = require('../../utils/csv-file.js');
const { formatValue, convertNumber, unitLabelShort, OPTIONS } = require('../../utils/units.js');
const { buildCardModel } = require('../../utils/share-card.js');
const { shareCardImage } = require('../../utils/share-canvas.js');

const PROBE_TYPE = '三孔';

// 三孔结果列（顺序即 CSV 输出列顺序）—— 不含 iterationCount，UI 不展示迭代次数
const RESULT_COLUMNS = ['alpha', 'machNumber', 'P0', 'Ps'];
// 每列的物理量类型（供单位换算与表头后缀）
const COLUMN_UNIT = { alpha: 'angle', machNumber: 'none', P0: 'pressure', Ps: 'pressure' };
// CSV 表格各列固定精度（小数位）—— 列宽统一 + 右对齐 + 固定精度 = 视觉对齐
// 角度 3 位、马赫数 4 位、压力 2 位（Pa 基准）
const COLUMN_DECIMALS = { alpha: 3, machNumber: 4, P0: 2, Ps: 2 };
// CSV 表头中文标签（替代英文 key + 后缀，提升可读性）
const COLUMN_LABEL = {
  alpha: '偏角α', machNumber: '马赫数', P0: '总压P0', Ps: '静压Ps',
};
// 单点结果展示定义（label + 单位类型）—— 不含 iterationCount，UI 不展示迭代次数
const FIELD_DEFS = [
  { key: 'alpha', label: '偏角 α', unitType: 'angle' },
  { key: 'machNumber', label: '马赫数 Ma', unitType: 'none' },
  { key: 'P0', label: '总压 P0', unitType: 'pressure' },
  { key: 'Ps', label: '静压 Ps', unitType: 'pressure' },
];

// 同步自桌面端 0.2.1：三孔结果状态统一显示"参考"。
// 已计算（calculated=true）一律标注"参考"并展示数值；超范围（isValid=false）琥珀色 + 原因；
// 计算失败（calculated=false）红色 + 原因；UI 与 CSV 同步该语义。
function statusTextOf(result) {
  if (!result) return '';
  if (!result.calculated) return '计算失败: ' + (result.warning || '未知原因');
  return result.isValid ? '参考' : '参考: ' + (result.warning || '超出校准范围');
}

function statusKindOf(result) {
  // 返回 '' | 'ok' | 'warn' | 'error'，供 UI 样式与状态条复用
  if (!result) return '';
  if (!result.calculated) return 'error';
  return result.isValid ? 'ok' : 'warn';
}

// 把算法基准结果按当前显示单位整理成展示行。
// calculated=false 时数值字段一律显示"-"（与桌面端 formatResultNum 一致）。
function buildDispRows(result, fieldDefs, units) {
  const showValue = !!(result && result.calculated);
  return fieldDefs.map((fd) => ({
    label: fd.label,
    value: showValue ? formatValue(fd.unitType, units[fd.unitType], result[fd.key]) : '-',
  }));
}

Page({
  data: {
    p1: '', p2: '', p3: '',
    patm: '101325', tatm: '20',
    files: [],          // [{ filePath, name, lines }]
    result: null,
    dispRows: [],       // 按单位格式化后的展示行
    statusText: '',     // 结果状态文本（参考 / 参考: 原因 / 计算失败: 原因）
    statusKind: '',     // '' | 'ok' | 'warn' | 'error'
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
    // 数据输入方式 tab：'manual'（手动输入单点） | 'csv'（CSV 批量）
    // 两种方式并列，用户二选一；计算按钮只在 manual tab 内（CSV 导入自带批量计算）
    activeInputTab: 'manual',
  },

  // 跳转使用说明页并预选三孔 tab
  goHelp() {
    wx.navigateTo({ url: '/pages/help/help?tab=three' });
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

  async onChooseFile() {
    try {
      const chosen = await choosePrbFiles();
      const files = this.data.files.concat(chosen.map((c) => ({ filePath: c.filePath, name: c.name, lines: c.lines })));
      this.setData({ files, status: '已加载 ' + files.length + ' 个 PRB 文件', statusType: 'ok' });
    } catch (err) {
      const msg = (err && err.errMsg) ? err.errMsg : (err && err.message) ? err.message : String(err);
      if (msg.indexOf('cancel') >= 0) return; // 用户取消
      this.setData({ status: '选择文件失败: ' + msg, statusType: 'error' });
    }
  },

  clearFiles() {
    this.setData({ files: [], status: '已清空校准文件', statusType: '' });
  },

  onCalculate() {
    const d = this.data;
    if (d.files.length === 0) {
      this.setData({ status: '请先上传至少一个 .prb 校准文件', statusType: 'error' });
      return;
    }
    const p1 = parseNumber(d.p1), p2 = parseNumber(d.p2), p3 = parseNumber(d.p3);
    if (![p1, p2, p3].every(isFinite)) {
      this.setData({ status: '请填写完整的 3 个孔压（Pa）', statusType: 'error' });
      return;
    }
    const patm = parseNumber(d.patm), tatm = parseNumber(d.tatm);
    if (!isFinite(patm) || !isFinite(tatm)) {
      this.setData({ status: '请填写大气压 Patm（Pa）与气温 TAtm（°C）', statusType: 'error' });
      return;
    }
    const input = { P1: p1, P2: p2, P3: p3, PAtm: patm, TAtm: tatm };

    const interp = new ThreeHoleInterpolator();
    const loadRes = interp.loadPrbData(d.files);
    if (!loadRes.ok) {
      this.setData({ status: '加载失败: ' + (loadRes.error || ''), statusType: 'error', result: null, dispRows: [], statusText: '', statusKind: '' });
      return;
    }
    const { result, error } = interp.calculate(input);
    if (error) {
      this.setData({ status: '计算失败: ' + error, statusType: 'error', result: null, dispRows: [], statusText: '', statusKind: '' });
      return;
    }
    // 校准加载阶段的 warnings（如退化边抖动）与单点结果 warning 合并到状态条提示
    const loadWarnings = (loadRes.warnings || []).filter(Boolean);
    const resultWarnings = result.warning ? [result.warning] : [];
    const allWarnings = loadWarnings.concat(resultWarnings);
    const kind = statusKindOf(result);
    const statusText = statusTextOf(result);
    // 状态条优先展示结果状态；若存在加载 warning 则附加在后
    const statusMsg = allWarnings.length
      ? statusText + (loadWarnings.length ? '；加载提示: ' + loadWarnings.join('；') : '')
      : '计算完成';
    this.setData({
      result,
      dispRows: buildDispRows(result, FIELD_DEFS, d.units),
      statusText,
      statusKind: kind,
      status: statusMsg,
      // 状态条颜色：参考-范围内=ok，参考-超范围=warn（映射为 ok 视觉相近，但保留语义），计算失败=error
      // 小程序 wxss 仅定义了 ok/error，warn 统一走 ok 视觉（实际 warn 文案已带"参考: 原因"）
      statusType: kind === 'error' ? 'error' : 'ok',
    });
  },

  onCopyResult() {
    const rows = this.data.dispRows || [];
    if (!rows.length) {
      this.setData({ status: '无结果可复制', statusType: 'error' });
      return;
    }
    const lines = rows.map((r) => r.label + '\t' + r.value);
    const text = '三孔探针结果\r\n状态: ' + (this.data.statusText || '') + '\r\n' + lines.join('\r\n');
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
    const ps = [d.p1, d.p2, d.p3];
    const inputs = [
      ps.map((v, i) => 'P' + (i + 1) + '=' + v + ' Pa').join('  '),
      'Patm=' + d.patm + ' Pa',
      'Tatm=' + d.tatm + ' °C',
    ];
    const model = buildCardModel({
      probeType: PROBE_TYPE,
      inputs,
      dispRows: d.dispRows,
      // 卡片状态语义对齐桌面端"参考"：calculated=false 显示"计算失败"，否则显示"参考"
      statusText: d.statusText,
      statusKind: d.statusKind,
      isValid: d.result.isValid,
      warning: d.result.warning,
      calculated: d.result.calculated,
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
  // 这样列宽统一 + 右对齐 + tabular-nums 后视觉严格对齐
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
  // CSV 表格输入列格式化：P1~P3/Patm/TAtm 统一 2 位。
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

  // 批量运行（含单位换算），供导入与单位切换复用。
  // 同步自桌面端 0.2.1：computed/reference/failed 三类统计，
  // calculated=false 时数值字段置空（CSV 输出空单元格，UI 显示"-"）。
  runBatchWith(text, units) {
    const d = this.data;
    if (d.files.length === 0) {
      this.setData({ batchStatus: '请先上传至少一个 .prb 校准文件', batchStatusType: 'error' });
      return;
    }
    const interp = new ThreeHoleInterpolator();
    const loadRes = interp.loadPrbData(d.files);
    if (!loadRes.ok) {
      this.setData({ batchStatus: '加载失败: ' + (loadRes.error || ''), batchStatusType: 'error' });
      return;
    }
    // 闭包统计：computed（已计算）/ reference（参考-超范围）/ failed（计算失败）
    let computed = 0, reference = 0, failed = 0;
    let out;
    try {
      out = runBatch(text, {
        holeCount: 3,
        defaults: { PAtm: Number(d.patm), TAtm: Number(d.tatm) },
        interp,
        // calculateRow 返回 raw warning（不带前缀），由 csv-batch 的 statusTextOf 统一加「参考: / 计算失败: 」前缀。
        // 这样单点 UI（statusTextOf）与 CSV 输出共用同一份 warning 文本，避免前缀重复。
        calculateRow: (it, input) => {
          const { result, error } = it.calculate(input);
          if (error) throw new Error(error);
          if (result.calculated) {
            computed++;
            if (!result.isValid) reference++;
            return {
              alpha: result.alpha,
              machNumber: result.machNumber,
              P0: result.P0,
              Ps: result.Ps,
              calculated: true,
              isValid: result.isValid,
              warning: result.warning || '',
            };
          }
          // 计算失败：数值字段置空，CSV 输出空单元格；warning 保留原始原因
          failed++;
          return {
            alpha: null, machNumber: null, P0: null, Ps: null,
            calculated: false,
            isValid: false,
            warning: result.warning || '',
          };
        },
        resultColumns: RESULT_COLUMNS,
        formatResultValue: this._fmtVal(units),
        resultColumnHeader: this._colHdr(units),
        // 输入列固定精度：P1~P3/Patm 压力 2 位、TAtm 温度 2 位，与结果列一起保证表格视觉对齐
        inputFormat: (k, v) => this._fmtInput(k, v, units),
        // 输入列表头中文标签
        inputHeaderMap: { P1: 'P1', P2: 'P2', P3: 'P3', Patm: '大气压', TAtm: '温度' },
        // 启用「状态」文本列（对齐桌面端 0.2.1「参考」语义），替代 isValid+warning 两列
        statusColumn: true,
      });
    } catch (e) {
      this.setData({ batchStatus: '批量计算失败: ' + (e.message || e), batchStatusType: 'error' });
      return;
    }
    const { summary, missing } = out;
    // 摘要：已计算 X 条（Y 条超出校准范围，仅供参考），失败 Z 条，错误 W 条
    let msg = '批量完成: 共 ' + summary.total + ' 行，已计算 ' + computed + ' 条';
    if (reference > 0) msg += '（' + reference + ' 条超出校准范围，仅供参考）';
    if (failed > 0) msg += '，失败 ' + failed + ' 条';
    if (summary.errors > 0) msg += '，解析错误 ' + summary.errors + ' 行';
    if (missing && missing.length) msg += '；缺列 ' + missing.join(',') + '（缺失列的行已标记为 ERROR）';
    this.setData({
      batchHeader: out.header,
      batchRows: out.rows,
      batchPreview: out.rows.slice(0, 200),
      batchSummary: summary,
      batchStatus: msg,
      batchStatusType: failed > 0 || summary.errors > 0 || missing.length ? 'error' : 'ok',
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
    const name = batchFileName('three');
    try {
      const r = await exportCsvFile(csv, name);
      // 根据 method 给出对应的友好提示
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
