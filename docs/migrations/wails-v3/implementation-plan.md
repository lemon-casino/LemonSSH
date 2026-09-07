# Wails v3 与 Go 全面迁移实施计划

状态：已批准，可供后续 AI 和工程人员执行

## 1. 计划头

### Goal

在保持 React/TypeScript 前端、用户数据、核心功能、安全边界、终端性能和
Windows/macOS/Linux 支持的前提下，将 Netcatty 的 Electron 与运行时
Node.js 全面迁移到 Wails v3 和 Go，并在验证完成后删除 Electron、运行时
Node、旧插件运行时及兼容专用路径。

### Architecture

- React、xterm.js、Monaco 和纯 TypeScript domain 资产继续作为前端。
- shell-neutral frontend ports 隔离 Electron/Wails 绑定。
- Go 是持久化、终端资源、权限、Agent、插件、进程与 OS 能力的最终 owner。
- 普通请求使用 Wails typed bindings；终端字节使用独立二进制数据面。
- 迁移期采用受控双壳，但任何 profile 同一时刻只有一个 writer。
- 每项能力通过门槛后，Go 成为 target owner，Wails 与旧逻辑断开，Electron
  release carrier 冻结；仓库删除发生在该能力记录的 cutover/rollback trigger。

### Tech Stack

- Wails v3
- Go 当前受 Wails v3 支持的稳定版本
- React 19、TypeScript、Vite
- xterm.js、Monaco
- Go 标准库、`golang.org/x/crypto/ssh`
- 经专项验证选定的 Go PTY、SFTP、SQLite/事务存储和平台 keyring 库
- `wazero` 作为普通插件 WASM runtime
- Go 原生 MCP、CLI、HTTP/SSE、JSONL/stdio 协议适配器

### Baseline / Authority Refs

执行任何任务前必须读取：

- `docs/migrations/wails-v3/README.md`
- `docs/migrations/wails-v3/architecture.md`
- `docs/migrations/wails-v3/capability-matrix.md`
- `docs/migrations/wails-v3/verification-gates.md`
- `AGENTS.md`

按任务额外读取：

- 终端：`electron/terminalWorker/`、`electron/bridges/terminal*.cjs`
- 数据：`infrastructure/config/storageKeys.ts`、`useVaultState.ts`、
  `useSessionState.ts`、`docs/session-restore.md`
- 插件：`docs/plugin-platform/`、`electron/plugins/`
- AI：`infrastructure/ai/harness/`、`electron/capabilities/`、
  `electron/bridges/aiBridge/`
- 同步：`docs/designs/convergent-sync-protocol-v2.md`、
  `domain/convergentSync/`、`infrastructure/services/cloudSync/`

### Compatibility Boundary

- 用户数据必须原位无损迁移，迁移前强制备份，失败不能破坏 Electron 源数据。
- Electron 与 Wails 不得并发写同一 profile。
- 不承诺旧 `0.1.0-internal` JavaScript/Node 插件运行兼容。
- 最终发布运行时不包含 Node；Node 仅可用于前端构建。
- 三个平台全部达到切换门槛后才替换默认发行物。
- 未经产品决策不得静默删除当前功能或缩小兼容范围。

### TDD Route

- Mode: off
- Decision: skipped
- Strict authority: not applicable
- Test posture: 每个切片完成最小实现后运行针对性回归、契约、属性、模糊、
  benchmark 或真实平台验证；不强制 RED/GREEN 仪式。
- Reason: 用户未要求严格 TDD，Aegis TDD mode 为 off。
- Verification: 以本计划任务和 `verification-gates.md` 的证据为准。

### Verification

每个任务必须同时定义并执行：

1. 聚焦测试或 benchmark；
2. 相关回归测试；
3. 适用平台证据；
4. capability matrix 状态更新；
5. migration ledger 回写；
6. Electron owner 删除或单一退役触发器。

仓库级常用命令在迁移期间逐步演变，但至少包括：

```bash
npm run lint
npm test
npm run build
go test ./...
go test -race ./...
wails3 build
```

执行者必须按任务选取聚焦子集，不得用一次全量测试代替专项证据。

## 2. 计划判断

### Requirement Ready Check

- 需求来源：用户批准的 Wails v3/Go 全面迁移目标与书面设计。
- 目标与范围：见本计划 Goal 和 `architecture.md`。
- 使用场景：现有三平台 Netcatty 用户与后续插件/AI 集成开发者。
- 验收来源：`verification-gates.md` 与 capability matrix 每行证据要求。
- 开放阻断问题：无计划级阻断；技术不确定性由 Phase 0 探针负责证伪。
- Decision: ready

Linux 无安全 keyring、外部 Agent 无非 Node 协议和 Wails 平台能力缺口均是已知
条件性阻断，而不是被假定已经解决的问题。Phase 0 必须把它们转化为支持边界、
实现路线或新的批准决策。

### Change Necessity

- 用户可见需要：最终产品切换到 Wails v3/Go 且无运行时 Electron/Node。
- 无代码方案：文档或配置不能替换 PTY、SSH、IPC、凭据、Agent 与插件 runtime。
- 必要性：必须创建 Go/Wails owner，并逐项替换现有 Electron owner。
- 最小边界：先建立可丢弃探针和 shell-neutral contract，再按 capability row
  建立单一 Go owner。
- Decision: code-change

### Existence Check

- 新增 surface：Wails shell、Go service owners、terminal data plane、profile
  store、WASM plugin runtime 和迁移台账。
- 可复用 surface：React/domain、现有行为测试、能力目录语义、插件权限/RPC
  语义、`netcattyBridge` 迁移接缝。
- 现有 surface 不足：Electron/Node owner 无法满足最终 runtime 边界，浏览器
  localStorage 无法承担跨壳无损 profile。
- 创建证据：每个新 owner 对应一个无法由现有 shell-neutral owner 承担的
 资源或政策边界。
- 熵控制：每个 Go owner 都绑定旧 owner 退役任务；禁止永久双实现。
- Decision: add-with-proof

### Architecture Integrity Lens

- Invariant：每项 mutable resource、policy 和 lifecycle decision 只有一个 owner。
- Canonical owner：目标 Go application service；Wails/MCP/CLI/UI 仅为 adapter。
- Responsibility overlap：迁移期 Electron 只作为行为基准和受控出口，不能发展
  新业务逻辑。
- Higher-level simplification：先建立 capability/profile/runtime contract，避免逐个
  Electron IPC 方法机械翻译。
- Retirement / falsifier：Go 路径通过门槛即成为 target owner，Wails 断开 legacy
  path，Electron 仅作为冻结 release carrier；在记录的 cutover/rollback trigger
  删除。若终端数据面、凭据迁移或三平台 Wails 能力无法满足门槛，暂停扩张并
  返回设计。
- Verdict: proceed

### Plan Pressure Test

- Owner / contract / retirement：所有任务绑定 capability ID 与旧 owner 退役。
- Architecture integrity：先探针和 contract，后业务重写。
- Verification scope：包含 unit、race、fuzz、benchmark、真实服务与打包平台。
- Task executability：每个切片有前置依赖、文件边界、命令和出口。
- Pressure result: proceed

### Plan-Time Complexity Check

- 当前高压力文件：`AppSideEffects.tsx`、`useVaultState.ts`、
  `useSessionState.ts`、`electron/main.cjs`、`electron/preload/api.cjs`。
- 风险：直接在这些文件加入 Wails 分支会形成双 owner 和更多条件路径。
- 更好边界：新增 shell-neutral ports 与 Go owner；旧文件只增加最薄迁移适配，
 随后删除。
- Recommendation: add owner files + split task；禁止在 mega files 内完成
  Wails 全部逻辑。

## 3. 全局执行规则

### 3.1 TaskStartSnapshot

每个任务首次写入前记录：

```bash
git status --short
git diff -- docs/migrations/wails-v3/README.md docs/migrations/wails-v3/implementation-plan.md
git log --oneline -5
```

第二条命令只是当前文档任务的实例。执行后续任务时，必须把路径替换为该任务
`Files` 列表中的精确路径，不得使用无路径限制的 broad diff 作为快照。

当前仓库可能有大量用户改动。执行者只能修改任务声明的路径，不得回滚或格式化
无关文件，不得使用 broad staging。

### 3.2 每项功能的文档回写

任务实现后必须：

1. 更新 `capability-matrix.md` 对应状态；
2. 在 `migration-ledger.md` 追加 slice record；
3. 可更新本计划任务状态和下一 safe slice，作为执行导航；
4. 如果当前 Electron owner 文档描述已过时，更新对应 owner doc；
5. 架构只有在批准决策变化时更新，不能为实现偏移改写基线。

缺少回写时最高状态为 `implemented`，不得标为 `verified` 或 `migrated`。
`capability-matrix.md` 与 append-only ledger 是状态和 resume 的唯一 authority；本计划
的执行状态与 next 文本只是导航提示，不能覆盖 matrix/ledger。P0-01B checker 强制
plan task IDs、依赖和跨文档引用，但不把计划 prose 状态解释为 capability 状态。

### 3.3 提交边界

- 每个编号任务是一个可独立 review/revert 的 coherent slice。
- 完成专项验证和文档回写后再创建 scoped commit。
- 用户未明确要求时不自动 push、开 PR 或合并。

### 3.4 暂停和回退

出现以下情况立即暂停，不得用 fallback 掩盖：

- terminal hot path 无法满足 Electron 基线且无批准的新阈值；
- 任一 meaningful secret 无法无损解封/转封；
- Wails v3 缺失三平台不可替代的窗口/生命周期能力；
- 外部 Agent 只有 Node SDK 且产品不接受退役；
- 新实现要求 Electron 与 Wails 双写；
- 旧 owner 无法给出具体退役触发器；
- 计划外 schema、public API 或安全边界变化。

### 3.5 Phase-entry decomposition gate

本计划中的后期任务是 durable roadmap slices，不授权一次性修改整个 subsystem。
进入 Phase 3-9 的任何复合任务前，执行者必须：

1. 在 capability matrix 中为可独立替换的功能增加 stable child rows；
2. 将该任务拆成精确 `Files`、依赖、命令、平台和退役触发器的执行卡；
3. 更新本计划，将 child tasks 登记在父任务下；
4. 运行文档一致性检查，确认 matrix row、plan task、ledger ID 可互相引用；
5. 若拆分引入新 owner/schema/security boundary，先获得 `decisions.md` decision。

没有通过 decomposition gate 的 roadmap task 不得直接实施。复合行至少包括
`TERM-03`、`AI-04`、`PLUG-02`、`PLUG-03` 和每个平台发布组合。

## 4. 阶段总览和依赖

