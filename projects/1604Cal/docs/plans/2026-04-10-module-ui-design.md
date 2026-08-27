# 模块UI照搬设计方案

**日期**: 2026-04-10  
**主题**: 计量模块与标定模块界面照搬  
**状态**: 已确认

---

## 1. 项目背景

本项目需要整合两个旧系统的UI界面：
1. **计量模块（1605MeassureApp）** - 单设备校准和数据采集
2. **标定模块（1604标定软件）** - 完整的1604标定流程

采用**完全照搬**策略，保持两个模块独立，最小化改动风险。

---

## 2. 整体设计决策

### 2.1 主题风格
- **选择**: 深色主题
- **配色方案**:
  - 背景色: `#1a1a2e`（深蓝黑）
  - 卡片背景: `#16213e`（深蓝）
  - 主色调: `#0f3460`（蓝）
  - 强调色: `#e94560`（红/粉）
  - 文字: `#ffffff`（白）、`#a0a0a0`（灰）

### 2.2 技术栈
- Vue 3 + TypeScript
- Element Plus UI组件库
- Pinia 状态管理
- Vue Router 路由管理
- SCSS/CSS变量主题系统

### 2.3 路由结构
```
/                    → 首页（模块选择）
/measurement         → 计量工作台
/multi-pressure      → 多设备打压
/calibration         → 标定工作台
```

---

## 3. 页面详细设计

### 3.1 首页（HomeView.vue）

**布局**: 左侧边栏导航 + 右侧主内容区

**侧边栏**:
- Logo区域: 1604校准系统标识
- 导航菜单:
  - 首页
  - 计量工作台
  - 多设备打压
  - 标定工作台
- 系统状态指示器

**主内容区**:
- 标题栏: 欢迎使用1604校准系统
- 功能入口卡片（3个）:
  1. **计量工作台**: 单设备校准和数据采集，支持自动/手动模式
  2. **多设备打压**: 并行控制多个打压设备，独立压力设定
  3. **标定工作台**: 完整的1604标定流程，6步向导式操作

---

### 3.2 计量工作台（CalibrationView.vue）

**布局**: 可折叠侧边栏 + 主工作区

**侧边栏**（可折叠）:
- 设备管理面板
  - 打压设备列表（PressureDeviceCard组件）
  - 计量设备列表（MeasureDeviceCard组件）
- 添加设备按钮

**主工作区**:

**第一行控制条**（校准参数设置）:
| 控件 | 类型 | 说明 |
|-----|------|------|
| 最小值/最大值 | 数字输入 | 校准范围 |
| 点数 | 数字输入(2-50) | 校准点数量 |
| 精度 | 数字输入 | 小数位数 |
| 平均数 | 数字输入 | 采集平均次数 |
| 稳定时间 | 下拉选择 | 1秒/3秒/5秒/10秒 |
| 精度Level | 下拉选择 | 0.01%-0.2%或自定义 |
| 生成压力表 | 按钮 | 生成校准点序列 |

**第二行控制条**（模式选择和操作）:
| 控件 | 类型 | 说明 |
|-----|------|------|
| 控制模式 | 切换按钮 | 自动/手动 |
| 打压模式 | 切换按钮 | 单程/回程 |
| 进度显示 | 进度条 | 当前点/总点数 |
| 稳定状态 | 状态标签 | 稳定中/已稳定 |
| 稳定倒计时 | 倒计时显示 | 剩余稳定时间 |
| 报警设置 | 开关组 | 启用/声音/确认/通道 |
| 操作按钮 | 按钮组 | 开始/暂停/恢复/停止/重置/导出 |

**数据表格区**:
- 16通道数据展示
- 列: 序号、状态、目标值、16个通道值、时间
- 行状态标记:
  - 待执行（灰色）
  - 打压中（蓝色）
  - 稳定中（黄色）
  - 采集中（橙色）
  - 已完成（绿色）
- 颜色标记:
  - 绿色: 合格
  - 黄色: 警告
  - 红色: 超差

