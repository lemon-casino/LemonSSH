# AI Phase 7 parity baseline（P7-01 前置核对）

状态：**基线文档，不改变任何 capability 状态**。采集日期 2026-09-14，基线 commit `95a7ee74`（分支 `pre-acceptance-backlog`）。正式归属 P7-01 前置核对（W01）；门禁未开，本文只做文档/冻结基线分析，不创建 `internal/capability`、`internal/agent`、`internal/rpc`、`cmd/netcatty-mcp`、`cmd/netcatty-tool` 等生产 owner。

配套：范围与切片顺序见 [ai-migration-execution-plan.md](../ai-migration-execution-plan.md)；协议与 DTO 设计见 [ai-migration-technical-design.md](../ai-migration-technical-design.md)；工作包见 [ai-migration-work-packages.md](../ai-migration-work-packages.md)。治理状态以 [capability-matrix.md](../capability-matrix.md)、[migration-ledger.md](../migration-ledger.md)、[decisions.md](../decisions.md) 为准。

## 1. 门禁现状（2026-09-14 核实）

| 检查项 | 现状 |
| --- | --- |
| ledger 头 | `WV3-L132`（SYNC-02，2026-09-12，本文采集时点）；2026-09-14 已追加 `WV3-L133`（Wails 版本对齐，见 §10）；全文无 `NONAI-COMPLETE` 记录 |
| matrix AI 行 | `AI-01`～`AI-04` 全部 `not-started`，均注明 hard-blocked by Non-AI Completion Gate |
| Wails runtime client | `infrastructure/runtime/wails/wailsRuntimeClient.ts:1412` 仍为 `agent: unimplemented("agent")` |
| release-target 决策 | `WV3-011/012/013` 已接受（2026-09-08），覆盖 `release-target:windows/macos/linux` 三类 |
| agent 决策 | `agent-runtime:cursor-bun`、`agent-runtime:opencode-bun`、`agent-disposition:copilot`、`agent-disposition:codebuddy`、`agent-disposition:cursor-cli` 五类**均无 accepted decision**（decisions.md *Required Future Decisions*） |
| 文档 checker | `npm run check:migration-docs` 通过（0 fail） |

结论：AI 生产实施仍被阻断。本文是 P7-01 前置核对的产物，供门禁通过后的执行 AI 直接引用，不能当作开工授权。

## 2. 冻结基线清单与 hash

来源目录 `testdata/migration/electron/`（与冻结 Electron release carrier 一致性由 `manifest.json` 维护）。本次采集的 SHA-256：

| 文件 | 条目数 | SHA-256 |
| --- | --- | --- |
| `capability-catalog.json` | 77 | `ca782801a35ead530bd51506a418dcf1c084781787ac7f0673eeee6b19c1b3f8` |
| `agent-tool-specs.json` | 67 | `59d6a24dbecb1746abe675ffa74c9156b7daaddc88a574a9cba4d7cae94fdf3b` |
| `mcp-tool-specs.json` | 67 | `c32121f561c191fb98707776400b97a4394ca1f4caf45e76347dd8934cf9127d` |
| `cli-capabilities.json` | 50 | `198065c57228d2a1fb75d9781aee2a93738c24f659e84895cb30d01c8d187b98` |
| `bridge-contract-index.json` | — | `81baad3aa4ba84d32af1792631fc8c47fa62300dcc12d94a5c5492c8982c6782` |
| `manifest.json` | — | `6564afdc24000ad50f93cc8bdb9db0fbd8c9ee00f941829a1c1517ce6e0d195c` |

全部 77 条 catalog 条目 baseline status 均为 `implemented`（冻结基线语义）。这是目录条目数，**不是**每个 Agent 可见 77 个工具，更不是 Go 已实现 77 个 handler。

## 3. Capability catalog 基线（77 条）

### 3.1 域分布与 policy 标志统计