```text
Phase 0  基线与证伪探针
   |
Phase 1  Shell-neutral contract 与 Wails 骨架
   |
Phase 2  Go profile、协调、凭据与无损迁移
   |
Phase 3  终端数据面、PTY、SSH、SFTP、传输、转发
   |
Phase 4  系统能力、多窗口、托盘、深链接、App Lock
   |
Phase 5  Plugin v2：WASM、声明式 UI、原生进程
   |
Phase 6  Sync 与非 AI 打包、更新、升级引导基础设施
   |
Non-AI Completion Gate
   |
Phase 7  Capability、MCP/CLI、Catty 与外部 Agent
   |
Phase 8  最终签名 RC、Gate 1-14 与默认 Wails 切换
   |
Phase 9  回滚观察收口、Electron/Node 删除与最终证明
```

Phase 0-9 按上述 exit/entry gate 串行推进；一个 phase 内可以并行的任务必须不共享
canonical owner 或 generated artifact。Phase 7 AI production implementation 不得与
Phase 3-6 并行或提前搭建 production owner；Phase 0 只允许 P0-05 的只读审计、协议
握手和 disposable fixtures。

## 5. Phase 0：基线与证伪探针

目标：在创建大量 production Go 代码前验证三个最可能推翻方案的假设。

### P0-01 冻结 Electron 行为与性能基线

执行状态：`needs-verification`。Contract exporter、bridge/storage/plugin/runtime
synthetic fixtures、canonical terminal workload、owner index、
benchmark protocol 和 Windows 本机观测已完成；三平台、30-round、remote SSH、
Windows repaint/cleanup 和 native PTY 证据仍缺失。详见
`baselines/electron-runtime-baseline.md` 与 ledger `WV3-L001`。

关联：全部 capability rows

Files:

- Create: `docs/migrations/wails-v3/baselines/electron-runtime-baseline.md`
- Create: `scripts/migration/` 下的只读清单/fixture 生成脚本，具体文件按数据域拆分
- Modify: `docs/migrations/wails-v3/capability-matrix.md`
- Modify: `docs/migrations/wails-v3/migration-ledger.md`

Why：没有可比较基线，Go 实现只能凭印象宣称 parity。

Change Necessity：需要脚本导出能力目录、桥接契约、CLI JSON、AgentEvent、
storage key 分类候选和 benchmark 元数据；不得导出用户 secrets。

Steps:

1. 为 terminal、SSH、SFTP、transfer、window、AI、plugin、storage 建立现状 owner
   索引，记录测试路径和现有性能测试命令。
2. 创建 `scripts/migration/export-electron-contract-fixtures.mjs`，生成
   capability/tool/CLI/MCP schema golden fixtures 到
   `testdata/migration/electron/`。
3. 生成代表性 AgentEvent、plugin RPC/package validator 和 session restore
   fixture；对敏感字段使用合成数据。
4. 固定 benchmark protocol：硬件等级、OS/WebView/Electron 版本、预热轮数、
   至少 30 个测量样本、并发 session 数、payload 分布、p50/p95/p99、RSS 峰值、
   允许方差和失败判定。
5. 在 Windows、macOS、Linux 记录当前 Electron terminal throughput、input
   latency、RSS、启动、窗口和打包 smoke 基线。
6. 将基线命令、环境、样本、统计接受 envelope 和结果摘要写入 baseline doc。
7. 回写 matrix/ledger；不提升任何 Go capability 状态。

Verification:

```bash
npm run lint
npm test
npm run test:xterm-keyword-highlight-performance
npm run test:xterm-keyword-highlight-throughput
```

Expected：fixtures 可重复生成且无 secrets；三平台基线具有可复现实验说明。

Exit：所有后续 parity task 都能引用一个稳定 fixture/metric，而不是手写期望。

### P0-01A 冻结三平台 release target matrix

执行状态：完成。required floor 已于 2026-09-08 由产品决策冻结：Windows 10 22H2
x64 + WebView2 Evergreen（`WV3-011`）、macOS 12+ x64/arm64（`WV3-012`）、
Linux GTK 4.14+/WebKitGTK（`WV3-013`，退休 RHEL 8 glibc 2.28 兼容目标）；
Linux 四种包格式保持 required（继承 `WV3-008`）；Windows ARM64 确认
unsupported。见 `release-target-matrix.md` 与 ledger `WV3-L012`。

关联：REL-01、REL-02、REL-03、FND-01、FND-04

Files:

- Create: `docs/migrations/wails-v3/release-target-matrix.md`
- Modify: `docs/migrations/wails-v3/verification-gates.md`
- Modify: matrix/ledger

Why：没有封闭的 OS、architecture、WebView、Linux distro/desktop 和 package
组合，就无法判断三平台同时切换或 evidence grade A。

Steps:

1. 从当前 electron-builder、CI 和 README 提取实际发布 targets。
2. 明确 Windows versions/architectures/WebView2、installed/portable/ZIP 组合。
3. 明确 macOS versions、x64/arm64、WKWebView、DMG/ZIP 组合。
4. 明确 Linux minimum distros、architectures、WebKitGTK、X11/Wayland desktop、
   Secret Service 和 AppImage/deb/rpm/pacman 组合。
5. 为每个组合标记 required、best-effort 或 unsupported；required 集合必须得到
   `decisions.md` 引用。
6. 定义增加、删除或降级 target 的批准流程。

Verification：对照 CI matrix、packaging config 和 clean-machine test inventory；无
`supported architectures` 等循环表述。

Exit：Gate 13 和 grade A 具有可枚举目标集合。

### P0-01B 建立迁移文档一致性检查

执行状态：完成。检查器 `scripts/migration/check-wails-migration-docs.mjs` 已并入
`npm run check:migration-docs` 和 CI；adversarial mutation suite 通过。见 ledger
`WV3-L003`。

关联：全部 capability rows

Files:

- Create: `scripts/migration/check-wails-migration-docs.mjs`
- Modify: package scripts and CI documentation check
- Test: `scripts/migration/check-wails-migration-docs.test.mjs`

Why：每项功能替换后的文档回写不能只靠人工记忆。

Steps:

1. 解析 capability matrix 的 stable row IDs、scope、status 和 evidence grade policy。
2. 验证每个非 `not-started` row 至少引用一个 plan task 和 ledger entry。
3. 验证每个 ledger entry 的 capability、plan task、status transition、Gate、decision
   IDs/categories、Scope change、Closure evidence 和 Electron retirement 字段完整，
   并按 chronology 执行 AI-last epochs、scope replay 和 REL lifecycle guards。
4. 验证复合 capability 首次实现前已拆 stable child rows。
5. 验证 README document links、plan task IDs 和 decision references 存在。
6. 将检查加入 CI；文档不一致时失败，不自动改写文档。

Verification:

```bash
node scripts/migration/check-wails-migration-docs.mjs
node --test scripts/migration/check-wails-migration-docs.test.mjs
```

Exit：代码/状态推进缺少 matrix、ledger、plan 或 decision 回写时 CI 可阻止合并。

### P0-02 Wails v3 三平台最小壳探针

执行状态：`needs-verification`。beta.12 独立探针已在 Windows 10 x64 完成编译、
production build、主进程启动与第二实例退出 smoke；交互矩阵及 macOS/Linux 证据待补。
见 `probes/wails-shell.md` 与 ledger `WV3-L009`。

关联：FND-01、FND-04、SYS-02、SYS-03、REL-01

Files:

- Create: `experiments/wails-shell-probe/`
- Create: `docs/migrations/wails-v3/probes/wails-shell.md`
- Modify: matrix/ledger

Why：先证明系统 WebView、多窗口、关闭拦截、native handle、托盘与深链接可满足
产品边界。

Boundary：`experiments/` 是 disposable probe，不得被生产 React 代码依赖。

Steps:

1. 创建最小 Wails v3 应用，加载一个独立测试页面。
2. 验证主窗口、设置窗口、session window 和 popup 的创建/关闭/聚焦。
3. 验证 close veto、窗口角色 token、renderer reload/crash 通知和第二实例 intent。
4. 验证 WebView2、WKWebView、WebKitGTK 的 xterm/Monaco 基本加载、IME、
   clipboard、WebGL/WASM 能力。
5. 验证 tray/global shortcut/deep link/native window handle 的平台可用性；缺失能力
   记录拟采用的 platform adapter，不写永久 fallback。
6. 记录三平台结果和阻断项。

Verification:

```bash
go test ./experiments/wails-shell-probe/...
wails3 build
```

另需三平台手工/CI smoke，单平台通过只能给 C 级证据。

Exit：三平台关键 shell 能力存在可行实现路径；否则 `blocked` 并返回设计。

### P0-03 终端二进制数据面探针

执行状态：`needs-verification`。Authenticated loopback WebSocket candidate、
canonical Electron workload、binary frame/credit/urgent/drain/rebind、独立 xterm
harness 和 Windows 10 WebView2 autorun 已通过；`TERM-01` 进入 `probe`。三平台、
30-round、paired Electron envelope、真实 popup movement 和 formal RSS/latency
evidence 仍缺失。见 `probes/terminal-data-plane.md` 与 ledger `WV3-L010`。

关联：TERM-01

Files:

- Create: `experiments/terminal-data-plane/`
- Create: `docs/migrations/wails-v3/probes/terminal-data-plane.md`
- Modify: matrix/ledger

Why：普通 Wails JSON event 不能假定满足 terminal hot path。

Steps:

1. 从 Electron fixture 重放真实大小分布的 terminal chunks。
2. 实现一个 authenticated loopback WebSocket binary candidate，使用一次性 token、
   origin/host 限制、session generation、sequence、credit 和 urgent channel。
3. 在 xterm 测试页面覆盖持续输出、短行、长行、多 session、stall/reload/rebind。
4. 测量吞吐、backend-to-xterm latency、Ctrl-C latency、RSS、queue high-water、
   pause/resume。
5. 与 P0-01 Electron baseline 比较。
6. 若失败，验证 Wails-native binary bridge 或 shared-memory/ring candidate；只能
   选择一个 production 方向，失败方案删除。

Verification:

```bash
go test ./experiments/terminal-data-plane/...
go test -race ./experiments/terminal-data-plane/...
```

另运行三平台 benchmark harness。

Exit：选定一个满足 Gate 3 的数据面；否则停止全面迁移。

### P0-04 Electron secret 解封与 Go 转封探针

执行状态：`needs-verification`。Windows DPAPI live 互操作（31 secret fixtures +
3 metadata）、三种 interrupt 注入的 fail-closed 行为、泄漏扫描与 Go 契约套件已通过；
macOS/Linux live keyring 证据缺失。收尾期间修正了陈旧的 Go KDF/AAD golden 向量，
并补齐了 Electron 驱动缺失的 metadata 帧阶段。见 `probes/secret-migration.md` 与
ledger `WV3-L011`。

