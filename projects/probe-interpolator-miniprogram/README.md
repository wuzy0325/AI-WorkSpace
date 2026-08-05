# 探针插值计算器 · 微信小程序

把桌面端 `probe-interpolator`（3/5/7 孔风洞探针插值）的 Go 核心算法，移植为**纯前端 JavaScript** 微信小程序。
全离线运行，无需服务器；校准 `.prb` 由用户从微信会话上传。

## 使用说明书

三/五/七孔探针各有一份 HTML 说明书，配套小程序界面截图（SVG），可在浏览器直接打开：

| 说明书 | 入口 | 适用场景 |
|---|---|---|
| [三孔使用说明](docs/manual-three.html) | 三孔探针 · 一维角度（α）求解 | 1~10 个 .prb 跨马赫数档，3 孔压输入 |
| [五孔使用说明](docs/manual-five.html) | 五孔探针 · 二维角度（α/β）求解 | 多马赫 .prb 校准，5 孔压输入 |
| [七孔使用说明](docs/manual-seven.html) | 七孔探针 · 大角度范围自动分区 | 7.prb 内区 + 1~6.prb 外区，7 孔压输入 |

**小程序内帮助**：首页 hero 右上角与三/五/七孔工作区标题右侧均有「使用说明 / ?」按钮，点击进入小程序内帮助页（`pages/help/help`），内含概览、校准文件准备步骤、输入/输出字段说明、**CSV 批量导入格式**（列约定 / 表头容错 / 输出列）与常见问题；从探针工作区进入时自动定位到对应探针的 tab。

### 界面截图（SVG）

所有截图均为小程序版本的真实界面示意（保留手机外框 + 微信胶囊 + Home 指示条）：

| 编号 | 文件 | 说明 |
|---|---|---|
| 01 | [01_home.svg](docs/screenshots/01_home.svg) | 首页探针选择瓦片（hero + 3/5/7 瓦片） |
| 02 | [02_three_input.svg](docs/screenshots/02_three_input.svg) | 三孔输入界面（校准文件 + 3 孔压 + 大气参数） |
| 03 | [03_three_result.svg](docs/screenshots/03_three_result.svg) | 三孔计算结果（α / Ma / P0 / Ps + 状态胶囊） |
| 04 | [04_csv_batch.svg](docs/screenshots/04_csv_batch.svg) | CSV 批量结果（斑马纹表格 + 错误行高亮） |
| 05 | [05_five_input.svg](docs/screenshots/05_five_input.svg) | 五孔输入界面（多 .prb + 每行 Ma 输入） |
| 06 | [06_five_result.svg](docs/screenshots/06_five_result.svg) | 五孔计算结果（13 项气动参数） |
| 07 | [07_seven_input.svg](docs/screenshots/07_seven_input.svg) | 七孔输入界面（7 个 prb 标签 + 7 孔压） |
| 08 | [08_seven_result.svg](docs/screenshots/08_seven_result.svg) | 七孔计算结果（9 项气动参数） |

## 架构

