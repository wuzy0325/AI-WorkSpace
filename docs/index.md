# 文档索引

> 本文件是所有规范和指南的统一入口。按角色查找你需要的内容。
> 文档分层：L0 入口（AGENTS / CLAUDE） → L1 总览 → L2 专题 → L3 任务映射 → L4 决策（ADR）

---

## 一、编码规范

| 文档 | 适用角色 | 内容 |
|------|---------|------|
| `runbooks/code-standards.zh-CN.md` | 开发者 / AI | 通用编码规范：文件结构、命名、注释、写法、复用、AI 友好 |
| `runbooks/ai-agent-execution-standard.zh-CN.md` | AI | AI 执行 SOP（流程模板、结构变更申请、汇报模板） |
| `runbooks/frontend-ai-rules.zh-CN.md` | AI / 前端开发者 | 前端规则薄索引（已拆分为 foundation/state/quality/deploy 4 个主题文件，每个 ≤ 8k token） |
| `runbooks/frontend-directory-rules.zh-CN.md` | AI / 前端开发者 | Vue 前端目录结构标准与文件放置规则 |
| `runbooks/development-rules.md` | 开发者 / AI | 开发流程规则：依赖边界、测试、结构变更、复用优先、验证 |
| `runbooks/win7-lts-development-workflow.md` | 开发者 / AI | master 与 lts/win7 日常开发、选择性同步、验证、回滚和发布流程 |

**快速定位：**
- 文件/函数该多长 → `code-standards.zh-CN.md` §一
- 命名规则 → `code-standards.zh-CN.md` §二
- 注释怎么写 → `code-standards.zh-CN.md` §三
- 复用与抽象 → `code-standards.zh-CN.md` §五
- 前端单文件体量阈值 → `frontend-ai-rules-deploy.zh-CN.md` §28.1
- 前端硬编码颜色禁令 → `frontend-ai-rules-deploy.zh-CN.md` §28.2
- TypeScript 严格性 → `frontend-ai-rules-quality.zh-CN.md` §18
- 命名规范（前端） → `frontend-ai-rules-quality.zh-CN.md` §19
- 注释规范（前端） → `frontend-ai-rules-quality.zh-CN.md` §20
- 函数复杂度 → `frontend-ai-rules-quality.zh-CN.md` §21
- 错误处理 → `frontend-ai-rules-quality.zh-CN.md` §22
- 输入校验 → `frontend-ai-rules-quality.zh-CN.md` §23
- 性能规范 → `frontend-ai-rules-quality.zh-CN.md` §24
- i18n → `frontend-ai-rules-quality.zh-CN.md` §25
- 生命周期与资源清理 → `frontend-ai-rules-quality.zh-CN.md` §26
- 并发与竞态 → `frontend-ai-rules-quality.zh-CN.md` §27
- 模块边界 → `frontend-ai-rules-foundation.zh-CN.md` §9
- import 组织 → `frontend-ai-rules-foundation.zh-CN.md` §10
- OOP 与领域模型 → `frontend-ai-rules-deploy.zh-CN.md` §31
- 状态机 → `frontend-ai-rules-state.zh-CN.md` §15
- AI 友好代码组织 → `frontend-ai-rules-deploy.zh-CN.md` §35
- Wails 桥接规范 → `frontend-ai-rules-deploy.zh-CN.md` §32
- Wails binding 同步 → `frontend-ai-rules-deploy.zh-CN.md` §32.1
- 测试规范 → `frontend-ai-rules-deploy.zh-CN.md` §33
- 前端验证要求 → `frontend-ai-rules-deploy.zh-CN.md` §34

---

## 二、架构规则

| 文档 | 内容 |
|------|------|
| `CLAUDE.md`（工作空间根目录） | 架构总纲、六边形硬约束、决策树、设计原则 |
| `architecture/workspace-engineering-rules.zh-CN.md` | 工程规则导航中枢：架构概览、前后端职责、代码放置决策表、文档导航、规则优先级 |
| `architecture/ai-context-loading.zh-CN.md` | AI 渐进式上下文加载协议：L1-L5 分层 + token 预算 + §10 章节级精读 + §11 自检协议 |
| `architecture/ai-document-responsibility-matrix.zh-CN.md` | AI 文档职责矩阵：定义 AGENTS、CLAUDE、README、专题文档、项目文档各自该承载什么 |
| `architecture/ai-task-context-map.zh-CN.md` | AI 任务上下文加载速查表：按任务类型列出应加载的文档、源码和验证上下文 |
| `architecture/module-design.md` | Go 包和 Vue 3 模块设计细节 |
| `architecture/project-variants.md` | 当前工作空间允许的项目结构变体 |
| `decisions/` | 架构决策记录（ADR） |
| `plans/2026-07-23-workspace-win7-lts-worktree.md` | 独立 Win7 LTS worktree 的 6 产品改造、同步、验收与发布计划 |

