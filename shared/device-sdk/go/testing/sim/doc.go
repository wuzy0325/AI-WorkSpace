// Package sim 提供设备协议模拟器框架（任务 TC-HW-SIM-01）。
//
// # 设计目标
//
// 在本地起一个 TCP 服务端，让真实 adapter（实现 ports.Device 接口、
// 通过 net.DialTimeout 连接设备）连接到模拟器，从而端到端测试：
//   - SCPI/二进制命令帧的解析与响应
//   - 数据帧的持续推送与解析
//   - 断连重连（DisconnectAll 触发 adapter 的 ErrorNotifiable 回调）
//   - 网络延迟/超时（SetLatency）
//   - 丢包/命令无响应（DropNext）
//   - 错误帧/脏帧注入（InjectFrame）
//   - 多设备并发（多个 Simulator 实例 + 多客户端连接）
//
// # 为什么放在 shared/device-sdk/go/testing/sim
//
// 模拟器是跨项目测试基础设施：wind-daq 之外的 daq-t1603、daq-p1604 等
// 独立 Wails 项目后续也会复用同一框架做端到端测试。放在 shared 下避免
// 在每个项目重复实现。包名 sim 表明是测试用模拟器，import 路径为：
//
//	import "shared.local/device-sdk/go/testing/sim"
//
// # 与项目内既有 sim 包的关系
//
// projects/wind-daq/.../adapters/hardware/sim 已有一套基于
// FrameProducer + CommandResponder 双接口的模拟器，但只支持单客户端、
// 故障注入能力有限。本包采用更通用的 ProtocolHandler 单接口 + 多客户端
// 设计，二者可并存：本包面向跨项目复用，既有包继续服务 wind-daq 现有集成
// 测试，后续可逐步迁移到本包。
//
// # 核心理念：Simulator 是哑管道
//
// Simulator 只负责 TCP 连接管理、命令读取、故障注入与帧广播；所有协议知识
// （命令格式、响应编码、数据帧长度前缀/校验和）都封装在 ProtocolHandler
// 实现里。新增设备类型只需实现 ProtocolHandler，无需改动 Simulator。
//
// # 使用示例
//
//	// 1. 实现设备协议处理器（实现 ProtocolHandler 接口）
//	handler := &myDeviceHandler{}
//
//	// 2. 创建并启动模拟器（端口 0 由系统分配，避免冲突）
//	s := sim.NewSimulator("127.0.0.1:0", handler)
//	if err := s.Start(); err != nil { /* ... */ }
//	defer s.Close()
//
//	// 3. 让真实 adapter 连接模拟器地址
//	profile.Address, profile.Port = sim.SplitAddr(s.Addr())
//	adapter := hardware.NewDeviceAdapter(profile)
//	_ = adapter.Connect()
//	_ = adapter.StartAcquisition()
//
//	// 4. 注入故障验证 adapter 容错
//	s.DisconnectAll()      // 触发断连重连
//	s.InjectFrame(dirtyFrame) // 注入错误帧验证解析容错
//	s.SetLatency(200 * time.Millisecond) // 模拟网络延迟/超时
//
// # 故障注入的线程安全
//
// SetLatency / DropNext / InjectFrame / DisconnectAll / ClientCount 均可在
// 测试 goroutine 中随时调用，与模拟器内部 goroutine 安全并发：
//   - latency / dropNextN / closed 用 atomic
//   - clients 用 sync.Map
//   - 每个客户端的写操作经独立 writeCh 串行化，避免交叉写导致帧边界错乱
package sim
