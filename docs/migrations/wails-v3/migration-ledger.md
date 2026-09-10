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

## WV3-L021 - 2026-09-08 - P2-05 Electron migration export broker core

- Capability rows: `FND-03`
- Plan task: `P2-05`
- Status change: `implemented -> implemented`
- Scope change: `none`
- Goal: 冻结 migration bundle schema 与 Electron export broker 安全边界：trusted
  origin、writer lease、protective backup、classified records、Electron source
  secret 只在内存解封、Wails X25519 target key 一次性加密、成功 receipt 前 source
  保持 writable。
- Go canonical owner: none; Electron broker core is transitional export owner,
  target import owner is P2-06 Go service
- Frontend adapter: `electron/bridges/profileMigrationExportBridge.cjs` trusted
  origin adapter over `profileMigrationBroker.cjs`
- Electron owner affected: credential/safeStorage and profile persistence remain
  unchanged; no steady-state dual writer introduced
- Preserved invariants: bundle version 1，X25519/HKDF-SHA256/AES-256-GCM，purpose
  AAD，raw/secret classification manifest，secret plaintext absent from returned
  encrypted bundle，lease release in finally，export one-shot
- Data/schema impact: encrypted bundle contains source fingerprint, backup
  manifest, record/secret counts, ephemeral public key, nonce and ciphertext；不
  写 plaintext temp file
- Security impact: untrusted origin rejected；target key must be X25519；malformed
  record/classification、missing backup、duplicate export、read failure all fail
  closed；lease release is guaranteed on error
- Verification: 4 broker tests（roundtrip decrypt、manifest/secret count、input
  validation、once-only export、release-on-error）；trusted adapter tests；P0-04
  Electron secret corpus remains the source unseal evidence
- Platforms covered: platform-independent Node broker core; Windows DPAPI source
  unseal evidence from WV3-L011; macOS/Linux source/provider live evidence pending
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-004`, `WV3-007`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: P2-06 verified import/promote + rollback
  replaces this export path; P9-01 deletes the bridge
- Documentation updated: ledger
- Residual risks: source reader must be wired to every P2-01 classified key and
  P2-01A plugin envelope；full Electron shutdown/renderer attack/crash matrix
  remains P2-05 integration work
- Next safe slice: P2-06 Wails import/verify/promote
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L022 - 2026-09-08 - P2-06 Wails import/verify/promote core

- Capability rows: `FND-03`
- Plan task: `P2-06`
- Status change: `implemented -> implemented`
- Scope change: `none`
- Goal: Go 侧先验证加密 bundle（fingerprint、X25519、HKDF/AES-GCM、record count、
  manifest/secret count），再将 secret 交给 P2-04 provider 转封，生成 staging
  profile，最后原子 promote + backup manifest + receipt；验证失败不得触碰 target。
- Go canonical owner: `internal/profile/migration` + `cmd/netcatty` ProfileMigrationService
- Frontend adapter: regenerated Wails bindings（4 services / 15 methods / 7 models）
- Electron owner affected: none until P2-07 writer/persistence cutover
- Preserved invariants: source fingerprint mismatch fail-closed；unknown JSON fields
  拒绝；provider unavailable 时 secret record 不导入；raw record 不要求 provider；
  staging completion marker 和 profile store atomic promotion
- Data/schema impact: P2-06 consumes bundle v1 and emits profile store schema v1
  plus migration receipt; reverse export/rollback remains to be wired
- Security impact: target private key only held by Wails process；provider sealed
  envelope purpose uses the fixed `profile-migration/` prefix plus the validated
  domain and key; plaintext 仅在 import call 内存生命周期存在并清零
- Verification: Go import tests 4 cases（raw+secret re-seal、fingerprint/malformed、
  provider unavailable、store-compatible mutations）；Go vet/build 全量通过；Wails
  skeleton build 通过
- Platforms covered: Windows 10 22H2 x64 build; cross-platform keyring/import smoke
  pending migration-evidence CI
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-004`, `WV3-007`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: P2-07 renderer persistence and reverse
  rollback integration; no Electron path deleted yet
- Documentation updated: capability matrix, ledger
- Residual risks: P2-05 source reader/full bundle integration、reverse export/rollback、
  crash matrix、三平台 provider evidence and semantic equality remain open
- Next safe slice: P2-07 settings/Vault/session restore persistence cutover
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L023 - 2026-09-08 - P2-07 host-backed persistence transition adapter

- Capability rows: `SYNC-01`
- Plan task: `P2-07`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: 为 settings/Vault/session restore 建立不破坏同步 render-time API 的
  host-backed transition：Electron 保持 localStorage 行为，Wails bootstrap 配置
  ProfileClient 并异步镜像 Go profile store。
- Go canonical owner: P2-02 `internal/profile/store` + Wails ProfileService；
  当前仍是 transition mirror，不宣称 renderer canonical cutover
- Frontend adapter: `infrastructure/persistence/hostStorageAdapter.ts` +
  `infrastructure/runtime/profile/profileClient.ts`；Wails `bootstrap.ts` 接线
- Electron owner affected: none in stable Electron；localStorage 继续 canonical
  until per-domain cutover evidence
- Preserved invariants: 现有同步 read/write/remove API、Quota/serialization
  语义、Electron storage event 行为不改；Wails 写入异步、pending flush 可等待；
  profile failures 不会静默抹掉 local cache
- Data/schema impact: settings domain raw base64 mirror；session restore storage
  可直接复用 adapter；Vault domain 尚未切 canonical
- Security impact: host store 只接收 opaque raw values；secret-bearing fields
  仍由 P2-04/P2-05 provider/broker 处理，不在 renderer adapter 解封
- Verification: hostStorageAdapter tests 2 项；全应用 TypeScript 检查无新错误；
  Wails bootstrap/adapter 类型通过；Go profile/store 与 migration tests 通过
