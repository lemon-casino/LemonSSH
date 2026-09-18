# AI 迁移工作包与验收手册

日期：2026-09-14（2026-09-18 更新）。状态：W01-W05 已实施（L160-L165；W04 Vault 域经审计裁决为无需提取，W05 落于 internal/capability 四提交并过 77 行 fixture parity）；W06 核心已落（L166，internal/rpc）；W07 首两刀已落（L167，native binaries + SDK v1.7.0，composition root 与 vendor 矩阵待后续切片）；W08 已落（L168，internal/platform/netpolicy）；W09 流装配核心已落（L169；Responses 族与 model list/probe 待补）；W11 三刀已落（L170-L172）；W10 首刀已落（L173 key 清单+迁移计划）；W12 三刀已落（L174-L176）；W10 切片 2/3 已落（L177 reseal；L178 secret 服务+原子推广）；活体冒烟待跑。本文件不记录 capability 完成状态。先读 [执行方案](ai-migration-execution-plan.md)，再读 [技术设计](ai-migration-technical-design.md)。W 编号仅为本手册内部工作包编号，不是新增正式 plan task、capability ID 或 ledger ID。

## 1. 执行顺序与本轮允许范围

**WV3-025 已开生产门。** 先读 matrix/ledger/decisions。不要伪造 `NONAI-COMPLETE`。W01 基线已在；W02 是非 AI 资格，不能计成 AI 完成。W03 契约已落（WV3-L160，AI-01 probe），W04 共享 use case 已收口（L161-L164），W05 catalog/policy/dispatch 已落（L165）。下一刀 W12 活体冒烟 / W10 收口审计 / W13。仍禁止永久 Node sidecar，仍禁止把 `not-started` 写成 `verified`。

开工后的推荐顺序：

```text
文档/资格准备：W01 + W02（NONAI-COMPLETE 不再是 W03+ 的开工前置，WV3-025；其证据债保留）
契约与宿主能力：W03 -> W04 -> W05 -> W06 -> W07
Provider：      W03 -> W08 -> W09
最小完整链路：  W05 + W07 + W09 -> W10 -> W11 -> W12
Catty 完整功能：W12 -> W13 -> W14 -> W15
外部 Agent：    W15 -> W16 -> W17/W18/W19/W20/W21（逐个资格验证）
收口：         所有保留能力验收 -> W22 -> W23 -> 正式发布阶段
```

这是依赖图，不要求自动启动多个 AI。共享 DTO/catalog/schema 的变更先合入，避免不同执行者同时修改 authority。一次工作包可拆成数个有独立验收的提交；一个提交不混入工具链升级、数据 schema 迁移和多 vendor 重写。

最小完整链路的定义：React 输入 -> Wails -> Go Prepare/Start -> fake/fixture Provider -> 一个真实受控只读工具 -> Go 事件/历史提交 -> React 恢复；Stop 能在各阶段收敛。它用于提前暴露跨层缺陷，不能替代所有工具和真实协议的验收，也不能作为功能缩水的发布版本。

## 2. 文件级工作包

表中的新文件名是建议位置；优先复用实际已存在的 owner。每包完成前都应补“旧功能 -> 新 owner -> 测试 -> 旧路径退出条件”，仅创建目录/空实现不算完成。

### W01：基线与缺口清单

- 正式归属：P7-01 前置核对；门禁前只做文档，符合范围的 disposable 探针归 P0-05。
- 读取：`testdata/migration/electron/`、`electron/capabilities/catalog/`、`infrastructure/runtime/generated/runtimePorts.ts`、AI hooks/harness、`baselines/external-agent-protocols.md`。
- 产物：`baselines/ai-phase7-parity.md`（实施时新增）与证据 manifest。77 个 catalog ID、全部 AgentPort 方法、所有 AI storage key、全部 vendor 逐项列举，注明 required/planned/removal 的真实治理状态。
- 操作：冻结 semantic schema/policy 对照，给每个方法查真实调用点和错误/取消语义；将当前 Go facade 能力与缺失的 scope/job/approval 区分。
- 验收：无未说明遗漏、无“方法名类似所以完成”；密钥/真实主机信息脱敏；基线 hash 与版本一致。缺稳定协议或 accepted disposition 的 vendor 保持阻塞。

### W02：Go/Wails 版本对齐与基础复验

- 正式归属：既有 foundation/release 资格验证，依影响范围记录；不能计成 AI 功能完成。
- 目标文件：`go.mod/go.sum`、`package.json/package-lock.json`、现有 CI/toolchain 配置、`scripts/wails-build.mjs` 与各平台构建入口、`infrastructure/runtime/wails/bindings/`；涉及 experiment modules 时逐个对齐资格范围。
- 操作：记录当前 compiler/module/runtime/generator 版本；评估技术设计中的受支持 Go 候选；锁定同一 Wails 发行组合并重新生成；建立版本漂移检查，不能只改 npm 不改 Go。
- 验收：新组合的 bindings、service/event、关闭/reload、三平台基础 shell 与受影响非 AI gates 复验。失败回退整个版本组合；不保留不可复现混搭。若资格变更使 gate 失效，先补新 gate，才继续 W03。

