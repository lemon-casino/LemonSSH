# LemonSSH 剩余工作清单（2026-09-16 更新）

本文档回答一个问题：**现在还有什么没有做完**。状态权威仍是
[capability-matrix.md](capability-matrix.md) 与 [migration-ledger.md](migration-ledger.md)；
本文是导航快照，与矩阵冲突时以矩阵为准。

当前台账头：`WV3-L176`。矩阵 required 叶行：implemented 15 / probe 14 /
not-started 7 / **verified 0 / migrated 0**。2026-09-14 基础切片：Wails
模块对齐 beta.12 + 版本漂移守卫（L133）、go1.27.1 工具链评估探针（L134，暂不锁
1.27，生成钉保持 go1.25.0）、存量漂移修复（L135）、插件 sidecar NUL 截断（L136）。
2026-09-14～16：五个 agent 去留决定 WV3-014～018（L137）、AI-04 拆八个子行（L138）、
脚本录制与回放全链路（L139、L140、L141、L142、L143、L146、L147、L148、L149、L150：录制、敏感对话框、waitForRegex/Any、
progress、startLog/stopLog、pause/resume、实时运行推送）、getText 行范围（L151）、
AI-04.4 至 AI-04.8 scope-removal（L152、L153、L154、L155、L156）、
dialog.form/select/radio/checkbox（L157）、unsigned qualification 不挡 P6-05（L158 / WV3-024）、
WV3-025 允许 unsigned 资格包之后开工 Phase 7（L159）、W03 Agent wire DTO（L160，AI-01 probe）、
W04 terminal 域共享 use case（L161）、W04 SFTP 域共享 use case（L162）、W04 forward 域共享 use case（L163）、W04 Vault 域裁决为无需提取（L164）、W05 catalog/policy/dispatch Go 权威（L165，四提交：
8f212038/5cef2877/4917b1fe/b7ded241，77 行 fixture parity + 投影语义全等）。
W06 本机 host RPC 核心已落 internal/rpc（L166：versioned envelope、frame 上限、
token 认证+principal scope、撤销/关闭生命周期、first-party discovery 契约；
composition root 随 W07 接线）。W07 原生二进制首两刀已落（L167：cmd/netcatty-tool + cmd/netcatty-mcp，
SDK 锁 v1.7.0，67 工具 stdio 冒烟 + typed unavailable 通过；composition root
与真实 vendor 矩阵仍欠）。W08 Provider 网络策略已落 internal/platform/netpolicy（L168：SSRF 守卫、
dial 地址校验防重绑定、redirect 逐跳重判、10MiB body 上限、自定义端点受限授权）。
W09 Provider 协议族流装配核心已落 internal/agent/providers（L169：SSE 解析器、
OpenAI Chat/Anthropic Messages/Google 三族装配器、usage 单次观测、签名 continuation
私有记录、单一 retry owner；Responses 族与 model list/probe 仍欠）。
W11 三刀已落 internal/agent/runtime（L170 Prepare 租约 T01-T03；L171 事件环
+ ReadEvents 对账 T04/T06/T07；L172 driver 生命周期 + 统一 Stop T02/T11，
单序列权威 + finalize 原子性，race count=10 干净）；W10 首刀已落 internal/agent/profiledata（L173：22 个 AI key 清单分类、
fail-closed 迁移计划、enc:v1 秘密标记、ephemeral 设备本地 T40）；
W12 Go 切片已落（L174：cmd/netcatty/agentService 五方法 facade +
NETCATTY_AI_DEV_DRIVER 门控的 fixture driver，发布包无 driver 时 UNAVAILABLE）；
W12 切片 2 已落（L175：bindings 22 services/229 methods 重新生成 +
agentRuntime 端口经生成模型直通 Go runtime，45/45 定向测试）；
W12 切片 3 已落（L176：aiGoTurn 运行器 + sendToCattyAgent 按 AgentStatus
路由，abort 走 Go owner 的 Stop，丢终态通知回退快照 T05/T07；产品路径在无
dev 旗标时不变）；最小完整链路代码面已通，活体 WebView 冒烟（以
NETCATTY_AI_DEV_DRIVER=1 启动发一条 Catty 消息）待跑。生产 AI 下一刀是
活体冒烟 + W10 后续（reseal/secret API），不是伪造 NONAI-COMPLETE。

处理标记：`已处理` = 本切片已接线或已诚实记录 pending；**不是** `verified`。

---

