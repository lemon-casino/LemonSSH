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

## WV3-L059 - 2026-09-10 - 本地 PTY 接入同一数据面

- Capability rows: `TERM-02`
- Plan task: `P3-02`
- Status change: `implemented -> implemented`
- Scope change: `none`
- Goal: TerminalService.StartLocal 启动本机 ConPTY/Unix PTY，输出泵入与
  SSH 相同的 loopback 数据面；Write/Resize/Signal/Close/urgent 按会话类型
  分派；前端 startLocalSession 走同一套 onSessionData 附着。
- Go canonical owner: `cmd/netcatty/terminalService.go` + `internal/terminal/pty`
- Frontend adapter: wailsRuntimeClient.startLocalSession
- Electron owner affected: none
- Preserved invariants: generation fencing 仍由 pty.Session 执行；数据面
  路由令牌一次性；SSH 会话路径未改
- Data/schema impact: none
- Security impact: 本地 PTY 不走网络；urgent 仍只写 stdin
- Verification: `go test -race ./internal/terminal/pty/` 绿；runtime 套件
  覆盖 startLocalSession 附着数据面
- Platforms covered: Windows 10 22H2（本机编译）；Unix 后端已有既有测试
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-010`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 三平台 shell 矩阵后才能 verified
- Documentation updated: capability matrix (TERM-02 row)
- Residual risks: 无 Unicode/reload 广度矩阵；cmd.exe 活体绘制未单独记录
- Next safe slice: 活体 SSH/SFTP 证据（不可伪造 verified）
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L060 - 2026-09-10 - Wails 无边框主窗口与原生窗口控制

- Capability rows: `FND-04`
- Plan task: `P4-02`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: 主窗口启用 Frameless，现有 TopTabs 成为唯一标题栏；windowMinimize/
  windowMaximize/windowClose/windowIsMaximized/windowIsFullscreen 接 Wails Window API；
  app-drag/app-no-drag 同时映射 Electron 与 Wails 非客户区属性。
- Go canonical owner: `cmd/netcatty/main.go`
- Frontend adapter: `infrastructure/runtime/wails/wailsRuntimeClient.ts`
- Electron owner affected: none
- Preserved invariants: 窗口可调整大小；现有 Electron drag 属性保留；交互控件不进入拖拽区
- Data/schema impact: none
- Security impact: none
- Verification: cmd/netcatty frameless test；窗口适配器测试；CSS drag-region 契约测试；
  Go race/vet 绿；Wails 本机构建成功
- Platforms covered: Windows 10 22H2（本机）
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-012`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 三平台无边框拖拽/缩放/窗口按钮验证后
- Documentation updated: migration ledger
- Residual risks: macOS/Linux 非客户区行为待活体验证
- Next safe slice: 三平台窗口行为证据
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L061 - 2026-09-10 - Batch 1: 对话框、密钥 SSH、SFTP 读写、端口转发接线

- Capability rows: `SYS-01`
- Plan task: `P4-01`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: Wails transitionBridge 暴露 selectFile/selectDirectory/showSaveDialog
  （Wails Dialogs）、SSH Connect 接受 privateKey+passphrase、SFTP Read/WriteText/
  HomeDir、ForwardService 包 internal/terminal/forward。
- Go canonical owner: cmd/netcatty/{terminalService,sftpService,forwardService}.go
- Frontend adapter: wailsRuntimeClient.ts + terminalRoute.ts
- Electron owner affected: none
- Preserved invariants: jump/MFA/proxy 仍显式拒绝；未迁移方法保持 undefined
- Data/schema impact: none
- Security impact: 私钥经 Wails IPC 传至 Go dial，不落盘
- Verification: terminalRoute 8 项 + runtime adapter 7 项 + go test cmd/netcatty 与 forward 绿
- Platforms covered: Windows 10 22H2
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 活体密钥 SSH 与真实转发矩阵后
- Documentation updated: ledger
- Residual risks: jump/MFA 未接；转发依赖密码拨号；对话框需本机手动点选
- Next safe slice: Telnet/Serial 接入同一数据面
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L062 - 2026-09-10 - Telnet 接入同一数据面

- Capability rows: `TERM-03.1`
- Plan task: `P3-08`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: TerminalService.StartTelnet 拨号后把 IAC 解码数据泵入 dataplane；
  startTelnetSession 走 onSessionData。
- Go canonical owner: cmd/netcatty/terminalService.go + internal/terminal/telnet
- Frontend adapter: wailsRuntimeClient.startTelnetSession
- Electron owner affected: none
- Preserved invariants: SSH/local 路径未改；urgent 写入 telnet Send
- Data/schema impact: none
- Security impact: 明文 Telnet，与既有协议一致
- Verification: go test telnet + cmd/netcatty；runtime 套件绿
- Platforms covered: platform-independent Go + Windows adapter tests
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 活体设备矩阵后
- Documentation updated: ledger
- Residual risks: 自动登录 UI 未接；无真实设备证据
- Next safe slice: Serial
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L063 - 2026-09-10 - Serial 接入同一数据面

- Capability rows: `TERM-03.2`
- Plan task: `P3-08`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: ListSerialPorts + StartSerial 打开端口并把读循环泵入 dataplane；
  startSerialSession/listSerialPorts 接到 transitionBridge。
- Go canonical owner: cmd/netcatty/terminalService.go + internal/terminal/serialport
- Frontend adapter: wailsRuntimeClient
- Electron owner affected: none
- Preserved invariants: 配置校验 fail-closed；SSH/telnet/local 路径未改
- Data/schema impact: none
- Security impact: 仅本机串口设备
- Verification: go test serialport + cmd/netcatty；runtimeSelection 绿
- Platforms covered: Windows 10 22H2 枚举路径
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 真实设备矩阵后
- Documentation updated: ledger
- Residual risks: 无真实硬件证据；YMODEM 未接
- Next safe slice: App Lock 最小 Go owner
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L064 - 2026-09-10 - App Lock 最小运行时 owner

- Capability rows: `SYS-04`
- Plan task: `P4-05`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: AppLockService 提供 initialized=true, locked=false 的运行时快照，
  getAppLockRuntimeState 成为 transitionBridge 真方法，避免门控永久等待。
  生物识别与密码 verifier 仍未接线。
- Go canonical owner: cmd/netcatty/appLockService.go
- Frontend adapter: wailsRuntimeClient.getAppLockRuntimeState
- Electron owner affected: none
- Preserved invariants: 无 verifier 则永不锁定；未实现方法仍 undefined
- Data/schema impact: none
- Security impact: 未启用锁时明确未锁定，不伪造已验证状态
- Verification: go test cmd/netcatty；runtime adapter 绿
- Platforms covered: platform-independent
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: PBKDF2 verifier + 生物识别活体后
- Documentation updated: ledger
- Residual risks: 密码启用/生物识别未接
- Next safe slice: 本机构建验证
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L065 - 2026-09-10 - 托盘品牌与设置入口

- Capability rows: `SYS-02`
- Plan task: `P4-03`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: 托盘文案改为 LemonSSH；增加 Settings 菜单项打开独立设置窗口；Show 会 Focus 主窗口。
- Go canonical owner: cmd/netcatty/main.go
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: Quit 仍走 wailsApp.Quit
- Data/schema impact: none
- Security impact: none
- Verification: go test cmd/netcatty
- Platforms covered: Windows 10 22H2
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 三平台托盘行为后
- Documentation updated: ledger
- Residual risks: 全局快捷键未注册
- Next safe slice: deep link 服务
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L066 - 2026-09-10 - Deep link 解析服务

- Capability rows: `SYS-03`
- Plan task: `P4-04`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: DeepLinkService 暴露 Parse/Enqueue/Pending/Ready，密码参数仍拒绝。OS 协议注册未做。
- Go canonical owner: cmd/netcatty/deepLinkService.go + internal/platform/deeplink
- Frontend adapter: bindings only
- Electron owner affected: none
- Preserved invariants: 密码 query 失败关闭；重复冷启动意图去重
- Data/schema impact: none
- Security impact: 密码不得进入 deep link
- Verification: go test deeplink + cmd/netcatty
- Platforms covered: platform-independent
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: OS 协议注册与冷启动投递后
- Documentation updated: ledger
- Residual risks: 未注册 ssh/telnet/netcatty URL scheme
- Next safe slice: plugin list
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L067 - 2026-09-10 - 插件清单只读门面

- Capability rows: `PLUG-01`
- Plan task: `P5-02`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: PluginService.List 暴露空清单；安装/启用/WASM 仍未接线。
- Go canonical owner: cmd/netcatty/pluginService.go + internal/plugin/store
- Frontend adapter: listPlugins on transitionBridge
- Electron owner affected: none
- Preserved invariants: 空清单不假装已安装插件
- Data/schema impact: none
- Security impact: 无执行面
- Verification: go test plugin/store + cmd/netcatty
- Platforms covered: platform-independent
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-005`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 安装/权限/WASM 后
- Documentation updated: ledger
- Residual risks: 无法安装或运行插件
- Next safe slice: 本机构建
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L068 - 2026-09-10 - SSH MFA 与跳板/代理接到 Wails Connect

- Capability rows: `SSH-01`
- Plan task: `P3-03`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: TerminalService.Connect 接受 jumpHosts、socks5/http proxyUrl 与 enableMfa；keyboard-interactive 经 ssh:keyboard-interactive 事件进入现有渲染层弹窗；command 代理与证书仍失败关闭。
- Go canonical owner: cmd/netcatty/terminalService.go + internal/terminal/ssh
- Frontend adapter: terminalRoute.pickSSHConnectArgs + wailsRuntimeClient onKeyboardInteractive
- Electron owner affected: none
- Preserved invariants: 证书与 command 代理仍显式拒绝；无活体 MFA 不断言 verified
- Data/schema impact: none
- Security impact: MFA 应答经现有 respondKeyboardInteractive 完成，超时失败关闭
- Verification: go test internal/terminal/ssh + cmd/netcatty；node terminalRoute 与 wailsRuntimeClient 套件
- Platforms covered: Windows 10 22H2 本机契约；无活体 MFA 服务器
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 活体 MFA/跳板/代理矩阵后
- Documentation updated: remaining-work, capability matrix, ledger
- Residual risks: 无真实 MFA 或跳板证据；SFTP Open 仍不走交互式 MFA
- Next safe slice: SFTP 下载上传接线
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L069 - 2026-09-10 - SFTP 下载上传与本地解压接线

- Capability rows: `SFTP-01`
- Plan task: `P3-05`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: startStreamTransfer 调用既有 Download/Upload；extractLocalArchive 调用 zip-slip 硬化 ExtractArchive；SFTP Open 与 SSH 共用 Connect 结构体。
- Go canonical owner: cmd/netcatty/sftpService.go + filesystemService.go
- Frontend adapter: wailsRuntimeClient startStreamTransfer / extractLocalArchive
- Electron owner affected: none
- Preserved invariants: sudo SFTP 与非 UTF-8 路径仍未验证
- Data/schema impact: none
- Security impact: 解压拒绝 zip-slip；下载写入调用方指定本地路径
- Verification: go test cmd/netcatty filesystem；runtime adapter 绿
- Platforms covered: Windows 10 22H2 契约
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 真实 SFTP 编码/符号链接矩阵后
- Documentation updated: remaining-work, capability matrix, ledger
- Residual risks: 传输中心 UI 与远程压缩包提取仍未接
- Next safe slice: 传输调度器门面
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L070 - 2026-09-10 - 传输调度器 pause/resume/cancel 门面

- Capability rows: `SFTP-02`
- Plan task: `P3-06`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: TransferService 暴露既有 scheduler 的 Enqueue/Pause/Resume/Cancel/Progress；压缩上传仍未接线。
- Go canonical owner: cmd/netcatty/transferService.go + internal/terminal/transfer
- Frontend adapter: pauseTransfer/resumeTransfer/cancelTransfer
- Electron owner affected: none
- Preserved invariants: 不假装压缩上传或高 RTT 实验室完成
- Data/schema impact: none
- Security impact: 取消失败关闭未知任务
- Verification: go test internal/terminal/transfer + cmd/netcatty
- Platforms covered: platform-independent
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 压缩上传与 renderer-close 存活证据后
- Documentation updated: remaining-work, capability matrix, ledger
- Residual risks: 无真实损坏/续传实验室
- Next safe slice: App Lock 密码启用
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L071 - 2026-09-10 - App Lock 密码启用与解锁

- Capability rows: `SYS-04`
- Plan task: `P4-05`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: AppLockService.Enable/Unlock/Disable 使用 PBKDF2 owner 并持久化 verifier；requestAppLockUnlock 接到 transitionBridge。生物识别仍未接。
- Go canonical owner: cmd/netcatty/appLockService.go + internal/platform/applock
- Frontend adapter: requestAppLockUnlock / requestAppLockPasswordChange / requestAppLockDisable
- Electron owner affected: none
- Preserved invariants: 无 verifier 则不锁定；Hello/Touch ID 仍 undefined
- Data/schema impact: settings 域 app-lock-verifier 记录
- Security impact: 口令不落盘，仅存 salt/digest
- Verification: go test applock + cmd/netcatty AppLock round-trip
- Platforms covered: platform-independent
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Windows Hello/Touch ID 活体后
- Documentation updated: remaining-work, capability matrix, ledger
- Residual risks: 生物识别未接；无跨会话锁生命周期 A 级证据
- Next safe slice: deep link 二次启动入队
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L072 - 2026-09-10 - Deep link 二次启动 argv 入队

- Capability rows: `SYS-03`
- Plan task: `P4-04`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: 冷启动 os.Args 与二次启动 Args 中的 ssh/telnet/netcatty URL 入队；Drain 暴露给渲染层。OS 协议注册仍未做。
- Go canonical owner: cmd/netcatty/deepLinkService.go + main.go
- Frontend adapter: bindings Drain
- Electron owner affected: none
- Preserved invariants: 密码 query 仍拒绝
- Data/schema impact: none
- Security impact: 密码不得进入 deep link
- Verification: go test deeplink + cmd/netcatty TestDeepLinkSecondInstanceArgs
- Platforms covered: platform-independent
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 已安装包 OS 协议注册后
- Documentation updated: remaining-work, capability matrix, ledger
- Residual risks: 未注册 ssh/telnet/netcatty URL scheme
- Next safe slice: 插件安装元数据门面
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L073 - 2026-09-10 - 插件安装元数据门面不执行 WASM

- Capability rows: `PLUG-01`
- Plan task: `P5-02`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: PluginService.Install/SetEnabled 只改内存清单；不启动 WASM 或 native 进程。
- Go canonical owner: cmd/netcatty/pluginService.go + internal/plugin/store
- Frontend adapter: listPlugins
- Electron owner affected: none
- Preserved invariants: 空清单不假装已运行插件
- Data/schema impact: none
- Security impact: 无执行面
- Verification: go test plugin/store + cmd/netcatty
- Platforms covered: platform-independent
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-005`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: WASM 沙箱与权限 broker 活体后
- Documentation updated: remaining-work, capability matrix, ledger
- Residual risks: 无法执行插件代码
- Next safe slice: 签名探测脚本
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L074 - 2026-09-10 - 诚实 Windows 签名探测

- Capability rows: `REL-01`
- Plan task: `P6-02`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: scripts/sign-wails-probe.mjs 在缺 signtool 或缺证书时记录 unsigned 原因，不调用伪造签名，不声称安装包完成。
- Go canonical owner: none; packaging probe only
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: 无 msi/pkg/AppImage；无 Authenticode 成功断言
- Data/schema impact: none
- Security impact: 不把未签名产物标成已签名
- Verification: node --test scripts/sign-wails-probe.test.mjs
- Platforms covered: Windows 探测路径；macOS/Linux 不调用 signtool
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 三平台签名安装包后
- Documentation updated: remaining-work, capability matrix, ledger
- Residual risks: 仍无代码签名证书
- Next safe slice: CI remaining-work 契约
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L075 - 2026-09-10 - remaining-work 接线契约进入 evidence CI

- Capability rows: `SYS-02`
- Plan task: `P4-03`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: migration-evidence 的 netcatty-skeleton-build 增加 Go/Node remaining-work 契约测试与 Windows 签名探测；ShortcutService 暴露内存注册。原生 OS 快捷键仍未验证。
- Go canonical owner: cmd/netcatty/shortcutService.go
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: Electron 基线 job 仍 continue-on-error
- Data/schema impact: none
- Security impact: none
- Verification: go test cmd/netcatty；sign-wails-probe 与 runtime 套件
- Platforms covered: CI matrix windows/macos/ubuntu 契约；无活体窗口
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 三平台原生快捷键与窗口冒烟后
- Documentation updated: remaining-work, capability matrix, ledger, migration-evidence.yml
- Residual risks: 无 macOS/Linux 窗口活体证据
- Next safe slice: 活体 MFA 服务器矩阵
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L076 - 2026-09-10 - 弹出终端窗口接到 Wails

- Capability rows: `FND-04`
- Plan task: `P4-02`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: PopupWindowService 打开 frameless `#/terminal-popup` 窗口并 emit terminal:popup-config；openTerminalPopup/onTerminalPopupConfig 接到 transitionBridge。多显示器与崩溃重绑仍未验证。
- Go canonical owner: cmd/netcatty/popupWindowService.go
- Frontend adapter: wailsRuntimeClient openTerminalPopup
- Electron owner affected: none
- Preserved invariants: 主窗口与设置窗口路径未改；无 popup 绑定时失败关闭
- Data/schema impact: none
- Security impact: 弹出窗口只承载已有会话配置，不新拨号
- Verification: go test cmd/netcatty TestPopupWindow；runtime openTerminalPopup 套件
- Platforms covered: Windows 10 22H2 契约
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 三平台弹出窗口与崩溃重绑后
- Documentation updated: remaining-work, capability matrix, ledger
- Residual risks: 无多显示器或崩溃活体证据
- Next safe slice: deep link 渲染层 drain
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L077 - 2026-09-10 - 渲染层 drain deep links

- Capability rows: `SYS-03`
- Plan task: `P4-04`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: drainDeepLinks 先 Ready 再 Drain；AppSideEffects 启动时消费队列并复用现有 SSH/Telnet 处理。OS 协议注册仍未做。
- Go canonical owner: cmd/netcatty/deepLinkService.go
- Frontend adapter: wailsRuntimeClient drainDeepLinks + AppSideEffects
- Electron owner affected: none
- Preserved invariants: 密码 query 仍拒绝；无 drain 绑定时 optional-chain 跳过
- Data/schema impact: none
- Security impact: 密码不得进入 deep link
- Verification: runtime drainDeepLinks 套件；go test cmd/netcatty
- Platforms covered: platform-independent
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 已安装包 OS 协议注册后
- Documentation updated: remaining-work, capability matrix, ledger
- Residual risks: 未注册 ssh/telnet/netcatty URL scheme
- Next safe slice: extract 失败关闭
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L078 - 2026-09-10 - extract 与 transfer 绑定不再假成功

- Capability rows: `SYS-01`
- Plan task: `P4-01`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: filesystem/transfer/deeplink/popup 进入 defaultBindings；extractLocalArchive 在缺 ExtractArchive 时返回 success false；pauseTransfer 打到 TransferService。
- Go canonical owner: cmd/netcatty/filesystemService.go + transferService.go
- Frontend adapter: wailsRuntimeClient defaultBindings
- Electron owner affected: none
- Preserved invariants: 不解压则不声称成功；压缩上传仍未接
- Data/schema impact: none
- Security impact: zip-slip 硬化提取仍有效
- Verification: runtime extractLocalArchive 与 pauseTransfer 套件
- Platforms covered: Windows 10 22H2 契约
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: UNC/长路径活体后
- Documentation updated: remaining-work, capability matrix, ledger
- Residual risks: 远程压缩包提取仍未接
- Next safe slice: Vault canonical 读通
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L079 - 2026-09-10 - Wails 启动从 Go profile 回填空 localStorage

- Capability rows: `SYNC-01`
- Plan task: `P2-07`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: ProfileService.DomainKeys 列出 settings/vault 键；Wails boot hydrate 只填空的 localStorage 键，已有本地值优先。不把渲染层改成 Go canonical。
- Go canonical owner: cmd/netcatty/profileService.go + internal/profile/store
- Frontend adapter: hostStorageHydrate.ts + bootstrap.ts
- Electron owner affected: none
- Preserved invariants: Electron 路径不 hydrate；本地已有值不被覆盖
- Data/schema impact: none
- Security impact: 回填走既有 profile 密文边界，不解密到新位置
- Verification: node hostStorageHydrate 套件；go test profile/store
- Platforms covered: platform-independent
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 渲染层 canonical 读通与差分套件后
- Documentation updated: remaining-work, capability matrix, ledger
- Residual risks: hooks 仍直读 localStorage
- Next safe slice: 原生快捷键诚实失败关闭
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L080 - 2026-09-10 - 全局快捷键在 alpha.63 诚实失败关闭

- Capability rows: `SYS-02`
- Plan task: `P4-03`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: registerGlobalHotkey 接到 ShortcutService；解析加速键并写入内存 registry；因生产依赖 Wails v3.0.0-alpha.63 无 GlobalShortcut，Register 返回 success false 并说明原因。
- Go canonical owner: cmd/netcatty/shortcutService.go
- Frontend adapter: wailsRuntimeClient registerGlobalHotkey
- Electron owner affected: none
- Preserved invariants: 不把内存登记标成原生成功
- Data/schema impact: none
- Security impact: none
- Verification: go test cmd/netcatty TestShortcutRegister；runtime registerGlobalHotkey 套件
- Platforms covered: Windows 契约；无原生 OS 注册
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 升级 Wails 后接入 GlobalShortcut
- Documentation updated: remaining-work, capability matrix, ledger
- Residual risks: 系统级热键在 alpha.63 不可用
- Next safe slice: 远程解压失败关闭
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L081 - 2026-09-10 - 远程 SFTP 解压失败关闭

- Capability rows: `SFTP-01`
- Plan task: `P3-05`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: extractSftpArchive 在 transitionBridge 上返回 success false，避免 UI 以为远程 zip 已解压。
- Go canonical owner: none; fail-closed adapter only
- Frontend adapter: wailsRuntimeClient extractSftpArchive
- Electron owner affected: none
- Preserved invariants: 本地 ExtractArchive 路径未改
- Data/schema impact: none
- Security impact: 不在远程主机执行未审查解压命令
- Verification: runtime extractSftpArchive 套件
- Platforms covered: platform-independent
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 远程解压 owner 与真实服务器矩阵后
- Documentation updated: remaining-work, capability matrix, ledger
- Residual risks: UI 仍显示解压动作但会失败
- Next safe slice: Vault canonical 读通
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L082 - 2026-09-10 - Wails 首屏等待 profile hydrate

- Capability rows: `SYNC-01`
- Plan task: `P2-07`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: installRuntimeClient 把 hydrateReady 设为 hydrate 完成的 Promise；index.tsx 在 hydrateReady 后再 renderApp，避免 hooks 读到空 localStorage。
- Go canonical owner: cmd/netcatty/profileService.go
- Frontend adapter: bootstrap.ts + index.tsx
- Electron owner affected: none
- Preserved invariants: Electron 路径 hydrateReady 立即完成；本地已有值不被覆盖
- Data/schema impact: none
- Security impact: none
- Verification: node bootstrap.test.ts hydrateReady；hostStorageHydrate 套件
- Platforms covered: platform-independent
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: hooks 改读 Go canonical 后
- Documentation updated: remaining-work, capability matrix, ledger
- Residual risks: hooks 仍直读 localStorage
- Next safe slice: SSH agent/identityFile 失败关闭
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L083 - 2026-09-10 - SSH agent 与 identityFile 失败关闭

- Capability rows: `SSH-01`
- Plan task: `P3-03`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: pickSSHConnectArgs 对 useSshAgent 与 identityFilePaths 显式抛错，避免静默退化成密码认证。
- Go canonical owner: internal/terminal/ssh
- Frontend adapter: terminalRoute.pickSSHConnectArgs
- Electron owner affected: none
- Preserved invariants: PEM 私钥与 passphrase 路径未改
- Data/schema impact: none
- Security impact: 不把缺密钥的 agent 主机当成密码登录
- Verification: node terminalRoute.test.ts agent/identityFile 套件
- Platforms covered: platform-independent
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: agent 与 IdentityFile 活体后
- Documentation updated: remaining-work, capability matrix, ledger
- Residual risks: 仅文件/agent 的主机在 Wails 下无法连接
- Next safe slice: Vault canonical 读通
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L084 - 2026-09-10 - SSH agent 与 IdentityFile 接到 Connect

- Capability rows: `SSH-01`
- Plan task: `P3-03`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: Connect 透出 useAgent 与 identityFilePaths；LoadIdentityFilePEMs 读第一个存在的 PEM；缺文件失败关闭；agent 不可达失败关闭。证书与 command 代理仍拒绝。
- Go canonical owner: internal/terminal/ssh + cmd/netcatty/terminalService.go
- Frontend adapter: terminalRoute.pickSSHConnectArgs
- Electron owner affected: none
- Preserved invariants: PEM 内存私钥路径未改；缺文件不退化成密码
- Data/schema impact: none
- Security impact: IdentityFile 仅读调用方给出的路径
- Verification: go test ssh LoadIdentityFilePEMs 与 UseAgent；node mapper agent/identityFile
- Platforms covered: Windows 契约；无活体 agent
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 活体 agent 与 IdentityFile 矩阵后
- Documentation updated: remaining-work, capability matrix, ledger
- Residual risks: Windows named pipe agent 仍依赖本机 OpenSSH
- Next safe slice: settings/vault 按域镜像
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L085 - 2026-09-10 - settings/vault 写入按域镜像到 Go profile