### W03：契约与测试用 driver seam

- 归属：P7-01；依赖 W01、W02 和有效 gate。
- 目标：`internal/app/contracts/{registry,errors}.go`、建议 `agent_contracts.go`、`tools/contracts-codegen/`、生成 TS；新增 fake driver 仅放 test support。
- 实施：落成技术设计 DTO/error、string revision、capabilities、幂等语义；编写 transport-independent interface；不提前做网络/vendor 实现。
- 验收：Go -> JSON -> TS roundtrip，超过 JS safe integer 的序列无损，未知版本/非法 union 拒绝，generator 无 drift；internal 包不 import Wails、Electron 或 UI。

### W04：共享 use case 提取

- 归属：P7-01；依赖 W03。
- 原 owner：`cmd/netcatty/{terminalService,sftpService,forwardService,profileService}.go`、Vault 相关 facade、`internal/terminal/`。
- 目标：`internal/app/` 中真正共用的 terminal/session/exec、SFTP、forward、Vault/snippet 接口与实现；Wails facade 只映射 DTO 和调用。
- 实施：每次提取一个域，保留现有连接池/事务/secret 边界；明确哪些业务仍在 React hook，迁成 canonical host command 后再允许 Agent 写。
- 验收：原非 AI tests 和真实行为不回退；两条入口调用同一实例/规则；SFTP/forward 不产生第二套 pool，Vault 写完原 UI 收到一致 revision；不能从 internal import package main。

### W05：catalog、policy 与可用性

- 归属：P7-01，AI-01；依赖 W04。
- 原 owner：`electron/capabilities/{catalog,policy.cjs,codegen/}`。
- 目标：`internal/capability/`、Go authority 生成器、TS sidebar/global 与 MCP/CLI 投影。
- 实施：保留 stable ID/schema/agentKinds/surface/policy 例外；runtime availability 与 baseline status 分开；注入已实现 host handler，harness 暂留显式缺口。未知 capability 默认拒绝。
- 验收：77 行逐 ID semantic diff；全 mode/surface/kind/scope policy 表；Confirm 参数绑定、grant 撤销和 Stop 竞态；harness 未就绪不暴露。该包不单独将 AI-01 标 verified。

### W06：本机 host RPC 与生命周期

- 归属：P7-02，AI-02；依赖 W05。
- 原 owner：`electron/bridges/mcpServerBridge.cjs`。
- 目标：`internal/rpc/`、桌面 composition root、discovery/token store。
- 实施：versioned request、强随机 token、first-party/external principal 区分、scope owner、deadline/cancel、frame bounds、启动/退出/锁屏/撤销清理；内部 discovery 只按既有 first-party 契约实现。
- 验收：伪造 chat ID、重用撤销 token、跨 profile、oversized/truncated frame、disconnect、app shutdown；stdio stdout 不夹日志，认证信息不在 argv/trace。Windows ACL 与 POSIX mode 用实平台证据。

### W07：原生 MCP/CLI

- 归属：P7-02，AI-02；依赖 W06。
- 目标：`cmd/netcatty-mcp/`、`cmd/netcatty-tool/`、打包 helper manifest；复用 W05/W06，不建立 policy 副本。
- 实施：官方 MCP Go SDK 做两代协议；对 catalog CLI fallback 和 exec/jobs/SFTP/session special cases 分别移植。旧 CLI JSON shape、exit code、Windows quoting 与 discovery launcher 做 golden。
- 验收：新版 server/discover/逐请求版本、旧版 initialize、真实旧 vendor client fallback、tools/list/call、cancel、错误/重连；MCP 版本拒绝不能降成更宽权限。binary 可脱离 Node 启动，未开 app 有 typed unavailable。

### W08：Provider 网络基础

- 归属：P7-03，AI-03；依赖 W03。
- 原 owner：`electron/bridges/aiBridge/providerHandlers.cjs`、provider fetch/headers/proxy tests。
- 目标：`internal/platform/netpolicy/`；实际 SDK client 必须注入此 transport。
- 实施：endpoint/auth/header/代理/TLS 规则、DNS 与实际 Dial 的地址一致、redirect 重判、body/header/idle 限额、取消；显式本地端点有受限授权。
- 验收：用本地 resolver/HTTP/TLS fixture 验证目的地址；远端解析代理的保证范围明确；无密钥跨 origin，不跟随未经允许的 redirect；不访问真实云 metadata。

