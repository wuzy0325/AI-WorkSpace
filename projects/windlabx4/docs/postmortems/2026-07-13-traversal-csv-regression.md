# 遍历测试启动失败回归分析

## 错误信息
```
Failed to start traversal test:创建遍历 CSV文件失败:open C:\Users\wuzhy\Pictures:is a directory
```

## 根因
`os.OpenFile` 收到的路径是一个**目录**（`C:\Users\wuzhy\Pictures`），而非文件，
因此底层返回 `EISDIR`（"is a directory"）。

失败点位于 v2 可靠存储的 `csvPort.Open` 调用链：
- `internal/usecase/traversal.go:436`  `CSVPath: config.SavePath`
- `internal/usecase/traversal.go:495`  `csvSession.Path: snapshot.CSVPath`（即 `config.SavePath` 原样传入）
- `internal/adapters/storage/traversal_csv_writer.go:161-163`
  `os.OpenFile(path, O_CREATE|O_WRONLY|O_EXCL, ...)` → 路径是目录 → 报错

**关键不一致**：
- 旧（legacy）sink 路径 `InitializeTraversal` → `resolveOutputPath(cfg)`
  会把目录型 `SavePath` 自动拼接文件名，得到 `C:\Users\wuzhy\Pictures\traversal_<taskid>.csv`（正确）。
- 新 v2 `csvPort.Open` **没有**走 `resolveOutputPath`，直接把 `config.SavePath`
  （前端传来的目录）当成完整文件路径去创建 → 失败。

前端/桌面端传入的 `savePath` 是目录（`C:\Users\wuzhy\Pictures`），符合
`resolveOutputPath` 的设计约定，但与 v2 csvPort 的假设（SavePath 应为完整 .csv 文件路径）冲突。

## 引入回归的提交
| 项 | 值 |
|---|---|
| Commit | `a382d7c194b138d74771c2b157f44da8d3a08712` |
| 作者 | wuzy0325 <wuzynovo@163.com> |
| 时间 | 2026-07-12 21:26:08 (+0800) |
| 标题 | `feat(WindLabX4/traversal): 遍历测试可靠性存储与崩溃恢复` |
| 规模 | 26 files changed, 3208 insertions(+), 117 deletions(-) |

该提交同时引入了三处失败点（`git blame` 均指向同一 commit）：
1. `pkg/appcontext/context.go:98`  `travSinkV2 := storage.NewTraversalCsvWriter()` 并 `SetCsvPort(travSinkV2)`
2. `internal/usecase/traversal.go:436`  `CSVPath: config.SavePath`
3. `internal/usecase/traversal.go:495`  `Path: snapshot.CSVPath`

### 改动前后对比（context.go 注入点）
- **改动前**：`NewTraversalManager(hub, motionMgr, nil, travStore, ...)` —— sink 第 3 个参数为 `nil`
  → `csvPort == nil` → `if csvPort != nil` 不成立 → `csvPort.Open` 从未被调用
  → 即使 `SavePath` 是目录也**不会报错**（只是静默不输出 CSV，见文件内注释）。
- **改动后**：注入 `travSinkV2` 并 `SetCsvPort(travSinkV2)`
  → `csvPort.Open(session.ctx, csvSession)` 被激活 → 用原始目录路径创建文件 → 抛出 "is a directory"。

## 结论
回归由提交 **a382d7c1**（"遍历测试可靠性存储与崩溃恢复"）引入。该提交的目的是
修复"桌面端遍历测试静默不输出 CSV"的问题，但顺带把 v2 `csvPort` 接入后，
其 `Open` 路径直接使用了未经 `resolveOutputPath` 解析的目录型 `SavePath`，
从而暴露了原本被 `nil` sink 掩盖的 "is a directory" 故障。

## 修复方向（已实施 — 2026-07-13）

根因已修复。改动要点：

1. **新增领域层路径解析函数** `internal/core/traversal/output_path.go`：
   - `ResolveOutputPath(cfg)`：按 `SavePath`(目录) + `SaveFileName`(文件名) 拼接完整 CSV 路径，
     处理空路径回退、`.csv` 后缀自动补全、`SavePath` 已带 `.csv` 视为完整路径三种情况。
   - `ResolveResultLogPath(cfg)`：返回与 CSV 同目录、同 stem 的 `.results.jsonl` 路径，
     修正原 `config.SavePath + ".results.jsonl"` 把结果日志错放到父目录的问题。

2. **v2 初始化使用正确路径** `internal/usecase/traversal.go`：
   ```go
   CSVPath:       traversal.ResolveOutputPath(config),
   ResultLogPath: traversal.ResolveResultLogPath(config),
   ```
   替代原 `CSVPath: config.SavePath`（目录）与 `config.SavePath + ".results.jsonl"`。

3. **消除重复逻辑**：`internal/adapters/storage/traversal_csv_writer.go` 的
   旧 `resolveOutputPath` 改为委托 `traversal.ResolveOutputPath`，单一来源。

4. **回归测试**：`internal/core/traversal/output_path_test.go` 锁定拼接规则，
   防止再次把目录当文件创建。

验证：`go build` / `go vet` 通过；`internal/core/traversal`、`internal/usecase`、
`internal/adapters/storage` 三个包的 `go test` 全部通过。

撞名行为（已优化 — 2026-07-13 追加）：同一天、同名重跑时，v2 `Open` 路径现已
**自动追加 `-2/-3` 后缀另存新文件**（复用 legacy 的 `createUniqueFile` 逻辑），
既不静默覆盖历史数据，也不再报错拒绝启动：

- `internal/adapters/storage/traversal_csv_writer.go`：新增 `openCreateUnique(path)`，
  先 `O_EXCL` 创建，遇 `os.ErrExist` 时委托 `createUniqueFile` 自动编号 `-2/-3`；
  `openCreateLocked` 改用它，清理失败文件时也用最终路径。
- `internal/adapters/storage/traversal_result_log.go`：`Open` 在 Create 模式下同样走
  `openCreateUnique`，保证结果日志与 CSV 同步编号，端到端同天重跑不再报错。

回归测试：`traversal_unique_file_test.go` 锁定 CSV 与结果日志撞名时分别生成
`run-2.csv` / `run.results-2.jsonl`、二次重跑到 `run-3.csv`，且原文件内容不被覆盖。

注：前端 `TraversalSettings.vue` 仍分开传 `savePath`(目录) 与 `saveFileName`(文件名)，
与 `ResolveOutputPath` 约定一致，无需改动。
