# 外部 Agent 嵌入与 Go/Wails 原生支持基线

状态：P0-05 静态审计完成，`needs-verification`

记录日期：2026-08-24

## 1. 核心结论

Netcatty 当前不是把外部 Agent 嵌入 React，而是采用三层宿主：

```text
React / AgentRuntime
  -> preload / Electron IPC
  -> Electron main SDK host
  -> Node SDK 或 vendor CLI/process
  -> injected Netcatty MCP (Node/Electron-as-Node)
  -> Netcatty capability/policy services
```

Go/Wails 迁移应保留“UI、宿主、vendor adapter”三层，但重新分配 owner：

```text
React / Wails UI
  -> typed Wails AgentClient
  -> Go Agent Host（唯一 turn/session/policy/event/process owner）
  -> Go vendor protocol adapters
  -> vendor native/Bun/Rust process or provider HTTP API
  -> native Go netcatty-mcp / direct capability dispatch
```

“Go 原生支持 Agent”在本项目中的定义：

- Go 原生拥有 turn、session、permission、MCP、event normalization、cancel、steer、
  process tree、secret injection 和 trace；
- Go 直接实现 HTTP/SSE、JSONL、ACP、Connect/protobuf 或 HTTP/OpenAPI client；
- 不加载任何 Node SDK，不通过 Electron preload/IPC，不启动 Node shebang；
- vendor Agent 本体可以是签名/固定版本的 native、Rust 或经架构批准的 Bun 进程，
  但必须通过 SBOM、interpreter、递归 process-tree 和权限审计；
- Wails 只提供 UI binding 和 event subscription，不拥有 Agent policy。

## 2. 当前嵌入架构

### 2.1 Renderer

`ExternalSdkTurnDriver` 调用 `runSdkAgentTurn()`，维护当前 turn 的 UI batching、
tool/activity/usage/session ID，并且只允许 Codex App Server steer：

- `infrastructure/ai/harness/turnDrivers/externalSdkTurnDriver.ts`
- `infrastructure/ai/sdkAgentAdapter.ts`

`sdkAgentAdapter.ts` 在 preload bridge 上注册 event/done/error listeners，再调用：

- `aiSdkAgentStream`
- `aiSdkAgentSteer`
- `aiSdkAgentCancel`
- `aiSdkAgentListModels`
- `aiSdkAgentCleanup`

它把 Electron stream events 映射回 canonical `AgentEvent`。这部分目标状态应缩减为
Wails AgentClient + presentation adapter，不再解密 vendor secrets 或编排 process。

### 2.2 当前 split lifecycle owner

当前 authoritative lifecycle 被 Renderer 和 Electron main 分割：

- Renderer `AgentRuntime` 拥有 one-active-turn、Netcatty turn ID、canonical turn
  start/end、trace fan-out，以及 stop/steer dispatch；
- Electron `registerSdkStreamHandlers()` 拥有 vendor process/session、MCP injection、
  model catalog、attachment staging 与协议翻译。

`registerSdkStreamHandlers()` 的职责包括：

- 校验 IPC sender；
- 创建每 request AbortController；
- 读取 shell env 和 agent config；
- 启动 Netcatty MCP/CLI host；
- 解析 vendor binary；
- 建立 runtime-qualified session identity；
- stage attachments；
- 选择 SDK/App Server/ACP/streaming-json driver；
- 缓存 model catalog；
- 处理 cancel、cleanup 和有限 steer。

关键文件：

- `electron/bridges/aiBridge/sdk/sdkStreamHandlers.cjs`
- `electron/bridges/aiBridge/sdk/index.cjs`
- `electron/shared/sdkSessionIdentity.cjs`
- `electron/bridges/aiBridge/sdk/emit.cjs`

### 2.3 Vendor driver 与工具注入

Driver registry 当前支持：

- Claude SDK；
- Codex SDK / Codex App Server；
- GitHub Copilot SDK；
- Cursor SDK / Cursor CLI；
- CodeBuddy SDK；
- OpenCode SDK；
- Grok ACP / streaming-json。

无论 vendor 是什么，`buildInjectedMcpServers()` 都先启动 Netcatty control host。
MCP mode 注入 stdio MCP；Skills mode 由 vendor shell 调用 Netcatty CLI。