```
probe-interpolator-miniprogram/
├── app.json / app.js / app.wxss       # 小程序入口与全局样式
├── project.config.json / sitemap.json # 微信开发者工具工程配置
├── docs/                              # 使用说明书（HTML）+ 小程序界面截图（SVG）
│   ├── manual-three.html              # 三孔使用说明
│   ├── manual-five.html               # 五孔使用说明
│   ├── manual-seven.html              # 七孔使用说明
│   └── screenshots/                   # 01_home ~ 08_seven_result 共 8 张 SVG
├── pages/
│   ├── select/                        # 探针选择页（3/5/7 孔，含帮助入口）
│   ├── five/                          # 五孔工作区（已可用，标题栏含帮助入口）
│   ├── three/                         # 三孔工作区（已可用，标题栏含帮助入口）
│   ├── seven/                         # 七孔工作区（已可用，大小角度模式，标题栏含帮助入口）
│   └── help/                          # 使用说明页（三/五/七孔 tab 切换）
├── utils/
│   ├── algorithms/
│   │   ├── atmospheric.js             # 大气数据计算器（端口自 atmospheric_data.go）
│   │   ├── fivehole/
│   │   │   ├── prb-interpolator.js    # 五孔 PRB 9 区域法（端口自 prb_interpolator.go）
│   │   │   └── multi-prb-interpolator.js # 多马赫封装（端口自 multi_prb_interpolator.go）
│   │   ├── threehole/
│   │   │   └── three-hole.js          # 三孔迭代插值（端口自 three_hole.go）
│   │   ├── sevenhole/
│   │   │   └── seven-hole.js          # 七孔遍历插值（端口自 sevenhole/interpolation 全套）
│   │   └── index.js                   # 算法统一出口
│   └── prb-file.js                    # wx.chooseMessageFile 选择 .prb
│   ├── csv-batch.js                  # 纯 JS：CSV 解析 + 逐行批量插值（wx 无关，可 Node 校验）
│   ├── csv-file.js                   # wx.chooseMessageFile 选 CSV + 导出分享/存盘
│   └── units.js                      # 纯 JS：压力/速度/角度/温度单位换算（wx 无关，可 Node 校验）
│   ├── share-card.js                 # 纯 JS：分享卡片数据模型（buildCardModel，wx 无关可 Node 校验）
│   └── share-canvas.js               # wx：Canvas 2D 绘制 + 导出三级回退分享
└── verify/                            # 数值校验（不打包进小程序）
    ├── go.mod                         # replace → 共享 Go 算法包
    ├── genref*.go                     # 用 Go 原版生成参考（五/三/七孔）
    ├── reference*.json                # 生成的参考值（125 / 50 / 490 用例）
    ├── verify*.js                     # Node 跑 JS 端口逐字段对比
    └── run*.sh                        # 一键校验（run / run_three / run_seven）
```

## 移植策略

- **核心算法**：Go 纯数学代码逐函数移植为 CommonJS JS，确保小程序端（`require`）与 Node 校验端都能直接运行。
- **数值可信**：用 Go 原版生成黄金参考，Node 端跑同输入对比。五孔、三孔容差 1e-9；七孔因逐位移植精度极高，最大误差 ~1e-14（角度）、~1e-15（马赫），theta/phi/速度/总静压完全逐位一致。
- **校准数据**：复用桌面端 `.prb` 格式。五孔多马赫 `.prb` 由文件名/表头携带马赫；三孔每个 `.prb` 首行内嵌一个校准马赫数，可一次加载 1~10 个文件跨马赫数档迭代插值；七孔为 `7.prb`（内区）+ `1~6.prb`（外区扇区）固定马赫集。

## 运行小程序

1. 用**微信开发者工具**打开本目录（`miniprogramRoot` = 仓库根）。
2. AppID 选「测试号」即可（project.config.json 已配 `touristappid`）。
3. 进入「五孔探针」：填 5 个孔压 + 大气参数，点「选择 .prb」从会话选校准文件，点「计算」。
4. 进入「三孔探针」：填 3 个孔压 + 大气参数，上传 `.prb`（首行内嵌校准马赫数，可多文件），点「计算」，结果含偏角 α、马赫数、总压/静压、迭代次数。
5. 进入「七孔探针」：从会话选 7 个 `.prb`（7.prb 内区 + 1~6.prb 外区扇区），点「加载校准」，填 7 个孔压 + 大气参数，点「计算」。结果含侧滑角 α、迎角 β、θ/φ、马赫数、速度、总/静压；大小角度模式自动判定，超出校准网格不支持外推。

> 三孔、七孔页均已可用，与五孔同构、复用同一套 verify 数值校验。

## 显示单位切换

每个工作区顶部有「显示单位」卡片，可分别切换 **压力**（Pa/kPa/MPa）、**角度**（deg/rad）、**温度**（°C/K/°F）、**速度**（m/s/km/h）。