### W09：三个 Provider 协议族

- 归属：P7-03，AI-03；依赖 W08。
- 原 owner：`infrastructure/ai/sdk/providers.ts`、`providersBridgeFetch`、`sdkProviders`、`providerContinuation` 相关 tests。
- 目标：`internal/agent/providers/`；官方 SDK 版本锁定，按 API 家族封装。
- 实施：OpenAI Chat/Responses、Anthropic、Google 按独立 fixture 逐个接入；功能包括 list/probe/model params/reasoning/continuation/tool fragments/usage/error；统一 retry owner，支持 fake clock。
- 验收：本手册 Provider 用例全通过；输入协议与旧配置一致；tool 参数完整前不 dispatch；未知 usage 不填 0；未获 API 消费授权时只运行本地 fixture/无付费探针，真实验证明确标未执行。

### W10：AI Profile 数据与 secret references

- 归属：P7-04 第一切片，AI-03；依赖 W05、W07、W09。
- 原 owner：`useAIState.ts`、`useAISettingsState.ts`、`aiStateSnapshots.ts`、`useAIPermissionGrantsState.ts`、`storageKeys.ts`、`profileDomain.ts`。
- 目标：Profile staging/migration、AI canonical records、专用 secret API、旧 DTO reader、React hydration/写入口。
- 实施：manifest/备份 -> 逐 key staging -> origin-aware decrypt/reseal -> 校验 -> 原子 promotion -> 单 writer 切换；sessions/continuation/active maps/grants/sync 分类一次映射清楚。
- 验收：中断各步后能重试或整套回滚；旧 history IDs/顺序/活动/usage 保留；密钥不会通过 raw Profile/通用 Open 绕回 UI；不可解密时 promotion 不发生。AI exclusion 只能在该 profile 成功 promotion 后改变行为。

### W11：Go Runtime 状态机、checkpoint 与交互

- 归属：P7-04，AI-03；依赖 W10。
- 原 owner：`harness/{agentRuntime,agentStop,traceStore}.ts`、`shared/approvalGate`。
- 目标：`internal/agent/{runtime,events,sessions}/`，使用 W03 fake driver 验证状态机。
- 实施：Prepare lease、幂等 Start、单 chat active slot、可恢复 snapshot/ring、唯一 finalize、InteractionRouter、统一 stop、崩溃 interrupted 与 tool unknown receipt。
- 验收：本手册状态机用例、synctest 竞态、race、事务失败/commit 后通知丢失；锁内无网络等待，取消不能终止其他 turn/shared pool。仅此包通过还不代表 Catty 功能 parity。

### W12：Wails AgentClient 与 UI 最小链路

- 归属：P7-04，AI-03；依赖 W11。
- 目标：建议 `cmd/netcatty/agentService.go`、`main.go`、`wailsRuntimeClient.ts` 中 defaultBindings/agent 分支、两层 `useAIChatStreaming.ts`、AI message reducer 和相关 UI tests。
- 实施：生成 Wails bindings，替换 unimplemented agent，接入 DTO/事件补读/快照；保留旧 AgentPort compatibility mapper，React 不启动第二个 authoritative runtime。使用 fake Provider 和一个真实只读 use case 先跑完整链路。
- 验收：reload、StrictMode 双挂载、多窗口、末条通知丢失、cursor expired；重复 Start 仅一条 user message/一轮模型；组件 unmount 不误触 Stop；用户明确 Stop 走 Go owner。fake 只能在 test/dev 专用依赖注入，发布包不能回退到假响应。

### W13：全部 host 工具与审批执行链

- 归属：P7-01/P7-02 的 handler 补齐，在 P7-04 集成；AI-01、AI-02、AI-03 逐行按实际证据记录。
- 依赖：W12；目标 W04 shared use cases、W05 dispatch、W11 interactions。
- 实施：terminal exec/job queue、session、SFTP/transfer、Vault/snippet/notes、forward、attachments、control/meta；一个域一次贯通 schema -> policy -> use case -> output -> UI artifact。
- 验收：77 行中 host 域无遗漏；至少每域一个真实正例和 deny/cancel/跨 scope 负例；远程写结果未知不自动重做；旧 UI 仍看到同一 canonical Vault/transfer/forward 状态。

### W14：输出 handle、context 与 continuation

- 归属：P7-04 第二切片，AI-03；依赖 W13。
- 原 owner：`toolOutputStore/toolResultDedup/contextBudget/contextManager/sessionState/compactionPruner/staleContextPruner/cattyRuntime`。
- 目标：`internal/agent/{tools,context}/` 与 checkpoint 的 provider-private records。
- 实施：先迁纯 helper 与现有 golden；再接 spill、TTL、scope、dedup；再接 pre-turn/step/413 与无损 continuation。目录 `harness.*` handler 在此/下一包按实际依赖注入。
- 验收：UTF-16 offset parity、handle 跨 turn/删除/TTL、预算边界、signed parts、未闭合 tool call、context reinjection；压缩不能重跑已执行写工具。