关联：FND-03

Files:

- Create: `experiments/profile-secret-migration/`
- Create: `docs/migrations/wails-v3/probes/secret-migration.md`
- Modify: matrix/ledger

Why：无损迁移无法依赖 Wails 直接读取 Electron `safeStorage` ciphertext。

Steps:

1. 使用合成 profile 覆盖 host/key/identity/proxy/group/AI/sync/password 等所有
   secret-bearing fields。
2. 在 Electron 中解封 `enc:v1:`，仅通过 authenticated ephemeral channel 传递。
3. 在 Go 中分别调用 Windows、macOS、Linux 目标 keyring provider 转封。
4. 验证日志、事件、临时目录、receipt 和 crash dump 不出现 plaintext。
5. 对损坏、不可解密、secure storage unavailable 和进程中断执行 fail-closed。
6. 清理 probe 产生的全部测试 secret。

Verification:

```bash
npm test -- electron/bridges/credentialBridge.test.cjs domain/credentials.test.ts
go test ./experiments/profile-secret-migration/...
```

Exit：三个平台均能解封/转封并通过泄漏扫描；否则 profile migration `blocked`。

### P0-05 外部 Agent 非 Node 协议审计

执行状态：`needs-verification`。每个 Agent 已分类并获得 Go 协议路线或 Phase 7
入口决策点；三平台 executable/process-tree/handshake 尚未验证。见
`baselines/external-agent-protocols.md` 与 ledger `WV3-L004`。

关联：AI-04、REL-03

Files:

- Create: `docs/migrations/wails-v3/baselines/external-agent-protocols.md`
- Modify: matrix/ledger

Why：最终 Node-free 要求必须在实施前知道哪些 Agent 可保留。

Steps:

1. 对 Catty、Codex、Claude、Copilot、Cursor、OpenCode、CodeBuddy、Grok 记录
   当前 driver、session identity、approval、stop/steer、model discovery 能力。
2. 为每个集成确认稳定 HTTP/CLI/stdio/ACP/app-server 协议和版本来源，并审计
   executable interpreter、bundled runtime 和 recursive child process tree。
3. 分类为 `replaceable`、`needs-upstream-protocol`、`retirement-decision`。
4. 禁止把 Node SDK wrapper、Node shebang CLI 或内嵌 Node executable 记录为
   最终 Node-free 协议。
5. 为无法替代者给出必须作出产品决定的最迟阶段（Phase 7 入口前）。

Verification：静态 owner 对照、协议最小握手 probe 和 schema fixture；不得使用真实
付费调用作为唯一证据。

Exit：每个 Agent 有明确 Go 适配路径或退役决策触发器。

### Phase 0 Exit Gate

- P0-01、P0-01A、P0-01B 均完成，baseline、target matrix 和文档一致性 CI 已建立。
- P0-02、P0-03、P0-04 均未处于 `blocked`。
- terminal data plane 只有一个被选中的 production candidate。
- 所有外部 Agent 有非 Node 路线或产品决策点。
- Electron baseline 可重放、可比较、不含 secrets。

## 6. Phase 1：Shell-neutral Contract 与 Wails 骨架

### P1-01 定义 shell-neutral RuntimeClient

执行状态：完成（代码与契约层）。`infrastructure/runtime/` 提供由
`generate:runtime-ports` 从 P0-01 契约 fixtures 生成的 9 个域端口（482 个方法，
覆盖性与不重叠由 `--check` 强制），Electron adapter 是唯一 `window.netcatty`
访问点（ESLint 强制），`netcattyBridge` 变为 transition facade 并保持精确语义；
`@wailsio/runtime` 仅允许 Wails adapter 导入。消费方迁移按域逐切片进行。
Wails adapter 在 P1-02 落地。见 ledger `WV3-L013`。

关联：FND-01

Files:

- Create: `infrastructure/runtime/` 下 capability-specific TypeScript ports
- Create: Electron adapter contract tests
- Modify: `infrastructure/services/netcattyBridge.ts`
- Modify: ESLint boundary tests/config

Why：直接让 application/domain import Wails 会把新壳耦合扩散到整个前端。

Steps:

1. 按 app/window/profile/credential/terminal/sftp/sync/agent/plugin 拆分接口。
2. 由现有 `window.netcatty` 构造 Electron RuntimeClient adapter。
3. 将 `netcattyBridge.get()` 变成 transition facade，不改变调用结果语义。
4. 新增架构规则：只有 Electron adapter 可访问 `window.netcatty`；只有 Wails
   adapter 可导入 generated bindings。
5. 迁移一组低风险 consumer 验证接口形状，不批量迁移所有 UI。

Verification:

```bash
npm run lint
npm test -- infrastructure/services application/state
```

Exit：现有 Electron 行为不变，Wails adapter 可以实现相同 contract。

### P1-02 创建正式 Go module 与 Wails 应用骨架

执行状态：完成（代码与构建）。根 `go.mod` 固定 Wails `v3.0.0-beta.12`/Go 1.25；
`cmd/netcatty` 是唯一 import Wails 的生产包（health/version/window-role 三个
use case 委托给无 Wails 依赖的 `internal/app`）；`npm run wails:build` 构建真实
Vite bundle 并嵌入骨架（`scripts/wails-prepare-frontend.mjs`）；生成的
typed bindings 驱动 fail-closed 的 `infrastructure/runtime/wails/` adapter，
bootstrap 按 shell 自动选择 runtime；Electron 仍是默认 dev/release。渲染进程
launch smoke 与 CI 三平台骨架 job 落在 migration-evidence workflow。见 ledger
`WV3-L014`。

关联：FND-01、REL-01

Files:

- Create: `go.mod`, `go.sum`
- Create: `cmd/netcatty/`
- Create: `internal/app/`, `internal/platform/`
- Create: Wails configuration/build scripts
- Modify: package scripts and CI only for additive Wails jobs

Why：建立 production owner 根目录和可测试的 Wails facade。

Steps:

1. 固定 Go/Wails 工具版本和升级策略。
2. 创建 Wails app，仅提供 health/version/window-role methods。
3. 加载现有 Vite build，不复制 React 源码。
4. Wails facade 只放在 `cmd/netcatty` 或其 outer adapter；`internal/app` 只暴露
   shell-neutral use cases，禁止 import Wails。
5. 实现 Wails RuntimeClient adapter、test host 和 runtime selection bootstrap。
6. 用 P1-01 的同一 contract suite 验证 request/error/cancel/subscription/lifecycle，
   不以“可编译”代替 adapter parity。
7. Electron 仍为默认 `npm run dev`/release；增加明确的 Wails dev/build 命令。
8. CI 在三平台构建最小 Wails artifact。

Verification:

```bash
go test ./...
go vet ./...
wails3 build
npm run build
npm test -- infrastructure/runtime
```

Exit：同一 React bundle 可在 Electron/Wails 测试壳启动，业务 owner 尚未复制。

### P1-03 建立 Go 错误、事件、身份与取消基础契约

执行状态：完成。`internal/app/contracts/` 提供 opaque IDs（inst_/win_/ses_/req_
前缀 + 24 hex）、稳定错误码与结构化 envelope（`AsError` 统一映射
deadline/cancel/policy 错误）、request/subscription envelope，以及 JSON 边界
策略（1 MiB、深度 32、±2^53-1 safe integer、unknown-field 拒绝）。
`tools/contracts-codegen` 反射生成 TS 声明（ErrorCode union 直接从 errors.go
AST 提取），`--check` 字节级 drift gate 并入 `check:contracts`；Go golden
fixtures（testdata/migration/contracts/）由 TS 侧跨语言测试消费；前端
`errorMapping.ts` 按 Go `AsError` 语义映射 BridgeUnavailableError/Abort/
Timeout。不在此任务定义 terminal byte frame 或 plugin public contract。见
ledger `WV3-L015`。

关联：FND-01、FND-04

Files:

- Create: `internal/app/contracts/`, TypeScript generated contract outputs
- Create: codegen/check scripts
- Test: Go/TS cross-language golden fixtures

Why：后续 terminal、Agent、plugin 不得各自发明错误和 lifecycle envelope。

Steps:

1. 定义 opaque instance/window/session IDs、stable error codes、request deadline、
   cancellation 和 subscription envelopes。
2. 为 JSON safe integer、size、depth 和 unknown-field policy 建立边界。
3. 从 Go 生成 TypeScript definitions，加入 byte-for-byte drift check。
4. 使用 Electron fixtures 验证错误/取消映射。
5. 不在此任务定义 terminal byte frame 或 plugin public contract。

Verification:

```bash
go test ./internal/app/contracts/...
npm test -- infrastructure/runtime
```

Exit：所有后续 service contract 复用统一基础类型。

### Phase 1 Exit Gate

- React/domain 不直接依赖 Electron/Wails globals。
- Wails 正式骨架三平台构建。
- Electron 和 Wails adapters 共享 contract tests。
- Electron 仍为默认发行物且无功能回归。

## 7. Phase 2：Profile、协调、凭据与无损迁移

### P2-01 完成持久化 key 与 Electron-main 文件清单

执行状态：完成。`scripts/migration/export-data-inventory.mjs` 从 P0-01 fixture
生成 `data-inventory.md`（178 个唯一 key：110 canonical-migrated / 57
device-local / 9 transient-cache / 2 retired，11 个显式 secret-bearing）与机器
清单 JSON；drift test 独立解析 storageKeys.ts，未分类新 key 使 CI 失败
（`check:data-inventory`）；Electron-main 文件（plugins.sqlite、vault 备份、
session/crash/agent/ssh 日志、window-state、专用 temp 目录、CLI discovery
file）与五类特殊存储已在文档中定位。sync-payload 精确组成由 P2-07 验证。见
ledger `WV3-L016`。

关联：FND-02、FND-03、SYNC-01、SYNC-02

Files:

- Create: `docs/migrations/wails-v3/data-inventory.md`
- Create: inventory drift test
- Modify: matrix/ledger

Steps:

1. 枚举 `storageKeys.ts`、sync keys、ad hoc legacy keys 和 Electron-main files。
2. 分类：canonical migrated、device-local、transient/cache、retired。
3. 标记 raw 编码、schema owner、secret fields、sync/backup/restore 关系。
4. 新增 drift test，未分类新 key 使 CI 失败。
5. 明确 plugin SQLite、vault backups、app-lock config、cloud password 和 portable
   profile 路径。

Verification：inventory generator/check，现有 storage/sync tests。

Exit：Gate 6 无未分类数据。

### P2-01A 冻结旧插件用户数据保留合同