---

### 3.3 多设备打压（PressureWorkbenchView.vue）

**布局**: 顶部工具栏 + 设备卡片网格（双列）

**工具栏**:
- 返回首页按钮
- 页面标题: 多设备打压控制
- 统计芯片:
  - 设备总数
  - 在线设备数
- 刷新状态按钮
- 添加设备按钮

**设备卡片网格**（双列布局）:
每张卡片包含:
- 设备名称（顶部）
- 连接状态指示灯（右上角）
- 当前压力值（大号居中显示）
- 设定压力输入框
- 单位选择下拉框（kPa/MPa/bar/psi）
- 连接/断开按钮
- 设定压力按钮

---

### 3.4 标定工作台（MainView.vue）

**布局**: 四行网格布局（最大宽度1600px居中）

**第一行**（3列，比例5:3:4）:

| 组件 | 功能 |
|------|------|
| **进度指示器** | 6步骤向导显示：设备连接→通道选择→开始校准→数据采集→数据拟合→完成 |
| **1604设备面板** | IP输入、连接/断开按钮、阀门状态、阀门控制按钮 |
| **打压设备面板** | IP+端口输入、连接按钮、升压/降压按钮、设定压力、当前压力 |

**第二行**（2列，比例7:5）:

| 组件 | 功能 |
|------|------|
| **通道选择矩阵** | 16通道复选框网格（默认16列，响应式8/4列）、已选计数、全选/清空按钮 |
| **校准控制面板** | 开始校准按钮（大号主要）、准备状态提示列表、数据拟合按钮、结束校准按钮 |

**第三行**（全宽）:
- **压力点列表**:
  - 表头: 序号、状态、目标压力、打压/确认、采集、操作
  - 列表项:
    - 序号显示
    - 状态标签（待打压/待确认/待采集/完成）
    - 目标压力输入框
    - 打压按钮（自动）/确认按钮（手动）
    - 采集按钮/重新采集按钮
    - 删除按钮
  - 视觉: 不同状态不同左边框颜色

**第四行**（全宽，条件显示）:
- **数据表格**:
  - 表头: 压力点、目标压力、已选通道列（动态）、状态
  - 行数据: 每个压力点的采集数据
  - 状态列: 已采集/待采集
  - 导出按钮: CSV格式导出

---

## 4. 组件清单

### 4.1 公共组件

| 组件名 | 用途 | 位置 |
|--------|------|------|
| Sidebar.vue | 侧边栏导航 | components/common/ |
| StatCard.vue | 统计数字卡片 | components/common/ |
| DeviceStatusBadge.vue | 设备状态标签 | components/common/ |

### 4.2 计量模块组件

| 组件名 | 用途 | 位置 |
|--------|------|------|
| DevicePanel.vue | 可折叠设备面板 | components/measurement/ |
| PressureDeviceCard.vue | 打压设备卡片 | components/measurement/ |
| MeasureDeviceCard.vue | 计量设备卡片 | components/measurement/ |
| CalibrationControlBar.vue | 校准参数控制条 | components/measurement/ |
| DataTable16Ch.vue | 16通道数据表格 | components/measurement/ |

### 4.3 标定模块组件

| 组件名 | 用途 | 位置 |
|--------|------|------|
| ProgressIndicator.vue | 步骤进度指示器 | components/calibration/ |
| Device1604Panel.vue | 1604设备连接面板 | components/calibration/ |
| PressDevicePanel.vue | 打压设备控制面板 | components/calibration/ |
| ChannelMatrix.vue | 16通道选择矩阵 | components/calibration/ |
| CalibrationControlPanel.vue | 校准控制按钮面板 | components/calibration/ |
| PressurePointList.vue | 压力点列表 | components/calibration/ |
| CalibrationDataTable.vue | 校准数据表格 | components/calibration/ |

---

## 5. 状态管理设计

### 5.1 Store结构