| 域 | 条目数 |
| --- | --- |
| attachment | 2 |
| harness | 6 |
| meta | 1 |
| portforward | 8 |
| session | 5 |
| sftp | 11 |
| terminal | 4 |
| vault | 40 |

policy 标志为 true 的条目数：`write`=42、`bypassesApproval`=37、`bypassesChatCancel`=37、`requiresChatSession`=33、`longRunning`=16、`sensitiveRead`=10、`bypassesObserverBlock`=2。

这七个布尔字段必须逐字段保留（技术设计 §2.1）；不能用单个 `write` 布尔重建策略。`bypassesApproval`/`bypassesChatCancel` 高密度集中在只读与控制类条目；`bypassesObserverBlock`=2 仅 `session.close`、`terminal.stop`。

### 3.2 surface / agentKind 投影

| 投影 | 条目数 | 说明 |
| --- | --- | --- |
| `builtin+cli` | 5 | attachment 2 + session.environment/get + meta.status（builtin 少、CLI 全量） |
| `catty` | 6 | 全部 harness.*（renderer-local，`surfaces.catty.toolName`） |
| `builtin+cli+public` | 17 | session/sftp/terminal 的 public MCP 面 |
| `global+public` | 21 | vault/portforward 的 global RPC + public MCP 面 |
| `cli+global+public` | 28 | vault.scripts/snippets 等三面齐全 |

显式 `agentKinds` 仅 1 条：`vault.host.open = ["global"]`（其余 76 条由 `resolveAgentKinds` 按 surface 推断）。Go authority 生成投影时必须保留这条显式优先规则。

投影计数对照：`agent-tool-specs.json` 67 条全部 `agentKind: "global"`；`mcp-tool-specs.json` 67 条全部 `cattyEnabled: false`；`cli-capabilities.json` 50 条命令。harness 6 条不在任何 RPC/MCP/CLI 投影中（只进 Catty sidebar specs）。

### 3.3 逐 ID 明细

policy 缩写：W=`write`、LR=`longRunning`、SR=`sensitiveRead`、CS=`requiresChatSession`、BA=`bypassesApproval`、BC=`bypassesChatCancel`、BO=`bypassesObserverBlock`；`—` 表示该投影无此面。