执行状态：完成。`plugin-v1-data-retention.md` 冻结 schema v3 全部 12 张表的
处置（preserve-metadata ×2、preserve-opaque ×5、invalidate-grants ×2、
re-seal-secrets ×1、drop-runtime-state ×2）；`check:plugin-retention` 直接解析
database.cjs 的 CREATE TABLE，新表未分类即 CI 失败；v1 入口（main.browser/
main.node）拒绝执行、grants 不继承、secret 仅经 P2-05→P2-04 broker 通道、
v2 认领需用户批准——四项属性在 fixture 中断言。已并入 P2-05/P2-06 bundle 与
equality 检查要求（namespace `plugin-v1/<id>/<table>/` + semantic hash）。见
ledger `WV3-L017`。

关联：PLUG-01、SYNC-01、SYNC-02、FND-03

Files:

- Create: `docs/migrations/wails-v3/plugin-v1-data-retention.md`
- Create: plugin data inventory fixtures/check
- Modify: matrix/ledger

Why：plugin v2 runtime 在 Phase 5 才实现，但 Phase 2 profile cutover 必须先知道
旧 plugin database、grants、settings、view state、sidecars、audit、versions 和
secrets 如何无损保留。

Steps:

1. 分类 v1 package code、runtime state、user-owned state、security state 和 sync
   sidecars。
2. 明确 v1 code 永不执行，但已安装版本 inventory 可作为 disabled metadata 保留。
3. 对 settings/view state/sidecars/audit 定义 opaque preservation envelope、namespace
   和 semantic hash，不要求 Phase 2 理解 v2 schema。
4. 旧 grants 默认失效；除非 v2 canonicalization 与 security principal 有逐字节
   等价证明，不得继承 authority。
5. 旧 secrets 通过 migration broker re-seal 并保持不可被 v1 code 使用；定义未来
   v2 plugin 在用户批准后认领数据的流程。
6. 将该合同加入 P2-05/P2-06 bundle 与 equality checks。

Verification：synthetic plugin DB、missing package、corrupt row、uninstalled plugin、
sidecar round-trip 和 no-v1-execution tests。

Exit：Phase 2 可证明插件代码不兼容但用户数据未丢失。

### P2-02 实现 Go transactional profile store

关联：FND-02

Files:

- Create: `internal/profile/store/`, migrations, tests
- Create: Wails profile service adapter
- Create: TypeScript ProfileClient implementation

Steps:

1. 基于跨平台 crash consistency 证据选择存储引擎并记录版本。
2. 实现 raw value compatibility、typed records、revision/CAS、transaction、
   schema version 和 durable notification。
3. 实现 staging profile、atomic promotion、backup manifest 和 migration receipt。
4. 模拟每个 commit/promotion 边界崩溃并验证恢复。
5. 使用 synthetic profile fuzz malformed/corrupt/oversized records。

Verification:

```bash
go test ./internal/profile/store/...
go test -race ./internal/profile/store/...
```

Exit：host-owned store 满足 transaction、recovery 和 revision semantics。

### P2-03 实现跨壳 profile writer lease 与协调服务

关联：FND-02

Files:

- Create: `internal/profile/coordination/`
- Create: Electron broker adapter
- Modify: Web Lock/storage event owners through transition adapter

Steps:

1. 先记录 writer lease protocol：使用 profile 根目录之外稳定路径上的 OS-native
   advisory lock，并配合持久 epoch/fencing token；不得只依赖超时判断 owner 死亡。
2. 定义 process/profile-scoped lease、renewal、失效、takeover、promotion 后身份稳定
   和 stale writer rejection。
3. 实现可由 Electron 与 Wails 同时调用的最小 Go broker/helper，或证明两壳可直接
   使用同一 OS primitive；Wails 主窗口存活不得成为锁服务前提。
4. 实现 exclusive named locks、commit events、source instance 和 monotonic revision。
5. Electron 在任何迁移/双壳写入前必须获取同一 lock/fencing authority。
6. 将 Vault import、sync apply、key rotation 的锁调用迁到 CoordinationPort。
7. 注入 process crash、window close、clock shift、stale lease、atomic profile promotion
   和 simultaneous launch。

Verification：Go race tests + Electron/Wails dual-process contention integration tests。

Exit：任意时刻只有一个 profile writer；没有 browser-lock fallback。

### P2-04 实现平台 credential providers

关联：FND-03、SYS-04

Files:

- Create: `internal/platform/credentials/` platform files
- Create: secret envelope schema and tests

Steps:

1. 实现 Windows、macOS、Linux provider availability/status。
2. 定义 version/provider/purpose-bound sealed envelope。
3. seal/open 时验证 purpose，禁止跨字段重放。
4. unavailable/insecure backend fail closed。
5. 添加日志 redaction 和 memory-lifetime review。

Verification：三平台 integration tests；Linux 无 keyring 的 negative test。

Exit：P0-04 probe 逻辑被 production owner 替换，probe 删除或仅保留 harness。

### P2-05 实现 Electron migration export broker

关联：FND-03

前置：P2-01、P2-01A、P2-02、P2-03、P2-04。

Files:

- Create: Electron-side migration broker and preload-restricted API
- Create: migration bundle schema/fixtures
- Modify: Electron shutdown/sync mutation gate

Steps:

1. 仅允许 trusted Netcatty origin、显式迁移状态和 writer lease 调用。
2. drain Vault/settings/sync pending writes。
3. 创建 encrypted protective backup 和 source fingerprint。
4. 导出 classified raw records 与 Electron-main files。
5. 在 Electron 内存解封所有 known secrets；扫描残余 placeholders。
6. 使用 Wails ephemeral public key 加密一次性 bundle；不写 plaintext temp file。
7. 在成功 receipt 前保持 Electron profile 不变且 writable ownership 未转移。

Verification：synthetic all-fields profile、tamper、cancel、unreadable secret、renderer
attack、crash tests。

Exit：能生成完整、验证过、无 plaintext 落盘的迁移 bundle。

### P2-06 实现 Wails import、验证、切换与反向回滚

关联：FND-03、SYNC-01、SYNC-02

前置：P2-01A、P2-05。

Files:

- Create: `internal/profile/migration/`
- Create: Wails migration UI state adapter
- Create: crash matrix integration harness

Steps:

1. 验证 bundle version、source fingerprint、signature/authentication。
2. 导入 fresh staging store 并使用 platform provider re-seal。
3. 执行 schema、count、ID、relation、raw encoding、CRDT 和 semantic hash 对比。
4. 原子 promote，写 cutover receipt，转移 writer lease。
5. Electron 检测 receipt 后进入 read-only/export-only mode。
6. 实现 Wails 变更后的 reverse export/verify/lease transfer rollback。
7. 在每个步骤注入 crash，验证 Gate 8 四种允许状态。

Verification：三平台 crash matrix、secret leak scan、source/target semantic equality。

Exit：无损 cutover 与真实 rollback 均可重复通过。

### P2-07 将 settings/Vault/session restore persistence 接到 Go owner

关联：SYNC-01

Files:

- Modify: `localStorageAdapter` transition implementation
- Modify: `useSettingsState.ts`, `useVaultState.ts`, `useSessionState.ts` 的 persistence
  boundary，不迁移 React in-memory view logic
- Create: Go profile service DTO/handlers and differential tests

Steps:

1. 先将 adapter 改为 host-backed cache，保持 raw read/write API 兼容。
2. 逐域迁移 settings、Vault、notes/logs/history、session restore。
3. 使用 host revision 替代 process-local stale write 防护。
4. 保留 restore sanitizer、peer-window non-owner 和 active-tab patch invariants。
5. 每迁移一域即删除该域 browser storage event/Web Lock owner。
6. 最后禁止 production Wails 直接使用 localStorage 作为 canonical data。

Verification：Electron/Wails differential suites、multi-window concurrency、quota/error
mapping、session restore regression。

Exit：Go store 是 canonical owner；Renderer localStorage 只剩明确的 UI cache/transient。

### Phase 2 Exit Gate

- Profile、writer lease、credential providers 与 migration crash matrix 三平台通过。
- Vault/settings/session restore 由 Go 持久化 owner 管理。
- Electron/Wails 不存在 steady-state dual write。
- meaningful secret 无丢失，CRDT lineage 未改变。
- v1 plugin code 不执行，user-owned plugin data 按 P2-01A 合同保留。

## 8. Phase 3：终端、SSH、SFTP、传输与转发

### P3-01 产品化 terminal data plane

关联：TERM-01

Files:

- Promote selected P0-03 implementation into `internal/terminal/dataplane/`
- Create: TS terminal transport adapter
- Modify: Wails terminal client only

Steps:

1. 固化 version/session/generation/sequence/flags/length frame contract。
2. 实现 one-use authentication、route bind/rebind、credit 和 urgent path。
3. 实现 bounded pre-route backlog、drain、close tombstone 和 metrics。
4. 将 xterm renderer 接到新 adapter，不改 terminal UI behavior。
5. 删除失败 probe implementation，禁止双 data-plane fallback。

Verification：Gate 3 全 workload、Go race/fuzz、三 WebView benchmark。

Exit：TERM-01 `verified`；Electron MessagePort 暂保留仅供 Electron 壳。

### P3-02 实现 Go local PTY runtime

关联：TERM-02

Files:

- Create: `internal/terminal/pty/`, `internal/terminal/session/`
- Create: platform PTY adapters and process supervision tests
- Modify: Wails terminal control handlers

Steps:

1. 定义 session generation、start/resize/input/signal/close lifecycle。
2. 实现 Unix PTY 和 Windows ConPTY adapters。
3. 实现 child process tree/group cleanup、shell args/env/cwd policy。
4. 接入 terminal data plane 和 session restore descriptors。
5. 覆盖 duplicate start、close-during-start、late exit、renderer reload。

Verification：shell/platform matrix、race tests、live Ctrl-C/resize/Unicode tests。

Exit：Wails 本地终端完整可用；`node-pty` 仍只属于 Electron 壳。

### P3-03 实现 Go SSH authentication 与 dial core

关联：SSH-01

Files:

- Create: `internal/terminal/ssh/`
- Port synthetic and live compatibility fixtures
- Create: auth challenge frontend mapping

Steps:

1. 实现 host-key policy、known-host persistence 和每 hop 验证。
2. 实现 password、key、passphrase、keyboard-interactive/MFA partial success。
3. 实现 agent/certificate/agent forwarding/jump/proxy/keepalive/timeouts 的 dial
   primitives，但不创建业务私有连接池。
4. 对 ssh2 patches 和 legacy algorithms 逐项建立 compatibility decision。
5. 输出供 P3-04 使用的 authenticated transport contract 和 cleanup semantics。

Verification：Gate 4 compatibility lab、live MFA、proxy/jump、host-key negative cases。

Exit：认证/dial core 可被共享 pool 调用；尚不宣称完整 SSH session `verified`。

