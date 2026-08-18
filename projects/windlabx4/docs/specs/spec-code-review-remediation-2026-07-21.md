# Spec: WindLabX4 代码审查问题整改

> 日期：2026-07-21
> 状态：规格与实施计划已批准；任务清单待审阅
> 阶段：Spec-Driven Development Phase 1 - Specify
> 来源：[code-review-2026-07-21.md](../code-review-2026-07-21.md)

## Assumptions

以下假设已经由用户确认：

1. 整改范围覆盖审查报告中的全部 P0、P1、P2 和 Nit，共 16 项。
2. 审查报告是整改入口，不高于代码事实；若测试或调用链证明某项无需修改，应更新报告并用证据关闭该项，不为制造 diff 强制改代码。
3. 不新增第三方依赖，不更改持久化格式、公开 HTTP 路径或 Wails 公共能力；如确有必要，必须回到规格阶段征求批准。
4. 保持现有操作员工作流。允许的行为变化仅包括：错误显式化、删除生产 demo、删除前端业务算法及本地算法 fallback，以及为 HTTP 模式补充与 Wails 等价的 calibration status 轮询，使后端权威 physics 能持续更新。该轮询必须防止请求重叠，并复用现有 status 路径。
5. 自动验证覆盖结构检查、Go test/vet、前端 typecheck/test/build；HIL 和真实运动控制器验证作为人工发布门禁，不伪造通过。
6. 本规格、后续计划和任务文档均保存在 `projects/WindLabX4/docs/specs/`；规格获批前不得进入计划、任务或实现阶段。

## Objective

整改 2026-07-21 静态代码审查确认的问题，使 WindLabX4 的 HTTP API、usecase、adapter、Vue 前端和异步生命周期重新符合工作空间六边形架构、前后端职责边界和可观测性规则，同时不破坏现有设备、校准与遍历工作流。

### Users

- 风洞操作员：后端或运动控制失败时收到明确错误，不被本地 fallback 或静默失败掩盖。
- 维护工程师：能够通过结构化日志、错误码和故障快照追踪文件、资源锁及急停失败。
- 开发人员：API、usecase、ports 和 adapters 依赖方向明确，demo 与生产代码边界可由脚本验证。

### In Scope

| ID | 目标 |
|---|---|
| C-1 | HTTP API 不再直接依赖配置和插值 adapter；显式插值文件导入复用现有 `ports.InterpolatorLoader` |
| C-2 | 从生产前端移除马赫数、静温、流速公式，由后端提供权威结果 |
| C-3 | 急停失败后的普通停止错误可观测，且保留结构化错误状态和故障快照 |
| R-1 | 生产入口不静态导入 `DEMO ONLY` / `simulate*` 算法模块，校验脚本自动阻止回归 |
| R-2 | `calibration.go` 的旧式业务日志迁移到结构化 slog |
| R-3 | 文件 writer 清理和资源锁释放错误可诊断，不在热路径重复记录 recorder 错误 |
| R-4 | 删除前端五孔蛇形布点本地 fallback，后端失败显式反馈 |
| R-5 | MotionSafety 前端校验只作提示，后端保持唯一权威，并增加契约验证 |
| R-6 | API 不直接调用点位生成和插值模式算法操作 |
| R-7 | 完成工作空间结构校验和项目验证，记录真实结果 |
| O-1 | API 不直接构造 shared 算法库输入类型 |
| O-2 | 校准 autoEngine goroutine 具备可取消、可等待且有界的生命周期，或以调用链证据关闭该项 |
| O-3 | DataStreamRelay shutdown 可确认订阅已注销，且不持锁等待 goroutine |
| O-4 | 删除无行为验证价值的测试 `var _` 占位，或改成真实契约测试 |
| N-1 | 保留当前 triggerMode 文案，除非确认存在外部 API 消费者；结论回写审查报告 |
| N-2 | points parser 支持多个中英文括号注释，并有单元测试 |

### Out of Scope

- 新增设备类型、运动控制器、校准算法或遍历算法。
- 修改 HTTP 路径、JSON 字段命名、Wails 对外方法语义或持久化格式。
- UI 重设计、样式整理和与本整改无关的重构。
- 新增日志、并发或测试第三方依赖。
- 声称完成真实硬件、机械限位或急停 HIL 验证。