当前 MCP command 是 `process.execPath`，Electron 设置 `ELECTRON_RUN_AS_NODE=1` 后
执行 `netcatty-mcp-server.cjs`：

- `electron/bridges/aiBridge/sdk/injectMcp.cjs`
- `electron/bridges/mcpServerBridge/configAndCleanup.cjs`

因此在 AI-02 的 native `netcatty-mcp`/`netcatty-tool` 完成前，没有任何外部 Agent
可以端到端 Node-free。

## 3. Go Agent Host 目标设计

### 3.1 Go package ownership

```text
internal/agent/
  runtime/       one active turn/chat, lifecycle, steer, stop
  events/        canonical AgentEvent and Wails serialization
  sessions/      runtime-qualified external session identities
  process/       supervised process, framing, tree cleanup, provenance
  adapters/
    claude/
    codex/
    cursor/
    copilot/
    codebuddy/
    opencode/
    grok/
    acp/
  models/        vendor model/auth discovery

internal/capability/
  policy/        Agent-facing Observer/Confirm/Auto and approval routing
  catalog/       capability identity, MCP/CLI and tool projections
```

`internal/capability` is deferred until Phase 7 and is independent from
`internal/plugin/permissions`. Both policy owners call shared Go application
services; neither authorizes or imports the other.

Wails facade 位于 `cmd/netcatty`，只调用 shell-neutral Go use cases：

```go
type AgentService interface {
    PrepareTurn(context.Context, StartTurnRequest) (TurnHandle, error)
    StartTurn(context.Context, TurnHandle) error
    ReadEvents(context.Context, ReadEventsRequest) (EventBatch, error)
    StopTurn(context.Context, StopTurnRequest) error
    SteerTurn(context.Context, SteerTurnRequest) (SteerResult, error)
    ResolveInteraction(context.Context, ResolveInteractionRequest) error
    ListModels(context.Context, ListModelsRequest) (ModelCatalog, error)
    InspectRuntime(context.Context, RuntimeInspectionRequest) (RuntimeInspection, error)
}
```

React 订阅统一事件，不直接理解 vendor wire event：

```go
type AgentEvent struct {
    Sequence      uint64
    Type          EventType
    TurnID        string
    ChatSessionID string
    Backend       string
    Timestamp     time.Time
    Payload       EventPayload
}
```

`PrepareTurn` 先分配 Go `TurnID` 和 event cursor，UI 先订阅/读取，再调用
`StartTurn`。事件在 Go 内按单调 `Sequence` 有界保留，允许窗口重挂载从 cursor 重放，
避免当前监听器安装顺序依赖。Vendor external session ID 只是后续 event，不替代
Go `TurnID`。

Approval、request-user-input、elicitation 和 ACP reverse request 通过统一 interaction
事件暴露：

```go
type InteractionRequest struct {
    InteractionID string
    TurnID        string
    SessionID     string
    Kind          InteractionKind
    Deadline      time.Time
    Subject       InteractionSubject
    Options       []InteractionOption
    InputSchema   JSONSchema
}

type InteractionSubject struct {
    CapabilityID string
    ToolName     string
    Summary      string
    Rationale    string
    Arguments    SanitizedArguments
    Resources    []CanonicalResource
    GrantScopes  []GrantScope
}
```

Wails 只提交 `InteractionID + decision/input`。Go 校验 turn/session scope、deadline、
allowed option、permission grant 和 schema 后，再关联回复 vendor request。禁止新增
vendor-specific Wails reply methods。

`Subject` 只承载 bounded、redacted display data；authoritative raw request 保留在 Go，
回复时重新匹配 capability、arguments、resources、runtime identity 和 policy revision。
Wails 不得把展示字段回传成新的授权事实。

事件缓冲达到上限后，如果 cursor 早于最旧 retained sequence，`ReadEvents`/
`subscribe` 必须返回 typed `cursor-expired`，并附 authoritative `TurnSnapshot`
（当前状态、session ID、pending interactions、latest sequence、terminal reason）。UI
必须用 snapshot 重建或 fail closed；禁止从最旧事件静默续播并假装无 gap。

### 3.2 通用 adapter contract

每个 adapter 必须实现：