### P3-04 实现共享 SSH transport pool

关联：SSH-02

Files:

- Create: `internal/terminal/sshpool/`

Steps:

1. 定义 immutable endpoint/auth/forwarding compatibility key。
2. 实现 single-flight dial、typed shell/SFTP/transfer/forward leases。
3. 实现 healthy return、discard、idle TTL/LRU 和 shutdown。
4. 验证 asymmetric agent-forwarding reuse policy。
5. 禁止各业务域私建 SSH pool。

Verification：property/concurrency/race tests、network trace one-auth evidence。

Exit：后续 SFTP/transfer/forwarding 全部复用该 owner。

### P3-04A 将 SSH session 集成到共享 transport pool

关联：SSH-01、SSH-02、TERM-01

Files:

- Modify: `internal/terminal/ssh/`, `internal/terminal/sshpool/`
- Modify: Wails terminal handlers
- Test: SSH session/pool integration suites

Steps:

1. 让 shell session 只通过 typed pool lease 获得 authenticated transport。
2. 接入 PTY channel、data plane、resize/input/signal/close 和 cancellation。
3. 验证 renderer/window teardown 不归还 unhealthy transport。
4. 验证 agent-forwarding compatibility 和 jump-chain cleanup。
5. 删除 P3-03 中任何临时 direct-dial session path。

Verification：Gate 4 complete session matrix、pool concurrency/race、network one-auth
evidence。

Exit：SSH-01/SSH-02 才可进入 `verified`。

### P3-05 实现 SFTP browsing

关联：SFTP-01

Files:

- Create: `internal/terminal/sftp/`
- Modify: Wails SFTP adapter

Steps:

1. 选择并验证 Go SFTP library 的 raw path、encoding、channel 和 cancel 能力。
2. 实现 session/dedicated clients、owner cleanup、bounded channel open。
3. 实现 list/stat/read/write/rename/delete/symlink semantics。
4. 实现 sudo SFTP supported path；不能满足时暂停并作产品决策。
5. 覆盖 non-UTF-8 filenames、stale channel、jump host 和 host key。

Verification：real OpenSSH/sudo/SCP fixtures 和现有 SFTP regression mapping。

Exit：Wails SFTP browsing 可替代当前 Electron owner。

### P3-06 实现 Go transfer scheduler

关联：SFTP-02

Files:

- Create: `internal/terminal/transfer/`
- Modify: transfer UI adapter/store only for backend contract

Steps:

1. 定义 task identity、source fingerprint、checkpoint、epoch 和 conflict state。
2. 实现 per-host/session concurrency、dedicated channels、range read/write。
3. 实现 pause/resume/cancel/retry、directory traversal 和 atomic remote upload。
4. 实现 compressed upload/extract 与 post-upload verification。
5. 聚合 progress，避免高频 Wails event flood。

Verification：high/low RTT、loss、large/small/folder、mutation、disconnect、hash tests。

Exit：全局 transfer 在 UI unmount/window close 后仍正确运行。

### P3-07 实现 Go port forwarding

关联：NET-01

Files:

- Create: `internal/terminal/forward/`
- Modify: Wails forwarding adapter

Steps:

1. 实现 local、remote、dynamic SOCKS forwarding。
2. 复用 SSH pool typed lease。
3. 实现 process epoch、monotonic revision、subscribe+snapshot。
4. 处理 half-close、transport death、listener cleanup 和 stop-all。
5. 验证 forwarding 不依赖任何 renderer window。

Verification：IPv4/IPv6、collision、concurrency、jump、transport loss integration tests。

Exit：NET-01 `verified`。

### P3-08 迁移 Telnet、Serial、Mosh、ET 与 ZMODEM/YMODEM

关联：TERM-03

Files:

- Create: transport-specific Go packages and supervised binary runner
- Modify: resource manifest/build inputs

Steps:

1. Telnet 和 serial 使用独立 protocol owner，不伪装为 SSH/PTY。
2. Mosh/ET 通过 hash/arch verified supervised binaries 启动。
3. 迁移 reconnect、encoding、auto-login、echo 和 device disconnect semantics。
4. 迁移 ZMODEM/YMODEM event 和文件安全边界。
5. 验证所有 child/device cleanup。

Verification：三平台 protocol/device/live helper matrix。

Exit：全部 required terminal protocol rows 达到 parity 并为 `verified`。若产品要删除
某协议，必须先通过独立 scope decision 将其从 required matrix 移出；本任务内的
`retired` 不能替代 Non-AI Completion Gate 的完成证据。

### Phase 3 Exit Gate

- Wails 中 local/SSH/SFTP/transfer/forwarding/其他支持协议达到 `verified`。
- Gate 3、4、5 三平台通过。
- 终端后台不依赖窗口挂载。
- 仍未删除 Electron terminal owner，直至整体 Wails cutover；但禁止继续向旧 owner
  添加新功能，除非是 release-blocking 修复并同步 Go parity assessment。

## 9. Phase 4：系统能力与多窗口

### P4-01 本地文件系统、对话框与专用临时目录

关联：SYS-01

实现 Go filesystem/temp services、Windows attributes/drives、POSIX metadata、
symlink-safe traversal、archive extraction、native dialog parenting 和 open/reveal。

Verification：traversal attack corpus、UNC/long/Unicode、broken symlink、temp root
substitution、三平台 dialog smoke。

Exit：所有 Wails file path 不直接调用 raw OS temp outside dedicated service。

### P4-02 多窗口、弹出终端和关闭生命周期

关联：FND-04

实现 Go window registry、opaque tokens、bounds、close veto、dirty editor、
close-to-tray、terminal route snapshot/rebind/restore、renderer crash cleanup。

Verification：multi-monitor/DPI、popup crash、main recreation、keyboard/zoom、navigation
blocking，三平台真实 WebView tests。

Exit：Wails 多窗口覆盖现有 hash routes 和 session ownership invariants。

### P4-03 Tray、global shortcuts、dock/menu

关联：SYS-02

实现 Go tray controller、Quake toggle、tray panel positioning、macOS dock menu、
Linux X11/Wayland documented behavior、App Lock redaction。

Verification：三平台/桌面 smoke，registration conflict、fullscreen hide、shutdown cleanup。

Exit：没有 hidden tray/window 阻止正常退出或更新。

### P4-04 Deep links、文件关联、Explorer/Finder integration

关联：SYS-03

实现 Go intent queue、single instance、ssh/telnet/jms parsing、pre-ready queue、
password redaction 和平台 installer registration。

Verification：cold/warm/multiple/malformed/disabled/path-with-space/non-ASCII installed
package tests。

Exit：所有 intent 在 app unlock/ready 后恰好投递一次。

### P4-05 App Lock、biometric 与系统凭据

关联：SYS-04

迁移 app-lock verifier/settings、idle/reopen lifecycle、Windows Hello/Touch ID、native
window ownership；必要 helper 由 Go supervisor 管理，不使用 Node。

Verification：startup/reopen/background lock、cancel/timeout/failure、helper cleanup、
verifier migration。

Exit：lock 状态由 Go owner 管理，所有窗口一致且 fail closed。

### Phase 4 Exit Gate

- 文件、窗口、tray、deep link、App Lock 在三平台达到 `verified`。
- Wails 可完成完整日常 UI/terminal workflow。

## 10. Phase 5：Plugin Platform v2

Phase 5 是非 AI migration domain。`internal/plugin/permissions` 独立拥有 plugin
runtime identity、security principals、grants、secret leases、quotas 和 broker
authorization；terminal、profile、filesystem、credential、network 等真实操作由共享
Go application services 拥有。Phase 5 不依赖延后到 Phase 7 的
`internal/capability`、Agent policy、MCP、CLI 或 Catty。

### P5-01 定义 manifest v2 和 Go-first contract codegen

关联：PLUG-01

创建 v2 schema，支持 WASM entrypoint、native variants、permissions、contributions、
declarative UI；生成 Go/TS/guest bindings。v1 `main.browser`/`main.node` 清晰拒绝，
不提供 runtime shim。

Verification：schema drift、cross-language fixtures、depth/node/byte/safe integer、v1
rejection。

Exit：v2 contract freeze 到 internal 版本，后续 runtime 共用。

### P5-02 迁移 package store、database 与 lifecycle manager

关联：PLUG-01

实现 immutable snapshot、archive attack validation、staging/publish、SQLite/schema、
serialized mutations、active version rollback、crash/quarantine、user-owned sidecars。

Verification：完整 package attack corpus 和每个 install/uninstall crash point。

Exit：Go package manager 达到现有 phase 2/3 security bar。

### P5-02A 冻结 plugin permission/broker 接口

关联：PLUG-01、PLUG-02、PLUG-03

Files:

- Create: `internal/plugin/permissions/` public internal interfaces and fixtures
- Create: broker authorization contract tests
- Modify: matrix/ledger

Why：WASM/native runtime 在暴露 host imports 前必须依赖 canonical fail-closed
authorization owner，不能创建 provisional policy，也不能等待或复用 Phase 7 Agent
capability policy。

Steps:

1. 定义 host-generated runtime identity、security principal、canonical resource 和
   grant lifetime contracts。
2. 定义 filesystem/network/secret/credential/process broker interfaces，默认 deny；
   broker 只授权并调用 shared Go application services，不复制业务操作。
3. 定义 stale runtime recheck、commit guard、deadline、quota 和 audit hooks。
4. 建立 permission x resource x lifetime fixtures，供 P5-03/P5-05/P5-06 共用。
5. 此任务只冻结接口和 fail-closed stub，不授予真实 privileged capability。

Verification：Go contract tests、unknown capability denial、stale identity、concurrent
prompt/cancel fixtures。

Exit：P5-03/P5-05 可调用统一 plugin broker contract，不拥有 policy，也不依赖
`internal/capability`。

### P5-03 实现 wazero runtime 和 host capability imports

关联：PLUG-02

实现 runtime identity、memory/time/host-call quotas、WASI default-off、cancellation、
broker-only I/O、trap/protocol cleanup、runtime state events。

前置：P5-01、P5-02A。

Verification：infinite loop、memory exhaustion、unauthorized import、stale identity、
concurrent host calls、teardown memory reclaim。

Exit：普通 plugin logic 不需要 BrowserWindow 或 Node。

### P5-04 实现声明式插件 UI 与 contributions

关联：PLUG-02

由 host schema 验证并由可信 React components 渲染 settings/form/list/card/view；
实现 commands、menus、keybindings、context keys、theme/i18n/accessibility 和 view state。

Verification：malformed schema、CSS/DOM/script injection negative tests、theme/a11y、
state namespace。

Exit：普通插件不能向主 WebView 注入 arbitrary HTML/JS/CSS。

