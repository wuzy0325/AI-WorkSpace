# AGENTS.md - 统一重构项目协作与编码规范
1
## 1. 目标与范围

本规范用于新系统（Vue3 前端 + Go 后端 API）的全生命周期开发。

目标：
- 代码整洁、结构清晰、可读性高。
- 注释完整且可维护，优先中文说明。
- 适度使用面向对象和简单设计模式，避免为“设计”而设计。

### 参考旧模块路径

本项目是对以下两个旧模块的功能融合重构：

1. **计量模块（1605MeassureApp）**
   - 路径：`C:\Users\wuzhy\Documents\D\SVN\SoftWare\AI Engineering\Measurement\1604 Measurement\1605MeassureApp`
   - 功能：设备连接、实时数据采集、单位检查

2. **标定模块（1604标定软件）**
   - 路径：`C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\标定软件\1604标定软件`
   - 功能：标定流程控制、报告生成、校准计算

---

## 2. 规范来源（GitHub 上游）

以下规范作为本项目的基线参考，冲突时以“可读性与可维护性优先”：
- Go 风格：`https://github.com/uber-go/guide`
- Go 命名与实践：`https://google.github.io/styleguide/go/`
- Vue 风格（Priority A/B）：`https://github.com/vuejs/docs/tree/main/src/style-guide`
- Go 项目布局参考：`https://github.com/golang-standards/project-layout`

说明：`project-layout` 仅作目录组织参考，不机械照搬。

---

## 3. 核心原则（强制）

1. 可读性优先于技巧性。
2. 简单优先于复杂，稳定优先于炫技。
3. 一个函数只做一件事，一个模块只承载一个核心职责。
4. 先定义边界与契约，再写实现。
5. 代码必须便于新成员在短时间内理解与接手。

---

## 4. 注释与文档规范（强制）

### 4.1 中文注释要求
- 公开类型、公开函数、关键领域对象必须有中文注释。
- 复杂逻辑必须解释“为什么这样做”，不是只描述“做了什么”。
- 协议命令、状态机迁移、容错策略、单位换算等关键点必须写注释。

### 4.2 注释质量要求
- 注释与代码同步更新，过期注释视为缺陷。
- 避免无价值注释（如“给变量赋值”）。
- 同一文件内术语保持一致（例如：采集、校准、回程、稳定时间）。

### 4.3 文档沉淀
- 重要架构决策记录到 `docs/plans/`。
- 每个阶段必须更新“功能覆盖矩阵”和“风险清单”。

---

## 5. Go 后端规范

### 5.1 代码风格
- 必须通过 `gofmt` 与 `goimports`。
- 包名简短、全小写，不使用下划线。
- 错误信息小写开头，不以句号结尾。
- 优先早返回，减少嵌套层级。

### 5.2 错误处理
- 禁止忽略错误（除非有明确注释说明且确实安全）。
- 错误只处理一次：要么就地降级，要么包装后上抛。
- 使用统一错误码与错误响应结构。

### 5.3 并发与资源
- 每个设备连接使用独立命令队列，避免并发乱序写入。
- 所有 I/O 操作必须带 `context`、超时与取消能力。
- goroutine 必须可控退出，禁止“fire-and-forget”泄漏。

### 5.4 结构约束
- 推荐结构：`cmd/`、`internal/`、`web/`、`docs/`。
- 领域接口定义在上层，具体实现下沉到基础设施层。
- 禁止跨层直接访问底层细节。

---

## 6. Vue3 前端规范

### 6.1 组件与状态
- 组件名使用多单词 PascalCase。
- `props` 必须写类型定义，禁止“裸数组 props”。
- `v-for` 必须提供稳定 `key`。
- 禁止同一元素同时使用 `v-if` 与 `v-for`。

### 6.2 页面与交互
- 页面按业务域拆分，避免“超大页面组件”。
- Store 只处理状态与业务动作，不做 UI 杂务。
- API 调用集中到 service 层，组件不直接拼接接口细节。

### 6.3 可维护性
- 样式按组件作用域隔离，避免全局污染。
- 命名统一：页面、组件、store、composable 的语义要一致。

---

## 7. UI 设计规范

UI 设计规范独立维护于 [DESIGN.md](./DESIGN.md)。

---

## 8. 面向对象与设计模式策略

### 7.1 面向对象使用原则
- 仅在“实体有稳定边界和行为”时使用对象封装（如设备、会话、报告任务）。
- 优先组合，谨慎继承。
- 通过小接口隔离变化点，避免巨型接口。

### 7.2 允许使用的简单模式
- Adapter：协议适配（1604、811A、820）。
- Factory：按设备型号创建驱动。
- Strategy：同类设备的命令差异处理。
- State：采集会话状态机。
- Repository：配置和会话持久化抽象。

### 7.3 禁止事项
- 禁止为追求“模式完整”引入复杂抽象层。
- 禁止出现“读不懂但很优雅”的过度设计。

---

## 9. 质量门禁（提交前必须通过）

### 8.1 Go
- `go test ./...`
- `go vet ./...`
- `staticcheck ./...`（如已接入）

### 8.2 Vue
- `npm run typecheck`
- `npm run lint`
- 关键流程最少一条端到端用例（后续接入时执行）

### 8.3 评审检查项
- 是否易读？
- 是否有必要注释且中文清晰？
- 是否满足单一职责？
- 是否存在过度抽象/过度模式化？

---

## 10. AI 协作流程（跨 Session）

- 所有关键设计决策必须落地到 `docs/plans/`。
- 在上下文接近上限时，必须主动提示用户开新 session。
- 提示时需附带“续接摘要”：
  - 已确认决策
  - 未决事项
  - 下一步执行入口
  - 相关文档路径

---

## 11. 执行口径

本文件是当前项目的执行基线。若与历史习惯冲突，以本文件为准；若需要例外，必须在评审中写明原因与影响范围。

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **1604Cal** (6240 symbols, 12891 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> If any GitNexus tool warns the index is stale, run `npx gitnexus analyze` in terminal first.

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `gitnexus_impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `gitnexus_detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `gitnexus_query({query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `gitnexus_context({name: "symbolName"})`.

## Never Do

- NEVER edit a function, class, or method without first running `gitnexus_impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `gitnexus_rename` which understands the call graph.
- NEVER commit changes without running `gitnexus_detect_changes()` to check affected scope.

## Resources

| Resource | Use for |
|----------|---------|
| `gitnexus://repo/1604Cal/context` | Codebase overview, check index freshness |
| `gitnexus://repo/1604Cal/clusters` | All functional areas |
| `gitnexus://repo/1604Cal/processes` | All execution flows |
| `gitnexus://repo/1604Cal/process/{name}` | Step-by-step execution trace |

## CLI

| Task | Read this skill file |
|------|---------------------|
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->