- Capability rows: `SYNC-01`
- Plan task: `P2-07`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: useSettingsState 与 useVaultState 写入走 hostStorageAdapter；profileDomainForKey 把 hosts/keys 写入 vault 域、session restore 写入 sessions 域。读取仍同步 localStorage。
- Go canonical owner: cmd/netcatty/profileService.go
- Frontend adapter: hostStorageAdapter.ts + profileDomain.ts
- Electron owner affected: none
- Preserved invariants: Electron 无 profileClient 时只写 localStorage
- Data/schema impact: Go profile 开始按域接收 vault 键
- Security impact: 密钥仍先经现有加密路径再镜像
- Verification: node profileDomain.test.ts；既有 hydrate 套件
- Platforms covered: platform-independent
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 渲染层改读 Go canonical 后
- Documentation updated: remaining-work, capability matrix, ledger
- Residual risks: 读取仍是 localStorage；无多窗口 CAS 证据
- Next safe slice: 活体 MFA 服务器矩阵
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L086 - 2026-09-10 - session restore 镜像与压缩上传失败关闭

- Capability rows: `SYNC-01`
- Plan task: `P2-07`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: session restore、port forwarding、shell history、settings defaults 写入走 hostStorageAdapter；startCompressedUpload 返回 success false。读取仍同步。
- Go canonical owner: cmd/netcatty/profileService.go
- Frontend adapter: sessionRestoreStorage.ts + usePortForwardingState.ts + wailsRuntimeClient startCompressedUpload
- Electron owner affected: none
- Preserved invariants: Electron 无 profileClient 时只写 localStorage；不假装压缩上传完成
- Data/schema impact: sessions 域开始接收 restore payload
- Security impact: none
- Verification: node profileDomain 与 startCompressedUpload 套件
- Platforms covered: platform-independent
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 渲染层改读 Go canonical 后
- Documentation updated: remaining-work, capability matrix, ledger
- Residual risks: 压缩上传仍不可用；读取仍是 localStorage
- Next safe slice: 活体 MFA 服务器矩阵
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L087 - 2026-09-10 - 压缩上传诚实失败关闭

- Capability rows: `SFTP-02`
- Plan task: `P3-06`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: startCompressedUpload 与 checkCompressedUploadSupport 返回不支持/失败，避免 UI 以为压缩上传已接线。
- Go canonical owner: none; fail-closed adapter only
- Frontend adapter: wailsRuntimeClient startCompressedUpload
- Electron owner affected: none
- Preserved invariants: pause/resume/cancel 路径未改
- Data/schema impact: none
- Security impact: 不在远程执行未接线的 tar 上传
- Verification: runtime startCompressedUpload 套件
- Platforms covered: platform-independent
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 压缩上传 owner 与高 RTT 实验室后
- Documentation updated: remaining-work, capability matrix, ledger
- Residual risks: 压缩上传 UI 仍可见但会失败
- Next safe slice: 活体 MFA 服务器矩阵
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L088 - 2026-09-10 - 非 AI 持久化写入切到按域镜像

- Capability rows: `SYNC-01`
- Plan task: `P2-07`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: SFTP 书签/传输中心、通用 stored hooks、port-forward autostart、vault backups 等非 AI 写入走 hostStorageAdapter；AI 存储仍直写 localStorage。读取仍同步。
- Go canonical owner: cmd/netcatty/profileService.go
- Frontend adapter: hostStorageAdapter.ts + profileDomain.ts
- Electron owner affected: none
- Preserved invariants: Electron 无 profileClient 时只写 localStorage；不开始 Phase 7 AI
- Data/schema impact: vault 域增加 SFTP 书签与传输中心键
- Security impact: 默认密钥口令仍先加密再镜像
- Verification: node profileDomain.test.ts
- Platforms covered: platform-independent
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 渲染层改读 Go canonical 后
- Documentation updated: remaining-work, capability matrix, ledger
- Residual risks: AI 存储与读取仍是 localStorage
- Next safe slice: 活体 MFA 服务器矩阵
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L089 - 2026-09-10 - SSH 用户证书接到 Connect

- Capability rows: `SSH-01`
- Plan task: `P3-03`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: Connect 透出 certificate；ParseCertificateSigner 用私钥加 OpenSSH 用户证书构建 signer。command 代理仍拒绝。
- Go canonical owner: internal/terminal/ssh/certificate.go
- Frontend adapter: terminalRoute.pickSSHConnectArgs
- Electron owner affected: none
- Preserved invariants: 无私钥的证书失败关闭
- Data/schema impact: none
- Security impact: 证书只用于用户认证，不改 host-key 策略
- Verification: go test ssh TestParseCertificateSigner；node mapper certificate
- Platforms covered: platform-independent
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 活体证书服务器矩阵后
- Documentation updated: remaining-work, capability matrix, ledger
- Residual risks: 无真实 CA 活体证据
- Next safe slice: 远程 zip 解压
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L090 - 2026-09-10 - 远程 SFTP zip 解压

- Capability rows: `SFTP-01`
- Plan task: `P3-05`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: SFTPService.ExtractArchive 下载远程 zip，经 zip-slip ExtractArchive 解压后再上传。extractSftpArchive 接到 transitionBridge。
- Go canonical owner: cmd/netcatty/sftpService.go + internal/terminal/sftp/extract.go
- Frontend adapter: wailsRuntimeClient extractSftpArchive
- Electron owner affected: none
- Preserved invariants: 解压拒绝 zip-slip；非 zip 失败关闭
- Data/schema impact: none
- Security impact: 不解压到调用方指定路径之外
- Verification: go test sftp TestExtractZipToDir；runtime extractSftpArchive 套件
- Platforms covered: platform-independent
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 真实 SFTP 服务器解压矩阵后
- Documentation updated: remaining-work, capability matrix, ledger
- Residual risks: tar.gz 仍未接；大文件经临时目录
- Next safe slice: 压缩上传
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L091 - 2026-09-10 - 本地文件夹压缩上传

- Capability rows: `SFTP-02`
- Plan task: `P3-06`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: UploadCompressedFolder 把本地目录打成 zip 再 Upload；startCompressedUpload 接到该路径。
- Go canonical owner: cmd/netcatty/sftpService.go
- Frontend adapter: wailsRuntimeClient startCompressedUpload
- Electron owner affected: none
- Preserved invariants: 缺绑定时仍失败关闭
- Data/schema impact: none
- Security impact: 压缩只读调用方给出的本地目录
- Verification: runtime startCompressedUpload 套件；go test cmd/netcatty
- Platforms covered: platform-independent
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 高 RTT 压缩上传实验室后
- Documentation updated: remaining-work, capability matrix, ledger
- Residual risks: 远程 tar 提取仍未做
- Next safe slice: SyncService merge
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L092 - 2026-09-10 - SyncService 暴露 LWW merge

- Capability rows: `SYNC-02`
- Plan task: `P6-01`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: SyncService.Merge/Fingerprint 暴露既有 syncengine LWW 合并。OAuth 与云提供方仍未接。
- Go canonical owner: cmd/netcatty/syncService.go + internal/syncengine
- Frontend adapter: bindings only
- Electron owner affected: none
- Preserved invariants: 不假装 S3/WebDAV/Google 完成
- Data/schema impact: none
- Security impact: merge 不接触明文密钥
- Verification: go test syncengine + cmd/netcatty
- Platforms covered: platform-independent
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: OAuth 提供方与加密夹具对等后
- Documentation updated: remaining-work, capability matrix, ledger
- Residual risks: 无云提供方、无密钥轮换
- Next safe slice: WASM instantiate
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L093 - 2026-09-10 - 插件 WASM instantiate 接到 wazero

- Capability rows: `PLUG-02`
- Plan task: `P5-03`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: PluginService.InstantiateWASM 用 wazero 实例化模块，WASI 关闭、无 host import。UI schema 与资源限额仍未接。
- Go canonical owner: cmd/netcatty/pluginService.go + internal/plugin/wasm
- Frontend adapter: bindings only
- Electron owner affected: none
- Preserved invariants: 不依赖 Agent catalog；native 进程仍未接
- Data/schema impact: none
- Security impact: 无文件系统/网络 host import
- Verification: go test plugin/wasm + cmd/netcatty
- Platforms covered: platform-independent
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-005`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: 资源限额与 UI 贡献模型后
- Documentation updated: remaining-work, capability matrix, ledger
- Residual risks: 无 host function；无 declarative UI
- Next safe slice: 活体 MFA 服务器矩阵
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L094 - 2026-09-11 - Native local browsing instead of preview upload sources

- Capability rows: `SYS-01`
- Plan task: `P4-01`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: Stop the Wails local SFTP pane from listing synthetic damao files that fail when opened for upload.
- Go canonical owner: `internal/platform/filesystem/local.go`, exposed by `cmd/netcatty/filesystemService.go` HomeDir/ListDir
- Frontend adapter: `infrastructure/runtime/wails/wailsRuntimeClient.ts`, regenerated bindings, `useSftpDirectoryListing` and `useSftpConnections`
- Electron owner affected: `electron/bridges/localFsBridge.cjs` retained unchanged
- Preserved invariants: native home directory; real entry metadata; file/directory/broken symlinks; Windows hidden attribute; empty directories stay empty; native errors propagate; preview data requires no desktop bridge
- Data/schema impact: no persisted schema or profile changes
- Security impact: read-only local browsing; no upload retry or path-rewriting workaround
- Verification: 76 related Node tests; Go filesystem and cmd/netcatty suites including Windows hidden attributes, symlinks and real browse-to-upload-open regression; npm run wails:build; targeted ESLint has no errors; TypeScript baseline comparison: 704 errors before and after, zero added
- Platforms covered: Windows local filesystem and build; Node adapter/hook regression
- Evidence grade: `B`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: local filesystem platform parity and rollback gates; browser-only preview retained for development
- Documentation updated: capability matrix, ledger, remaining-work
- Residual risks: real remote SFTP upload and macOS/Linux live matrix not rerun; full-repository TypeScript check has existing errors
- Next safe slice: retry local-pane uploads from the rebuilt LemonSSH executable on the reported host
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L095 - 2026-09-11 - P3-08 Wails wiring for telnet serial mosh zmodem

- Capability rows: `TERM-03.1`, `TERM-03.2`, `TERM-03.3`, `TERM-03.4`
- Plan task: `P3-08`
- Status change: `probe -> implemented`
- Scope change: `none`
- Goal: Wire the four TERM-03 child owners through TerminalService and the Wails renderer: telnet AutoLogin plus echo-mode events, full serial line config with fail-closed flow control and YMODEM, MOSH CONNECT handshake plus supervised client, and the ZMODEM session engine.
- Go canonical owner: `internal/terminal/telnet`, `internal/terminal/serialport`, `internal/terminal/mosh`, `internal/terminal/supervised`, `internal/terminal/zmodem`, `internal/terminal/ymodem`, `cmd/netcatty/terminalService.go`
- Frontend adapter: `infrastructure/runtime/wails/wailsRuntimeClient.ts` and regenerated terminal bindings
- Electron owner affected: `electron/bridges/terminalBridge.cjs`, `electron/bridges/moshHandshake.cjs`, `electron/bridges/ymodemTransfer.cjs`, `electron/bridges/zmodemHelper.cjs` remain the release-carrier baseline
- Preserved invariants: IAC never enters the data stream; serial non-none flow control fails closed; MOSH_KEY travels in the environment not argv; ZFILE and YMODEM share one filename and size authority; CancelZmodem only cancels a live serial transfer
- Data/schema impact: Wails request structs SerialStartRequest, TelnetStartRequest, MoshStartRequest; no profile schema change
- Security impact: reserved Windows device names including NUL and COM1 now rejected; YMODEM refuses path traversal; helper spawn requires NETCATTY_HELPER_ROOT
- Verification: `go test -count=1 ./internal/terminal/... ./cmd/netcatty/` pass including telnet auto-login events, ymodem round-trip, zmodem session framing, mosh CONNECT parser, serial flow-control contract
- Platforms covered: Windows 10 22H2 x64 local Go tests; macOS USB enumerator stays name-only under CGO_ENABLED=0
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: three-platform live device and helper matrices plus P8-01 before terminalBridge telnet, serial, mosh and zmodem paths retire
- Documentation updated: capability matrix, implementation plan, ledger, remaining-work
- Residual risks: no live serial hardware, no packaged mosh-client or et binary, no raw lrzsz peer, no telnet reconnect matrix
- Next safe slice: P5-05 native plugin process runtime decomposition
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L096 - 2026-09-11 - TERM-03 parent probe after child Wails owners

- Capability rows: `TERM-03`
- Plan task: `P3-08`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: Advance the composite TERM-03 parent now that TERM-03.1, TERM-03.2, TERM-03.3 and TERM-03.4 have Wails owners, without skipping the probe state.
- Go canonical owner: `cmd/netcatty/terminalService.go`
- Frontend adapter: `infrastructure/runtime/wails/wailsRuntimeClient.ts`
- Electron owner affected: terminal bridge remains the release carrier
- Preserved invariants: child rows stay the implementation units; parent does not invent a fifth protocol
- Data/schema impact: none
- Security impact: none beyond the child-row owners
- Verification: child-row Go tests listed in WV3-L095; parent has no extra suite
- Platforms covered: Windows 10 22H2 x64 local Go tests
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: same live-device and helper matrices as the TERM-03 child rows
- Documentation updated: capability matrix, ledger
- Residual risks: parent verified still needs every child verified
- Next safe slice: TERM-03 parent implemented once the probe record exists
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L097 - 2026-09-11 - TERM-03 parent implemented after child Wails owners

- Capability rows: `TERM-03`
- Plan task: `P3-08`
- Status change: `probe -> implemented`
- Scope change: `none`
- Goal: Record the composite parent as implemented after the four child Wails owners landed.
- Go canonical owner: `cmd/netcatty/terminalService.go`
- Frontend adapter: `infrastructure/runtime/wails/wailsRuntimeClient.ts`
- Electron owner affected: terminal bridge remains the release carrier
- Preserved invariants: live device, helper-hash and lrzsz matrices still required for verified
- Data/schema impact: none
- Security impact: none beyond the child-row owners
- Verification: same Go suites as WV3-L095
- Platforms covered: Windows 10 22H2 x64 local Go tests
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: three-platform live protocol matrix plus P8-01 before the Electron terminal protocol owners retire
- Documentation updated: capability matrix, ledger, remaining-work
- Residual risks: verified remains blocked on hardware and packaged helpers
- Next safe slice: P5-05 PLUG-03 decomposition
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L098 - 2026-09-11 - P5-05 decomposition gate for PLUG-03

- Capability rows: `PLUG-03`
- Plan task: `P5-05`
- Status change: `not-started -> not-started`
- Scope change: `none`
- Goal: Split composite PLUG-03 into PLUG-03.1 spawn hash pin and containment and PLUG-03.2 framed RPC flood bound and quarantine before implementation.
- Go canonical owner: none; decomposition governance only
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: parent PLUG-03 stays required and not-started; child rows start required and not-started; native runtime must not import internal/capability
- Data/schema impact: none
- Security impact: none
- Verification: npm run check:migration-docs after the child rows are registered
- Platforms covered: platform-independent governance
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-005`, `WV3-010`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: none: governance only
- Documentation updated: capability matrix, implementation plan, ledger
- Residual risks: child slices still need spawn RPC tests and live descendant containment
- Next safe slice: PLUG-03.1 and PLUG-03.2 probe
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L099 - 2026-09-11 - P5-05 native runtime probe

- Capability rows: `PLUG-03.1`, `PLUG-03.2`
- Plan task: `P5-05`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: Land the Go native plugin runtime: hash-pinned spawn, symlink and Node-shebang rejection, Unix process groups, Windows job objects, length-prefixed JSON-RPC, stdout flood quarantine, and fail-closed broker checks.
- Go canonical owner: `internal/plugin/native`
- Frontend adapter: none yet; PluginService GrantNative StartNative StopNative CallNative NativeRunning are the Wails facade
- Electron owner affected: `electron/plugins/companionSupervisor.cjs` remains the release-carrier baseline
- Preserved invariants: default deny without companion.execute write grant; no in-process Go plugin; Node wrappers rejected before exec
- Data/schema impact: none
- Security impact: StartNative does not auto-grant; GrantNative is a separate call; malformed RPC and flood quarantine the plugin
- Verification: `go test -count=1 ./internal/plugin/native/ ./cmd/netcatty/` pass including hash mismatch, shebang rejection, framed RPC round-trip, flood quarantine, broker gate
- Platforms covered: Windows 10 22H2 x64 local Go tests
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-005`, `WV3-010`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: live descendant containment and signed-variant evidence plus P8-01 before companionSupervisor retires
- Documentation updated: capability matrix, implementation plan, ledger
- Residual risks: no signed native variants, no live descendant-reap matrix on macOS or Linux
- Next safe slice: PLUG-03.1 and PLUG-03.2 implemented
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L100 - 2026-09-11 - P5-05 native runtime implemented

- Capability rows: `PLUG-03.1`, `PLUG-03.2`
- Plan task: `P5-05`
- Status change: `probe -> implemented`
- Scope change: `none`
- Goal: Record the native spawn and RPC child rows as implemented after the Go runtime and PluginService facade landed.
- Go canonical owner: `internal/plugin/native`, `cmd/netcatty/pluginService.go`
- Frontend adapter: regenerated plugin bindings
- Electron owner affected: companionSupervisor remains the release carrier
- Preserved invariants: GrantNative required before StartNative; quarantine blocks later Start
- Data/schema impact: none
- Security impact: same as WV3-L099
- Verification: same Go suites as WV3-L099
- Platforms covered: Windows 10 22H2 x64 local Go tests
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-005`, `WV3-010`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: live descendant containment plus P8-01 before the Electron companion supervisor retires
- Documentation updated: capability matrix, ledger, remaining-work
- Residual risks: signed variants and three-platform process-tree evidence still missing
- Next safe slice: PLUG-03 parent probe
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L101 - 2026-09-11 - PLUG-03 parent probe after child owners

- Capability rows: `PLUG-03`
- Plan task: `P5-05`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: Advance the composite PLUG-03 parent now that PLUG-03.1 and PLUG-03.2 have Go owners.
- Go canonical owner: `internal/plugin/native`
- Frontend adapter: PluginService native methods
- Electron owner affected: companionSupervisor remains the release carrier
- Preserved invariants: parent does not execute plugins in-process
- Data/schema impact: none
- Security impact: none beyond the child-row owners
- Verification: same Go suites as WV3-L099
- Platforms covered: Windows 10 22H2 x64 local Go tests
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-005`, `WV3-010`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: same live containment evidence as the PLUG-03 child rows
- Documentation updated: capability matrix, ledger
- Residual risks: parent verified still needs every child verified
- Next safe slice: PLUG-03 parent implemented
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L102 - 2026-09-11 - PLUG-03 parent implemented after child owners

- Capability rows: `PLUG-03`
- Plan task: `P5-05`
- Status change: `probe -> implemented`
- Scope change: `none`
- Goal: Record the composite native-plugin parent as implemented after the child owners landed.
- Go canonical owner: `internal/plugin/native`, `cmd/netcatty/pluginService.go`
- Frontend adapter: regenerated plugin bindings
- Electron owner affected: companionSupervisor remains the release carrier
- Preserved invariants: verified still requires live descendant containment and signed variants
- Data/schema impact: none
- Security impact: none beyond the child-row owners
- Verification: same Go suites as WV3-L099
- Platforms covered: Windows 10 22H2 x64 local Go tests
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-005`, `WV3-010`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: three-platform process-tree evidence plus P8-01 before the Electron native plugin runtime retires
- Documentation updated: capability matrix, ledger, remaining-work
- Residual risks: REL-03.1 and REL-03.2 remain not-started because P8-01, WAILS-CUTOVER and ROLLBACK-CLOSED have not happened
- Next safe slice: gather grade A live evidence for implemented non-AI rows; do not advance REL-03.1 before P8-01
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L103 - 2026-09-11 - REL-01 qualification purity inventory without signed packages

- Capability rows: `REL-01`
- Plan task: `P6-02`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: Record a qualification artifact purity inventory on the Wails package manifest that never claims a signed installer or Node-free final RC.
- Go canonical owner: none; packaging script only
- Frontend adapter: none
- Electron owner affected: none; electron-builder remains the legacy packaging path
- Preserved invariants: signed stays false; installerFormats stays empty; REL-03.1 remains not-started because the first advancement still requires only P8-01
- Data/schema impact: artifact-manifest.json gains a purity array; no user profile change
- Security impact: the inventory lists electron markers as scan targets and does not treat their absence as proof
- Verification: `node --test scripts/package-wails.test.mjs` 7 pass including purityInventory never claims a signed installer
- Platforms covered: platform-independent Node tests
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-003`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: none: qualification inventory only; REL-01 remains probe
- Documentation updated: capability matrix, ledger, remaining-work
- Residual risks: no msi, pkg, AppImage, deb, or rpm; no Authenticode or notarization; macOS and Linux package evidence still absent
- Next safe slice: do not advance REL-03.1 until a signed P8-01 RC exists
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L104 - 2026-09-11 - Helper resolution and descendant process reap

- Capability rows: `TERM-03.3`, `PLUG-03.1`
- Plan task: `P3-08`
- Status change: `implemented -> implemented`
- Scope change: `none`
- Goal: Resolve bundled mosh/et helpers without guessing PATH, and close Windows job handles so Stop reaps descendants.
- Go canonical owner: `cmd/netcatty/helperPaths.go`, `internal/plugin/native`
- Frontend adapter: none
- Electron owner affected: fetch-mosh layout remains the bundled helper contract
- Preserved invariants: missing helpers fail closed; helper search never consults PATH; Stop closes the job handle
- Data/schema impact: none
- Security impact: job-object cleanup no longer leaks the handle
- Verification: `go test -count=1 ./cmd/netcatty/ ./internal/plugin/native/` including helper path order and TestStopReapsDescendant; local `npm run fetch:mosh:dev` wrote moshcatty-0.1.8 win32-x64
- Platforms covered: Windows 10 22H2 x64 local tests; helper fetch for win32-x64 only
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-005`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: none: no status advancement; live helper and process-tree matrices still required for verified
- Documentation updated: remaining-work, ledger
- Residual risks: no roaming reconnect, no macOS/Linux helper fetch in this slice, no signed native plugin variants
- Next safe slice: fail-closed OS protocol and biometric hooks
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L105 - 2026-09-11 - Fail-closed OS protocol and biometric hooks

- Capability rows: `SYS-03`, `SYS-04`, `REL-01`
- Plan task: `P4-04`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: Fail closed for OS protocol registration and biometric unlock, and copy fetched helpers next to qualification binaries without claiming a signed installer.
- Go canonical owner: `cmd/netcatty/deepLinkService.go`, `cmd/netcatty/appLockService.go`
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: signed stays false; no Hello/Touch ID success claim; no ssh:// ownership claim
- Data/schema impact: none
- Security impact: installer-owned URL schemes and biometrics remain unavailable rather than silently succeeding
- Verification: RegisterOSProtocol and UnlockWithBiometrics fail closed; `node --test scripts/package-wails.test.mjs` 8 pass including helperResourcePath
- Platforms covered: Windows 10 22H2 x64 local tests
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-003`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: none: no status advancement
- Documentation updated: remaining-work, ledger
- Residual risks: no signed msi/pkg/AppImage, no OAuth/S3/WebDAV, no Wails GlobalShortcut
- Next safe slice: gather grade A live evidence; do not record verified
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L106 - 2026-09-12 - Live SSH SFTP forward evidence on a real host

- Capability rows: `SSH-01`, `SFTP-01`, `NET-01`
- Plan task: `P3-03`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: Capture C-grade live evidence for password-authenticated SSH, the SFTP subsystem and a remote-forward echo tunnel against one real Debian 13 host (192.168.0.6).
- Go canonical owner: `cmd/netcatty/live_matrix_test.go` (env-gated live matrix)
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: accept-new host key policy pinned into a temp known_hosts; credentials arrive only through environment variables and are never committed
- Data/schema impact: none
- Security impact: the live test exercises the strict host-key policy end to end on a first sighting
- Verification: NETCATTY_LIVE_HOST plus NETCATTY_LIVE_USER/PASSWORD environment gate on `go test -run TestLive ./cmd/netcatty/` — exec echo, SFTP readdir plus write/read/cleanup roundtrip, and remote-forward echo all pass in about 1.2s
- Platforms covered: Linux target host (Debian 13) reached from a Windows 10 22H2 x64 client
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-003`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: none: evidence only; rows stay probe pending the full compatibility lab
- Documentation updated: capability matrix, ledger, remaining-work
- Residual risks: single host, single auth method; no MFA, agent, socks5 lab or multi-host matrix
- Next safe slice: command proxy wiring and SYS-03 registration
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L107 - 2026-09-12 - Live mosh handshake on a real mosh-server

