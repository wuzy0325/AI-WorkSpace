---
name: gitnexus-guide
description: "Use when the user asks about GitNexus itself — available tools, how to query the knowledge graph, MCP resources, graph schema, or workflow reference. Works in Claude Code, OpenCode, Trae, Cursor, Codex, CodeBuddy, Qoder. Examples: \"What GitNexus tools are available?\", \"How do I use GitNexus?\""
---

# GitNexus Guide

Quick reference for all GitNexus MCP tools, resources, and the knowledge graph schema.
Compatible with Claude Code, **OpenCode**, **Trae**, Cursor, Codex, and any AI coding assistant that supports MCP.

## Always Start Here

For any task involving code understanding, debugging, impact analysis, or refactoring:

1. **Read `gitnexus://repo/{name}/context`** — codebase overview + check index freshness
2. **Match your task to a skill below** and **read that skill file**
3. **Follow the skill's workflow and checklist**

> If step 1 warns the index is stale, run `node .gitnexus/run.cjs analyze` in the terminal first — or `npx gitnexus analyze` if the local runner is not yet generated.

## Skills

| Task                                         | Skill to read       |
| -------------------------------------------- | ------------------- |
| Understand architecture / "How does X work?" | `gitnexus-exploring`         |
| Blast radius / "What breaks if I change X?"  | `gitnexus-impact-analysis`   |
| Trace bugs / "Why is X failing?"             | `gitnexus-debugging`         |
| Rename / extract / split / refactor          | `gitnexus-refactoring`       |
| Tools, resources, schema reference           | `gitnexus-guide` (this file) |
| Index, status, clean, wiki CLI commands      | `gitnexus-cli`               |

## Tools Reference

| Tool             | What it gives you                                                        |
| ---------------- | ------------------------------------------------------------------------ |
| `gitnexus_query`          | Process-grouped code intelligence — execution flows related to a concept |
| `gitnexus_context`        | 360-degree symbol view — categorized refs, processes it participates in  |
| `gitnexus_impact`         | Symbol blast radius — what breaks at depth 1/2/3 with confidence         |
| `gitnexus_trace`          | Shortest path between two symbols — "how does A reach B?" in one call    |
| `gitnexus_detect_changes` | Git-diff impact — what do your current changes affect                    |
| `gitnexus_rename`         | Multi-file coordinated rename with confidence-tagged edits               |
| `gitnexus_cypher`         | Raw graph queries (read `gitnexus://repo/{name}/schema` first)           |
| `gitnexus_explain`        | Persisted taint findings — source→sink data flows (needs `analyze --pdg`)|
| `gitnexus_pdg_query`      | Control/data dependence — what gates X / where Y flows (needs `analyze --pdg`) |
| `gitnexus_check`          | Check graph invariants such as circular imports                          |
| `gitnexus_list_repos`     | Discover indexed repos (paginated — `limit`/`offset`)                    |

### gitnexus_list_repos pagination

`gitnexus_list_repos` is paginated. It takes optional `limit` (default 50, max 200) and `offset`:

```
gitnexus_list_repos {}                 → repos 1-50,    nextOffset 50,  hasMore true
gitnexus_list_repos({ offset: 50 })    → repos 51-100,  nextOffset 100, hasMore true
```

Returns `{ repositories: [...], pagination: { total, limit, offset, returned, hasMore, nextOffset } }`.

### gitnexus_trace — shortest path between two symbols

`gitnexus_trace` answers "how does A reach B?" in one call instead of chaining multiple `gitnexus_context`/`gitnexus_impact` hops:

```
gitnexus_trace({ from: "validateUser", to: "executeQuery" })
→ status: ok, hopCount: 3
→ hops: validateUser → checkPermissions → dbQuery → executeQuery
```

When no path exists, reports the furthest reachable node. Supports `maxDepth` (default 10, max 30) and `includeTests`.

### gitnexus_explain — taint findings (needs `analyze --pdg`)

```
gitnexus_explain {}                               ← all findings
gitnexus_explain({ target: "src/vuln.ts" })       ← findings in a file
gitnexus_explain({ target: "runUserCommand" })    ← findings in a function
```

Intra-procedural only — absence of finding is not proof of safety.

### gitnexus_pdg_query — control & data dependence (needs `analyze --pdg`)

```
gitnexus_pdg_query({ mode: "controls", target: "processPayment" })    ← CDG: what gates X
gitnexus_pdg_query({ mode: "flows", target: "processPayment", variable: "amount" }) ← REACHING_DEF
```

### gitnexus_check — structural invariants

```
gitnexus_check()                                 ← circular imports, orphaned symbols, etc.
```

## Resources Reference

Lightweight reads (~100-500 tokens) for navigation:

| Resource                                       | Content                                   |
| ---------------------------------------------- | ----------------------------------------- |
| `gitnexus://repo/{name}/context`               | Stats, staleness check                    |
| `gitnexus://repo/{name}/clusters`              | All functional areas with cohesion scores |
| `gitnexus://repo/{name}/cluster/{clusterName}` | Area members                              |
| `gitnexus://repo/{name}/processes`             | All execution flows                       |
| `gitnexus://repo/{name}/process/{processName}` | Step-by-step trace                        |
| `gitnexus://repo/{name}/schema`                | Graph schema for Cypher                   |
| `gitnexus://group/{name}/contracts`            | Cross-repo contracts (multi-repo groups)  |
| `gitnexus://group/{name}/status`               | Staleness of repos in a group             |

## Graph Schema

**Nodes:** File, Function, Class, Interface, Method, Community, Process
**Edges (via CodeRelation.type):** CALLS, IMPORTS, EXTENDS, IMPLEMENTS, DEFINES, MEMBER_OF, STEP_IN_PROCESS, HAS_METHOD, HAS_PROPERTY, ACCESSES

```cypher
MATCH (caller)-[:CodeRelation {type: 'CALLS'}]->(f:Function {name: "myFunc"})
RETURN caller.name, caller.filePath
```