```go
type Adapter interface {
    Inspect(context.Context, Executable) (RuntimeInspection, error)
    Capabilities(context.Context, Executable) (AgentCapabilities, error)
    Start(context.Context, TurnRef, TurnRequest, EventSink, InteractionSink) (SessionRef, error)
    Resume(context.Context, TurnRef, SessionRef, TurnRequest, EventSink, InteractionSink) (SessionRef, error)
    Stop(context.Context, TurnRef) error
    Steer(context.Context, TurnRef, SteerRequest) (SteerResult, error)
    ListModels(context.Context, Executable) (ModelCatalog, error)
}
```

缺少某项时返回 typed `unsupported`，不能伪造成功或静默创建新 session。
`TurnRef` 由 Go runtime 在 vendor 启动前创建；`SessionRef` 只代表可持久化的 vendor
conversation。Adapter 的 reverse request 必须调用 `InteractionSink` 或返回
Method Not Found，不能悬空。

### 3.3 Process supervisor

Go supervisor 必须统一负责：

- 只启动绝对、验证过的 executable；
- 拒绝 `.js`、Node shebang、npm/npx wrapper 和间接 Node child；
- 分开记录 launcher type、artifact format（PE/Mach-O/ELF）、implementation language、
  embedded runtime（Node/Bun/none）和 transitive child runtimes；
- 记录 SHA-256、signature/publisher、version、SBOM 和 observed recursive process tree；
- 最小 environment、独立 cwd/config/cache、stdout/stderr 限额；
- JSONL/Content-Length/SSE/protobuf bounds；
- `context.Context` cancel -> protocol cancel -> grace -> process-tree kill；
- Windows Job Object 与 POSIX process group；
- 不把 secret 写入 argv、项目 config 或日志；
- crash/quarantine 与 stale session invalidation。

PE/Mach-O/ELF 只证明 artifact format，不证明没有嵌入 Node/Bun。Process-tree 观测
用于 qualification 和运行期告警，不能被描述为跨平台、无竞态地阻止所有间接 child；
真正 containment 仍依赖 Job Object/process group、isolated config 和禁用扩展点。

### 3.4 Permission owner

Vendor built-in shell/edit/write、敏感读取、网络和 extension/plugin 工具不能绕过
Netcatty policy。目标规则：

1. 默认禁用 vendor local write/shell tools；
2. 远程运维副作用通过 native Netcatty MCP 或 direct capability dispatch；
3. Observer：拒绝所有 write；
4. Confirm：每项匹配 Netcatty approval/grant；
5. Auto：允许已在 Netcatty scope/policy 内的 write；
6. vendor native approval 只能作为附加限制，不能成为另一个授权 owner；
7. 不支持 approval callback 的 adapter 必须禁用 vendor built-in writes，不能用
   `--force`/`--always-approve` 模拟 Confirm。
8. 每个 adapter 声明可执行工具、filesystem roots、symlink policy、network class、
   config/plugin/hook/LSP/formatter isolation 和 sensitive-read 能力；无法限制时 fail closed；
9. terminal/session capability 同时校验 profile、workspace、chat scope 和 policy revision；
10. vendor 本地读权限不能因 Observer 而默认拥有整个 workspace/home directory。

Mode semantics：

| Operation | Observer | Confirm | Auto |
| --- | --- | --- | --- |
| ordinary read in declared scope | allow | allow | allow |
| sensitive read (secret-bearing metadata, terminal selection, credential refs) | deny unless explicitly classified safe | per-operation approval; persistent grant only for canonical non-secret resource | allow only inside declared capability/scope; secret plaintext still host-only |
| network request | deny by default except approved provider/agent protocol endpoints | approve canonical origin/resource; redirects reauthorize | allow declared origins only；private/metadata destinations remain denied |
| write | deny | per-operation approval/grant | allow in scope |
| shell/process execution | deny | per-operation approval; vendor local shell disabled unless fully mediated | allow only through Netcatty capability/process policy |
| plugin/hook/LSP/formatter/extension execution | deny | session-only explicit approval after provenance and scope checks | allow only if declared, provenance-verified and contained |

Vendor-local actions never receive an unbounded `always` grant. Persistent grants belong to
Netcatty capabilities and canonical resources, not opaque vendor tool names。