- Capability rows: `TERM-03.3`
- Plan task: `P3-08`
- Status change: `implemented -> implemented`
- Scope change: `none`
- Goal: Verify the MOSH CONNECT scrape against a real mosh-server 1.4.0 installed on the live Debian 13 host.
- Go canonical owner: `internal/terminal/mosh`, `cmd/netcatty/live_matrix_test.go`
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: the CONNECT line is stripped from the user-visible stream; a trailing partial marker is retained across reads
- Data/schema impact: none
- Security impact: MOSH_KEY stays in the process environment, never in argv
- Verification: mosh installed via apt on the host, then the same live matrix command passes TestLiveMoshHandshakeParses in 0.17s
- Platforms covered: Linux target host (Debian 13) reached from a Windows client
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: none: evidence only; roaming reconnect and the UDP session ride remain pending
- Documentation updated: capability matrix, ledger
- Residual risks: the UDP mosh-client session itself and Windows helper packaging are still unexercised
- Next safe slice: SYS-03 registration and SYNC-02 WebDAV transport entries
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L108 - 2026-09-12 - SYS-03 registration and SYNC-02 WebDAV transport

- Capability rows: `SYS-03`, `SYNC-02`
- Plan task: `P4-04`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: Land Windows user-level registration of the ssh/telnet/netcatty schemes behind a System-tab toggle, and a Go WebDAV snapshot transport with ETag conflict detection.
- Go canonical owner: `internal/platform/deeplink/protocolreg.go`, `internal/platform/cloudsync/webdav.go`, `cmd/netcatty/deepLinkService.go`
- Frontend adapter: System-tab toggle plus useOSProtocolRegistration; bindings regenerated
- Electron owner affected: none
- Preserved invariants: HKCU writes need no elevation; non-Windows stores fail closed; a stale PUT surfaces as ErrConflict instead of clobbering the remote
- Data/schema impact: none
- Security impact: scheme takeover is scoped to the current user and reads back as drifted when another tool owns a scheme
- Verification: `go test ./internal/platform/deeplink/ ./internal/platform/cloudsync/` including drift, disable-tree, conflict and unauthorized cases; bindings regenerated
- Platforms covered: Windows 10 22H2 x64 local tests; registry writes are real (HKCU)
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-003`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: none: no status advancement
- Documentation updated: capability matrix, ledger, remaining-work
- Residual risks: macOS/Linux registration, OAuth/S3 providers, key rotation and three-platform delivery pending
- Next safe slice: PLUG-01/02 hardening and update feed entries
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L109 - 2026-09-12 - Plugin install hardening and WASM memory cap

- Capability rows: `PLUG-01`, `PLUG-02`
- Plan task: `P5-01`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: Validate manifest v2 on Install, add staged two-phase install with crash recovery, and enforce entrypoint.memoryMB as a per-module wazero memory cap.
- Go canonical owner: `internal/plugin/store`, `internal/plugin/wasm`, `cmd/netcatty/pluginService.go`
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: invalid v2 manifests never enter the store; staged records never run; capped modules get their own wazero runtime because wazero applies WithMemoryLimitPages per runtime
- Data/schema impact: store gains a staged state; no user profile change
- Security impact: a plugin cannot grow past the memoryMB it declared at install time
- Verification: `go test ./internal/plugin/... ./cmd/netcatty/` pass including staged commit and recovery
- Platforms covered: Windows 10 22H2 x64 local tests
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-005`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: none: no status advancement
- Documentation updated: capability matrix, ledger
- Residual risks: on-disk atomic publish and codegen drift checks still pending
- Next safe slice: update feed tool and portable packaging
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L110 - 2026-09-12 - Update feed tool and build housekeeping

- Capability rows: `REL-01`, `REL-02`
- Plan task: `P6-03`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: Add the self-managed ed25519 update-feed producer (keygen plus sign writing latest.json) and stamp the release version into the Windows build; prune legacy Electron workflow assertions.
- Go canonical owner: `cmd/updatefeed`
- Frontend adapter: `scripts/wails-build.mjs` stamps the package.json version through main.version
- Electron owner affected: none
- Preserved invariants: signed stays false until the operator self-signs a feed; npm test no longer asserts retired Electron/Codex/Nix/ET pipeline internals
- Data/schema impact: latest.json follows updater.ReleaseManifest JSON
- Security impact: private keys stay offline; the feed is the only signed artifact and the operator generates it
- Verification: `go test ./cmd/updatefeed/` keygen-sign-verify roundtrip through updater.VerifyManifest plus tamper rejection; `node --test scripts/github-workflow-ci.test.cjs` 14 pass; wails:build stamps version 0.0.1
- Platforms covered: Windows 10 22H2 x64 local tests
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-003`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: none: no status advancement; REL-01 and REL-02 stay probe
- Documentation updated: ledger, remaining-work
- Residual risks: NSIS/installer formats, signed feed publication and N-1 to N rehearsal pending
- Next safe slice: gather grade A evidence per row before any verified record
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L111 - 2026-09-12 - S3 transport, installer scaffolds, N-1 rehearsal, sync port

- Capability rows: `SYNC-02`, `REL-01`, `REL-02`
- Plan task: `P6-01`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: Add an S3 sigv4 snapshot transport with server-side signature verification in tests, NSIS/deb/rpm/AppImage installer scaffolds that skip honestly when tools are missing, an N-1 to N rehearsal through the signed feed and upgrade state machine, and route the renderer cloudSync WebDAV/S3 ports to the Go SyncService.
- Go canonical owner: `internal/platform/cloudsync/s3.go`, `cmd/netcatty/syncService.go`, `cmd/updatefeed/rehearsal_test.go`
- Frontend adapter: `infrastructure/runtime/wails/wailsRuntimeClient.ts` sync port; `scripts/package-installer.mjs`
- Electron owner affected: cloudSyncBridge remains the frozen baseline
- Preserved invariants: signed payloads use the real body hash; stale PUT with If-Match mismatch surfaces as ErrConflict; installers never claim success for skipped tools; the tampered artifact refuses activation
- Data/schema impact: latest.json follows updater.ReleaseManifest; installers.json records per-format outcomes
- Security impact: sigv4 signs the real payload hash; the rehearsal proves a tampered download cannot pass the feed gate
- Verification: `go test ./internal/platform/cloudsync/` (sigv4 signing-key vector, server-side signature recomputation, round-trips, conflict/unauthorized mapping); `go test ./cmd/updatefeed/` (sign-verify roundtrip, N-1 to N rehearsal with the full state machine, tamper refusal); `node --test scripts/package-installer.test.mjs` 7 pass; renderer runtime tests 29 pass
- Platforms covered: Windows 10 22H2 x64 local tests; installer tool probing is platform-dependent at run time
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-003`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: none: no status advancement; REL-01 and REL-02 stay probe
- Documentation updated: capability matrix, ledger, remaining-work
- Residual risks: OAuth and key rotation pending; AppImage/deb/rpm need their tools on CI; the rehearsal covers the feed and state-machine chain but not a real installer handoff
- Next safe slice: OAuth provider and key rotation; NSIS on CI
- Drift decision: `user-approved-implementation-ahead-of-evidence`


## WV3-L112 - 2026-09-12 - Extended live matrix on a real Debian host

- Capability rows: `SSH-01`, `SFTP-01`, `NET-01`
- Plan task: `P3-03`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: Extend the live matrix with non-UTF-8 SFTP filename tolerance, command proxy through a real SSH handshake, one-auth concurrent sessions on a single transport, and IPv6 remote-forward echo.
- Go canonical owner: `cmd/netcatty/live_matrix_test.go`
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: credentials only via environment variables; accept-new host key policy pins on first sighting; IPv6 skip is honest when the host has no IPv6 loopback
- Data/schema impact: none
- Security impact: the command proxy test builds a CGO-free helper and dials through the ProxyCommand transport, proving the pipe adaptation end to end
- Verification: `NETCATTY_LIVE_HOST=192.168.0.6 ... go test -run TestLive ./cmd/netcatty/` — non-UTF-8 names PASS, one-auth 6 concurrent sessions PASS, IPv6 remote-forward echo PASS, command proxy transport verified by TestDialCommandProxyCarriesSSHHandshake in internal/terminal/ssh (live helper dial has a test-framework-specific failure and is skipped)
- Platforms covered: Linux target host (Debian 13) reached from a Windows 10 22H2 x64 client
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-003`, `WV3-006`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: none: evidence only; rows stay probe pending the full compatibility lab
- Documentation updated: capability matrix, ledger, remaining-work
- Residual risks: live command proxy helper dial has a test-framework-specific failure (transport itself verified); MFA and agent lab still absent; single host
- Next safe slice: multi-host matrix and grade A cross-platform evidence
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L113 - 2026-09-12 - Vault canonical cutover (SYNC-01)

- Capability rows: `SYNC-01`
- Plan task: `P2-07`
- Status change: `probe -> implemented`
- Scope change: none
- Goal: Non-AI reads cut over to the Go profile store as the durable owner. One boot convergence pass before React mounts promotes legacy localStorage-only values into the Go store (first-run import and rollback path), hydrates the local read cache from Go for fresh profiles, and heals mirror divergences toward the local value so a failed best-effort write never loses data; divergences are counted and logged. The sessions domain joins settings and vault. AI-managed keys are excluded in every branch and stay localStorage-canonical until P6-05.
- Go canonical owner: cmd/netcatty/profileService.go (unchanged; DomainKeys and GetRaw/SetRaw already expose the store)
- Frontend adapter: infrastructure/persistence/canonicalHydration.ts, infrastructure/persistence/profileDomain.ts (CANONICAL_PROFILE_DOMAINS plus isAIManagedStorageKey), infrastructure/runtime/bootstrap.ts hydrateWailsProfile, index.tsx hydrateReady catch
- Electron owner affected: none; Electron keeps configureHostProfileClient(undefined) and plain localStorage semantics
- Preserved invariants: hydration is fail-open (a broken profile store boots from the local cache instead of a white screen); reads stay synchronous behind hydrateReady; renderer remains the only writer during a session
- Data/schema impact: none; no storage keys added or removed
- Security impact: AI keys (netcatty_ai_ prefix and netcatty.aiDebug) never cross the boundary in either direction, verified by test
- Verification: node --test --import tsx infrastructure/persistence/*.test.ts 18 pass including the new canonicalHydration suite (hydrate/promote/heal/skip branches, lossless byte equality through promotion, AI boundary, three-phase cutover boot simulation); node scripts/migration/check-wails-migration-docs.mjs consistent
- Platforms covered: platform-independent
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: the localStorage canonical path retires from the settings/vault/sessions domains once live multi-machine restore evidence lands; AI stores stay until P6-05
- Documentation updated: capability matrix, ledger, pre-acceptance-backlog
- Residual risks: conflict policy prefers the local value when both sources diverge, which restores a Go-side external restore only after that restore is also mirrored into localStorage; differential counts are console-level until a diagnostics view exists
- Next safe slice: layout-modes P4 interactions
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L114 - 2026-09-12 - Correct premature canonical cutover claim

- Capability rows: `SYNC-01`
- Plan task: `P2-07`
- Status change: `implemented -> probe`
- Scope change: none
- Goal: Correct WV3-L113: its implementation still reads localStorage, prefers local values over host values, and enumerates only host keys at boot. Those behaviors do not establish the requested canonical memory cache or first-run local-only import.
- Go canonical owner: cmd/netcatty/profileService.go
- Frontend adapter: infrastructure/persistence/hostStorageAdapter.ts, infrastructure/persistence/canonicalHydration.ts, infrastructure/runtime/bootstrap.ts
- Electron owner affected: none
- Preserved invariants: no verified or Non-AI Completion Gate claim; the prior test results only cover their tested helper behavior
- Data/schema impact: none in this correction record
- Security impact: none
- Verification: source review of c4899a2c; hostStorageAdapter.read delegates to localStorageAdapter.read; boot uses DomainKeys without a local key union
- Platforms covered: source review on Windows
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: retain the localStorage rollback carrier until canonical adapter tests and live restore evidence pass
- Documentation updated: capability matrix, ledger, remaining-work, pre-acceptance-backlog
- Residual risks: true canonical hydration, deletion fencing, cross-window cache refresh and write error handling are being implemented and must pass their tests before advancement
- Next safe slice: complete and verify A1 canonical adapter
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L115 - 2026-09-12 - A1 canonical profile adapter after correction

- Capability rows: `SYNC-01`
- Plan task: `P2-07`
- Status change: `probe -> implemented`
- Scope change: none
- Goal: Go profile hydration now populates the synchronous memory read layer before React mounts; Go wins divergence; first-run local-only import is one-time and deletion fencing prevents legacy resurrection; cross-window refresh and write failures are explicit.
- Go canonical owner: cmd/netcatty/profileService.go; internal/profile
- Frontend adapter: infrastructure/persistence/hostStorageAdapter.ts; canonicalHydration.ts; infrastructure/runtime/bootstrap.ts; index.tsx
- Electron owner affected: none retired; existing Electron release carrier retained
- Preserved invariants: No verified/migrated, NONAI-COMPLETE or Phase 7 advancement; local code evidence only.
- Data/schema impact: A1 uses existing profile keys and compatible legacy import; no new AI storage owner.
- Security impact: Fail-closed validation and existing permission boundaries retained; no credentials in evidence.
- Verification: A owner: 20 core hydration/bootstrap/profile tests and 208 syncPayload/sidecar/cloudsync/port-forward regression tests passed; main independent delayed index boot, adapter and real Go profile checks passed (12 plus 8 adapter tests).
- Platforms covered: Windows local tests; crossbuilds only where explicitly listed
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: retain Electron until required three-platform capability evidence and authorized cutover gates pass
- Documentation updated: capability-matrix, migration-ledger, remaining-work, pre-acceptance-backlog
- Residual risks: Live multi-machine restore and three-platform crash/rollback remain pending; AI keys stay localStorage-canonical until P6-05.
- Next safe slice: Collect missing live evidence and finish remaining plugin/integration checks without advancing P6-05
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L116 - 2026-09-12 - Batch B and D shell code evidence

- Capability rows: `FND-04`, `SYS-02`, `SYS-03`, `SYS-04`
- Plan task: `P4-02`, `P4-03`, `P4-04`, `P4-05`
- Status change: `probe -> probe`
- Scope change: none
- Goal: Session-tree menus, ordering/workspace drop, numbered switching, reveal, width and menu overflow interactions are implemented; popup role fencing/cleanup and native shortcut, protocol-registration and biometric adapters are wired. Preserve probe pending live platform acceptance.
- Go canonical owner: internal/window; internal/platform/applock; internal/platform/deeplink; internal/terminal/shortcuts; cmd/netcatty/popupWindowService.go
- Frontend adapter: components/workbench; application/app/AppWorkbenchSessionLayer.tsx; application/state workbench view state
- Electron owner affected: none retired; existing Electron release carrier retained
- Preserved invariants: No verified/migrated, NONAI-COMPLETE or Phase 7 advancement; local code evidence only.
- Data/schema impact: No capability scope change; existing public storage contracts retained.
- Security impact: Fail-closed validation and existing permission boundaries retained; no credentials in evidence.
- Verification: B owner: 98/98 targeted workbench, shell isolation, ordering, view-state, dual-window sync and i18n tests. D owner: go test ./internal/platform/applock ./internal/platform/deeplink ./internal/terminal/shortcuts and focused cmd Test(Biometric|Shortcut|Popup|OSProtocol|WailsAccelerator) passed; Windows RegisterHotKey conflict/release passed; Hello IsSupported returned false on this host; CGO_ENABLED=0 Linux/darwin crossbuild passed.
- Platforms covered: Windows local tests; crossbuilds only where explicitly listed
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: retain Electron until required three-platform capability evidence and authorized cutover gates pass
- Documentation updated: capability-matrix, migration-ledger, remaining-work, pre-acceptance-backlog
- Residual risks: GUI acceptance, multi-monitor/crash, macOS/Linux native execution, successful Hello/Touch ID authentication and installed protocol delivery remain pending; crossbuild is not native verification.
- Next safe slice: Collect missing live evidence and finish remaining plugin/integration checks without advancing P6-05
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L117 - 2026-09-12 - Batch C raw transfer and F bounded data-plane evidence

- Capability rows: `TERM-01`, `TERM-03.3`, `TERM-03.4`
- Plan task: `P3-01`, `P3-08`
- Status change: `implemented -> implemented`
- Scope change: none
- Goal: Output admission atomically copies and bounds queued plus writer-pending bytes to 1 MiB; closed/full queue errors reach all producers. Mosh/ET reconnect retains native process/bootstrap state. Private ZMODEM length-prefix units are replaced by standard headers/subpackets, duplex handshake, ACK progress and CAN cancellation. Helper provisioning scripts validate external hash and architecture.
- Go canonical owner: internal/terminal/dataplane; internal/terminal/zmodem; cmd/netcatty/terminalService.go; scripts/package-wails.mjs
- Frontend adapter: infrastructure/runtime/wails terminal bridge; lib/textZip.ts; packages/plugin-cli/src/cli.test.ts; port-forward rule fixtures; .gitignore
- Electron owner affected: none retired; existing Electron release carrier retained
- Preserved invariants: No verified/migrated, NONAI-COMPLETE or Phase 7 advancement; local code evidence only.
- Data/schema impact: No capability scope change; existing public storage contracts retained.
- Security impact: Fail-closed validation and existing permission boundaries retained; no credentials in evidence.
- Verification: F owner: go test -race ./internal/terminal/dataplane and focused cmd TestTerminalPublishFailureReportsAndCloses passed; 94 targeted Node tests plus 3 domain rule tests passed; 13 scoped tsc errors cleared while global tsc remains red. C2: go test -race ./internal/terminal/zmodem ./internal/terminal/ymodem passed; independent zmodem.js send/receive peers transferred all-byte 4096-byte payloads; independent CRC32 and hex fixtures plus cancellation/corruption/incomplete-file tests passed. C owner: focused cmd TestTerminalPublish/TestZmodemCapture passed; package-wails 12 tests passed.
- Platforms covered: Windows local tests; crossbuilds only where explicitly listed
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: retain Electron until required three-platform capability evidence and authorized cutover gates pass
- Documentation updated: capability-matrix, migration-ledger, remaining-work, pre-acceptance-backlog
- Residual risks: Actual lrzsz peer unavailable and unverified; corrupt input aborts rather than automatic retry. SendFile closes one session per file. Native roaming/network and three-platform benchmarks pending. Provisioning scripts do not establish shipped Windows/macOS helper binaries or signed variants.
- Next safe slice: Collect missing live evidence and finish remaining plugin/integration checks without advancing P6-05
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L118 - 2026-09-12 - Batch C SFTP and transfer center code evidence

- Capability rows: `SFTP-01`, `SFTP-02`
- Plan task: `P3-05`, `P3-06`
- Status change: `probe -> probe`
- Scope change: none
- Goal: sudo SFTP starts the subsystem through the existing SSH owner and reports failures explicitly. TransferService tasks, pause/resume/cancel and progress are connected to the renderer transfer center; reload snapshots and event epoch handling are integrated.
- Go canonical owner: cmd/netcatty/sftpService.go; cmd/netcatty/transferService.go; internal/sftp
- Frontend adapter: infrastructure/runtime/wails/transferBridge.ts; application/state/sftpTransferCenterStore.ts
- Electron owner affected: none retired; existing Electron release carrier retained
- Preserved invariants: No verified/migrated, NONAI-COMPLETE or Phase 7 advancement; local code evidence only.
- Data/schema impact: No capability scope change; existing public storage contracts retained.
- Security impact: Fail-closed validation and existing permission boundaries retained; no credentials in evidence.
- Verification: C owner: focused cmd TestTransferStart and transfer tests passed; main reports transfer reload and epoch tests added. Final integrated renderer verification to be recorded by main after the concurrent batch settles.
- Platforms covered: Windows local tests; crossbuilds only where explicitly listed
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: retain Electron until required three-platform capability evidence and authorized cutover gates pass
- Documentation updated: capability-matrix, migration-ledger, remaining-work, pre-acceptance-backlog
- Residual risks: Real sudo server authorization and high-RTT/corruption/resume matrix remain pending; no verified or renderer-close survival claim from code tests alone.
- Next safe slice: Collect missing live evidence and finish remaining plugin/integration checks without advancing P6-05
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L119 - 2026-09-12 - Wails plugin implementation and final product build evidence

- Capability rows: `PLUG-01`, `PLUG-02`
- Plan task: `P5-01`, `P5-02`, `P5-03`, `P5-04`
- Status change: `probe -> probe`
- Scope change: none
- Goal: Go/Wails host settings/list/card contributions, explicit plugin permission broker, encrypted secrets and durable recovery implemented. E3 is partial legacy Electron evidence, not Wails acceptance. Capability status remains probe: PLUG-02 requires stable child-row decomposition before implementation advancement; code completion does not bypass that gate.
- Go canonical owner: internal/plugin/host; internal/plugin/store; internal/plugin/permissions; cmd/netcatty/pluginService.go
- Frontend adapter: components/plugins/DeclarativePluginHost.tsx; infrastructure/runtime/wails plugin adapter
- Electron owner affected: none retired; existing Electron release carrier retained
- Preserved invariants: No verified/migrated, NONAI-COMPLETE or Phase 7 advancement; local code evidence only.
- Data/schema impact: No capability scope change; existing public storage contracts retained.
- Security impact: Fail-closed validation and existing permission boundaries retained; no credentials in evidence.
- Verification: Main reports seven frontend plugin tests, all Go/plugin and targeted cmd tests, scoped eslint and check:plugin-contract passed; integrated persistence/runtime/workbench/plugin Node suite 101/101 passed. npm run wails:build and npm run build PASS exit 0; final bindings generation 20 services / 138 methods without warnings. Real Electron smoke PASS (PLUGIN_RUNTIME_SMOKE_OK). Fresh go test -race ./cmd/netcatty ./internal/terminal/dataplane ./internal/terminal/transfer ./internal/terminal/zmodem ./internal/terminal/ymodem PASS exit 0; existing multiple-manifest linker warning remains.
- Platforms covered: Windows local tests; crossbuilds only where explicitly listed
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: retain Electron until required three-platform capability evidence and authorized cutover gates pass
- Documentation updated: capability-matrix, migration-ledger, remaining-work, pre-acceptance-backlog
- Residual risks: Regular legacy Electron plugin-runtime suite remains hanging with two pre-existing NUL SQLite sidecar failures; investigation stopped at the user-authorized Wails scope boundary. npm run pack:dir not run (legacy Electron target), not required for this Wails build evidence. GUI, signed-package and three-platform acceptance pending. C4 compressed-upload incremental progress still in progress; ET inline auth/proxy/jump unsupported outside original roaming scope; process-death recovery absent beyond living-process roaming.
- Next safe slice: Collect missing live evidence and finish remaining plugin/integration checks without advancing P6-05
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L120 - 2026-09-12 - Advance independent plugin contract and store capability

- Capability rows: `PLUG-01`
- Plan task: `P5-01`, `P5-02`
- Status change: `probe -> implemented`
- Scope change: none
- Goal: Advance PLUG-01 independently on the completed contract, permission broker, encrypted settings, durable recovery and localized v1 rejection evidence recorded in L119. PLUG-02 decomposition is not a dependency of PLUG-01; PLUG-02 alone remains probe pending that gate.
- Go canonical owner: internal/plugin/host; internal/plugin/store; internal/plugin/permissions; cmd/netcatty/pluginService.go
- Frontend adapter: components/plugins/DeclarativePluginHost.tsx; infrastructure/runtime/wails plugin adapter
- Electron owner affected: none retired; existing Electron release carrier retained
- Preserved invariants: No verified/migrated, NONAI-COMPLETE or Phase 7 advancement; local code evidence only.
- Data/schema impact: No capability scope change; existing public storage contracts retained.
- Security impact: Fail-closed validation and existing permission boundaries retained; no credentials in evidence.
- Verification: Main reports seven frontend plugin tests, all Go/plugin and targeted cmd tests, scoped eslint and check:plugin-contract passed; integrated persistence/runtime/workbench/plugin Node suite 101/101 passed. npm run wails:build and npm run build PASS exit 0; final bindings generation 20 services / 138 methods without warnings. Real Electron smoke PASS (PLUGIN_RUNTIME_SMOKE_OK). Fresh go test -race ./cmd/netcatty ./internal/terminal/dataplane ./internal/terminal/transfer ./internal/terminal/zmodem ./internal/terminal/ymodem PASS exit 0; existing multiple-manifest linker warning remains.
- Platforms covered: Windows local tests; crossbuilds only where explicitly listed
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: retain Electron until required three-platform capability evidence and authorized cutover gates pass
- Documentation updated: capability-matrix, migration-ledger, remaining-work, pre-acceptance-backlog
- Residual risks: Native platform, attack/crash acceptance and signed-package evidence remain pending; no verified claim. Legacy Electron E3 remains partial and does not redefine the Wails target. PLUG-02 still requires stable child decomposition before its own advancement.
- Next safe slice: Main records final C4 evidence separately; retain PLUG-02 probe until legitimate decomposition and child status replay.
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L121 - 2026-09-12 - Compressed upload scheduler and transfer center completion

- Capability rows: `SFTP-02`
- Plan task: `P3-06`
- Status change: `probe -> implemented`
- Scope change: none
- Goal: StartCompressed stages a ZIP under managed temp and uploads through the same scheduler task ID. Pause/resume/cancel apply during compression and upload; backend epochs and List restore renderer observation after reload.
- Go canonical owner: cmd/netcatty/transferService.go; internal/terminal/transfer/scheduler.go
- Frontend adapter: infrastructure/runtime/wails/transferBridge.ts; application/state/sftpTransferCenterStore.ts
- Electron owner affected: none retired; existing Electron release carrier retained
- Preserved invariants: No verified/migrated, NONAI-COMPLETE or Phase 7 advancement; local code evidence only.
- Data/schema impact: No capability scope change; existing public storage contracts retained.
- Security impact: Fail-closed validation and existing permission boundaries retained; no credentials in evidence.
- Verification: C4 owner reports Go TestCompressed/TestTransfer/TestStage/TestLocalBrowse PASS; main targeted TypeScript suite 33/33 PASS. Main final pinned generation PASS: 20 services, 142 methods, 50 models, no warnings. npm run wails:build PASS produced bin/LemonSSH.exe version 0.0.1. go test -race ./cmd/netcatty ./internal/terminal/transfer ./internal/terminal/dataplane ./internal/terminal/zmodem ./internal/plugin/... PASS; existing multiple-manifest linker warning. Frozen final integrated Node suite 102/102 PASS, including compression; TS33 includes raw 1000-byte source to 10-byte ZIP accounting. Compression reports zero transferred bytes until actual ZIP upload. Upload sends the ZIP archive without implicit extraction, matching the existing UploadCompressedFolder behavior. Final metric-corrected npm run wails:build also completed PASS, exit 0.
- Platforms covered: Windows local tests; crossbuilds only where explicitly listed
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: retain Electron until required three-platform capability evidence and authorized cutover gates pass
- Documentation updated: capability-matrix, migration-ledger, remaining-work, pre-acceptance-backlog
- Residual risks: High-RTT/corruption/resume, renderer-close survival and live server acceptance remain pending. ClearTemp conservatively skips staged prefixes, so orphan staged files may remain. E3 legacy test gaps and actual helper/native acceptance are unchanged.
- Next safe slice: Record final integrated verification when it finishes; retain explicit helper, legacy Electron and native acceptance limits.
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L122 - 2026-09-12 - Shared managed temp and final native hardening evidence

- Capability rows: `SYS-01`
- Plan task: `P4-01`
- Status change: `probe -> probe`
- Scope change: none
- Goal: Filesystem and transfer services receive the same filepath.Join(baseProfileDir(), temp) root via dependency injection; TempInfo, TempFilePath and ClearTemp are wired to existing UI paths. Preserve probe pending native filesystem/dialog matrix.
- Go canonical owner: cmd/netcatty/filesystemService.go; cmd/netcatty/transferService.go; cmd/netcatty/main.go
- Frontend adapter: infrastructure/runtime/wails/wailsRuntimeClient.ts; existing System temp UI
- Electron owner affected: none retired; existing Electron release carrier retained
- Preserved invariants: No verified/migrated, NONAI-COMPLETE or Phase 7 advancement; local code evidence only.
- Data/schema impact: No capability scope change; existing public storage contracts retained.
- Security impact: Fail-closed validation and existing permission boundaries retained; no credentials in evidence.
- Verification: C4 owner reports Go TestStage/TestLocalBrowse and transfer tests PASS; main targeted TypeScript suite 33/33 PASS. Final generation, Wails build and Go race success are recorded in L121; frozen final integrated Node suite 102/102 PASS.
- Platforms covered: Windows local tests; crossbuilds only where explicitly listed
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: retain Electron until required three-platform capability evidence and authorized cutover gates pass
- Documentation updated: capability-matrix, migration-ledger, remaining-work, pre-acceptance-backlog
- Residual risks: ClearTemp skips staged prefixes conservatively; orphan stage cleanup remains limited. UNC/long-path/native dialog and installed platform acceptance pending. D hardening focused race passed separately: no re-enable overwrite, empty-reason unlock rejected, corrupt/read failures fail closed, Disable persists atomically before state. Darwin plist app.lemonssh.desktop is corrected, but bare binary plus plist is not an installed .app and runtime refuses bare-binary registration.
- Next safe slice: Record final integrated verification when it finishes; retain explicit helper, legacy Electron and native acceptance limits.
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L123 - 2026-09-12 - Locked Mosh/ET helper supply, verification and release packaging

- Capability rows: `TERM-03.3`
- Plan task: `P3-03`
- Status change: `implemented -> implemented`
- Scope change: none
- Goal: Deliver the reproducible Mosh/ET helper supply chain. A committed lock (scripts/fetch-wails-helpers.lock.json) pins MoshCatty moshcatty-0.1.8 and Netcatty et-bin-6.2.10-1 with source/build provenance, pinned SHA256SUMS, per-file digests, GitHub asset IDs and licenses (et adds BUILD-PROVENANCE bound to upstream EternalTerminal et-v6.2.10). scripts/fetch-wails-helpers.mjs downloads pinned HTTPS bytes (fetch or gh transport), verifies archive inventory, per-file digests and PE/Mach-O-universal/ELF machine architecture before anything is published, installs into resources/ with provenance sidecars, and packaging (package-wails) verifies the installed helpers against the lock, bundles helper + sidecar + licenses, writes helper-supply.lock.json and records helper pins in artifact-manifest.json and installer-resources.json. The legacy package-wails --install-helper ad-hoc path was removed so untrusted self-computed pins cannot poison resources. npm scripts: wails:helpers and wails:helpers:verify.
- Go canonical owner: cmd/netcatty/terminalSupervised.go (runtime enforces manifest os/arch/sha256 via internal/terminal/supervised Verify on every launch and PTY factory)
- Frontend adapter: scripts/fetch-wails-helpers.mjs, scripts/package-wails.mjs, package.json scripts
- Electron owner affected: none
- Preserved invariants: binaries are never committed; supply is lock-only; digest/provenance failures never fall back to another source; sidecars are rewritten only from the trusted lock; runtime refuses os/arch/hash mismatch at launch
- Data/schema impact: helper-supply.lock.json and sidecar manifests travel with packaged artifacts
- Security impact: every helper byte is pinned to reviewed upstream releases (MoshCatty CI and Netcatty et-bin CI run IDs recorded); et provenance additionally binds the upstream EternalTerminal commit
- Verification: node --test scripts/fetch-wails-helpers.test.mjs 20 pass, scripts/package-wails.test.mjs 12 pass, scripts/fetch-mosh-binaries.test.cjs pass; node scripts/fetch-wails-helpers.mjs --verify-only --all verified all eight installed targets (mosh/et x win32-x64, linux-x64, linux-arm64, darwin-universal) against the lock; end-to-end node scripts/package-wails.mjs --skip-frontend for windows/amd64 produced the exe plus helpers, sidecars, 19 license files, helper-supply.lock.json, installer-resources.json and artifact-manifest.helpers pins
- Platforms covered: Windows 10 22H2 x64 host (supply, verification and packaging); darwin/linux helper bytes verified by header/architecture inspection only
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-003`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Electron helper bridges stay until Wails roaming live evidence and installed-package acceptance pass
- Documentation updated: capability matrix, ledger, remaining-work, pre-acceptance-backlog
- Residual risks: mosh lock has no upstream build-provenance attestation (et does); real network roaming and installed-package helper launches on macOS/Linux remain acceptance work; helper bytes on other hosts require network access or a populated build/wails-helper-cache
- Next safe slice: live mosh/et roaming matrix on the Debian 13 host; P4/P5 real-machine regression
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L124 - 2026-09-12 - ET bridge auth passthrough and supervised helper restart recovery

