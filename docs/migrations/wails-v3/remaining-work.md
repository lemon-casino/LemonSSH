# LemonSSH 剩余工作清单（2026-09-12 更新）

本文档回答一个问题：**现在还有什么没有做完**。状态权威仍是
[capability-matrix.md](capability-matrix.md) 与 [migration-ledger.md](migration-ledger.md)；
本文是导航快照，与矩阵冲突时以矩阵为准。

当前台账头：`WV3-L122`。矩阵 36 行：implemented 17 / probe 12 / not-started 7 /
**verified 0 / migrated 0**。

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
| Mosh / ET | StartMosh/StartEt 解析 bundled/dev helper 路径；192.168.0.6 (Debian 13) 上对真实 mosh-server 1.4.0 的 MOSH CONNECT 抓取已活体通过（L107）；route reconnect 保留原生进程/bootstrap 已实现；helper hash/arch 供给脚本已测，实际 Windows/macOS 二进制捆绑和网络漫游活体仍缺（L117） | TERM-03.3 | 已处理 |
| ZMODEM 完整 rz/sz 会话 | 私有长度前缀已替换为标准 ZMODEM wire；独立 zmodem.js 双向传输/CRC32 夹具通过；真实 lrzsz 对端仍未验证，CRC 错误中止而非重传（L117） | TERM-03.4 | 已处理 |
| 串口 YMODEM | SendSerialYmodem/ReceiveSerialYmodem 接到打开的串口会话 | TERM-03.2 | 已处理 |
| Serial/Telnet 活体设备矩阵 | 无真实硬件证据 | TERM-03.1, TERM-03.2 | pending |

ET inline auth/proxy/jump 仍不支持（超出原始 roaming 范围）；route/roaming 仅保留活进程，进程死亡恢复未实现。

Helper 交付审计：Git 仅跟踪 resources/mosh 与 resources/et 的 README；本地
win32-x64 mosh-client.exe 存在但缺少 manifest pin，其余 Windows/macOS
helper binary 和对应 pin 均缺。已通过的 Wails build 不等于 package-wails
带 helper 的发布打包成功；供给/校验脚本已实现，实际 helper 交付待完成。

### SFTP / 传输
- 本地面板 HomeDir/ListDir 接到真实文件系统；桌面桥缺失能力时报错，演示文件仅用于无后端浏览器预览（SYS-01 / WV3-L094）— 已处理；真实远端上传复验仍 pending
- 下载/上传经 `startStreamTransfer` 接到现有 ClientFS；本地 zip 解压接到 `ExtractArchive`（SFTP-01）— 已处理
- filesystem/transfer 绑定已进入 defaultBindings；缺 ExtractArchive 时失败关闭，不再假成功 — 已处理
- 调度器 pause/resume/cancel 接到 Wails TransferService；startCompressedUpload 走本地 zip 再 Upload（SFTP-02）— 已处理
- sudo SFTP 子系统启动和显式错误代码已接；真实 sudo 权限服务器未验证 — pending（L118）；非 UTF-8 文件名矩阵已在 192.168.0.6 上活体通过（L112）
- 远程 zip 解压：下载到临时目录、zip-slip 提取、再上传（SFTP-01）— 已处理

C4 已完成 ZIP staging 与同 task ID scheduler 上传，两阶段控制、backend epoch、List reload observation 均已接；Go 定向测试与 main TS 33/33 通过（L121）。最终 pinned generation 20 services/142 methods/50 models 无 warnings，Wails build 产出 bin/LemonSSH.exe v0.0.1，cmd/transfer/dataplane/zmodem/plugin Go race 全部 PASS（既存 multiple-manifest linker warning）；冻结后集成 Node suite 102/102 PASS；TS33 覆盖 raw1000 → ZIP10 计量，压缩期间 transferred bytes=0，实际上传后才计 ZIP 字节。上传 ZIP 不隐式解压，沿用 UploadCompressedFolder 路径。最终 metric 修正后的 npm run wails:build 亦 PASS / exit 0。filesystem/transfer 共用 profile temp，UI TempInfo/TempFilePath/ClearTemp 已接；ClearTemp 跳过 staged prefixes，可能保留孤儿 stage（L122）。

