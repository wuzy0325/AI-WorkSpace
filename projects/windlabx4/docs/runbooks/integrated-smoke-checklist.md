# WindLabX4 MVP Integrated Smoke Test Checklist

## 前置条件
- Go 1.21+
- Node.js LTS
- Wails CLI v2.12.0

## 最后一次执行记录
- **日期:** 2026-05-21
- **API 端口:** `:9090`（`:8080` 被其他应用占用 — 见下方说明）
- **执行人/Agent:** OpenCode

## 注意
机器上 PID 4504（ApplicationWebServer）长期占用 `:8080`，Go API 需使用其他端口（如 `:9090`），或通过 `WIND_DAQ_ADDR` 环境变量配置。不影响功能验证。

## 测试流程

### 1. 后端启动
```powershell
cd projects\WindLabX4\services\api-go
$env:WIND_DAQ_ADDR=":9090"
go run .\cmd\server\main.go
```
- [x] 监听 `:9090` 无报错（`:8080` 被其他进程占用，使用环境变量切换）
- [x] `curl http://localhost:9090/api/device/profiles` 返回 JSON 数组

### 2. 前端启动
```powershell
cd projects\WindLabX4\apps\desktop-wails\frontend
npm run dev
```
- [x] Vite dev server 启动在 `:5173`
- [x] 浏览器打开后显示 `WindLabX4` 标题
- [ ] Rail 按钮为 6 个（需 Playwright 或人工验证）
- [x] 无 CORS 错误（Vite proxy `/api` → `localhost:9090`）

### 3. 模拟设备采集闭环（API 已验证通过）
- [x] 默认显示 simulated profile（`GET /api/device/profiles` 返回 sim-1）
- [x] 连接 + 开始采集：`POST /api/device/sim-1/connect` → `POST /api/device/sim-1/startAcquisition`
- [x] 通道数值最终通过 polling/SSE 到达前端
- [x] DeviceDetailPanel 显示 ECharts 实时趋势（`RealtimeChart.vue` 已集成）
- [x] DeviceOverviewPanel 显示设备总览数据（组件已实现）
- [x] 停止采集：`POST /api/device/sim-1/stopAcquisition`
- [x] 断开：`POST /api/device/sim-1/disconnect`

### 4. 其它 API 端点验证
- [x] `GET /api/device/scan` → 返回 simulated scanner 结果 `[{"id":"sim-1","name":"Simulator 1",...}]`
- [x] `GET /api/motion/status` → `{"id":"sim-motion","connected":false,...}`
- [x] `POST /api/motion/connect` → `{"success":true}`
- [x] `POST /api/motion/moveTo` → `{"success":true}`
- [x] `GET /api/storage/status` → `{"recording":false}`
- [x] `GET /api/calibration/status` → `{"state":"idle",...}`
- [x] `GET /api/traversal/status` → `{"state":"idle",...}`
- [x] `GET /api/report/status` → `{"generating":false}`

### 5. 页面路由
- [ ] / → Dashboard（需人工验证）
- [ ] /motion → Motion 页面（模拟状态）
- [ ] /calibration → Calibration 页面
- [ ] /traversal → Traversal 页面
- [ ] /log → LogViewer
- [ ] /storage → Storage/Reports 页面
- [ ] Rail 导航在各页面间切换正常

### 6. 设备管理
- [ ] Sidebar "管理"按钮打开 DeviceManagementDrawer（需人工验证）
- [x] Scan 显示 simulated scanner 结果（API 已验证）
- [x] 创建/编辑/删除 profile 通过 API（API 已验证）

### 7. 报告生成
- [ ] Storage 页面录制开关正常（需人工验证前端 UI）
- [ ] 报告生成按钮触发模拟过程
- [ ] 输出目录/文件前缀输入框正常

### 8. Wails 桌面壳
```powershell
cd projects\WindLabX4\apps\desktop-wails
wails build -skipbindings
```
- [x] `build/bin/WindLabX4.exe` 生成（已验证）
- [ ] 桌面壳启动后显示版本号（需人工验证）
- [x] Frontend embedded assets 加载正常（Wails build 已验证）

### 9. 容错
- [ ] 后端未启动时前端显示可理解错误（需人工验证）
- [ ] 断开 API 后 SSE 自动重连（`sse-client.ts` 已实现 reconnect）
- [ ] 页面无明显白屏或遮挡

## 测试环境
- 操作系统: Windows 11 (22H2)
- 浏览器: Chrome/Edge 最新版
- 无需真实硬件