## Tech Stack

- Go 1.25（`services/api-go/go.mod`），标准库 `testing`、`log/slog`、`context`、`sync`、`errors`。
- 六边形架构：`core`、`ports`、`usecase`、`adapters`、薄 `api` 和 composition roots。
- Vue 3.5、TypeScript 5.8、Pinia 3、Vite 5、Vitest 4。
- Wails v3 `3.0.0-alpha.95`。
- 现有 shared 算法模块：five-hole 和 seven-hole interpolation。
- 不增加依赖；优先复用现有 `ports.InterpolatorLoader`、`adapters/interpolation.Loader` 和三个装配根中的注入模式。

## Commands

命令均从工作空间根目录执行，使用 Windows PowerShell 5.1 语法。

```powershell
# 工作空间结构与依赖方向
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\validate-structure.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\validate-import-direction.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\validate-frontend-structure.ps1 -ProjectDir .\projects\WindLabX4\apps\desktop-wails\frontend\src -CheckFileSize

# 后端格式、构建、静态检查和测试
Push-Location .\projects\WindLabX4\services\api-go
gofmt -l .
go build -buildvcs=false ./...
go vet ./...
go test ./internal/... ./api/...
go test -race ./internal/usecase/... ./api/...
Pop-Location

# 前端类型、单测和生产构建
Push-Location .\projects\WindLabX4\apps\desktop-wails\frontend
npm run typecheck
npm run test
npm run build
Pop-Location

# Wails binding 签名发生变化时才运行
Push-Location .\projects\WindLabX4\apps\desktop-wails
go run github.com/wailsapp/wails/v3/cmd/wails3 generate bindings
Pop-Location

# 发布构建仅在全部自动验证通过后运行
Push-Location .\projects\WindLabX4\apps\desktop-wails
task release
Pop-Location
```

若本机或 Go 版本不支持 `-race`，必须记录命令、平台限制和未覆盖风险，不得把未执行标记为通过。

## Project Structure

本规格只定义职责和允许触及的区域；确切文件清单在 Phase 2 Plan 中根据调用链确定。

```text
projects/WindLabX4/
├── docs/
│   ├── code-review-2026-07-21.md             # 活的整改状态与证据
│   └── specs/
│       └── spec-code-review-remediation-2026-07-21.md
├── services/api-go/
│   ├── api/                                   # HTTP DTO、校验、usecase 调用、响应映射
│   ├── internal/
│   │   ├── core/                              # 纯领域规则和算法
│   │   ├── ports/                             # 外部能力接口；零实现
│   │   ├── usecase/                           # 导入、校准、遍历和生命周期编排
│   │   ├── adapters/                          # 配置、插值文件、存储、硬件实现
│   │   └── bootstrap/                         # composition root
│   └── pkg/
│       ├── appcontext/                        # composition root
│       └── apiserver/                         # composition root
└── apps/desktop-wails/
    ├── backend/                               # 薄 Wails bridge
    └── frontend/src/
        ├── api/                               # 后端能力调用
        ├── components/                        # 展示与交互
        ├── composables/                       # UI 状态组合；无业务算法
        ├── stores/                            # Pinia UI 状态；无气动公式
        ├── shared/                            # TS DTO 与展示分类
        ├── utils/simulate*                    # 仅 demo 构建允许导入
        └── **/__tests__/                      # Vitest 测试
```

新增 Go 测试与被测文件同包放置为 `*_test.go`；前端测试放在相邻 `__tests__/`。不得新增通用 `util` 包来消除一行 helper 重复。

## Code Style

### Go

- 使用 `gofmt`，导出符号写简洁英文注释。
- 业务事件使用英文 slog message 和稳定的 snake_case 字段。
- 错误使用 `%w` 或 `errors.Join` 保留 cause；不得用 `_ =` 丢弃安全或资源清理错误。
- usecase 只依赖 `core` 和 `ports`，composition root 是 adapter 与 usecase 相遇的位置。