| ID | policy | MCP tool | rpcMethod | CLI | Catty |
| --- | --- | --- | --- | --- | --- |
| attachment.list | BA,BC,CS | list_attachments | netcatty/listAttachments | attachment list | — |
| attachment.read | BA,BC,CS,SR | read_attachment | netcatty/readAttachment | attachment read | — |
| harness.terminal.read_context | BA,BC,CS | — | — | — | terminal_read_context |
| harness.tool_output.read | BA,BC,CS | — | — | — | tool_output_read |
| harness.url.fetch | BA,BC,CS | — | — | — | url_fetch |
| harness.web.search | BA,BC,CS | — | — | — | web_search |
| harness.workspace.get_info | BA,BC,CS | — | — | — | workspace_get_info |
| harness.workspace.get_session_info | BA,BC,CS | — | — | — | workspace_get_session_info |
| meta.status | BA,BC | — | netcatty/getStatus | status | — |
| portforward.rules.create | W | — | — | — | — |
| portforward.rules.delete | W | — | — | — | — |
| portforward.rules.duplicate | W | — | — | — | — |
| portforward.rules.list | BA,BC | — | — | portforward rules list | — |
| portforward.rules.update | W | — | — | — | — |
| portforward.start | LR,W | — | — | portforward start | — |
| portforward.stop | W | — | — | portforward stop | — |
| portforward.tunnels.list | BA,BC | — | — | portforward tunnels list | — |
| session.cancel | BA,BC,CS | — | netcatty/setCancelled | cancel | — |
| session.close | BA,BC,BO,CS,W | — | — | — | — |
| session.environment | BA,BC,CS | get_environment | netcatty/getContext | env | — |
| session.get | BA,BC,CS | — | netcatty/getContext | session | — |
| session.resume | BA,BC,CS | — | netcatty/setCancelled | resume | — |
| sftp.chmod | LR,CS,W | — | netcatty/sftp/chmod | sftp chmod | — |
| sftp.delete | LR,CS,W | — | netcatty/sftp/delete | sftp delete | — |
| sftp.download | LR,CS,W | — | netcatty/sftp/download | sftp download | — |
| sftp.home | BA,BC,LR,CS,SR | — | netcatty/sftp/home | sftp home | — |
| sftp.list | BA,BC,LR,CS,SR | — | netcatty/sftp/list | sftp list | — |
| sftp.mkdir | LR,CS,W | — | netcatty/sftp/mkdir | sftp mkdir | — |
| sftp.read | BA,BC,LR,CS,SR | — | netcatty/sftp/read | sftp read | — |
| sftp.rename | LR,CS,W | — | netcatty/sftp/rename | sftp rename | — |
| sftp.stat | BA,BC,LR,CS,SR | — | netcatty/sftp/stat | sftp stat | — |
| sftp.upload | LR,CS,W | — | netcatty/sftp/upload | sftp upload | — |
| sftp.write | LR,CS,W | — | netcatty/sftp/write | sftp write | — |
| terminal.execute | LR,CS,W | terminal_execute | netcatty/exec | exec | — |
| terminal.poll | BA,BC,CS | terminal_poll | netcatty/jobPoll | job-poll | — |
| terminal.start | LR,CS,W | terminal_start | netcatty/jobStart | job-start | — |
| terminal.stop | BA,BC,BO,CS,W | terminal_stop | netcatty/jobStop | job-stop | — |
| vault.group.create | W | — | — | — | — |
| vault.group.delete | W | — | — | — | — |
| vault.group.list | BA,BC | — | — | — | — |
| vault.group.update | W | — | — | — | — |
| vault.host.connectScripts.list | BA,BC | — | — | vault host connect-scripts list | — |
| vault.host.connectScripts.set | W | — | — | vault host connect-scripts set | — |
| vault.host.delete | W | — | — | — | — |
| vault.host.get | BA,BC,SR | — | — | vault host get | — |
| vault.host.import | W | — | — | — | — |
| vault.host.list | BA,BC,SR | — | — | — | — |
| vault.host.notes.get | BA,BC,SR | — | — | vault host-notes get | — |
| vault.host.notes.set | W | — | — | vault host-notes set | — |
| vault.host.open | W（agentKinds: global） | — | — | vault host open | — |
| vault.host.update | W | — | — | — | — |
| vault.hosts.create | W | — | — | — | — |
| vault.identity.list | BA,BC,SR | — | — | — | — |
| vault.note.create | W | — | — | — | — |
| vault.note.delete | W | — | — | — | — |
| vault.note.get | BA,BC | — | — | — | — |
| vault.note.list | BA,BC | — | — | — | — |
| vault.note.update | W | — | — | — | — |
| vault.proxyProfile.list | BA,BC,SR | — | — | — | — |
| vault.scripts.create | W | — | — | scripts create | — |
| vault.scripts.delete | W | — | — | scripts delete | — |
| vault.scripts.get | BA,BC | — | — | scripts get | — |
| vault.scripts.list | BA,BC | — | — | scripts list | — |
| vault.scripts.reference | BA,BC | — | — | scripts reference | — |
| vault.scripts.run | LR,CS,W | — | — | scripts run | — |
| vault.scripts.run.pause | CS,W | — | — | scripts run pause | — |
| vault.scripts.run.resume | CS,W | — | — | scripts run resume | — |
| vault.scripts.run.stop | CS,W | — | — | scripts run stop | — |
| vault.scripts.runs.list | BA,BC | — | — | scripts runs list | — |
| vault.scripts.targets.set | W | — | — | scripts targets set | — |
| vault.scripts.update | W | — | — | scripts update | — |
| vault.snippets.create | W | — | — | snippets create | — |
| vault.snippets.delete | W | — | — | snippets delete | — |
| vault.snippets.get | BA,BC | — | — | snippets get | — |
| vault.snippets.list | BA,BC | — | — | snippets list | — |
| vault.snippets.run | LR,CS,W | — | — | snippets run | — |
| vault.snippets.update | W | — | — | snippets update | — |

