# Wails v3 Migration Ledger

Status: active when implementation begins

This file and `capability-matrix.md` are the sole authority for migration status
and resume position. This append-only evidence index is not a changelog and must
not duplicate raw test logs. Plan status and next-action prose are navigation
hints only and cannot override the matrix or ledger. Link to the relevant
capability row, source paths, verification output or CI run.

## Rules

1. Append one entry whenever a capability status advances, regresses or becomes
   blocked.
2. Never edit prior evidence to make a later result look successful. Add a new
   correction or regression entry.
3. A capability cannot become `migrated` without a ledger entry identifying the
   retired Electron owner.
4. Keep secrets, full logs and generated output out of this document.
5. Use [templates/slice-record.md](./templates/slice-record.md).
6. Every entry has `Gate: none` unless it is a canonical P6-05
   `NONAI-COMPLETE`, P8-02 `WAILS-CUTOVER`, or P8-03 `ROLLBACK-CLOSED` record.
   Gate records have no capability status effect. NONAI may be recorded again
   after a required non-AI regression is recovered; every record starts a new
   gate epoch.
7. Every entry records `Scope change`. Ordinary entries use exact `none`; a
   removal uses exact `<CAPABILITY>: required -> removed`, transitions that row
   to `retired`, cites `scope-removal:<CAPABILITY>` in the same entry, and records
   canonical `removed:` or `deleted:` evidence.
8. Every entry records `Closure evidence`. Ordinary entries use exact `none`;
   only `ROLLBACK-CLOSED` uses the structured closure form.
9. NONAI `Verification` uses exact ordered semicolon fields:
   `nonAiRows=verified; releaseTargets=<decision IDs>;
   agentDecisions=<decision IDs>; qualification=P6-02,P6-03,P6-04;
   authority=<nonempty>`. Decision ID lists are comma-separated without spaces,
   must also appear in `Decision references`, and are category-checked against
   accepted decisions.

## Entries

## WV3-L001 - 2026-08-23 - P0-01 Electron 行为与性能基线

- Capability rows: all rows; no status advancement
- Plan task: `P0-01`
- Status change: `not-started -> not-started`
- Scope change: `none`
- Goal: 冻结可机器比较的 Electron contract fixtures、owner/test index、benchmark
  protocol 和可获得的本机运行证据。
- Go canonical owner: none; baseline task only
- Frontend adapter: none
- Electron owner affected: none; all current owners remain unchanged
- Preserved invariants: capability identity/policy/surfaces、CLI/MCP/Agent projections、
  bridge method inventory、storage key candidates、plugin manifest/RPC/path validation、
  terminal flow constants、AgentEvent/session restore shape
- Data/schema impact: synthetic fixtures only; no user profile access
- Security impact: exporter does not read localStorage/userData/environment secrets；测试
  扫描 secret-bearing field names 和 markers
- Verification: baseline generate/check；6 fixture tests；AgentEvent TypeScript
  assignability check；42 capability projection tests；
  Windows keyword performance pass；sustained-only throughput pass