### P5-05 实现 native child-process runtime

关联：PLUG-03

实现 signed/hash-pinned variants、package containment、minimal environment、private
dir、framed RPC/streams、job object/process group、graceful/forced stop、quarantine。

前置：P5-01、P5-02A。Native variant 必须是经批准的 native/WASM binary，不能是
Node shebang、内嵌 Node runtime 或启动 Node 的包装器；验证包含传递性 SBOM 和
递归 process tree。

Verification：wrong arch/digest/symlink、stdout flood、malformed RPC、descendants、
unreaped containment failure。

Exit：高级 plugin 不在 Go host 进程内执行。

### P5-06 迁移 permission、secret lease、brokers 和 audit

关联：PLUG-01、PLUG-02、PLUG-03

在 `internal/plugin/permissions` 实现 canonical resource grants、security principal、
once/session/application/always、network redirect policy、filesystem handle safety、
secret refs/one-use leases、quotas/audit。Broker authorization 调用共享 Go application
services；不得导入 Phase 7 `internal/capability` 或复制 Agent principal/grant 语义。

Verification：permission x resource/lifetime matrix、stale runtime race、secret non-
disclosure、network/filesystem attack tests。

Exit：plugin privileged boundary 由 Go 单一 owner 承担。

### P5-07 迁移 provider 与 terminal pipeline contributions

关联：PLUG-02、PLUG-03、TERM-01

按 command/importer/sync/connection/terminal providers 分子任务；terminal interceptor
使用专用 data plane，不经过 general JSON RPC。

Verification：bounded streams、provider exact result schema、terminal 4ms/throughput
目标对照当前 baseline、sensitive-input bypass。

Exit：当前 required plugin feature set 全部 v2 化并为 `verified`。不支持的能力必须
先通过独立 scope decision 从 required matrix 移出，不能在 Non-AI Completion Gate
内用 `retired` 代替实现完成。

### P5-08 断开 Wails v1 plugin path 并冻结 Electron plugin release carrier

关联：PLUG-01、PLUG-02、PLUG-03、REL-03.1、REL-03.2

前置：P5-01、P5-02、P5-02A、P5-03、P5-04、P5-05、P5-06、P5-07 的相关
required leaf rows 已 `verified`，且 v1 package UX 已验证。

Steps:

1. Wails 明确拒绝 v1 package，断开所有 Electron plugin runtime、preload、protocol
   和 Node package 的调用路径。
2. Electron BrowserWindow/utilityProcess/plugin preload/protocol 继续只服务当前稳定
   Electron release，冻结为 `release-carrier-only`，只接受 release-blocking 修复并附
   Go parity assessment。
3. Electron plugin runtime 不得成为 Wails fallback，也不得授权 Go plugin runtime。
4. 记录 P9-01 为 BrowserWindow/utilityProcess/plugin preload/protocol 和相关 Node
   runtime package 的唯一实际删除任务。

Verification：Wails v1 rejection、legacy path negative tests、Wails package dependency
scan、Electron release-carrier smoke。

Exit：Wails plugin v2 不执行 JavaScript/Node plugin runtime；Electron plugin runtime
仍存在但只作为冻结 release carrier，尚未删除。

### Phase 5 Exit Gate

- Gate 12 全部通过。
- ordinary plugin 只能 WASM + declarative UI。
- native process 有 OS containment 和 quarantine。
- v1 插件不执行，用户得到清晰迁移/不兼容结果。
- Wails 已与旧 plugin path 断开；Electron plugin runtime 冻结到 P9-01。
- Plugin policy 不依赖 Phase 7 Agent capability/policy owner。

## 11. Phase 6：Sync 与非 AI 发布基础设施

Phase 6 产出供集成和 qualification 使用的 packages、updater 与 upgrade bootstrap。
这些产物不包含 AI production owner，不能称为 signed RC、release candidate 或可发布
Wails 版本；Electron 仍是默认稳定发行物。

### P6-01 将 cloud/convergent sync 迁入 Go

关联：SYNC-02

实现 encryption/KDF、OAuth/providers、CRDT、anchors/baselines、read-merge-write-verify、
auto-sync、protective backup、key rotation 和 interrupted apply recovery。

Verification：现有 encrypted fixtures、provider mock/live sandbox、CRDT property tests、
rotation/crash/rollback、zero-knowledge inspection。

Exit：Renderer CloudSyncManager 不再是 canonical owner。

### P6-02 建立 Wails 三平台 qualification packaging 与资源清单

关联：REL-01

创建供集成验证使用的 macOS DMG/ZIP/sign/notarize，Windows installer/portable/ZIP，
Linux AppImage/deb/rpm/pacman；迁移 icon/protocol/resources/Mosh/ET/helper manifests。

Verification：clean-machine install、resource hash/arch、PTY/SSH/SFTP/helper/deep-link
focused smoke 和 package content inspection。

Exit：三平台 qualification artifacts 可独立安装运行；它们不是 final RC，且缺少
Phase 7 AI production implementation。

### P6-03 实现 signed updater 基础设施

关联：REL-02

按平台实现 signed manifest/package verification、download/progress、dirty editor、
tray shutdown、installer handoff、failure rollback、installed/portable/package-manager
差异。

Verification：qualification artifact 上的 N-1->N、tamper、interrupt、elevation
cancel、stale download、relaunch、rollback focused tests。

Exit：更新失败不会留下 committed quit 状态或破坏安装；final signed RC 的完整
N-1->N qualification 仍属于 P8-01。

### P6-04 实现 Electron N-1 到 Wails 的升级引导链

关联：FND-03、REL-01、REL-02、REL-03.1

Files:

- Create: cross-version upgrade/bootstrap coordinator under Go release tooling
- Create: Electron N-1 launcher/broker compatibility adapter
- Create: installed/portable/package-manager upgrade integration harness
- Modify: updater manifests and release documentation

Why：profile migration broker 只有被现有 Electron 安装可信地启动并与目标 Wails
版本握手，才构成真实升级路径。

Steps:

1. 定义 Electron N-1 与 Wails target 的 protocol/version compatibility、trust root、
   release channel 和 fail-closed mismatch behavior。
2. 为 Windows installed/portable、macOS bundle、Linux AppImage/deb/rpm/pacman 定义
   target artifact 获取、签名验证、profile 定位和启动顺序。
3. Electron 保持 writer lease，启动并认证 Wails migrator；Wails import/verify/promote
   成功后才转移 fencing ownership。
4. 定义任一进程 crash 后的 restart/recovery owner，禁止两个 updater/migrator 同时
   继续。
5. 将 Electron updater 已下载状态、channel 和信任信息映射到 Wails updater，不能
   信任 renderer payload。
6. 覆盖 broker/target version mismatch、partial install、portable moved profile、
   package-manager downgrade 和 interrupted migration。

Verification：使用 qualification artifacts 从 Electron N-1 到 Wails integration
target 的三平台/包格式升级、crash recovery、signature tamper 和 rollback focused
tests。

Exit：每种 required release target 都有可执行、可恢复的首次 Wails 迁移入口；完整
signed RC qualification 等待 P8-01。

### P6-05 Record Non-AI Completion Gate

关联：FND-01、FND-02、FND-03、FND-04、TERM-01、TERM-02、TERM-03、SSH-01、
SSH-02、SFTP-01、SFTP-02、NET-01、SYS-01、SYS-02、SYS-03、SYS-04、SYNC-01、
SYNC-02、PLUG-01、PLUG-02、PLUG-03、REL-01、REL-02

前置：P6-01、P6-02、P6-03、P6-04 完成，且下列 gate 证据同时成立。本任务只记录
docs/evidence gate，不创建或修改 production owner。

Files:

- Modify: `docs/migrations/wails-v3/migration-ledger.md`
- Verify: capability matrix、release target matrix、accepted decisions and focused evidence

Required evidence:

1. `FND`、`TERM`、`SSH`、`SFTP`、`NET`、`SYS`、`SYNC`、`PLUG` domains 中每个
   required leaf row，以及 `REL-01` 和 `REL-02`，均为 `verified`。Aggregate rows
   不参与；更早 accepted scope-removal decision 已改为 `removed` 的 rows 不参与。
2. Wails 已与这些 Electron/Node paths 断开；旧 paths 是冻结、不可演进且不被 Wails
   调用的 release carriers，不是 fallback 或第二 owner。
3. `AI-01`、`AI-02`、`AI-03`、`AI-04` 仍为 `not-started`。
4. `release-target-matrix.md` 中每个 `decision-required` classification 已由 accepted
   decision 解决；gate 引用的 decisions collectively carry exact categories
   `release-target:windows`, `release-target:macos`, `release-target:linux`,
   `agent-runtime:cursor-bun`, `agent-runtime:opencode-bun`,
   `agent-disposition:copilot`, `agent-disposition:codebuddy`, and
   `agent-disposition:cursor-cli`.
5. P6-02 package、P6-03 updater、P6-04 upgrade-bootstrap focused evidence 覆盖冻结的
   required release targets；这些 qualification artifacts 仍不是 final RC。
6. `Verification` 按 exact ordered semicolon parser 记录：`nonAiRows=verified`；
   `releaseTargets` 只列 collectively carrying 三个 `release-target:*` categories 的
   accepted decision IDs；`agentDecisions` 只列 collectively carrying 五个 Agent
   runtime/disposition categories 的 accepted decision IDs；`qualification` 固定为
   `P6-02,P6-03,P6-04`；`authority` 非空。ID 列表使用 `WV3-NNN,WV3-NNN`，无空格、
   无重复，且每个 ID 也必须在 `Decision references`。

Canonical ledger syntax:

```markdown
- Capability rows: all non-AI required rows
- Plan task: `P6-05`
- Status change: `not-started -> not-started`
- Scope change: `none`
- Verification: nonAiRows=verified; releaseTargets=<accepted release-target decision IDs>; agentDecisions=<accepted Agent decision IDs>; qualification=P6-02,P6-03,P6-04; authority=<nonempty>
- Evidence grade: `A`
- Decision references: `<accepted decisions collectively carrying every required NONAI category>`
- Gate: `NONAI-COMPLETE`
- Closure evidence: `none`
- Electron retirement: `none: Wails disconnected; Electron frozen release carrier until approved cutover/rollback triggers`
```

Exit：checker 按 ledger chronological order 接受 `Gate: NONAI-COMPLETE` record；
该 record 不推进任何 capability status。后续 required non-AI regression 会使当前 gate
epoch 失效；恢复到 verified 后必须追加新的 P6-05 gate 才可继续推进 AI。

### Phase 6 Exit Gate

- SYNC-01/SYNC-02 target owners 通过 Gate 6-9 的适用证据并与旧路径断开。
- P6-02/P6-03/P6-04 的 package/updater/bootstrap focused evidence 在 required target
  matrix 上完成，足以进入最终 AI 集成，但不构成 release qualification。