- Platforms covered: platform-independent frontend transition; Wails/Go build
  Windows 本机通过，三平台 CI 继续收集
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-004`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Settings/Vault/session restore 各域差分
  suite + revision/CAS 多窗口证据通过后逐域退役 localStorage/Web Lock owner
- Documentation updated: capability matrix, implementation plan, ledger
- Residual risks: 仍有大量直接 localStorageAdapter consumer；异步 host mirror
  的跨窗口 ordering、quota/error 映射和 crash recovery 需 P2-07 后续 child
  slices；SYNC-01 不能标 verified/migrated
- Next safe slice: P2-07 child slice: settings domain differential cutover
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L024 - 2026-09-08 - P3-01 production terminal frame codec

- Capability rows: `TERM-01`
- Plan task: `P3-01`
- Status change: `probe -> implemented`
- Scope change: `none`
- Goal: 将 P0-03 已验证的 terminal binary frame v2 contract 提升为 production
  `internal/terminal/dataplane` codec owner，不复制 probe 的 Wails/WebSocket
  orchestration，也不改变 Electron MessagePort owner。
- Go canonical owner: `internal/terminal/dataplane/frame.go`
- Frontend adapter: none yet; WebView transport orchestration follows after
  codec differential gate
- Electron owner affected: none; MessagePort remains release baseline
- Preserved invariants: magic NTDP、version 2、40-byte header、128 KiB payload
  bound、generation/sequence/credit/correlation/timestamp fields、8 allowed
  frame kinds、zero reserved bytes、strict payload length
- Data/schema impact: production Go codec only; probe remains disposable test
  source and no user data is touched
- Security impact: bounded parser rejects malformed magic/version/kind/length;
  fuzz target ensures arbitrary input does not panic
- Verification: Go unit tests + race + vet；fuzz seed/target；Node differential
  test confirms production codec constants match P0-03 probe exactly；
  `check:terminal-dataplane-core` passes
- Platforms covered: platform-independent codec; WebView/WS three-platform
  evidence remains in migration-evidence workflow
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: P3-01 route orchestration + Gate 3
  paired benchmarks; probe deleted only after production candidate is accepted
- Documentation updated: capability matrix and ledger
- Residual risks: no production WebSocket listener/credit controller/rebind
  service yet; three-platform paired Electron envelope and 30-round evidence
  still pending
- Next safe slice: P3-01 production WebSocket route service and credit controller
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L025 - 2026-09-08 - P3-01 production route controller

- Capability rows: `TERM-01`
- Plan task: `P3-01`
- Status change: `implemented -> implemented`
- Scope change: `none`
- Goal: 在 binary codec 之上建立无 transport 依赖的 production route owner，
  固定 token authentication、generation fencing、credit admission 与 rebind
  基础语义。
- Go canonical owner: `internal/terminal/dataplane/route_controller.go`
- Frontend adapter: none; WebSocket listener/urgent route next child slice
- Electron owner affected: none; Electron MessagePort remains baseline
- Preserved invariants: 32-byte hex one-use route tokens、generation stale reject、
  initial 1 MiB credit、applied sequence monotonic、output admission never超过
  available credit、route replacement generation increment
- Data/schema impact: shell-neutral RouteBootstrap/RouteController types only
- Security impact: constant-time token compare、route/session/generation binding、
  bounded output admission
- Verification: route controller unit tests + race + vet；tests cover token/auth,
  generation replacement, bounded credit, sequence rejection and concurrent
  admission；codec + differential check remains green
- Platforms covered: platform-independent Go controller
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: WebSocket transport + Gate 3 paired
  benchmarks; no Electron owner retired
- Documentation updated: implementation plan and ledger
- Residual risks: WebSocket listener Host/Origin enforcement, drain/rebind,
  urgent channel and three-platform benchmark still pending
- Next safe slice: P3-01 authenticated WebSocket transport integration
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L026 - 2026-09-08 - P3-02 Go local PTY lifecycle owner

- Capability rows: `TERM-02`
- Plan task: `P3-02`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: 将 local PTY lifecycle 从 node-pty owner 拆为 Go session owner，固定
  start/resize/input/interrupt/close/reconnect、generation fencing 和 process
  reap 语义。
- Go canonical owner: `internal/terminal/pty` session contract + Unix
  creack/pty v2.0.1 backend
- Frontend adapter: none yet; Wails terminal control wiring is next child slice
- Electron owner affected: none; node-pty remains Electron release baseline
- Preserved invariants: TERM=xterm-256color/truecolor defaults、shell/cwd/env
  policy、80x24 defaults、stale generation reject、close/reconnect kill+wait；
  invalid zero resize fail-closed
- Data/schema impact: shell-neutral Config/Event/Process interfaces only
- Security impact: cwd invalid falls back home；no pipes-only Windows fallback
  that would falsely claim PTY semantics
- Verification: lifecycle/fake backend tests、generation/reconnect、bounds、
  `go test -race`、`go vet`；Unix backend compiles against creack/pty；Windows
  backend explicit ErrUnsupported until native ConPTY child slice；状态按
  状态机保持 probe，Windows ConPTY + 接线证据到位后由后续 ledger 升级
- Platforms covered: Windows 10 22H2 x64 contract tests; Unix backend build
  verified by Go package compilation; ConPTY live evidence absent
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Windows ConPTY + Wails terminal wiring,
  shell matrix, resize/Unicode/Ctrl-C and child cleanup evidence; node-pty remains
  until that gate
- Documentation updated: capability matrix, implementation plan, ledger
- Residual risks: native Windows ConPTY adapter、PTY data-plane integration、
  process-tree/job cleanup and three-platform live shell matrix remain
- Next safe slice: P3-02 native Windows ConPTY adapter and Wails control facade
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L027 - 2026-09-08 - P3-03 SSH authentication and dial core

- Capability rows: `SSH-01`
- Plan task: `P3-03`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: 建立 Go SSH 认证与 dial 核心：known-hosts policy、多策略认证、jump 链
  transport，供 P3-04 共享 pool 调用；不创建业务私有连接池。
- Go canonical owner: `internal/terminal/ssh`（hostkeys/auth/dial）
- Frontend adapter: none yet; P3-04 pool 与 Wails terminal 接线后续切片
- Electron owner affected: none; `sshBridge.cjs`/ssh2 patches remain baseline
- Preserved invariants: host key 首见 pin、变更 fail-closed（OpenSSH 文件格式
  持久化）；每跳 host key policy 必填；认证优先级 key → password →
  keyboard-interactive（MFA challenge 回调）；jump 链任一跳失败即关闭已建立
  跳；Transport.Close 顺序关闭全部跳
- Data/schema impact: known-hosts 文件 0600 追加写
- Security impact: nil policy fail-closed；私钥 passphrase 解析失败不重试明文；
  keepalive 独立 goroutine 随 stop channel 退出
- Verification: `go test -count=1 ./internal/terminal/ssh/`（known-hosts
  pin/变更/persist、auth 方法顺序与无效 PEM、跨实例 policy）；`go vet`；live
  MFA/proxy/jump/agent/certificate 证据 pending
- Platforms covered: platform-independent Go core; live SSH server evidence
  absent on this host
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: P3-04 pool + P3-04A session integration
  + Gate 4 compatibility lab 通过后才退役 ssh2 patches
- Documentation updated: capability matrix, implementation plan, ledger
- Residual risks: agent/certificate/agent-forwarding、proxy dial 原语、ssh2
  legacy algorithm 逐项 decision、Gate 4 live MFA/jump/proxy 证据全部待后续
  child slices
- Next safe slice: P3-04 shared SSH transport pool
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L028 - 2026-09-08 - P3-04 shared SSH transport pool

- Capability rows: `SSH-02`
- Plan task: `P3-04`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: 建立唯一共享 SSH transport pool：immutable 兼容 key、single-flight
  dial、引用计数 lease、健康回收、idle TTL/LRU、统一 shutdown；禁止业务域私建
  连接池。
- Go canonical owner: `internal/terminal/sshpool`（dial 函数注入，对接 P3-03）
- Frontend adapter: none yet; P3-04A session 集成与 SFTP/transfer/forwarding
  各域接入后续切片
- Electron owner affected: none; `sshConnectionPool.cjs` remains baseline
- Preserved invariants: 兼容 key 含 auth 指纹（哈希）与 jump 链，auth/host/
  jump 任一变化即分线；一条 transport 合法承载多个并发 channel（引用计数）；
  Discard 立即关闭防止 unhealthy 复用；TTL/LRU 只驱逐 outstanding=0
- Data/schema impact: none
- Security impact: auth material 只进哈希；race 测试覆盖并发 single-flight
- Verification: `go test -count=1 ./internal/terminal/sshpool/`（6 tests：key
  敏感性、共享/single-flight、并发 32 单拨、TTL 驱逐 + shutdown、auth 变更
  分线）、`go test -race`、`go vet`；live one-auth network trace 证据 pending
- Platforms covered: platform-independent Go core; live SSH server evidence
  absent on this host
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: P3-04A/SFTP/transfer/forwarding 全部
  接入本 pool 且 one-auth 网络证据通过后，`sshConnectionPool.cjs` 才退役
- Documentation updated: capability matrix, implementation plan, ledger
- Residual risks: agent-forwarding 非对称 policy 待 P3-04A 施加；live one-auth
  证据与真实网络 property 测试待后续
- Next safe slice: P3-04A SSH session integration over the shared pool
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L029 - 2026-09-09 - P3-01 authenticated WebSocket transport

- Capability rows: `TERM-01`
- Plan task: `P3-01`
- Status change: `implemented -> implemented`
- Scope change: `none`
- Goal: 建立生产 authenticated loopback WebSocket transport：127.0.0.1 绑定、
  精确 Host、显式 Origin 白名单、one-use token 子协议、credit 流控、分片输出、
  urgent ACK、stale generation 清理。
- Go canonical owner: `internal/terminal/dataplane/server.go`/`handlers.go`
  （对接 RouteController，无业务依赖）
- Frontend adapter: TS frame/credit adapter 属下一子切片
- Electron owner affected: none; MessagePort remains baseline
- Preserved invariants: one-use 64-hex token（constant-time 比对）、generation
  失配断连、初始 1 MiB credit 精确窗口、无信不发送（chunk 保留重试）、Publish
  按 128 KiB 帧上限分片、入站 SetReadLimit(MaxFrameBytes)、reader 退出即关 queue
  与连接
- Data/schema impact: none
- Security impact: coder/websocket 库内 origin 检查改由 authorize() 显式白名单
  承担（InsecureSkipVerify 仅关闭 pattern 检查，鉴权边界保留）
- Verification: 5 个集成测试（Host/Origin/token 拒绝 403/401、credit 窗口流控与
  超窗拒绝、分片 1 MiB 投递、urgent handler+ACK correlation、rebind 后旧代连接
  关闭）；`go test -race`、`go vet` 全绿
- Platforms covered: Windows 10 22H2 x64 loopback; WKWebView/WebKitGTK paired
  benchmark evidence pending
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Gate 3 paired benchmarks + xterm adapter
  接线后才退役 MessagePort
- Documentation updated: ledger
- Residual risks: TS frame/credit adapter、PTY→Publish 接线、drain marker 完整
  流程、三平台 paired benchmark 与 30-round formal evidence
- Next safe slice: P3-01 TS frame/credit adapter + differential fixtures
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L030 - 2026-09-09 - P3-02 native Windows ConPTY adapter and Wails facade

- Capability rows: `TERM-02`
- Plan task: `P3-02`
- Status change: `probe -> implemented`
- Scope change: `none`
- Goal: Windows 真实 ConPTY 后端与 Wails PTY control facade，使本地终端在
  Wails 壳内可运行（而非显式 unsupported）。
- Go canonical owner: `internal/terminal/pty` Windows backend 改用
  UserExistsError/conpty v0.1.4；`cmd/netcatty` PTYService facade
- Frontend adapter: 再生成 Wails bindings（5 services / 20 methods）
- Electron owner affected: none; node-pty remains Electron baseline
- Preserved invariants: generation fencing（resize/write/interrupt 按 generation
  拒绝）、close/reap 契约、cwd/shell/env 默认策略；手写 CreatePseudoConsole
  实现因 0xC0000142（STATUS_DLL_INIT_FAILED，x/sys attribute/handle 顺序差异）
  被验证过的库替换——记录为兼容性决策而非 fallback
- Data/schema impact: none
- Security impact: job/ConPTY 生命周期由库管理；Ctrl+C 以 0x03 写入 PTY 输入
- Verification: 本机 live smoke（cmd.exe ConPTY：banner、echo
  NETCATTY_CONPTY_OK、resize 100x30→120x40、close 清理、reader 终止）+ race +
  vet；Wails skeleton build 与 bindings 再生成通过；剩余 live 矩阵（PowerShell/
  WSL/WSL 原生、Unicode、resize flood、reload）待后续 child slice
- Platforms covered: Windows 10 22H2 x64 live；Unix backend 编译验证；macOS/
  Linux live pending CI
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: PTY 输出接 P3-01 data plane + shell
  矩阵证据后，node-pty/terminal worker 退役
- Documentation updated: capability matrix, implementation plan, ledger
- Residual risks: PTY→data-plane 接线未完成；shell 矩阵/Unicode/resize flood/
  多进程树清理 live 证据 pending
- Next safe slice: P3-02 PTY output into the terminal data plane
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L031 - 2026-09-09 - P3-03 SSH agent, proxy and ssh2 compat decisions

- Capability rows: `SSH-01`
- Plan task: `P3-03`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: 补齐 P3-03 剩余 dial primitives：ssh-agent 认证（Windows named pipe +
  SSH_AUTH_SOCK）、agent forwarding 原语、socks5/http proxy dial、ssh2 patch
  逐项兼容决策。
- Go canonical owner: `internal/terminal/ssh/agentauth.go`（agent/proxy/
  forwarding）+ `dial.go` ProxyURL 注入
- Frontend adapter: none yet; P3-04A 接线
- Electron owner affected: none; ssh2 patches remain the Electron baseline
- Preserved invariants: agent 不可达 fail-closed（ErrAgentUnavailable）；proxy
  URL 解析失败 fail-closed；ForwardAgentToClient 仅绑定本地 agent；auth 指纹
  不含 agent 内容
- Data/schema impact: `docs/migrations/wails-v3/ssh2-compat-decisions.md`
  固化 7 个 patch 区域的 Go 决策（2 native test-pinned、1 native、1 deferred
  P3-05、3 pending compatibility lab）
- Security impact: Comware/legacy DHGEX 类老设备今天 fail-closed，不做静默
  降级；Gate 4 实验室必须为 pending 行取证后 ssh2 patches 才可退役
- Verification: `go test -count=1 ./internal/terminal/ssh/`（新增 agent 不可达
  fail-closed、proxy 校验/死代理、RSA 证书 AlgorithmSigner sha2 能力 pin）+
  race-free；`go vet`
- Platforms covered: Windows named pipe agent 路径按 runtime 分支编译；live
  agent/证书服务器证据 pending Gate 4
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: pending compatibility lab 行取证后，
  ssh2+1.17.0.patch 随 P3-04A/Gate 4 退役
- Documentation updated: ssh2-compat-decisions.md (new), ledger
- Residual risks: live MFA/jump/proxy/agent 服务器证据；SFTP header-spanning
  行为在 P3-05 用 pkg/sftp 验证
- Next safe slice: P3-04 agent-forwarding asymmetric reuse policy
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L032 - 2026-09-09 - P3-04 agent-forwarding asymmetric reuse policy

- Capability rows: `SSH-02`
- Plan task: `P3-04`
- Status change: `probe -> implemented`
- Scope change: `none`
- Goal: 施加 agent-forwarding 非对称复用 policy：开启 ForwardAgent 的
  transport 单次使用，绝不入池复用。
- Go canonical owner: `internal/terminal/sshpool`（singleUse 标记 + Return
  关闭）与 `internal/terminal/ssh` DialConfig.ForwardAgent
- Frontend adapter: none; P3-04A session 集成消费
- Electron owner affected: none
- Preserved invariants: ForwardAgent 参与兼容 key；单次 lease Return 即关闭
  transport；非 forwarding 路径行为不变
- Data/schema impact: none
- Security impact: 持本地 agent 通道的连接不跨 lease 暴露给其他会话/域
- Verification: `go test -race -count=1 ./internal/terminal/sshpool/`（新增
  forwarding 单次使用 + key 区分 2 tests；全套 key 敏感性/共享/single-flight/
  TTL/shutdown 保持通过）
- Platforms covered: platform-independent Go core
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: P3-04A/各域接入 + one-auth 网络证据
- Documentation updated: ledger
- Residual risks: live one-auth 证据、P3-04A session 集成
- Next safe slice: P3-04A SSH session integration over the shared pool
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L033 - 2026-09-09 - P2-05/P2-06 source reader, leak scan and reverse rollback

- Capability rows: `FND-03`
- Plan task: `P2-05`, `P2-06`
- Status change: `implemented -> implemented`
- Scope change: `none`
- Goal: 把迁移链从 schema 落到数据面：按 P2-01 清单读取 canonical keys、加密
  bundle 的明文/base64 泄漏扫描、Go store → Electron 的反向回滚导出、bundle
  tamper 崩溃矩阵。
- Go canonical owner: `internal/profile/migration/rollback.go`（反向导出，
  secret envelope 不解封）
- Frontend adapter: `electron/bridges/profileMigrationSourceReader.cjs`
  （P2-01 清单驱动 classification、canonical-only 过滤）+ leak scan
- Electron owner affected: none yet; source reader 通过 accessor 注入，
  credentialBridge 解封由调用方组合
- Preserved invariants: 未分类 key 抛错（drift 联动 P2-01）；device-local/
  transient/retired 不导出；secret 记录以 plaintext 进内存 payload 且 bundle
  立即加密（泄漏扫描双形态断言）；rollback bundle 与 cutover bundle 同格式、
  secret 不解封直接以 sealed envelope 回传；tamper 矩阵逐字节位翻转全部
  fail-closed；错误 fingerprint 拒绝；空 store 回滚拒绝
- Data/schema impact: rollback bundle 复用 bundle v1；无新 schema
- Security impact: 泄漏扫描成为 export 必经断言；回滚路径不增加 plaintext 面
- Verification: Node 2 tests（canonical 过滤 + 分类、泄漏扫描正/反/base64）；
  Go 3 tests（rollback→Import roundtrip、16 步 tamper 矩阵、wrong fingerprint、
  空 store）+ race；`go vet`
- Platforms covered: platform-independent core; 全 profile live 导出证据 pending
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-004`, `WV3-007`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 反向回滚是 P2-06 rollback 承诺的实现；
  Electron 持久化退役仍在 P2-07 逐域证据后