```go
func (m *CalibrationManager) stopAfterEmergencyFailure(
    runtime calibration.RuntimeAccess,
    emergencyErr error,
) error {
    fallbackErr := runtime.StopMotion()
    if fallbackErr == nil {
        return emergencyErr
    }

    slog.Error("calibration emergency stop fallback failed",
        "component", "calibration",
        "emergency_stop_error", emergencyErr,
        "fallback_stop_error", fallbackErr,
    )
    return errors.Join(emergencyErr, fmt.Errorf("fallback stop failed: %w", fallbackErr))
}
```

该片段只定义错误处理风格，不强制新增同名 helper；实现应选择最小代码并保留现有状态更新顺序。

### TypeScript / Vue

- 使用现有 setup-style Pinia 和 Vue Composition API。
- 前端只展示后端提供的气动结果；不得复制 Ma、速度、密度、动压或校准插值公式。
- 后端失败必须进入现有 feedback/error UI，不得 catch 后静默执行本地业务算法。
- demo-only 模块必须有 `// DEMO ONLY - not for production use` ASCII 标记，并且生产入口不得静态导入。

```ts
const physics = computed(() => calibrationStore.calculatedPhysics)

function formatPhysics(value: number | undefined): string {
  return value === undefined ? '--' : value.toFixed(3)
}
```

## Testing Strategy

### Test Levels

| 层级 | 关注点 | 位置/工具 |
|---|---|---|
| Go unit | usecase 导入编排、急停双失败、资源清理、relay/autoEngine 生命周期 | 相邻 `*_test.go`，标准 `testing` |
| Go API | 路由保持原 HTTP contract，只委托 usecase | `services/api-go/api/*_test.go` |
| Go integration | composition roots 注入 loader/decoder 后流程可用 | `tests/integration`、`pkg/*/*_test.go` |
| Frontend unit | 后端 physics 显示、错误传播、MotionSafety 非权威提示、parser 多括号 | Vitest 相邻 `__tests__` |
| Structural | API/usecase import 方向、生产 demo 静态导入 | workspace PowerShell validators |
| Build | TypeScript、Vite production bundle、Go 全包编译 | `npm run build`、`go build` |
| HIL/manual | 急停、fallback stop、机械状态与实际 UI 提示 | 发布前人工执行并记录 |

### Required Regression Coverage

1. 每种显式插值导入（单 PRB、五孔 CSV、多 PRB、七孔 PRB、七孔 CSV）至少覆盖成功和 loader error；HTTP 状态与响应字段保持兼容。
2. 校准配置解码覆盖有效嵌套 channel DTO 和无效输入，不让 usecase import config adapter。
3. 急停路径覆盖急停成功、急停失败但普通停止成功、两次均失败；最后一条断言两个 error 均可通过 `errors.Is` 或等价 cause 检查识别，并校验错误码/故障快照。
4. 前端不再计算 atmospheric physics；后端值为 0、有效数值和缺失值时分别正确显示。
5. 后端蛇形点位调用失败时 UI 暴露错误，且不产生本地点位。
6. production entry 的静态依赖图不包含 `simulateFiveHoleCalibration.ts` 或 `simulateTraversalRun.ts`。
7. MotionSafety 前端校验不能绕过后端；后端对同一非法配置仍拒绝。
8. DataStreamRelay `Stop` 返回后，测试可确认所有订阅执行 `unsub()`；重复 Stop 安全。
9. 若为 autoEngine 增加等待，测试证明正常完成、Stop 和 shutdown 都在有界时间内返回；不得引入锁内 Wait。
10. parser 覆盖 `X(mm)(deg)`、`X（mm）（deg）` 和中英文混合括号。

### Coverage Policy

不设虚构的全仓百分比门槛。所有修改的分支必须有针对性回归测试；现有测试不得删除或弱化来获得通过。无法自动验证的硬件行为必须列入 HIL 清单。

## Boundaries

### Always Do

- 修改函数、方法或类前执行 GitNexus upstream impact 分析；HIGH/CRITICAL 先告警。
- 先写或更新失败测试，再修改行为；文档、纯日志字段和校验脚本小改可使用定向验证。
- 保持 `api -> usecase -> core/ports` 与 composition root 注入方向。
- 复用现有 `ports.InterpolatorLoader`，不创建功能重叠的 loader abstraction。
- API 和 Wails 公开行为变化时同步测试，并按项目规则检查 bindings。
- 每完成一个垂直切片运行定向测试，最终运行 Commands 中适用的全套验证。
- 发现审查项为误报时，把代码证据和验证结果回写审查报告。