注意：表中"—"是冻结基线 JSON 里该投影缺失，不是遗漏；`portforward.rules.*`、多数 `vault.*` 无 builtin/CLI 面是基线事实。生成 Go 投影时与上表 semantic diff 即可发现漂移。

## 4. Go 现状 substrate 与能力缺口

**总判定：`internal/capability` 不存在；77 条 ID 目前没有任何 Go capability dispatch owner。** 下表列出的只是可复用的底层 substrate，"方法名类似"不构成 handler 完成（P7-01 出口条件的反面清单）。

| 域 | 现有 Go substrate（文件:证据） | 缺失的 Agent 语义 |
| --- | --- | --- |
| terminal（4） | `TerminalService.Write/Resize/Signal/Close`（terminalService.go:807-876，PTY 原始字节）；`monitoringExec`（terminalMonitoring.go:32，仅监控内部用）；数据面 `internal/terminal/dataplane` | 有界 exec 契约（退出码/cwd/env/超时）、job owner + poll/stop、会话串行化、输出 handle、与 chat scope 的绑定、`TerminalService.Write` 不能证明命令完成（技术设计 §6.2） |
| sftp（11） | `SFTPService`：Open/List/Stat/Mkdir/Remove/Rename/Read/WriteText/HomeDir/Chmod/Download/Upload/ExtractArchive/UploadCompressedFolder/Close（sftpService.go:64-469），共享 SSH 池 | chat/terminal scope 校验、输出 handle（大结果 spill）、size/rate 边界、审批接入、sensitiveRead 的脱敏策略（host 名/路径进 trace 的规则） |
| portforward（8） | `ForwardService.Start/Stop/StopByRuleId/List/Snapshot/RuntimeSnapshot`（forwardService.go:66-200） | rules CRUD（create/delete/duplicate/update 是 vault 配置操作，当前无 Go owner 面）、longRunning 生命周期与 turn 取消的关系 |
| session（5） | `TerminalService.Close/Signal`；`netcatty/setCancelled`、`netcatty/getContext` 无 Go 实现 | session.get/environment/cancel/resume 的 Agent 契约（hostChain、activePortForwards 上下文由 `buildAITerminalSessionInfo` 在 renderer 组装，需迁 Go） |
| vault（40） | Profile store（bbolt CAS）+ vault facade（UI 用）；AGENTS.md 规则：vault bridge 永不返回 password/privateKey | Agent dispatch、secret 字段脱敏/引用化、`vault.host.get` 等 sensitiveRead 的字段级过滤、写入与 UI 的一致 revision |
| attachment（2） | 无 | 全部：专用 temp staging、count/size/MIME/hash 校验、scope lease |
| harness（6） | 无（renderer-local：toolOutputStore/contextManager 等） | Go runtime 内的 tool_output_read、url_fetch、web_search（无配置则不暴露）等 |
| meta（1） | `main.go` 骨架 health/version（`version = "0.0.0-wails-skeleton"`） | catalog 语义的 meta.status handler |

必须先解的依赖（执行方案 §2）：`cmd/netcatty` 的 Wails-facing service 不能被 `internal/capability` import；P7-01/02 先抽 shell-neutral use case。

## 5. AgentPort 61 方法 → 目标 seam

`infrastructure/runtime/generated/runtimePorts.ts:10`（生成文件，勿手改；生成源为 `bridge-contract-index.json` + `global.d.ts`）。61 个方法分组如下；逐方法调用点审计在 W01 验收时已抽查，完整调用图随 P7-04 W12 迁移时以 import 检查证据固化。