- Documentation updated: ledger
- Residual risks: 真实 Electron localStorage 全量导出的端到端运行、Electron
  端 crash/取消/renderer 攻击矩阵、双进程争用集成测试
- Next safe slice: P2-07 settings domain cutover with differential suites
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L034 - 2026-09-09 - P2-07 host revision multi-window CAS evidence

- Capability rows: `SYNC-01`
- Plan task: `P2-07`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: 以可执行证据固化 host-revision 取代 process-local stale write 防护：
  两个窗口同起点 CAS 写入，过期者被宿主拒绝。
- Go canonical owner: `internal/profile/store`（CAS 语义既有）；本条补 renderer
  契约层证据
- Frontend adapter: `infrastructure/persistence/hostStorageAdapter.test.ts`
  新增双窗口 CAS 场景
- Electron owner affected: none
- Preserved invariants: 同起点 revision、先写者胜、stale CAS 抛 revision
  conflict、宿主 revision 单调
- Data/schema impact: none
- Security impact: 跨窗口写序由宿主 revision 而非 renderer 本地状态裁决
- Verification: hostStorageAdapter tests 3 项（含新增双窗口 CAS）；Go store
  CAS/concurrency 测试保持通过
- Platforms covered: platform-independent
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-004`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 各域 canonical cutover + 差分套件后退役
  localStorage owner
- Documentation updated: ledger
- Residual risks: 真实双 WebView 进程并发（非模拟）与 quota/error 映射证据
- Next safe slice: P3-04A SSH session integration over the shared pool
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L035 - 2026-09-09 - P3-05 SFTP browsing core

- Capability rows: `SFTP-01`
- Plan task: `P3-05`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: 建立 transport-neutral SFTP browsing owner：list/stat/mkdir/rename/
  remove/read/write，路径词法规范化与受限并发 client。
- Go canonical owner: `internal/terminal/sftp`（pkg/sftp v1.13.9 客户端适配 +
  RemoteFS 接口 + Session client 上限 + NormalizePath/SortEntries）
- Frontend adapter: none yet; Wails SFTP adapter 与 P3-04A pool lease 接线后续
- Electron owner affected: none; `sftpBridge.cjs` remains baseline
- Preserved invariants: backslash 名拒绝、base 逃逸词法拒绝（server 仍为真正
  权限边界）、目录优先排序契约、client 并发上限
- Data/schema impact: none
- Security impact: 路径规范化为词法层；真实访问控制仍由远端 SFTP 服务端执行
- Verification: 进程内真实 pkg/sftp server（net.Pipe）集成测试 6 项（列表排序、
  stat/mkdir/rename/create/read/remove 全链、分块读、client 上限、路径规范化、
  部分读）+ `go vet`；live sudo/非 UTF-8/raw path 矩阵 pending
- Platforms covered: platform-independent Go core；真实服务器矩阵 pending
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: P3-04A pool 接线 + 真实服务器矩阵
  （sudo/非 UTF-8/symlink/断连）通过后退役 sftpBridge
- Documentation updated: capability matrix and ledger
- Residual risks: sudo SFTP 支持（计划要求不能满足时暂停并产品决策）、raw byte
  文件名、symlink 遍历、断连重试
- Next safe slice: P3-06 Go transfer scheduler
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L036 - 2026-09-09 - P3-06 Go transfer scheduler

- Capability rows: `SFTP-02`
- Plan task: `P3-06`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: 建立 Go transfer scheduler owner：task identity、chunk checkpoint、
  pause/resume/cancel、per-host 并发上限、聚合进度快照（UI 按快照订阅而非逐
  chunk 事件，避免 Wails 事件洪水）。
- Go canonical owner: `internal/terminal/transfer`（Source/Sink 抽象，SFTP/
  local/relay 共用同一调度器）
- Frontend adapter: none yet; Wails transfer adapter 后续接入
- Electron owner affected: none; renderer scheduler remains baseline
- Preserved invariants: task ID 唯一、chunk 边界与 offset 可续传、pause 门控
  响应 cancel、cancel 优先于 worker 失败、host 槽位并发上限、snapshot 聚合
- Data/schema impact: Progress/Chunk/TaskSpec 类型；无用户数据
- Security impact: chunk worker 60s 超时 fail-closed；ctx 取消传播到 Source/Sink
- Verification: `go test -race -count=1 ./internal/terminal/transfer/`（完成与
  聚合、重复任务拒绝、cancel 收敛、pause/resume、unknown/double-start 状态机）
- Platforms covered: platform-independent Go core
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 压缩上传 + 高 RTT/损坏实验室通过后，
  renderer scheduler 退役
- Documentation updated: capability matrix and ledger
- Residual risks: 压缩上传/extract、hash 校验、renderer 关闭存活、高 RTT 实验室
- Next safe slice: P3-07 Go port forwarding
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L037 - 2026-09-09 - P3-07 Go port forwarding manager

- Capability rows: `NET-01`
- Plan task: `P3-07`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: 建立 Go forwarding owner：local/remote/dynamic SOCKS5、绑定 0 端口回报、
  monotonic revision + subscribe/snapshot、注入式 tunnel（不依赖真实 SSH 即可
  测试生命周期）。
- Go canonical owner: `internal/terminal/forward`
- Frontend adapter: none yet; Wails forwarding adapter 与 P3-04A pool tunnel
  接线后续
- Electron owner affected: none; `portForwardingBridge.cjs` remains baseline
- Preserved invariants: 不依赖 renderer window；BindPort=0 由 OS 分配并回报实际
  端口；重复 ID/不支持类型/坏端口 fail-closed；Stop 从注册表移除并推进 revision；
  SOCKS5 握手完整读取 greeting（修复 method 字节污染）
- Data/schema impact: none
- Security impact: SOCKS5 仅支持 no-auth CONNECT（面向本机渲染进程的回环监听）
- Verification: `go test -race -count=1 ./internal/terminal/forward/`（本地转发
  字节 roundtrip、重复/类型/端口拒绝、dynamic SOCKS5 全握手 roundtrip、8 并发
  start + StopAll、订阅 revision）
- Platforms covered: platform-independent Go core; IPv6/跳板/transport-loss live
  证据 pending
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: pool tunnel 接线 + IPv6/碰撞/transport
  loss 证据通过后退役 portForwardingBridge
- Documentation updated: capability matrix and ledger
- Residual risks: SSH pool tunnel 真实接线（P3-04A 之后）、IPv6 与端口碰撞 live
  证据、remote forwarding 的服务端监听请求路径
- Next safe slice: P3-08 decomposition gate (child rows) then telnet core
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L038 - 2026-09-09 - P3-08 decomposition gate for TERM-03

- Capability rows: all rows; no status advancement
- Plan task: `P3-08`
- Status change: `not-started -> not-started`
- Scope change: `none`
- Goal: 按计划 §3.5 decomposition gate 把复合行 TERM-03 拆为 4 个 stable child
  rows（TERM-03.1 Telnet、TERM-03.2 Serial、TERM-03.3 Mosh/ET supervised
  binaries、TERM-03.4 ZMODEM/YMODEM）并登记执行卡，实施前先满足拆分要求。
- Go canonical owner: none; decomposition governance only
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: parent TERM-03 保持 required/not-started；child rows 均
  required/not-started；checker 的 composite-before-implementation 规则现在由
  真实子行满足（夹具同步移除 TERM-03 fixture 子行）
- Data/schema impact: none
- Security impact: none
- Verification: `npm run check:migration-docs` 63/63 通过；执行卡（文件边界、
  依赖、命令、平台、退役触发）登记于 implementation-plan P3-08 节
- Platforms covered: platform-independent governance
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: none: governance only
- Documentation updated: capability matrix, implementation plan, ledger, fixture
  test
- Residual risks: 4 个 child slices 的实施与三平台设备/协议证据全部待做
- Next safe slice: P3-08.1 telnet protocol owner
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L039 - 2026-09-09 - P3-08.1 telnet protocol owner

- Capability rows: `TERM-03.1`
- Plan task: `P3-08`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: 建立 Go telnet 协议 owner：IAC 转义/解析、WILL/WONT/DO/DONT 协商、
  NAWS 窗口（含 255 转义）、echo 模式追踪、auto-login 提示应答。
- Go canonical owner: `internal/terminal/telnet`（Client + readLoop 协议状态机）
- Frontend adapter: none yet; Wails telnet 接线与 P3-01 data plane 桥接后续
- Electron owner affected: none; terminal bridge telnet paths remain baseline
- Preserved invariants: 数据中 255 双写/还原；协商字节永不进数据流；DO ECHO
  回 WONT（客户端不回显）、WILL ECHO 回 DO 并翻转 remoteEcho；未知 option
  WONT/DONT 拒绝；NAWS 全帧带 SE 终止；写路径经 writeMu 串行化防止协商与
  数据交错
- Data/schema impact: Event/EventKind 类型；无用户数据
- Security impact: 无凭据明文落盘（auto-login 仅内存）；prompt 应答经认证通道
- Verification: `go test -race -count=1 ./internal/terminal/telnet/`（3 项：
  协商+NAWS 默认 80 与 resize 200、IAC 转义上线验证、auto-login
  admin/secret123 线上应答）；`go vet`
- Platforms covered: platform-independent Go core（进程内 TCP server 测试）
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 与 P3-01 data plane 桥接 + 真实设备
  矩阵通过后，terminal bridge telnet 路径退役
- Documentation updated: capability matrix and ledger
- Residual risks: TTYPE/TLS、设备矩阵与断连重连 live 证据；PTY/data plane 桥接
- Next safe slice: P3-08.2 serial protocol owner
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L040 - 2026-09-09 - P3-08.2 serial protocol owner

- Capability rows: `TERM-03.2`
- Plan task: `P3-08`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: 建立 Go serial owner：配置契约校验（baud/data bits/parity/stop bits）、
  端口枚举、open/write/close 生命周期、CloseAll 会话回收。
- Go canonical owner: `internal/terminal/serialport`（go.bug.st/serial v1.6.4
  后端 + 可注入 Backend 接口）
- Frontend adapter: none yet
- Electron owner affected: none; terminal bridge serial paths remain baseline
- Preserved invariants: 未验证配置不开设备；重复 open fail-closed；未知端口
  写/关 fail-closed；CloseAll 回收全部句柄
- Data/schema impact: none
- Security impact: 设备句柄生命周期受会话边界约束，无全局泄漏路径
- Verification: `go test -race -count=1 ./internal/terminal/serialport/`（6 项：
  配置契约、枚举/开/写/关、重复 open、缺设备 fail-closed、CloseAll、list 错误
  传播）；`go vet`；真机设备矩阵（USB-串口适配器、断连热拔）待硬件
- Platforms covered: Windows 10 22H2 x64（枚举在无设备时返回空表验证）；三平台
  真机证据待硬件
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 真机设备矩阵通过后 terminal bridge
  serial 路径退役
- Documentation updated: capability matrix and ledger
- Residual risks: 真机设备矩阵（热拔/驱动错误/流控）需硬件；parity/stop bits
  非默认组合 live 未测
- Next safe slice: P3-08.3 Mosh/ET supervised binary runner
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L041 - 2026-09-09 - P3-08.3 Mosh/ET supervised binary runner

- Capability rows: `TERM-03.3`
- Plan task: `P3-08`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: 建立经 hash/arch 校验的外部二进制监督 runner：manifest 校验（存在性、
  GOOS/GOARCH、SHA-256）、restart 预算、立即死亡 fast-fail、干净 teardown。
- Go canonical owner: `internal/terminal/supervised`
- Frontend adapter: none yet; mosh/et 会话协议层后续接入
- Electron owner affected: none; packaged binaries + bridges remain baseline
- Preserved invariants: 二进制不按文件名信任（SHA-256 必须 match manifest）；
  arch/OS 失配 fail-closed；立即退出（arch/ABI 失配症状）触发 fast-fail 与
  restart 预算；stdin pipe 保持打开防 EOF 早退；Kill 后 Wait 收敛
- Data/schema impact: Manifest JSON 类型
- Security impact: hash pinning 防替换攻击；资源清单（P6-02）复用此 Manifest
- Verification: `go test -race -count=1 ./internal/terminal/supervised/`（3 项：
  missing/wrong-arch/wrong-hash 拒绝、本机真实 cmd.exe live 运行+停止+双停
  收敛、立即死亡 cmd /c exit 1 fail-closed）；`go vet`
- Platforms covered: Windows 10 22H2 x64 live（cmd.exe）；mosh/et 真实二进制与
  三平台 reconnect 证据 pending
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: mosh/et 会话协议 parity + 资源清单接入
  （P6-02 manifest）后，packaged bridges 退役
- Documentation updated: capability matrix and ledger
- Residual risks: mosh/et 协议层 parity（reconnect/roaming）、真实 helper 二进制
  哈希清单、资源打包接入（P6-02）
- Next safe slice: P3-08.4 ZMODEM/YMODEM service
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L042 - 2026-09-09 - P3-08.4 ZMODEM/YMODEM service boundary

- Capability rows: `TERM-03.4`
- Plan task: `P3-08`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: 建立 ZMODEM 服务安全边界：CRC-16 帧解析（binary header + tamper
  拒绝）、ZFILE 元数据文件安全（traversal/分隔符/保留名/大小上限）、ctx 取消
  语义；完整 rz/sz 会话引擎后续接入。
- Go canonical owner: `internal/terminal/zmodem`
- Frontend adapter: none yet; terminal data plane 事件接线后续
- Electron owner affected: none; `zmodemHelper.cjs` remains baseline
- Preserved invariants: 文件名必须 base 名（拒绝路径/分隔符/..）、Windows 保留
  设备名拒绝、大小上限 256 MiB fail-closed、CRC 不匹配拒绝、取消立即生效
- Data/schema impact: FileMeta/Frame 类型
- Security impact: 本包是"什么允许进文件系统"的唯一权威边界
- Verification: `go test -count=1 ./internal/terminal/zmodem/`（6 项：安全名/
  危险名矩阵、大小上限、CRC 已知向量、header 合法/篡改、取消即时生效、
  big-endian CRC 组装）；`go vet`
- Platforms covered: platform-independent Go core
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 完整会话引擎 + 真实 lrzsz 对端矩阵
  通过后 zmodemHelper 退役
- Documentation updated: capability matrix and ledger
- Residual risks: 完整 rz/sz 会话引擎（ZRINIT 参数协商、32-bit CRC、escape
  编码）与真实对端（lrzsz）矩阵待做
- Next safe slice: P4-01 filesystem and dedicated temp directory service
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L043 - 2026-09-09 - P4-01 dedicated temp directory and filesystem service

- Capability rows: `SYS-01`
- Plan task: `P4-01`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: 专用临时目录服务（AGENTS 契约：一切临时文件进 Netcatty temp root）+
  symlink 安全路径解析 + zip-slip 加固解压。
- Go canonical owner: `internal/platform/filesystem`（TempService/Resolve/
  ExtractArchive）
- Frontend adapter: none yet; Wails filesystem/dialog adapter 后续
- Electron owner affected: none; tempDirBridge remains Electron baseline
- Preserved invariants: 一切临时文件在专用 root 内；词法+symlink 双重逃逸
  拒绝（悬空 symlink 仅允许作叶子）；WriteFile 自动建父目录 0700/0600；
  Clear 保 root；root 替换不串库；zip 解压总大小上限 + 条目名安全 + 逐条目
  目标路径 within-base 校验
- Data/schema impact: none
- Security impact: traversal 攻击语料（..、绝对、UNC 前缀、盘符、反斜杠）与
  zip-slip 均 fail-closed；Settings > System 可展示 Usage 并 Clear
- Verification: `go test -race -count=1 ./internal/platform/filesystem/`（7 项：
  write/read/remove+usage、逃逸拒绝、root 替换隔离、clear 保 root、悬空
  symlink 叶子/中间件语义、zip-slip 中止且无外部写入、干净解压）；`go vet`
- Platforms covered: Windows 10 22H2 x64 live（symlink 测试依赖本机特权，不可用时 skip）；UNC/长路径/native dialog 三平台 live 证据 pending
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: native dialog parenting + UNC/长路径
  live 矩阵通过后，local FS/temp bridges 退役
- Documentation updated: capability matrix and ledger
- Residual risks: native dialog parenting（需 Wails UI）、UNC/长路径/Windows
  attributes live 矩阵、archive 格式扩展（tar/7z）
- Next safe slice: P4-02 multi-window and popup terminal lifecycle
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L044 - 2026-09-09 - P4-02 Go window registry and lifecycle core

- Capability rows: `FND-04`
- Plan task: `P4-02`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: Go window registry owner：role 单实例约束、opaque capability token、
  close veto、dirty-editor guard、crash 清理、session/popup 角色围栏。
- Go canonical owner: `internal/terminal/windows`
- Frontend adapter: none yet; Wails 窗口接线（真实 BrowserWindow 生命周期、
  multi-monitor/DPI）为下一子切片
- Electron owner affected: none; Electron window manager remains baseline
- Preserved invariants: token 不匹配即窗口不可见；main/settings 单实例；
  close 顺序 = token → dirty → veto；crash 清理免 token 但不删窗口数据；
  destroy 从 role 列表移除
- Data/schema impact: none
- Security impact: 无 token 的控制调用一律 fail-closed
- Verification: `go test -race -count=1 ./internal/terminal/windows/`（6 项：
  角色/单实例、token 门、veto+dirty 关闭门、crash 清理、角色围栏、destroy
  列表维护）；`go vet`
- Platforms covered: platform-independent Go core
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Wails 窗口接线 + multi-monitor/crash
  live 矩阵通过后 Electron window manager 退役
- Documentation updated: capability matrix and ledger
- Residual risks: Wails 窗口生命周期接线、route rebind 集成（P3-01）、
  multi-monitor/DPI/键盘缩放 live 证据
- Next safe slice: P4-03 tray and global shortcuts
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L045 - 2026-09-09 - P4-05 App Lock verifier core

- Capability rows: `SYS-04`
- Plan task: `P4-05`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: Go app-lock verifier owner：口令永不存储——盐化 PBKDF2（210k 迭代、
  SHA-256）verifier 经 P2-04 provider 密封进 OS keyring；verifier 记录本身
  不含口令。
- Go canonical owner: `internal/platform/applock`（Enable/Verify/
  ChangePassword）
- Frontend adapter: none yet; Wails lock 生命周期接线（idle/reopen 触发）后续
- Electron owner affected: none; Electron app-lock runtime remains baseline
- Preserved invariants: 无 verifier = 无锁（不伪造锁定态）；无法解封 =
  fail-closed（锁定）；弱口令（<4）拒绝；换口令必须先验旧口令；verifier
  记录 JSON 不含口令（测试断言）
- Data/schema impact: Verifier JSON（salt/digest/createdMs）
- Security impact: PBKDF2-SHA256 210k 迭代；constant-time digest 比较；密封
  envelope purpose 绑定 app-lock/verifier/v1
- Verification: `go test -race -count=1 ./internal/platform/applock/`（4 项：
  enable/verify/reject + 口令不入记录、弱口令、换口令链、缺 verifier
  fail-closed）；`go vet`
- Platforms covered: platform-independent Go core；Windows Hello/Touch ID live
  证据 pending
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: lock 生命周期接线（idle/reopen）+
  verifier 迁移（经 P2-05 broker）+ biometric 证据通过后 Electron app-lock
  runtime 退役
- Documentation updated: capability matrix and ledger
- Residual risks: biometric（Hello/Touch ID）native 接线、verifier 迁移矩阵、
  lock 生命周期（idle 触发/reopen 强制）Wails 侧接线
- Next safe slice: P4-03 tray and global shortcuts owner
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L046 - 2026-09-09 - P3-01 TS frame/credit adapter + P2-07 settings differential

- Capability rows: all rows; no status advancement
- Plan task: `P3-01`, `P2-07`
- Status change: `not-started -> not-started`
- Scope change: `none`
- Goal: 渲染侧补齐 P3-01 数据面最后一块代码：TS frame codec（与 Go codec 逐
  字段同构）+ credit controller；P2-07 补 settings 域差分套件（adapter↔host
  字节等值）。
- Go canonical owner: `internal/terminal/dataplane`（不变）；`internal/profile/store`
  （不变）
- Frontend adapter: `infrastructure/terminal/dataplane/frame.ts`+
  `creditController.ts`；`infrastructure/persistence/settingsDifferential.test.ts`
- Electron owner affected: none
- Preserved invariants: NTDP magic/version/40 字节头/128 KiB 上限/8 种 kind/
  零保留位；初始窗口精确 1 MiB；applied 序列单调；镜写字节等值（含 Unicode）
- Data/schema impact: none
- Security impact: 越界 payload/非法 kind/坏 magic/坏保留位全部 fail-closed
- Verification: `node --test --import tsx` 8 项（frame round-trip/malformed/
  oversized/max-size + credit 窗口/applied + settings 差分 3 项）；`go vet`；
  全套既有 TS 测试不回归
- Platforms covered: platform-independent TS
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: WebSocket 连接接线 + Gate 3 paired
  benchmark 后 MessagePort 退役
- Documentation updated: ledger
- Residual risks: WebSocket 连接管理（重连/背压）与真实 xterm 实例接线；
  Gate 3 formal benchmark
- Next safe slice: P3-04A SSH session integration over the shared pool
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L047 - 2026-09-09 - P4-04 deep link parser and intent queue

- Capability rows: `SYS-03`
- Plan task: `P4-04`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: Go deep-link intent owner：严格解析 ssh/telnet/netcatty 三种 scheme、
  拒绝携带 password/secret/token 参数的意图、pre-ready 队列恰好一次投递、
  重复冷启动意图去重、并发入队安全。
- Go canonical owner: `internal/platform/deeplink`（Parse + Queue）
- Frontend adapter: none yet; Wails second-instance 事件对接后续
- Electron owner affected: none; Electron deep-link 路径 remains baseline
- Preserved invariants: 未知 scheme 拒绝；空 host/含空白 user/host 拒绝；
  口令类参数拒绝（口令永不走 deep link）；Ready 后按到达序投递且仅一次
- Data/schema impact: Action JSON 类型
- Security impact: URL 解析不使用宽松 url.Parse 的 host 语义（手工 authority
  切分），避免 user-info/方括号 IPv6 边界歧义被利用
- Verification: `go test -race -count=1 ./internal/platform/deeplink/`（6 项：
  三 scheme 解析、malformed/不安全拒绝、缓冲顺序投递、去重、16 并发入队）
- Platforms covered: platform-independent Go core
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 安装包注册 + 三平台 cold/warm/畸形 URL
  证据通过后，deep-link/main/installer 代码退役
- Documentation updated: capability matrix and ledger
- Residual risks: 安装包注册（installer 层）、文件关联、context menu、disabled
  preference 证据
- Next safe slice: P5-01 manifest v2 and Go-first contract codegen
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L048 - 2026-09-09 - P5-01/P5-02A manifest v2 and permission broker

- Capability rows: `PLUG-01`
- Plan task: `P5-01`, `P5-02A`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: 冻结插件 v2 contract 与安全边界：WASM-only entrypoint 的 manifest v2
  schema 校验（13 例拒绝矩阵）与 fail-closed 权限 broker（canonical resource、
  lifetime、过期、默认拒绝）。
- Go canonical owner: `internal/plugin/manifest`（contract）与
  `internal/plugin/permissions`（fail-closed broker；不执行能力，仅授权）
- Frontend adapter: none yet; WASM runtime（P5-03）与 package store（P5-02）
  后续消费
- Electron owner affected: none; v1 runtime remains Electron baseline
- Preserved invariants: v1 main.browser/main.node 在 schema 层不可表达
  （WV3-005）；entrypoint 必须 .wasm + 64 hex sha256；权限 kind 白名单、mode
  限 read/write、重复拒绝；contribution 类型白名单、ID 规范、去重；broker
  默认拒绝、过期即拒、principal 未授权即拒
- Data/schema impact: Manifest/Permission/Contribution JSON 契约；Grant 记录
- Security impact: manifest 校验是安装边界第一道；broker 是 P5-03/P5-05 的
  唯一授权面；口令/secret 类资源拒绝经 deep-link 层（P4-04）
- Verification: `go test -count=1 ./internal/plugin/manifest/`（3 项：合法
  接受、13 例拒绝矩阵、JSON round-trip + v1 拒绝说明）；
  `go test -race ./internal/plugin/permissions/`（5 项：默认拒绝、写授权、
  过期、revoke/revoke-all、资源别名隔离）；`go vet`
- Platforms covered: platform-independent Go core
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-005`, `WV3-010`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: P5-02 package store + P5-08 断开 v1
  路径后，Electron plugin runtime 冻结（P9-01 删除）