## 一、Wails 壳已落地的功能（近期完成）

以下已在 `bin/LemonSSH.exe` 中可用并有对应 ledger 记录：

- 无边框主窗口，应用内标题栏拖拽/最小化/最大化/关闭（`--wails-draggable`）
- 单实例：二次启动唤起已有主窗口（还原最小化并聚焦）
- 设置独立窗口：启动 2 秒后后台预创建，首屏绘制完成后显示（无黑/白闪）
- 无控制台黑窗（`-H windowsgui`）
- 本地 PTY / 密码 SSH / 密钥 SSH / Telnet / 串口，全部走同一 loopback 数据面
- SFTP：打开/列表/建目录/删除/重命名/stat/文本读写/下载/上传
- 端口转发（local/remote/SOCKS5，经共享 SSH 池）
- 系统：文件/目录/保存对话框、托盘（LemonSSH 品牌 + Settings 入口）、
  deep link 解析（未注册 OS 协议）、App Lock 最小运行时（未锁定态）、
  插件清单（只读、为空）
- 品牌：应用名/窗口标题/托盘/设置页均为 LemonSSH；任务栏图标为
  exe 内嵌 ICO（资源 ID 3）；应用内图标切换功能已移除

---

## 一点五、本轮新增活体与工具（L106–L110）

- 活体矩阵门控测试 `cmd/netcatty/live_matrix_test.go`：SSH exec、SFTP 往返、
  远程转发回显、真实 mosh-server 握手全部 PASS（凭据仅走环境变量）。
- `cmd/updatefeed`：自管 ed25519 更新 feed 的 keygen/sign 工具，roundtrip
  经 `updater.VerifyManifest` 验证。
- `npm run wails:build` 现在把 package.json 版本戳进二进制；遗留 Electron
  流水线断言已从 workflow 测试移除（14/14 绿）。

---

## 二、功能缺口（有 Go owner 但壳未接，或 owner 不完整）

### 终端协议
| 项 | 缺口 | 涉及行 | 处理 |
| --- | --- | --- | --- |
| SSH MFA / keyboard-interactive | Wails Connect 现把挑战发到现有渲染层弹窗；command proxy 与 one-auth 并发已活体通过（L112）；活体 MFA 服务器仍缺 | SSH-01 | 已处理 |
| SSH 跳板链 / socks5/http proxy | Connect 结构体透出 jumpHosts + proxyUrl；command 代理仍显式拒绝 | SSH-01 | 已处理 |
| SSH 用户证书 | Connect 透出 certificate + 私钥，ParseCertificateSigner 接到 x/crypto | SSH-01 | 已处理 |
| SSH agent / IdentityFile | Connect 透出 useAgent 与 identityFilePaths；缺文件失败关闭；活体 agent 仍缺 | SSH-01 | 已处理 |
| Mosh / ET | StartMosh/StartEt 解析 bundled/dev helper 路径；192.168.0.6 (Debian 13) 上对真实 mosh-server 1.4.0 的 MOSH CONNECT 抓取已活体通过（L107）；route reconnect 保留原生进程/bootstrap 已实现；helper 供给/校验/打包链路已交付：lock 锁定 MoshCatty 0.1.8 与 et-bin 6.2.10-1、8 目标本机校验通过、打包自动捆绑 helper+许可证并写入清单（L123）；进程死亡恢复（同会话手动重启新 shell）与 proxy/jumpHosts 渲染层透传已接（L124）；网络漫游活体与装机后的真机验收仍缺 | TERM-03.3 | 已处理 |
| ZMODEM 完整 rz/sz 会话 | 私有长度前缀已替换为标准 ZMODEM wire；独立 zmodem.js 双向传输/CRC32 夹具通过；真实 lrzsz 对端仍未验证，CRC 错误中止而非重传（L117） | TERM-03.4 | 已处理 |
| 串口 YMODEM | SendSerialYmodem/ReceiveSerialYmodem 接到打开的串口会话 | TERM-03.2 | 已处理 |
| Serial/Telnet 活体设备矩阵 | 无真实硬件证据 | TERM-03.1, TERM-03.2 | pending |

ET inline auth/proxy/jump 仍不支持（超出原始 roaming 范围）；route/roaming 仅保留活进程，进程死亡恢复未实现。