### W15：完整 Catty tool loop 与功能 parity

- 归属：P7-04 第二切片，AI-03；依赖 W14。
- 原 owner：`turnDrivers/cattyTurnDriver.ts`、`cattyStreamProcessor.ts`、`cattyToolApproval.ts`、`cattyRuntimeContext.ts`。
- 实施：拼合 Provider -> tool selection -> policy/approval -> execution -> result -> next step；max iterations/timeouts/retry/stop/compaction；补齐 skills/system prompt、搜索配置条件、terminal hostChain/activePortForwards 上下文。
- 验收：Gate 11 canonical traces、真实 terminal/SFTP/Vault 跨层闭环；React Wails bundle 不再持有 `streamText` 执行路径；harness 仅 sidebar，global 不新增产品入口；所有保留 catalog handler 均有证据才关闭 AI-01 对应缺口。

### W16：外部 process、session identity 与行政操作基础

- 归属：P7-05，AI-04；依赖 W15。
- 前置：先按治理规则创建 AI-04 子行并取得所需 accepted runtime/disposition decisions；内部包号不代替子行。
- 目标：`internal/agent/{process,adapters,models}/`、runtime inspection、account/config administration contracts。
- 实施：process supervisor、JSONL correlation、session fingerprint、capability declaration、interaction、Go MCP injection、附件 staging；不同协议独立 lifecycle，基础 parsing/cleanup 可共用。
- 验收：crash/EOF/未知 ID/重复响应/超大帧、cancel grace/kill、递归 Node 检测、cwd/env/config isolation、重新登录或 roots 改变的 stale-session；不能靠一个 Go wrapper 包住 Node 然后声称完成。

### W17：Codex App Server

- 归属：P7-05.1，AI-04.1；依赖 W16。
- 读取：`electron/bridges/aiBridge/codexAppServer/` 与 committed protocol schema；依照技术设计的官方来源复核锁定版本。
- 实施：initialize/initialized、thread/start/resume、turn/start/interrupt/steer、model/list、native approval/user input、账户/登录、usage/events；复用 InteractionRouter，SDK owner 从 Wails 退出。
- 验收：retryable error 后继续、turn/completed 唯一终结、pending native grant 不永久化、SDK identity 拒绝 resume、cancel 后最终通知与 crash 均覆盖；schema 生成/check 与实 binary smoke 对应同版本。

### W18：Claude native headless

- 归属：P7-05.2，AI-04.2；依赖 W16。
- 实施：验证 native provenance、stream-json/partial/result、resume、MCP、附件、权限交互、models、账户/配置；没有稳定模型发现协议时保留明确缺口，不用任意硬编码列表冒充实时发现。
- 验收：完整最终 result 不被截断，流慢读/EOF/非零退出有区分；内置工具受约束，Confirm 不用自动批准；CLI 原生 runtime 与 models parity 均有目标平台证据才可完成。

### W19：Grok/ACP

- 归属：P7-05.3，AI-04.3；依赖 W16。
- 实施：ACP initialize/auth/new/load/prompt/update/cancel 与 reverse permission/fs/terminal；host 只 advertise 可中介的能力，再加供应商具体扩展。
- 验收：prompt response 的 stop reason、cancel 与 reverse request 竞态、deny/timeout、无 scope 拒绝；不支持 load/steer 返回 unsupported；旧 Confirm always-approve 路径退出。

### W20：OpenCode 与 Cursor API

- 归属：P7-05.4（AI-04.4 Cursor API-key）与 P7-05.5（AI-04.5 OpenCode）；依赖 W16、WV3-014、WV3-015、WV3-019、WV3-020。
- 分开提交：两行已 `removed` / `retired`（L152, L153）。本包在 P6-05 之后只做 typed-unavailable 映射，不实现 Bun owner，也不再补 scope-removal。若 superseding decision 重开，OpenCode 锁 HTTP/OpenAPI/SSE 与私有 server lifecycle；Cursor 锁 sdk.v1 descriptor、Connect/protobuf 与 CancelRun/model/resume。两者只有 supervisor/host policy 可共用，不能臆造一个通用协议。
- 验收：AgentPort fail-closed、设置页 unavailable 原因、历史可读、零付费调用、无 Bun/Node child。

### W21：其余 Agent disposition 与完整设置入口