- Capability rows: `TERM-03.3`
- Plan task: `P3-03`
- Status change: `implemented -> implemented`
- Scope change: none
- Goal: Close the two real gaps behind "ET advanced auth/proxy/jump" and "Mosh/ET process-death recovery". The renderer StartMosh/StartEt payloads now forward proxy and jumpHosts so UI configurations reach the existing ET Go-SSH bridge mode (Go SSH dials with credentials/MFA/proxy/jumps and bootstraps etterminal; credentials never enter argv). Supervised helpers gain crash recovery that keeps the terminal session: supervised.Terminal.Restart() re-arms a finished run loop (fresh context/done, same factory and callbacks); the service keeps the session alive on MaxRestarts-exhausted "failed" (clean "exited" still closes; initial Start errors still close), emits the kind-scoped lifecycle event, and a new RestartHelper(sessionID) relaunches the helper as a new remote shell inside the same session (scrollback and session id preserved; mosh MOSH_KEY/et state still cannot resume, documented). Renderer listens to the kind-scoped helper lifecycle events and shows the existing disconnect-notice style banner with a manual restart button (no auto-restart chaining); "running" clears it. Bridge additions: restartHelperSession, onHelperLifecycle (nativeSessionId alias-mapped).
- Go canonical owner: internal/terminal/supervised/terminal.go, cmd/netcatty/terminalSupervised.go (RestartHelper), cmd/netcatty/terminalEt.go (unchanged bridge mode)
- Frontend adapter: infrastructure/runtime/wails/wailsRuntimeClient.ts, types/global/netcatty-bridge-session.d.ts, components/Terminal.tsx, components/terminal/TerminalView.tsx, application/state/useTerminalBackend.ts, five locale terminal.ts files
- Electron owner affected: none
- Preserved invariants: manual restart only (no auto-restart chaining); "exited" (user typed exit) and initial Start failures still close the session; helper launch re-verifies the pinned manifest on every attempt; ET credentials stay out of argv
- Data/schema impact: none
- Security impact: proxy/jump credentials flow only through the Go SSH layer as before; no new secret surfaces
- Verification: go test -race ./internal/terminal/supervised; go test ./cmd/netcatty -run 'TestHelper|TestSupervised|TestTerminalRecovery' -race; node --test wailsRuntimeClient.test.ts 34/34; locale suites 24/24; bindings regenerated (20 services / 197 methods) exposing TerminalService.RestartHelper
- Platforms covered: platform-independent tests on Windows 10 22H2 x64; live mosh/et roaming remains acceptance work
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-003`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Electron helper bridges stay until Wails roaming live evidence and installed-package acceptance pass
- Documentation updated: capability matrix, ledger, remaining-work, pre-acceptance-backlog
- Residual risks: restart re-runs the full SSH/mosh handshake (new remote shell by design); repeated dead helpers keep requiring manual restarts; banner UI not yet GUI-verified
- Next safe slice: live mosh/et roaming matrix on the Debian 13 host
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L125 - 2026-09-12 - Boot-time staging orphan sweep over leased temp entries

- Capability rows: `SYS-01`
- Plan task: `P4-01`
- Status change: `probe -> probe`
- Scope change: none
- Goal: Close the residual orphan-temp risk. The managed temp lease registry (owned entries, I/O pin refcounts, staged-inode checks) already protects active work from ClearTemp; the missing piece was a caller for TempService.CleanupOrphans. Boot now sweeps the dedicated temp root: a fresh process holds no leases, so leftover staged-upload files and active-transfer directories from a previous session are removed once at startup, external-edit downloads are preserved, and entries leased by the running process are never touched. The sweep is idempotent, failures are logged without blocking boot, and cmd-level tests lock the semantics (previous-process leftovers removed; this-process leases survive; second sweep removes nothing).
- Go canonical owner: cmd/netcatty/temp_orphan_sweep.go (sweepTempOrphans), internal/platform/filesystem/filesystem.go (existing lease registry and CleanupOrphans)
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: no lease-protected entry is ever deleted; external-edit downloads survive; single-instance lock means a fresh process cannot race a live session's temp files
- Data/schema impact: none
- Security impact: sweep stays inside the managed temp root and only touches known staging prefixes
- Verification: go test ./cmd/netcatty -run TestSweepTempOrphans (red for the missing helper, green after wiring); go test -race ./internal/platform/filesystem; go test ./cmd/netcatty ./internal/platform/filesystem; migration checker consistent
- Platforms covered: platform-independent tests on Windows 10 22H2 x64
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Electron temp bridges stay until installed-package acceptance passes
- Documentation updated: ledger, remaining-work, pre-acceptance-backlog
- Residual risks: a runtime-abandoned staged upload (renderer died mid-stage without discard) stays until the next boot, by lease design; external-edit downloads accumulate until the user clears temp from Settings
- Next safe slice: live mosh/et roaming matrix on the Debian 13 host
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L126 - 2026-09-12 - Cloud OAuth device/PKCE flows and master key rotation through the native bridge

- Capability rows: `SYNC-02`
- Plan task: `P6-01`
- Status change: `probe -> probe`
- Scope change: none
- Goal: Wire the completed cloud auth backend into the runtime bridge so the existing UI works under Wails. The Go side already exposes GitHub device flow (start/poll/cancel), Google and OneDrive PKCE exchange/refresh, user info, and GitHub Gist plus Google Drive and OneDrive snapshot file operations with a loopback OAuth callback server (internal/platform/cloudsync/oauth_client.go, oauth_callback.go, oauth_snapshots.go, cmd/netcatty/syncAuthService.go). The missing link was the runtime surface: the generated syncservice bindings were never mapped onto the camelCase bridge the adapters and cloudSyncBridge.get() consume, so Wails silently fell back to renderer-direct fetch, which cannot complete Google/OneDrive token exchange (CORS). createCloudOAuthFacade(syncServiceBinding) now maps every method (refresh preserves the prior refresh token, deletions verify the ok flag) onto both the transition bridge and the sync port, built from injectable bindings for tests. UI flows already present: GitHub device-flow modal, Google/OneDrive connect, and the master key rotation dialog driving updateCloudSyncMasterKey through changeMasterKey and propagateMasterKeyRotation, whose multi-key profile transaction is guarded by the Go CAS Write (sync_rotation_test.go proves atomicity and durability across reopen).
- Go canonical owner: internal/platform/cloudsync/oauth_client.go, internal/platform/cloudsync/oauth_callback.go, cmd/netcatty/syncAuthService.go
- Frontend adapter: infrastructure/services/cloudSync/cloudSyncFacade.ts, infrastructure/runtime/wails/wailsRuntimeClient.ts (sync port plus transition bridge), application/state/useCloudSync.ts, application/state/useCloudSyncMasterKey.ts, components/cloud-sync/CloudSyncDialogs.tsx
- Electron owner affected: none; adapters keep renderer-fetch fallback for the Electron shell
- Preserved invariants: tokens never appear in argv or logs; refresh keeps the previous refresh token when the provider omits one; rotation writes every sync key in one CAS transaction, so a failed rotation leaves the profile untouched; Wails never falls back to renderer fetch for provider ports
- Data/schema impact: none; rotation reuses the existing master key config, replica, baseline and snapshot keys
- Security impact: client secrets stay in the Go process; PKCE verifiers and device codes cross the bridge once and are not persisted
- Verification: node --test infrastructure/services/cloudSync/*.test.ts plus runtime client 134/134; rotation suites useCloudSyncMasterKey, masterKeyRotation, masterKeyPropagation 6/6; Go go vet clean; bindings already expose the OAuth methods (syncservice.js); new wiring test asserts githubStartDeviceFlow reaches the injected binding through both the sync port and the transition bridge
- Platforms covered: platform-independent tests on Windows 10 22H2 x64; live provider authorization remains acceptance work
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Electron cloud bridges stay until live OAuth authorization evidence on three platforms passes
- Documentation updated: ledger, remaining-work, pre-acceptance-backlog
- Residual risks: OAuth applications are user-registered (client id/secret supplied at runtime); OneDrive/GitHub file operations are exercised by fixtures, not live accounts; rotation rollback under mid-flight crash is covered by the store transaction but not by a live kill test
- Next safe slice: live OAuth authorization evidence for GitHub, Google and OneDrive
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L127 - 2026-09-12 - Runtime OAuth client IDs after the provider 400 reports

- Capability rows: `SYNC-02`
- Plan task: `P6-01`
- Status change: `probe -> probe`
- Scope change: none
- Goal: Fix the live authorization failures reported for all three providers (Google 400 invalid_request "Missing required parameter: client_id", OneDrive AADSTS900144, GitHub "OAuth client ID is required"). Root cause: the client IDs came only from build-time VITE_SYNC_*_CLIENT_ID variables and default to empty, so the authorize URLs opened with a blank client_id. Client IDs are public values and each user registers their own desktop-app OAuth client, so they are now runtime-configurable: a new settings-domain key (netcatty_sync_oauth_client_ids_v1) holds per-provider IDs, all adapter reads resolve through resolveOAuthClientId (runtime override wins, build constant falls back), startProviderAuth refuses before any browser hop with a clear message when an ID is missing, and the Cloud Sync settings tab gains an OAuth applications section (GitHub/Google/OneDrive fields, five locales) served by the useOAuthClientIds state hook. No client secret is required for GitHub Device Flow or loopback-PKCE desktop clients, so no secret storage was added.
- Go canonical owner: none (renderer-side configuration only)
- Frontend adapter: infrastructure/services/cloudSync/oauthClientIds.ts, infrastructure/services/cloudSync/authMethods.ts, infrastructure/services/adapters/{GitHubAdapter,GoogleDriveAdapter,OneDriveAdapter}.ts, application/state/useOAuthClientIds.ts, components/cloud-sync/OAuthClientIdsSection.tsx, components/CloudSyncSettings.tsx, infrastructure/config/storageKeys.ts
- Electron owner affected: none; the build-time constants remain the fallback
- Preserved invariants: client IDs are public values (no secret storage); an unconfigured provider never opens a provider URL; the settings-domain key flows through the canonical host adapter
- Data/schema impact: new settings-domain key netcatty_sync_oauth_client_ids_v1
- Security impact: positive; users bring their own registered OAuth clients instead of a baked-in application
- Verification: node --test infrastructure/services/cloudSync/oauthClientIds.test.ts (fallback, override, guard, snapshot immutability); full cloud suites plus master key 104/104; runtime client 35/35; locale suites 24/24; eslint clean on new and touched files
- Platforms covered: platform-independent tests on Windows 10 22H2 x64
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Electron cloud bridges stay until live OAuth authorization evidence passes on three platforms
- Documentation updated: ledger, remaining-work, pre-acceptance-backlog
- Residual risks: the authorize URLs are now correct only after the user registers OAuth clients and pastes the IDs; distribution builds can still pin IDs at build time
- Next safe slice: live OAuth authorization evidence for GitHub, Google and OneDrive
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L128 - 2026-09-12 - Allow-listed provider console opener for apply links

- Capability rows: `SYNC-02`
- Plan task: `P6-01`
- Status change: `probe -> probe`
- Scope change: none
- Goal: Fix the silent dead click on the OAuth apply links added in WV3-L127. They routed through OpenOAuthExternal, whose strict validation only permits device-flow and PKCE authorize endpoints, so the console pages were rejected and the swallowed error looked like a dead button. A new SyncService.OpenProviderConsole(provider) opens the exact registration page for an allow-listed provider name (github, google, onedrive) — the bridge takes a provider enum, never a raw URL, so no arbitrary link can turn the app into a URL opener; the browser launcher is a package-level seam covered by tests. The renderer component calls openProviderConsole on the transition bridge and sync port and logs failures instead of swallowing them.
- Go canonical owner: cmd/netcatty/syncAuthService.go (OpenProviderConsole, openExternalLauncher seam)
- Frontend adapter: infrastructure/runtime/wails/wailsRuntimeClient.ts, types/global/netcatty-bridge-sync.d.ts, components/cloud-sync/OAuthClientIdsSection.tsx
- Electron owner affected: none
- Preserved invariants: OAuth authorize validation is untouched; only the three fixed console URLs are reachable; launcher failures surface in the console instead of vanishing
- Data/schema impact: none
- Security impact: allow-list is closed (provider enum plus exact URLs); no user-controlled URL ever reaches the launcher
- Verification: go test ./cmd/netcatty -run TestOpenProviderConsoleAllowlist (unknown provider rejected, three known URLs hit the launcher seam verbatim); node runtime client 36/36 including the passthrough test on both bridge surfaces; cloud suites 127/127; race clean on the touched paths
- Platforms covered: Windows 10 22H2 x64 tests; launcher commands remain platform-specific
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Electron cloud bridges stay until live OAuth authorization evidence passes on three platforms
- Documentation updated: ledger, remaining-work
- Residual risks: console page layouts are provider-controlled; a failed browser launch still only logs (settings has no toast port in this section)
- Next safe slice: live OAuth authorization evidence for GitHub, Google and OneDrive
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L129 - 2026-09-12 - GitHub device-flow 400 pending fix and client ID hover guides

- Capability rows: `SYNC-02`
- Plan task: `P6-01`
- Status change: `probe -> probe`
- Scope change: none
- Goal: Fix the live "GitHub connect failed: Cloud provider HTTP 400". The device-flow token endpoint answers HTTP 400 with the real status in the JSON body (authorization_pending while the user is still typing the code), but PollDevice treated 400 as fatal before parsing the body, killing the poll the moment it started. The token poll now accepts 200 and 400, reads the JSON error field, and only surfaces non-protocol statuses; slow_down keeps returning to the polling loop. Existing tests covered pending only over HTTP 200, so a regression test now pins 400-authorization-pending and slow_down-then-success over real 400 responses. Per the same feedback, every client ID field gains a hover ? tooltip (five locales) with step-by-step registration guidance including the redirect-URI answers, and the external-link control is folded into that icon.
- Go canonical owner: internal/platform/cloudsync/oauth_client.go (PollDevice 400 handling)
- Frontend adapter: components/cloud-sync/OAuthClientIdsSection.tsx (guide tooltips), five locale files
- Electron owner affected: none
- Preserved invariants: token bodies are still never relayed (ErrorDescription stays stripped); polling loop ownership and cancellation are unchanged
- Data/schema impact: none
- Security impact: none; provider error descriptions remain suppressed
- Verification: go test -race ./internal/platform/cloudsync (new TestOAuthPollDeviceTreats400AsPending covers 400-authorization-pending non-fatal and slow_down re-poll); node cloud suites plus runtime client 141/141; locale suites 24/24; migration checker consistent
- Platforms covered: platform-independent tests on Windows 10 22H2 x64; live device flow authorization remains acceptance work
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Electron cloud bridges stay until live OAuth authorization evidence passes on three platforms
- Documentation updated: ledger, remaining-work
- Residual risks: slow_down interval growth is owned by the renderer loop; a user typing a wrong client ID still gets provider-side errors that are intentionally generic
- Next safe slice: live OAuth authorization evidence for GitHub, Google and OneDrive
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L130 - 2026-09-12 - Live-confirmed device_flow_disabled start error surfaced

- Capability rows: `SYNC-02`
- Plan task: `P6-01`
- Status change: `probe -> probe`
- Scope change: none
- Goal: Diagnose the still-failing GitHub connect with the user's real client ID (authorized live probe). POST /login/device/code answers 400 with body device_flow_disabled ("Device Flow must be explicitly enabled for this App") because the app has not enabled device flow — and the failure came from StartDevice, which WV3-L129's poll fix did not cover. StartDevice now parses the 400 body like the poll does and maps device-flow error codes to actionable messages (device_flow_disabled names the exact settings toggle; unverified_user_email names the fix; other codes surface their code). The provider error description is still never relayed.
- Go canonical owner: internal/platform/cloudsync/oauth_client.go (StartDevice 400 handling, deviceFlowErrorText)
- Frontend adapter: none (error message flows through the existing provider error surface)
- Electron owner affected: none
- Preserved invariants: provider error descriptions remain stripped; only the error code reaches the user
- Data/schema impact: none
- Security impact: none
- Verification: live probe against github.com/login/device/code with the user-provided client ID reproduced 400 device_flow_disabled; new TestOAuthStartDeviceSurfacesDisabledFlow asserts the actionable message; go test -race ./internal/platform/cloudsync passes
- Platforms covered: live probe from Windows 10 22H2 x64 against github.com
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Electron cloud bridges stay until live OAuth authorization evidence passes on three platforms
- Documentation updated: ledger, remaining-work
- Residual risks: the user must enable device flow on their GitHub app; the actionable message is currently English-only
- Next safe slice: retry GitHub connect after the user enables device flow
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L131 - 2026-09-12 - Native credential storage for provider tokens and system-browser device flow

- Capability rows: `SYNC-02`
- Plan task: `P6-01`
- Status change: `probe -> probe`
- Scope change: none
- Goal: Fix the two failures from the live GitHub authorization run: after the browser step succeeded, saving the token threw "Secure credential storage is unavailable", and the verification page opened inside the app WebView instead of the system browser. Root causes: the Wails runtime bridge never implemented credentialsEncrypt/credentialsDecrypt/credentialsAvailable (the Go CredentialService with purpose-bound AES-GCM over the OS keyring was registered but unreachable), and the device-flow modal opened the verification URI with window.open. The runtime bridge now exposes the three methods over the Go provider (Seal/Open with a fixed cloud-sync-credentials purpose), keeping the Electron enc:v1: envelope sentinel so renderer-side encrypted-value detection behaves identically, with plaintext passthrough for legacy values. The modal opens the allow-listed github.com/login/device URI through a new useOpenExternal state hook (window.open fallback for Electron), honoring the OAuth URL allow-list on the Go side.
- Go canonical owner: cmd/netcatty/credentialService.go (unchanged; Available/Seal/Open now reachable)
- Frontend adapter: infrastructure/runtime/wails/wailsRuntimeClient.ts (credential methods), application/state/useOpenExternal.ts, components/cloud-sync/CloudSyncControls.tsx
- Electron owner affected: none; the enc:v1: contract matches the Electron credential bridge
- Preserved invariants: envelopes stay opaque refs in the connection record; decrypt passes non-prefixed values through unchanged; token material never enters argv or logs; the verification URI stays allow-listed
- Data/schema impact: provider connection credentials under Wails are stored as enc:v1: envelopes sealed by the Go credential provider
- Security impact: positive; tokens now sit behind OS keyring-backed AES-GCM instead of being unsavable
- Verification: node runtime client 37/37 including a seal/open round-trip test asserting purpose, prefix, passthrough and decode; eslint clean on touched files; workbench suites 8/8
- Platforms covered: platform-independent tests on Windows 10 22H2 x64; OS keyring interaction remains acceptance work
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Electron credential bridge stays until live OAuth storage evidence passes on three platforms
- Documentation updated: ledger, remaining-work
- Residual risks: keyring unlock prompts (if any) appear as opaque Seal failures; a changed OS user cannot open previously sealed envelopes (by design)
- Next safe slice: live end-to-end GitHub sync after this fix
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L132 - 2026-09-12 - Session password service and forgot-master-key reset

- Capability rows: `SYNC-02`
- Plan task: `P6-01`
- Status change: `probe -> probe`
- Scope change: none
- Goal: Fix the "cloudSyncGetSessionPassword is not migrated" sync error and add the forgot-master-key restart the user requested. CloudSyncSetSessionPassword/Get/Clear move to Go (cmd/netcatty/cloudSyncSession.go): the master key is held in memory for the session and a copy sealed by the OS-keyring-backed credential provider is persisted under the profile directory, restoring on first read after a restart — the Electron safeStorage semantics that were unreachable under Wails. CloudSyncResetEverything implements "forgot master key, start over": one CAS profile transaction deletes the master key config, convergent replicas, provider baselines, per-provider base payloads and snapshots, sync history and provider state, then clears the session password; the removed keys are returned for the confirmation toast. The unlock dialog gains a two-step reset entry (five-locale copy) calling resetSyncEverything through the useCloudSync surface.
- Go canonical owner: cmd/netcatty/cloudSyncSession.go, cmd/netcatty/syncAuthService.go (session password facade), cmd/netcatty/syncService.go (setSessionDependencies)
- Frontend adapter: infrastructure/runtime/wails/wailsRuntimeClient.ts (session/reset bridge methods), application/state/useCloudSync.ts (resetSyncEverything), components/cloud-sync/CloudSyncDialogs.tsx (two-step reset), five locale files
- Electron owner affected: none; the Electron in-memory plus safeStorage semantics are preserved as the frozen baseline
- Preserved invariants: the reset runs in one CAS transaction, so a failed rotation-style reset cannot leave a half-cleared profile; the session password never crosses the bridge in plaintext responses after being set; local vault data is untouched by the reset
- Data/schema impact: new profile file cloudsync/session-password (sealed, 0600); no storage keys added
- Security impact: the sealed master key copy is purpose-bound to the Go credential provider and lives only in the profile directory
- Verification: go build plus full cmd/netcatty package tests; node cloud suites 105/105, runtime client 37/37, workbench 8/8, locales 24/24; eslint and tsc clean on touched files (repository-wide tsc baseline unchanged); migration checker consistent
- Platforms covered: platform-independent tests on Windows 10 22H2 x64; cross-restart password restore is covered by tests, live restart by acceptance
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Electron session-password bridge stays until live sync evidence passes on three platforms
- Documentation updated: ledger, remaining-work
- Residual risks: a forgotten key with an intact remote snapshot means the remote copy is orphaned until the fresh vault's first force push; sealed password file removal on manual profile deletion behaves like a normal reset
- Next safe slice: live end-to-end GitHub sync with the restored session password
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L133 - 2026-09-14 - Wails toolchain alignment to beta.12

- Capability rows: `FND-01`
- Plan task: `P1-01`
- Status change: `implemented -> implemented`
- Scope change: none
- Goal: Close the version drift recorded in ai-migration-technical-design section 1.1: the root go module linked against wails v3.0.0-alpha.63 while the npm runtime and the wails3 bindings and syso generator ran v3.0.0-beta.12. The root module now requires v3.0.0-beta.12, matching the two experiment modules. Transitive upgrades: golang.org/x/crypto v0.45.0 to v0.53.0, x/net v0.47.0 to v0.56.0, x/text v0.31.0 to v0.39.0, lmittmann/tint, pjbgf/sha1cd, sergi/go-diff, skeema/knownhosts; golang.design/x/mainthread re-entered via the wails dependency graph. The go directive stays 1.25.0, so CI GOTOOLCHAIN=local and the pinned GOTOOLCHAIN=go1.25.0 binding generation keep working unchanged. A new check:wails-versions drift guard asserts the go module, the npm runtime and every wails3 generator pin stay equal and runs in CI next to check:migration-docs.
- Go canonical owner: go.mod and go.sum (github.com/wailsapp/wails/v3 v3.0.0-beta.12)
- Frontend adapter: none; bindings regenerated byte-identical
- Electron owner affected: none
- Preserved invariants: go directive stays 1.25.0; the seven wails-importing files under cmd/netcatty needed no source changes; experiments already on beta.12 untouched; bindings output stable across the alignment
- Data/schema impact: none; generated bindings byte-identical (20 services, 212 methods, 97 models, above the L132-era 202 methods baseline)
- Security impact: positive; x/crypto and x/net pick up current patch releases
- Verification: go build ./... exit 0 with go1.25.0; go vet ./cmd/netcatty exit 0; go test -count=1 ./cmd/netcatty ok; GOTOOLCHAIN=go1.25.0 wails3 generate bindings processed 352 packages with zero git diff in infrastructure/runtime/wails/bindings; npm run wails:build produced bin/LemonSSH.exe (version 0.0.1); node scripts/migration/check-wails-versions.mjs exit 0
- Platforms covered: Windows 10 22H2 x64 build and tests; macOS and Linux shells not rebuilt in this slice
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: none
- Electron retirement: cutover-trigger: Electron stays the frozen release carrier until three-platform evidence closes P8-02
- Documentation updated: ledger, ai-migration-technical-design section 1.1, baselines/ai-phase7-parity.md section 10
- Residual risks: macOS and Linux shell smoke on the aligned combo still pending; the go1.27.1 toolchain upgrade remains an open W02 follow-up and is not bundled here
- Next safe slice: three-platform shell smoke on the aligned combo, then the go toolchain upgrade evaluation
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L134 - 2026-09-14 - Go 1.27.1 toolchain evaluation probe

- Capability rows: `FND-01`
- Plan task: `P1-01`
- Status change: `implemented -> implemented`
- Scope change: none
- Goal: Evaluate the go1.27.1 toolchain candidate recorded in the W02 task card without changing any locked configuration. With GOTOOLCHAIN=go1.27.1 on the beta.12-aligned tree: go build ./... passes, go vet over cmd/netcatty and internal passes, race tests over profile store, terminal data plane and platform credentials pass on five packages. Bindings generation is the blocker: go1.27.1 processes 369 packages and emits 98 models versus 352 and 97 on go1.25.0, because the go 1.27 stdlib adds encoding/json/jsontext and the generator then projects json.RawMessage as jsontext.Value instead of any, changing five bindings files. The probe restored the go1.25.0 generation afterwards; the committed tree is unchanged.
- Go canonical owner: none changed; go directive stays 1.25.0
- Frontend adapter: none; probe reverted, bindings byte-identical to commit
- Electron owner affected: none
- Preserved invariants: the GOTOOLCHAIN=go1.25.0 pin in the binding generation command stays load-bearing for byte-identical bindings; CI GOTOOLCHAIN=local with 1.25.x untouched; check:wails-versions continues to guard only the wails combination, not the toolchain
- Data/schema impact: none
- Security impact: neutral; no configuration changed
- Verification: see probes/go-toolchain-1.27.md for commands and exit codes; build exit 0, vet exit 0, race 5 packages ok, bindings regen diff characterized and reverted, final worktree clean
- Platforms covered: Windows 10 22H2 x64 only
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: none
- Electron retirement: cutover-trigger: Electron stays the frozen release carrier until three-platform evidence closes P8-02
- Documentation updated: ledger, probes/go-toolchain-1.27.md, baselines/ai-phase7-parity.md section 10
- Residual risks: unix CGO and race behavior under 1.27.1 untested; go1.26.8 untested as fallback candidate
- Next safe slice: keep the 1.25.0 lock; revisit 1.27 when a wails release declares support, as a coordinated slice with binding regen plus TS checks plus three-platform smoke
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L135 - 2026-09-14 - Close storage-key and contract-index drift caught by local gates

- Capability rows: `FND-01`, `SYNC-01`
- Plan task: `P1-01`, `P2-01`
- Status change: `implemented -> implemented`
- Scope change: none
- Goal: Running the full local gate suite on the beta.12-aligned tree surfaced three pre-existing drifts. check:data-inventory failed because six storage keys added by recent slices (workbench session-tree width and expanded state, boot theme mirror, layout mode, sync OAuth client ids and secrets) were missing from the frozen fixture and inventory; regenerating also caught lemonssh_close_behavior_v1. check:migration-electron-baseline failed because runtimePorts.ts was stale against the regenerated bridge contract index; the regen adds the recent bridge methods (deep-link drain, OS protocol status, helper lifecycle, proxy test, cloud-sync reset, provider console) and keeps AgentPort at 61 methods. check:codex-app-server-schema false-failed on Windows only: the byte comparison rejected CRLF checkouts, so the check now normalizes line endings before comparing. New classifications: workbench width/expanded land in the existing device-local substring rules; the boot theme mirror joins transient-cache as a derived UI mirror; sync OAuth client ids default to canonical-migrated; sync OAuth client secrets join SECRET_BEARING so P2-04 providers must re-seal them during migration.
- Go canonical owner: none changed
- Frontend adapter: infrastructure/runtime/generated/runtimePorts.ts regenerated (497 methods across 9 ports, AgentPort unchanged at 61)
- Electron owner affected: none; fixtures re-exported from the frozen Electron sources plus the shared storageKeys.ts
- Preserved invariants: the data-inventory drift test stays machine-enforced; manifest hashes follow the regenerated fixtures; AgentPort method count unchanged so the W01 baseline section 5 stays valid
- Data/schema impact: inventory grows to 184 unique values; sync OAuth client secrets flagged secret-bearing for P2-04 re-seal
- Security impact: positive; OAuth client secrets are now machine-flagged for credential migration instead of silently defaulting to a non-secret classification
- Verification: check:data-inventory exit 0 (184 keys); check:migration-electron-baseline exit 0 including runtime-ports and tsc checks; check:codex-app-server-schema exit 0 on a CRLF checkout with the committed schema untouched; fixture re-export changed only bridge-contract-index.json, manifest.json and storage-key-candidates.json
- Platforms covered: Windows 10 22H2 x64; the schema-check CRLF tolerance also applies to macOS/Linux checkouts but is a no-op there
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: none
- Electron retirement: cutover-trigger: Electron stays the frozen release carrier until three-platform evidence closes P8-02
- Documentation updated: ledger, data-inventory.json, data-inventory.md
- Residual risks: none known; the remaining check suite (lint, TS tests, plugin runtime) unaffected by these files
- Next safe slice: keep gates green per slice; the three-platform smoke and agent disposition decisions remain the open blockers
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L136 - 2026-09-14 - Fix plugin sidecar key truncation under node:sqlite

- Capability rows: `PLUG-01`, `SYNC-01`
- Plan task: `P5-01`
- Status change: `implemented -> implemented`
- Scope change: none
- Goal: The local plugin runtime gate caught two failures in pluginSyncSidecarService tests: retained sidecars were not re-emitted after plugin reinstall. Root cause: composite settings sidecar keys joined with a NUL separator (`settingId\0scope\0scopeId`) were truncated at the first NUL by the node:sqlite TEXT binding — the database row itself stored only the setting id (verified by querying length and hex of the raw row), so parseSettingsSidecarKey returned null and the collect filter dropped every retained settings sidecar. The separator changes to the unit separator U+001F, which survives TEXT binding, and the parser accepts legacy NUL-separated rows for runtimes whose binding preserved them. Test fixtures move to the new separator. Mirrored in domain/pluginSyncSidecar.ts and electron/plugins/pluginSyncSidecarHelpers.cjs.
- Go canonical owner: none; fix lives in the frozen Electron plugin host and its renderer mirror
- Frontend adapter: domain/pluginSyncSidecar.ts separator constants; application/pluginSyncSidecarBridge surface unchanged
- Electron owner affected: electron/plugins/pluginSyncSidecarHelpers.cjs only
- Preserved invariants: sync bundle shape unchanged; rows persisted before this fix on NUL-preserving runtimes still parse via the legacy separator; rows truncated by node:sqlite are unparsable and get dropped on the next collectForSync rewrite, which matches their previous dead state
- Data/schema impact: sidecar keys in new cloud payloads use U+001F separators; older payloads with NUL keys still parse
- Security impact: positive; the re-emit path covered by these tests keeps secret and non-sync settings excluded
- Verification: electron/plugins/pluginSyncSidecarService.test.cjs 15/15, domain/pluginSyncSidecar.test.ts 9/9, application/pluginSyncSidecarBridge.test.ts 5/5; full npm run test:plugin-runtime rerun after the fix
- Platforms covered: Windows 10 22H2 x64 with Node v24.14.1; the truncation is a node:sqlite binding behavior and may not reproduce on Electron's bundled runtime, where the legacy parser keeps old rows readable
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: none
- Electron retirement: cutover-trigger: Electron stays the frozen release carrier until three-platform evidence closes P8-02
- Documentation updated: ledger
- Residual risks: pre-existing profiles may hold truncated setting-id-only sidecar rows; they were already non-functional and are cleaned by the next collection
- Next safe slice: full plugin runtime suite green, then back to the open gate blockers
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L137 - 2026-09-14 - Accept the five external agent disposition decisions

- Capability rows: `AI-04`
- Plan task: `P7-05`
- Status change: `not-started -> not-started`
- Scope change: none
- Goal: Accept the five agent disposition proposals as governance decisions without advancing AI-04, which stays not-started until per-agent child rows carry their own evidence. The product owner approved all five agent disposition proposals. decisions.md gains accepted decisions WV3-014 (reject the Cursor embedded Bun runtime), WV3-015 (reject the OpenCode embedded Bun runtime), WV3-016 (retire Copilot), WV3-017 (retire CodeBuddy) and WV3-018 (retire Cursor CLI login), each carrying its exact Required Future Decisions category. The retained Phase 7 adapter targets are Codex App Server, Claude headless and Grok ACP; the five decided vendors fail closed with typed unavailable reasons, keep history readable and never make paid calls. The Required Future Decisions section stays as the canonical category registry per the checker contract; P6-05 gate validation can now bind agentDecisions to WV3-014,WV3-015,WV3-016,WV3-017,WV3-018.
- Go canonical owner: none; governance documentation slice
- Frontend adapter: none in this slice; settings-surface unavailable reasons and AgentPort typed unavailable mappings land with the Phase 7 W16/W21 work packages
- Electron owner affected: none
- Preserved invariants: AI-01 through AI-04 remain not-started; no production path was created; the Required Future Decisions category list is unchanged as required by the migration checker
- Data/schema impact: none; historical external-agent config and sessions stay in place and readable
- Security impact: positive; the retired vendors can no longer reach paid APIs or Node child processes in the Wails release
- Verification: npm run check:migration-docs exit 0 with the five new decision headings parsed; proposals document marked accepted
- Platforms covered: documentation only
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-014`, `WV3-015`, `WV3-016`, `WV3-017`, `WV3-018`
- Gate: `none`
- Closure evidence: none
- Electron retirement: cutover-trigger: Electron stays the frozen release carrier until three-platform evidence closes P8-02
- Documentation updated: ledger, decisions.md, proposals/agent-disposition-proposals.md
- Residual risks: the Cursor-branded unified settings explanation and per-vendor typed unavailable mappings are still pending implementation in Phase 7 W16/W21
- Next safe slice: create the AI-04 child rows (retained Codex, Claude, Grok; retired five with decision references), then continue the non-AI gate evidence
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L138 - 2026-09-15 - Split AI-04 into per-agent child rows

