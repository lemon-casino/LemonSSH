# Agent disposition 提案（已全部接受：WV3-014～018，2026-09-14）

日期：2026-09-14。状态：**已接受**。产品 owner 于 2026-09-14 批准全部五条建议；正式决定见 [decisions.md](../decisions.md) 的 WV3-014（拒绝 Cursor 内嵌 Bun）、WV3-015（拒绝 OpenCode 内嵌 Bun）、WV3-016（退休 Copilot）、WV3-017（退休 CodeBuddy）、WV3-018（退休 Cursor CLI login）。本文保留作为决定的证据基础与落地顺序说明。

证据基线：[baselines/external-agent-protocols.md](../baselines/external-agent-protocols.md) §4/§11、[baselines/ai-phase7-parity.md](../baselines/ai-phase7-parity.md) §7（commit `4a641b5e` 时点）。共同硬约束：WV3-002（运行时排除 Electron 与 Node）、WV3-011～013（三平台 release target）、`internal/agent/process` 的 provenance 规则（拒绝 Node shebang/npm/npx/wrapper，递归 Node child 阻断 verified）。

## 1. 逐类提案

### 1.1 `agent-runtime:cursor-bun` — 建议：拒绝（reject）

- 现状：Cursor API key 模式经 `@cursor/sdk` 在 Electron 内嵌 Bun 桥接运行；`sdk.v1` Connect/protobuf 协议本身可由 Go 实现，但当前可执行入口依赖内嵌 Bun runtime。
- 建议：拒绝在 Wails 版本中接受内嵌 Bun runtime。后果：Cursor API key adapter 在 Phase 7 保持退休态；`aiSdkAgent*` 中 Cursor 相关方法返回 typed unavailable。
- 重开条件：Cursor 官方提供非 Bun 的 native 可执行入口（含可验证 provenance 与 `sdk.v1` 兼容）时，以新 decision 重开。

### 1.2 `agent-runtime:opencode-bun` — 建议：拒绝（reject）

- 现状：OpenCode 以 `opencode serve`（内嵌 Bun 的二进制）+ HTTP/OpenAPI/SSE 提供集成；Go client 技术上可行，但 server 进程本身是 Bun 可执行体。
- 建议：拒绝内嵌 Bun runtime。后果：OpenCode adapter 退休；设置页明示不可用原因，历史会话保持可读。
- 重开条件：OpenCode 提供非 Bun native 分发，且 loopback 随机端口/认证/进程隔离通过 provenance 审查。

### 1.3 `agent-disposition:copilot` — 建议：退休（retire）

- 现状：`@github/copilot-sdk` + Copilot CLI，运行时内嵌 Node（P0-05 判定 `retirement-decision`）；无官方非 Node runtime。
- 建议：退休。后果：Copilot 从 Wails 版本的外部 Agent 列表移除；历史会话只读；`aiSdkAgent*` 对应方法返回 typed unavailable；不产生付费调用。
- 重开条件：GitHub 提供非 Node 的 Copilot agent runtime，并通过 `internal/agent/process` provenance 审查。

### 1.4 `agent-disposition:codebuddy` — 建议：退休（retire）

- 现状：`@tencent-ai/agent-sdk` + Node CLI；ACP/headless 语义存在但可执行体是 Node（P0-05 判定 `retirement-decision`）。
- 建议：退休。后果与 Copilot 相同（typed unavailable、历史只读、无付费调用）。
- 重开条件：腾讯提供非 Node 可执行入口。

### 1.5 `agent-disposition:cursor-cli` — 建议：退休（retire）

- 现状：`cursor-agent` stream-json；Windows 路径实为 `node.exe + index.js`（P0-05 判定 `needs-upstream-protocol`）：无可验证的非 Node ACP runtime，且 CLI 的 model catalog 与 `supportedModels()` 等价性未证明。
- 建议：退休。后果：Cursor CLI login 模式移除；Cursor API key 模式按 1.1 一并不可用——Wails 版本中 Cursor 品牌的两个入口都不可用，设置页给出统一说明。
- 重开条件：上游提供非 Node 的 authenticated ACP runtime 且 model catalog parity 可证。

## 2. 批准后的落地顺序

1. decisions.md 新增五条 accepted decision（Categories 精确匹配上表五个类别）。
2. capability-matrix AI-04 建子行：Codex App Server（保留，继续 W17）、Claude（保留候选，W18 待 provenance/catalog）、Grok ACP（保留候选，W19 待 conformance 设计）、五个退休项（`retired`，引用 decision + scope-removal 证据）。
3. AgentPort 映射表（ai-phase7-parity.md §5）标注退休项的 typed unavailable 目标 seam；历史数据按 data-inventory 只读保留。
4. UI（五语言）：外部 Agent 设置页移除退休项的可用状态，给出不可用原因；不得留"假开关"。

## 3. 明确不属于本提案的事项

- 不改变 AI-04 aggregate 的 `not-started` 状态（子行建立后仍逐行取证）。
- 不删任何历史数据、不迁移删除 `netcatty_ai_external_agents_v1` 中已退休 vendor 的历史记录。
- 不修改 decisions.md——本文件只是提案。