**快速定位：**
- 想按任务类型判断 AI 该加载什么 → `architecture/ai-task-context-map.zh-CN.md`
- 想治理 AI 文档分工和避免重复 → `architecture/ai-document-responsibility-matrix.zh-CN.md`
- 想优化 AI 启动规则加载方式 → `architecture/ai-context-loading.zh-CN.md` §3 token 预算 / §10 章节级精读 / §11 自检协议
- 想看工程规则总览与代码放置决策表 → `architecture/workspace-engineering-rules.zh-CN.md`
- 前端 UI / 控件 / 样式 / store / API client 修改 → `runbooks/frontend-ai-rules.zh-CN.md`
- 前端目录结构 / 文件放置 → `runbooks/frontend-directory-rules.zh-CN.md`
- 六边形架构约束 → `CLAUDE.md` Hard Constraints
- 代码放哪里 → `CLAUDE.md` Decision Tree 或 `architecture/workspace-engineering-rules.zh-CN.md` §3
- 项目结构验证 → `scripts/validate-structure.ps1`

---

## 三、工作空间规则

| 文档 | 内容 |
|------|------|
| `AGENTS.md`（工作空间根目录） | AI Agent 启动入口：架构 + 硬约束速查 + 命令 + 渐进加载协议 |
| `runbooks/workspace-directory-rules.zh-CN.md` | 工作空间目录规则 |

---

## 四、项目级补充

每个项目可在 `projects/<name>/AGENTS.md` 中覆盖或补充规则。

当前项目：
- `projects/wind-daq/AGENTS.md` — Wind-DAQ 构建命令 + 提交前检查
- `projects/wind-daq/docs/STRUCTURE.md` — Wind-DAQ 目录结构详解
- `projects/wind-daq/README.md` — Wind-DAQ 运行、构建、迁移入口
- `projects/daq-t1603/AGENTS.md` — DAQ-T-1603 项目级 AI 入口与渐进加载导航
- `projects/daq-t1603/README.md` — DAQ-T-1603 独立桌面应用入口
- `projects/daq-t1603/CLAUDE.md` — DAQ-T-1603 单 Go module 架构约束
- `decisions/ADR-007-daq-t1603-win7-lts.md` — DAQ-T1603 Win7 已验证技术基线和真机证据
- `decisions/ADR-008-workspace-win7-lts-worktree.md` — 独立 Win7 LTS worktree、产品范围和 AI 选择性同步策略
- `projects/daq-p1604/AGENTS.md` — DAQ-P-1604 项目级 AI 入口
- `projects/daq-p1604/README.md` — DAQ-P-1604 独立桌面应用入口
- `projects/motion-controller/AGENTS.md` — Motion Controller 项目级 AI 入口与渐进加载导航
- `projects/motion-controller/README.md` — Motion Controller 独立桌面应用入口
- `projects/motion-controller/SPEC.md` — Motion Controller 产品规范
- `projects/motion-controller/PLAN.md` — Motion 模块共享计划
- `projects/motion-controller/TASKS.md` — Motion 模块共享任务状态
- `projects/five-hole-interpolator/README.md` — 五孔探针插值工具入口
- `projects/five-hole-interpolator/SPEC.md` — 五孔探针插值工具规范
- `projects/three-hole-interpolator/README.md` — 三孔探针插值工具入口
- `projects/three-hole-interpolator/SPEC.md` — 三孔探针插值工具规范

---

## 五、脚本