- 归属：P7-05.6（AI-04.6 Copilot）、P7-05.7（AI-04.7 CodeBuddy）、P7-05.8（AI-04.8 Cursor CLI）；依赖 W16、WV3-016、WV3-017、WV3-018、WV3-021、WV3-022、WV3-023。
- 实施：三行已 `removed` / `retired`（L154, L155, L156）。本包不再补 scope-removal。P6-05 之后补 AgentPort 中 discover、account/login/logout、skills、MCP integration、plugin/marketplace 管理的 fail-closed 映射与明确 UI 原因。
- 验收：历史可读，旧 active/default Agent 不可用时不盲选另一账户或发起付费调用；选模型/登录/取消/退出/设置启停均有可观察结果。

### W22：前端与 CJS/Node 调用链收口

- 归属：P7-06，AI-01、AI-02、AI-03、AI-04；依赖 W15 及所有 retained vendor 工作包。
- 目标：runtime adapter、AI hooks、generated tool specs、Wails bundler 入口/打包资源、MCP/CLI binary 注入配置。
- 实施：确认所有 Wails AI 调用有 Go owner 后删除该调用链中的 CJS catalog/bridge/Node SDK 使用；保留冻结 Electron release carrier 到正式 P9 删除条件。构建时 Node 与发布运行时 Node 分开审计。
- 验收：对保留旧文件的引用逐个说明为何只属于冻结 Electron；Wails bundle/process tree/helper SBOM 无禁止 Node runtime；AI state/permission/runtime 无双 owner。不能仅 grep 到零个单词就宣称无 Node。

### W23：验收封包与发布阶段交接

- 归属：P7-06 的证据收口；P8-01、P8-02、P8-03、P9-01、P9-02 按正式顺序另行执行。
- 产物：逐 row evidence 索引、协议版本/产物 hash、三平台矩阵、迁移/回滚报告、残留旧路径及唯一 retirement trigger、后续 release checklist。
- 验收：AI required leaves 与 aggregate 状态满足真实证据，Gate 10/11/14 对应部分齐全；REL-03.1/REL-03.2 不越级。未执行的签名/平台/真实 Provider 测试标未执行及原因，不能用 fixture PASS 填入 A 级验收。

## 3. 具体测试用例：输入、故障点、可观察结果

下面用例编号仅用于 evidence。新 Go test 函数/fixture 文件在对应工作包实施时创建；本轮不假称这些测试已存在或通过。现有 Electron/TS tests 是对照输入，不能直接当作 Go 已通过。

### 3.1 状态机、UI 与持久性

| 用例 | 输入/故障注入 | 必须观察到的结果 |
| --- | --- | --- |
| T01 | 同 chat 两个不同 requestId 同时 Prepare | 一个 reservation，另一个 busy；无模型调用 |
| T02 | 相同 requestId 重试 Prepare/Start；另测同 ID 改参数 | 相同 turn/结果、单条 user message；改参数 conflict |
| T03 | Prepare 后 UI 消失直到 lease 到期 | reservation 释放，下一次可 Prepare；无伪 turn_end |
| T04 | Start 成功后首条 Wails 通知丢失 | ReadEvents 补出 turn_start/后续文本，历史无重复 |
| T05 | 只丢最后一条完成通知，之后无新事件 | reconciliation/重新聚焦读到 terminal snapshot，不永久转圈 |
| T06 | 先收到较大 sequence，再收到重复/较小 sequence | 缺口补齐、只应用一次；同序不同 payload 报协议/一致性错误 |
| T07 | 消费 cursor 已被 ring 淘汰 | snapshot 原子替换，并只追加 throughSequence 之后事件 |
| T08 | React StrictMode 双 mount/unmount；reload 同 chat | 只保留一个有效订阅/补读 loop，不调用隐式 Stop，不启动第二 runtime |
| T09 | 同一审批同时来自两个窗口的 approve/deny | 一个有效消费，另一方 stale/already-resolved；至多一个 dispatch |
| T10 | approve 与 Stop/policy revision 更新同时发生 | 按 host 原子顺序决胜；取消/撤销后不启动新写 |
| T11 | 在 Provider 等待、tool 等待、compaction、审批各阶段 Stop | 每阶段都有有界收敛、一个 terminal record；共享其他会话继续运行 |
| T12 | terminal 事务 commit 后故意丢通知并重启 | 历史/usage/terminal 状态一致，ReadEvents 可恢复；无重复 usage |
| T13 | tool dispatch 前 checkpoint 后进程崩溃 | 恢复 interrupted/unknown，不自动重做写操作 |
| T14 | 最终持久事务失败（磁盘错误） | 不报告持久成功；storage-unavailable，拒绝继续副作用，不切回 localStorage |
| T15 | 活跃 turn 时 DeleteChatSession 重试，随后晚到 vendor 回调 | 统一停止后清理/墓碑；晚到事件不得复活历史或 handles |
| T16 | sequence/revision 大于 9007199254740991 | Go/JSON/TS roundtrip 精确，比较用整数语义而非字符串字典序 |

