# AI 迁移执行方案（Electron/Node → Go/Wails v3）

状态：**已开工**。WV3-025 之后 W03 契约已落（WV3-L160，`AI-01` probe），下一刀是 W04 共享 use case。基线日期：2026-09-14。供后续 AI 按切片落地；本文不改变任何 capability 状态，也不是 `NONAI-COMPLETE` 的证据。

本方案已扩展成三份配套文档，按下列顺序使用：

| 文档 | 执行 AI 要解决的问题 |
| --- | --- |
| 本文 | 是否允许开工、迁移范围、owner、正式阶段与退出条件 |
| [技术设计与协议契约](ai-migration-technical-design.md) | 版本选型、DTO、状态机、数据迁移、Provider continuation、权限、取消、崩溃恢复如何实现 |
| [工作包与验收手册](ai-migration-work-packages.md) | 从哪个文件开始、先后依赖、每包产物、具体用例、命令及交接提示词 |

**增强版采用的方法：**官方 Go 协议 SDK + 项目自己的薄 adapter；Go 单一运行时 + 可恢复事件投影；逐能力行为对照 + 故障注入；先跑通最小完整链路再扩展工具和 vendor。新 SDK/协议只在已锁版本与原功能兼容测试通过后采用。三份文档中的新路径、接口、测试名称及调优值均为待实施设计，不能据此声称代码已经存在。

## 1. 执行入口与硬边界

状态只以 [capability-matrix.md](capability-matrix.md) 和 [migration-ledger.md](migration-ledger.md) 为准；顺序与门禁以 [implementation-plan.md](implementation-plan.md)、[verification-gates.md](verification-gates.md)、[decisions.md](decisions.md) 为准。后续 AI 每次开工前重读这五份文件，不得把本文的时间快照当成新授权。

**WV3-025 取代 WV3-009。** unsigned 资格包（v0.0.2 与本机 `package-wails`）和三平台开窗之后，可以创建 `internal/capability` / `internal/agent`。不要伪造 `NONAI-COMPLETE`。矩阵中 `AI-01` 已随 W03 契约落地升为 `probe`（WV3-L160）；`AI-02`, `AI-03`, `AI-04`, `AI-04.1`, `AI-04.2`, `AI-04.3` 仍为 `not-started`；`AI-04.4` 至 `AI-04.8` 已 `removed` / `retired`。`SYNC-01` 明确记载 AI 数据仍使用 renderer localStorage；Wails `agent` 端口仍是 `unimplemented("agent")`。下一刀是 W04 共享 use case，不是完整 Catty。

P7 开工检查由执行 AI 实际读取并记录（第 1、2 条的 verified/断开要求已被 WV3-025 移出生产开工条件，转为并行偿还的证据债；P8-02 前仍须闭合）：

1. 所有 `FND/TERM/SSH/SFTP/NET/SYS/SYNC/PLUG` **required leaf** 和 `REL-01/REL-02` 为 `verified`；Wails 与旧 Electron 路径已断开，Electron 仅为冻结的 release carrier。
2. P6-02/03/04 的发布、更新、升级 bootstrap 定向证据完整；ledger 中已有通过 `npm run check:migration-docs` 的 P6-05 `NONAI-COMPLETE` 记录，且之后没有使 gate epoch 失效的回退或 scope change。
3. 三平台 release-target 决策已被引用；Cursor/OpenCode Bun，以及 Copilot/CodeBuddy/Cursor CLI login 的五个 Agent 入口 disposition 决策已获批准。当前 [decisions.md](decisions.md) 的 *Required Future Decisions* 小节与已接受的 `WV3-011`～`013` 同时存在，执行时以 accepted decision 和 checker 为准，不根据文字推测 gate 已开。
4. AI adapter 若缺稳定非 Node 协议，只能按已接受的 capability-specific scope-removal decision 退休；不能悄悄降级、加永久 Node sidecar 或把 `not-started` 写成 `verified`。

## 2. 当前代码与目标 owner

