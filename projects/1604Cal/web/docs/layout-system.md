# 布局与间距系统规范

## 间距系统

本项目使用 **4pt 基准** 的间距系统，提供以下标准变量：

```scss
$spacing-0: 0;
$spacing-1: 4px;
$spacing-2: 8px;
$spacing-3: 12px;
$spacing-4: 16px;
$spacing-5: 20px;
$spacing-6: 24px;
$spacing-8: 32px;
$spacing-10: 40px;
$spacing-12: 48px;
$spacing-16: 64px;
$spacing-20: 80px;
$spacing-24: 96px;
```

### 语义化别名（向后兼容）

```scss
$spacing-xs: $spacing-1;  // 4px
$spacing-sm: $spacing-2;  // 8px
$spacing-md: $spacing-4;  // 16px
$spacing-lg: $spacing-6;  // 24px
$spacing-xl: $spacing-8;  // 32px
```

## 视觉节奏原则

### 紧凑分组 vs 宽松分离

- **紧凑分组（8-16px）**：用于相关内容之间的间距
  - 表单字段之间
  - 按钮组内部
  - 卡片内的元素

- **宽松分离（32-48px）**：用于不同区块之间的间距
  - 页面区块之间
  - 主要内容区域
  - 侧边栏与主内容区

### 页面结构层次

```
PageLayout (页面容器)
├── padding: $spacing-6 (32px)
│
├── PageHeader (页面头部)
│   ├── padding-bottom: $spacing-4
│   └── margin-bottom: $spacing-6
│
├── Section 1 (内容区块)
│   ├── margin-bottom: $spacing-8 或 $spacing-10
│   └── 内部使用 gap: $spacing-4 ~ $spacing-6
│
└── Section 2 (内容区块)
    └── ...
```

## 布局工具类

### 工作台布局

```html
<!-- 标准工作台布局（侧边栏 + 主内容） -->
<div class="workbench-layout">
  <Sidebar />
  <main class="workbench-content">
    <!-- 内容 -->
  </main>
</div>
```

### 卡片网格

```html
<!-- 标准卡片网格 -->
<div class="card-grid">
  <div class="feature-card">...</div>
  <div class="feature-card">...</div>
</div>

<!-- 小型卡片网格 -->
<div class="card-grid-sm">
  <div class="stat-card">...</div>
  <div class="stat-card">...</div>
</div>
```

### 行布局

```html
<!-- 紧凑行（按钮组等） -->
<div class="row-compact">
  <button>操作1</button>
  <button>操作2</button>
</div>

<!-- 紧凑行（对齐元素） -->
<div class="row-tight">
  <span>标签</span>
  <input />
</div>
```

### 页面区块

```html
<!-- 标准页面区块 -->
<section class="page-section">
  <h3>区块标题</h3>
  <!-- 内容 -->
</section>

<!-- 大间距页面区块 -->
<section class="page-section-lg">
  <h3>区块标题</h3>
  <!-- 内容 -->
</section>
```

## 组件使用规范

### PageHeader 组件

统一使用 PageHeader 组件确保页面头部一致性：

```vue
<template>
  <PageLayout>
    <PageHeader
      title="页面标题"
      subtitle="页面副标题"
      @back="goBack"
    >
      <template #actions>
        <!-- 操作按钮/状态标签 -->
      </template>
    </PageHeader>
    
    <!-- 页面内容 -->
  </PageLayout>
</template>
```

### 避免的问题

1. **不要混合使用 px 和 SCSS 变量**
   ```scss
   // ❌ 错误
   padding: 16px;
   
   // ✅ 正确
   padding: $spacing-4;
   ```

2. **不要使用相等的间距 everywhere**
   ```scss
   // ❌ 错误 - 没有节奏感
   gap: $spacing-4; // 到处都用 16px
   
   // ✅ 正确 - 有节奏感
   gap: $spacing-6; // 区块之间
   gap: $spacing-3; // 相关元素之间
   ```

3. **不要嵌套卡片**
   ```html
   <!-- ❌ 错误 -->
   <el-card>
     <el-card>...</el-card>
   </el-card>
   
   <!-- ✅ 正确 - 使用间距分隔 -->
   <el-card>
     <div class="section" style="margin-bottom: $spacing-4;">
       ...
     </div>
     <div class="section">
       ...
     </div>
   </el-card>
   ```

## 响应式断点

```scss
// 移动端适配
@media (max-width: 768px) {
  // 垂直堆叠布局
  // 减少间距
  // 单列网格
}
```

## 视觉层次检查清单

使用 **Squint Test**（眯眼测试）：

- [ ] 眯起眼睛后，能识别出最重要的元素
- [ ] 次要元素也有清晰的层次
- [ ] 相关元素通过间距自然分组
- [ ] 不同区块之间有清晰的分隔
- [ ] 整体有视觉节奏感（紧-松-紧-松）