| 组 | 方法 | 目标 seam |
| --- | --- | --- |
| Catty chat/exec（7） | aiChatStream、aiChatCancel、aiExec、aiCattyCancelExec、aiSetChatSessionCancelled、aiPrewarmShellEnv、aiAllowlistAddHost | AgentService turn 方法 + host use case + Go policy（技术设计 §2.2） |
| Provider 配置（2） | aiFetch、aiSyncProviders | typed provider/config 方法；逐个迁走裸 fetch |
| External SDK agent（13） | aiSdkAgentStream/Cancel/Steer/ListModels/AccountInfo/Cleanup/ElicitationResponse/McpStatus/Plugin{Install,Enable,Disable}/Marketplace{Install,Remove} | `internal/agent/adapters` + 有类型 adapter administration |
| Codex 账户/App Server 交互（8） | aiCodex{StartLogin,CancelLogin,GetLoginSession,Logout,GetIntegration}、codexAppServerGetStatus、respondCodexAppServerInteraction、cancelCodexAppServerInteractionTimeout | InteractionRouter + 账户 facade + runtime inspection |
| MCP 集成（14） | aiMcp{UpdateSessions,UpdateLiveSessions,MergeSessions,SetToolIntegrationMode,SyncPermissionGrants}、externalMcp{SetEnabled,SetConfig,GetStatus,ClaudeAdd,ClaudeGetStatus,CodexAdd,CodexGetStatus,GrokAdd,GrokGetStatus} | Go external integration service；token 分离可 revoke |
| 用户技能（3） | aiUserSkills{BuildContext,GetStatus,OpenFolder} | Go skills/config use case；renderer 不读任意路径 |
| 发现（1） | aiDiscoverAgents | runtime inspection |
| 事件（11） | onAiStream{Data,End,Error}、onAiAgent{Stdout,Stderr,Exit}、onAiSdkAgent{Event,Done,Error}、onCodexAppServerInteraction{Request,Cleared} | AgentEventEnvelope + ReadEvents 补读 |
| Vault 请求（2） | onVaultAgentRequest、respondVaultAgent | 归入 InteractionRouter + shared Vault use case |

合计 61，组间无重复计数。Wails client 现状：`agent: unimplemented("agent")`（wailsRuntimeClient.ts:1412）。`script`/`plugin` port 同为 unimplemented，但插件实际经 `bindings/.../pluginservice` 直连（:323），与 AgentPort 无关——迁移 AgentPort 时不得动插件路径。

## 6. AI storage keys（30 条）

`infrastructure/config/storageKeys.ts` 中 `netcatty_ai_*` 28 条 + `netcatty.aiDebug.*` 2 条。`profileDomain.ts:44-46` 的 `isAIManagedStorageKey` 把全部 30 条排除在 Go profile 读写之外（hostStorageAdapter 跳过投影、canonicalHydration 跳过导入）；P6-05 前不得改变该行为。

| 组（key） | data-inventory 分类 | 迁移语义（技术设计 §7.1） |
| --- | --- | --- |
| providers/active provider/active model/agent {model,provider,thinking} map/composer prefs（7） | canonical-migrated | schema 化配置；`providers` 内 apiKey 拆 secret reference；config revision 参与 turn/resume 身份 |
| permission mode/tool integration mode/host permissions/permission grants/command blocklist（5） | canonical-migrated | Go policy owner；session-only grant 不升永久 |
| command timeout/response idle timeout/max iterations/session idle timeout（4） | canonical-migrated | 校验单位/范围；0/undefined 语义保持 |
| sessions/active session map（2） | canonical-migrated | 消息顺序、usage/activity、continuation、runtime identity；active map 引用先验存在性 |
| external agents/default agent（2） | canonical-migrated | 配置与 runtime 可用性分离；enabled 不可绕过 disposition |
| external MCP {enabled,mode,idle,focus,silent}（5） | canonical-migrated | 同上，单独 token |
| web search/quick messages（2） | canonical-migrated | web search 含 secret，拆分；无配置不暴露 harness.web.search |
| show_terminal_selection_action（1） | **device-local** | 不默认上传 sync |
| aiDebug.hide / aiDebug.profile（2） | **transient-cache** | 不同步 |