### 3.2 Provider、context 与计量

| 用例 | 输入/故障注入 | 必须观察到的结果 |
| --- | --- | --- |
| T17 | SSE 每个字节拆 chunk，含中文/emoji、CRLF、多行 data | 重组后语义与完整帧相同；无替代字符/重复文本 |
| T18 | 两个 tool-call ID 的 JSON 参数交错到达 | 分别组装并校验后调用；不执行半截 JSON、不混 ID |
| T19 | 模型只返回工具，无文本；或 finish/incomplete/error | 正确进入工具循环或明确错误；无空响应假成功 |
| T20 | Anthropic thinking/signature/tool 与 Google signed Part 跨轮 | signature 原值/Part 绑定不变；可构造下一轮请求；UI 不接收私有 continuation |
| T21 | 从 provider/model/API family A 切到 B 但带旧 continuation | 不把 A 私有字段发送 B；stale/新安全历史路径明确 |
| T22 | 每 step usage + 最终 turn total + 重复通知 | 最终统计按协议只计一次，cache/reasoning 不错算；unknown 保留未知 |
| T23 | 429 带 Retry-After，SDK 也配置 retry | 实际重试次数只符合单 owner 策略，无乘法重试；Stop 中止等待 |
| T24 | 收到输出/执行工具后连接断裂 | 不透明地拼接重试、不重跑写；partial/interrupted 可恢复显示 |
| T25 | 413 出现在首步与有 tool progress 后，摘要再次失败 | 按各分支有界补救；不无限重试；工具/结果对与已完成副作用保留 |
| T26 | 预算恰好低于/等于/高于 threshold，窗口缺省/非法 | 与旧 helper golden 一致；非法配置校验；输出 reserve/summary 不意外为零 |
| T27 | prepareStep 高 token，spy 统计 summarize 调用 | 只做 typed prune，零次 LLM summarize；pre-turn/413 才走对应摘要 |
| T28 | 超时配置 0/undefined/NaN/极大值，审批持续刷新 idle | 默认/无该层 deadline 正确；hard deadline 不无限延后；内层不超过剩余预算 |

### 3.3 工具、数据、权限和协议

| 用例 | 输入/故障注入 | 必须观察到的结果 |
| --- | --- | --- |
| T29 | 遍历 catalog × surface × AgentKind × mode × scope | 暴露集合/策略与基线或 accepted 差异一致；未知项拒绝、harness 不泄漏 |
| T30 | 批准 path A 后 renderer 将参数换 B；撤销 grant 后重放 | 原参数 hash 绑定生效，篡改/旧 grant 拒绝 |
| T31 | 同 terminal 来自两 chat 的 exec；另一个 terminal 同时 exec | 同 terminal 按规定串行、不同 terminal 有界并发；control 不被普通队列阻塞 |
| T32 | 远程命令超时/断线，客户端被 kill，但无远端退出确认 | outcome-unknown，不返回 exitCode 0，不自动再次执行 |
| T33 | 获取其他 chat/已关闭 terminal 的 handle，TTL 边界读 | scope/失效规则生效；TTL/LRU 使用正确时钟与访问语义 |
| T34 | 中文/emoji 输出在 UTF-16 offset 边界分页与 search | 与旧结果匹配或明确版本化适配；无静默 byte/rune 偏移 |
| T35 | 单 handle/chat/global 超限，spill 写失败，删除清理失败 | 有界内存、明确截断/错误；清理可重试，失败不使文件暴露到通用临时目录 |
| T36 | AI migration 在 export/staging/reseal/promotion 前后逐点崩溃 | 要么原完整状态，要么新完整状态；receipt 可恢复；不半迁移 |
| T37 | Electron 与 Go 两类相同 enc:v1 前缀、origin 缺失、错误密钥 | 正确选 broker/purpose；未知 origin/解密失败阻断，不保存空字符串 |
| T38 | 通过 raw Profile/GetRaw 和通用 Credential Open 请求 AI secret | 新 canonical AI secret 无解密绕读；Provider request-local 仍能使用 |
| T39 | 旧 sessions 含 images、attachments、pending approval、运行态、失效 vendor identity | 历史可读且引用有效；运行态 interrupted，审批不自动批准，盲 resume 被拒绝 |
| T40 | 云同步往返含 device-local debug、runtime token、vendor session grant | 不外泄这些本机/临时数据；既有 canonical AI 数据按已确认规则保持一致 |
| T41 | 旧 MCP 客户端 initialize，新客户端逐请求版本与未知版本 | 正确兼容或 typed version error；server/discover/fallback 可观察；不混协议状态机 |
| T42 | first-party/external token 互换、撤销后旧连接继续调用 | principal 隔离，撤销后不执行，伪造 session 参数不提升 scope |
| T43 | JSONL 超长、无换行、重复 response ID、非法 reverse method、stderr 洪水 | 有界解析/日志；受影响请求/进程被收敛；不会误报成功或耗尽内存 |
| T44 | Codex retryable error -> 文本 -> turn/completed，再重复 final | warning 后继续，最终一次结束；item completed 不提前结束 turn |
| T45 | ACP permission pending 时 cancel；无 fs capability 却收到 fs request | 取消响应正确、未 advertise 的能力拒绝；不自动放行 |
| T46 | vendor 登录账户、binary、roots、policy 变化后 resume | 按 identity/compatibility rule 拒绝或显式新建；不会恢复到另一权限环境 |
| T47 | vendor 内置工具绕开 MCP，或启动 Node child | 检测/约束证据失败，阻断 verified；不能仅修 UI 把错误隐藏 |
| T48 | DNS 地址变化、redirect 到另一 origin、远端 DNS 代理、本地用户端点 | 实际连接符合声明边界，header 不串域；允许的本地端点仍可用 |