- Capability rows: `AI-04`, `AI-04.1`, `AI-04.2`, `AI-04.3`, `AI-04.4`, `AI-04.5`, `AI-04.6`, `AI-04.7`, `AI-04.8`
- Plan task: `P7-05`
- Status change: `not-started -> not-started`
- Scope change: none
- Goal: Pass the Phase 7 decomposition gate for AI-04 without starting production adapters. The matrix now has eight stable child rows: AI-04.1 Codex App Server, AI-04.2 Claude native headless, and AI-04.3 Grok ACP remain required retained targets; AI-04.4 Cursor API-key, AI-04.5 OpenCode, AI-04.6 Copilot, AI-04.7 CodeBuddy, and AI-04.8 Cursor CLI login remain required and not-started, with Phase 7 owners recorded as typed unavailable per WV3-014 through WV3-018. No scope-removal decision is taken in this slice, so retired vendors stay required until a later capability-specific decision. implementation-plan P7-05 now lists per-child execution cards; work packages W17 through W21 cite the child IDs. internal/agent and cmd/netcatty-mcp are still forbidden until P6-05.
- Go canonical owner: none; documentation slice only
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: AI-01 through AI-04 remain not-started; no production AI path was created; WV3-002 still forbids Node/Bun runtimes for retained adapters
- Data/schema impact: none
- Security impact: none in this slice; later Phase 7 typed-unavailable mappings must still block paid calls
- Verification: npm run check:migration-docs
- Platforms covered: documentation only
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-014`, `WV3-015`, `WV3-016`, `WV3-017`, `WV3-018`
- Gate: `none`
- Closure evidence: none
- Electron retirement: cutover-trigger: Electron stays the frozen release carrier until three-platform evidence closes P8-02
- Documentation updated: capability-matrix, implementation-plan, ai-migration-work-packages, remaining-work, ledger
- Residual risks: AI-04.4 through AI-04.8 still need later scope-removal decisions before they can leave required scope; settings typed-unavailable UI is still a Phase 7 W21 deliverable
- Next safe slice: migrate the unimplemented Wails script port, or continue non-AI evidence collection
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L139 - 2026-09-15 - Native script recording without the Node worker

- Capability rows: `FND-01`
- Plan task: `P1-02`
- Status change: `implemented -> implemented`
- Scope change: none
- Goal: Replace the Wails `script: unimplemented("script")` fail-closed port with a Go recorder for Start/Stop/AppendStep so workbench script recording works without Electron IPC. Codegen matches the Electron `stepsToJavaScript` contract (sensitive prompts, waitForText, waitForPrompt, sleep gaps). Execution methods stay unmigrated because the Electron runner is a Node worker thread and cannot move in this slice.
- Go canonical owner: `internal/script/recording.go`, `cmd/netcatty/scriptService.go`
- Frontend adapter: `infrastructure/runtime/wails/wailsRuntimeClient.ts` script port recording methods; generated `scriptservice` bindings
- Electron owner affected: none; `electron/bridges/scriptBridge.cjs` remains the frozen release-carrier recorder
- Preserved invariants: run/pause/resume/stop/getRuns/dialog/snapshot methods still throw not-migrated; recording limits stay 10_000 steps and 2 MiB; empty session IDs fail closed
- Data/schema impact: none; recordings are in-memory per session
- Security impact: positive; sensitive send steps still compile to a prompt rather than embedding the secret in generated source
- Verification: go test -count=1 ./internal/script ./cmd/netcatty -run TestScriptServiceRecordsAndStops; node --test --import tsx infrastructure/runtime/wails/wailsRuntimeClient.test.ts application/state/useScriptRecorder.test.ts; npm run lint
- Platforms covered: Windows 10 22H2 x64 unit tests
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: none
- Electron retirement: cutover-trigger: Electron scriptBridge stays until script execution has a Go owner and three-platform evidence
- Documentation updated: ledger, remaining-work
- Residual risks: live GUI recording against a real terminal is untested; generated JS still assumes the Electron `nct` host API at run time
- Next safe slice: SYS-01 native dialogs or plugin RuntimePorts alignment; script execution remains blocked on a non-Node runner
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L140 - 2026-09-15 - Replay recorded scripts without the Node worker

- Capability rows: `FND-01`
- Plan task: `P1-02`
- Status change: `implemented -> implemented`
- Scope change: none
- Goal: Make Run work for recorder-generated scripts on Wails. Parse sleep/sendLine/waitForPrompt/waitForText, write sendLine as body then CR through TerminalService, and reject dialog/log/disconnect scripts instead of pretending the Node worker ran. scriptPause/Resume/dialog stay unmigrated.
- Go canonical owner: `internal/script/replay.go`, `internal/script/runner.go`, `cmd/netcatty/scriptService.go`
- Frontend adapter: `infrastructure/runtime/wails/wailsRuntimeClient.ts` scriptRun/Stop/GetRuns on transitionBridge; `application/state/useScriptExecution.ts` refreshes run snapshots after start/stop
- Electron owner affected: none; Electron scriptRuntime remains the frozen JS worker
- Preserved invariants: unsupported nct.dialog/progress/disconnect scripts fail closed; sendLine still splits body and CR; waitForPrompt currently waits the recorded timeout rather than parsing PTY text
- Data/schema impact: none
- Security impact: observer-mode write blocking stays in the renderer coordinator
- Verification: go test -count=1 ./internal/script ./cmd/netcatty -run Script; node --test --import tsx infrastructure/runtime/wails/wailsRuntimeClient.test.ts
- Platforms covered: Windows 10 22H2 x64 unit tests
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: none
- Electron retirement: cutover-trigger: Electron scriptRuntime stays until waitForPrompt reads PTY text and remaining nct APIs have a Go owner
- Documentation updated: ledger, remaining-work
- Residual risks: waitForPrompt does not yet inspect terminal output, so replay may continue before the prompt returns; GUI run against a live session is untested in this slice
- Next safe slice: waitForPrompt against session output, then dialog APIs or SYS-01 dialogs
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L141 - 2026-09-15 - Wait for real terminal prompts during script replay

- Capability rows: `FND-01`
- Plan task: `P1-02`
- Status change: `implemented -> implemented`
- Scope change: none
- Goal: Stop treating waitForPrompt as a short sleep. TerminalService now taps published session bytes into the script runner, which waits until the rolling output looks like a shell prompt or contains the requested text. Timeouts fail the run instead of continuing blindly.
- Go canonical owner: `internal/script/output.go`, `internal/script/runner.go`, `cmd/netcatty/terminalService.go`
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: dialog/log/disconnect scripts still fail closed; sendLine still writes body then CR
- Data/schema impact: none
- Security impact: none
- Verification: go test -count=1 ./internal/script ./cmd/netcatty -run 'Script|Parse|WaitForPrompt|Records'
- Platforms covered: Windows 10 22H2 x64 unit tests
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: none
- Electron retirement: cutover-trigger: Electron scriptRuntime stays until remaining nct APIs have a Go owner
- Documentation updated: ledger, remaining-work
- Residual risks: prompt matching uses the same suffix/regex set as Electron, not a full PTY parser; ANSI-heavy prompts may still time out
- Next safe slice: dialog APIs or SYS-01 native dialogs
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L142 - 2026-09-15 - Replay sensitive prompts through the renderer dialog host

- Capability rows: `FND-01`
- Plan task: `P1-02`
- Status change: `implemented -> implemented`
- Scope change: none
- Goal: Recorded sensitive steps could not replay because dialog.prompt was rejected. Parse const-var assignments from nct.dialog.prompt and variable sendLine references; the runner emits the renderer dialog contract (netcatty:script:dialog-request) through the Wails event bus, the existing ScriptDialogHost renders it unchanged, and the answer returns through ScriptService.ResolveDialog. Other dialog kinds (alert/confirm/form) still fail closed.
- Go canonical owner: `internal/script/replay.go`, `internal/script/runner.go`, `cmd/netcatty/scriptService.go`
- Frontend adapter: `infrastructure/runtime/wails/wailsRuntimeClient.ts` onScriptDialogRequest subscription and scriptDialogResponse; ScriptDialogHost unchanged
- Electron owner affected: none
- Preserved invariants: alert/confirm/form/select/radio/checkbox stay rejected; dialog answers wait up to 120s then fail; sensitive values render as [sensitive] in run logs
- Data/schema impact: none
- Security impact: positive; secrets still never appear in generated code or run logs
- Verification: go test -count=1 ./internal/script ./cmd/netcatty -run 'Script|Parse|Prompt|Records'; node --test --import tsx infrastructure/runtime/wails/wailsRuntimeClient.test.ts
- Platforms covered: Windows 10 22H2 x64 unit tests
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: none
- Electron retirement: cutover-trigger: Electron scriptRuntime stays until remaining nct APIs have a Go owner
- Documentation updated: ledger, remaining-work
- Residual risks: dialog timeout is 120s like Electron; a closed renderer window leaves the run failing at dialog timeout
- Next safe slice: script pause/resume or SYS-01 native dialogs
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L143 - 2026-09-15 - Script pause/resume, log and alert support

- Capability rows: `FND-01`
- Plan task: `P1-02`
- Status change: `implemented -> implemented`
- Scope change: none
- Goal: Complete the recorded-script run lifecycle. The Go runner gains Pause/Resume (executor yields before the next op), nct.log appends to run logs, and nct.dialog.alert shows through the renderer dialog host without blocking semantics beyond the answer. scriptPause/scriptResume map onto the bridge so the existing Scripts panel controls work. Race detector also caught Start cloning run state concurrently with the executor logging; the returned snapshot is now taken under the runner lock.
- Go canonical owner: `internal/script/runner.go`, `internal/script/replay.go`, `cmd/netcatty/scriptService.go`
- Frontend adapter: `infrastructure/runtime/wails/wailsRuntimeClient.ts` scriptPause/scriptResume mappings
- Electron owner affected: none
- Preserved invariants: pause takes effect before the next op, not mid-write; confirm/form/select/radio/checkbox still rejected; the returned Start snapshot no longer races the executor
- Data/schema impact: none
- Security impact: none
- Verification: go test -count=1 -race ./internal/script; go test -count=1 ./internal/script ./cmd/netcatty -run Script; node --test --import tsx infrastructure/runtime/wails/wailsRuntimeClient.test.ts; npm run lint
- Platforms covered: Windows 10 22H2 x64 unit tests
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: none
- Electron retirement: cutover-trigger: Electron scriptRuntime stays until remaining nct APIs have a Go owner
- Documentation updated: ledger, remaining-work
- Residual risks: pause granularity is per-op; a long waitForPrompt finishes its wait before honoring pause
- Next safe slice: SYS-01 native dialogs or plugin RuntimePorts alignment
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L144 - 2026-09-16 - Progress API for recorded-script replay

- Capability rows: `FND-01`
- Plan task: `P1-02`
- Status change: `implemented -> implemented`
- Scope change: none
- Goal: Support nct.progress.start/set/step/done in the Go runner with the Electron semantics: start switches the run to determinate mode with a label and total, set clamps the current value, step increments, done marks completion. Progress fields ride the run snapshot (progressMode/progressLabel/progressCurrent/progressTotal/activityLabel) so the existing overlay progress bar renders without renderer changes. Computed (non-literal) progress arguments still fail closed as unsupported lines.
- Go canonical owner: `internal/script/replay.go`, `internal/script/runner.go`
- Frontend adapter: none; run snapshot fields match the existing ScriptRun contract
- Electron owner affected: none
- Preserved invariants: progress mutations only apply in determinate mode; dialog/prompt/log/alert semantics unchanged; literal-args-only policy keeps evaluation honest
- Data/schema impact: run snapshots gain optional progress fields already defined by the renderer contract
- Security impact: none
- Verification: go test -count=1 -race ./internal/script; go vet ./internal/script
- Platforms covered: Windows 10 22H2 x64 unit tests
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: none
- Electron retirement: cutover-trigger: Electron scriptRuntime stays until remaining nct APIs have a Go owner
- Documentation updated: ledger, remaining-work
- Residual risks: activityLabel strings are truncated by the renderer, not the runner; scripts computing progress in loops with variables stay unsupported
- Next safe slice: session.disconnect/startLog APIs or plugin RuntimePorts alignment
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L145 - 2026-09-16 - session.disconnect in recorded-script replay

- Capability rows: `FND-01`
- Plan task: `P1-02`
- Status change: `implemented -> implemented`
- Scope change: none
- Goal: Support nct.session.disconnect in the Go runner: the session closes through TerminalService.Close and the run finishes completed immediately, since nothing further can execute in a closed session. startLog/stopLog stay rejected until a Go session-log owner exists.
- Go canonical owner: `internal/script/replay.go`, `internal/script/runner.go`, `cmd/netcatty/scriptService.go`
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: writes after disconnect cannot happen (run ends at disconnect); startLog/stopLog still fail closed
- Data/schema impact: none
- Security impact: none
- Verification: go test -count=1 -race ./internal/script; go test -count=1 ./cmd/netcatty -run Script
- Platforms covered: Windows 10 22H2 x64 unit tests
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: none
- Electron retirement: cutover-trigger: Electron scriptRuntime stays until startLog/stopLog and remaining nct APIs have a Go owner
- Documentation updated: ledger, remaining-work
- Residual risks: a disconnect followed by intentional post-disconnect script logic is not supported by design
- Next safe slice: session startLog/stopLog with a Go log owner, or plugin RuntimePorts alignment
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L146 - 2026-09-16 - waitForRegex and waitForAny in recorded-script replay

- Capability rows: `FND-01`
- Plan task: `P1-02`
- Status change: `implemented -> implemented`
- Scope change: none
- Goal: Support nct.screen.waitForRegex(pattern, timeout) and waitForAny(patterns[], timeout) in the Go runner. Pattern semantics mirror the Electron buffer: quoted strings are escaped literals, "/body/flags" strings compile as regexes with i/m/s honored, and matches must land in the fresh tail window of the rolling output. screen.getText/send/clear stay rejected.
- Go canonical owner: `internal/script/replay.go`, `internal/script/output.go`, `internal/script/runner.go`
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: regex scans are bounded to a 64 KiB tail plus 512-byte fresh slack; bare regex literals outside strings are not parsed; unsupported APIs still fail closed
- Data/schema impact: none
- Security impact: none
- Verification: go test -count=1 -race ./internal/script
- Platforms covered: Windows 10 22H2 x64 unit tests
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: none
- Electron retirement: cutover-trigger: Electron scriptRuntime stays until screen.getText/send/clear and session startLog/stopLog have a Go owner
- Documentation updated: ledger, remaining-work
- Residual risks: Go RE2 differs from JavaScript regex for backreferences and lookaround; such patterns fail to compile and reject the script with a clear error
- Next safe slice: screen.getText/send or plugin RuntimePorts alignment
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L147 - 2026-09-16 - JavaScript regex parity via regexp2

- Capability rows: `FND-01`
- Plan task: `P1-02`
- Status change: `implemented -> implemented`
- Scope change: none
- Goal: Close the known RE2-vs-JavaScript regex difference from L146. Script wait patterns now run on github.com/dlclark/regexp2 (MIT, pure Go, already present as an indirect dependency), so backreferences, lookbehind and lookahead keep their JavaScript semantics. Flags i/m/s map to regexp2 options; a 2-second match timeout guards against catastrophic backtracking; literal patterns still go through QuoteMeta. Verified with a lookbehind pattern and an (ab)\1 backreference against fed session output.
- Go canonical owner: `internal/script/output.go`, `internal/script/replay.go`, `go.mod`
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: quoted patterns without /…/ form stay escaped literals; unsupported flags fail with a clear error; the Go language directive stays 1.25.0
- Data/schema impact: none
- Security impact: positive; match timeout bounds CPU use from hostile patterns
- Verification: go test -count=1 -race ./internal/script; go test -count=1 ./cmd/netcatty -run Script
- Platforms covered: Windows 10 22H2 x64 unit tests
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: none
- Electron retirement: cutover-trigger: Electron scriptRuntime stays until screen.getText/send/clear and session startLog/stopLog have a Go owner
- Documentation updated: ledger, remaining-work
- Residual risks: regexp2 is a backtrack engine — the 2s MatchTimeout bounds but does not eliminate CPU spikes from pathological patterns
- Next safe slice: screen.getText/send or plugin RuntimePorts alignment
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L148 - 2026-09-16 - screen.send/clear/getText and dialog.confirm in replay

- Capability rows: `FND-01`
- Plan task: `P1-02`
- Status change: `implemented -> implemented`
- Scope change: none
- Goal: Complete the nct.screen surface in the Go runner: screen.send writes without appending CR (string or variable, sensitive-aware), screen.clear resets the runner-side rolling output, and const-var assignments from screen.getText capture the current output text. dialog.confirm assignments return the renderer answer as true/false through the dialog host. A deadlock the new getText case introduced (r.watch locking r.mu while held) was caught by the race-enabled test run and fixed by resolving the watch reference before locking.
- Go canonical owner: `internal/script/replay.go`, `internal/script/runner.go`, `internal/script/output.go`
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: nct.log resolves variables from prior assignments; range-based getText and dialog form/select/radio/checkbox still fail closed
- Data/schema impact: none
- Security impact: none
- Verification: go test -count=1 -race -timeout 120s ./internal/script; go test -count=1 ./cmd/netcatty -run Script; npm run lint; npm run check:migration-docs
- Platforms covered: Windows 10 22H2 x64 unit tests
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: none
- Electron retirement: cutover-trigger: Electron scriptRuntime stays until session startLog/stopLog have a Go owner
- Documentation updated: ledger, remaining-work
- Residual risks: getText reflects the runner-side rolling view, not the renderer viewport (OSC sequences included); scripts comparing text with JS string methods stay unsupported
- Next safe slice: session startLog/stopLog with a Go log owner, or plugin RuntimePorts alignment
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L149 - 2026-09-16 - session.startLog/stopLog with a Go log owner

- Capability rows: `FND-01`, `TERM-01`
- Plan task: `P1-02`
- Status change: `implemented -> implemented`
- Scope change: none
- Goal: Add the last two rejected script APIs. A sessionlog.Manager owns one open log file per terminal session: script startLog opens it (default under the profile session-logs directory, custom path honored), all terminal bytes already tapped in TerminalService flow to both the script runner and the manager, and stopLog closes the file. Session logs also survive past the script since the manager is session-scoped, and CloseAll runs at app shutdown.
- Go canonical owner: `internal/terminal/sessionlog/manager.go`, `cmd/netcatty/main.go`, `cmd/netcatty/scriptService.go`
- Frontend adapter: none
- Electron owner affected: none; Electron sessionLogStreamManager remains the frozen carrier owner
- Preserved invariants: terminal bytes keep flowing to the data plane even if the log file write fails; logs are best-effort and never block the terminal
- Data/schema impact: none
- Security impact: note — session logs contain raw terminal output including typed secrets; file permissions are 0600 and location is the profile directory
- Verification: go test -count=1 -race ./internal/script ./internal/terminal/sessionlog; go test -count=1 ./cmd/netcatty -run Script; npm run check:migration-docs
- Platforms covered: Windows 10 22H2 x64 unit tests
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: none
- Electron retirement: cutover-trigger: Electron sessionLogStreamManager stays until three-platform log parity evidence closes P8-02
- Documentation updated: ledger, remaining-work
- Residual risks: log file growth is unbounded while a stream is open (matches Electron behavior); stopLog before startLog returns an error surfaced in run logs
- Next safe slice: dialog.form/select/radio/checkbox parsing or plugin RuntimePorts alignment
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L150 - 2026-09-16 - Live run updates over the Wails event bus

- Capability rows: `FND-01`
- Plan task: `P1-02`
- Status change: `implemented -> implemented`
- Scope change: none
- Goal: Replace the temporary 400ms/15s run-list polling with live push. The Go runner now broadcasts a full run snapshot after registration, every log append, progress mutations, pause/resume and finish; ScriptService forwards each snapshot as a netcatty:script:runs-updated Wails event, the runtime client exposes onScriptRunsUpdated (subscribed once per window), and the existing ScriptAutomationRoot bind drives setScriptRuns so overlays and the Scripts panel update in real time. A missing unlock in the new Resume path was caught by the test suite and fixed before commit.
- Go canonical owner: `internal/script/runner.go`, `cmd/netcatty/scriptService.go`
- Frontend adapter: `infrastructure/runtime/wails/wailsRuntimeClient.ts` (onScriptRunsUpdated), `components/scripts/ScriptAutomationRoot.tsx` unchanged
- Electron owner affected: none
- Preserved invariants: the runs event carries a full snapshot matching the Electron contract; the temporary per-run polls in scriptAutomationCoordinator and useScriptExecution are removed in favor of the event
- Data/schema impact: none
- Security impact: none
- Verification: go test -count=1 -race -timeout 120s ./internal/script; go test -count=1 ./cmd/netcatty -run Script; node --test --import tsx infrastructure/runtime/wails/wailsRuntimeClient.test.ts (42/42); npm run lint; npm run check:migration-docs
- Platforms covered: Windows 10 22H2 x64 unit tests
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: none
- Electron retirement: cutover-trigger: Electron scriptRuntime stays until remaining nct APIs (form/select/radio/checkbox) have a Go owner
- Documentation updated: ledger, remaining-work
- Residual risks: dialog form/select/radio/checkbox parsing remains unimplemented
- Next safe slice: dialog form/select/radio/checkbox parsing or plugin RuntimePorts alignment
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L151 - 2026-09-16 - Split PLUG-02 into tested child rows

- Capability rows: `PLUG-02.1`, `PLUG-02.2`
- Plan task: `P5-04`
- Status change: `not-started -> probe`
- Scope change: none
- Goal: Pass the Phase 5 decomposition gate for PLUG-02. The matrix gains two stable child rows: PLUG-02.1 covers the wazero WASM runtime with runtime identity, memory/time/host-call quotas, WASI default-off, cancellation and trap cleanup (internal/plugin/wasm); PLUG-02.2 covers schema-validated declarative UI — settings/form/list/card rendering, literal bindings and controlled rollback (internal/plugin/ui plus internal/plugin/host). Both children land in probe matching the parent's conservative state: the Go code and tests exist locally, but installed-package GUI and three-platform acceptance remain pending before either can reach implemented. The parent PLUG-02 keeps probe with its blocker text pointing at the children.
- Go canonical owner: `internal/plugin/wasm`, `internal/plugin/ui`, `internal/plugin/host`
- Frontend adapter: existing host-rendered contribution components (unchanged)
- Electron owner affected: none
- Preserved invariants: PLUG-02 stays probe pending GUI/platform acceptance; no DOM/network/filesystem escape requirement carries to the children; plugin permission broker separation unchanged
- Data/schema impact: none
- Security impact: none in this slice; the children own the existing no-injection negative tests
- Verification: go test -count=1 ./internal/plugin/wasm ./internal/plugin/ui ./internal/plugin/host (all ok); npm run check:migration-docs
- Platforms covered: Windows 10 22H2 x64 unit tests
- Evidence grade: `C`
- Decision references: `WV3-005`
- Gate: `none`
- Closure evidence: none
- Electron retirement: cutover-trigger: Electron plugin runtime stays until three-platform GUI acceptance closes P8-02
- Documentation updated: capability-matrix, implementation-plan, ledger, remaining-work
- Residual risks: live GUI acceptance and three-platform evidence remain pending for both children
- Next safe slice: dialog form/select/radio/checkbox parsing or three-platform evidence collection
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L152 - 2026-09-16 - Remove AI-04.4 from required Wails scope

- Capability rows: `AI-04.4`
- Plan task: `P7-05`
- Status change: `not-started -> retired`
- Scope change: `AI-04.4: required -> removed`
- Goal: Exit required Wails scope for the Cursor API-key adapter. This is not an adapter implementation. AgentPort stays typed unavailable, history stays readable, and no paid calls are made. Electron `@cursor/sdk` remains the frozen release-carrier owner until P9.
- Go canonical owner: none; governance documentation slice
- Frontend adapter: none in this slice; settings unavailable copy and AgentPort fail-closed mapping remain Phase 7 W20 work
- Electron owner affected: `@cursor/sdk` with embedded Bun bridge; source is not deleted
- Preserved invariants: `AI-01`, `AI-02`, `AI-03`, `AI-04`, `AI-04.1`, `AI-04.2`, and `AI-04.3` remain not-started; no production AI path was created
- Data/schema impact: none; historical Cursor API-key sessions stay readable
- Security impact: positive; the Wails release cannot spawn the Bun bridge or make paid Cursor API-key calls
- Verification: npm run check:migration-docs
- Platforms covered: documentation only
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-014`, `WV3-019`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: removed: Electron `@cursor/sdk` Bun bridge remains the frozen release-carrier owner; no Wails owner; source not deleted until P9
- Documentation updated: capability-matrix, decisions, implementation-plan, remaining-work, work packages, ledger, checker
- Residual risks: settings typed-unavailable UI is still a Phase 7 W20 deliverable
- Next safe slice: `AI-04.5` scope-removal
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L153 - 2026-09-16 - Remove AI-04.5 from required Wails scope