- Documentation updated: capability matrix and ledger
- Residual risks: guest bindings codegen、WASM runtime（P5-03）、声明式 UI、
  package store、native 进程 runtime、真实攻击语料
- Next safe slice: P5-02 package store over the profile store
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L049 - 2026-09-09 - P4-03 global shortcut registry and tray state

- Capability rows: `SYS-02`
- Plan task: `P4-03`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: 建立 shortcut 注册/冲突/查找核心与 tray 菜单状态生成器，使 Wails
  adapter 只需做 OS-level 注册映射。
- Go canonical owner: `internal/terminal/shortcuts`
- Frontend adapter: none yet; Wails tray/shortcut adapter 后续
- Electron owner affected: none; global shortcut/window bridges remain baseline
- Preserved invariants: accelerator 解析（modifier 校验）、冲突检测（同加速器
  拒绝）、大小写不敏感查找、注销即移除
- Data/schema impact: none
- Security impact: none
- Verification: `go test -race -count=1 ./internal/terminal/shortcuts/`（6 项：
  合法解析、非法拒绝、冲突/查找/注销、大小写不敏感、tray 菜单生成）；`go vet`
- Platforms covered: platform-independent Go core
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Wails tray/shortcut adapter 接线 +
  native smoke 矩阵通过后，global shortcut/window bridges 退役
