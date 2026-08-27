# Spec: WTNMC4A 运动控制器崩溃加固(2026-08-27 现场闪退)

> 关联现场日志:`temp/WindLabX4-20260827.log`
> 状态:评审修订版(v2) — 已回应 4 项 Findings,待最终批准后进入 Phase 4
> 范围决策:仅 A1 + A4 + B5(不含 C7 进程隔离)

## 1. Objective

### 背景

目标机上 WindLabX4 反复闪退。现场日志最后一行是
`09:19:05 WTNMC4A DEV_CreateA returned handle address=192.168.3.235`,随后进程硬崩溃,
无任何 Go shutdown 日志、无 recover 痕迹。同时,整个会话期间:

- X/Y/Z/U 四轴全部返回同一恒定垃圾值 `raw_pulse=1482709088`(0x58605860)
- DAQ-P-1604(192.168.3.7)连上 10s 即断流,之后彻底 unreachable
- 崩溃前 `/api/motion/status` 每个请求耗时 1.3~2.6s(FFI 调用在超时)

结论:崩溃点位于 `DEV_CreateA` 成功之后的 FFI 调用链
(`verifyConnectionLocked`→`getRR1` 或 `cacheAxisSpeedsLocked` 的 setXX),
特征与已知"控制器不可达 → DLL 内部访问违例 0xc0000005"一致
(wtnmc4a_motion.go:1646-1651 注释记载同类崩溃)。

### 目标

降低 WTNMC4A FFI 路径在"控制器不可达/链路损坏"场景下的崩溃概率,并让故障可诊断。
**注意:本次只降低概率,不承诺消除崩溃** —— 唯一可靠隔离原生 DLL 崩溃的强边界是 C7 进程隔离(本次不做)。