Helper 供给与校验已由 L123 交付：`fetch-wails-helpers.lock.json` 锁定
MoshCatty 0.1.8 与 et-bin 6.2.10-1，八个目标本机 `--verify-only --all` 通过，
`package-wails` 经 lock 校验捆绑 helper、sidecar 与许可证。Git 仍只跟踪
`resources/mosh` 与 `resources/et` 的 README，binary 由脚本下载不入库。
装机后的三平台活体连接仍属验收。

### SFTP / 传输
- 本地面板 HomeDir/ListDir 接到真实文件系统；桌面桥缺失能力时报错，演示文件仅用于无后端浏览器预览（SYS-01 / WV3-L094）— 已处理；真实远端上传复验仍 pending
- 下载/上传经 `startStreamTransfer` 接到现有 ClientFS；本地 zip 解压接到 `ExtractArchive`（SFTP-01）— 已处理
- filesystem/transfer 绑定已进入 defaultBindings；缺 ExtractArchive 时失败关闭，不再假成功 — 已处理
- 调度器 pause/resume/cancel 接到 Wails TransferService；startCompressedUpload 走本地 zip 再 Upload（SFTP-02）— 已处理
- sudo SFTP 子系统启动和显式错误代码已接；真实 sudo 权限服务器未验证 — pending（L118）；非 UTF-8 文件名矩阵已在 192.168.0.6 上活体通过（L112）
- 远程 zip 解压：下载到临时目录、zip-slip 提取、再上传（SFTP-01）— 已处理

C4 已完成 ZIP staging 与同 task ID scheduler 上传，两阶段控制、backend epoch、List reload observation 均已接；Go 定向测试与 main TS 33/33 通过（L121）。最终 pinned generation 20 services/142 methods/50 models 无 warnings，Wails build 产出 bin/LemonSSH.exe v0.0.1，cmd/transfer/dataplane/zmodem/plugin Go race 全部 PASS（既存 multiple-manifest linker warning）；冻结后集成 Node suite 102/102 PASS；TS33 覆盖 raw1000 → ZIP10 计量，压缩期间 transferred bytes=0，实际上传后才计 ZIP 字节。上传 ZIP 不隐式解压，沿用 UploadCompressedFolder 路径。最终 metric 修正后的 npm run wails:build 亦 PASS / exit 0。filesystem/transfer 共用 profile temp，UI TempInfo/TempFilePath/ClearTemp 已接；租约注册表保护运行中条目，boot 清扫移除上一会话孤儿 staging（L125），运行中放弃的上传须等下次启动回收。

### 系统能力
- App Lock 密码启用 / PBKDF2 verifier / Unlock/Disable 已接；UnlockWithBiometrics 已接原生 Hello/Touch ID adapter；本机 Hello IsSupported=false，成功认证与 macOS 活体仍缺（L116）（SYS-04）— 已处理
- deep link 二次启动 argv 入队；System 标签新增开关，Windows 下写 HKCU 注册 ssh/telnet/netcatty 协议（无需管理员）；macOS/Linux 注册 adapter 已实现，安装包三平台投递仍缺（L116）（SYS-03）— 已处理
- 快捷键 Register 接到 ShortcutService；原生 accelerator adapter 已接；Windows RegisterHotKey 冲突/释放实测通过，macOS/Linux 原生执行仍缺（L116）（SYS-02）— 已处理
- 脚本录制 Start/Stop/AppendStep 接到 Go `internal/script` + ScriptService（L139）；敏感步骤、sleep gap、步数上限与 Electron codegen 对齐
- 录制脚本回放 `scriptRun`/`scriptStop`/`scriptGetRuns` 接到 Go runner（L140）：sleep、sendLine、waitForPrompt/waitForText；含 dialog/log/disconnect 的脚本仍拒绝，不假装 Node Worker 已迁移
- waitForPrompt/waitForText 现在观察 TerminalService 输出（L141），匹配 Electron 的常见提示符和文本身，超时才失败
- 敏感步骤可回放（L142）：`nct.dialog.prompt` 经 Wails 事件发给现有 ScriptDialogHost，答案经 ResolveDialog 回给 Go runner
- 脚本 pause/resume 接到 Go runner（L143），`nct.log` 和 `dialog.alert` 也已支持（log/alert 走运行日志与对话框）
- `nct.progress.start/set/step/done` 接到 Go runner（L144）：确定型进度写入运行快照，面板进度条可见；含变量的进度参数仍拒绝
- `session.disconnect` 接到 Go runner（L145）：断开后运行标记完成，不再写终端；`startLog/stopLog` 仍拒绝（无 Go 日志 owner）
- `waitForRegex` / `waitForAny` 接到 Go runner（L146）：字符串按字面量、`/正则/i` 形式按正则，匹配新鲜输出尾部；`screen.getText/send/clear` 仍拒绝
- 正则引擎换成 regexp2（L147）：回引用、环视等 JavaScript 特有语法可以工作，2 秒匹配超时防回溯挂死
- `screen.send/clear/getText` 与 `dialog.confirm` 接到 Go runner（L148）：send 不追加回车，clear 清空 runner 输出视图，getText 捕获当前输出到变量，confirm 返回 true/false
- `session.startLog/stopLog` 接到 Go 会话日志流（L149）：默认写入 profile 目录 session-logs/，支持自定义路径
- `dialog.form/select/radio/checkbox` 接到 Go runner（L157）：select/radio/checkbox 收成 form，答案 JSON 回传现有 ScriptDialogHost
- 运行状态改为实时推送（L150）：runner 每次变更广播 `netcatty:script:runs-updated`，面板进度条/日志/状态即时刷新，替代临时轮询
- 弹出终端窗口：PopupWindowService 打开 `#/terminal-popup` 并 emit config；会话窗口角色栅栏与清理代码已接；多显示器/崩溃活体矩阵仍缺（L116）（FND-04）— 已处理