- Platforms covered: Windows 10 x64 local observation only
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-003`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: none: this task only records the release carrier baseline
- Documentation updated: baseline、matrix、plan、ledger
- Residual risks: macOS/Linux 未运行；Windows full repaint 失败；WebGL overflow temp
  cleanup hangs；native node-pty rebuild 缺 Spectre libraries；remote SSH stress skipped
- Next safe slice: finish missing P0-01 platform evidence; P0-01A/P0-01B may proceed in
  parallel, but no production Go owner may start
- Drift decision: `needs-verification`

## WV3-L002 - 2026-08-23 - P0-01A 三平台发布目标矩阵

- Capability rows: `FND-01`, `FND-04`, `REL-01`, `REL-02`, `REL-03`
- Plan task: `P0-01A`
- Status change: `not-started -> not-started`
- Scope change: `none`
- Goal: 区分当前 Electron release facts 与 Wails required/best-effort/unsupported
  targets，并闭合 evidence grade A 的平台集合。
- Go canonical owner: none; release governance task only
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: 三平台同时切换、当前 architecture/artifact classes、Linux
  Secret Service fail-closed、Windows official x64-only reality
- Data/schema impact: none
- Security impact: Linux 无 secure Secret Service 不进入 lossless migration support
- Verification: 对照 `.github/workflows/build.yml`、`electron-builder.config.cjs`、
  `flake.nix`、`nix/package.nix`、README 与 Wails upstream releases/go.mod
- Platforms covered: release configuration inventory only；未运行新平台测试
- Evidence grade: `C`
- Decision references: `WV3-003`, `WV3-007`, `WV3-008`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: none: release governance task only
- Documentation updated: README document map、release target matrix、gates、decisions、
  plan、ledger
- Residual risks: minimum Windows/macOS versions 未批准；Wails GTK 4.14+ 与当前
  RHEL 8/UOS/Deepin compatibility floor 冲突；Windows ARM64 README/CI 漂移
- Next safe slice: obtain user platform-support decisions；P0-01B 可独立继续
- Drift decision: `pause-for-user`

## WV3-L003 - 2026-08-23 - P0-01B 迁移文档一致性检查

- Capability rows: all rows; no status advancement
- Plan task: `P0-01B`
- Status change: `not-started -> not-started`
- Scope change: `none`
- Goal: 把每项替换后的 matrix/ledger/plan/decision 回写变成 CI 可执行的一致性检查。
- Go canonical owner: none; documentation governance tooling only
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: matrix stable IDs/状态机、append-only ledger transition 链、
  verified/migrated 需 A 级证据、migrated 需具体退役证据、retired 需批准决策、
  复合 capability 实施前拆分、链接不越出仓库
- Data/schema impact: none; checker is read-only
- Security impact: 链接解析先做 lexical containment，拒绝绝对盘符/UNC/越界路径；
  checker 不写治理文档
- Verification: `npm run check:migration-docs` 通过，adversarial mutation suite 覆盖
  regression/grade/decision/heading/ID/链接/只读退出语义；workflow 断言通过
- Platforms covered: 平台无关的 Node 文档检查
- Evidence grade: `B`
- Decision references: none
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: none: baseline governance task only
- Documentation updated: plan、ledger、template canonical heading/retirement 语法
- Residual risks: 尚未与真实 CI 全量结合；与 GitHub workflow 已有 3 项 CRLF 解析
  失败无关，未顺带修复
- Next safe slice: P0-02/P0-03 可在本机探针范围内推进；三平台证据仍需平台主机
- Drift decision: `needs-verification`

## WV3-L004 - 2026-08-24 - P0-05 外部 Agent 非 Node 协议审计

- Capability rows: `AI-04`, `REL-03`
- Plan task: `P0-05`
- Status change: `not-started -> not-started`
- Scope change: `none`
- Goal: 识别当前 Agent 嵌入拓扑、稳定非 Node 协议、transitive runtime 风险和
  Phase 5 前必须作出的保留/退休决策。
- Go canonical owner: none; protocol baseline task only
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: runtime-qualified session identity、single host policy、resume/
  cancel/steer/model/attachment capability、canonical AgentEvent、no duplicate approval
- Data/schema impact: none
- Security impact: 识别 common Node MCP child、Node shebang/embedded runtime、vendor
  built-in write/sensitive-read/network bypass、Grok/Cursor non-interactive auto-approval
  和 project config token persistence 风险
- Verification: local owner/package/launcher inspection；Codex/Grok/session 专项 102
  tests passed；全 SDK suite 366/375 passed，9 个 pre-existing Windows path failures；
  three parallel upstream documentation reviews summarized in
  `baselines/external-agent-protocols.md`
- Platforms covered: Windows static/package evidence only；no authenticated agent turn
- Evidence grade: `C`
- Decision references: `WV3-002`, `WV3-003`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: none: baseline task only
- Documentation updated: external Agent protocol baseline、matrix、plan、ledger
- Residual risks: 三平台 provenance/process tree 未测；所有 vendor 仍依赖 Node
  Netcatty MCP；Copilot/CodeBuddy/Cursor CLI-login runtime 阻断；Cursor/OpenCode Bun
  acceptance 未决定；Grok ACP permission/reverse RPC/cancel 不完整
- Next safe slice: 取得 Bun 和 blocked Agent 产品决策；P0-02/P0-03/P0-04 探针可继续
- Drift decision: `needs-verification`

## WV3-L005 - 2026-08-24 - AI-last sequencing governance realignment

- Capability rows: all rows; no status advancement
- Plan task: `P0-01B` governance follow-up
- Status change: `not-started -> not-started`
- Scope change: `none`
- Goal: 记录用户批准的 sequencing drift，将 Plugin v2、Sync 和非 AI release
  infrastructure 放在 AI production implementation 之前，并消除 cutover 与
  Electron repository retirement 的循环前置。
- Go canonical owner: none; documentation and architecture governance only
- Frontend adapter: none
- Electron owner affected: none; Electron remains the current release shell
- Preserved invariants: AI 是最终 migration domain、P0-05 仅只读审计、plugin/Agent
  policy principal 分离、单一 Go application-service operation owner、三平台同时切换、
  bounded rollback 后才删除 Electron
- Data/schema impact: none
- Security impact: 明确 `internal/plugin/permissions` 独立拥有 plugin runtime identity、
  principals、grants、secret leases、quotas 和 broker authorization；延后到 Phase 7 的
  `internal/capability` 只拥有 Agent catalog/policy/MCP/CLI surfaces
- Verification: docs-only authority cross-reference review；`npm run check:migration-docs`；
  docs-scoped `git diff --check`
- Platforms covered: documentation governance only; no platform runtime evidence
- Evidence grade: `C`
- Decision references: `WV3-009`, `WV3-010`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: none: P5-08 freezes the Electron plugin release carrier;
  P9-01 performs actual Electron/plugin runtime deletion after rollback closure
- Documentation updated: README、architecture、implementation plan、verification gates、
  capability/release matrices、external-Agent baseline、ledger
- Residual risks: P0-01 remains `needs-verification`; P0-01A remains `pause-for-user`;
  P0-05 remains `needs-verification`; release-target and Bun/Agent retirement decisions
  remain open
- Next safe slice: `P0-02` disposable Wails shell probe; no production AI owner before
  the Non-AI Completion Gate
- Drift decision: `accepted-docs-only`; this entry supersedes only WV3-L004 future
  Phase 5 scheduling language and does not alter its historical evidence

## WV3-L006 - 2026-08-24 - Stage-2 migration governance hardening

- Capability rows: all rows; no status advancement
- Plan task: `P0-01B` governance follow-up
- Status change: `not-started -> not-started`
- Scope change: `none`
- Goal: Make capability scope, AI-last gating, REL-03 child ordering, retirement
  triggers and stable-ID references machine-enforced without advancing implementation.
- Go canonical owner: none; documentation governance tooling only
- Frontend adapter: none
- Electron owner affected: none; all runtime owners remain unchanged
- Preserved invariants: matrix/ledger status authority, append-only chronological
  evidence, stable capability/task IDs, AI-last sequencing, bounded rollback deletion
- Data/schema impact: documentation schema adds capability Scope and ledger Gate fields
- Security impact: premature AI ownership, unapproved scope removal and unverifiable
  Electron retirement now fail the consistency check
- Verification: `npm run check:migration-docs`; adversarial mutation suite; scoped
  `git diff --check`; forbidden ID-range scan
- Platforms covered: platform-independent documentation governance only
- Evidence grade: `C`
- Decision references: `WV3-009`, `WV3-010`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: none: governance hardening only; no capability owner changed
- Documentation updated: README, capability matrix, plan, ledger, template,
  external-Agent baseline and checker
- Residual risks: P0-01, P0-01A and P0-05 retain their recorded evidence gaps;
  release-target and Agent runtime decisions remain open
- Next safe slice: `P0-02` disposable Wails shell probe
- Drift decision: `accepted-docs-only`

## WV3-L007 - 2026-08-24 - Final stage-2 migration governance hardening

- Capability rows: all rows; no status advancement
- Plan task: `P0-01B` governance follow-up
- Status change: `not-started -> not-started`
- Scope change: `none`
- Goal: Enforce structured decisions, chronological scope and gate replay,
  release lifecycle closure, retirement persistence and the pre-gate AI source boundary.
- Go canonical owner: none; documentation governance tooling only
- Frontend adapter: none
- Electron owner affected: none; all runtime owners remain unchanged
- Preserved invariants: matrix/ledger status authority, stable IDs, AI-last epochs,
  capability-specific scope removal, bounded rollback and retained retirement evidence
- Data/schema impact: ledger schema adds Scope change and Closure evidence; decision
  schema adds exact Categories metadata
- Security impact: production AI owners, unapproved removals, premature cutover and
  rollback deletion fail the consistency check
- Verification: `npm run check:migration-docs`; adversarial checker suite; scoped
  `git diff --check`
- Platforms covered: platform-independent documentation governance only
- Evidence grade: `C`
- Decision references: `WV3-009`, `WV3-010`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: none: governance hardening only; no capability owner changed
- Documentation updated: decisions, matrix, plan, ledger, gate guide, template,
  Agent baseline and checker
- Residual risks: release-target, Bun/runtime, Agent disposition and rollback closure
  decisions remain intentionally open
- Next safe slice: `P0-02` disposable Wails shell probe
- Drift decision: `accepted-docs-only`

## WV3-L008 - 2026-08-25 - Stage-2 semantic governance completion

- Capability rows: all rows; no status advancement
- Plan task: `P0-01B` governance follow-up
- Status change: `not-started -> not-started`
- Scope change: `none`
- Goal: Complete structured gate authority, chronological invalidation/repass,
  release lifecycle and retained retirement evidence enforcement.
- Go canonical owner: none; documentation governance tooling only
- Frontend adapter: none
- Electron owner affected: none; all runtime owners remain unchanged
- Preserved invariants: matrix/ledger authority, accepted decision categories,
  AI-last epochs, chronological scope, bounded rollback and stable retirement proof
- Data/schema impact: NONAI Verification now uses exact structured fields; no user
  or runtime schema changed
- Security impact: self-asserted gates, production AI paths after regression and
  premature Electron deletion now fail the consistency check
- Verification: `npm run check:migration-docs`; expanded adversarial mutation
  suite; scoped `git diff --check`; independent checker logic review
- Platforms covered: platform-independent documentation governance only
- Evidence grade: `C`
- Decision references: `WV3-009`, `WV3-010`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: none: governance hardening only; no capability owner changed
- Documentation updated: decisions, capability matrix, plan, ledger, gate guide,
  template and checker
- Residual risks: release-target, Bun/runtime, Agent disposition and rollback
  closure decisions remain intentionally open
- Next safe slice: `P0-02` disposable Wails shell probe
- Drift decision: `accepted-docs-only`

## WV3-L009 - 2026-08-28 - P0-02 Wails v3 Windows shell probe

- Capability rows: `FND-01`, `FND-04`, `SYS-02`, `SYS-03`, `REL-01`
- Plan task: `P0-02`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: 建立独立、可丢弃的 Wails v3 beta.12 多窗口壳，验证 Windows 上的编译、
  原生启动、第二实例入口及 WebView capability surface。
- Go canonical owner: none; `experiments/wails-shell-probe/` is disposable evidence
- Frontend adapter: independent vanilla TypeScript frontend under the probe; production React is unchanged
- Electron owner affected: none; Electron remains the release carrier and sole production shell
- Preserved invariants: four explicit window roles with opaque tokens, close veto, runtime-ready
  events, second-instance/deep-link intent capture, tray/shortcut entry, native parent handle check
- Data/schema impact: none; the probe does not read or write Netcatty profiles
- Security impact: probe-specific URL scheme, bounded in-memory event history, no secrets or
  production bridge access
- Verification: Go unit tests and vet; clean locked frontend install/build; CI probe check;
  pinned Wails beta.12 Windows amd64 build; bounded primary-process launch and
  second-instance exit smoke
- Platforms covered: Windows 10 22H2 x64 build 19045 only; interactive WebView checks and
  macOS/Linux remain pending
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-003`, `WV3-006`, `WV3-008`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: no Electron owner retires from a disposable probe;
  P1-02 production shell verification starts the later retirement track
