# Spec: 遍历测试·自定义点模式 4 轴扩展 + TXT/CSV 点位导入

> 来源：interview-me → 确认意图
> 日期：2026-07-10
> 状态：已确认

## Objective

**目标**：遍历测试·自定义点(Custom)模式从当前 2D (X/Y) 扩展到最多 4 轴 (X/Y/Z/U)，支持从 TXT/CSV 文件批量导入点位列表（替换当前列表）；配置预览画布加轴对选择器替代硬编码 X-Y 坐标轴。

**用户**：wind-daq 操作员，配置遍历测试时通过自定义点模式录入/导入最多 4 轴点位坐标，利用位移机构 X/Y/Z/U 全 4 轴能力。

**背景**：

- [shared/device-sdk/go/motion/core/types.go:7-11](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/motion/core/types.go#L7-L11) 已定义 `AxisX / AxisY / AxisZ / AxisU` 四个轴常量
- [core/traversal/types.go:36-40](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/services/api-go/internal/core/traversal/types.go#L36-L40) `Point` 已有 X/Y/Z 三轴，但 U 缺失
- [core/traversal/path.go:289-295](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/services/api-go/internal/core/traversal/path.go#L289-L295) `CustomLayout.Points` 只有 X/Y
- [usecase/traversal_helpers.go:82-95](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/services/api-go/internal/usecase/traversal_helpers.go#L82-L95) `availableAxisTargets` 只映射 X/Y/Z
- [PointsPreview.vue](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/apps/desktop-wails/frontend/src/components/traversal/PointsPreview.vue) 硬编码 X-Y 坐标轴，bounds/tags/animation 均假设 2D
- [traversal.ts:18-21](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/apps/desktop-wails/frontend/src/shared/types/traversal.ts#L18-L21) `TraversalPoint` 只有 `{ x, y }`
- [traversal.ts:124-126](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/apps/desktop-wails/frontend/src/shared/types/traversal.ts#L124-L126) `TraversalLayout.custom.points` 只有 `{ x, y }`

**成功画面**：

1. 前端 CustomLayout 编辑面板每行编辑 X/Y/Z/U 四坐标
2. 导入按钮→选 TXT/CSV→按列名匹配解析→替换 customPoints 列表
3. 文件首行列名匹配（大小写不敏感，归一化规则见 §导入列名归一化），分隔符自适应（逗号→CSV，空白/Tab→TXT），缺列按 0 填充
4. PointsPreview 画布加横轴/纵轴下拉选择器，默认 X-Y
5. 后端 `Point` 加 U 字段，`CustomLayout.Points` 加 u 字段，`availableAxisTargets` 加 U 轴映射
6. 配置保存/加载/恢复全链路携带 4 轴数据
7. `go build/test/vet` + `npm typecheck/build` 全绿

## Tech Stack

| 层 | 技术 | 版本 |
|---|---|---|
| 后端 | Go | 1.25（go.work 主干） |
| 前端 | Vue 3 + TypeScript + Vite + Naive UI | 与 wind-daq 现有 |
| 运动控制 | `shared/device-sdk/go/motion/core` | 已有，AxisU 已定义 |

## Commands

```powershell
# Backend
cd projects\wind-daq\services\api-go
go build -buildvcs=false ./...
go vet ./...
go test ./internal/...

# Frontend
cd projects\wind-daq\apps\desktop-wails\frontend
npm run typecheck
npm run build
```

## Architecture Changes

### A1. Point 结构体加 U 字段

位置：`internal/core/traversal/types.go:36-40`

```go
// Point 遍历测试点坐标（4 轴：X/Y/Z/U，对应位移机构全轴能力）
type Point struct {
    X float64 `json:"x"`
    Y float64 `json:"y"`
    Z float64 `json:"z"`
    U float64 `json:"u"`
}
```

`U float64` 零值为 0，旧配置文件无 u 字段时 Go JSON 反序列化自动填 0，向后兼容。

### A2. CustomLayout 点结构加 u 字段

位置：`internal/core/traversal/path.go:289-295`

```go
type CustomLayout struct {
    Points []struct {
        X float64 `json:"x"`
        Y float64 `json:"y"`
        // Z 和 U 新增，零值语义：无此字段的旧配置自动填 0
        Z float64 `json:"z"`
        U float64 `json:"u"`
    } `json:"points"`
}
```

注意：当前 `PointsFromLayout` custom 分支只传 X/Y（见 path.go 对应 switch case），需同步带上 Z/U。

### A3. availableAxisTargets 加 U 轴映射

位置：`internal/usecase/traversal_helpers.go:82-95`

```go
func availableAxisTargets(status motion.ControllerStatus, point traversal.Point) map[motion.AxisName]float64 {
    targets := make(map[motion.AxisName]float64, len(status.Axes))
    for _, axis := range status.Axes {
        switch axis.Name {
        case motion.AxisX:
            targets[axis.Name] = point.X
        case motion.AxisY:
            targets[axis.Name] = point.Y
        case motion.AxisZ:
            targets[axis.Name] = point.Z
        case motion.AxisU:   // 新增
            targets[axis.Name] = point.U
        }
    }
    return targets
}
```

仅当控制器 profile 实际配置了 U 轴（`status.Axes` 包含 `AxisU`）时才生效，无 U 轴时跳过。

### A4. traversalAPIConfig 加 u 字段

位置：`internal/usecase/traversal_config.go:335-340`

```go
Custom: &struct {
    Points []struct {
        X float64 `json:"x"`
        Y float64 `json:"y"`
        Z float64 `json:"z"`   // 新增
        U float64 `json:"u"`   // 新增
    } `json:"points"`
} `json:"custom"`
```

`ParseAndStartTraversal` 的 Custom 映射（traversal_config.go:462-474）同步添加 Z/U。

### A5. 前端类型扩展

位置：`shared/types/traversal.ts`

```typescript
export interface TraversalPoint {
  x: number
  y: number
  z: number  // 新增
  u: number  // 新增
}
```

`TraversalLayout.custom.points` 同步扩展：

```typescript
custom?: {
  points: Array<{ x: number; y: number; z: number; u: number }>
}
```

## § 导入列名归一化

### 列名匹配映射（大小写不敏感）

| 标准轴名 | 匹配的列名别名 |
|---|---|
| X | `x`, `X`, `posx`, `PosX`, `pos_x`, `Pos_X`, `X(mm)`, `x(mm)`… |
| Y | `y`, `Y`, `posy`, `PosY`, `pos_y`, `Pos_Y`, `Y(mm)`, `y(mm)`… |
| Z | `z`, `Z`, `posz`, `PosZ`, `pos_z`, `Pos_Z`, `Z(mm)`, `z(mm)`… |
| U | `u`, `U`, `posu`, `PosU`, `pos_u`, `Pos_U`, `U(°)`, `u(°)`, `α`, `alpha`… |

匹配算法：取列名 → TrimSpace → ToLower → 正则 `^[xyzuposαalpha]+` 提取 → 查表。

### 单位处理

**不做单位换算**。列名中的单位注释 `(mm)` `(°)` 等忽略，数值直接按文件原值使用。位移机构单位由控制器 profile 轴配置决定，遍历测试层不负责单位换算。

### 缺列处理

缺失的轴列按 0 填充，不报错。

## § PointsPreview 画布改造

### 轴对选择器

画布左上角增加两个下拉选择器：

- **横轴 (X axis)**：选项 X / Y / Z / U，根据 customPoints 中非零列决定可用选项
- **纵轴 (Y axis)**：选项 X / Y / Z / U，与横轴不能相同

默认值：横轴 X、纵轴 Y。

切换轴对后：
- bounds 重新按选中轴对的数据范围计算
- 底部标签文本同步更新（如 `X: 0 ~ 100 | Z: -50 ~ 50`）
- 坐标轴十字线跟随切换（居中位置用选中的两轴 0 点）
- 当前点闪烁、已完成/未完成/采集中颜色逻辑不变

### 实现要点

- `bounds` 由硬编码 `point.x / point.y` 改为 `point[hAxis] / point[vAxis]`（通过 `computed` 签名注入当前选中的轴名）
- `transformX/transformY` 不再绑定到 `point.x/point.y`，改为接收 `getCoord(point, axisName)` 辅助函数
- 底部标签文本改为模板字符串 `{hAxisName}: {hAxisName}Min ~ {hAxisName}Max | {vAxisName}: …`

## Out of Scope

- Line/Rectangle/Sector 布局不扩展 4 轴参数化定义
- 导入追加/合并模式
- 轴单位自动换算
- 重复点/越界校验
- 多面板小图、颜色大小编码
- Z/U 轴在 Line/Rectangle/Sector 模式中的使用