| 领域 | 迁移前的权威路径 | 当前 Go/Wails 可复用部分与缺口 | Phase 7 目标 owner |
| --- | --- | --- | --- |
| 能力目录/策略 | `electron/capabilities/catalog/`、`policy.cjs`、`codegen/toolSurfaces.cjs`、`mcpToolRegistry.cjs` | `internal/app/contracts/` 有基础 wire contract；`internal/capability/` 尚不存在 | `internal/capability/{catalog,policy,dispatch}` 单一 typed authority；生成各 surface 投影 |
| MCP/CLI/host RPC | `electron/bridges/mcpServerBridge.cjs`、`electron/mcp/netcatty-mcp-server.cjs`、`electron/cli/netcatty-tool-cli.cjs` | Go 终端/SFTP/转发已有运行服务，但 Node CLI/MCP 仍拥有 Agent 调用路径 | `internal/rpc/` + 原生 `cmd/netcatty-mcp/`、`cmd/netcatty-tool/`；统一 Go policy/dispatch |
| Catty | `infrastructure/ai/harness/agentRuntime.ts`、`turnDrivers/catty*`、`capabilityTools.ts`、`application/state/useAIChatStreaming.ts` | Wails `AgentPort` 未接线；React AI 面板可保留 | `internal/agent/{runtime,events,context,tools,trace,sessions}`；React 仅做展示和输入 |
| Provider/网络 | `infrastructure/ai/sdk/providers.ts`、`electron/bridges/aiBridge/providerHandlers.cjs` | Go `internal/platform/credentials/` 与 Profile Store 已有，AI provider HTTP/SSE 未有 owner | `internal/agent/providers/` + `internal/platform/netpolicy/` |
| 外部 Agent | `electron/bridges/aiBridge/sdk/`、`codexAppServer/`、`sdkAgentAdapter.ts`，Node MCP child | Go process/adapter 未有 owner；P0-05 只有审计 | `internal/agent/{process,adapters,models}`，复用原生 MCP/CLI |
| AI 数据 | `application/state/useAIState.ts`、`useAISettingsState.ts`、`aiStateSnapshots.ts`、`storageKeys.ts` | `internal/profile/store/` + `cmd/netcatty/profileService.go` 可做 CAS/事务；AI hydration 尚未迁移 | Go Profile/credential owner；React hook 保留视图状态和表单 |

**必须先解的依赖：** `cmd/netcatty/terminalService.go`、`sftpService.go`、`forwardService.go` 中的 Wails-facing service 不能被 `internal/capability` import。P7-01/02 先把它们需要的操作抽成 shell-neutral `internal/app` use case/interface，现有 Wails service 退成薄 facade，Go capability dispatch 调用同一 use case。不得在 capability 层复制连接池、SFTP、转发或 Vault 规则。`internal/plugin/permissions` 继续独立持有插件 principal、grants 和 secret lease；Agent policy 不能 import 或授权它。

现有 Go 方法不等于 AI 工具已完成：`TerminalService` 有 Connect/Write/Resize/Close，`SFTPService` 有 List/Read/WriteText/Download/Upload，`ForwardService` 有 Start/Stop/List；但 Agent 所需的**会话 scope、可取消命令执行/后台 job、附件、输出 handle、审批**尚未由 Go capability owner 统一实现。对每个 catalog ID 必须把“现成 Go handler”“需要扩展 use case”“不支持/待决策”逐项列入覆盖表，禁止用 Wails 方法名推断 parity。

## 3. 固定的跨层契约

先冻结 `testdata/migration/electron/` 的 catalog、Agent tool、MCP schema、CLI JSON、bridge contract；读取 `infrastructure/ai/harness/types.ts` 的 `AgentEvent` 与 `turnDrivers/types.ts` 的 `TurnInput`，补充脱敏的成功、报错、取消、审批、tool loop、413、resume golden traces。fixture 是**迁移前对照**，只用已批准的行为更改覆盖差异；不要把 Electron 的已知不安全放行行为原样移植。