- Documentation updated: README, capability matrix, implementation plan, probe report, ledger
- Residual risks: renderer crash is not publicly exposed on Windows/Linux beta.12; installed
  deep-link, tray, shortcut, IME, clipboard, WebGL, Monaco and native dialog interactions need
  manual evidence; macOS/Linux build and smoke are absent
- Next safe slice: complete the P0-02 three-platform manual matrix; P0-03 may run as an
  independent disposable probe while P0-01/P0-01A evidence remains open
- Drift decision: `needs-verification`

## WV3-L010 - 2026-08-31 - P0-03 Windows terminal data-plane probe

- Capability rows: `TERM-01`
- Plan task: `P0-03`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: 验证 authenticated loopback WebSocket binary candidate 能在真实 Windows
  WebView2/xterm 中以 bounded credit、generation/sequence fencing、ordered drain 和
  separate urgent route 无损重放 canonical Electron terminal workload。
- Go canonical owner: none; `experiments/terminal-data-plane/` is disposable evidence
- Frontend adapter: independent TypeScript/xterm harness under the probe; production React is unchanged
- Electron owner affected: none; MessagePort/output-flow owners remain the release baseline
- Preserved invariants: zero loss/duplication/reordering、post-xterm credit return、metadata-only
  ingress credit、stale generation rejection、bounded replay、urgent ETX during stall、
  reload/rebind/popup recovery and one active route owner