1. **A1**:Go 工具链升级到 ≥1.26.2,排除 Go 1.25.0–1.26.1 栈破坏 bug(#77975)干扰
2. **A4**:连接前 TCP 预检 —— 控制器不可达时快速失败返回错误,不进 DLL
3. **B5**:验证并测试现有握手 fail-fast;必要时最小 seam/调用复用。
   **当前控制流已具备该行为**(`verifyConnectionLocked` 失败即 cleanup+return),B5 不预设重写控制流

### 明确不做(本次范围外)

- **C7 进程隔离**:把 DLL 调用隔离到独立 driver 子进程(唯一能 100% 防 DLL 崩溃拖垮主程序),
  改动大,另行排期。若 A1+A4+B5 落地后现场仍偶发且已通过 WER dump 实锤 DLL,再上 C7。
- **C8 Vectored Exception Handler**:不采用(捕获后堆可能损坏,行为不可预测)。
- A2/A3(现场运维):确认目标机部署版本含 2026-08-13 `bee038d` 修复、恢复 192.168.3.x 链路。
  写入验收标准由现场执行,不做代码改动。

## 2. Tech Stack

| 项 | 值 |
|---|---|
| Go | `go.mod` 声明 `go 1.25.0` → 需提升;`go.work` 声明 `go 1.26.1` → 需同步提升;目标工具链 ≥1.26.2(用户自行安装) |
| 平台 | Windows only(`wtnmc4a_motion.go` 是 `//go:build windows`) |
| 涉及模块 | `shared/device-sdk/go/motion`(跨 windlabx4 / motion-controller / wispa / wista / 1604Cal / shared/motion-control / 诊断程序共享) |
| 测试 | Go 标准 `testing`,无额外框架 |

## 3. Commands

```powershell
# 单元测试(改动模块)
cd C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\shared\device-sdk\go
go test ./motion/... -count=1

# 全量验证(Windows)
go test ./... -count=1
gofmt -l .            # 期望:无输出

# windlabx4 后端(消费方)
cd C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\projects\windlabx4\services\api-go
go build -buildvcs=false ./...
go test ./internal/... ./api/... ./pkg/...

# 桌面壳(消费方)
cd C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\projects\windlabx4\apps\desktop-wails
go build -buildvcs=false ./...
```

## 4. Project Structure

```
shared/device-sdk/go/motion/adapters/hardware/
├── wtnmc4a_motion.go          # 改动:Connect 预检 + verifyConnectionLocked 复用 seam
├── wtnmc4a_motion_test.go     # 改动:预检/握手失败路径测试
├── wtnmc4a_singleflight_test.go
└── wtnmc4a_bench_test.go

go.work                            # A1:go 指令 1.26.1 → 1.26.2
projects/windlabx4/.../go.mod      # A1:go 指令 1.25.0 → 1.26.2(desktop-wails + api-go)
shared/device-sdk/go/go.mod        # A1:go 指令 1.25.0 → 1.26.2

docs/plans/
└── spec-wtnmc4a-crash-hardening.md   # 本 spec
tasks/                                # Phase 2/3 产物(plan.md, todo.md)
```

## 5. Code Style

对齐现有风格。改动集中在 `Connect`/`verifyConnectionLocked`/`cleanupConnectionLocked`。
**评审决定(Findings 1/4):预检位于 `ioMu` 锁内、`mu` 锁外,使用 address 快照;dial 走包级 seam。**

```go
// 包级 seam:默认 net.DialTimeout,测试替换后 t.Cleanup 恢复。
// 使预检失败可在不依赖真实网络的情况下稳定、快速复现。
var dialTCP = func(ctx context.Context, network, addr string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout(network, addr, timeout)
}

// preflightReachable 在进入 DLL 前做 TCP 预检。
// 控制器不可达时快速失败,避免 DEV_CreateA 在损坏链路上留下半初始化句柄。
// 注意:WTNMC4A_DEV_CreateA 只接收 IP,端口由 DLL 内部固定为 defaultPort(5000),
// 因此预检必须探测 defaultPort,不能用 profile.Port(该字段对 WTNMC4A 连接无意义)。
func (c *WTNMC4AMotionController) preflightReachable(ctx context.Context, address string) error {
	addr := net.JoinHostPort(address, strconv.Itoa(defaultPort))
	conn, err := dialTCP(ctx, "tcp", addr, 2*time.Second)
	if err != nil {
		return fmt.Errorf("WTNMC4A 控制器预检失败 %s: %w", addr, err)
	}
	_ = conn.Close()
	return nil
}
```

### 锁范围(评审 Finding 1)

`Connect` 内预检的锁序(与现有 `Connect` 结构兼容,且避免 TOCTOU):

1. 先获取 `c.ioMu`(保持现有外层锁)
2. 短暂获取 `c.mu`,复制 `address := c.profile.Address`(以及可选的连接状态检查),随即释放 `c.mu`
3. 持有 `c.ioMu`(不持 `c.mu`)执行 `preflightReachable(address)` —— 网络等待不阻塞状态读锁
4. 重新获取 `c.mu`,使用**同一个 address 快照**调用 `DEV_CreateA`

这把 2s 网络等待放到 `ioMu` 内、`mu` 外,与 `ApplyConfig`(`wtnmc4a_motion.go:349`,持 mu 改 profile)串行,同时不长时间阻塞读锁。**此项属于原 spec 的 Ask first 锁范围变更,已按评审批准纳入;若实现时发现与现有锁结构冲突,先停下询问。**

约定:
- 新增错误统一 `fmt.Errorf("WTNMC4A ...")` 前缀,与现有错误风格一致
- 预检超时 2s(与默认 200ms FFI 超时形成两级防护)
- 日志用 `slog`,带 `component`/`id` 字段
- 每函数 ≤50 行、每文件 ≤500 行(AGENTS.md 硬约束);超限拆函数
- 测试注入沿用现有 seam(`c.readRR1`/`c.readLP`/`c.startMove`) + 新增包级 `dialTCP` seam,不引入新 mock 框架

## 6. Testing Strategy

| 级别 | 位置 | 覆盖 |
|---|---|---|
| 单元 | `wtnmc4a_motion_test.go` | 预检失败→Connect 返回错误且不创建 gate;握手失败→cleanup 且不再 setXX |
| 单元 | `wtnmc4a_singleflight_test.go` | 预检失败在 single-flight 下正确传播(可选) |
| 集成 | `go test ./motion/...` | 全模块回归 |

### 关键测试点(评审 Finding 4 —— dial seam)

预检失败测试用包级 `dialTCP` seam 注入,断言必须覆盖:
- **dial 地址确实是 `Address:5000`**(捕获 seam 收到的 addr)
- **传入超时为 2s**
- `Connect` 返回错误、`c.handle==0`、`Connected==false`、`LastError != ""`
- `c.ffi == nil`(未创建 gate)
- **未进入 DLL 加载/调用路径**(通过 seam 记录或状态断言证明 `loadWTNMC4AProcs` / `DEV_CreateA` 未被触发)
- `t.Cleanup(func() { dialTCP = realDial })` 恢复 seam

握手失败测试(评审 Finding 2 —— B5 语义):
- **当前控制流已具备 fail-fast**:`Connect` 中 `verifyConnectionLocked` 失败 → `cleanupConnectionLocked` + return(`wtnmc4a_motion.go:422-426`),`cacheAxisSpeedsLocked` 在其后(`:428-434`)。
- **但 `verifyConnectionLocked` 直接调用 `c.procs.getRR1.Call`**(`:1293`),未走 `c.readRR1` seam。因此:
  - 最小生产改动:**让 `verifyConnectionLocked` 复用 `getRR1Status`(含 `c.readRR1` seam)**,而不是直接调 `getRR1.Call`
  - 测试通过注入 `c.readRR1` 返回错误,驱动"握手失败 → cleanup → 无 setXX"路径
- **不重复实现已存在的 return 分支**;只有测试证明存在遗漏时才改生产控制流
- 预检成功但握手失败:预检通过 → DEV_CreateA 返回句柄 → 握手失败 → 已 cleanup
- 不回归:`TestWTNMC4A*` 现有测试全绿

**注意**:锁范围变更(预检在 `ioMu` 内、`mu` 外)可能影响现有测试的加锁顺序断言,需同步调整(已按评审批准)。

## 7. Boundaries

### Always do
- 预检失败/握手失败必须返回错误并标记 `c.status.LastError`,设置 `Connected=false`
- 预检使用 TCP `DialTimeout`(有界),绝不用无界 Dial
- 所有新错误带 `WTNMC4A` 前缀
- 跑 `gofmt` + `go test ./motion/...`

### Ask first
- 改变 `Connect`/`Disconnect` 的加锁顺序或锁范围
- 修改 `shared/device-sdk/go` 的公共导出 API(ports 接口签名)
- 提升/改动 `ffiCallTimeout`、gate 队列容量等超时常量
- 新增依赖(不预期需要,但如需要先问)
- 改动 motion-controller / wispa 等其他消费方代码(本 spec 不预期触碰)

### Never do
- 不卸载 WTNMC4A DLL(沿用 `wtnmc4aSharedDLL` 常驻策略,防止 worker 卡死返回已卸载代码)
- 不删除或禁用现有 FFI gate 超时机制
- 不依赖 Go `recover` 防护 DLL 原生崩溃(无效)
- 不提交任何日志/密钥/token
- 不修改 `wtnmc4a.go` 的 FFI 函数签名(与 DLL 导出必须严格一致)

## 8. Success Criteria

1. **A1**:`go.mod`(windlabx4 desktop-wails + api-go + shared/device-sdk)与根 `go.work` 的 `go` 指令
   提升到 ≥1.26.2(工具链由用户安装);构建不再报 "requires go >= 1.26.2" 错误
2. **A4**:控制器不可达时,`Connect` 在进入 DLL 前返回错误:
   - 新增 `preflightReachable` TCP 预检,超时 2s,探测 `Address:5000`(defaultPort),走 `dialTCP` seam
   - 预检位于 `ioMu` 锁内、`mu` 锁外,使用 address 快照
   - 预检失败 → `Connect` 返回错误,`c.handle==0`、`Connected==false`、`LastError` 有值、`c.ffi==nil`
3. **B5**:`verifyConnectionLocked` 复用 `getRR1Status`(seam);握手失败 → `cleanupConnectionLocked`,
   `Connected==false`,不再执行 `cacheAxisSpeedsLocked` 的 setXX 调用(回归测试锁定该行为)
4. 新增测试覆盖上述失败路径,`go test ./motion/... -count=1` 全绿
5. `gofmt -l` 无输出;windlabx4(api-go / desktop-wails)构建通过;
   共享模块消费者(motion-controller / wispa / wista / 1604Cal / shared/motion-control / 诊断程序)
   的构建影响在 A1 时评估并记录(不承诺全部验证,风险在 plan 中明确)
6. 现场验收(运维项):部署版本 ≥ 2026-08-13(bee038d);恢复 192.168.3.x 链路后
   复测运动控制正常;若再闪退,取 WER dump 确认故障模块

## 9. Open Questions

1. ~~预检端口~~ **已确认**:`WTNMC4A_DEV_CreateA` 只接收 IP(syscall.BytePtrFromString(c.profile.Address)),
   端口由 DLL 内部固定为 `defaultPort=5000`;`profile.Port` 对 WTNMC4A 连接无意义。预检探测 `Address:5000`。
2. ~~预检位置~~ **已按评审批准**:预检在 `ioMu` 内、`mu` 外,address 快照,避免 TOCTOU(见 §5 锁范围)。
   若实现时与现有锁结构冲突,先停下询问。
3. 预检通过率与现场网络波动的关系 —— 预检失败时保留"可手动重试"语义(现有 Connect 被
   startMotionAutoConnect / 前端 /api/motion/connect 反复调用,失败即重试,无需额外机制)。
4. `ctx` 参数:示例中 `preflightReachable(ctx, address)` 保留 `ctx` 仅为与调用方签名一致,
   预检实际用固定 2s `DialTimeout`,不承诺响应调用方取消(评审 Finding 4 提示)。