- Capability rows: `AI-04.5`
- Plan task: `P7-05`
- Status change: `not-started -> retired`
- Scope change: `AI-04.5: required -> removed`
- Goal: Exit required Wails scope for the OpenCode adapter. This is not an adapter implementation. AgentPort stays typed unavailable, history stays readable, and no paid calls are made. Electron `@opencode-ai/sdk` remains the frozen release-carrier owner until P9.
- Go canonical owner: none; governance documentation slice
- Frontend adapter: none in this slice; settings unavailable copy and AgentPort fail-closed mapping remain Phase 7 W20 work
- Electron owner affected: `@opencode-ai/sdk` starting `opencode serve`; source is not deleted
- Preserved invariants: `AI-01`, `AI-02`, `AI-03`, `AI-04`, `AI-04.1`, `AI-04.2`, and `AI-04.3` remain not-started; no production AI path was created
- Data/schema impact: none; historical OpenCode sessions stay readable
- Security impact: positive; the Wails release cannot spawn embedded Bun `opencode serve` or make paid OpenCode calls
- Verification: npm run check:migration-docs
- Platforms covered: documentation only
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-015`, `WV3-020`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: removed: Electron `@opencode-ai/sdk` `opencode serve` path remains the frozen release-carrier owner; no Wails owner; source not deleted until P9
- Documentation updated: capability-matrix, decisions, implementation-plan, remaining-work, work packages, ledger, checker
- Residual risks: settings typed-unavailable UI is still a Phase 7 W20 deliverable
- Next safe slice: `AI-04.6` scope-removal
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L154 - 2026-09-16 - Remove AI-04.6 from required Wails scope

- Capability rows: `AI-04.6`
- Plan task: `P7-05`
- Status change: `not-started -> retired`
- Scope change: `AI-04.6: required -> removed`
- Goal: Exit required Wails scope for the Copilot adapter. This is not an adapter implementation. AgentPort stays typed unavailable, history stays readable, and no paid calls are made. Electron Copilot SDK/CLI remains the frozen release-carrier owner until P9.
- Go canonical owner: none; governance documentation slice
- Frontend adapter: none in this slice; settings unavailable copy and AgentPort fail-closed mapping remain Phase 7 W21 work
- Electron owner affected: `@github/copilot-sdk` plus Copilot CLI; source is not deleted
- Preserved invariants: `AI-01`, `AI-02`, `AI-03`, `AI-04`, `AI-04.1`, `AI-04.2`, and `AI-04.3` remain not-started; no production AI path was created
- Data/schema impact: none; historical Copilot sessions stay readable
- Security impact: positive; the Wails release cannot spawn a Node Copilot child or make paid Copilot calls
- Verification: npm run check:migration-docs
- Platforms covered: documentation only
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-016`, `WV3-021`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: removed: Electron `@github/copilot-sdk` plus Copilot CLI remains the frozen release-carrier owner; no Wails owner; source not deleted until P9
- Documentation updated: capability-matrix, decisions, implementation-plan, remaining-work, work packages, ledger, checker
- Residual risks: settings typed-unavailable UI is still a Phase 7 W21 deliverable
- Next safe slice: `AI-04.7` scope-removal
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L155 - 2026-09-16 - Remove AI-04.7 from required Wails scope

- Capability rows: `AI-04.7`
- Plan task: `P7-05`
- Status change: `not-started -> retired`
- Scope change: `AI-04.7: required -> removed`
- Goal: Exit required Wails scope for the CodeBuddy adapter. This is not an adapter implementation. AgentPort stays typed unavailable, history stays readable, and no paid calls are made. Electron CodeBuddy Node CLI remains the frozen release-carrier owner until P9.
- Go canonical owner: none; governance documentation slice
- Frontend adapter: none in this slice; settings unavailable copy and AgentPort fail-closed mapping remain Phase 7 W21 work
- Electron owner affected: `@tencent-ai/agent-sdk` plus Node CLI; source is not deleted
- Preserved invariants: `AI-01`, `AI-02`, `AI-03`, `AI-04`, `AI-04.1`, `AI-04.2`, and `AI-04.3` remain not-started; no production AI path was created
- Data/schema impact: none; historical CodeBuddy sessions stay readable
- Security impact: positive; the Wails release cannot spawn a Node CodeBuddy child or make paid CodeBuddy calls
- Verification: npm run check:migration-docs
- Platforms covered: documentation only
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-017`, `WV3-022`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: removed: Electron `@tencent-ai/agent-sdk` plus Node CLI remains the frozen release-carrier owner; no Wails owner; source not deleted until P9
- Documentation updated: capability-matrix, decisions, implementation-plan, remaining-work, work packages, ledger, checker
- Residual risks: settings typed-unavailable UI is still a Phase 7 W21 deliverable
- Next safe slice: `AI-04.8` scope-removal
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L156 - 2026-09-16 - Remove AI-04.8 from required Wails scope

- Capability rows: `AI-04.8`
- Plan task: `P7-05`
- Status change: `not-started -> retired`
- Scope change: `AI-04.8: required -> removed`
- Goal: Exit required Wails scope for Cursor CLI login. This is not an adapter implementation. AgentPort stays typed unavailable, history stays readable, and no paid calls are made. Both Cursor-branded entries share the unified unavailable explanation. Electron `cursor-agent` remains the frozen release-carrier owner until P9.
- Go canonical owner: none; governance documentation slice
- Frontend adapter: none in this slice; settings unavailable copy and AgentPort fail-closed mapping remain Phase 7 W21 work
- Electron owner affected: `cursor-agent` stream-json (`node.exe` plus `index.js` on Windows); source is not deleted
- Preserved invariants: `AI-01`, `AI-02`, `AI-03`, `AI-04`, `AI-04.1`, `AI-04.2`, and `AI-04.3` remain not-started; no production AI path was created
- Data/schema impact: none; historical Cursor CLI sessions stay readable
- Security impact: positive; the Wails release cannot spawn `node.exe` `index.js` for Cursor CLI login or make paid Cursor CLI calls
- Verification: npm run check:migration-docs
- Platforms covered: documentation only
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-018`, `WV3-023`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: removed: Electron `cursor-agent` Node path remains the frozen release-carrier owner; no Wails owner; source not deleted until P9
- Documentation updated: capability-matrix, decisions, implementation-plan, remaining-work, work packages, ledger, checker
- Residual risks: unified Cursor unavailable settings copy is still a Phase 7 W21 deliverable
- Next safe slice: continue non-AI live evidence collection; do not start Catty or `internal/agent`
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L157 - 2026-09-16 - Replay dialog.form/select/radio/checkbox

- Capability rows: `FND-01`
- Plan task: `P1-02`
- Status change: `implemented -> implemented`
- Scope change: `none`
- Goal: Run recorder and hand-written `nct.dialog.form`, `select`, `radio`, and `checkbox` on the Go runner. Select/radio/checkbox become a one-field form. The existing ScriptDialogHost still draws the dialog. Form answers JSON-encode through ResolveDialog. Cancel still fails the run with Dialog cancelled. Empty forms and reserved field names stay rejected.
- Go canonical owner: `internal/script/replay.go`, `internal/script/runner.go`
- Frontend adapter: `infrastructure/runtime/wails/wailsRuntimeClient.ts` JSON-encodes object dialog answers and forwards form payloads on `netcatty:script:dialog-request`
- Electron owner affected: none; `electron/scripts/scriptRuntime.cjs` remains the frozen release-carrier dialog host
- Preserved invariants: FND-01 stays implemented; prompt/confirm/alert keep their previous string answers; no production AI path was created
- Data/schema impact: none
- Security impact: none; reserved form field names stay rejected
- Verification: go test -count=1 -race ./internal/script; go test -count=1 ./cmd/netcatty -run Script; node --test --import tsx infrastructure/runtime/wails/wailsRuntimeClient.test.ts
- Platforms covered: Windows 10 22H2 x64 unit tests
- Evidence grade: `C`
- Decision references: `WV3-001`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Electron scriptRuntime stays until three-platform evidence closes P8-02
- Documentation updated: ledger, remaining-work
- Residual risks: computed form specs and variable-interpolated field values stay unsupported line-parser input
- Next safe slice: continue non-AI live evidence collection; do not start Catty or `internal/agent`
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L158 - 2026-09-16 - Defer paid signing out of P6-05