- Data/schema impact: synthetic workload fixture and bounded in-memory reports only; no user profile access
- Security impact: IPv4 loopback-only listener、exact Host/Origin allowlist、independent
  cryptographic one-use data/urgent tokens、strict binary frame bounds and no token logging
- Verification: `npm run check:migration-electron-baseline`; `npm run
  check:terminal-data-plane-probe`; focused Go race/vet and frontend protocol/controller
  suites; native Windows amd64 build; Windows 10 WebView2 autorun processed 1600 chunks and
  9708106 bytes with sequence/byte/credit/digest integrity true, urgent ACK during stall,
  generation 2 rebind, four memory phases and exit code 0
- Platforms covered: Windows 10 22H2 x64 build 19045 with WebView2 149.0.4022.52 only
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-003`, `WV3-006`, `WV3-008`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: P3-01 may replace the Electron data plane only after
  Gate 3 passes on every required WebView; a falsified probe candidate is deleted before another
  candidate is implemented
- Documentation updated: README, Electron baseline, capability matrix, implementation plan,
  terminal data-plane probe report and ledger
- Residual risks: macOS/Linux、3 warm-up/30-round paired Electron evidence、native popup
  movement、hidden/reload formal rounds、4/8-session full workloads and retained raw
  RSS/latency reports are absent; Windows WebView2 emitted shutdown diagnostic error 1412
- Next safe slice: finish P0-03 platform/formal evidence; P0-04 may proceed independently while
  P0-01/P0-01A evidence and decisions remain open
- Drift decision: `needs-verification`

## WV3-L011 - 2026-09-08 - P0-04 Windows secret unseal and Go re-seal probe

- Capability rows: `FND-03`
- Plan task: `P0-04`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: 验证 Electron safeStorage 解封的合成 secret 能经 authenticated ephemeral channel
  无损进入 Go 侧 DPAPI user-range 转封，并以 fail-closed 方式处理损坏源、不可用
  keyring 与进程中断。
- Go canonical owner: none; `experiments/profile-secret-migration/` is disposable evidence
- Frontend adapter: Electron-side CJS corpus/channel/runner under the probe; production React
  and bridges are unchanged
- Electron owner affected: none; `safeStorage` and credential/vault backup bridges remain the
  release baseline
- Preserved invariants: X25519 + HKDF-SHA256 transcript-bound channel key、双向 HMAC
  confirmation、AES-256-GCM direction-separated nonce 与 canonical AAD 绑定
  direction/sequence/ID/format/purpose、strict JSON 帧边界、空 stderr、no-plaintext-leak
- Data/schema impact: synthetic corpus only (24 `enc:v1`, 3 `safeStorage-raw`, 4 edge, 3
  metadata); no real profile or credential data touched
- Security impact: purpose-bound DPAPI entropy、seal 后 in-process round-trip 校验、negative
  source rejection、31-canary leak scan over stdout/stderr/argv/env/isolated root、interrupt
  injection 不得产出 passing receipt
- Verification: `go -C experiments/profile-secret-migration test ./...` and `go vet`; Node
  `crypto.hkdfSync` 独立复算 KDF golden; live Windows runner 2026-09-08 passed/cleanup/leakScan
  all true for 31 fixtures plus 3 metadata; three `--interrupt` modes each reported
  passed false with cleanup and leakScan true
- Platforms covered: Windows 10 22H2 x64 build 19045 only; macOS/Linux live keyrings and leak
  scans absent; non-Windows providers are fail-closed stubs
- Evidence grade: `C`
- Decision references: `WV3-002`, `WV3-004`, `WV3-007`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: P2-04/P2-05/P2-06 production owners replace the probe;
  the probe is deleted or reduced to a harness once they land
- Documentation updated: README, capability matrix, implementation plan, secret-migration probe
  report and ledger
- Residual risks: macOS Keychain/Linux Secret Service live evidence、three-platform leak scans
  and the Phase 0 exit gate remain open; the Electron driver metadata phase was added during
  close-out and still needs cross-platform runs
- Next safe slice: close P0-01A release-target decisions and complete P0-01/P0-02/P0-03
  platform evidence; production Go owners stay forbidden until the Phase 0 exit gate closes
- Drift decision: `needs-verification`

## WV3-L012 - 2026-09-08 - P0-01A release target matrix closed

- Capability rows: all rows; no status advancement
- Plan task: `P0-01A`
- Status change: `not-started -> not-started`
- Scope change: `none`
- Goal: 以产品决策冻结三平台 required 发布目标与最低支持版本，解除 P0-01A 的
  `pause-for-user` 停止状态并闭合 Gate 13 的 A 级目标集合。
- Go canonical owner: none; release governance task only
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: 三平台同时切换（`WV3-003`）、Linux Secret Service fail-closed
  （`WV3-007`）、当前 architecture/artifact classes 继承（`WV3-008`）、矩阵变更须新
  decision 的规则
- Data/schema impact: none
- Security impact: 旧 glibc 2.28 兼容目标退休后，无法升级到 GTK 4.14+/WebKitGTK 的
  发行版明确退出支持边界；Secret Service fail-closed 边界不变
- Verification: 产品所有者 2026-09-08 批准三项决策；`release-target-matrix.md` 的
  5 个 required rows 全部携带决策引用；`npm run check:migration-docs` 通过
- Platforms covered: release governance only; no new platform evidence claimed
- Evidence grade: `C`
- Decision references: `WV3-003`, `WV3-007`, `WV3-008`, `WV3-011`, `WV3-012`, `WV3-013`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: none: release governance task only
- Documentation updated: decisions, release target matrix, verification gates,
  implementation plan, README and ledger
- Residual risks: 新 floor 需要在 Phase 6/P8 按 section 5 认证 profiles 取得 grade A
  证据；README 的 Windows ARM64 声明漂移仍待独立文档修复；P0-01/P0-02/P0-03/P0-05
  的三平台证据仍开放
- Next safe slice: complete P0-01/P0-02/P0-03/P0-05 three-platform and formal evidence,
  then close the Phase 0 exit gate before P1-01
- Drift decision: `accepted-docs-only`

## WV3-L013 - 2026-09-08 - P1-01 shell-neutral RuntimeClient

- Capability rows: all rows; no status advancement
- Plan task: `P1-01`
- Status change: `not-started -> not-started`
- Scope change: `none`
- Goal: 将 NetcattyBridge 表面拆分为 shell-neutral 域端口，使 application/UI 与
  shell 解耦，且 Wails adapter 可以实现同一契约而不改变任何 Electron 行为。
- Go canonical owner: none; this slice is frontend TypeScript only
- Frontend adapter: `infrastructure/runtime/` ports generated from the P0-01
  contract fixtures (9 ports, 482 methods, coverage and disjointness enforced by
  `generate:runtime-ports --check`); `infrastructure/runtime/electron/` is the
  single window.netcatty access point; `netcattyBridge` becomes a transition
  facade with exact prior semantics
- Electron owner affected: none; all current owners remain unchanged
- Preserved invariants: every bridge method stays reachable with identical
  signatures; facade get/require semantics unchanged; ESLint forbids
  window.netcatty outside the Electron adapter and `@wailsio/runtime` outside
  the future Wails adapter
- Data/schema impact: none
- Security impact: no new bridge surface; boundary rules reduce future shell
  coupling
- Verification: `node --test --import tsx infrastructure/runtime/runtimeClient.test.ts`
  (5 tests); `npm run check:migration-electron-baseline` extended with the
  runtime-ports drift check and wired into CI; `npm run lint` passes with the
  new boundary rules; application-wide `tsc --noEmit` shows no errors in the
  new modules
- Platforms covered: platform-independent TypeScript contract layer
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-002`, `WV3-008`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: the facade and transitionBridge are
  deleted with the Electron path once port-by-port consumer migration completes
