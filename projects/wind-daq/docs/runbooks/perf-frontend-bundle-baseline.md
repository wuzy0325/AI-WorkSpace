# Frontend Bundle 性能基线与修复方案

> 测量日期：2026-06-23
> 来源：`apps/desktop-wails/frontend/dist/assets/`（已构建产物）
> 方法：`gzip -c | wc -c` 直接测量；entry preload 链路通过 `dist/index.html` 静态分析
> 后续工作流：MEASURE（已完成）→ IDENTIFY（已完成）→ FIX（待执行）→ VERIFY → GUARD

---

## 1. 测量基线（MEASURE）

### 1.1 总体体积

| 指标 | 实测 | 性能 budget（来自 performance-optimization skill）| 状态 |
|---|---|---|---|
| 总 JS（raw） | 2 217 KB | — | — |
| 总 JS（gzipped） | **691 KB** | < 200 KB（初始加载） | ❌ 超 3.5× |
| 总 CSS（gzipped） | 28 KB | < 50 KB | ✅ |
| 首屏入口 `index-*.js`（gzipped） | 80 KB | — | — |
| 首屏 `modulepreload` echarts（gzipped） | 407 KB | — | — |
| **首屏实际下载 JS** | **~487 KB gzip / ~1 476 KB raw** | < 200 KB | ❌ |

### 1.2 Top 10 JS chunk

| 文件 | RAW (KB) | GZIP (KB) | 备注 |
|---|---:|---:|---|
| `echarts-*.js` | 1 198.0 | **406.5** | manualChunks 单独成块；首屏 modulepreload |
| `MainDashboardView-*.js` | 331.4 | 94.2 | 首屏路由组件 |
| `index-*.js` | 277.6 | 79.6 | App entry |
| `UiCheckbox.vue_*.js` | 121.5 | 35.5 | naive-ui 共享 chunk |
| `i18nStore-*.js` | 77.2 | 24.3 | 多语言资源 |
| `FiveHoleMain-*.js` | 29.0 | 8.8 | 路由 lazy chunk ✅ |
| `FiveHoleSettings-*.js` | 21.1 | 6.9 | 路由 lazy chunk ✅ |
| `MotionView-*.js` | 21.0 | 6.2 | 路由 lazy chunk ✅ |
| `ThreeHoleMain-*.js` | 20.2 | 5.5 | 路由 lazy chunk ✅ |
| `MotionControllerConfig-*.js` | 19.3 | 6.3 | 路由 lazy chunk ✅ |

**注**：除前 5 项以外，其余均已按路由分包，体积合理。问题集中在 echarts 与 entry 两块。

---

## 2. 瓶颈定位（IDENTIFY）

### 2.1 P0：echarts 整包导入 + 首屏 preload

**证据 A —— 整包导入**

`src/components/traversal/visualization/composables/useECharts.ts:2`

```ts
import * as echarts from 'echarts'
```

这一行禁用了 echarts 自身的 tree-shaking 能力，把图表类型、渲染器、组件、所有特性全部拉进 bundle，1.2 MB raw / 407 KB gzip 即由此产生。

对比 `src/components/device/RealtimeChart.vue` 的正确写法：

```ts
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, DataZoomComponent } from 'echarts/components'
```

**证据 B —— 首屏 preload**

`dist/index.html:8`：

```html
<link rel="modulepreload" crossorigin href="/assets/echarts-BUdWWCzo.js">
```

`vite.config.ts:33-36` 用 `manualChunks` 单独分出 echarts 块，但因为 `useECharts.ts` 在静态导入链中被 entry 间接引用，Vite 会自动给它打 `modulepreload`。结果是：用户进入任何首屏（含没有图表的页面）都会下载、解析、编译这 1.2 MB JS。

**影响**：
- 首屏加载 +约 1.5 秒（按 4G 网络 + 中端硬件估算，仅 JS 下载 + 解析）
- Wails 桌面应用启动时主进程也要等前端 JS 解析完成，体感卡顿
- 中低端工业电脑（标定现场）受影响更明显

### 2.2 P1：`MainDashboardView` 332 KB raw / 94 KB gzip

首屏路由组件单体偏大。需要进一步用 `rollup-plugin-visualizer` 拆解：
- 是 echarts type 二次引入？
- 还是有重逻辑可下沉成子组件 lazy load？

**目前数据不足以下定论**，需要先生成 treemap 报告再说。

### 2.3 P2：未启用 CSP/Bundle CI 守护

没有自动化的 budget 检查，下次有人 `import * as` 任何大库都会再次出现这类问题。`performance-optimization` skill 明确要求 **GUARD** 步骤。

---

## 3. 修复方案（FIX）

### 3.1 修复 #1：echarts 按需导入

**改 `src/components/traversal/visualization/composables/useECharts.ts`**