### 数据与同步
- 非 AI Go canonical adapter 已在 L115 完成修正并重新推进 implemented：内存同步读取、挂载前 hydrate、一次性 legacy 导入、删除防复活及跨窗口刷新均有测试；保留 L114 对早期误报的纠正。多机恢复验收待补；AI 继续 localStorage，P6-05 未解锁。
- 云同步：Go WebDAV 与 S3 sigv4 快照传输均已落地并接入渲染层 cloudSync 端口（SYNC-02，L111）；GitHub 设备流、Google/OneDrive PKCE（本地回调服务器）经原生桥接通，密钥轮换对话框走 Go CAS 事务（L126）；三个 provider 的 client ID 支持设置页运行时配置（L127），空值不再打开坏 URL；申请入口经白名单 OpenProviderConsole 打开（L128）、client ID 字段带悬停填写指导，GitHub 设备流轮询的 400 authorization_pending 不再误报失败（L129），live 实测确认用户应用未开启设备流，StartDevice 现将 device_flow_disabled 映射为可操作提示（L130）；provider 令牌经 Go 凭据提供者（OS 钥匙环）密封存储、设备流验证页经系统浏览器打开（L131）；会话主密钥由 Go 服务密封持久化（修复 cloudSyncGetSessionPassword 未迁移报错），解锁弹窗新增两步「忘记主密钥」重置（L132） — 已处理；活体授权证据仍缺

### 插件
- Go/Wails plugin host settings/list/card、显式 once/session broker、encrypted secrets、durable recovery 和 localized v1 rejection 已实现；7 个 frontend tests、全部 Go/plugin、targeted cmd、scoped eslint/check:plugin-contract 通过（L119）。
- WASM 与 host-rendered 声明式 UI 已接，不依赖 Agent catalog；GUI/native 平台验收仍 pending（PLUG-02）。
- native 进程运行时已接到 PluginService；Stop 关闭 Windows job handle 以回收子孙；本机 TestStopReapsDescendant 通过；签名变体和 macOS/Linux 活体树仍 pending（PLUG-03）— 已处理

### AI（全部）
- WV3-025 取代 WV3-009：unsigned 资格包之后可建 `internal/capability` / `internal/agent`。不要伪造 NONAI-COMPLETE。
- W03 已落（L160）：`internal/app/contracts/agent.go` 的 Prepare/Prepared/Command/ReadEvents/EventPage DTO、
  十进制字符串序列、fake driver 仅测试；`AI-01` 已 `not-started -> probe`。
- W04 已收口（L161-L164）：terminal/session/exec → `internal/app/terminaluse`，
  SFTP → `internal/app/sftpuse`（复用 terminaluse 传输 seam，无第二套池），
  forward → `internal/app/forwarduse`（同池 KindForward 租约，epoch 参数留给
  capability dispatch）；Vault 复核确认 Go 侧 owner 已是 `internal/profile/store`
  + 薄 facade，不再包一层（L164）。