- **输入固定国际单位**（孔压 Pa、气温 °C），以保证 CSV 导入约定不变、插值计算零改动。
- 单位切换只作用于 **结果展示** 与 **CSV 导出结果列**：单点结果实时按新单位重新格式化；已导入的批量 CSV 会按新单位（精度无损地）重跑一次。
- 单位偏好存入 `app.js` 的 `globalData.units`，跨三/五/七孔页面共享。
- 切换后 CSV 表头的结果列自动加单位后缀，例如 `P0_kPa`、`velocity_km_h`、`sat_degC`、`theta_rad`。

## 批量 CSV 导入 / 导出

三/五/七孔工作区均内置「批量 CSV」卡片：把多组孔压一次性灌入插值，并把结果回写为 CSV。

**导入（计算）**
- 点「导入 CSV 并计算」，从微信会话选一个 `.csv` 文件。
- 列约定：必需 `P1..Pn`（n=孔数 3/5/7，单位 Pa）；可选 `Patm`/`TAtm`（缺省时回退到页面当前大气输入）。
  - 表头容错：支持 `P1 (Pa)`、`大气压`、`气温(℃)`、`Patm`、`TAtm` 等写法（忽略空格/括号/单位/中文）。
- 逐行调用与单点「计算」完全相同的插值器（已加载的校准），每行得到一组结果。
- 输出表头固定为：`P1..Pn, Patm, TAtm, <结果列>, isValid, warning`
  - 三孔结果列：`alpha, machNumber, P0, Ps`
  - 五孔结果列：`alpha, beta, machNumber, v, vx, vy, vz, cas, sat, dynamicPressure, density, P0, Ps`
  - 七孔结果列：`alpha, beta, theta, phi, machNumber, velocity, totalPressure, staticPressure, dynamicPressure`
  - `isValid = 1/0`；解析或计算失败的该行 `isValid = ERROR` 并在 `warning` 列写明原因（缺列、非法数值、内部异常）。

**导出（分享）**
- 点「导出 CSV」，把全部结果行写成 `probe_batch_<孔型>_<时间戳>.csv`。
- 写入 `wx.env.USER_DATA_PATH` 后，优先 `wx.shareFileMessage` 调起「分享到会话」（工程师可直接把结果发回聊天）；若不可用则回退 `wx.saveFileToDisk`（PC 端存盘）。

> CSV 解析按 RFC4180 风格（支持引号、`""` 转义、引号内逗号/换行、UTF-8 BOM 剥离）。批量逻辑在 `utils/csv-batch.js` 中与 `wx` 解耦，可在 Node 端用 `verify/verify_csv.js` 直接校验。

**结果复制**

- 单点「计算结果」卡片右上角有「复制」按钮，把当前结果（按显示单位）整理为制表符分隔的文本写入剪贴板，便于粘贴到微信/Excel/文档。

## 结果分享卡片

单点「计算结果」卡片右上角另有「分享卡片」按钮，把当前结果（含输入摘要、按显示单位的结果、有效性、警告、生成时间）绘制成一张图片，便于直接发到微信会话或存相册。

- 卡片内容由 `utils/share-card.js` 的 `buildCardModel`（纯 JS、可 Node 校验）构建；真正的 Canvas 绘制在 `utils/share-canvas.js`，仅在真机/开发者工具运行时执行。
- 导出与分享走三级回退：`wx.shareFileMessage`（分享到会话）→ `wx.saveImageToPhotosAlbum`（存相册）→ `wx.previewImage`（打开预览，可长按手动保存/分享），确保任意环境都能落到可用路径。
- 离屏 `<canvas type="2d" id="shareCanvas">` 放在各页 WXML 末尾，CSS 用 `.offscreen-canvas` 定位到屏幕外，不参与布局。

## 数值校验（开发用）

五孔：

```bash
cd projects/probe-interpolator-miniprogram/verify
GOWORK=off go run genref.go ../../../shared/algorithms/go/fivehole/interpolation/testdata/golden/prb reference.json
node verify.js
# 或 ./run.sh
```

