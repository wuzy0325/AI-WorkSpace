# 文档索引

> 本文件是所有规范和指南的统一入口。按角色查找你需要的内容。

---

## 一、编码规范

| 文档 | 适用角色 | 内容 |
|------|---------|------|
| `runbooks/code-standards.zh-CN.md` | 开发者 / AI | 架构选型、目录结构、命名、注释、写法、复用规范 |
| `runbooks/ai-agent-execution-standard.zh-CN.md` | AI | AI 执行工作流标准 |
| `runbooks/development-rules.md` | 开发者 / AI | 开发规则（设计原则补充） |

**快速定位：**
- 新项目选架构 → `code-standards.zh-CN.md` 第一章
- 文件/函数该多长 → `code-standards.zh-CN.md` 第二章
- 命名规则 → `code-standards.zh-CN.md` 第三章
- 注释怎么写 → `code-standards.zh-CN.md` 第四章

---

## 二、架构规则

| 文档 | 内容 |
|------|------|
| `CLAUDE.md`（工作空间根目录） | 架构总纲、六边形硬约束、决策树、设计原则 |
| `architecture/workspace-engineering-rules.zh-CN.md` | 工作空间工程规则总览：前后端分离、后端规则、UI 设计规则、编码规则、良好架构标准 |
| `architecture/ai-context-loading.zh-CN.md` | AI 渐进式上下文加载协议：适配 OpenCode / Claude Code 的启动轻量化与按需加载流程 |
| `architecture/ai-document-responsibility-matrix.zh-CN.md` | AI 文档职责矩阵：定义 AGENTS、CLAUDE、README、专题文档、项目文档各自该承载什么 |
| `architecture/ai-task-context-map.zh-CN.md` | AI 任务上下文加载速查表：按任务类型列出应加载的文档、源码和验证上下文 |
| `architecture/module-design.md` | Go 包和 Vue 3 模块设计细节 |
| `architecture/project-variants.md` | 当前工作空间允许的项目结构变体 |
| `decisions/` | 架构决策记录（ADR） |

**快速定位：**
- 想按任务类型判断 AI 该加载什么 → `architecture/ai-task-context-map.zh-CN.md`
- 想治理 AI 文档分工和避免重复 → `architecture/ai-document-responsibility-matrix.zh-CN.md`
- 想优化 AI 启动规则加载方式 → `architecture/ai-context-loading.zh-CN.md`
- 想看一份整合版总规则 → `architecture/workspace-engineering-rules.zh-CN.md`
- 六边形架构约束 → `CLAUDE.md` Hard Constraints
- 代码放哪里 → `CLAUDE.md` Decision Tree
- 项目结构验证 → `scripts/validate-structure.ps1`

---

## 三、工作空间规则

| 文档 | 内容 |
|------|------|
| `AGENTS.md`（工作空间根目录） | AI Agent 快速参考（架构 + 硬约束 + 命令） |
| `runbooks/workspace-directory-rules.zh-CN.md` | 目录规则 |

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
| `scripts/validate-structure.ps1` | 结构合规检查 | `powershell -File .\scripts\validate-structure.ps1` |
| `scripts/lint-go.ps1` | Go 代码检查（gofmt + build） | `powershell -File scripts/lint-go.ps1` |
| `scripts/new-project.ps1` | 创建新项目骨架 | `powershell -File scripts/new-project.ps1 -Name foo` |

---

## 六、快速决策：遇到问题该看什么

```
遇到编译错误？
  → 先看项目 AGENTS.md -> Pre-submit Checklist
  → 跑 go build / gofmt

不知道代码放哪个目录？
  → CLAUDE.md -> Decision Tree

不确定怎么命名？
  → code-standards.zh-CN.md -> 第三章 命名规范

不确定注释怎么写？
  → code-standards.zh-CN.md -> 第四章 注释规范

新增功能怕改崩现有逻辑？
  → code-standards.zh-CN.md -> 第一/五/七章（分层 + 写法 + AI友好）

修改硬件驱动？
  → CLAUDE.md -> Hard Constraints（adapters/hardware/ 零业务逻辑）
  → ports/ 接口定义 -> 项目 docs/STRUCTURE.md
```