### 系统能力
- App Lock 密码启用 / PBKDF2 verifier / Unlock/Disable 已接；UnlockWithBiometrics 已接原生 Hello/Touch ID adapter；本机 Hello IsSupported=false，成功认证与 macOS 活体仍缺（L116）（SYS-04）— 已处理
- deep link 二次启动 argv 入队；System 标签新增开关，Windows 下写 HKCU 注册 ssh/telnet/netcatty 协议（无需管理员）；macOS/Linux 注册 adapter 已实现，安装包三平台投递仍缺（L116）（SYS-03）— 已处理
- 快捷键 Register 接到 ShortcutService；原生 accelerator adapter 已接；Windows RegisterHotKey 冲突/释放实测通过，macOS/Linux 原生执行仍缺（L116）（SYS-02）— 已处理
- 弹出终端窗口：PopupWindowService 打开 `#/terminal-popup` 并 emit config；会话窗口角色栅栏与清理代码已接；多显示器/崩溃活体矩阵仍缺（L116）（FND-04）— 已处理

### 数据与同步
- 非 AI Go canonical adapter 已在 L115 完成修正并重新推进 implemented：内存同步读取、挂载前 hydrate、一次性 legacy 导入、删除防复活及跨窗口刷新均有测试；保留 L114 对早期误报的纠正。多机恢复验收待补；AI 继续 localStorage，P6-05 未解锁。
- 云同步：Go WebDAV 与 S3 sigv4 快照传输均已落地并接入渲染层 cloudSync 端口（SYNC-02，L111）；OAuth 与密钥轮换仍未接 — 已处理

### 插件
- Go/Wails plugin host settings/list/card、显式 once/session broker、encrypted secrets、durable recovery 和 localized v1 rejection 已实现；7 个 frontend tests、全部 Go/plugin、targeted cmd、scoped eslint/check:plugin-contract 通过（L119）。
- WASM 与 host-rendered 声明式 UI 已接，不依赖 Agent catalog；GUI/native 平台验收仍 pending（PLUG-02）。
- native 进程运行时已接到 PluginService；Stop 关闭 Windows job handle 以回收子孙；本机 TestStopReapsDescendant 通过；签名变体和 macOS/Linux 活体树仍 pending（PLUG-03）— 已处理

### AI（全部）
- P7-01~P7-06：capability catalog、MCP/CLI、providers、Catty runtime、
  外部 Agent、退役 CJS 路径——**硬阻塞于 P6-05 gate**（AI-01~04）— pending（禁止开工）

---

## 三、验收与证据缺口（无法本机伪造）

`verified` 需要证据等级 A。以下缺失导致矩阵 verified=0：

1. **三平台活体证据**：Windows 仅有本机 C 级；macOS/Linux 无窗口、拖拽、
   托盘、PTY、数据面配对基准 — pending；CI 已扩 remaining-work 契约测试
2. **真实服务器矩阵**：SSH MFA/跳板、真实 SFTP 服务器、编码/符号链接 — pending
3. **签名与安装包**：无代码签名、无 msi/pkg/AppImage/deb/rpm 打包 — pending；
   `scripts/sign-wails-probe.mjs` 只记录 unsigned 原因，不伪造签名；
   `package-wails.mjs` 现在写出 purity inventory，signed 恒为 false（WV3-L103）
4. **自动更新**：无签名 feed、无 N-1→N 活体演练（REL-02 probe）— pending
5. **Electron 性能基线**：CI 上 3 个 best-effort 基线 job 抖动失败
   （不阻塞 `test` workflow）— pending
6. **干净机冒烟**：P8-01 Gate 所需的 signed clean-machine 矩阵 — pending
7. **已完成**：NSIS/deb/rpm/AppImage 打包脚本（工具缺失诚实跳过，L111）、updatefeed 自签 feed 工具、N-1→N 演练（feed+升级状态机+篡改拒绝，L111）、S3 sigv4 传输（L111）

---

## 四、硬门槛与顺序（不能跳）

```
P6-05 NONAI-COMPLETE（全部非 AI required 叶子 verified + REL-01/02 verified）
        │  ← 当前卡这里
        ▼
Phase 7 AI（P7-01 → P7-06）
        ▼
P8-01 签名 RC 全量 Gate → P8-02 WAILS-CUTOVER → P9 退役 Electron
```

- `NONAI-COMPLETE` **不能记录**：verified=0。
- Phase 7 任何 production 代码在 gate 前禁止开工（WV3-009）。
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