预期输出：`PASS: 125  FAIL: 0`，JS 端口与 Go 原版逐位一致。

三孔：

```bash
cd projects/probe-interpolator-miniprogram/verify
GOWORK=off go run genref_three.go \
  ../../../shared/algorithms/go/threehole/interpolation/testdata/golden/threehole \
  reference_three.json \
  ../../../shared/algorithms/go/threehole/interpolation/testdata/0.8.prb
node verify_three.js
# 或 ./run_three.sh
```

预期输出：`PASS: 50  FAIL: 0`，JS 端口与 Go 原版逐位一致（容差 1e-9）。
用例覆盖：真实 `0.8.prb` 黄金用例（8）+ Kb 扫描（33）+ 多马赫合成双表线性插值（9）。

七孔：

```bash
cd projects/probe-interpolator-miniprogram/verify
GOWORK=off go run genref_seven.go \
  ../../../shared/algorithms/go/sevenhole/interpolation/testdata/prb \
  ../../../shared/algorithms/go/sevenhole/interpolation/testdata/golden/golden.json \
  ../../../shared/algorithms/go/sevenhole/interpolation/testdata/golden/boundary.json \
  reference_seven.json
node verify_seven.js
# 或 ./run_seven.sh
```

预期输出：`PASS: 490  FAIL: 0`，JS 端口与 Go 原版**逐位一致**（最大误差 alpha/beta ~1e-14、Ma ~1e-15，theta/phi/速度/总静压完全一致）。
用例覆盖：golden.json（481，含内区/外区/次候选回退/超网格 out 模式）+ boundary.json（9，边界与负表压）。

CSV 批量逻辑（不依赖小程序运行时）：

```bash
cd projects/probe-interpolator-miniprogram/verify
node verify_csv.js
```

预期输出：`PASS: 21  FAIL: 0`。覆盖：parseCsv/toCsv 往返（含引号/逗号/BOM/中文单位表头）、runBatch 的列匹配与缺列(ERROR)/异常(ERROR)/有效行分类、以及三/五/七孔 `resultColumns` 均为各自 reference `go` 输出键的子集（防止列名写错导致空白列）。

单位换算（不依赖小程序运行时）：

```bash
cd projects/probe-interpolator-miniprogram/verify
node verify_units.js
```

预期输出：`PASS: 52  FAIL: 0`。覆盖：压力/速度/角度/温度四类换算正确、formatValue 数值格式化与单位后缀、runBatch 的 `formatResultValue`/`resultColumnHeader` 单位回调（切换 kPa/MPa 后表头与值同步变化）、以及三/五/七孔全部结果列均有已知单位类型映射。

分享卡片模型（不依赖小程序运行时）：

```bash
cd projects/probe-interpolator-miniprogram/verify
node verify_share_card.js
```

预期输出：`PASS: 18  FAIL: 0`。覆盖：`buildCardModel` 的 probeType/inputs/results/isValid/warning 透传、单位值正确保留、`generatedAt` 时间格式、以及缺参边界归一。

## 分阶段计划

| 阶段 | 内容 | 状态 |
|---|---|---|
| 1 | 工程骨架 + 五孔 PRB 算法移植 + 数值校验 | ✅ 完成 |
| 2 | 五孔工作区页面（输入/上传/计算/结果） | ✅ 完成 |
| 3 | 三孔算法移植（shared/threehole） + 页面 | ✅ 完成 |
| 4 | 七孔算法移植（shared/sevenhole，含大小角度） + 页面 | ✅ 完成 |
| 5a | 批量 CSV 导入/导出（三/五/七孔通用，含 RFC4180 解析 + 分享/存盘） | ✅ 完成 |
| 5b | 单位切换（压力/角度/温度/速度）+ 结果复制（单点剪贴板） | ✅ 完成 |
| 5c | 结果分享卡片（Canvas 绘图 + 三级回退分享，三/五/七孔通用） | ✅ 完成 |
| 6 | 七孔校准 CSV 导入（端口自 csv_loader.go，与 .prb 校准等效） | ✅ 完成 |

