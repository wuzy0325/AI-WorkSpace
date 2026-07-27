// 结果分享卡片 —— 纯模型校验（Node 端，不涉及 wx/canvas）。
// 校验 buildCardModel 的字段完整性与内容透传，确保分享卡片拿到的是正确数据。
const { buildCardModel, nowStamp } = require('../utils/share-card.js');

let pass = 0, fail = 0;
function ok(cond, name, extra) {
  if (cond) { pass++; console.log('  PASS ' + name); }
  else { fail++; console.log('  FAIL ' + name + (extra !== undefined ? '  -> ' + JSON.stringify(extra) : '')); }
}

console.log('== buildCardModel 三孔 ==');
const three = buildCardModel({
  probeType: '三孔',
  inputs: ['P1=1000 Pa  P2=980 Pa  P3=1010 Pa', 'Patm=101325 Pa', 'Tatm=20 °C'],
  dispRows: [
    { label: '偏角 α', value: '30.5 °' },
    { label: '马赫数 Ma', value: '0.42' },
    { label: '总压 P0', value: '120 kPa' },
    { label: '静压 Ps', value: '101 kPa' },
    { label: '迭代次数', value: '12' },
  ],
  isValid: true,
  warning: '',
});
ok(three.probeType === '三孔', 'probeType 透传', three.probeType);
ok(Array.isArray(three.inputs) && three.inputs.length === 3, 'inputs 透传(3行)', three.inputs.length);
ok(three.results.length === 5, 'results 行数=dispRows(5)', three.results.length);
ok(three.results[0].label === '偏角 α' && three.results[0].value === '30.5 °', '首行 label/value 透传', three.results[0]);
ok(three.results[2].value === '120 kPa', '单位值透传(120 kPa)', three.results[2].value);
ok(three.isValid === true, 'isValid 透传', three.isValid);
ok(three.warning === '', '空 warning 归一为字符串', three.warning);
ok(typeof three.generatedAt === 'string' && /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}$/.test(three.generatedAt), '生成时间格式', three.generatedAt);

console.log('== buildCardModel 七孔（含 warning / 无效）==');
const seven = buildCardModel({
  probeType: '七孔',
  inputs: ['P1=.. Pa  ...  P7=.. Pa', 'Patm=101325 Pa', 'Tatm=20 °C'],
  dispRows: [
    { label: '侧滑角 α', value: '5.1 °' },
    { label: '迎角 β', value: '12.3 °' },
    { label: '有效性', value: '无效' },
  ],
  isValid: false,
  warning: '超出校准网格，不支持外推',
});
ok(seven.probeType === '七孔', 'probeType 透传', seven.probeType);
ok(seven.results.length === 3, 'results 行数(3)', seven.results.length);
ok(seven.isValid === false, 'isValid=false 透传', seven.isValid);
ok(seven.warning === '超出校准网格，不支持外推', 'warning 透传', seven.warning);

console.log('== buildCardModel 边界 ==');
const empty = buildCardModel({});
ok(empty.probeType === '', '缺 probeType→空串', empty.probeType);
ok(Array.isArray(empty.inputs) && empty.inputs.length === 0, '缺 inputs→空数组', empty.inputs);
ok(Array.isArray(empty.results) && empty.results.length === 0, '缺 dispRows→空数组', empty.results);
ok(empty.isValid === false, '缺 isValid→false', empty.isValid);
ok(empty.warning === '', '缺 warning→空串', empty.warning);

console.log('== nowStamp 格式 ==');
ok(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}$/.test(nowStamp()), 'nowStamp 格式正确', nowStamp());

console.log('\n== 结果 ==');
console.log('PASS=' + pass + '  FAIL=' + fail);
process.exit(fail ? 1 : 0);