- W05 已落（L165）：`internal/capability` 拥有 77 行 catalog（含 CJS last-wins
  查找语义）、policy 决策（observer/confirm/auto、chatSession/cancel 门）、
  permission grant 匹配（含 shell 分段安全边界：heredoc/算术展开/注释/后台段）、
  fail-closed dispatch（未知默认拒绝、harness 显式 HANDLER_MISSING、审批与
  grant 双路径 + 授权后 re-check 挡撤销/Stop 竞态）、rpc 超时、agent-kind 投影
  与 `cmd/netcatty-capability-codegen`。parity 由 `testdata/ai/catalog` fixtures
  钉住（77 行 + 73 schema + 72/67/67 specs 与 CJS 语义全等）；Node 生成器保持
  提交物 owner 至 W22。
- W06 核心已落（L166）：`internal/rpc` 拥有 versioned NDJSON 协议（1 MiB 帧
  上限、oversized 排空后连接可用、EOF 截断终止）、SHA-256 digest token 认证
  （first-party/external principal、RevokeAll 永久撤销）、scope 守卫
  （RequireSession 挡伪造 session）、per-request deadline、frames-only-on-wire、
  Close 收敛全部连接，discovery 文件复用既有 0600 first-party 契约。
- W07 首两刀已落（L167）：`cmd/netcatty-tool`（catalog CLI 投影 + RPC client +
  typed unavailable + CJS 错误措辞/退出码）与 `cmd/netcatty-mcp`（官方 Go SDK
  v1.7.0 stdio server，67 工具投影，tools/call 中继 host RPC，零 policy 副本）。
  旧 CLI 特例命令的输出格式随 W13 落入 host handler；真实 vendor 客户端矩阵未跑。
- W08 已落（L168）：`internal/platform/netpolicy` 端点白名单/SSRF 守卫/
  dial-IP 校验（DNS 重绑定拒绝）/redirect 重判/body 限额；NewClient 输出标准
  http.Client 供 W09 的 Provider SDK 注入。
- W09 核心已落（L169）：`internal/agent/providers` — SSE 解析器（T17 字节边界
  安全）、三族装配器（OpenAI Chat 交错工具调用 T18/T19；Anthropic 签名 continuation
  私有记录 T20；Google 整体 functionCall + 累计 usage 单次观测 T22）、单一 retry
  owner（T23 Retry-After 精确遵从 + capped 指数退避 + fake clock）。
- W11 首刀已落（L170）：`internal/agent/runtime` TurnManager 租约与幂等
  （T01/T02/T03）。
- W11 切片 2 已落（L171）：有界事件环（重复投递幂等/冲突拒绝 T06）+
  ReadEvents 分页与 CursorExpired→快照对账（T04/T07）。
- W11 切片 3 已落（L172）：TurnDriver seam、幂等 Start、有界 Stop 收敛、
  单终态记录、跨 chat 隔离、driver 失败即 interrupted。
- W10 首刀已落（L173）：AI key 清单分类 + fail-closed 迁移计划构建器。
- W12 Go 切片已落（L174）：AgentService facade（五方法直映 runtime）+
  dev 门控 fixture driver + facade 级最小链路测试。
- W12 切片 2 已落（L175）：agentservice bindings + AgentRuntimePort
  （WailsRuntimeClient = RuntimeClient & { agentRuntime }），无绑定时 typed
  unavailable。
- W12 切片 3 已落（L176）：aiGoTurn 运行器 + AgentStatus 路由；产品路径
  无旗标时不变。
- 下一刀：NETCATTY_AI_DEV_DRIVER=1 活体 WebView 冒烟（T08 场景），随后
  W10 后续（reseal/secret API/staging 推进）或 W13 host 工具。
- 保留实现：AI-02, AI-03, AI-04.1 Codex, AI-04.2 Claude, AI-04.3 Grok — 仍 `not-started`
- AI-04.4, AI-04.5, AI-04.6, AI-04.7, AI-04.8 已 `removed` / `retired`（WV3-019 至 WV3-023，L152 至 L156）。
  Phase 7 对这五家只做 fail-closed、设置页原因、历史可读、禁止付费/盲切账户。

---

## 三、验收与证据缺口（无法本机伪造）

`verified` 需要证据等级 A。以下缺失导致矩阵 verified=0：

1. **三平台活体证据**：Windows 仅有本机 C 级；macOS/Linux 无窗口、拖拽、
   托盘、PTY、数据面配对基准 — pending；CI 已扩 remaining-work 契约测试