每完成一种探针移植，都复用 `verify/` 的同款数值校验，保证与桌面端 Go 版一致。

## 代码审查修复（/code-review 后应用）

对已完成代码做结构化审查，发现 8 项（Critical 1 / Important 4 / Note 3）。已修复 5 项影响正确性与健壮性的问题：

- **#1（Critical）空孔压静默算 0 Pa**：三/五/七孔页 `onCalculate` 改用 `parseNumber`，空串/空白/非数字 → 报错返回（合法 0 仍允许），不再被 `Number('')` 静默转 0。
- **#2 `prb-file.js` 误用 `f.name` 作 `filePath`**：改为 `f.path`（真实临时文件路径）。
- **#3 批量摘要文案误导**：「按 0 处理会报错」改为「缺失列的行已标记为 ERROR」。
- **#4 CSV 非数值 Patm/TAtm 静默回退**：列存在但非数字时该行标记为 `ERROR`（附 `Patm/TAtm 非法数值` 提示），不再静默用默认值。
- **#5 `seven.js` clearFiles 未清 `_lastCsvText`**：清空校准时一并重置，避免 stale CSV 在切单位时被重跑。

Note #6–#8（canvas 文本溢出、全局 `createSelectorQuery`、density 无单位标签）为体验/健壮性注记，非阻塞，后续按需处理。

修复后全量回归：verify 六套共 **756 断言全 PASS**（五孔125 / 三孔50 / 七孔490 / CSV 21 / 单位 52 / 分享卡片 18）。

## 阶段 6：七孔校准 CSV 导入

七孔页除 `.prb` 校准外，新增「**导入校准 CSV**」入口，支持直接加载校准 CSV（1 份内区 + 6 份外区扇区），与 `.prb` 校准等效。移植自桌面 Go 版 `shared/algorithms/go/sevenhole/interpolation/csv_loader.go`。

- 新文件 `utils/algorithms/sevenhole/csv-loader.js`（`wx` 无关纯 JS，可 Node 校验）：
  - **按列位置读取**（col 0,1,12,13,14,15），不依赖表头名称（outer CSV 表头有历史遗留命名错误，故必须按位置）；必需 ≥16 列。
  - **退化边确定性抖动**：相邻网格点 ka/kb 相等导致 bilinear 不可逆时，施加 `1e-9 × nudge 计数` 偏移，最多 100 轮；仍有退化边则报错（避免后续 NaN/Inf）。
  - 外区 theta 网格派生（起点 30、步长 5 校验）→ 重建 PRB 行 → 复用既有的 `loadInnerPrbLines` / `loadOuterPrbLines`。
- 七孔页 `pages/seven/`：`onImportCalibrationCsv` 选 7 个 CSV → 按 basename 路由（inner/内区/7=内区，1~6=扇区，否则按选择顺序首=内区）→ `loadCalibrationCSV` → `this._interp`。退化边抖动等 warning 透出到状态栏。
- **编码**：以 UTF-8 读取；校准软件常见 GB18030 导出，但其数字列与逗号/换行为 ASCII，GB18030 不会把逗号/换行编入多字节序列，故数字列仍能正确切分解析，仅中文表头乱码（触发一条非致命 warning）。若数值列因编码异常无法解析，请用 Excel/记事本另存为 UTF-8。

数值校验（`verify/verify_seven_csv.js`）：用真实 PRB 测试数据反向构造校准 CSV → `loadCalibrationCSV` 得到的插值器，与 PRB 直载插值器**逐位一致（最大误差 0）**，且与 Go 参考一致；另含退化边抖动单测。

```bash
cd projects/probe-interpolator-miniprogram/verify
node verify_seven_csv.js
```

预期输出：`PASS: 1384  FAIL: 0`（490 黄金用例 × 多断言 + 抖动单测），`CSV 与 PRB 直载最大误差` 全字段 `0.00e+0`。