- Documentation updated: capability matrix and ledger
- Residual risks: OS-level 注册（Wails adapter 接线）、三平台 native smoke
- Next safe slice: P5-02 package store
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L050 - 2026-09-09 - P5-02 package store + P5-04 declarative UI schema

- Capability rows: `PLUG-01`
- Plan task: `P5-02`, `P5-04`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: 插件 v2 package store（内存清单 + 生命周期状态机 + 排序列表）与声明式
  UI schema 校验（settings form + view，注入向量拒绝）。
- Go canonical owner: `internal/plugin/store` + `internal/plugin/ui`
- Frontend adapter: none yet; Wails 渲染层消费 ui.Schema
- Electron owner affected: none
- Preserved invariants: package record 含 manifest 快照/sha256/state/时间戳；
  重复安装拒绝；UI schema ID 全小写+限定字符；HTML/JS/CSS 注入向量全拒；
  select 必须有 options
- Data/schema impact: PackageRecord/SettingField/ViewDef JSON 契约
- Security impact: UI schema 是"什么允许渲染"的唯一权威；注入向量在 schema
  校验层拒绝而非渲染时转义
- Verification: `go test ./internal/plugin/store/`（3 项）+ `./internal/plugin/ui/`（4 项 race）+ `go vet`；全量 `internal/plugin/` 3 包全过
- Platforms covered: platform-independent Go core
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-005`, `WV3-010`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: P5-03 wazero + P5-05 native 进程 +
  P5-08 断开 v1 后，Electron plugin runtime 冻结
- Documentation updated: capability matrix and ledger
- Residual risks: wazero WASM runtime、native 进程 runtime、真实攻击语料、
  codegen drift 检查
- Next safe slice: P5-03 wazero WASM runtime
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L051 - 2026-09-10 - 切片 A: data plane 接入 Wails 壳（缓冲/重连/origin 修复）

- Capability rows: `TERM-01`
- Plan task: `P3-01`
- Status change: `implemented -> implemented`
- Scope change: `none`
- Goal: 生产 data plane 在 Wails 壳内可用：Publish 懒建队列（渲染层接入前输出
  缓冲而非丢弃）、WebSocket 断开后重连换取新队列、DropOutput 会话清理、
  origin 校验放行 wails.localhost host（http/https/wails scheme 任意端口）。
- Go canonical owner: `internal/terminal/dataplane`（handlers.go/server.go）
- Frontend adapter: Wails bindings 重新生成（6 services / 27 methods，新增
  terminalservice.js）；renderer WS 消费待切片 C
- Electron owner affected: none
- Preserved invariants: route token 一次性 + generation fencing；credit 门控
  输出；Host 精确匹配 loopback；其余 origin 仍全部拒绝
- Data/schema impact: none
- Security impact: origin 校验扩展面仅为 wails.localhost host 前缀域（scheme
  白名单），无新增网络暴露
- Verification: `go test -race ./internal/terminal/dataplane/` 全绿；新增 4 项
  测试（pre-attach 缓冲、重连新队列、DropOutput、origin 矩阵）；`go vet` 干净
- Platforms covered: platform-independent Go core
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-010`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 三平台配对基准证据后 TERM-01 才能进入 verified
- Documentation updated: capability matrix (TERM-01 row)
- Residual risks: 输出预缓冲无上限（首个 credit 前依赖写入速率）；断连-重连
  窗口内的 publish 可能落入垂死队列；三平台基准证据缺失