Go 对 React 暴露一个通用 `AgentService`，放在 `cmd/netcatty/agentService.go`，只调用 `internal/agent`；通过 Wails generated binding 接到 `infrastructure/runtime/wails/` 的 AgentClient。建议的最小协议如下，具体 wire 类型纳入 `internal/app/contracts/registry.go` 并生成 TS，不在 React 手写第二套 DTO：

```text
PrepareTurn(request) -> {turnId, cursor, snapshotRevision}
ReadEvents(turnId, afterSequence, limit) -> {events, nextCursor, snapshot?, cursorExpired}
StartTurn(turnId) -> accepted / typed error
StopTurn(turnId, reason) -> idempotent result
SteerTurn(turnId, input) -> accepted / busy / unsupported / inactive / cancelled
ResolveInteraction(interactionId, decisionOrInput, expectedRevision) -> result
ListModels(agentId, configRevision) -> model catalog
InspectRuntime(agentId) -> protocol/runtime capabilities and provenance
DeleteChatSession(chatSessionId) -> stopped turn + scoped cleanup
```

`PrepareTurn` 先在 Go 分配 turn ID、以有期限的 lease 占用 chat-session 活跃槽并建立有界事件缓冲；UI 获取 cursor、安装 Wails event listener 后才 `StartTurn`，随后用 `ReadEvents` 补齐竞态窗口。每个 event 至少含 `schemaVersion`、`sequence`、`turnId`、`chatSessionId`、`backend`、`type`、`timestamp` 与现有 event payload；新 wire 中序列与 revision 为十进制字符串，避免 Go uint64 到 JS number 的精度损失。成功 Start 的 turn 才产生 `turn_start`，最终由同一 owner 产生唯一逻辑 `turn_end`；Prepare 过期只释放 reservation。Wails event 仅作通知，`ReadEvents` 是缺口恢复来源，过期 cursor 返回 authoritative snapshot。详细状态转换、幂等及崩溃边界见技术设计。终端原始字节继续走 `internal/terminal/dataplane`。

所有审批、vendor `request_user_input`、ACP reverse request 统一成 `InteractionRequest`（ID、turn/session、deadline、kind、已脱敏 subject、选项或输入 schema）。React 只回 interaction ID 和用户决定；Go 用保留的原始 request、policy revision、scope、deadline 重新校验。UI 短暂卸载时请求可等待授权窗口重新挂载，但不能自动放行；超时、永久关闭、App Lock 按统一规则拒绝/取消。UI、`/stop`、MCP 和 app quit 均调用同一 `StopTurn` owner。只回收该 turn 所拥有的执行/传输资源与临时 lease；chat 级 output handle 按 TTL/删除规则保留，共享 SSH 连接不能因一个 turn 取消被销毁。

**数据边界：** AI storage keys 逐 key 依 [data-inventory.md](data-inventory.md) 与 `infrastructure/config/storageKeys.ts` 迁入 Profile Store；会话消息、活动/usage、active session map、provider、permissions/grants、external-agent config 均需 schema/兼容读取测试，device-local 分类保持原规则。React 仍可缓存 UI draft/panel state，但不能成为 turn、授权或持久 AI 历史的第二 owner。`ProviderConfig.apiKey`、web-search key 等迁为 secret reference，Go 按 purpose 在 request-local 内存打开；已保存密钥不得解密回送 Wails、写 trace/log、argv、项目配置或通用 temp。迁移 `enc:v1:` 根据导出 origin/receipt 选择 Electron migration broker 或已知 purpose 的 Go opener，再 staging/reseal；**不能**把前缀相同视作加密 owner 相同。CAS、备份、校验、原子 promotion、失败回滚遵循既有 FND-02、FND-03、SYNC-01 的单 writer 机制；外部 session identity 需版本化并拒绝跨 runtime 盲 resume。

## 4. 可交给其他 AI 的实施切片