2. **真实服务器矩阵**：SSH MFA/跳板、真实 SFTP 服务器、编码/符号链接 — pending
3. **签名与安装包**：WV3-024 把付费证书推迟到以后。REL-01 保持 probe，不挡 P6-05。
   v0.0.2 与本机 `package-wails` 三平台开窗已通过（C 级）。`signed` 恒为 false。
4. **自动更新**：无生产签名 feed（REL-02 probe）。N-1→N 演练基础设施在；P8-01 仍要签名。不挡 P6-05。
5. **Electron 性能基线**：CI 上 3 个 best-effort 基线 job 抖动失败
   （不阻塞 `test` workflow）— pending
6. **干净机冒烟**：P8-01 Gate 所需的 signed clean-machine 矩阵 — pending
7. **已完成**：NSIS/deb/rpm/AppImage 打包脚本（工具缺失诚实跳过，L111）、updatefeed 自签 feed 工具、N-1→N 演练（feed+升级状态机+篡改拒绝，L111）、S3 sigv4 传输（L111）

---

## 四、硬门槛与顺序（不能跳）

```
P6-05 NONAI-COMPLETE（FND/TERM/SSH/SFTP/NET/SYS/SYNC/PLUG required 叶子 verified；REL-01/02 按 WV3-024 不参与）
        │  ← 当前卡这里
        ▼
Phase 7 AI（P7-01 → P7-06）
        ▼
P8-01 签名 RC 全量 Gate → P8-02 WAILS-CUTOVER → P9 退役 Electron
```

- `NONAI-COMPLETE` **不能记录**：verified=0。
- Phase 7 production 路径按 WV3-025 可在 unsigned 资格包之后开工；P8-02 仍要 AI 叶子 verified 或 removed。
- Phase 8/9 切默认发行物、删除 Electron 全部排队。
- REL-03 是 aggregate，必须保持 not-started。
- REL-03.1 第一次推进只能走 P8-01；现在还没有签名 RC，所以保持 not-started。
- REL-03.2 在 WAILS-CUTOVER 和 ROLLBACK-CLOSED 之前不能推进。

---

## 五、建议优先级（下一步可执行）

> 可编码功能已拆成带复选框的执行清单：
> [pre-acceptance-backlog.md](pre-acceptance-backlog.md)。

1. SSH MFA / 跳板 UI 回调 — 已处理（C 级；活体服务器仍缺）
2. SFTP 高级路径接线 — 已处理（下载/上传/本地解压；传输中心 UI/progress/pause/resume/cancel 与 reload/epoch 接线已补，最终集成及活体吞吐矩阵待补（L118））
3. App Lock 密码启用/解锁 — 已处理（生物识别 adapter 已接，成功原生认证证据仍缺）
4. CI 化三平台冒烟扩充 — 已处理（契约测试接入 migration-evidence；活体窗口仍缺）
5. 签名与安装包试点 — 已处理（诚实 unsigned probe；不伪造 signtool 成功）
6. 以上每项落地后回写矩阵/台账；凑齐 A 级证据后逐行升 `verified`，
   最后记 P6-05 gate。

---

## 六、已知的非阻塞瑕疵

- F3 范围 textZip/plugin-cli/port-forward rule 的 13 个类型错误已清零；全仓 `tsc --noEmit` 仍有大量历史错误和并行变更诊断，不能宣称全量通过。
- F1/F2 已修复：首 credit 前及 writer pending 合计 1 MiB；复制 producer 数据、原子 admission；closed/full 显式错误传至 producer，发送失败关闭会话；数据面 race 测试通过（L117）。
- F4 已修复：根 `.test.ts`/`.test.tsx` 精确放行，真实 Git 行为测试通过；scratch/dependencies 仍忽略。
- 插件 E3 为遗留 Electron 部分证据：真实 smoke PASS / PLUGIN_RUNTIME_SMOKE_OK；常规 suite 挂起，两个既存 NUL SQLite sidecar 失败及 open handle 未清除。按用户 Wails 目标停止扩大遗留排查，不计为完整 E3 通过，也不当作 Wails 产品验收。
- `npm run pack:dir` 未运行（遗留 Electron 目标），不作为本轮 Wails 构建要求。main 报告 Wails build 与 frontend build PASS / exit 0，最终 bindings 20 services / 138 methods、无 warnings，集成 Node tests 101/101；fresh Go cmd/dataplane/transfer/zmodem/ymodem race tests PASS（有既存 linker multiple-manifest warning）（L119）。