```
stores/
├── measurement/          # 计量模块状态
│   ├── deviceStore.ts   # 设备管理（打压设备、计量设备）
│   ├── calibrationStore.ts # 校准流程状态（参数、进度、模式）
│   └── dataStore.ts     # 采集数据（16通道数据、表格数据）
├── calibration/          # 标定模块状态
│   ├── deviceStore.ts   # 设备连接（1604、打压设备）
│   ├── channelStore.ts  # 通道选择（16通道选中状态）
│   ├── processStore.ts  # 标定流程（当前步骤、压力点列表）
│   └── dataStore.ts     # 标定数据（采集结果、拟合数据）
└── common/
    └── appStore.ts      # 全局状态（主题、导航、系统配置）
```

### 5.2 关键状态定义

**计量模块**:
```typescript
// 设备状态
interface Device {
  id: string;
  name: string;
  type: 'pressure' | 'measurement';
  ip: string;
  port: number;
  status: 'connected' | 'disconnected' | 'error';
  currentPressure?: number;
}

// 校准参数
interface CalibrationParams {
  minValue: number;
  maxValue: number;
  points: number;
  precision: number;
  averageCount: number;
  stableTime: number; // 秒
  precisionLevel: string;
}

// 数据行
interface DataRow {
  index: number;
  status: 'pending' | 'pressurizing' | 'stabilizing' | 'collecting' | 'completed';
  targetValue: number;
  channelValues: number[]; // 16个通道值
  timestamp: Date;
}
```

**标定模块**:
```typescript
// 校准步骤
enum CalibrationStep {
  DEVICE_CONNECT = 0,
  CHANNEL_SELECT = 1,
  START_CALIBRATION = 2,
  DATA_COLLECTION = 3,
  DATA_FITTING = 4,
  COMPLETED = 5
}

// 压力点
interface PressurePoint {
  id: string;
  index: number;
  targetPressure: number;
  status: 'pending_press' | 'pending_confirm' | 'pending_collect' | 'completed';
  collectedData?: number[]; // 已选通道的采集值
}

// 通道选择
interface ChannelSelection {
  selectedChannels: boolean[]; // 16个布尔值
  selectedCount: number;
}
```

---

## 6. 样式系统设计

### 6.1 CSS变量定义

```css
:root {
  /* 背景色 */
  --bg-primary: #1a1a2e;
  --bg-secondary: #16213e;
  --bg-tertiary: #0f3460;
  
  /* 强调色 */
  --accent-primary: #e94560;
  --accent-secondary: #533483;
  
  /* 文字 */
  --text-primary: #ffffff;
  --text-secondary: #a0a0a0;
  --text-muted: #666666;
  
  /* 状态色 */
  --status-success: #10b981;
  --status-warning: #f59e0b;
  --status-error: #ef4444;
  --status-info: #3b82f6;
  
  /* 边框 */
  --border-color: rgba(255, 255, 255, 0.1);
  
  /* 间距 */
  --spacing-xs: 4px;
  --spacing-sm: 8px;
  --spacing-md: 16px;
  --spacing-lg: 24px;
  --spacing-xl: 32px;
  
  /* 圆角 */
  --radius-sm: 4px;
  --radius-md: 8px;
  --radius-lg: 12px;
}
```

### 6.2 组件样式规范

**卡片样式**:
```scss
.card {
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: var(--spacing-md);
}
```

**按钮样式**:
```scss
.btn-primary {
  background: linear-gradient(135deg, var(--accent-primary), var(--accent-secondary));
  color: var(--text-primary);
  border: none;
  border-radius: var(--radius-sm);
  padding: var(--spacing-sm) var(--spacing-md);
}
```

**表格样式**:
```scss
.data-table {
  background: var(--bg-secondary);
  border-radius: var(--radius-md);
  
  th {
    background: var(--bg-tertiary);
    color: var(--text-secondary);
    font-weight: 500;
  }
  
  td {
    border-bottom: 1px solid var(--border-color);
  }
  
  tr.completed {
    background: rgba(16, 185, 129, 0.1);
  }
}
```