### 3.5 Attachment owner

Go 是 attachment staging owner。`TurnRequest` 接收 bounded bytes/opaque upload handle，
不信任 Wails 提供的任意本地 path。Go 校验 count、aggregate size、MIME、hash、scope
和 retention，再决定：

- vendor native image/content block；
- 只读临时文件 + capability-scoped path；
- native Netcatty MCP attachment handle；
- unsupported（不偷偷降级或把内容塞入 argv）。

临时文件只写入 Netcatty 专用目录，turn/session cleanup 或 retention deadline 后删除。

## 4. Agent 协议与替代矩阵

| Agent/surface | 当前嵌入 | Go 目标协议 | Resume | Cancel | Steer | Models | Node-free classification |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Catty | Renderer Vercel AI SDK + Electron provider proxy | Go HTTP/SSE provider adapters + Go tool loop | Netcatty session | context cancel | Go runtime owned | provider APIs/config | `replaceable` |
| Claude | `@anthropic-ai/claude-agent-sdk` + `claude` | native Claude headless stream-json | documented `--resume` | protocol/process signal | not currently portable | SDK has catalog；CLI parity unresolved | `replaceable-candidate` pending provenance/catalog/policy fixtures |
| Codex SDK | `@openai/codex-sdk` spawning Codex | retire into App Server owner | yes | SDK signal | no | none | `retirement-candidate` after App Server parity；do not retain second owner |
| Codex App Server | persistent `codex app-server --stdio` | Go JSONL RPC client | `thread/resume` | `turn/interrupt` | `turn/steer` | `model/list` | `replaceable-candidate` preferred Codex owner |
| Copilot | `@github/copilot-sdk` + Copilot CLI | official Go SDK / JSON-RPC v3 | yes | session abort | protocol supports send modes | yes | `retirement-decision`: current runtime embeds Node |
| Cursor API key | `@cursor/sdk` in Electron | Cursor SDK Bridge `sdk.v1` Connect/protobuf | yes | CancelRun | no general steer | yes | `replaceable` only with approved embedded Bun runtime |
| Cursor CLI login | `cursor-agent` stream-json；Windows node.exe+index.js | ACP if upstream supplies non-Node authenticated runtime | yes | ACP cancel | no stable v1 steer | CLI/human output today | `needs-upstream-protocol` |
| OpenCode | `@opencode-ai/sdk` starts `opencode serve` | Go HTTP/OpenAPI/SSE client to native binary | session ID | session abort | no current portable steer | `/config/providers` | `replaceable-candidate`；Bun accepted only by decision |
| CodeBuddy | `@tencent-ai/agent-sdk` + Node CLI | ACP/headless protocol has semantics, but executable is Node | yes | interrupt/cancel | queued input exists but Netcatty V2 steer disabled | yes | `retirement-decision` until native runtime exists |
| Grok ACP | per-turn `grok agent stdio` ACP client | reusable Go ACP v1 core + Grok extension adapter | resume/load/new | graceful `session/cancel` | ACP v1 no portable steer | Grok `_meta`/session options | `replaceable-candidate` after permission/reverse-RPC fixes |
| Grok streaming-json | `grok -p --output-format streaming-json` | explicit degraded fallback only | `-r` | process kill only | no | `grok models` | `degraded-candidate`，not recommended owner |

## 5. Agent-specific decisions

### 5.1 Catty

Catty 不是外部 CLI，而是 Netcatty 自有 Agent。最终 Go runtime 直接实现：

- OpenAI Chat/Responses-compatible；
- Anthropic Messages；
- Google Generative Language；
- provider streaming、timeouts、tool loop、compaction、trace、redaction。

React 不再使用 Vercel AI SDK 承担 authoritative turn。Catty 是最完整的 Go-native
路径，但属于 P7-03/P7-04，不在 P0-05 实施。

### 5.2 Claude

当前 owner：`claudeDriver.cjs` 动态 import Node SDK，Node SDK 通过配置的
`claude` executable 工作。当前 SDK 版本 `0.3.161`，要求 Node >=18。

目标：Go 启动官方 native Claude executable 的 headless stream-json 协议，解析
JSONL，保留 session ID/resume/images/MCP/stop。必须拒绝 legacy `cli.js` 与 Node
launcher。