### Ask First

- 新增任何第三方依赖或新顶层/基础目录。
- 更改 HTTP 路径、JSON 字段、Wails 签名或错误码语义。
- 更改持久化格式、CSV schema、checkpoint 或历史配置兼容行为。
- 改变 demo 产品能力：完全删除模拟模式，而不是从生产 bundle 隔离。
- 修改 CI、发布脚本、构建 target 或跳过 HIL 门禁。
- 需要在 Stop/shutdown 中引入新的超时数值且现有代码没有权威默认值。

### Never Do

- 让 `usecase` import `internal/adapters/*`，或让 API 承担文件 I/O 和算法编排。
- 在生产前端保留气动、校准、遍历或运动控制业务算法 fallback。
- 吞掉急停、fallback stop 或资源清理错误。
- 为通过测试删除失败测试、放宽安全断言或伪造 HIL 结果。
- 改动无关代码、格式化整个仓库、删除用户已有修改或提交 secrets。
- 用运行时 `if (import.meta.env.DEV)` 代替对生产静态依赖和 bundle 的验证。

## Success Criteria

### Architecture and Contracts

- [ ] `services/api-go/api` 不 import `internal/adapters/config` 或 `internal/adapters/interpolation`。
- [ ] `services/api-go/internal/usecase` 不 import 任何 `internal/adapters/*`。
- [ ] 显式插值文件导入复用 `ports.InterpolatorLoader`；API 不直接调用 `Load*` adapter 函数或设置 interpolator mode。
- [ ] API 不直接调用 `calibration.GenerateFiveHoleSnakePoints`，不直接构造 five-hole/seven-hole shared `InterpolationInput`。
- [ ] 既有 HTTP 路径、请求字段、成功响应字段和错误状态保持兼容，除非本规格明确要求错误从静默 fallback 变为显式失败。
- [ ] 七孔 PRB/CSV 导入响应继续返回现有约定的 `pointCount`：内区 169、外区 52；本轮不改为从文件动态统计，避免响应值兼容性变化。
- [x] 五孔 tTunnel 通道前后端对齐：前端 FiveHoleSettings 已把 `fiveHole.tTunnel` 列为必填，后端 roleMap、FiveHoleRawData、CalculateFiveHoleAverage、computeLivePhysicsFromGauge 全链路补齐 TTunnel 字段，TAT 优先级 TTunnel > TAtm 与七孔/总压一致。（P1-4 修复：types.go 新增 `TTunnel *float64`、read_probe_channels.go 新增 `"fiveHole.tTunnel": "tTunnel"` 映射、formulas.go 平均值累加 TTunnel、calibration.go computeLivePhysicsFromGauge 传 raw.TTunnel。）

### Frontend Boundaries

- [ ] 生产前端中不存在 atmospheric physics、五孔插值、遍历派生物理量或蛇形布点业务算法实现。
- [x] 校准页面的 Ma/V 来自后端权威数据；缺失显示 `--`，数值 0 不被当成缺失。（P1-3 修复：终态 completed/error/stopped 下 Status() 不再组装 LivePhysics，避免 reader 仍在线时返回最后一帧 stale Ma/V；新增 TestCalibrationStatusPhysics_TerminalStatesSkipLivePhysics + TestCalibrationStatusPhysics_RunningAndPausedIncludeLivePhysics 回归测试。P1-2 修复：calibrationStore 轮询使用 generation token 隔离跨代乱序响应，旧 polling 代的响应不会覆盖新任务状态。P2-7 修复：HTTP pause/resume 失败显示 toast，与 stop 路径一致。）
- [ ] `TraversalMain.vue` 及其生产依赖不静态导入 `DEMO ONLY` / `simulate*` 算法模块。
- [ ] 结构校验脚本能够对一个最小违规 fixture 或现有测试场景报错，并对合规生产入口通过。
- [x] MotionSafety 前端校验明确为 UX 提示，后端在提交时重新校验且保持最终权威。

### Safety, Errors, and Logging