---

## 7. 错误处理策略

### 7.1 设备连接失败
- **表现**: 显示红色错误消息，提供重试按钮
- **日志**: 记录错误详情到控制台
- **恢复**: 用户点击重试后重新尝试连接

### 7.2 数据采集异常
- **表现**: 标记异常行（红色背景），显示错误提示
- **处理**: 允许用户重新采集该点数据
- **日志**: 记录异常详情

### 7.3 网络断开
- **表现**: 全局提示（顶部横幅），显示"网络已断开"
- **恢复**: 尝试自动重连（最多3次），成功后自动恢复
- **兜底**: 提供手动刷新按钮

---

## 8. 测试策略

### 8.1 组件测试
重点测试交互组件:
- 设备卡片（连接/断开、压力设定）
- 控制按钮（开始/暂停/停止）
- 通道选择矩阵（全选/清空）
- 进度指示器（步骤切换）

### 8.2 集成测试
完整流程测试:
- **计量流程**: 连接设备 → 设置参数 → 开始校准 → 数据采集 → 导出报告
- **多设备打压**: 添加多个设备 → 并行设定压力 → 监控状态
- **标定流程**: 6步完整标定流程（从设备连接到数据拟合）

### 8.3 视觉回归测试
- 深色主题样式验证
- 响应式布局测试（不同屏幕尺寸）
- 状态颜色一致性检查

---

## 9. 实施注意事项

### 9.1 照搬原则
1. **保持布局一致**: 严格按照旧模块的布局结构
2. **保留交互逻辑**: 按钮操作、状态切换逻辑不变
3. **样式复刻**: 使用CSS变量精确还原深色主题
4. **组件独立**: 两个模块的组件相互独立，避免耦合

### 9.2 代码组织
```
src/
├── views/              # 页面视图
│   ├── HomeView.vue
│   ├── measurement/
│   │   ├── CalibrationView.vue
│   │   └── PressureWorkbenchView.vue
│   └── calibration/
│       └── MainView.vue
├── components/
│   ├── common/         # 公共组件
│   ├── measurement/    # 计量模块组件
│   └── calibration/    # 标定模块组件
├── stores/
│   ├── measurement/    # 计量模块状态
│   ├── calibration/    # 标定模块状态
│   └── common/         # 公共状态
├── styles/
│   ├── variables.scss  # CSS变量定义
│   ├── dark-theme.scss # 深色主题
│   └── components/     # 组件样式
└── router/
    └── index.ts        # 路由配置
```

### 9.3 依赖清单
```json
{
  "dependencies": {
    "vue": "^3.4.x",
    "vue-router": "^4.2.x",
    "pinia": "^2.1.x",
    "element-plus": "^2.5.x",
    "@element-plus/icons-vue": "^2.3.x"
  },
  "devDependencies": {
    "sass": "^1.70.x",
    "typescript": "^5.3.x",
    "vitest": "^1.2.x",
    "@vue/test-utils": "^2.4.x"
  }
}
```

---

## 10. 验收标准

- [ ] 首页显示3个功能入口卡片，导航正常
- [ ] 计量工作台界面与旧模块一致，所有控件可用
- [ ] 多设备打压界面支持添加多个设备卡片，压力设定正常
- [ ] 标定工作台6步流程完整，通道选择、压力点列表功能正常
- [ ] 深色主题样式统一，无视觉问题
- [ ] 所有页面响应式适配正常

---

## 11. 后续优化方向（V2）

1. **提取公共组件**: 识别两个模块的公共UI组件，提取到共享库
2. **统一设备管理**: 两个模块共用设备管理逻辑
3. **数据导出增强**: 支持更多格式（PDF、Excel）
4. **主题切换**: 支持浅色/深色主题切换
5. **移动端适配**: 优化移动端交互体验

---

**文档版本**: v1.0  
**编写者**: AI Assistant  
**审核状态**: 待实施