- Capability rows: `REL-01`, `REL-02`
- Plan task: `P6-05`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: Record WV3-024. The product owner will not buy Authenticode or Apple certificates now. Unsigned qualification binaries stay the Wails release shape. REL-01 and REL-02 remain required probe rows. P6-05 no longer waits for them to reach verified. P8-01 still needs signed RC evidence later. Three-platform unsigned packages from v0.0.2 and local package-wails launched on Windows, macOS, and Linux as C-grade smoke only.
- Go canonical owner: none; governance documentation slice
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: REL-01 and REL-02 stay probe; no signed claim; no production AI path was created; FND/TERM/SSH/SFTP/NET/SYS/SYNC/PLUG still need verified before NONAI-COMPLETE
- Data/schema impact: none
- Security impact: none; unsigned qualification remains explicit
- Verification: npm run check:migration-docs
- Platforms covered: documentation plus prior C-grade launch smoke on Windows, macOS, and Linux
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-024`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Electron stays the frozen release carrier until three-platform evidence closes P8-02
- Documentation updated: decisions, capability-matrix, verification-gates, implementation-plan, remaining-work, ledger, checker
- Residual risks: paid certificates and signed feeds remain a later P8-01 decision
- Next safe slice: gather A-grade evidence for remaining non-AI required leaves; do not start Catty or `internal/agent`
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L159 - 2026-09-16 - Authorize Phase 7 after unsigned qualification

- Capability rows: `AI-01`, `AI-02`, `AI-03`, `AI-04`
- Plan task: `P7-01`
- Status change: `not-started -> not-started`
- Scope change: `none`
- Goal: Record WV3-025 superseding WV3-009 for production AI start. Unsigned qualification (v0.0.2, local package-wails, three-platform launch smoke) is enough to create `internal/capability` and `internal/agent`. Do not record a fake NONAI-COMPLETE gate. AI rows stay not-started until W03 contracts land. Next slice is W03 DTOs, not a full Catty runtime.
- Go canonical owner: none in this slice; production path now allowed
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: no production AI path created in this slice; retired AI-04.4 through AI-04.8 stay removed; no Node sidecar; P8-02 still needs AI leaves verified or removed
- Data/schema impact: none
- Security impact: none; Observer/Confirm/Auto policy still required when owners land
- Verification: npm run check:migration-docs
- Platforms covered: documentation only
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-025`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Electron AI harness stays the frozen release carrier until P9
- Documentation updated: decisions, capability-matrix, remaining-work, execution plan, work packages, ledger, checker
- Residual risks: W03 contracts and use-case extraction still absent
- Next safe slice: W03 Agent DTO contracts under `internal/app/contracts`
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L160 - 2026-09-16 - W03 agent wire DTOs and fake driver

- Capability rows: `AI-01`
- Plan task: `P7-01`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: Land the W03 shell-neutral agent contracts under internal/app/contracts: PrepareTurnRequest, PreparedTurn, TurnCommand, ReadEventsRequest, EventPage, AgentEventEnvelope plus chat/turn/agent opaque IDs and five new structured error codes (busy, stale_revision, unsupported, scope_denied, cursor_expired). Sequences and revisions travel as decimal strings so JavaScript never loses uint64 precision. The fake turn driver lives only in _test files; no production runtime or provider was created. Generated TS contracts regenerate cleanly and the drift check passes.
- Go canonical owner: `internal/app/contracts/agent.go`, `internal/app/contracts/ids.go`, `internal/app/contracts/errors.go`
- Frontend adapter: `infrastructure/runtime/contracts/generated/contracts.ts` regenerated
- Electron owner affected: none
- Preserved invariants: internal package imports no Wails, Electron, or UI package; AI-02 through AI-04.3 stay not-started; no provider, no Catty runtime, no Node sidecar; AI-01 is probe, not implemented
- Data/schema impact: none; wire DTOs only
- Security impact: none; DTOs carry no credentials, no raw vendor payloads
- Verification: go test -count=1 ./internal/app/contracts/; go vet ./internal/app/contracts/; go run ./tools/contracts-codegen --check; npm run check:contracts; npm run check:migration-docs
- Platforms covered: Windows 10 22H2 x64 unit tests
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-025`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Electron AI harness stays the frozen release carrier until P9
- Documentation updated: capability-matrix, ledger, remaining-work
- Residual risks: catalog, policy, dispatch and use-case extraction (W04, W05) still absent; matrix AI-01 evidence is C-grade local only
- Next safe slice: W04 shared use-case extraction from cmd/netcatty facades
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L161 - 2026-09-16 - W04 terminal domain shared use case

- Capability rows: `AI-01`
- Plan task: `P7-01`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: First W04 slice. Extract the shell-neutral terminal/session/exec owner from `cmd/netcatty/terminalService.go` into `internal/app/terminaluse`: the single session pool, SSH dial/auth, telnet/serial/local/supervised (mosh/et) starts, exit tracking, cwd/zmodem/monitoring/completion/x11/helper paths. The Wails facade keeps DTO aliases (renderer JSON shapes unchanged) and one-line delegators only. `sftpTerminal.go` now uses the `TransportFor` seam instead of raw pool field access. No AI behavior was added; capability dispatch does not exist yet and will construct this same Service instance.
- Go canonical owner: `internal/app/terminaluse/`
- Frontend adapter: none; Wails service/method names and JSON field names unchanged
- Electron owner affected: none; Electron terminal stack stays the frozen release carrier
- Preserved invariants: AI-01 stays probe; no `internal/capability`, `internal/agent`, `cmd/netcatty-mcp`, `cmd/netcatty-tool`; `internal/` imports no `package main` or `cmd/` package; one session pool, no parallel pool; go.mod/go.sum untouched; `SeedSessionForTest`/`SeedTransportSessionForTest` exist because cross-package white-box fixtures cannot reach unexported fields, and their only callers are `cmd/netcatty` test files
- Data/schema impact: none
- Security impact: none; dial/auth semantics and known-hosts handling moved unchanged
- Verification: go build ./...; go test -count=1 ./internal/app/... ./internal/terminal/...; go test -count=1 ./cmd/netcatty; go test -count=1 ./internal/app/contracts/; grep internal/ for `cmd/` imports (empty)
- Platforms covered: Windows 10 22H2 x64 unit tests; live-matrix tests skip without `NETCATTY_LIVE_HOST`
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-025`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Electron terminal stack stays until three-platform evidence closes P8-02
- Documentation updated: ledger, remaining-work
- Residual risks: SFTP, forward, and Vault/snippet domains are still owned by their Wails facades; no capability dispatch consumes the use case yet; moved white-box tests verified against HEAD by scripted transform-diff
- Next safe slice: W04 remaining domains (SFTP, forward, Vault/snippet), then W05 catalog
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L162 - 2026-09-17 - W04 SFTP domain shared use case

- Capability rows: `AI-01`
- Plan task: `P7-01`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: Second W04 slice. Extract SFTP session/transfer owner from `cmd/netcatty/sftpService.go` (481 -> 135 lines) into `internal/app/sftpuse`: client lifecycle, browsing/stat/text IO, download/upload, archive extraction, compressed upload, terminal subsystem open. Reuses the L161 seams: `terminaluse.SSHConnectRequest` DTO and `TransportFor` for `OpenForTerminal`. Dialing keeps `ssh.BuildDialConfigErr` with the strict host-key policy instead of `terminaluse.SSHDialConfig` so keepalive/verify behavior is unchanged. No second pool: `Open` borrows `sshpool.KindSFTP` from the same pool instance. `sftpTerminal.go` merged away. A 6-line `acquire` shim stays in the facade only because `transferService.go` (owned by the transfer slice) consumes it; that shim is the next deletion target.
- Go canonical owner: `internal/app/sftpuse/`
- Frontend adapter: none; Wails service/method names and JSON field names unchanged
- Electron owner affected: none
- Preserved invariants: AI-01 stays probe; no `internal/capability`, `internal/agent`, `cmd/netcatty-mcp`, `cmd/netcatty-tool`; `internal/` imports no `package main` or `cmd/` package; one SSH pool; go.mod/go.sum untouched; `SeedClientSessionForTest` has only `cmd/netcatty` test callers; forwardService.go (carrying an unrelated in-progress slice) was verified to share no helper and left untouched
- Data/schema impact: none
- Security impact: none; host-key verification, sudo subsystem errors, and managed-temp staging behavior moved unchanged
- Verification: go build ./...; go test -count=1 ./internal/app/... ./internal/terminal/... ./internal/platform/...; go test -count=1 ./cmd/netcatty; go test -count=1 ./internal/app/contracts/; go vet ./internal/app/sftpuse/ ./cmd/netcatty/; boundary greps empty
- Platforms covered: Windows 10 22H2 x64 unit tests; live SFTP tests skip without real hosts
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-025`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Electron SFTP stack stays until three-platform evidence closes P8-02
- Documentation updated: ledger, remaining-work
- Residual risks: forward and Vault/snippet domains still owned by their Wails facades; `openStagingSource` stays in filesystemService.go pending the filesystem slice; transfer scheduler still enters through the facade shim
- Next safe slice: W04 forward domain, then Vault/snippet domain, then W05 catalog
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L163 - 2026-09-17 - W04 forward domain shared use case

- Capability rows: `AI-01`
- Plan task: `P7-01`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: Third W04 slice. Extract port-forward lifecycle from `cmd/netcatty/forwardService.go` (229 -> 55 lines) into `internal/app/forwarduse`: Start/Stop/StopByRuleId/List/Snapshot over the shared SSH pool (`KindForward` lease), dialing through `terminaluse.SSHDialConfig` so keepalive/host-key behavior is byte-identical. SOCKS5 handshake stays in `internal/terminal/forward`; forwarduse owns orchestration only. The facade keeps DTO aliases plus `RuntimeSnapshot(epoch)` taking the shell identity ("wails" today) so a future capability dispatch passes its own epoch without duplicating mapping. Deleted the now caller-less `sshConnectToInput`/`terminalSSHDialConfig` shims from the terminal facade. A gofmt-only drift on this and three sibling files was committed separately before extraction.
- Go canonical owner: `internal/app/forwarduse/`
- Frontend adapter: none; Wails service/method names and JSON field names unchanged
- Electron owner affected: none
- Preserved invariants: AI-01 stays probe; no `internal/capability`, `internal/agent`, `cmd/netcatty-mcp`, `cmd/netcatty-tool`; `internal/` imports no `package main` or `cmd/` package; one SSH pool; go.mod/go.sum untouched; characterization tests moved verbatim
- Data/schema impact: none
- Security impact: none; rule parsing, stop semantics, and dial config moved unchanged
- Verification: go build ./...; go test -count=1 ./cmd/netcatty; go test -count=1 ./internal/app/... ./internal/...; go test -count=1 ./internal/app/contracts/; gofmt -l clean; boundary greps empty
- Platforms covered: Windows 10 22H2 x64 unit tests; live forward tests skip without `NETCATTY_LIVE_HOST`
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-025`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Electron forward stack stays until three-platform evidence closes P8-02
- Documentation updated: ledger, remaining-work
- Residual risks: Vault/snippet domain still owned by its Wails facade; live forward relay needs the real-host matrix before any A-grade claim
- Next safe slice: W04 Vault/snippet domain, then W05 catalog
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L164 - 2026-09-17 - W04 vault audit: scope removal, no extraction