- Next safe slice: 切片 B SFTP Service（over SSH transport）
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L052 - 2026-09-10 - 切片 A: Wails SSH Terminal Service 端到端接线

- Capability rows: `SSH-01`
- Plan task: `P3-03`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: cmd/netcatty 新增 TerminalService：SSH dial → StrictPolicy host-key →
  PTY shell → data plane Publish → resize/signal/close，urgent 通道注入 stdin，
  Bootstrap 暴露 route 凭证给渲染层；main.go 完成 data plane 启停与服务注册。
- Go canonical owner: `cmd/netcatty/terminalService.go` + `internal/terminal/ssh`
- Frontend adapter: Wails service 绑定（terminalservice.js）；renderer 消费待切片 C
- Electron owner affected: none
- Preserved invariants: host-key 走 StrictPolicy（accept-new/reject-changed，
  known_hosts 落 profile 数据目录，OpenSSH 行格式）；会话关闭统一回收
  transport/route/输出队列；无业务逻辑进组件层（facade 模式）
- Data/schema impact: 新增 known_hosts 文件
- Security impact: 密码参数经 Wails 绑定传输（loopback IPC）；键盘交互认证
  回调暂未接 UI，密码模式先行
- Verification: `go test -race ./internal/... ./cmd/...` 全绿；修复 telnet
  `c.handler` data race（flush/handleNegotiation 改为锁内快照）；`go vet` 干净；
  `npm run wails:build` 产出 bin/netcatty-wails.exe 并完成 6 秒启动冒烟
  （WebView2 加载真实前端资源，6 services 注册）