阻断：CLI 的 machine-readable model catalog 尚未证明与 `supportedModels()` 等价。
若 Phase 7 前仍无稳定 catalog，需批准 curated models 或降低 model discovery parity。

### 5.3 Codex

Codex App Server 已接近目标形态：当前 Electron 只是 JSONL client。它支持：

- thread start/resume；
- turn start/completed/interrupt/steer；
- model list；
- native approvals 和 `request_user_input`；
- image inputs；
- MCP 配置与重复 approval 抑制。

目标：把 `CodexAppServerConnection` 和 runtime state machine 移植到 Go，版本锁定
schema。Codex SDK path 应删除；`codex exec --json` 只能在另获批准的 reduced mode
中保留，不能成为永久第二 owner。

注意：当前 connection 允许 `.js` entry 走 Node；Go 版本必须要求 native executable。

### 5.4 GitHub Copilot

当前 SDK `1.0.0`，CLI `1.0.59`，npm loader 是 `#!/usr/bin/env node`。官方 Go SDK/
JSON-RPC v3 可以替代 Netcatty Node wrapper，但当前 Copilot agent runtime 自身包含或
嵌入 Node。因此“Go client”不等于端到端 Node-free。

状态：`retirement-decision`。Phase 7 前必须二选一：

1. 上游提供经三平台验证的非 Node runtime；
2. 批准在 Wails 版本中 disable/retire Copilot。

ACP 仍处 public preview，不能单独解除 runtime blocker。

### 5.5 Cursor

API-key mode 当前把 SDK loop 跑在 Electron Node 进程中，`autoReview:false` 不是
Netcatty Confirm approval。推荐 Cursor SDK Bridge `sdk.v1`，由 Go 走 Connect/
protobuf。Bridge 是 standalone executable，但内嵌 Bun/TypeScript SDK：不属于 Node，
仍属于外部 JavaScript runtime。是否允许该 runtime 必须有 architecture decision，
并要求 hash/signature/SBOM/process-tree、built-in write tools disabled。

CLI-login mode 当前 Windows 明确解析到 `node.exe + index.js`，Confirm/Auto 又使用
`--force`。ACP 协议语义可用，但登录 runtime 仍是 Node。状态：
`needs-upstream-protocol`；Phase 7 前若无非 Node runtime，则转为 retirement decision。

### 5.6 OpenCode

当前 Node SDK 本质上启动 `opencode serve`，再用 HTTP/SSE client 访问 session 和
events。Go 可直接使用 server OpenAPI/SSE，保留：

- session create/resume；
- async prompt/events；
- session abort；
- provider/model catalog；
- MCP status/config；
- permission request/reply；
- image/file parts。

必须直接启动 native release binary，拒绝 npm Node launcher。OpenCode native binary
内嵌 Bun，和 Cursor Bridge 一样需要外部 runtime 接受 decision。使用 `--pure` 或
隔离 config 禁止用户 plugin/LSP/formatter 无界启动，并给 server 随机 password。

### 5.7 CodeBuddy

当前 SDK `0.3.230`，CLI `2.127.2`；本地 `bin/codebuddy` 明确是
`#!/usr/bin/env node`。ACP/headless 协议虽然支持 session、cancel、permission、image
和输入流，但没有非 Node executable，不能满足 WV3-002。

状态：`retirement-decision`。Phase 7 前必须得到上游 native runtime，否则在 Wails
版本中 disable/retire。

### 5.8 Grok

Grok 本体可直接使用 native Rust binary，不能通过 npm/npx Node trampoline。
ACP v1 是推荐 owner，streaming-json 仅作显式 degraded fallback。

当前 ACP driver 存在目标阻断：

- Confirm/Auto 统一 `--always-approve`；
- Observer 只是软 auto/plan，不是硬 read-only；
- permission request 未经过 Renderer/Go policy；
- unknown reverse JSON-RPC request 无 method-not-found response；
- 未校验返回的 protocolVersion；
- cancel 后立即 kill，丢弃 final updates；
- 每 turn 新 process，不支持 long-lived connection/steer；
- attachments 只作为路径提示，不使用 ACP capability-negotiated content；
- model/auth 使用 Grok `_meta`，未采用稳定 session config/auth capability。