- Wails qualification artifacts 不被发布为 RC 或默认发行物。
- Electron 全部旧实现冻结为 release carrier；尚未删除 runtime。
- P6-05 canonical gate record 已通过 checker。

## 12. Non-AI Completion Gate

Phase 7 的任何 production source edit 或 maintained owner 创建前，必须由 P6-05 在
matrix、ledger 和本计划中记录以下证据：

1. `FND`、`TERM`、`SSH`、`SFTP`、`NET`、`SYS`、`SYNC`、`PLUG` domains 的所有
   required leaf target owners 以及 `REL-01`、`REL-02` 均为 `verified`；required child
   rows 已按 decomposition gate 建立。Aggregate rows 不参与。Scope removal 必须由
   更早的 accepted decision 把稳定 row 改为 `removed`/`retired`，不能删除 row 或在
   gate record 中临时用 `retired` 代替完成。
2. Wails 已与每个被替换的 Electron/Node path 断开；Electron 只作为冻结
   `release-carrier`，不得作为 Wails fallback 或第二 policy/data owner。
3. P6-02/P6-03/P6-04 的 packaging、updater、upgrade-bootstrap focused evidence 在
   frozen required target matrix 上完成；这些仍是 qualification artifacts，不是 RC。
4. `AI-01`、`AI-02`、`AI-03`、`AI-04` 均保持 `not-started`；除 `experiments/`、
   docs/testdata 和 P0-05 disposable fixtures 外，没有 `internal/capability`,
   `internal/agent`, `cmd/netcatty-mcp`, or `cmd/netcatty-tool` production path。
5. Gate decision references collectively carry exact categories
   `agent-runtime:cursor-bun`, `agent-runtime:opencode-bun`,
   `agent-disposition:copilot`, `agent-disposition:codebuddy`, and
   `agent-disposition:cursor-cli`; P0-05 本身可保持 `needs-verification`，其剩余三平台
   implementation evidence 由 Phase 7 child tasks 完成。
6. `release-target-matrix.md` 不存在 `decision-required` release row，且 gate references
   collectively carry `release-target:windows`, `release-target:macos`, and
   `release-target:linux`。Verification prose 或 irrelevant decision IDs 不足以开 gate。

Exit：P6-05 的 canonical ledger record 宣布 Non-AI Completion Gate 通过并列出证据；
在此之前，P7-01、P7-02、P7-03、P7-04、P7-05、P7-06 全部被硬阻断。
Gate 按 replay 时点使用 scope；gate 后 removal 不会 retroactively 满足它。
Gate 后 required non-AI row 从 `verified` 或 `migrated` 降到未完成状态会立即使
latest epoch 失效；post-gate approved removal 也会使曾把该 row 计为 required 的
epoch 失效。Production AI path 再次 forbidden，recovery 或 scope change 后必须追加
新的 P6-05 record。

## 13. Phase 7：Capability、MCP/CLI 与 AI Agent

Hard prerequisite：P6-05 已完成，且 append-only ledger 中按 chronology 存在通过
checker 的 `Gate: NONAI-COMPLETE` record；P0-05/Bun/runtime/Agent retirement 入口决策
和 release-target matrix 决策均已批准。AI production implementation 不能提前开始；
Phase 0 唯一允许的 AI 工作是 P0-05 只读 probes。

`internal/capability` 从本 phase 开始拥有 Agent-facing catalog/policy、Agent kinds、
Observer/Confirm/Auto projections、MCP/CLI 和 Catty/global tool metadata。它调用与
plugin 相同的 shared Go application services，但不导入、替代或授权
`internal/plugin/permissions`。

### P7-01 建立 typed Go capability catalog

关联：AI-01

Files:

- Create: `internal/capability/`
- Create: projection generator and TS/MCP/CLI fixtures

Steps:

1. 将 ID、schema、Agent policy、agent kinds、surface、handler registration 合并为
   单一 typed authority。
2. 生成 Catty/global/MCP/CLI/Wails projections。
3. 对 frozen CJS fixtures 做 differential check。
4. 实现 capability x surface x mode policy matrix。
5. Go catalog ready 后冻结 CJS catalog，只允许 parity fixes。

Verification：Go unit/race、projection byte drift、Gate 10 policy matrix。

Exit：所有 Agent adapter 从 Go catalog 派生；plugin policy owner 未改变。

### P7-02 实现 Go host RPC、native MCP 与 CLI

关联：AI-02

前置：P7-01。

Files:

- Create: `internal/rpc/`, `cmd/netcatty-mcp/`, `cmd/netcatty-tool/`

Steps:

1. 实现 authenticated local transport、bounded NDJSON/framing、deadline/cancel。
2. 实现 discovery file/token rotation/ACL 和 session scope。
3. MCP stdio 和 CLI 调用同一 host dispatch/policy。
4. 迁移 jobs、terminal execute、SFTP 和 catalog commands。
5. 对现有 CLI JSON 和 MCP tool schema golden fixtures 做 differential tests。

Verification：fuzz malformed frames、auth/scope/approval/cancel、Windows quoting、process
cleanup。

Exit：Node MCP/CLI 不再被 Wails/Go runtime 使用。

### P7-03 实现 Go provider HTTP/SSE 基础设施

关联：AI-03

Files:

- Create: `internal/agent/providers/`, `internal/platform/netpolicy/`

Steps:

1. 实现 proxy/TLS、hostname+resolved-IP SSRF policy 和 redirect reauthorization。
2. 实现 OpenAI-compatible、Anthropic、Google 和本地 provider stream adapters。
3. API key 只在 Go request-local memory 注入。
4. 实现 body/header/stream idle/total limits 和 cancellation。
5. 覆盖 DNS rebinding、private IP、metadata endpoint 和 cross-origin auth stripping。

Verification：HTTP fixture servers、fuzz framing、security negative tests。

Exit：Wails Catty provider 不需要 Electron AI HTTP bridge。

### P7-04 实现 Go AgentRuntime

关联：AI-03

Files:

- Create: `internal/agent/runtime|events|context|tools|trace|sessions/`
- Modify: React AI hook 成为 event/presentation adapter

前置：P7-01、P7-02、P7-03 均完成；每个 capability-backed tool dispatch 还必须
依赖该 capability 的已验证 Go handler。

Steps:

1. 移植 canonical AgentEvent 和 one-active-turn owner。
2. 实现 unified stop/steer、trace、session identity 和 cleanup。
3. 实现 tool execution、output handles、dedup、redaction 和 per-session queue。
4. 实现 token budget、pre-turn/413 compaction、step pruning 和 reinjection。
5. 使用 frozen event traces 做 differential parity。

Verification：Gate 11、Go race、large input/output、cancel at every lifecycle boundary。

Exit：Go 是 Catty turn canonical owner；React 不再编排 authoritative turn。

### P7-05 迁移外部 Agent adapters

关联：AI-04

前置：P7-04；每个 retained Agent 的 Phase 7 入口 decision 已批准。仅 P0-05 的
协议握手和 disposable fixture probe 可在本 phase 前存在。

按 P0-05 审计结果逐个拆子任务，每个 agent 一个 capability child row/ledger entry：

1. Codex app-server JSONL。
2. ACP agents。
3. Cursor/OpenCode/Claude/Copilot/CodeBuddy 的已批准稳定 CLI/stdio/HTTP 协议。
4. model discovery、session resume、approval、stop、steer、usage/event normalization。
5. 没有稳定非 Node 协议的 agent 按入口决定退休，不创建永久 sidecar。
6. 审计 retained executable 的 shebang、bundled runtime、动态依赖、SBOM 和递归
   process tree；用户自行安装但由 Netcatty 启动的 Node CLI 同样违反 WV3-002。

Verification：每 adapter protocol fixture、process supervisor、session runtime identity、
cancel/kill、malformed output 和 required target provenance。

Exit：所有 retained agent 不依赖 Node SDK、Node launcher、embedded Node runtime 或
recursive Node child，并有 Gate 14 provenance/process evidence；AI-04 才可升级状态。

### P7-06 退役 Wails CJS capability/MCP/CLI/AI path

关联：AI-01、AI-02、AI-03、AI-04、REL-03.1、REL-03.2

前置：P7-01、P7-02、P7-03、P7-04、P7-05 的全部 required leaf rows `verified`；
不可保留 Agent 必须由更早 accepted scope-removal decision 改为 `removed`，并具有
`retired` status 和 retirement evidence。

Steps:

1. 删除 Wails 不再使用的 CJS catalog/codegen/MCP/CLI/AI bridges。
2. 移除 Wails package 的 runtime Node AI SDK dependencies 和 packaged resources。
3. 保留 Electron 壳所需的最小冻结适配直到 P9-01；不得成为 Wails business owner。
4. 运行 package import/child command scan，确认 Wails 不调用 Node。

Exit：Go/Wails AI 与工具链 runtime Node-free；Electron AI path 仍只是冻结 release
carrier，实际 repository deletion 等待 P9-01。

### Phase 7 Exit Gate

- Capability、MCP、CLI、Catty 和 retained external agents 通过 Gate 10/11。
- Wails runtime 不加载任何 Node Agent SDK，retained Agent 也不使用 Node launcher、
  embedded Node runtime 或 recursive Node child。
- 不可替代 Agent 已按 Phase 7 entry decision 退休。
- 所有 required AI leaf rows 至少 `verified`；被更早 accepted decision 改为
  `removed` 的 rows 保持 `retired` 并退出 required scope，可以创建最终 RC。

## 14. Phase 8：最终签名 RC 与默认 Wails 切换

### P8-01 最终签名 Wails RC 全量回归

关联：全部 pre-cutover required leaf rows，包括 REL-03.1；明确排除 REL-03.2

前置：Phase 7 Exit Gate 通过。Phase 6 artifacts 只能作为输入重新构建，不得直接
提升为 RC。

Steps:

1. 从最终 non-AI + AI source set 构建签名 RC，执行完整 Gate 1-14；Gate 14 在切换
   前验证 `REL-03.1` Wails artifact/runtime purity，不要求 `REL-03.2` repository
   retirement。
2. 在三平台执行 profile migration、Electron N-1 到 Wails N signed upgrade 和真实
   reverse rollback。
3. 执行 install/launch/terminal/SSH/SFTP/AI/plugin/sync/update/uninstall journeys。
4. 对功能、性能、内存、启动时间与 Electron baseline 生成差异报告。
5. 所有 gap 分类为 blocker、批准阈值变化或明确产品 retirement。

Exit：除 REL-03.2 外的所有 required leaf rows 至少 `verified`；removed rows 保持有
批准依据的 `retired`；REL-03.1 具有 Gate 14 A 级 artifact/process evidence；无未批准
blocker。REL-03.2 仍为 `not-started`，不阻止 cutover。