- Platforms covered: Windows 10 22H2（本机）
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-010`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 渲染层 WS 消费 + 活体 MFA/proxy/jump
  证据后 SSH-01 才能进入 verified
- Documentation updated: capability matrix (SSH-01 row)
- Residual risks: 无活动 SSH 服务器可用的本机环境仅验证接线与编译产物；
  键盘交互认证 UI 回调缺失；legacy 算法决策待定
- Next safe slice: 切片 B SFTP Service（over SSH transport）
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L053 - 2026-09-10 - 切片 B: 生产 SFTP ClientFS + Wails SFTPService

- Capability rows: `SFTP-01`
- Plan task: `P3-05`, `P3-04A`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: pkg/sftp 生产适配器（ClientFS 实现 RemoteFS，含符号链接标记、递归
  删除、PosixRename、下载/上传流式传输）+ cmd/netcatty SFTPService 门面
  （共享 SSH pool KindSFTP 租约、每会话有界并发、路径规范化、池归还）。
- Go canonical owner: `internal/terminal/sftp/clientfs.go` +
  `cmd/netcatty/sftpService.go` + `internal/terminal/sshpool`
- Frontend adapter: Wails bindings 7 services / 36 methods（新增
  sftpservice.js）；renderer 消费待切片 C
- Electron owner affected: none
- Preserved invariants: 服务从不自建 SSH 连接（仅经 pool 租约）；路径一律
  NormalizePath 规范化；会话关闭释放 SFTP 子系统并把租约归还池
- Data/schema impact: none
- Security impact: SFTP 凭据与 TerminalService 同源（StrictPolicy known_hosts）；
  本地落盘路径由调用方（渲染层）显式指定，切片 C 接入专用临时目录
- Verification: ClientFS 对真实 pkg/sftp server（net.Pipe + TempDir root）做
  协议级 round-trip 测试（create/write/read/stat/mkdir/readdir/rename/
  recursive remove）；`go test -race ./internal/... ./cmd/...` 全绿；`go vet`
  干净；bindings 重新生成；`npm run wails:build` 重建 exe 并 6 秒启动冒烟
- Platforms covered: Windows 10 22H2（本机；协议级测试与平台无关）
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-010`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 真实服务器矩阵 + 渲染层消费后
  SFTP-01 才能进入 verified
- Documentation updated: capability matrix (SFTP-01 row)
- Residual risks: 真实服务器/编码/符号链接活体矩阵缺失；上传无断点续传
  （P3-06 scheduler 待接）；sudo SFTP 未实现
- Next safe slice: 切片 C 前端 service 层（Wails binding 路由）
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L054 - 2026-09-10 - 切片 C: Wails 适配器 terminal/sftp 端口路由

- Capability rows: `FND-01`
- Plan task: `P1-01`, `P1-02`
- Status change: `implemented -> implemented`
- Scope change: `none`
- Goal: 前端 service 层接通 Go 面：Wails adapter 的 terminal 端口实现
  startSSHSession/writeToSession/resizeSession/interruptSession/closeSession，
  sftp 端口实现 openSftp/listSftp/mkdirSftp/deleteSftp/renameSftp/statSftp/
  closeSftp（Entry/FileInfo → RemoteFile/SftpStatResult 契约映射）；新增
  goTerminalSurface() 暴露 data plane WS URL/route token 组装与流式
  Download/Upload；未迁移方法保持 fail-closed。
- Go canonical owner: none（复用 TerminalService/SFTPService 绑定）
- Frontend adapter: `infrastructure/runtime/wails/wailsRuntimeClient.ts` +
  纯映射模块 `terminalRoute.ts`（唯一 bindings 导入边界不变）
