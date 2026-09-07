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