每个切片单独提交，输入为上一切片的已通过证据；修改前确认 gate 仍有效。每次按 [templates/slice-record.md](templates/slice-record.md) 追加 ledger，更新相应 matrix 行和 plan 进度。测试通过只允许升到对应证据支持的状态；`verified` 要求 [verification-gates.md](verification-gates.md) 的 A 级平台/协议证据，不能由本机编译替代。

### P7-01 前置核对：门禁与差异清单（不创建 production owner）

- 产物：`docs/migrations/wails-v3/baselines/ai-phase7-parity.md`，逐 capability ID 列 Electron surface、policy、schema、Go use case、数据依赖、测试 fixture、retirement trigger；逐 external agent 列已接受 decision ID、协议、runtime provenance 与 unsupported 功能。
- 复核 `testdata/migration/electron/{capability-catalog,agent-tool-specs,mcp-tool-specs,cli-capabilities}.json`，确认 fixture 与冻结 release carrier 一致；新增 canonical event traces 时全部脱敏。
- 出口：P6-05 checker 通过、缺口表无“由名称推断已实现”；无法映射的 required handler 留为明确阻塞，不得先接假成功。

### P7-01：共享 use case、Go catalog 与 policy（AI-01）

1. 在 `internal/app/` 定义 Agent 能调用的 terminal/session、SSH exec/job、SFTP、transfer、Vault/snippet、port-forward、attachment use case 接口；从 `cmd/netcatty` 提取需要共享的实际逻辑，保持 Wails API 行为和已有 tests 不变。对尚无 Go handler 的 catalog 条目写明确 failing/unsupported 契约，随后逐项实现；不调用 Electron bridge。
2. 新建 `internal/capability/{catalog,policy,dispatch}`。一个 typed record 定义 stable ID、baseline status、input/output schema、Agent kinds（sidebar/global）、surface（Wails/Catty/MCP/CLI）、read/write/sensitive/long-running 分类及 scope 规则。声明完整目录，handler 由 composition root 注入；单独记录运行时可用性，不能把冻结基线的 implemented 改成 planned 掩盖缺口。启动时检测重复 ID/alias、暴露但无 handler 的项、schema 漂移。
3. 从 Go authority 生成 TS Catty/global specs、Wails metadata、MCP tool schema、CLI command table 和 policy fixture；对冻结 JSON 做结构/语义 diff。`harness.*` 只进 sidebar，不泄漏到 MCP/CLI；保留现有显式 `agentKinds` 优先规则。
4. 实现 `Observer/Confirm/Auto`：认证→scope→policy revision→grant 匹配→审批→dispatch。写操作必有有效 chat session；取消后的写拒绝。敏感读和各 surface 的显式例外按 fixture/决策测试，不能将 CJS `getCapabilityByRpcMethod` 未命中时的宽松放行带入 Go。
- 出口：目录和 policy 全量对照通过，本切片启用的 host 工具有真实 handler；`harness.*` handler 留到 P7-04 注入，并保持未暴露、缺口有记录。这解除“catalog 等 runtime、runtime 又等 catalog”的循环依赖。AI-01 整体完成仍要求所有保留的 implemented ID 接齐后再验证，不能在本切片提前标 verified。全量 capability × surface × mode × scope 表、grant 匹配/不匹配、并发撤销、Go `-race` 通过；插件 policy tests 不回退。

### P7-02：原生 host RPC、MCP、CLI（AI-02，依赖 P7-01）