目标 Go ACP core 应提供 correlation、version negotiation、session resume/load、
reverse request router、permission callback、graceful cancellation、attachments、
config options 和 agent-specific extensions。Go community ACP SDK 可参考，但当前落后
稳定 ACP v1 schema，不能未经 gap audit 直接采用。

streaming-json 还会临时写 `.grok/config.toml` 并可能把 MCP token 留在项目目录，
因此不能作为最终默认路径。

## 6. Session identity 与历史

当前 `sdkSessionIdentity` 把 external session ID 与以下字段绑定：

- backend；
- binary path；
- runtime（sdk/app-server/acp/streaming-json）；
- Cursor auth mode；
- Cursor CLI ask/agent mode。

Go 必须保留并扩展 identity：

```text
schemaVersion
profileId
chatSessionId
vendor
adapterProtocol
protocolVersion
executableDigest
executableVersion
authProfile
permissionClass
toolIntegrationMode
workspaceIdentity
canonicalCwd
filesystemRootsDigest
networkClass
capabilityCatalogRevision
policyRevision
vendorConfigIsolationMode
externalSessionId
```

只要以上任一 security/runtime boundary 不兼容，就不能 resume。Vendor
resume 失败时不能静默创建 fresh session 后丢失未 replay 的历史；应返回 typed
`stale-session`，由 Go runtime 决定是否用完整安全 history seed 新建。

## 7. Wails UI binding

不建议为每个 vendor 暴露一套 Wails method。只暴露通用 Agent service：

```ts
interface AgentClient {
  prepareTurn(request: StartTurnRequest): Promise<{ turnId: string; cursor: number }>;
  startTurn(turnId: string): Promise<void>;
  stopTurn(turnId: string): Promise<void>;
  steerTurn(turnId: string, input: SteerInput): Promise<SteerResult>;
  resolveInteraction(input: InteractionResponse): Promise<void>;
  listModels(agentId: string): Promise<ModelCatalog>;
  inspectRuntime(agentId: string): Promise<RuntimeInspection>;
  subscribe(turnId: string, cursor: number, listener: (event: AgentEvent) => void): () => void;
}
```

事件包括 text/reasoning/tool call/result/file change/web search/plan/approval/usage/
warning/error/end。Go 负责 event ordering、exactly-once terminal event 和 bounded
payload。React 只更新 UI。

普通 Agent event 可以走 Wails event/stream；terminal raw bytes 仍走独立 terminal
data plane，不能与 Agent event stream 混为一体。

## 8. 实施顺序

Phase 7 production work has a hard prerequisite: P6-05 and its chronological
`Gate: NONAI-COMPLETE` ledger record have passed, including all release-target
and P0-05 Bun/runtime/retirement entry decisions. Before that gate, only P0-05
read-only handshake probes and disposable fixtures are allowed.

1. P7-01 Go capability catalog/policy and P7-03 Go providers may start after the gate.
2. P7-02 native `netcatty-mcp` and `netcatty-tool` follows P7-01 and removes every
   vendor's common Node child.
3. P7-04 Go AgentRuntime follows P7-01, P7-02 and P7-03 plus verified capability handlers.
4. P7-05 then migrates retained adapters: Codex App Server, Claude headless, Grok
   ACP, OpenCode HTTP/SSE, and only decision-approved Cursor/Copilot/CodeBuddy paths.
5. P7-06 finally deletes Node SDK dependencies, CJS drivers and Wails-side
   Electron IPC/preload surfaces.

P7-05 must establish child rows before Agent adapter implementation. The
following are suggested names, not current capability IDs:

- Claude
- Codex
- Copilot
- Cursor API key
- Cursor CLI login
- OpenCode
- CodeBuddy
- Grok

## 9. 验证要求

每个 retained adapter 在 Windows/macOS/Linux required rows 上执行无付费握手：

1. executable provenance：file format、shebang、hash、signature、SBOM；
2. recursive process tree：拒绝 Node/未批准 wrapper；
3. protocol initialize/version/capabilities；
4. model catalog；
5. create/resume/stale-session；
6. cancel graceful completion 与 forced cleanup；
7. permission allow/deny/timeout/unknown request；
8. MCP native subprocess 和 scope；
9. image/file attachment capability；
10. malformed/oversized/partial frame；
11. process crash、restart、orphan cleanup；
12. event normalization golden fixtures。