- Documentation updated: README, implementation plan and ledger
- Residual risks: P0-01/P0-02/P0-03/P0-05 three-platform evidence is still
  pending; the Phase 0 exit gate has not closed. Phase 1 contract work proceeds
  under the user-approved full-implementation directive with the
  migration-evidence CI workflow collecting the outstanding evidence; the Wails
  RuntimeClient adapter and production Go skeleton start at P1-02
- Next safe slice: P1-02 production Go module and Wails skeleton
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L014 - 2026-09-08 - P1-02 production Go module and Wails skeleton

- Capability rows: `FND-01`
- Plan task: `P1-02`
- Status change: `probe -> implemented`
- Scope change: `none`
- Goal: 建立生产 Go owner 根目录与可测试的 Wails facade：health/version/
  window-role 三个 shell-neutral use case，加载现有 Vite build，不复制 React
  源码；Electron 仍是默认 dev/release。
- Go canonical owner: `internal/app`（shell-neutral use cases，禁止 import
  Wails）+ `cmd/netcatty`（唯一 Wails facade）+ `internal/platform`
- Frontend adapter: `infrastructure/runtime/wails/` fail-closed adapter 使用
  生成的 typed bindings（`@wailsio/runtime` 3.0.0-beta.12）；bootstrap 按
  shell 自动选择 runtime；未迁移端口在调用时显式拒绝