| 脚本 | 用途 | 运行方式 |
|------|------|---------|
| `scripts/validate-structure.ps1` | 后端/整体结构合规检查 | `powershell -File .\scripts\validate-structure.ps1` |
| `scripts/validate-frontend-structure.ps1` | 前端目录结构合规检查（含 `-CheckFileSize` 量化检查） | `powershell -File scripts/validate-frontend-structure.ps1 -ProjectDir <path> [-CheckFileSize]` |
| `scripts/check-wails-bindings.ps1` | Wails binding 同步检查（方法名 diff + 时间戳） | `powershell -File scripts/check-wails-bindings.ps1 [-Projects wind-daq,daq-t1603] [-StaleMinutes 30]` |
| `scripts/check-naive-imports.ps1` | wind-daq naive-ui 直接导入检查 | `powershell -File scripts/check-naive-imports.ps1 -ProjectDir "projects/wind-daq/apps/desktop-wails/frontend/src"` |
| `scripts/validate-import-direction.ps1` | hexagonal 边界 import 方向检查 | `powershell -File scripts/validate-import-direction.ps1` |
| `scripts/lint-go.ps1` | Go 代码检查（gofmt + build） | `powershell -File scripts/lint-go.ps1` |
| `scripts/new-project.ps1` | 创建新项目骨架 | `powershell -File scripts/new-project.ps1 -Name foo` |
| `scripts/install-hooks.ps1` | 安装 git hooks | `powershell -File scripts/install-hooks.ps1` |
| `scripts/copy-release-artifacts.ps1` | 发布产物归档到 `releases/bin/` | `powershell -File scripts/copy-release-artifacts.ps1 -Project <name> [-Version <ver>] [-DryRun]` |
| `scripts/estimate-token.ps1` | AI 上下文 token 估算（对照 §3 预算） | `powershell -File scripts/estimate-token.ps1 -Path <file_or_dir> [-Recurse] [-Budget N]` |

### `-CheckFileSize` 量化检查项

`validate-frontend-structure.ps1 -CheckFileSize` 触发的检查（对应 `frontend-ai-rules.zh-CN.md` 章节）：

| 检查项 | 对应章节 |
|---|---|
| 单文件体量（行数 / script setup / scoped / 函数数） | §28.1 |
| 硬编码颜色（hex / rgba / hsl / blur） | §28.2 |
| 跨组件重复 CSS class | §28.2 |
| composable 缺失（store ≥ 4 或业务组件 ≥ 6 时未建 `composables/`） | §28.1 |
| TypeScript `: any` / `as any` / `@ts-ignore` | §18 |
| `.vue` 文件名非 PascalCase / composable 非 `useXxx` | §19 |
| `addEventListener` / `setInterval` / `setTimeout` / `Events.On` 未配对清理 | §26 |
| 空 catch 块 / `catch(e: any)` / 通用错误消息 | §22 |
| `reactive<T[]>` 反模式 | §12 |
| `v-for` 缺 `:key` / `v-if + v-for` 同元素 | §24 |
| 模板内硬编码中文 | §25 |
| `inject('xxx')` 字符串 key | §13 |
| options 风格 `defineStore` | §14 |
| TODO/FIXME 缺 owner | §20 |
| 函数参数 >6 | §21 |
| 非空断言 `!` / `JSON.parse` 无 try/catch | §23 |
| 跨 domain import | §9 |
| 通用变量名（data/info/manager/temp/obj/item） | §35 |

---

## 六、快速决策：遇到问题该看什么

```
遇到编译错误？
  → 先看项目 AGENTS.md -> Pre-submit Checklist
  → 跑 go build / gofmt

不知道代码放哪个目录？
  → CLAUDE.md -> Decision Tree
  → 或 architecture/workspace-engineering-rules.zh-CN.md §3 代码放置快速决策表

不知道前端规则在哪？
  → frontend-ai-rules.zh-CN.md 薄索引（按主题跳转 foundation/state/quality/deploy）

不确定怎么命名？
  → 通用：code-standards.zh-CN.md §二
  → 前端：frontend-ai-rules-quality.zh-CN.md §19

不确定注释怎么写？
  → 通用：code-standards.zh-CN.md §三
  → 前端：frontend-ai-rules-quality.zh-CN.md §20

新增功能怕改崩现有逻辑？
  → code-standards.zh-CN.md §一/§四/§六（分层 + 写法 + AI友好）
  → 前端：frontend-ai-rules-foundation.zh-CN.md §9 模块边界 + §10 import 组织

修改硬件驱动？
  → CLAUDE.md -> Hard Constraints（adapters/hardware/ 零业务逻辑）
  → ports/ 接口定义 -> 项目 docs/STRUCTURE.md

打包发布？
  → runbooks/release-versioning.zh-CN.md（版本号、CHANGELOG、release note、归档）

Wails binding 同步？
  → frontend-ai-rules-deploy.zh-CN.md §32.1（修改 Go 方法签名后必须 wails3 generate bindings）

估算文档 token / 检查是否超预算？
  → ai-context-loading.zh-CN.md §3 token 预算表 + §10 章节级精读 + §11 自检协议
  → scripts/estimate-token.ps1 -Path <file>
```