- Electron owner affected: none（Electron adapter 不变）
- Preserved invariants: 仅 wailsRuntimeClient.ts 可导入 bindings（ESLint）；
  []byte stdin 经 base64 过 JSON 绑定层；Electron 形状之外的认证选项
  （privateKey/passphrase/jumpHosts/proxy/MFA）显式拒绝而非静默降级
- Data/schema impact: none
- Security impact: route token 只在 bootstrap 响应中交付渲染层；WS URL 由
  服务端 listenAddr 组装，渲染层不可改写 Host
- Verification: terminalRoute.test.ts 8 项 + runtime 套件 24/24 全绿（含
  更新后的 fail-closed 契约测试）；`tsc --noEmit` 干净；ESLint 干净；
  `npm run wails:build` 重建 exe + 启动冒烟通过
- Platforms covered: Windows 10 22H2（本机）
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-010`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 全部 9 端口迁移完成且 transitionBridge
  删除后，Electron preload/netcattyBridge 才能退役
- Documentation updated: capability matrix (FND-01 row)
- Residual risks: xterm.js 渲染层尚未消费 data plane WS（Electron 事件管线
  仍在）；key/MFA 认证选项待 Go 绑定扩展；三平台证据缺失
- Next safe slice: 切片 D（P6-02 打包 / P6-04 迁移执行 / P6-05 gate）
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L055 - 2026-09-10 - P6-02: Wails 资格打包管线（Windows 实证）

- Capability rows: `REL-01`
- Plan task: `P6-02`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: 可重复的资格打包脚本 `scripts/package-wails.mjs`：版本 ldflags 戳记
  （-X main.version，已验证替换默认字符串）、trimpath + 符号剥离、产物命名
  Netcatty-{version}-{goos}-{goarch} 加平台可执行后缀、SHA-256 checksums.txt 与
  artifact-manifest.json（version/commit/goos/goarch/cross/artifacts）、
  交叉编译强制 CGO_ENABLED=0 并告警；node:test 单测 5 项覆盖纯函数。
- Go canonical owner: none（打包脚本 + `cmd/netcatty` 构建产物）
- Frontend adapter: 复用 npm run build + wails-prepare-frontend（--skip-frontend 可跳）
- Electron owner affected: none
- Preserved invariants: 产物清单与校验和先于发布证据；跨平台二进制不冒充
  原生 GUI 产物（cross 标记）
- Data/schema impact: 新增 dist/wails/{artifact-manifest.json, checksums.txt}
  （构建产物，gitignore）
- Security impact: 符号剥离 + trimpath 降低可利用面；签名仍缺（诚实记录）
- Verification: node --test scripts/package-wails.test.mjs（5/5）；实际打包
  Windows amd64 产物并 8 秒启动冒烟（WebView2 加载前端，trimpath 生效）；
  grep 验证默认版本字符串已被 ldflags 替换
- Platforms covered: Windows 10 22H2（本机实证）；macOS/Linux 目标为脚本
  支持但仅有 CI 占位，无本机证据
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-010`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 三平台签名包 + P8-01 干净机冒烟后
  REL-01 才能进入 verified
- Documentation updated: capability matrix (REL-01 row)
- Residual risks: 签名、安装包格式（msi/pkg/AppImage/deb/rpm/pacman）、
  更新feed 与干净机矩阵全部缺失
- Next safe slice: P6-05 gate 前置缺口清点（其余 needs-verification 行）
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L056 - 2026-09-10 - P6-04: 升级引导链持久化 + UpgradeService

- Capability rows: `REL-02`
- Plan task: `P6-04`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: Electron N-1 → Wails 升级引导链产品化：internal/platform/upgrade
  新增 PersistentCoordinator（upgrade-state.json 原子落盘 temp+rename、
  崩溃后按存储 step 恢复、版本对不匹配 fail-closed、损坏状态拒绝、
  ClearState 重置）；cmd/netcatty UpgradeService 门面（Status/Begin/
  Advance/Cancel，无状态句柄，每调用重开持久协调器）。
- Go canonical owner: `internal/platform/upgrade/persistent.go` +
  `cmd/netcatty/upgradeService.go`
- Frontend adapter: Wails bindings 8 services / 40 methods（新增
  upgradeservice.js）；renderer 消费待接
- Electron owner affected: none
- Preserved invariants: 仅前向状态迁移（状态机校验不变）；状态写入原子
  （tmp+rename）；损坏/不匹配状态 fail-closed 而非静默重置
- Data/schema impact: profile 数据目录 upgrade/upgrade-state.json
  （from/to/step/history/updatedAt）
- Security impact: 升级状态不含机密；版本对不匹配显式拒绝避免错误回滚覆盖
- Verification: persistent_test.go 4 项 race 全绿（begin/resume/原子性/
  损坏/mismatch/clear）；`go test -race ./internal/... ./cmd/...` 全绿；
  `go vet` 干净；bindings 重新生成
- Platforms covered: platform-independent Go core
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-010`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 真实 N-1 升级活体演练（含中断恢复）
  + 签名 feed 后 REL-02 才能进入 verified
- Documentation updated: capability matrix (REL-02 row: not-started -> probe)
- Residual risks: 无生产 updater feed/签名密钥管理；P8-01 终局 N-1→N、
  篡改、中断、提权、托盘、回滚矩阵全部待做
- Next safe slice: P6-05 NONAI-COMPLETE gate 缺口清点
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L057 - 2026-09-10 - 渲染层消费 loopback 数据面（transitionBridge）

- Capability rows: `FND-01`
- Plan task: `P1-01`
- Status change: `implemented -> implemented`
- Scope change: `none`
- Goal: 现有 UI 通过 netcattyBridge.transitionBridge 走到 Go 终端/SFTP
  而不改组件：startSSHSession 拨号后自动 Bootstrap + 打开数据面 WebSocket，
  onSessionData 把 Output 帧解码为 UTF-8 回调，closeSession 关闭套接字。
- Go canonical owner: none（复用 TerminalService/SFTPService）
- Frontend adapter: `infrastructure/runtime/wails/wailsRuntimeClient.ts` +
  `dataPlaneSession.ts`（纯客户端：开窗、授信、ACK、Complete）
- Electron owner affected: none
- Preserved invariants: 未迁移方法仍 fail-closed；Electron adapter 未改；
  数据面帧编解码复用 infrastructure/terminal/dataplane/frame.ts
- Data/schema impact: none
- Security impact: 路由令牌仍只经 Bootstrap 交付；Host 由 listenAddr 组装
- Verification: dataPlaneSession.test.ts 2 项 + wailsRuntimeClient.test.ts
  3 项 + runtime 套件 29/29 全绿；tsc --noEmit 干净；ESLint 干净
- Platforms covered: platform-independent TypeScript
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-010`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 三平台配对基准证据后 TERM-01 才能进入 verified
- Documentation updated: capability matrix (FND-01, TERM-01 rows)
- Residual risks: 无活体 SSH 服务器的端到端绘制证据；urgent 通道尚未接到
  Ctrl-C 以外的 UI 路径；未迁移端口仍 fail-closed
- Next safe slice: 活体 SSH/SFTP 证据或 P6-05 缺口清点（不可伪造 verified）
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L058 - 2026-09-10 - 数据面 Output 帧接入 onSessionData

- Capability rows: `TERM-01`
- Plan task: `P3-01`
- Status change: `implemented -> implemented`
- Scope change: `none`
- Goal: 渲染层 dataPlaneSession 客户端：开窗后授予 1 MiB receive window，
  Output 帧解码为 UTF-8 回调并按 creditCost 回授信，Complete 关闭套接字。
- Go canonical owner: `internal/terminal/dataplane`
- Frontend adapter: `infrastructure/runtime/wails/dataPlaneSession.ts`
- Electron owner affected: none
- Preserved invariants: 帧编解码与 Go v2 契约字节一致；初始授信必须是
  整窗 1 MiB；Host 由 listenAddr 组装
- Data/schema impact: none
- Security impact: 一次性路由令牌经 Bootstrap 交付，不进 URL
- Verification: dataPlaneSession.test.ts 2 项（授信/输出/Complete、dispose
  忽略后续帧）；runtime 套件 29/29
- Platforms covered: platform-independent TypeScript
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-010`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 三平台配对基准证据后才能 verified
- Documentation updated: capability matrix (TERM-01 row)
- Residual risks: 无活体 SSH 绘制证据；urgent 通道未接到 UI
- Next safe slice: 活体 SSH 证据（不可伪造 verified）
- Drift decision: `user-approved-implementation-ahead-of-evidence`