`canonical-migrated` 是 inventory 的目标分类声明，不是本机已 promotion 的证据（data-inventory.md:14）。

## 7. 外部 Agent vendor 矩阵

来源：[baselines/external-agent-protocols.md](external-agent-protocols.md) §4/§11 与 decisions.md。P0-05 停在 `needs-verification`：所有 vendor 均未完成三平台 executable/process-tree/handshake 证据，AI-04 不能升级。

| Agent | 当前嵌入 | Go 目标协议 | Node-free 判定 | 最迟决策点 |
| --- | --- | --- | --- | --- |
| Catty | renderer Vercel AI SDK + Electron provider proxy | Go HTTP/SSE provider adapters + Go tool loop | replaceable | Phase 7 入口固定 provider protocol |
| Claude | `@anthropic-ai/claude-agent-sdk` + `claude` | native headless stream-json | replaceable-candidate（provenance/catalog/policy 未证） | model catalog 与 `supportedModels()` 等价性证明 |
| Codex SDK | `@openai/codex-sdk` spawn | **retire**，并入 App Server owner | retirement-candidate | App Server parity 后不留第二 owner |
| Codex App Server | `codex app-server --stdio` | Go JSONL RPC client | replaceable-candidate（首选 Codex owner） | schema 锁定 + native provenance |
| Copilot | `@github/copilot-sdk` + CLI | official Go SDK / JSON-RPC v3 | **retirement-decision**（runtime 内嵌 Node） | `agent-disposition:copilot` |
| Cursor API key | `@cursor/sdk` | sdk.v1 Connect/protobuf | 需 accepted Bun decision | `agent-runtime:cursor-bun` |
| Cursor CLI login | `cursor-agent`（Windows node.exe+index.js） | ACP（需上游非 Node runtime） | needs-upstream-protocol | `agent-disposition:cursor-cli` |
| OpenCode | `@opencode-ai/sdk` 启 `opencode serve` | Go HTTP/OpenAPI/SSE | replaceable-candidate（Bun 需决策） | `agent-runtime:opencode-bun` |
| CodeBuddy | `@tencent-ai/agent-sdk` + Node CLI | ACP 语义有、可执行体是 Node | **retirement-decision** | `agent-disposition:codebuddy` |
| Grok ACP | per-turn `grok agent stdio` | Go ACP v1 core + Grok 扩展 | replaceable-candidate（permission/reverse-RPC 待修） | ACP conformance 设计；旧 Confirm→always-approve 不可复制 |
| Grok streaming-json | `grok -p --output-format streaming-json` | 仅显式降级 fallback | degraded-candidate（非推荐 owner） | 明确降级决策 |

## 8. MCP 协议代际基线

| 事实 | 证据 |
| --- | --- |
| Electron MCP server 是 Node：`electron/mcp/netcatty-mcp-server.cjs` + `@modelcontextprotocol/sdk@1.29.0`（package.json:105） | server 不设 protocolVersion；initialize 由 SDK 处理，支持 `2024-10-07`～`2025-11-25`（`SUPPORTED_PROTOCOL_VERSIONS`，默认协商 `2025-03-26`） |
| Netcatty 自有代码不解析 initialize/_meta：`normalizeMcpCallArguments.cjs` 只补全 `tools/call` 的 arguments | 协议代际行为实际全部委托 Node SDK |
| `2026-07-28` 规范：无 initialize handshake，逐请求 `_meta` 携带版本；server 不发起 JSON-RPC request；stdio 取消用 `notifications/cancelled`，HTTP 用关响应流；`server/discover` 必须实现 | modelcontextprotocol.io/specification/2026-07-28（versioning/transports，2026-09-14 核对） |
| 官方 Go SDK 兼容表：v1.7.0+ 才覆盖 `2026-07-28`（同时保留四个旧版）；`go.mod` 目前**无**该依赖；`cmd/netcatty-mcp/` 不存在 | github.com/modelcontextprotocol/go-sdk README；go.mod |
| 内部 TCP host RPC（`internal/rpc/` 目标）是 first-party 协议，不是 MCP | 技术设计 §8.1 |