每个 retained vendor 重跑适用的生命周期/权限/协议用例，不能只用 Codex 结果代表全部。schema parser fuzz 与真实子进程 smoke 分开：fake 可制造错误，真实 binary 证明集成入口和运行时来源。

## 4. Fixture、对照方法与性能证据

### 4.1 可复现 fixture 目录

建议在正式开工后增加 `testdata/ai/`，不要覆盖冻结 Electron fixture：

```text
testdata/ai/
  manifest.json                  source hashes, protocol/SDK versions, redaction
  provider/                      Chat, Responses, Messages, Google stream chunks
  turns/                         canonical event traces and expected snapshots
  context/                       budget, compaction, signed continuation
  migration/                     old storage exports and expected projections
  tools/                         policy cases, UTF-16 outputs, job receipts
  adapters/                      per-vendor handshake/cancel/interaction frames
```

每个 fixture 包含 `caseId`、source/version/hash、输入与分片策略、故障点、expected state/events/side effects、是否需要平台/网络/secret。只提交合成或脱敏数据；复制旧 tests 的语义和观察点，不把当前实现输出自动保存成预期来“自证正确”。

对照规则：随机 ID 和 wall-clock 可归一，stream chunk 边界允许不同；工具名/参数、scope/policy 决定、消息与 tool/result 顺序、usage 语义、terminal reason、数据引用必须一致。已接受差异单独有 allowlist/decision；禁止“忽略所有错误/usage”使 golden 通过。LLM 正文的随机性不做硬 equality，用 fake Provider 验证程序确定性，真实模型仅做协议/行为 smoke。

### 4.2 初始压测负载与判定方式

以下为工程测试负载，非已批准产品 SLA：1 个 turn 长流、8 个独立 chat 并行、100 次连续 start/stop、10 MiB 工具输出、1000 条历史消息、慢 UI consumer 和中途 reload。先记录旧 Electron与新 Go 在同机器/fixture 的 RSS、Go heap、goroutine/process/handle 数、首个本地事件延迟、Stop 收敛、队列长度、Profile 写次数、UI render 次数。

通过条件首先是 invariant：内存/队列不超过锁定预算；control/terminal 事件不被数据洪水饿死；循环后资源回到合理稳定范围；共享会话不受误杀。绝对耗时阈值由平台基线测出后填入 evidence，不能在无测试时承诺“提升 50%”。model/network 延迟与 Netcatty 本地调度分开测量。

## 5. 执行命令与结果保存

### 5.1 当前文档变更的检查

```powershell
npm run check:migration-docs
git diff --check
```

本轮只需要文档检查；不为文档改动启动付费模型、升级 SDK、生成/覆盖 Wails frontend dist 或运行完整生产构建。

### 5.2 未来 Go/TS 工作包检查

下列例子按已经出现且受影响的包执行，不存在的包不视为 PASS；命令逐条运行并检查 exit code。Go race 在具备对应平台工具链时执行；不能以关闭 race 消除失败。

```powershell
go test -count=1 ./internal/app/... ./internal/capability/...
go test -count=1 ./internal/agent/... ./internal/rpc/...
go test -race -count=1 ./internal/agent/... ./internal/capability/... ./internal/rpc/...
go vet ./internal/agent/... ./internal/capability/... ./internal/rpc/...
go test -count=1 ./cmd/netcatty/...
npm run check:contracts
npm run check:migration-electron-baseline
npm run check:migration-docs
```

