// 结果分享卡片 —— 纯模型校验（Node 端，不涉及 wx/canvas）。
// 校验 buildCardModel 的字段完整性与内容透传，确保分享卡片拿到的是正确数据。
//
// 状态语义对齐桌面端 0.2.1「参考」体系：
//   - calculated=true & isValid=true  → statusText="参考"，statusKind="ok"
//   - calculated=true & isValid=false → statusText="参考: 原因"，statusKind="warn"
//   - calculated=false                → statusText="计算失败: 原因"，statusKind="error"
// 5/7 孔旧路径（无 calculated）回退到 isValid 决定「参考」/「参考: 原因」。
const { buildCardModel, nowStamp, statusTextOf, statusKindOf } = require('../utils/share-card.js');

let pass = 0, fail = 0;
function ok(cond, name, extra) {
  if (cond) { pass++; console.log('  PASS ' + name); }
  else { fail++; console.log('  FAIL ' + name + (extra !== undefined ? '  -> ' + JSON.stringify(extra) : '')); }
}

console.log('== buildCardModel 三孔（calculated=true & isValid=true → 参考）==');
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
  calculated: true,
  isValid: true,
  warning: '',
});
ok(three.probeType === '三孔', 'probeType 透传', three.probeType);
ok(Array.isArray(three.inputs) && three.inputs.length === 3, 'inputs 透传(3行)', three.inputs.length);
ok(three.results.length === 5, 'results 行数=dispRows(5)', three.results.length);
ok(three.results[0].label === '偏角 α' && three.results[0].value === '30.5 °', '首行 label/value 透传', three.results[0]);
ok(three.results[2].value === '120 kPa', '单位值透传(120 kPa)', three.results[2].value);
ok(three.statusText === '参考', 'calculated=true & isValid=true → statusText="参考"', three.statusText);
ok(three.statusKind === 'ok', 'statusKind="ok"', three.statusKind);
ok(three.warning === '', '空 warning 归一为字符串', three.warning);
ok(typeof three.generatedAt === 'string' && /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}$/.test(three.generatedAt), '生成时间格式', three.generatedAt);

console.log('== buildCardModel 三孔（calculated=true & isValid=false → 参考: 原因）==');
const threeOor = buildCardModel({
  probeType: '三孔',
  inputs: ['P1=.. Pa'],
  dispRows: [{ label: '偏角 α', value: '-' }],
  calculated: true,
  isValid: false,
  warning: '恢复Ma=0.95，校准范围[0.30,0.80]',
});
ok(threeOor.statusText === '参考: 恢复Ma=0.95，校准范围[0.30,0.80]', '超范围 → statusText="参考: 原因"', threeOor.statusText);
ok(threeOor.statusKind === 'warn', 'statusKind="warn"', threeOor.statusKind);

console.log('== buildCardModel 三孔（calculated=false → 计算失败: 原因）==');
const threeFail = buildCardModel({
  probeType: '三孔',
  inputs: ['P1=.. Pa'],
  dispRows: [{ label: '偏角 α', value: '-' }],
  calculated: false,
  isValid: false,
  warning: '总压低于静压',
});
ok(threeFail.statusText === '计算失败: 总压低于静压', '计算失败 → statusText="计算失败: 原因"', threeFail.statusText);
ok(threeFail.statusKind === 'error', 'statusKind="error"', threeFail.statusKind);

console.log('== buildCardModel 七孔（旧路径，无 calculated，回退到 isValid）==');
const seven = buildCardModel({
  probeType: '七孔',
  inputs: ['P1=.. Pa  ...  P7=.. Pa', 'Patm=101325 Pa', 'Tatm=20 °C'],
  dispRows: [
    { label: '侧滑角 α', value: '5.1 °' },
    { label: '迎角 β', value: '12.3 °' },
    { label: '状态', value: '参考' },
  ],
  isValid: false,
  warning: '超出校准网格，不支持外推',
});
ok(seven.probeType === '七孔', 'probeType 透传', seven.probeType);
ok(seven.results.length === 3, 'results 行数(3)', seven.results.length);
ok(seven.statusText === '参考: 超出校准网格，不支持外推', '旧路径 isValid=false → "参考: 原因"', seven.statusText);
ok(seven.statusKind === 'warn', '旧路径 isValid=false → statusKind="warn"', seven.statusKind);
ok(seven.warning === '超出校准网格，不支持外推', 'warning 透传', seven.warning);

console.log('== buildCardModel 边界 ==');
const empty = buildCardModel({});
ok(empty.probeType === '', '缺 probeType→空串', empty.probeType);
ok(Array.isArray(empty.inputs) && empty.inputs.length === 0, '缺 inputs→空数组', empty.inputs);
ok(Array.isArray(empty.results) && empty.results.length === 0, '缺 dispRows→空数组', empty.results);
ok(empty.statusText === '参考: 超出校准范围', '缺 calculated/isValid → 回退"参考: 超出校准范围"（isValid=false 默认，warning 为空时填默认原因）', empty.statusText);
ok(empty.statusKind === 'warn', '缺 calculated/isValid → statusKind="warn"', empty.statusKind);
ok(empty.warning === '', '缺 warning→空串', empty.warning);

console.log('== statusTextOf / statusKindOf 直接调用 ==');
ok(statusTextOf(false, false, 'X') === '计算失败: X', 'statusTextOf(false,false,X)', statusTextOf(false, false, 'X'));
ok(statusTextOf(true, true, '') === '参考', 'statusTextOf(true,true,"")', statusTextOf(true, true, ''));
ok(statusTextOf(true, false, 'R') === '参考: R', 'statusTextOf(true,false,R)', statusTextOf(true, false, 'R'));
ok(statusKindOf(false, false) === 'error', 'statusKindOf(false,false)', statusKindOf(false, false));
ok(statusKindOf(true, true) === 'ok', 'statusKindOf(true,true)', statusKindOf(true, true));
ok(statusKindOf(true, false) === 'warn', 'statusKindOf(true,false)', statusKindOf(true, false));

console.log('== nowStamp 格式 ==');
ok(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}$/.test(nowStamp()), 'nowStamp 格式正确', nowStamp());

console.log('\n== 结果 ==');
console.log('PASS=' + pass + '  FAIL=' + fail);
process.exit(fail ? 1 : 0);