迁移规则：`cmd/netcatty-mcp` 做 dual-era server——锁 v1.7.0+ 具体 tag；旧客户端走 `initialize`、新客户端走逐请求 `_meta`，按客户端开场选代际且不混用；版本拒绝返回 typed error 且不得降成更宽权限。

## 9. 明确阻塞（迁移前必须关闭）

1. **P6-05 `NONAI-COMPLETE` 未达成**：ledger 无记录；AI 四行 `not-started`。
2. **五个 agent 决策类别无 accepted decision**：cursor-bun、opencode-bun、copilot、codebuddy、cursor-cli。retention/retirement 都必须走 decision + ledger；建议草案见 [proposals/agent-disposition-proposals.md](../proposals/agent-disposition-proposals.md)（未接受，待产品 owner 批准）。
3. **Claude model catalog parity 未证明**：CLI machine-readable catalog 与 `supportedModels()` 等价性缺失。
4. **Codex schema 锁定与 native provenance 未完成**：`.js` 启动分支必须证明可拒绝。
5. **Grok ACP conformance/permission 设计未完成**：旧 always-approve 不可复制。
6. **工具链漂移（W02）**：见 §10 提案。
7. **两代 MCP 测试需要真实旧 vendor 客户端**：fixture 只有 tool schema（67 条），无 initialize/_meta 帧样本；W07 需补。

## 10. W02 提案：Go/Wails 版本对齐（模块对齐已于 2026-09-14 执行，WV3-L133）

- **归属**：既有 foundation/release 资格验证（非 AI 功能）；技术设计 §1.1 要求在取得有效 NONAI-COMPLETE 前完成。
- **漂移与执行结果**：根 `go.mod` 原为 `v3.0.0-alpha.63`，npm runtime 和 generator 为 `3.0.0-beta.12`。已按 WV3-L133 将根模块对齐到 `v3.0.0-beta.12`：`go build ./...`、`go vet ./cmd/netcatty`、`go test -count=1 ./cmd/netcatty` 通过（go 1.25.0，go directive 不变）；`GOTOOLCHAIN=go1.25.0` 重新生成 bindings 为 20 services / 212 methods / 97 models，与已提交内容零 diff；`npm run wails:build` 产出 `bin/LemonSSH.exe`。新增 `npm run check:wails-versions` 漂移守卫（scripts/migration/check-wails-versions.mjs）并接入 CI。
- **剩余任务**：① 三平台 shell smoke（macOS/Linux 未在本切片重建）；② go 工具链升级评估——**已完成本地评估（WV3-L134，[probes/go-toolchain-1.27.md](../probes/go-toolchain-1.27.md)）**：build/vet/race 全过，但 1.27.1 生成的 bindings 因标准库新增 `encoding/json/jsontext` 而漂移（98 vs 97 models），暂不锁定，保持 `GOTOOLCHAIN=go1.25.0` 生成钉；③ 绑定生成命令保持 `GOTOOLCHAIN=go1.25.0` + 位置参数 `./cmd/netcatty`。
- **边界**：失败回退整个版本组合，不留混搭；改动使已验证 non-AI gate 失效时按 gate epoch 规则补证/重开。

## 11. 复验命令

```powershell
npm run check:migration-docs
node --input-type=module -e "import{readFileSync}from'node:fs';const c=JSON.parse(readFileSync('testdata/migration/electron/capability-catalog.json','utf8'));console.log(c.length)"
rg -n 'unimplemented\("agent"\)' infrastructure/runtime/wails/wailsRuntimeClient.ts
rg -n "isAIManagedStorageKey" infrastructure/persistence/profileDomain.ts
```

本文数字与 §2 hash 绑定基线 commit `95a7ee74`；fixture 变更后须重算并同步更新本文。