涉及数据/密钥分别加现有 `npm run check:profile-store`、`npm run check:credentials`、`npm run check:data-inventory`；涉及插件边界跑相应 plugin contract/runtime tests；涉及 Codex schema 跑现有 generate/check 命令并检查生成 diff。未来 Go authority 接管 schema 生成后，明确迁移其生成入口，不继续依赖 production Node adapter。

受影响 TS/React tests 用项目现有 Node test runner。例如下列现存测试可作为局部回归起点，执行时补齐本包实际影响测试；它们本身不证明 Go parity：

```powershell
node --test --import tsx infrastructure/ai/providerContinuation.test.ts infrastructure/ai/harness/contextBudget.test.ts infrastructure/ai/harness/streamTimeouts.test.ts
node --test --import tsx application/state/aiStateSnapshots.test.ts application/state/useAIChatStreaming.architecture.test.ts
npm run lint
```

在 Go parser 定义 fuzz target 后，单 package 跑显式 target，例如 `go test ./internal/rpc -run '^$' -fuzz '^FuzzFrameDecode$' -fuzztime=30s`；这只是未来命名示例，需由 W06 真实创建并输出 seed corpus/结果。发现 failure seed 固化成 regression，再继续下个切片。

bindings 用 W02 锁定的 generator 生成，不复制本手册中的过时版本号。Windows 可用当前 `npm run wails:build`；macOS/Linux 使用对应已验收脚本。纯 Go unit PASS 不替代 WebView/Wails/认证/进程树/签名安装包的实平台测试。

### 5.3 Evidence 格式

每包把原始输出保存到现有 migration evidence 约定的位置；若新增目录先在对应 ledger 明确引用，不写空的“PASS”。建议文件集：summary.md、environment.json、commands.txt、stdout/stderr、semantic-diff.json、fixture-manifest.json、platform-smoke.md、artifact-hashes.json。脱敏后记录：

| 字段 | 内容 |
| --- | --- |
| 输入基线 | commit/相关 dirty file hash、门禁 ledger ID、matrix row/child、accepted decision |
| 变更 | 实际文件、唯一 Go owner、前端 adapter、旧调用链、迁移/回滚影响 |
| 验证 | 命令、时间、平台/arch/compiler、exit code、原始输出位置、适用用例编号 |
| 差异 | 预期差异及 decision、未通过、未执行、环境阻断分别列出 |
| 结论 | 当前证据支持的状态、不能证明的部分、下一包与 retirement trigger |

只有满足治理文件证据等级与状态图，才更新 matrix/ledger。`templates/slice-record.md` 需要的 scope change、retirement、Gate 等字段不能用本手册的工作包摘要替代。未通过项保留原始证据，不改测试去“追认”当前输出。

## 6. 可直接交给实施 AI 的提示词

以下模板中的占位符由安排实施的人/AI 根据实时仓库填写。不要把多个包一次性要求为“全量重写”。

```text
你在 Netcatty 仓库执行 Go/Wails v3 AI 迁移，只实施工作包 <W编号及名称>。

先读取 AGENTS.md 和 docs/migrations/wails-v3/ 中的：
ai-migration-execution-plan.md、ai-migration-technical-design.md、
ai-migration-work-packages.md、capability-matrix.md、migration-ledger.md、
implementation-plan.md、verification-gates.md、decisions.md。

本包正式归属：<现有 P7 task>；capability：<真实 row/已建立 child row>。
前置包证据：<路径>；有效 NONAI-COMPLETE：<真实 ledger ID>。
旧 owner/fixtures：<本包路径>；目标 Go owner/前端 seam：<本包路径>。

先核验 gate 和 accepted decisions 仍有效，并检查当前 dirty files。
gate 不满足时仅完善允许的文档/基线，明确 blocker，不创建 AI production path。
gate 满足时完成本包实际实现、受影响回归、对应 T 用例与原始 evidence。
以技术设计契约为起点；API 以锁定版本源码为准，官方新方案先做兼容验证。
不得修改无关用户改动，不得复制业务 owner、绕过 policy、伪造成功或丢 required 功能。
任何 unknown 写操作结果不得自动重试；Go 是 turn/审批/AI 历史唯一 owner。
不跨包升级工具链、不额外创建多 Agent 产品/外部公共 API、不提前删除冻结 Electron。

结束时报告：实际文件、observable behavior、命令和结果、证据位置、
未完成/未执行项、旧路径退出条件、合法的 matrix/ledger 更新与下一安全工作包。
测试或范围不足时如实保留阻塞，不把 implemented 当 verified。
```

完成检查只问可验证事实：从旧功能能否追到新 owner；一次成功/拒绝/取消能否定位到真实证据；数据是否可迁入并回滚；required 平台和 vendor 是否都覆盖。任何一项只能回答“理论上可以”时，该项仍未完成。