### P8-02 切换默认发行物并开启 bounded rollback window

关联：REL-01、REL-02、REL-03.1

前置：P8-01 final signed RC 和 pre-cutover Gate 1-14 通过。

Steps:

1. 将 Wails 设为默认 dev/build/release entry。
2. Electron 只允许 read-only/export/reverse rollback，不再写 profile。
3. 发布 migration diagnostics 和 recovery instructions。
4. 监测 migration/update/terminal/credential failure signals。
5. 在切换前批准并记录 rollback window policy：开始/最晚结束版本或日期、迁移失败率
   阈值、最低有效样本、隐私允许的诊断来源、support blocker 分类、延长/紧急回滚
   authority 和关闭证据；不得写“以后删除”。

Exit：Wails 成为三平台默认发行物；Electron 从冻结 release carrier 转为限期
read-only/export-only rollback carrier。追加 grade A ledger gate：只引用 `P8-02`，
`Capability rows: REL-03.1`，`Gate: WAILS-CUTOVER`，并使用 exact structured
`Verification: rc=<nonempty>; platforms=<nonempty>; migration=<nonempty>;
rollback=<nonempty>; authority=<nonempty>`。该 gate 不推进 status。

### P8-03 Record rollback-window closure gate

关联：REL-03.2

前置：P8-02 `WAILS-CUTOVER` gate 已通过；accepted decision carrying exact category
`rollback-window-closure` 已批准；实际 observation evidence 满足该 decision。

Steps:

1. 追加只引用 `P8-03`、grade A、`Gate: ROLLBACK-CLOSED` 的 ledger record。
2. 使用 `Capability rows: REL-03.2`，并重复其 current status，不推进 capability。
3. `Closure evidence` 必须使用 exact form `thresholds=<nonempty>;
   sample=<nonempty>; blockers=<nonempty>; authority=<nonempty>`。
4. Decision references 必须包含 carrying `rollback-window-closure` 的 accepted decision；
   title/prose 或 irrelevant decision 不足以开 gate。

Exit：checker 接受 chronological `ROLLBACK-CLOSED` gate；Phase 9 才可开始。

## 15. Phase 9：回滚观察、Electron/Node 删除与最终收口

### P9-01 关闭 rollback window 并删除 Electron runtime

关联：REL-03、REL-03.2

前置：P8-03 已追加 grade A `Gate: ROLLBACK-CLOSED` record，引用 accepted decision category
`rollback-window-closure`，并记录 exact structured Closure evidence
`thresholds=<nonempty>; sample=<nonempty>; blockers=<nonempty>;
authority=<nonempty>`。P9-01 的 first advancing ledger entry 必须只引用 `P9-01`
并引用该 category decision；后续 `verified`/`migrated` advancement 必须只引用
`P9-02`。仅等待 P8-02 定义的时间流逝、decision title 或 verification prose 均不足。

Steps:

1. 删除 Electron main/preload/bridges/worker/window runtime。
2. 删除冻结的 Electron plugin BrowserWindow/utilityProcess runtime、plugin preload、
   plugin protocol、JavaScript/Node plugin bootstrap 和相关 runtime packages。
3. 删除 electron-builder 配置和 Electron packaging scripts。
4. 删除只为 Electron runtime 存在的 Node dependencies/patches/resources。
5. 删除 Electron profile writer/export compatibility paths；只有在产品明确要求时
   才保留独立、版本化且不依赖 Node runtime 的 offline recovery tool。
6. 更新 README、AGENTS、CONTRIBUTING、CI 和 release docs 的 runtime truth。

Verification：repository import/dependency scan、build/package/test、plugin legacy path
negative tests 和无可启动 Electron entry proof。

Exit：代码库不存在可启动 Electron 路径或 Electron plugin runtime；rollback carrier
已实际删除。

### P9-02 证明 repository 与 packaged runtime Electron/Node-free

关联：REL-03、REL-03.2

Steps:

1. 生成三平台 SBOM 和 package file inventory。
2. 扫描 repository 和 artifacts 中的 Electron binary/library、`.asar`、runtime
   `node_modules`、Node executable、CJS runtime bootstrap 和 Node SDK imports。
3. 监控所有 child process command，确认不调用 `node`。
4. clean machine 启动全部 retained Agent/plugin/helper 路径。
5. 重跑完整 Gate 14 和所有受删除影响的 Gate 1-13 regression evidence。

Exit：REL-03.2 与 Gate 14 最终 A 级 repository/artifact/process proof 完成。

### P9-03 最终文档与 capability 收口

Steps:

1. 将全部 required leaf rows 标为 `migrated`；removed rows 保持有批准依据的
   `retired`，aggregate rows 保持 `not-started`。
2. 确认每行都有 ledger evidence 和 old-owner retirement。
3. 将 `README.md` Current State 更新为 migration complete。
4. 把仍有效的目标架构内容同步到 `AGENTS.md`，避免后续 AI 继续按 Electron
   边界工作。
5. 将本计划标记 complete，并记录最终 release/commit refs。

Exit：文档、代码、包内容和运行时 owner 一致。

## 16. 推荐执行批次

### Batch A：必须串行

当前下一项可独立推进的安全切片为 `P0-04`；P0-02/P0-03 同时保留三平台和
formal evidence 补齐路线，P0-05 可继续只读验证。但 `P0-01` 的三平台证据、
`P0-01A` 的用户决策与所有 Phase 0 Exit 条件闭合后，才允许
`P1-01 -> P1-02 -> P1-03`。

原因：先固定 baseline 和可行性，再建立 production owners。

### Batch B：Profile 核心串行

`P2-01 -> P2-01A -> P2-02 -> P2-03 -> P2-04 -> P2-05 -> P2-06 -> P2-07`

原因：writer lease、credential 和 cutover 是数据安全链，不能跳步。

### Batch C：非 AI 功能域

- Terminal track：`P3-01 -> P3-02 -> P3-03 -> P3-04 -> P3-04A ->
  P3-05/P3-07 -> P3-06/P3-08`
- System track：Phase 3 exit 后执行 P4 tasks；P4-02 复用已验证的 P3-01 route
  rebind。

并行任务不得修改同一 canonical owner 或同一 generated artifact。

### Batch D：Plugin、Sync 与非 AI Gate

- Plugin track：`P5-01 -> P5-02A`；之后 P5-02、P5-03、P5-04、P5-05 可由
  不同 owner 并行，P5-06/P5-07 在基础 runtime 后合并，最后执行 P5-08 冻结
  Electron plugin release carrier。
- Phase 6：`P6-01` 与 `P6-02 -> P6-03` 可在 owner 不重叠时并行，随后
  `P6-04 -> P6-05 -> Non-AI Completion Gate`。
- 此 batch 不包含 AI production implementation；Phase 6 artifacts 仅用于
  integration/qualification。

### Batch E：AI、最终 RC、切换与删除

`P6-05/Non-AI Completion Gate -> (P7-01 -> P7-02) + P7-03 -> P7-04 -> P7-05 ->
P7-06 -> P8-01 -> P8-02 -> 观测窗口 -> P8-03 -> P9-01 -> P9-02 -> P9-03`

P7-01/P7-03 只可在 Non-AI Completion Gate 后并行；P7-02 只依赖 P7-01，P7-04
等待 P7-01、P7-02、P7-03 和 required handlers。P8-01 是唯一 final signed RC
qualification；P9-01/P9-02 在 rollback observation closure 后完成 REL-03.2。

## 17. Execution Readiness View

- Intent Lock：全面迁移到 Wails v3/Go，最终 runtime Electron/Node-free。
- Scope Fence：保留 React/Vite build toolchain；不顺带重写 UI 或缩减产品功能。
- Baseline Lock：以 migration docs、AGENTS、现有 owner tests 和 P0-01 fixtures 为准。
- Approved Behavior：无损 profile、单 writer、三平台同时切换、plugin v2 不兼容旧
  runtime、WASM+declarative UI、AI final-domain sequencing、Agent 非 Node 协议。
- Owner / Contract Constraints：Go application services 是 operation owner；
  `internal/plugin/permissions` 独立拥有 plugin principals/grants/brokers；延后的
  `internal/capability` 只拥有 Agent catalog/policy/MCP/CLI projections；adapters 无政策权。
- Compatibility Boundary：现有用户数据/核心行为保留；旧 plugin runtime 不保留。
- Retirement Boundary：Wails 验证后断开旧路径并冻结 Electron release carrier；
  P9-01 在 cutover 后观察窗口关闭时统一执行实际 Electron/plugin runtime 删除。
- Task Batches：Phase 0-9、Non-AI Completion Gate 和 Batch A-E。
- Test Obligations：Gate 1-14，专项证据优先于一次全量命令。
- Review Gates：每个 phase exit；data/security/architecture/release 变更需独立 review。
- Drift / Rewind Rules：关键 probe 失败、secret 不可迁移、双写、永久 sidecar 或
  三平台缺口时回到设计，不添加 fallback。
- Evidence Required Before Phase 7：P6-05 ledger gate 存在；所有 required non-AI leaf
  owners 已验证并断开旧路径、Electron 冻结、Phase 6 focused qualification evidence、
  AI rows 仍 `not-started`、release/Agent entry decisions 已解决。
- Evidence Required Before Cutover：P8-01 final signed RC、完整 Gate 1-14 和
  REL-03.1 artifact purity；REL-03.2 不得作为 pre-cutover circular prerequisite。
- Evidence Required Before Completion：matrix、ledger、tests、benchmarks、三平台
  package smoke、REL-03.2 repository retirement、最终 SBOM 和 old-owner deletion。
- Advisory Boundary：本计划是执行指导和证据边界，不是自动完成授权。

## 18. 首个安全执行切片

当前下一项可独立推进的 non-AI 工作为 P0-01A 发布目标决策关闭与 P0-01/P0-02/P0-03/
P0-05 的三平台/formal 证据补齐；`P0-04` 已取得 Windows DPAPI C 级证据并保持
`needs-verification`。这些 probe 不声称关闭彼此的证据/决策缺口，也不创建 production
Go owner。

在 P0-01、P0-01A、P0-01B 完成前不得：

- 创建 production Go terminal/SSH/SFTP owner；
- 修改用户 profile 或 credential format；
- 删除 Electron dependencies；
- 将 Wails 设置为默认 dev/release；
- 宣称任何 capability 已迁移。

在 Non-AI Completion Gate 通过前还不得创建 `internal/capability`、Go
AgentRuntime、native MCP/CLI 或任何 maintained external-Agent adapter；P0-05 的只读
审计和 disposable handshake fixtures 是唯一例外。