- [ ] 急停和 fallback stop 同时失败时，两个 cause、结构化错误码和 MotionSafetyFailure 快照均保留。
- [ ] `calibration.go` 审查列出的 19 处 `log.Printf` 迁移为合适级别的 slog，关键字段独立可检索。
- [ ] writer Flush 和资源锁 Release 错误被聚合或记录；StorageRecorder 错误不在 data sink 重复记录。
- [ ] 后端点位生成失败时前端显示错误，不静默生成本地点位。

### Lifecycle and Tests

- [x] DataStreamRelay `Stop` 的完成语义由测试定义并满足，且实现不持有 mutex 等待订阅 goroutine。（P1-1 修复：Unsubscribe 锁外等 sub.done 期间移入 retiring 集合，Stop 遍历 subs+retiring 合并等待，避免并发 Stop 看到空 map 漏等中间态订阅；新增 TestDataStreamRelayStopWaitsForRetiringSubscriptions + TestDataStreamRelayStopWithConcurrentUnsubscribe 两个并发契约测试。）
- [ ] autoEngine 生命周期经调用链和测试复核：若存在不可等待风险则实现有界 shutdown；若不存在则以证据更新报告关闭 O-2。
- [x] `var _` 测试占位被真实契约测试替代或删除，不保留无行为价值占位。（Task 23 已删除两处 `var _` + 未使用 import；既有 HTTP/core 行为测试覆盖 `ProbeTypeSevenHole` 与 `MotionInterruptNone` 符号。）
- [x] triggerMode 文案经消费者复核；没有外部消费者时保持现状并关闭 N-1，避免投机性兼容代码。（Task 23：triggerMode 数值 0/2 是 HTTP/Wails/持久化/驱动四条契约统一值，非内部枚举；N-1 标记 Closed with Evidence，profile-upsert 校验一致性差异登记为 FU-1。）
- [x] points parser 正确处理多个中英文括号段，原有解析测试继续通过。（P2-6 修复：skip 列名带括号注释时基于剥离后的 normalized base 判定反转，与 normalizeConfigKey 内部一致；新增 4 个中英文/多括号 skip 列测试用例 17a-17d。）

### Verification Gate

- [ ] 所有适用的结构、Go、前端验证命令退出码为 0；任何未执行项在报告中明确标记原因和风险。**【Blocked / Waiver】**
  - **通过项（exit 0）**：
    - **Go**：`go build -buildvcs=false ./...` → exit 0；`go vet ./...` → exit 0；`go test ./...` → 全 PASS（usecase 76.5s + apiserver 10.5s + integration 10.5s）。
    - **前端**：`npm run typecheck` → exit 0；`npm run test -- --run` → 18 files / 248 tests 全 PASS（含 P2-6 新增 4 个 skip 列括号注释测试）；`npm run build` → exit 0。
    - **import 方向**：`validate-import-direction.ps1` → exit 0（699 Go files checked）。
  - **未通过项（waiver/blocker，预存工作区问题，非本轮整改回归）**：
    - `validate-structure.ps1` → exit 1（12 个未知顶层条目：`.trae`/`.codebuddy`/`.workbuddy`/`.worktrees`/`analysis`/`pdf-qa`/`temp`/`WindLabX4-intro-images`/`WindLabX4-trae-work-prototype`/`.tmp-protocol-remaining.html`/`multi-probe-traversal-design.html`/`W505-Protocol-V2.0.html`）。
    - `validate-frontend-structure.ps1 -CheckFileSize` → exit 1（i18n 硬编码、`: any`、CSS class 重复、泛型变量名——均为预存问题，review 报告已记录）。
  - **状态说明**：acceptance 要求"exit 0"，上述两项因预存工作区问题未满足，登记为 waiver/blocker。本轮整改改动本身（P1-1~P1-4 + P2-6/P2-7）符合结构规范，未引入新的结构违规。预存问题的修复属独立后续项，不在本轮 24 个 Task 范围内。
- [ ] `git diff --check` 通过，最终 diff 只包含规格、审查报告和整改所需文件。**【Blocked / Waiver】**
  - 本轮整改改动的文件 `git diff --check` → exit 0（干净，无行尾空白）。
  - 全工作区 `git diff --check` → exit 2，13 处行尾空白全部在 Wails 生成 bindings 文件中（生成器输出特征，非本轮回归，review 报告已记录）。
  - **状态说明**：acceptance 要求"git diff --check 通过"，全工作区因 Wails 生成 bindings 行尾空白未满足，登记为 waiver/blocker。生成器输出特征不属本轮整改可控范围；bindings 重新生成需 Wails CLI，不在本轮 24 个 Task 范围内。