- Electron owner affected: none; Electron remains the default release carrier
  and the facade semantics are unchanged
- Preserved invariants: internal/app 不 import Wails；单一 runtime 选择
  bootstrap；Electron adapter 行为不变（node 下保持 refuse-to-install）；
  transition bridge 在 Wails 下 fail-closed
- Data/schema impact: none; skeleton owns no user data
- Security impact: 未迁移能力调用时 fail-closed，不静默 fallback；绑定仅暴露
  3 个方法
- Verification: `go test ./internal/...`、`go vet ./cmd/... ./internal/...`、
  `go build ./cmd/netcatty`（占位资产）本地通过；`npm run wails:build` 链路
  （vite build -> prepare-frontend -> go build）在 CI 三平台 job 执行；
  `node --test --import tsx infrastructure/runtime/*.test.ts` 9 项通过；
  ESLint 边界（仅 Wails adapter 可 import @wailsio/runtime）通过
- Platforms covered: Windows 10 22H2 x64 build 19045 local build; three-platform
  build/launch smoke delegated to migration-evidence CI (renderer launch smoke
  still pending everywhere)
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-002`, `WV3-003`, `WV3-011`, `WV3-012`, `WV3-013`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Electron remains the default until
  P8-02; `FND-01` retirement begins only after three-platform launch evidence
  and P8-01 RC
- Documentation updated: README, capability matrix, implementation plan and ledger
- Residual risks: renderer launch smoke（真实 bundle 在 Wails 壳中启动）未在任何
  平台执行；479 个端口方法仍未迁移；三平台 launch 证据缺失
- Next safe slice: P1-03 base contracts with Go-to-TS codegen and drift check
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L015 - 2026-09-08 - P1-03 base contracts with Go-to-TS codegen

- Capability rows: all rows; no status advancement
- Plan task: `P1-03`
- Status change: `not-started -> not-started`
- Scope change: `none`
- Goal: 建立所有后续 service contract 复用的统一基础类型：opaque 身份、稳定
  错误码、request/subscription envelope 与 JSON 边界策略，并以 Go 为单一
  source of truth 生成 TS 声明。
- Go canonical owner: `internal/app/contracts/`（纯契约，无 shell 依赖）；
  `tools/contracts-codegen` 为 codegen owner
- Frontend adapter: `infrastructure/runtime/contracts/` 消费生成的声明并提供
  `toServiceError` 错误/取消映射
- Electron owner affected: none; the base contracts add no bridge methods
- Preserved invariants: 错误码永不改名/复用（AST 提取保证 union 同步）；IDs
  前缀+长度封闭可验证；JSON 边界在唯一入口 Encode/Decode 强制；未知字段
  fail-closed
- Data/schema impact: golden fixtures under testdata/migration/contracts only
- Security impact: safe-integer/size/depth/unknown-field policy 缩小后续
  service 面；错误 envelope 不泄漏 internals（unknown → netcatty.internal）
- Verification: `go test ./internal/app/contracts/`（IDs、错误映射、JSON
  策略、golden 写入/漂移检测）；`go run ./tools/contracts-codegen --check`
  字节级 drift gate；TS 跨语言测试消费同一 fixtures（base64 payload、ID
  形状、code union、错误映射语义）；`npm run lint`、全应用 tsc 过滤无新错误
- Platforms covered: platform-independent contract layer
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-002`, `WV3-008`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: none: base contracts only; existing owners unchanged
- Documentation updated: README, implementation plan and ledger
- Residual risks: terminal byte frame 与 plugin public contract 刻意不在本层；
  Electron fixtures 的错误/取消映射在 P2-05/P2-06 迁移 broker 时做端到端验证
- Next safe slice: Phase 2 Batch B, P2-01 persistence key and Electron-main
  data inventory
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L016 - 2026-09-08 - P2-01 persistence data inventory

- Capability rows: all rows; no status advancement
- Plan task: `P2-01`
- Status change: `not-started -> not-started`
- Scope change: `none`
- Goal: 冻结 storageKeys.ts 全部 178 个唯一 key 与 Electron-main 持久文件的
  四分类清单（canonical-migrated / device-local / transient-cache / retired），
  使 Gate 6 的"无未分类数据"成为机器强制。
- Go canonical owner: none; inventory task only
- Frontend adapter: none
- Electron owner affected: none; current owners remain unchanged
- Preserved invariants: 分类即迁移真值；retired/transient/device-local 永不进
  sync payload；secret-bearing keys 显式枚举并须由 P2-04 provider 转封
- Data/schema impact: `data-inventory.md` + `testdata/migration/electron/data-inventory.json`
  generated from the frozen P0-01 fixture (110/57/9/2 classification split)