授权测试账号仅在上述无付费握手通过后使用，用于验证真实 permission、resume、usage
和 model response。付费调用不能是唯一证据。

## 10. 当前证据

锁定版本：

| Package | Current version | Runtime signal |
| --- | ---: | --- |
| `@anthropic-ai/claude-agent-sdk` | 0.3.161 | Node >=18 SDK |
| `@openai/codex-sdk` | 0.144.3 | Node >=18 SDK |
| `@github/copilot-sdk` | 1.0.0 | Node >=20 SDK |
| `@github/copilot` | 1.0.59 | `copilot -> npm-loader.js` Node shebang |
| `@cursor/sdk` | 1.0.18 | Node >=18 SDK |
| `@opencode-ai/sdk` | 1.17.9 | Node client/server launcher |
| `@tencent-ai/agent-sdk` | 0.3.230 | CodeBuddy CLI Node shebang |

本机 PATH 不存在 claude/codex/copilot/cursor-agent/opencode/grok/codebuddy，因此没有
执行真实 handshake、auth 或 paid turn。

测试：

- Codex App Server + Grok ACP/streaming-json + session identity 专项：102 通过；
- 全部 SDK/App Server/session identity 测试：375 项中 366 通过、9 失败；
- 9 个失败均发生在当前 Windows 对 POSIX path/workspace fixture 的断言，集中于
  Cursor MCP merge 和 OpenCode path/shim tests；P0-05 未修改这些 owner。

测试通过不消除已被测试固定的不安全语义，例如 Grok Confirm→always-approve。

## 11. 状态与决策点

| Agent | P0-05 status | 最迟决策点 |
| --- | --- | --- |
| Catty | replaceable | Phase 7 入口固定 provider protocol |
| Claude | replaceable-candidate | Phase 7 Agent adapters 前解决 model catalog/native provenance/tool policy |
| Codex | replaceable-candidate | Phase 7 选择 App Server 单 owner并完成 schema/native provenance |
| Copilot | retirement-decision | Phase 7 AI 入口前 native runtime 或退休 |
| Cursor API key | replaceable with architecture decision | Phase 7 AI 入口前批准/拒绝 embedded Bun Bridge |
| Cursor CLI login | needs-upstream-protocol | Phase 7 AI 入口前 non-Node ACP runtime 或退休 |
| OpenCode | replaceable-candidate with architecture decision | Phase 7 AI 入口前批准/拒绝 native Bun runtime |
| CodeBuddy | retirement-decision | Phase 7 AI 入口前 native runtime 或退休 |
| Grok | replaceable-candidate after protocol fixes | Phase 7 adapters 前完成 ACP conformance/permission design |

P0-05 停止状态：`needs-verification`。每个 Agent 已有 Go 路线或明确产品决策点，
但未完成三平台 executable/process/protocol handshake，因此不能升级 AI-04 状态。

## 12. 上游参考

- Claude headless: `https://code.claude.com/docs/en/headless`
- Claude Agent SDK overview: `https://platform.claude.com/docs/en/agent-sdk/overview`
- Codex App Server: `https://developers.openai.com/codex/app-server/`
- Codex non-interactive: `https://developers.openai.com/codex/non-interactive-mode/`
- Copilot SDK Go: `https://github.com/github/copilot-sdk/tree/v1.0.11/go`
- Copilot ACP: `https://docs.github.com/en/copilot/reference/copilot-cli-reference/acp-server`
- Cursor SDK Bridge: `https://cursor.com/docs/sdk/bridge`
- Cursor ACP: `https://cursor.com/docs/cli/acp`
- OpenCode server: `https://opencode.ai/docs/server/`
- OpenCode ACP: `https://opencode.ai/docs/acp/`
- CodeBuddy ACP: `https://cnb.cool/codebuddy/codebuddy-code/-/blob/main/docs/acp.md`
- ACP v1: `https://agentclientprotocol.com/protocol/overview`
- Grok Build: `https://github.com/xai-org/grok-build`
