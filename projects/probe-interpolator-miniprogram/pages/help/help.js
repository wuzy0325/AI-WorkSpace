// 使用说明页：三/五/七孔通用 help 页面
// 通过 query.tab 指定初始 tab（'three' / 'five' / 'seven'），缺省 'three'
// current 字段统一指向当前 tab 的说明对象，wxml 用 {{current.xxx}} 渲染，避免三段 wx:if 重复
Page({
  data: {
    activeTab: 'three',
    tabs: [
      { key: 'three', label: '三孔' },
      { key: 'five', label: '五孔' },
      { key: 'seven', label: '七孔' },
    ],
    // 三孔说明数据
    three: {
      overview: '一维角度（侧滑角 α）求解 · 1~10 个 .prb 跨马赫数档 · 3 孔压输入',
      calib: [
        '把 .prb 发到微信聊天或「文件传输助手」',
        '在「校准文件」卡片点「选择 .prb」',
        '从微信会话文件列表中选择文件（支持一次选多个，或多次累加）',
        '支持加载多份不同马赫档 .prb，算法会按 Ma 迭代收敛',
        '多文件约束：各档 Nalpha 与 Alpha 网格必须完全一致，每档 Kb 严格单调递增',
      ],
      inputs: [
        { name: 'P1', unit: 'Pa', req: true, desc: '第 1 孔压力' },
        { name: 'P2', unit: 'Pa', req: true, desc: '第 2 孔压力（中心孔）' },
        { name: 'P3', unit: 'Pa', req: true, desc: '第 3 孔压力' },
        { name: 'Patm', unit: 'Pa', req: true, desc: '大气压（缺省 101325）' },
        { name: 'Tatm', unit: '℃', req: true, desc: '大气温度（缺省 20）' },
      ],
      outputs: [
        { name: 'alpha', unit: 'deg', desc: '偏角 α（可切 rad）' },
        { name: 'machNumber', unit: '—', desc: '马赫数' },
        { name: 'P0', unit: 'Pa', desc: '总压（可切 kPa/MPa）' },
        { name: 'Ps', unit: 'Pa', desc: '静压（可切 kPa/MPa）' },
      ],
      faq: [
        { q: '空字段会怎样？', a: '空串/空白/非数字会直接报错（合法 0 仍允许），不会静默按 0 Pa 处理。' },
        { q: '为什么 PC 微信打不开文件选择器？', a: 'PC 微信 wx.chooseMessageFile 兼容性较差，建议用手机微信扫码预览小程序。' },
        { q: '导出 CSV 时点「取消分享」没生成文件？', a: '取消分享面板会显示「已取消分享，未导出」，结果数据仍保留在内存中，可重新点「导出结果」。' },
        { q: '结果状态「参考: 超范围」是什么意思？', a: 'α 角度超出 .prb 校准网格覆盖范围，结果仅供参考（仍输出数值，置信度低）。CSV 同步输出「参考: 原因」。' },
      ],
      // CSV 批量导入格式说明（与 utils/csv-batch.js 的实际列匹配逻辑一致）
      csv: {
        note: '「CSV 批量」tab 内点「导入 CSV 并计算」，一次灌入多组孔压。第一行为表头，逐行计算，结果同步写入表格并可导出。',
        columns: [
          { name: 'P1 ~ P3', req: true, desc: '3 个孔压，单位 Pa；缺列或非数字该行标 ERROR' },
          { name: 'Patm', req: false, desc: '大气压 Pa，可选；缺省回退页面当前大气输入' },
          { name: 'TAtm', req: false, desc: '大气温度 ℃，可选；缺省回退页面当前大气输入' },
        ],
        headerTol: [
          '表头忽略大小写、空格、下划线与标点',
          '单位括号自动忽略：P1 (Pa)、大气压(Pa) 均识别',
          '中文别名支持：大气压/环境压 → Patm，气温/温度/环境温 → TAtm',
          'Patm/TAtm 列存在但非数字 → 该行标 ERROR，不静默用默认值',
        ],
        outputNote: '输出：原输入列 + Patm、TAtm + 结果列（偏角α、马赫数、总压P0、静压Ps）+ 状态列（参考 / 参考:原因 / 计算失败:原因 / ERROR）。',
      },
    },
    // 五孔说明数据
    five: {
      overview: '二维角度（偏角 α + 俯仰角 β）求解 · 多马赫 .prb 校准 · 5 孔压输入',
      calib: [
        '把 .prb 发到微信聊天或「文件传输助手」',
        '在「校准文件」卡片点「选择 .prb」',
        '从微信会话文件列表中选择文件',
        '检查马赫数：文件名含 0.8Ma 等字样会自动识别，否则在右侧 Ma 输入框手动填写',
        '每份 .prb 都必须关联一个马赫数',
      ],
      inputs: [
        { name: 'P1', unit: 'Pa', req: true, desc: '第 1 孔压力' },
        { name: 'P2', unit: 'Pa', req: true, desc: '第 2 孔压力' },
        { name: 'P3', unit: 'Pa', req: true, desc: '第 3 孔压力' },
        { name: 'P4', unit: 'Pa', req: true, desc: '第 4 孔压力' },
        { name: 'P5', unit: 'Pa', req: true, desc: '第 5 孔压力（中心孔）' },
        { name: 'Patm', unit: 'Pa', req: true, desc: '大气压（缺省 101325）' },
        { name: 'Tatm', unit: '℃', req: true, desc: '大气温度（缺省 20）' },
      ],
      outputs: [
        { name: 'alpha', unit: 'deg', desc: '偏角 α（可切 rad）' },
        { name: 'beta', unit: 'deg', desc: '俯仰角 β（可切 rad）' },
        { name: 'machNumber', unit: '—', desc: '马赫数' },
        { name: 'v', unit: 'm/s', desc: '速度 V（可切 km/h）' },
        { name: 'cas', unit: 'm/s', desc: '校正空速（可切 km/h）' },
        { name: 'sat', unit: '℃', desc: '静温（可切 K/°F）' },
        { name: 'dynamicPressure', unit: 'Pa', desc: '动压 q（可切 kPa/MPa）' },
        { name: 'density', unit: 'kg/m³', desc: '密度 ρ' },
        { name: 'P0', unit: 'Pa', desc: '总压（可切 kPa/MPa）' },
        { name: 'Ps', unit: 'Pa', desc: '静压（可切 kPa/MPa）' },
      ],
      faq: [
        { q: '文件名不带马赫数怎么办？', a: '在文件列表右侧的 Ma 输入框手动填写马赫数即可。每份 .prb 都必须关联一个马赫数。' },
        { q: 'vx/vy/vz 是什么坐标系？', a: '体轴系下的速度三分量：vx 沿探针轴向，vy/vz 垂直探针轴。' },
        { q: '结果状态「参考: 超范围」是什么意思？', a: 'α/β 角度超出 .prb 校准网格覆盖范围，或当前马赫数不在已加载 .prb 的马赫区间内。结果仅供参考。' },
        { q: '空字段会怎样？', a: '空串/空白/非数字会直接报错（合法 0 仍允许），不会静默按 0 Pa 处理。' },
      ],
      // CSV 批量导入格式说明（与 utils/csv-batch.js 的实际列匹配逻辑一致）
      csv: {
        note: '「CSV 批量」tab 内点「导入 CSV 并计算」，一次灌入多组孔压。第一行为表头，逐行计算，结果同步写入表格并可导出。',
        columns: [
          { name: 'P1 ~ P5', req: true, desc: '5 个孔压，单位 Pa；缺列或非数字该行标 ERROR' },
          { name: 'Patm', req: false, desc: '大气压 Pa，可选；缺省回退页面当前大气输入' },
          { name: 'TAtm', req: false, desc: '大气温度 ℃，可选；缺省回退页面当前大气输入' },
        ],
        headerTol: [
          '表头忽略大小写、空格、下划线与标点',
          '单位括号自动忽略：P1 (Pa)、大气压(Pa) 均识别',
          '中文别名支持：大气压/环境压 → Patm，气温/温度/环境温 → TAtm',
          'Patm/TAtm 列存在但非数字 → 该行标 ERROR，不静默用默认值',
        ],
        outputNote: '输出：原输入列 + Patm、TAtm + 结果列（攻角α、侧滑角β、马赫数、速度V/Vx/Vy/Vz、CAS、SAT、动压、密度、总压P0、静压Ps）+ isValid/warning 列。',
      },
    },
    // 七孔说明数据
    seven: {
      overview: '大角度范围二维角度（α + β）求解 · 7.prb 内区 + 1~6.prb 外区 · 大小角度自动判定',
      calib: [
        '把 7 个 .prb（或 7 份校准 CSV）发到微信聊天',
        '在「校准文件」卡片点「加载校准文件（.prb / .csv）」',
        '从微信会话文件列表中选择 7 个文件',
        '按 basename 路由：7=内区，1~6=外区扇区',
        '加载成功后显示「内区：7.prb · 外区：1~6.prb」',
      ],
      inputs: [
        { name: 'P1', unit: 'Pa', req: true, desc: '第 1 孔压力（外区扇区 1）' },
        { name: 'P2', unit: 'Pa', req: true, desc: '第 2 孔压力（外区扇区 2）' },
        { name: 'P3', unit: 'Pa', req: true, desc: '第 3 孔压力（外区扇区 3）' },
        { name: 'P4', unit: 'Pa', req: true, desc: '第 4 孔压力（外区扇区 4）' },
        { name: 'P5', unit: 'Pa', req: true, desc: '第 5 孔压力（外区扇区 5）' },
        { name: 'P6', unit: 'Pa', req: true, desc: '第 6 孔压力（外区扇区 6）' },
        { name: 'P7', unit: 'Pa', req: true, desc: '第 7 孔压力（中心孔，内区）' },
        { name: 'Patm', unit: 'Pa', req: true, desc: '大气压（缺省 101325）' },
        { name: 'Tatm', unit: '℃', req: true, desc: '大气温度（缺省 20）' },
      ],
      outputs: [
        { name: 'alpha', unit: 'deg', desc: '侧滑角 α（可切 rad）' },
        { name: 'beta', unit: 'deg', desc: '迎角 β（可切 rad）' },
        { name: 'theta', unit: 'deg', desc: '流场坐标角 θ（可切 rad）' },
        { name: 'phi', unit: 'deg', desc: '流场坐标角 φ（可切 rad）' },
        { name: 'machNumber', unit: '—', desc: '马赫数' },
        { name: 'velocity', unit: 'm/s', desc: '速度 V（可切 km/h）' },
        { name: 'totalPressure', unit: 'Pa', desc: '总压 P0（可切 kPa/MPa）' },
        { name: 'staticPressure', unit: 'Pa', desc: '静压 Ps（可切 kPa/MPa）' },
        { name: 'dynamicPressure', unit: 'Pa', desc: '动压 q（可切 kPa/MPa）' },
      ],
      faq: [
        { q: '为什么必须 7 个文件？', a: '校准网格由内区 + 6 个外区扇区组成。内区 7.prb 覆盖小角度，外区 1~6.prb 分别覆盖大角度的 6 个 60° 扇区。' },
        { q: 'θ/φ 和 α/β 有什么区别？', a: 'α/β 是探针体轴系角度（侧滑角/迎角），θ/φ 是流场坐标角度（流速方向在球面投影下的极角/方位角）。' },
        { q: 'CSV 校准文件格式要求？', a: '必需 ≥16 列，按列位置读取（col 0,1,12,13,14,15）。外区 theta 网格起点 30°、步长 5° 校验。' },
        { q: '加载 CSV 报「退化边」警告怎么办？', a: '相邻网格点 ka/kb 相等导致 bilinear 不可逆时会施加 1e-9 偏移最多 100 轮。仍有退化边则报错，通常是 CSV 数据质量问题。' },
      ],
      // CSV 批量导入格式说明（与 utils/csv-batch.js 的实际列匹配逻辑一致）
      csv: {
        note: '「CSV 批量」tab 内点「导入 CSV 并计算」，一次灌入多组孔压。第一行为表头，逐行计算，结果同步写入表格并可导出。',
        columns: [
          { name: 'P1 ~ P7', req: true, desc: '7 个孔压，单位 Pa；缺列或非数字该行标 ERROR' },
          { name: 'Patm', req: false, desc: '大气压 Pa，可选；缺省回退页面当前大气输入' },
          { name: 'TAtm', req: false, desc: '大气温度 ℃，可选；缺省回退页面当前大气输入' },
        ],
        headerTol: [
          '表头忽略大小写、空格、下划线与标点',
          '单位括号自动忽略：P1 (Pa)、大气压(Pa) 均识别',
          '中文别名支持：大气压/环境压 → Patm，气温/温度/环境温 → TAtm',
          'Patm/TAtm 列存在但非数字 → 该行标 ERROR，不静默用默认值',
        ],
        outputNote: '输出：原输入列 + Patm、TAtm + 结果列（侧滑角α、迎角β、θ俯仰、φ方位、马赫数、速度V、总压Pt、静压Ps、动压）+ isValid/warning 列。',
      },
    },
    // 当前 tab 的说明对象引用（onLoad/onSwitchTab 同步更新）
    current: null,
  },

  onLoad(query) {
    // 支持 query.tab 指定初始 tab，非法/缺省回退 'three'
    const raw = query && query.tab;
    const tab = (raw === 'five' || raw === 'seven') ? raw : 'three';
    this.setData({ activeTab: tab, current: this.data[tab] });
  },

  onSwitchTab(e) {
    const tab = e.currentTarget.dataset.tab;
    if (tab && tab !== this.data.activeTab) {
      this.setData({ activeTab: tab, current: this.data[tab] });
    }
  },
});