- Security impact: 11 个 secret-bearing keys（hosts/keys/identities/proxy
  profiles/group configs/default passphrases/http proxy/AI providers/AI agents/
  AI web search/legacy records）成为 P2-04/P2-05 的输入；session logs 与 CLI
  discovery file 的敏感性已标注
- Verification: `node --test scripts/migration/data-inventory.test.mjs`
  （4 tests：全量覆盖、分类合法性、非 canonical 永不同步、secret 清单锚点）；
  `npm run check:data-inventory`（生成漂移 gate）接入 test.yml CI
- Platforms covered: platform-independent inventory
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-004`, `WV3-008`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: none: inventory task only
- Documentation updated: data-inventory.md (new), implementation plan, ledger
- Residual risks: sync-payload 精确组成由 P2-07 验证；classification 边界
  （如 sftp 偏好键）可随 P2-07 差分测试修订，修订须重生成清单并回写本 ledger
- Next safe slice: P2-01A plugin v1 user-data retention contract
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L017 - 2026-09-08 - P2-01A plugin v1 user-data retention contract

- Capability rows: all rows; no status advancement
- Plan task: `P2-01A`
- Status change: `not-started -> not-started`
- Scope change: `none`
- Goal: 在 Phase 2 profile cutover 前冻结插件 v1 全部用户数据的保留方式，
  使 v2 runtime 断代（WV3-005）不丢失任何用户数据。
- Go canonical owner: none; contract task only (future owner: P2-05/P2-06
  bundle, P5-02 store)
- Frontend adapter: none
- Electron owner affected: none; `electron/plugins/` continues to own the v1
  database today
- Preserved invariants: schema v3 全部 12 张表逐表处置；`preserve-opaque`
  行进 plugin-v1 命名空间 envelope（plugin_id 与表名构成路径，含 row_count 与
  semantic hash，Phase 2 不解释 v2 语义）；v1 代码永不执行；grants/provider
  bindings 默认失效；secrets 只走 P2-05 解封加 P2-04 转封；v2 认领需逐
  plugin 用户批准
- Data/schema impact: `plugin-v1-data-retention.md` + fixture
  `testdata/migration/electron/plugin-v1-data-retention.json`
- Security impact: no grant inheritance, no shim for main.browser/main.node,
  re-sealed secrets unusable by v1 and unclaimed v2 code
- Verification: `npm run check:plugin-retention`（4 tests：CREATE TABLE 全覆盖、
  SCHEMA_VERSION 锚点、fail-closed 属性断言、secrets/grants 处置断言）接入
  test.yml CI
- Platforms covered: platform-independent contract
- Evidence grade: `C`
- Decision references: `WV3-004`, `WV3-005`, `WV3-010`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: none: contract task only; P9-01 deletes the v1 runtime
  after cutover
- Documentation updated: plugin-v1-data-retention.md (new), implementation
  plan, ledger
- Residual risks: P2-05/P2-06 必须按本合同实现 bundle 与 equality 检查；
  schema v3 之后新增表须先过本 check
- Next safe slice: P2-02 Go transactional profile store
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L018 - 2026-09-08 - P2-02 Go transactional profile store

- Capability rows: `FND-02`
- Plan task: `P2-02`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: 建立 host 拥有的事务性 profile store：raw value 兼容、revision/CAS、
  原子事务、崩溃恢复、staging/备份/回执，作为后续所有持久化迁移的 canonical
  owner 根基。
- Go canonical owner: `internal/profile/store`（bbolt v1.4.3，纯 Go，无 cgo）
  暴露为 `cmd/netcatty` ProfileService（6 方法，绑定已再生成）
- Frontend adapter: `infrastructure/runtime/profile/profileClient.ts`（base64
  线协议、文本/JSON 助手、absent-key → undefined 契约）
- Electron owner affected: none; renderer persistence stays on localStorage
  until P2-07
- Preserved invariants: 每次变更单事务原子提交；revision 单调且 CAS 冲突
  fail-closed；封闭域与 key/值边界（1 MiB 内核级 4 MiB 值上限）；promote 前
  崩溃 target 不变、不完整 staging 拒晋升、备份 manifest + receipt 落盘
- Data/schema impact: 新 store 文件格式（schema version 1）；不触碰现有用户数据
- Security impact: store 文件 0600、目录 0700；无 secret 语义（secrets 在
  P2-04 provider 层）
- Verification: `go test -count=1 ./internal/profile/...`（12 tests：round
  trip、bounds、CAS、原子性、通知、reopen、并发串行化、staging/promote、
  崩溃矩阵）；`go test -race -count=1`；`go vet`；TS profileClient 测试 3 项；
  全部接入 `check:profile-store` 与 test.yml
- Platforms covered: Windows 10 22H2 x64 本地；三平台 CI 构建在
  migration-evidence workflow
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-004`, `WV3-008`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: renderer localStorage 退役发生在
  P2-07；store 层无直接 retirement
- Documentation updated: capability matrix, implementation plan, ledger (FND-02 held at probe by the transition state machine; P2-03 advances it)
- Residual risks: 跨平台 crash-consistency 声明目前只有 Windows 本机 + race
  测试；P2-03 writer lease 与 P2-07 差分测试未落地；bbolt 版本升级需重跑
  crash matrix