1. `internal/rpc/` 实现本机 authenticated transport、随机 token、权限受限 discovery file、token rotate/revoke、逐连接 principal、chat/terminal/workspace scope、frame/深度/大小限制、deadline/cancel。主应用启动/退出管理 listener 和子进程；外部 MCP enable/disable 要使旧 token 立即失效。
2. `cmd/netcatty-mcp/` 使用锁定的官方 Go MCP SDK 承担 stdio/版本兼容，只实现 catalog/schema/结果 adapter；`cmd/netcatty-tool/` 只做 CLI 解析和稳定 JSON/exit code。两者经同一 host dispatch；不创建第二份策略。当前 CLI 是 Netcatty 内部集成面，parity 重点是 first-party launcher、JSON 与 quoting；不要凭空扩张为第三方公共 API。官方 SDK 的协议支持范围及选型条件见技术设计。
3. 对 `electron/cli` 的 special-case exec、jobs、SFTP、session 及 catalog fallback 逐项迁到 Go。exec 要有输出界限、超时、取消和会话串行化；后台 job 要有 owner、poll/stop、过期回收。Vault/notes/snippets 通过共享 Profile use case，不向 Agent 返回密码/私钥。
4. MCP 注入改指向签名/哈希校验的 Go binary；服务端重验 token/scope/policy，不信任客户端声称的 chatSessionId 或被展示的参数。
- 出口：MCP 旧版 initialize/list/call 与 2026-07-28 版逐请求元数据、版本回退/拒绝测试，以及 CLI golden JSON diff；错误、畸形/截断/超大帧 fuzz；撤销、重启、取消、Windows 引号、三平台 discovery ACL、清理进程与文件均有证据。两代 MCP 语义不能混用，依据 [官方版本兼容规范](https://modelcontextprotocol.io/specification/2026-07-28/basic/versioning)。Wails/Go 路径不再调用 Node MCP/CLI。

### P7-03：Go Provider 与网络策略（AI-03，可在门禁后与 P7-01 并行）

1. `internal/platform/netpolicy/` 统一代理/TLS、DNS 解析后 IP 校验、重定向重新授权、私网/loopback/metadata 限制和 header 跨 origin 清理；用户明确配置的本地 Ollama/自建端点需要**独立受限规则**，不能靠关掉整个 SSRF 策略。
2. `internal/agent/providers/` 实现现有 `ProviderStyle` 三协议族：OpenAI-compatible Chat/Responses、Anthropic Messages、Google；保留模型列表/上下文窗口、advanced params、reasoning、tool-call fragment 重组、usage、stream idle/step/total timeout、429/413 分类和取消。依据 `infrastructure/ai/types.ts` 与 `sdk/providers.ts` 的具体配置语义做 fixture，不按 provider 图标分支。
3. Go 从 credential reference 注入 API key；UI `aiFetch`/`aiChatStream` 兼容调用逐个迁到 typed service；可配置 `skipTLSVerify` 的行为/风险边界要有专门负例，不因旧 setting 存在而绕过 endpoint policy。
- 出口：本地 HTTP/SSE fixture 的三协议族流式 tool/usage/error parity；DNS rebinding、重定向、跨域 header、私网和 body/header/idle bounds 负例；Go race 通过。

### P7-04 第一切片：AI 数据与 Wails AgentClient（AI-03，依赖 P7-01/02/03）

1. 为 AI settings/history/grants/provider secret references 做 Profile schema、迁移 receipt、Go 事务读写及 React hydration；同一 profile 禁止 Electron/Wails 同写。保留历史 ChatMessage、AgentActivity/AgentUsage、context compaction、legacy images/attachments 兼容读；发现不可解密密钥就阻断而非占位。
2. 在 `cmd/netcatty/main.go` 注册 `AgentService`，生成 binding；在 `infrastructure/runtime/wails/wailsRuntimeClient.ts` 把 `agent: unimplemented("agent")` 改为 typed Wails AgentClient。改 `application/state/useAIChatStreaming.ts` 和 `components/ai/hooks/useAIChatStreaming.ts` 为输入/事件展示适配。外部方法继续走统一 client，不加 per-vendor Wails methods。
3. 实现第 3 节的 prepare/read/start/stop/interaction 流程及窗口重挂载恢复。持久事件只保留所需审计信息，流式 event ring 有上限；输出大对象转 scoped handle。
- 出口：旧 AI 数据与密钥的备份→staging→re-seal→校验→promotion/回滚测试；Wails reload/多窗口 event gap 与 cursor-expired 测试；已保存的 provider secret 不再由 Go 解密回传 WebView。用户初次输入密钥是专用写入口，响应只返回 secret reference，不能用通用 credential Open/Seal 绕回读取。

### P7-04 第二切片：Go Catty AgentRuntime（AI-03）

1. Go 唯一持有 one-active-turn/chat、turn ID、session queue、trace、event normalization、stop/steer 和 resource cleanup。按 `infrastructure/ai/harness/agentRuntime.ts`、`agentStop.ts`、`traceStore.ts` 以及 `turnDrivers/catty*` 的 golden trace 验证成功、error、abort 都恰好一个 terminal event。
2. 将 `capabilityTools.ts`、`toolOutputStore.ts`、`toolResultDedup.ts` 语义迁为 Go：工具参数/结果脱敏、完整性校验、重复 read 提示、持久化 scoped output handle、chat/terminal 删除时回收；大型 SFTP/终端输出不能塞入事件或模型上下文。
3. 将 `contextManager.ts`、`contextBudget.ts`、`cattyRuntime.ts`、`compactionPruner.ts`、`staleContextPruner.ts`、`sessionState.ts` 的阈值/行为冻结后迁移。pre-turn 与 413 retry 可摘要；step pruning 只做 typed 压缩，不触发 LLM；压缩后重注入当前目标、session 状态与权限边界。保留多轮 tool loop 和 provider continuation。
4. 终端命令、SFTP、网络工具统一经过 Go capability policy；用户在 UI 的全局 permission mode/host scope 改变后，旧请求按 policy revision 失效。
- 出口：Gate 11 canonical traces、并发 turn busy、每个 lifecycle 点取消、413 至多一次强制压缩后重试及 tool-progress 两分支、output handle 越权读/过期、redaction、large input/output、Go `-race`；React 不再启动 authoritative `streamText`。

### P7-05：外部 Agent 逐个适配（AI-04，依赖 P7-04 第二切片）

先在 matrix 中按既定 decomposition 规则建立 **AI-04 子行**，每个保留/退休的 Agent 独立 ledger 证据；不能直接把 AI-04 aggregate 标为完成。所有 adapter 共用 `internal/agent/process` supervisor、session identity、interaction router、Go MCP 注入和 canonical AgentEvent。进程只接受绝对、校验过的可执行文件；拒绝 Node shebang/npm/npx/wrapper，限制 env/cwd/config/cache、stdout/stderr、帧大小；取消顺序为协议 cancel→短 grace→Windows Job Object 或 POSIX process group 回收。附件在 Netcatty 专用 temp 中由 Go 校验 count/size/MIME/hash/scope 后 staging，不能信任 WebView 给的任意本地 path。

| Agent | Go 实施入口 | 必做 parity 与阻断 |
| --- | --- | --- |
| Codex | `internal/agent/adapters/codex/`：移植 `electron/bridges/aiBridge/codexAppServer/{connection,runtime}.cjs` 的 JSONL 相关性、thread/resume、turn/interrupt/steer、model/list、approval/user-input；使用锁定 schema | App Server 为单一 Codex owner；SDK 路径退出。必须验证 native executable，不能允许旧 `.js` 启动分支 |
| Grok | `internal/agent/adapters/acp/` 共用 ACP core，再做 Grok 扩展 | initialize/version、reverse RPC、permission allow/deny/timeout、resume/load、cancel；旧 Grok Confirm→always-approve 不可复制；streaming-json 只在明确决定的降级范围内使用 |
| Claude | 原生 headless stream-json adapter | session resume、MCP、图片、取消、model catalog 和 tool policy；若 native provenance 或 model discovery parity 仍缺，先记录阻断/产品决定 |
| OpenCode | none; typed unavailable | WV3-015 plus WV3-020 / L153 already `removed` / `retired`. Phase 7 maps fail-closed only. Reopen needs a superseding decision plus native non-Bun provenance |
| Cursor API key | none; typed unavailable | WV3-014 plus WV3-019 / L152 already `removed` / `retired`. Phase 7 maps fail-closed only. SDK `autoReview:false` is not Netcatty Confirm |
| Cursor CLI login、Copilot、CodeBuddy | none; typed unavailable | WV3-016, WV3-017, WV3-018 plus WV3-021, WV3-022, WV3-023 / L154, L155, L156 already `removed` / `retired`. History stays readable; do not call a Node CLI |

每个可保留 adapter 必须声明 `Start/Resume/Stop/Steer/ListModels/Inspect` 支持矩阵；不支持的操作返回 typed `unsupported`，不得假造成功或默默新建 session。外部 session identity 至少绑定 profile/chat、vendor/protocol/version、binary digest/version、auth profile、permission/tool mode、workspace/cwd/roots、network class、catalog/policy revision。任一边界不匹配返回 `stale-session`；只有完整安全 history seed 且明确创建新会话时才可续聊。vendor 内置 shell/edit/network/plugin/extension 路径无法经过 Netcatty policy 时禁用，不能用 `--force` 或永久自动批准代替 Confirm。

- 出口：每个保留 Agent 在 required Windows/macOS/Linux target 上完成无付费 handshake/provenance/递归进程树，再做有授权的真实响应/权限/usage/恢复测试；malformed frame、crash、cancel/kill、MCP 注入、attachment、event golden traces 全通过。任何 recursive Node child 均阻断该 adapter `verified`。

### P7-06：Wails 路径断开 CJS/Node（AI-01～04）

1. 删除 Wails 调用链中的旧 CJS catalog/MCP/CLI/AI bridge 与运行时 Node SDK 依赖/资源；前端构建仍可用 Node。冻结 Electron release carrier 在 P9-01 回滚窗口关闭后才删除仓库源码，当前切片不能提前删其唯一稳定 release 路径。
2. 扫描 Wails 打包产物及递归 process tree：无 Electron、`.asar`、runtime `node_modules`、Node executable/shebang、Node AI SDK、Node child；检查保留的 vendor/helper/plugin 二进制 SBOM、hash、签名和动态依赖。
3. 对比每一 capability 的 Go owner、前端 adapter、旧路径 retirement trigger、数据迁移证据；AI required leaf 至少 `verified`，已批准移除的 row 为 `retired` 并有 decision/ledger。
- 出口：Gate 10/11/14 的 Phase 7 部分通过，`REL-03.1` 留到 P8-01 最终签名 RC；`REL-03.2` 留到 P8-02 cutover、P8-03 rollback closure 后的 P9 删除。

## 5. 验证命令与交付格式

每个切片按影响范围跑 `go test -count=1 ./internal/app/... ./internal/capability/... ./internal/agent/... ./internal/rpc/... ./cmd/netcatty/...`（目录出现后再纳入）、`go test -race -count=1` 对应 Go 包、`go vet` 对应包、`npm run check:contracts`、`npm run check:migration-electron-baseline`、`npm run check:migration-docs`、受影响 TS/React tests 与 `npm run lint`。生成 Wails binding 后在 Windows 执行 `npm run wails:build`；当前脚本显式使用 `-H windowsgui`，不能作为 macOS/Linux 构建命令。先按技术设计完成 Wails Go module/runtime/CLI 版本对齐，再按各平台已验收的构建入口验证。MCP/CLI 做协议 golden/fuzz 和实进程 smoke；最终 P8-01 才做签名 RC、全平台洁净机与 Gate 14 artifact proof。不要把运行不了的 macOS/Linux 或签名测试写成 PASS。

交给下一位 AI 时，每个任务必须附：`前置 gate ledger ID`、`matrix row/child row`、`Electron owner + fixture`、`Go owner + Wails adapter`、`实际改动文件`、`数据/权限/取消边界`、`回归命令及原始结果位置`、`旧路径 retirement trigger`、`未完成 blocker`。一次只实施一个能独立验证的切片；失败时修复当前切片或记录阻塞，不能绕过 policy、范围或发布门禁继续推进。