```ts
import { onBeforeUnmount, onMounted, shallowRef, type Ref } from 'vue'
import { use, init, type ECharts } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
// 只导入 traversal 可视化实际用到的图表类型与组件
import { LineChart, ScatterChart, HeatmapChart, CustomChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  VisualMapComponent,
  TitleComponent,
  LegendComponent,
  DataZoomComponent,
} from 'echarts/components'

use([
  CanvasRenderer,
  LineChart, ScatterChart, HeatmapChart, CustomChart,
  GridComponent, TooltipComponent, VisualMapComponent,
  TitleComponent, LegendComponent, DataZoomComponent,
])

// ... 其余逻辑保持不变
```

**注意事项**：
- 需要先审计 `CrossSectionView.vue` / `HeatmapView.vue` / `PressureRadarView.vue` / `VectorFieldView.vue` 真正使用的 series 类型，按需补全 `use([...])` 列表
- 漏注册会**运行时报错**（`series type not registered`），必须在改完后跑 traversal 路由的烟雾测试
- `import type { EChartsOption } from 'echarts'` 这种 type-only 导入不影响体积，可保留

**预期效果**：echarts chunk 从 407 KB gzip 降至 80–120 KB gzip（参考 vue-echarts 官方按需示例数据）

### 3.2 修复 #2：echarts chunk 不再首屏 preload

修复 #1 完成后，traversal 路由若已经是 lazy，echarts 自动只在进入该路由时下载。如果发现 `useECharts` 被某个**非 lazy** 路由静态引用，需要：

1. 把对应路由也改成 lazy import：`component: () => import('@views/Foo.vue')`
2. 或者在 `vite.config.ts` 把 echarts 从 `manualChunks` 拿掉，让它自然分到 lazy chunk

**验证方法**：
- `npm run build` 后查看新 `dist/index.html`，echarts 应**不再**出现在 `modulepreload`
- 也可以加 `import.meta.glob` 看 router 配置确认

### 3.3 修复 #3：MainDashboardView 拆解

```bash
# 先生成 treemap 找拆分点
npm i -D rollup-plugin-visualizer
# vite.config.ts 加 visualizer() 插件
npm run build
```

看 stats.html → 找出可 `defineAsyncComponent(() => import(...))` 的子组件（设备管理抽屉、计算结果面板等不需要首屏立刻渲染的）。

### 3.4 修复 #4：CI Bundle Budget

新增 `frontend/bundlesize.config.json`：

```json
{
  "files": [
    {
      "path": "./dist/assets/index-*.js",
      "maxSize": "100 kB",
      "compression": "gzip"
    },
    {
      "path": "./dist/assets/echarts-*.js",
      "maxSize": "150 kB",
      "compression": "gzip"
    }
  ]
}
```

`package.json` 加 `"size": "bundlesize"`，在 CI workflow 里调用。

---

## 4. 验证步骤（VERIFY）

每完成一个修复，按以下顺序验证：

```bash
cd apps/desktop-wails/frontend
npm run build
# 1. 重新测量
du -bc dist/assets/*.js | tail -1
cat dist/assets/echarts-*.js | gzip -c | wc -c
# 2. 检查 index.html preload 列表
cat dist/index.html
# 3. 运行 traversal 烟雾测试（人工或 vitest 路由测试）
npm run test
# 4. Wails 启动验证（中低端机器更敏感）
cd ../.. && wails dev
```

**通过标准**：
- [ ] 首屏 JS gzip ≤ 200 KB
- [ ] 总 JS gzip ≤ 400 KB
- [ ] traversal 路由 4 个可视化视图渲染正常（CrossSection / Heatmap / PressureRadar / VectorField）
- [ ] `RealtimeChart` 在 MainDashboard 渲染正常
- [ ] 现有 vitest 全部通过

---

## 5. 防回归（GUARD）

- [ ] CI 跑 `bundlesize` 检查
- [ ] 在 `docs/runbooks/code-standards.zh-CN.md`（或前端规则补充）加入：
  - 禁止 `import * as echarts`，必须按需导入
  - 重型库（>50 KB gzip）默认 lazy import
- [ ] 半年复测一次，更新本文档基线

---

## 附录 A：测量复现命令

```bash
# 在仓库根目录运行
DIST=projects/wind-daq/apps/desktop-wails/frontend/dist/assets
cd "$DIST"

# 单文件 raw + gzip
for f in *.js; do
  raw=$(stat -c%s "$f")
  gz=$(gzip -c "$f" | wc -c)
  echo "$raw $gz $f"
done | sort -rn | head -10 | awk '{
  printf "%-55s %8.1fK %8.1fK\n", $3, $1/1024, $2/1024
}'

# 总和
echo "JS  total gzip: $(cat *.js | gzip -c | wc -c) bytes"
echo "CSS total gzip: $(cat *.css | gzip -c | wc -c) bytes"
```

## 附录 B：执行前需澄清的问题

1. echarts 修复涉及运行时报错风险，是否允许我**先做修复 #1**并在本机跑 traversal 烟雾测试再交给你？
2. 是否有现成的 traversal 可视化最小测试用例？目前 `tests/` 下需要确认
3. Bundle budget 数值（200 KB / 400 KB）是否符合现场工业电脑的网络/磁盘约束？这是 skill 默认值，可调整