- Next safe slice: P2-03 cross-shell profile writer lease and coordination
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L019 - 2026-09-08 - P2-03 cross-shell writer lease and coordination

- Capability rows: `FND-02`
- Plan task: `P2-03`
- Status change: `probe -> implemented`
- Scope change: `none`
- Goal: 任意时刻只允许一个 shell 写 profile：OS 原生活性信号 + 持久 epoch
  fencing，超时只用于拒绝而非判死，Electron 与 Wails 共用同一锁权威。
- Go canonical owner: `internal/profile/coordination`（flock + epoch + lease）
  与最小 broker `cmd/netcatty-profile-broker`（JSONL stdio）
- Frontend adapter: `electron/bridges/profileLeaseBroker.cjs`（spawn helper，
  供迁移/双壳写入前取锁；非随 Electron 发行物分发）
- Electron owner affected: none yet; P2-07 将 Vault import/sync apply/key
  rotation 的写路径迁到 CoordinationPort 后旧 Web Lock/storage event owner 退役
- Preserved invariants: 活性判定不依赖超时（flock 随进程死亡释放）；epoch
  单调持久使 stale writer 可被 fence；lease 过期但持锁存活时冲突拒绝而非抢占；
  release 后 epoch 不回退
- Data/schema impact: 三个辅助文件（writer.lock/epoch/lease）与 profile 同目录
- Security impact: 文件 0600/目录 0700；broker 仅接受本地 env 指定的 profile
  路径，无网络面
- Verification: `go test -count=1 ./internal/profile/coordination/`（5 tests：
  冲突与释放、崩溃接管 epoch 单调、续期扩展与过期拒绝、过期但存活拒抢、
  双重获取拒绝）；`go test -race`；`go vet`；broker 构建 + Electron 适配器
  端到端 smoke（acquire/renew/status/release）；全部在
  `check:profile-store` 覆盖范围内
- Platforms covered: Windows 10 22H2 x64（gofrs/flock 跨平台原语；三平台 CI
  构建于 migration-evidence workflow）
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-004`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 旧 storage event/Web Lock owner 在
  P2-07 逐域退役；broker helper 随 P9-01 与 Electron runtime 一起删除
- Documentation updated: capability matrix, implementation plan, ledger
- Residual risks: 双进程争用集成测试（Electron 真进程 + Wails 同时启停）在
  P2-05/P2-06 迁移链路中补；epoch fencing 在 store 写路径的强制校验于
  P2-07 接线时启用
- Next safe slice: P2-04 platform credential providers
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L020 - 2026-09-08 - P2-04 platform credential providers

- Capability rows: `FND-03`
- Plan task: `P2-04`
- Status change: `probe -> implemented`
- Scope change: `none`
- Goal: 建立 Go credential owner：Windows Credential Manager、macOS Keychain、
  Linux Secret Service 统一抽象，purpose-bound envelope，keyring unavailable
  时 fail-closed，禁止 plaintext fallback。
- Go canonical owner: `internal/platform/credentials`（go-keyring v0.2.6 +
  AES-256-GCM provider）与 `cmd/netcatty` CredentialService facade
- Frontend adapter: Wails generated `CredentialService` bindings；P2-05
  migration broker 消费 `Seal/Open`
- Electron owner affected: none yet; Electron safeStorage 仍是 P2-05 的 source
  unseal owner
- Preserved invariants: 每个 purpose 独立随机 key（keyring service/user
  namespace）；fresh nonce；purpose AAD；version/provider/purpose metadata
  完整绑定；tamper、cross-purpose replay、损坏 envelope、不可用 keyring、
  空/超大 plaintext 全部 fail-closed；内存 buffer 在 provider 边界清零
- Data/schema impact: 新 JSON envelope v1（version/provider/purpose/nonce/
  ciphertext），不会直接兼容 Electron `enc:v1:`，由 P2-05 负责转换
- Security impact: 无 keyring 时不降级明文；provider key 不返回给 renderer；
  facade 只返回 sealed envelope 或短生命周期 plaintext result
- Verification: `go test -race ./internal/platform/credentials/...`、`go vet`；
  4 个测试覆盖 roundtrip/purpose replay/nonce/tamper/malformed/unavailable/
  bounds/per-purpose isolation；`check:credentials` 接入 test.yml；Wails
  skeleton build 3 services/13 methods 通过
- Platforms covered: Windows 10 22H2 x64 本机接口/race；Windows Credential
  Manager、macOS Keychain、Linux Secret Service live smoke 由
  migration-evidence 三平台 job 负责，尚未取得本地 A 级证据
- Evidence grade: `C`
- Decision references: `WV3-002`, `WV3-004`, `WV3-007`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: P2-05/P2-06 完成 source unseal、转封、
  atomic cutover 与 rollback 后，Electron credential owner 才断开
- Documentation updated: capability matrix, implementation plan, ledger
- Residual risks: 三平台真实 keyring availability/locked negative smoke 待 CI；
  P2-05 需要把 Electron `enc:v1:` corpus 全量映射到 purpose 命名空间，并做
  无 plaintext leak/crash matrix
- Next safe slice: P2-05 Electron migration export broker
- Drift decision: `user-approved-implementation-ahead-of-evidence`