- [x] GitNexus `detect_changes` 显示受影响流程符合批准后的计划；HIGH/CRITICAL 变更已有人工复核。
  - `npx gitnexus detect-changes --repo AI-WorkSpace` → exit 0。本轮 Task 18/19 改动 7 个文件（399 insertions, 72 deletions），受影响流程包括 `ProcessPoint → NormalizeAxisName`、`ProcessPoint → OnMotionSafetyFailure` 等，与 Task 18 MotionSafety 契约对齐 + Task 19 parser 多括号支持的批准计划一致。
- [x] 审查报告每个项目更新为 Fixed、Closed with Evidence 或 Blocked，附测试/命令证据。
  - R-5（Task 18）→ Fixed；N-2（Task 19）→ Fixed。review 总览表 Pending 项清零。
- [ ] 真实硬件急停和运动停止仍为发布前人工 HIL 门禁，未执行时不得声明整体发布就绪。
  - HIL Release Gate 由具备设备与安全条件的维护人员在发布前执行，自动测试不得代替 HIL 结论。

## Open Questions

以下问题必须在 Phase 2 Plan 前确定；规格获批不等于默认选择某个实现：

1. **已决定 - 后端气动数据载体**：扩展既有 `calibration.Status`，新增可选 physics 快照。`GET /api/calibration/status` 和 Wails `CalibrationStatus()` 保持路径、方法名和既有字段不变；HTTP/Wails 共用该载体。
2. **已决定 - 零流量语义**：`Pt == Ps` 是有效无流状态，后端统一返回 `machNumber=0`、`velocity=0`。前端将 0 作为有效数值展示，而非缺失值。
3. **已决定 - demo 模拟功能**：删除无可见调用入口的 traversal simulation composable、模拟算法和嵌入校准数据；不保留开发专用入口。生产依赖图和 bundle 中不得保留这些模块。
4. **已决定 - 校准配置解码边界**：嵌套 channel 到 core config 的转换属于 HTTP/Wails 传输边界。将 DTO 及 `ToCore` 移至 `pkg/types`，由 HTTP 与 Wails 共用；不新增 `CalibrationConfigDecoder` port，也不向 usecase 传递 raw JSON。
5. **已决定 - autoEngine shutdown 契约**：现有 `Start/Stop` 不保证退出且存在会话替换竞态。引入 manager-owned run session、可取消 engine 执行和有界 join；超时后禁止新任务接管资源。
6. **已决定 - DataStreamRelay Stop 语义**：`Stop` 返回前必须确认每个 subscription 已完成 `unsub()`；该路径只涉及内存 hub，使用无超时的取消后等待，且不得持 relay mutex 等待。
7. **待人工执行 - HIL**：真实急停失败、fallback 普通停止和控制器复位由具备设备与安全条件的维护人员在发布前执行；结果记录位置由发布负责人确定。自动测试不得代替 HIL 结论。
8. **已决定 - HTTP 状态轮询**：允许为 HTTP 模式新增与 Wails 等价的 calibration status 轮询，这是删除前端 physics 公式的必要配套。轮询复用既有 `/api/calibration/status`，不得重叠请求或新增 endpoint。
9. **已决定 - 七孔 pointCount 兼容性**：本轮保持现有响应值内区 169、外区 52，不改为动态真实计数。真实点数语义作为独立后续项记录。

## Phase Gate

进入 Phase 2 Plan 前必须同时满足：

- [x] 规格包含 Objective、Commands、Project Structure、Code Style、Testing Strategy、Boundaries。
- [x] 成功标准具体且可测试。
- [x] Always / Ask First / Never 边界已定义。
- [x] 规格已保存到仓库工作区。
- [x] 用户已审阅并批准本规格。
- [x] Open Questions 已由代码调查收敛；HIL 责任人为发布前人工门禁。
- [x] 用户已审阅并批准 Phase 2 实施计划及全部审批门禁。
- [ ] 用户已审阅并批准 Phase 3 任务清单。