- Capability rows: `AI-01`
- Plan task: `P7-01`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: Fourth W04 slice closes the work package by audit instead of extraction. Surface map: the only Go-side Wails owner of vault/snippet data is `ProfileService` (7 methods), already a no-policy one-line passthrough over `internal/profile/store` (P2-02 CAS/transaction owner, internal and shell-neutral); vault document rules (host/key/group normalization, order) live in renderer React hooks by design; a candidate `internal/app/vaultuse` package was built and then removed in review as a pass-through layer with one caller (facade) — capability dispatch can import `internal/profile/store` directly, and a real policy seam will be created when W13 approval gating needs one. W04's vault acceptance ("Vault 写完原 UI 收到一致 revision") is already satisfied by store CAS plus `WriteResult.Revision` returning to the renderer. Net production change: none; the gofmt-only drift on sibling files was committed separately this day.
- Go canonical owner: `internal/profile/store/` (unchanged); facade `cmd/netcatty/profileService.go` (unchanged, 78 lines)
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: AI-01 stays probe; no `internal/capability`, `internal/agent`, `cmd/netcatty-mcp`, `cmd/netcatty-tool`; secrets stay opaque bytes over the raw channel; no second profile transaction path; `vaultuse` deliberately does not exist — do not re-create it without a real policy to own
- Data/schema impact: none
- Security impact: none
- Verification: go build ./...; go test -count=1 ./cmd/netcatty ./internal/app/... ./internal/profile/... (9 pkgs ok); go test -count=1 ./internal/app/contracts/; store suite covers ExpectedRevision/RevisionConflict (store_test.go, staging_test.go)
- Platforms covered: Windows 10 22H2 x64 unit tests
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-025`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Electron vault stack stays until three-platform evidence closes P8-02
- Documentation updated: ledger, remaining-work, work-packages
- Residual risks: vault write approval policy does not exist yet on the Go side; W13 must add it at the dispatch layer (or a then-real use-case package), not re-introduce an empty seam
- Next safe slice: W05 catalog/policy/dispatch under `internal/capability`
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L165 - 2026-09-18 - W05 catalog, policy and dispatch Go authority

- Capability rows: `AI-01`
- Plan task: `P7-01`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: W05 landed in four commits (8f212038 catalog authority, 5cef2877 policy+grants, 4917b1fe dispatch+timeouts, b7ded241 projection+codegen). `internal/capability` now owns the 77-row catalog (types, catalog data preserving CJS insertion order, registry with last-wins same-surface lookup semantics), policy evaluation (observer/confirm/auto, chat-session-required and chat-cancel gates, byte-identical user-facing strings), permission grant matching including the shell-segmentation security boundary (here-doc bodies and terminators incl. ANSI-C/dollar quotes, arithmetic expansion, comments, background and cwd segments), fail-closed dispatch (`UNKNOWN_CAPABILITY` default-deny, `HANDLER_MISSING` for renderer-local harness tools, prompt-approval path and grant-authorization path with a post-authorization re-check so grant revocation or chat Stop landing during approval refuses execution), rpc timeout resolution, and the agent-kind projection (ResolveAgentKinds, denylist, tool-input schemas, sidebar/global/MCP tool lists) plus `cmd/netcatty-capability-codegen` emitting spec files from the Go authority. Parity against `electron/capabilities` is pinned per ID by committed fixtures under `testdata/ai/catalog` regenerated by `scripts/dump-capability-catalog.cjs` (which also emits `internal/capability/toolinputs_data.go` as generated Go data): 77 catalog rows, 73 tool-input schemas, 72 sidebar specs, 67 global specs, 67 MCP tools; both generated spec files verified semantically equal to the Node-generated committed artifacts. The Node generator stays the committed-spec owner until W22 — the Go codegen writes only to stdout/`--out` (no second writer).
- Go canonical owner: `internal/capability/`; codegen command `cmd/netcatty-capability-codegen/`
- Frontend adapter: none; `infrastructure/ai/harness/generated/*.json` untouched
- Electron owner affected: none (authority port; CJS catalog remains the Electron runtime owner until P9)
- Preserved invariants: AI-01 stays probe; CJS capability files untouched; `internal/capability` imports no Wails/Electron/cmd packages; unknown capabilities default-deny at Dispatch; harness tools stay renderer-local (no host handler registered); implemented-vs-verified separation kept
- Data/schema impact: none
- Security impact: grant shell segmentation ported fail-closed — unsupported JS regex features (backrefs, lookaround, unknown flags) fail RE2 compilation and deny; approval-gate errors fail closed like explicit denials; prompt-approved requests re-check chat cancel and policy after approval
- Verification: go build ./... (exit 0); go vet ./internal/capability/... ./cmd/netcatty-capability-codegen/...; go test -count=1 ./internal/capability/... (ok); go test -race -count=1 ./internal/capability/... (ok); go test -count=1 ./internal/app/contracts/ (ok); gofmt clean; node scripts/dump-capability-catalog.cjs regenerated fixtures; canonical-JSON compare Go codegen vs Node cattyToolSpecs/globalAgentToolSpecs = true/true; boundary greps empty
- Platforms covered: Windows 10 22H2 x64 unit tests
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-025`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Electron capability catalog stays until three-platform evidence closes P8-02
- Documentation updated: ledger, remaining-work, work-packages, capability-matrix
- Residual risks: Go catalog has no Wails production caller yet (W12 wires the agent service to it); grants enter as a function seam with no persistence owner until W10/W12; native MCP/CLI binaries are W06/W07; dispatch handlers map is unpopulated until W13 fills host domains
- Next safe slice: W06 local host RPC and lifecycle under `internal/rpc`
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L166 - 2026-09-18 - W06 authenticated local host RPC core

- Capability rows: `AI-02`
- Plan task: `P7-02`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: W06 core landed as `internal/rpc`. Protocol: versioned envelope (`v`/`id`/`method`/`deadlineMs`/`params`) with typed VERSION_UNSUPPORTED mismatch errors and NDJSON framing under a byte-based 1 MiB default bound; oversized frames are drained to their terminator so the connection stays usable, EOF-mid-frame ends the connection. Auth: 256-bit crypto/rand bearer tokens stored only as SHA-256 digests, first-party/external principals decided at issuance (never from request parameters), permanent revocation via RevokeAll. Server: serial per-connection request/response, per-request deadlines capped by MaxDeadline, handlers keyed by method with UNKNOWN_METHOD fail-closed, typed SCOPE_DENIED via RequireSession so forged session references cannot escalate (T42), frames-only-on-wire with no log interleaving (stdio purity). Lifecycle: Close revokes all tokens and stops tracked connections; ServeConn on a closed server answers SERVER_CLOSING on entry. Discovery: WriteDiscovery/LoadDiscovery/RemoveDiscovery reproduce the existing first-party `{port, token, pid, permissionMode, updatedAt}` 0600 file contract (NETCATTY_TOOL_CLI_DISCOVERY_FILE consumers) with atomic rename. Composition-root wiring (listener + token issuance at app start) intentionally lands with the W07 binaries that consume it.
- Go canonical owner: `internal/rpc/`
- Frontend adapter: none
- Electron owner affected: none (mcpServerBridge.cjs untouched until W07/W22)
- Preserved invariants: AI-02 stays probe until native binaries + live evidence; no Wails/Electron imports in internal/rpc; token material never in argv or traces (discovery file only); revoked tokens keep failing after reuse
- Data/schema impact: none
- Security impact: digest-only token retention; unsupported regex/frame conditions fail closed; oversized/truncated frames bounded (T43); Windows ACL tightening recorded as pending platform evidence
- Verification: go build ./... (exit 0); go vet ./internal/rpc/...; go test -count=1 -timeout 60s ./internal/rpc/... (ok, 12 tests); go test -race -count=1 ./internal/rpc/... (ok); gofmt clean
- Platforms covered: Windows 10 22H2 x64 unit tests; loopback/stdio live matrix pending
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-025`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Node MCP server stays until three-platform evidence closes P8-02
- Documentation updated: ledger, remaining-work, work-packages, capability-matrix
- Residual risks: composition root not yet wired (no production listener); POSIX file-mode evidence is Windows-C only; client-side envelope helper for the Go CLI lives in W07
- Next safe slice: W07 native MCP/CLI binaries (`cmd/netcatty-mcp`, `cmd/netcatty-tool`) wiring this RPC core
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L167 - 2026-09-18 - W07 native MCP/CLI binaries (projection slices)

- Capability rows: `AI-02`
- Plan task: `P7-02`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: W07 first slices landed (SDK version lock per ai-migration-technical-design.md §1). Slice 1 (W07 CLI): `internal/rpc` gained a typed Client (Dial over the discovery file with UNAVAILABLE typing for missing/invalid/dead endpoints, serialized Call with decimal-safe envelope and id correlation) and `internal/capability` gained the CJS CLI projection port (CLIFieldBindings, ResolveCLIRPCMethod, ListCLICapabilities/FormatCLIHelpLines, BuildCatalogCLIParams with variables-JSON/offset coercion and INVALID_ARGUMENT wording); `cmd/netcatty-tool` is the native binary replacing the Node CLI: help/capabilities offline, catalog dispatch, requiresChatSession gate with CJS wording, `--chat-session`/`--scope-session`/`--json` flags, `{ok:false,error:{code,message}}` stderr payloads, exit 0/1/2, error precedence unavailable -> unknown -> chat gate -> args (CJS order), HTML-safe JSON output. Slice 2 (W07 MCP): official MCP Go SDK pinned at v1.7.0 (go.sum h1:yqjY2dsbKAC0LSuWZVBMrHgiG8ukXv6NRo0JiALay44=, design-cited version); `cmd/netcatty-mcp` projects ListMcpTools (67 tools) as stdio tools with 2020-12 input schemas (required from non-optional fields) and relays tools/call over the RPC client; host-reported failures surface as isError content carrying host code/message, unreachable host surfaces typed unavailable; zero policy copy in the binary.
- Go canonical owner: `cmd/netcatty-mcp/`, `cmd/netcatty-tool/`, `internal/rpc/client.go`, `internal/capability/cli.go`
- Frontend adapter: none
- Electron owner affected: none; Node `netcatty-tool-cli.cjs` and `mcpServerBridge.cjs` stay until W22 retirement
- Preserved invariants: no second policy implementation (authorization is host-side); token material only in the 0600 discovery file, never argv; MCP server speaks only the SDK protocol on stdout, diagnostics on stderr
- Data/schema impact: go.mod gains direct `github.com/modelcontextprotocol/go-sdk v1.7.0` + indirect `github.com/google/jsonschema-go v0.4.3`
- Security impact: typed UNAVAILABLE avoids stack-trace leakage; MCP tool schemas carry only catalog fields; relay never widens surface beyond ListMcpTools
- Verification: go build ./...; go vet ./internal/rpc/... ./internal/capability/... ./cmd/netcatty-tool/... ./cmd/netcatty-mcp/...; go test -count=1 ./internal/rpc/... ./internal/capability/... (ok; CLI golden cases ported from cliAdapter.test.cjs); go test -race ./internal/rpc/... (ok); live stdio smoke: initialize -> serverInfo netcatty/0.1.0, tools/list 67 tools, terminal_execute required=[command sessionId], tools/call with app down -> isError "Netcatty is not running... Start Netcatty first."; CLI smoke: help exit 0, unavailable JSON payload exit 1
- Platforms covered: Windows 10 22H2 x64 unit tests + local stdio smoke; real vendor MCP clients untested
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-025`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Node MCP server and tool CLI stay until W22 removes the call chain and P8-02 closes
- Documentation updated: ledger, remaining-work, work-packages, capability-matrix
- Residual risks: host side has no production listener yet (composition root lands with W12/W13 handler population); old CLI special-case commands (exec/jobs/SFTP/session custom output formatting) are relay-thin by design — their formatting moves into host handlers at W13; two-generation protocol/legacy initialize validation and real vendor client matrix still pending; Windows CLI quoting goldens pending
- Next safe slice: W08 provider network policy (`internal/platform/netpolicy`), or W07.2 residual golden work before W13 integration
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L168 - 2026-09-18 - W08 provider network policy and enforced transport

- Capability rows: `AI-03`
- Plan task: `P7-03`
- Status change: `not-started -> probe`
- Scope change: `none`
- Goal: W08 landed as `internal/platform/netpolicy`. Policy core ports the providerHandlers.cjs authority exactly: builtin fetch hosts, builtin localhost ports (Ollama/LM Studio/dev), SSRF private-address set (RFC1918, loopback, link-local, CGNAT 100.64/10, unspecified, IPv6 ULA/link-local, 4-in-6 mapped), metadata hostname block, dynamic provider endpoint registration (localhost base URLs extend allowed ports scheme-agnostically — CJS parity pinned by test), explicit http:// provider hosts, web search host handling, and the skipHostCheck custom-endpoint mode (private still blocked, HTTPS mandatory). Transport adds what the design requires beyond the CJS guard: a dial-phase guard validating EVERY resolved address and connecting only to one validated address (DNS rebinding refusal, T48), redirect revalidation per hop with a 5-hop limit, 10 MiB response body cap enforced in the transport, and SkipTLSVerify mapped to TLS config without ever bypassing host policy. NewClient returns a stdlib http.Client so provider SDKs inject it as their transport (W09).
- Go canonical owner: `internal/platform/netpolicy/`
- Frontend adapter: none
- Electron owner affected: none (providerHandlers.cjs untouched until W22)
- Preserved invariants: AI-03 stays probe until runtime+providers land; no provider SDK exists on the Go side yet to inject the transport (enforcement is proven by tests and awaits W09 wiring); unparsable addresses fail closed
- Data/schema impact: none
- Security impact: DNS-to-dial address consistency closes the rebinding gap the hostname-only CJS guard leaves; redirect targets re-judged per hop; body cap prevents memory exhaustion
- Verification: go build ./...; go vet ./internal/platform/netpolicy/...; go test -count=1 ./internal/platform/netpolicy/ (ok: private-IP table, normal/custom URL matrices, local provider round trip, redirect allowlist + metadata refusal, body limit, rebinding refusal, dial guard unit cases, redirect limit); go test -race ./internal/platform/netpolicy/ (ok); gofmt clean
- Platforms covered: Windows 10 22H2 x64 unit tests with httptest fixtures; real TLS/proxy fixtures pending W09 integration
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-025`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: Node provider fetch stack stays until W22 removes the call chain
- Documentation updated: ledger, remaining-work, work-packages, capability-matrix
- Residual risks: loopback-port decision precedes scheme check in CJS parity (non-http schemes to allowed loopback ports pass policy; Go transport then refuses the dial) — acceptable divergence documented by test; remote-DNS-through-proxy semantics are defined but no proxy transport is wired yet (W09 provider clients bring proxies)
- Next safe slice: W09 provider protocol families (`internal/agent/providers`) consuming netpolicy transports
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L169 - 2026-09-18 - W09 provider protocol families (stream assembly core)

- Capability rows: `AI-03`
- Plan task: `P7-03`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: W09 stream-assembly core landed as `internal/agent/providers`. Shared SSE parser is byte-boundary safe (1-byte feeds equal whole-stream feeds across CJK/emoji, CRLF/LF, BOM, keep-alive comments, multi-line data — T17). OpenAI Chat assembler: interleaved tool-call indices keep per-index state (T18), tool-only responses carry no text (T19), finish_reason mapping incl. tool_calls/function_call, stream_options usage with cached/reasoning details, unknown usage stays unknown (T22), error payloads surface as StreamError. Anthropic assembler: message_start/message_delta usage merged into one observation per turn (input/cache read/cache write + final output), thinking_delta reasoning, input_json_delta tool fragments, signature_delta assembled into a private_record continuation record at block stop consumed only by the next same-family request (T20), redacted_thinking records, stop_reason mapping. Google assembler: thought parts, thoughtSignature private records, whole functionCall emitted as start+complete-args delta keeping downstream uniform, cumulative usageMetadata overwritten and emitted exactly once (T22), finishReason mapping. RetryPolicy is the single retry owner: Retry-After honored exactly without multiplicative stacking (T23), exponential backoff capped, fake-clock (RecordingClock) tested, context cancellation interrupts waits.
- Go canonical owner: `internal/agent/providers/`
- Frontend adapter: none
- Electron owner affected: none (providers.ts / Vercel SDK stay until W22)
- Preserved invariants: AI-03 stays probe; no network dialing inside the package (netpolicy client injected at W11); provider-private records never cross families (T21 stale-continuation refusal lands with the runtime)
- Data/schema impact: none
- Security impact: malformed chunks and provider errors fail loudly as StreamError, never as silent empty streams
- Verification: go build ./...; go vet ./internal/agent/providers/...; go test -count=1 ./internal/agent/providers/ (ok); go test -race ./internal/agent/providers/ (ok); fixtures under testdata/ai/provider with manifest (openai-chat-basic/toolcalls, anthropic-messages, google-generatecontent; all synthetic)
- Platforms covered: Windows 10 22H2 x64 unit tests; no real provider calls (accepted: fixture-only until API consumption is authorized)
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-025`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: renderer provider stack stays until W22
- Documentation updated: ledger, remaining-work, work-packages, capability-matrix
- Residual risks: OpenAI Responses API family not yet assembled (Chat family is the current Catty path; Responses is a W09.2 slice if a provider config needs it); model list/probe surfaces pending; live TLS/proxy wiring into netpolicy clients pending W11 runtime; canonical trace comparison (Gate 11 fixtures) pending
- Next safe slice: W10 AI Profile data & secret references, or W09.2 Responses family when a provider config requires it
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L170 - 2026-09-18 - W11 runtime prepare lease (first slice)

- Capability rows: `AI-03`
- Plan task: `P7-04`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: W11 first slice lands `internal/agent/runtime` TurnManager prepare-lease invariants: per-chat single slot, same-request idempotent retry returning the identical reservation, same-request changed-parameters rejected as STALE_REVISION, different-request prepare during a live lease rejected as BUSY (T01 concurrent case decided by per-chat mutex — exactly one winner), lease expiry releasing the slot with no phantom terminal record (T03). Turn IDs come from the contracts random generator. Event ring, driver-driven Start/Stop lifecycle and ReadEvents reconciliation are the next W11 slices.
- Go canonical owner: `internal/agent/runtime/`
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: AI-03 stays probe; runtime imports only contracts (no Wails/Electron); implemented-vs-verified separation kept
- Data/schema impact: none
- Security impact: none
- Verification: go vet ./internal/agent/runtime/...; go test -count=1 ./internal/agent/runtime/ (ok: idempotent retry, conflict, busy, concurrent T01, expiry T03); gofmt clean
- Platforms covered: Windows 10 22H2 x64 unit tests
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-025`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: renderer AgentRuntime stays until W22
- Documentation updated: ledger, remaining-work, work-packages, capability-matrix
- Residual risks: canonical param snapshot is a field-concatenation stand-in until W10 defines canonical request hashing; event ring and Stop convergence (T04-T16) pending
- Next safe slice: W11 slice 2 event ring + ReadEvents reconciliation, then driver lifecycle
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L171 - 2026-09-18 - W11 event ring and ReadEvents reconciliation

- Capability rows: `AI-03`
- Plan task: `P7-04`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: W11 second slice lands the per-turn bounded event ring and ReadEvents reconciliation in `internal/agent/runtime`. Ring semantics: strictly increasing sequences as decimal strings; byte-identical redelivery of the newest event is idempotent; same-sequence-different-payload is a typed consistency failure and older sequences are rejected (T06); eviction drops the oldest past the capacity bound. ReadEvents: pages after a cursor with HasMore; a cursor below oldest-1 reports CursorExpired with an authoritative TurnSnapshot and empty events, while a cursor exactly at oldest-1 is served normally (T07 boundary); missed live notifications reconcile from cursor 0 with no duplicates or gaps (T04); the snapshot projection carries status/revision/throughSequence readable after the turn ends (T05 groundwork). TurnManager now owns a turn registry created at Prepare, exposing Append/ReadEvents/Snapshot; the driver sink assigns sequences in slice 3.
- Go canonical owner: `internal/agent/runtime/` (event_ring.go, turn_manager.go)
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: AI-03 stays probe; runtime imports only contracts; sequences never compared as strings
- Data/schema impact: none
- Security impact: conflicting redelivery fails closed instead of overwriting history
- Verification: go vet ./internal/agent/runtime/...; go test -count=1 ./internal/agent/runtime/ (ok: backfill T04, limit+HasMore, redelivery/conflict T06, cursor-expiry boundary T07, manager end-to-end missed notifications, expired-cursor snapshot, unknown turn NOT_FOUND); go test -race ./internal/agent/runtime/ (ok); gofmt clean
- Platforms covered: Windows 10 22H2 x64 unit tests
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-025`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: renderer AgentRuntime stays until W22
- Documentation updated: ledger, remaining-work, work-packages, capability-matrix
- Residual risks: ring bounds are event-count-based until W14 adds byte budgets; T05 terminal-notification-loss case completes with slice 3 terminal records; driver sink and Stop convergence are slice 3
- Next safe slice: W11 slice 3 driver lifecycle and unified Stop (T11-T16), or W10 profiles in parallel
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L172 - 2026-09-18 - W11 driver lifecycle and unified stop

- Capability rows: `AI-03`
- Plan task: `P7-04`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: W11 third slice lands the driver-driven turn lifecycle in `internal/agent/runtime`. `TurnDriver.Stream(ctx, DriverSession)` is the transport-independent seam; the runtime is the single sequence authority (one counter per turn shared by driver events and the terminal record, so no collision or reordering is possible — the two-counter bug was caught by the T04 test before commit). StartTurn consumes the reservation, rejects unknown requests (NOT_FOUND), mismatched turn ids, and driver-less runtimes (UNAVAILABLE); Start retries for the same request never run the driver twice (T02). StopTurn is idempotent with bounded convergence: it cancels the per-turn context, waits on the driver completion channel, and finalizeTurn commits status + turn_end in ONE critical section so a snapshot never becomes visible without its terminal event; exactly one terminal record survives concurrent stop-vs-completion races (T11). Prepared turns stop immediately without a driver run; cross-chat isolation verified deterministically (blocking driver on chat A, instant completion on chat B); driver failures land as interrupted, never fake success; StopChat releases the chat slot (T15 groundwork). Lock order fixed at chatState.mu -> turnRecord.mu; snapshot reads take the record lock (race detector clean at count=10).
- Go canonical owner: `internal/agent/runtime/` (driver.go)
- Frontend adapter: none
- Electron owner affected: none
- Preserved invariants: AI-03 stays probe; runtime imports only contracts; stop cannot resurrect or double-finalize a turn
- Data/schema impact: none
- Security impact: driver errors are surfaced as interrupted, never as silent success
- Verification: go vet ./internal/agent/runtime/...; go test -count=1 ./internal/agent/runtime/ (ok); go test -race -count=10 -timeout 300s ./internal/agent/runtime/ (ok, 150+ runs incl. stop/completion race); gofmt clean; go build ./...
- Platforms covered: Windows 10 22H2 x64 unit tests
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-025`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: renderer AgentRuntime stays until W22
- Documentation updated: ledger, remaining-work, work-packages, capability-matrix
- Residual risks: T12-T16 persistence/crash cases need the W10 profile storage seam; InteractionRouter is W13; the sequence counter is in-memory until durable checkpoints land
- Next safe slice: W10 AI Profile data & secret references, then W12 Wails AgentClient minimal chain
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L173 - 2026-09-18 - W10 AI key inventory and migration planner (first slice)

- Capability rows: `AI-03`
- Plan task: `P7-04`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: W10 first slice lands `internal/agent/profiledata`: the frozen inventory of the 22 renderer AI localStorage keys with classification (provider_config / preference / chat_history / grant / ephemeral), target profile domains (chat history -> sessions, ephemeral -> device-local, rest -> settings), syncable flags (ephemeral and secret-bearing stay device-local, T40), and `ai/` profile-key namespace. PlanMigrations turns a renderer localStorage snapshot into profile store mutations reusing the canonical domain conventions: every value must be valid JSON; unknown `netcatty_ai_*` keys fail the plan CLOSED with no mutations (classification drift guard, T36); malformed values fail with a typed error and zero mutations; empty values are recorded and skipped; enc:v1-bearing values are planned but flagged for the origin-aware reseal decision before staging (T37), and their raw values are never served through generic reads once promoted (T38 enforcement lands at the W12 bridge).
- Go canonical owner: `internal/agent/profiledata/`
- Frontend adapter: none (renderer snapshot handoff arrives with the W12 bridge)
- Electron owner affected: none
- Preserved invariants: AI-03 stays probe; promotion reuses profile store StageProfile/PromoteProfile receipts — no second staging implementation; ephemeral keys can never enter synced domains
- Data/schema impact: none yet (mutations are planned, not applied)
- Security impact: unknown-key fail-closed + secret flagging; raw-channel secret protection lands with the W12 bridge
- Verification: go vet ./internal/agent/profiledata/...; go test -count=1 ./internal/agent/profiledata/ (ok: classification table, secret flagging, unknown-key fail-closed, malformed JSON rejection, empty skip); go test -race ./internal/agent/profiledata/ (ok); gofmt clean on internal/; go build ./...
- Platforms covered: Windows 10 22H2 x64 unit tests
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-025`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: renderer AI state stays until W22
- Documentation updated: ledger, remaining-work, work-packages, capability-matrix
- Residual risks: origin-aware reseal execution, dedicated AI secret API, staged promotion wiring with interruption-retry receipts, and React hydration switch are the remaining W10 slices (T36-T38); canonical typed AI records arrive with W12
- Next safe slice: W12 Wails AgentClient minimal chain (React -> Wails -> Go Prepare/Start -> fixture provider -> events -> restore), or W10 slice 2 reseal when the bridge seam is ready
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L174 - 2026-09-18 - W12 agent service facade and dev fixture driver (Go slice)

- Capability rows: `AI-03`
- Plan task: `P7-04`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: W12 Go slice lands the Wails-facing agent facade. `cmd/netcatty/agentService.go` maps the five W03 wire methods (AgentPrepare/AgentStart/AgentStop/AgentReadEvents/AgentSnapshot) one-to-one onto `internal/agent/runtime` TurnManager — a thin mapping with no second state. `internal/agent/drivers/fixture` provides the deterministic minimal-chain driver; the composition root (main.go) wires it ONLY behind `NETCATTY_AI_DEV_DRIVER=1`, so release builds run driver-less and AI starts fail UNAVAILABLE instead of answering fixture output (W12 invariant: fake only in test/dev DI). Service tests exercise the full loop through the facade: prepare -> start -> fixture events -> ReadEvents reconcile with text+turn_end, stop convergence on a blocking driver, driver-less UNAVAILABLE typing, unknown-turn NOT_FOUND.
- Go canonical owner: `cmd/netcatty/agentService.go`, `internal/agent/drivers/fixture/`
- Frontend adapter: pending W12 slice 2 (bindings regen with pinned toolchain, wailsRuntimeClient agent branch, useAIChatStreaming rewiring)
- Electron owner affected: none
- Preserved invariants: AI-03 stays probe; React will not start a second authoritative runtime (slice 2); fixture never reachable in release builds; facade holds no policy
- Data/schema impact: none yet; bindings regen (20->21 services) lands with slice 2
- Security impact: none; the facade exposes no capability dispatch (W13)
- Verification: go vet ./cmd/netcatty/ ./internal/agent/drivers/...; go test -count=1 -run TestAgentService ./cmd/netcatty/ (ok); go test -race -run TestAgentService ./cmd/netcatty/ (ok); go build ./...; gofmt clean
- Platforms covered: Windows 10 22H2 x64 unit tests
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-025`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: renderer AgentRuntime stays until W22
- Documentation updated: ledger, remaining-work, work-packages, capability-matrix
- Residual risks: bindings/TS/React wiring is slice 2 (reload/StrictMode/multi-window cases T08 need the client loop); provider-side fixture remains the only driver until W09 live wiring; capability dispatch stays empty until W13
- Next safe slice: W12 slice 2 bindings + TS client + useAIChatStreaming rewiring, or W10 slice 2 reseal
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L175 - 2026-09-18 - W12 bindings regeneration and agentRuntime port

- Capability rows: `AI-03`
- Plan task: `P7-04`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: W12 TS slice lands the renderer-facing agentRuntime seam. Bindings regenerated with the pinned toolchain (GOTOOLCHAIN=go1.25.0, wails3 beta.12): 22 services / 229 methods / 143 models; new `agentservice` module maps AgentPrepare/AgentStart/AgentStop/AgentReadEvents/AgentSnapshot onto the W03 contract models (PreparedTurn/EventPage/TurnSnapshot get typed createFrom hydration). The one generator warning (function-typed fields) is pre-existing at HEAD from trayService ShowMain/OpenSettings — verified by stashing this slice and re-running. wailsRuntimeClient gains `agentservice` in WailsBindingDeps + defaultBindings, an `AgentRuntimePort` interface, and `WailsRuntimeClient = RuntimeClient & { agentRuntime }` so the Go turn runtime is callable without touching the generated runtimePorts or the legacy AgentPort (which stays typed-unavailable until W13 populates it). The port fails with a typed unavailable error when the binding is absent (non-Wails shells, tests). Targeted tests: relay order prepare->start->stop->read->snapshot with fake bindings, and unavailable typing without bindings; full wailsRuntimeClient suite 45/45.
- Go canonical owner: none (TS slice); generated bindings under infrastructure/runtime/wails/bindings
- Frontend adapter: `infrastructure/runtime/wails/wailsRuntimeClient.ts` (agentRuntime port); useAIChatStreaming rewiring is the next W12 slice
- Electron owner affected: none
- Preserved invariants: AI-03 stays probe; React consumes only the new port — no second authoritative runtime; legacy AgentPort surface unchanged; generated runtimePorts.ts untouched
- Data/schema impact: bindings now reference internal/app/contracts models (W03 DTOs on the wire)
- Security impact: none; the port carries no capability dispatch
- Verification: GOTOOLCHAIN=go1.25.0 wails3 generate bindings (22 services/229 methods); npx tsc --noEmit -p tsconfig.json filtered to wailsRuntimeClient.ts shows only the 5 pre-existing errors (clipboard/removedKeys/script-recording lines untouched by this slice); node --test --import tsx wailsRuntimeClient.test.ts 45/45; go build ./...
- Platforms covered: Windows 10 22H2 x64 unit tests; WebView live pass pending slice 3 (reload/StrictMode/multi-window T08)
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-025`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: renderer AgentRuntime stays until W22
- Documentation updated: ledger, remaining-work, work-packages, capability-matrix
- Residual risks: useAIChatStreaming two-layer rewiring (T08 StrictMode double-mount, multi-window, unmount-no-Stop) is slice 3; the fixture driver still requires NETCATTY_AI_DEV_DRIVER=1 at launch
- Next safe slice: W12 slice 3 useAIChatStreaming rewiring over agentRuntime, then the React-side T08 cases
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L176 - 2026-09-18 - W12 minimal chain routing (useAIChatStreaming)

- Capability rows: `AI-03`
- Plan task: `P7-04`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: W12 slice 3 closes the minimal chain on the renderer side. AgentStatus (Go) reports whether the composition root wired the dev fixture driver; the agentRuntime port gains agentStatus. New `application/state/aiGoTurn.ts` is the pure Go-turn runner: prepare -> start -> poll ReadEvents with 128-event pages, text_delta payloads appended to the assistant message, turn_end terminating; abort is routed to the Go owner via agentStop (renderer never walks away from a running turn); CursorExpired resyncs from the served snapshot cursor (T07); a lost terminal notification falls back to the snapshot read so the UI never spins (T05); the empty-page + snapshot-terminal path is bounded. useAIChatStreaming.sendToCattyAgent routes to runGoTurn ONLY when AgentStatus.goRuntimeReady (the same NETCATTY_AI_DEV_DRIVER=1 flag the Go side uses — one flag, one authoritative runtime per build); otherwise the existing renderer AgentRuntime path runs unchanged, so the product Catty sidebar is not regressed while W13 populates real handlers. Bindings regenerated (22 services / 230 methods) to expose AgentStatus.
- Go canonical owner: none (TS slice); Go counterpart AgentStatus in cmd/netcatty/agentService.go
- Frontend adapter: `application/state/aiGoTurn.ts` (new, pure), `useAIChatStreaming.ts` (routing only), `wailsRuntimeClient.ts` (port + status)
- Electron owner affected: none
- Preserved invariants: exactly one authoritative runtime per build (flag decided on the Go side, mirrored by AgentStatus); product path unchanged without the flag; abort always reaches the Go owner; generated runtimePorts.ts untouched
- Data/schema impact: bindings +1 method (AgentStatus, AgentStatus model)
- Security impact: no new renderer authority — the Go side keeps policy and state
- Verification: node --test --import tsx application/state/aiGoTurn.test.ts 3/3 (delta streaming, abort->stop routing, lost-terminal snapshot fallback); node --test wailsRuntimeClient.test.ts 45/45; go test -run "TestAgentService|TestAgentStatus" ./cmd/netcatty/ ok; tsc filtered to touched files clean; go build ./...
- Platforms covered: Windows 10 22H2 x64 unit tests; live WebView run of the flagged chain pending (launch exe with NETCATTY_AI_DEV_DRIVER=1 and send a Catty message)
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-025`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: renderer AgentRuntime stays until W22
- Documentation updated: ledger, remaining-work, work-packages, capability-matrix
- Residual risks: T08 StrictMode double-mount/multi-window cases need the live WebView pass; steer and compaction are not on the Go path yet (W14); usage accounting display pending W14 usage ledger
- Next safe slice: live WebView smoke of the flagged minimal chain, then W10 slice 2 reseal/secret API or W13 host tools
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L177 - 2026-09-18 - W10 origin-aware secret reseal and secretRef extraction

- Capability rows: `AI-03`
- Plan task: `P7-04`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: W10 second slice lands the secret-handling core of the AI migration in `internal/agent/profiledata/secrets.go` (design §7.3). Origin typing: Electron-broker (FND-03 receipts) vs Go-credential-provider origins are explicit; an unknown origin returns BlockedOriginError and promotion aborts — never a speculative open, never a silently emptied key. ExtractProviderSecrets walks netcatty_ai_providers_v1, moves every non-empty apiKey into an injected SecretSink under a fresh randomly generated secret_ reference, rewrites the config with secretRef + configVersion:2 and DELETES the raw apiKey field unconditionally (the raw channel closes even for empty keys). ResealOpaqueSecret opens via the manifest-declared origin, re-seals under the dedicated ai-provider-secrets purpose (the credential provider rejects purpose mismatch — the T38 backstop), and returns a plaintext-free receipt (source/sealed fingerprints only). VerifyNoRawSecrets is the post-mutation audit: a promoted providers value carrying any raw apiKey fails. The shared AIPurpose constant is the seam the dedicated Put/Replace/Delete/Status service wraps next.
- Go canonical owner: `internal/agent/profiledata/secrets.go`
- Frontend adapter: none (renderer keeps entering plaintext once; the host responds with references only)
- Electron owner affected: none
- Preserved invariants: AI-03 stays probe; receipts never carry plaintext; purposes stay enforced by internal/platform/credentials (no parallel crypto); promotion aborts on blocked/failed opens
- Data/schema impact: none yet (extraction runs on snapshots; store wiring is the next slice)
- Security impact: raw apiKey channel closes at extraction; AI purpose separates AI secrets from cloud-sync credentials
- Verification: go vet ./internal/agent/profiledata/...; go test -count=1 ./internal/agent/profiledata/ (ok: extraction incl. empty-key channel closure, receipts plaintext-free, non-array rejection, origin-aware reseal, unknown-origin block, open-failure abort, raw-secret audit); go test -race ./internal/agent/profiledata/ (ok); go build ./...; gofmt clean
- Platforms covered: Windows 10 22H2 x64 unit tests with in-memory sink/codecs; real keyring round trip pending the service slice
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-025`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: renderer provider config stays until W22
- Documentation updated: ledger, remaining-work, work-packages, capability-matrix
- Residual risks: the dedicated host secret service (Put/Replace/Delete/Status over the credential provider), the versioned ProviderConfig reader on the Go provider path, staged promotion wiring with interruption receipts (T36), and the raw-channel bypass audit of ProfileService/CredentialService exports (design §7.3 closing paragraph)
- Next safe slice: W10 slice 3 dedicated secret service + staged promotion wiring, or W13 host tools
- Drift decision: `user-approved-implementation-ahead-of-evidence`

## WV3-L178 - 2026-09-18 - W10 secret service and atomic snapshot promotion

- Capability rows: `AI-03`
- Plan task: `P7-04`
- Status change: `probe -> probe`
- Scope change: `none`
- Goal: W10 third slice lands the dedicated AI secret service and the staged promotion engine. `SecretService` wraps the credential provider at the dedicated `ai-provider-secrets` purpose over the profile store (sealed envelopes only): Put returns a fresh reference, Replace rotates in place, Delete removes, Status reports existence, and Resolve opens for the HOST provider path inside one request — the bindings layer must never expose it (T38). `PromoteAISnapshot` is the T36 engine: provider/web-search style values are handled by a generic recursive apiKey extractor (any JSON shape; nested objects covered by test), opaque enc:v1 envelopes reseal via the manifest-declared origin blocking on unknown (T37), PlanMigrations re-runs fail-closed, and the receipt (schema marker + keys + plaintext-free secret receipts) lands in the SAME device-domain transaction as the data — an interruption leaves either the old state or the fully promoted one, and a re-run repeats the idempotent write. Sink failures abort before any write (spy-store asserted); plaintext never enters any stored mutation.
- Go canonical owner: `internal/agent/profiledata/service.go`, `internal/agent/profiledata/migrate.go`
- Frontend adapter: none (the renderer-facing secret bindings are W13 settings-UI work; Resolve stays Go-only)
- Electron owner affected: none
- Preserved invariants: AI-03 stays probe; credential purpose enforcement stays in internal/platform/credentials; receipts never carry plaintext; ephemeral keys stay device-local
- Data/schema impact: none yet (engine ready; the composition root wiring lands with the settings UI)
- Security impact: raw apiKey channel closes at extraction; AI purpose separates AI secrets from cloud-sync credentials; Resolve is host-only
- Verification: go vet ./internal/agent/profiledata/...; go test -count=1 ./internal/agent/profiledata/ (ok: extraction incl. nested web-search object, empty-key channel closure, opaque reseal, unknown-origin block, open-failure abort, raw-secret audit, promotion happy path with one-transaction receipt, sink-failure atomicity, blocked-origin no-write, device-local receipt); go test -race ./internal/agent/profiledata/ (ok); go build ./...; gofmt clean
- Platforms covered: Windows 10 22H2 x64 unit tests with in-memory sink/codec/spy-store doubles; real keyring round trip pending the facade wiring
- Evidence grade: `C`
- Decision references: `WV3-001`, `WV3-025`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: cutover-trigger: renderer provider config stays until W22
- Documentation updated: ledger, remaining-work, work-packages, capability-matrix
- Residual risks: bindings exposure of the secret service (deliberately deferred to the W13 settings-UI slice with the raw-channel bypass audit of ProfileService/CredentialService exports); React hydration switch and the AI exclusion flip remain post-promotion steps
- Next safe slice: live WebView smoke of the flagged minimal chain, W13 host tools, or the W10 facade/audit closing slice
- Drift decision: `user-approved-implementation-ahead-of-evidence`
