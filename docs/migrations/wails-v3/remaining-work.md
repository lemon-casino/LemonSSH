# LemonSSH 剩余工作清单（2026-09-10 快照）

本文档回答一个问题：**现在还有什么没有做完**。状态权威仍是
[capability-matrix.md](capability-matrix.md) 与 [migration-ledger.md](migration-ledger.md)；
本文是导航快照，与矩阵冲突时以矩阵为准。

当前台账头：`WV3-L093`。矩阵 34 行：implemented 6 / probe 19 / not-started 9 /
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

## 二、功能缺口（有 Go owner 但壳未接，或 owner 不完整）

### 终端协议
| 项 | 缺口 | 涉及行 | 处理 |
| --- | --- | --- | --- |
| SSH MFA / keyboard-interactive | Wails Connect 现把挑战发到现有渲染层弹窗；活体 MFA 服务器仍缺 | SSH-01 | 已处理 |
| SSH 跳板链 / socks5/http proxy | Connect 结构体透出 jumpHosts + proxyUrl；command 代理仍显式拒绝 | SSH-01 | 已处理 |
| SSH 用户证书 | Connect 透出 certificate + 私钥，ParseCertificateSigner 接到 x/crypto | SSH-01 | 已处理 |
| SSH agent / IdentityFile | Connect 透出 useAgent 与 identityFilePaths；缺文件失败关闭；活体 agent 仍缺 | SSH-01 | 已处理 |
| Mosh / ET | 监督 runner 仍在；产品路径诚实失败，reconnect 协议未接 | TERM-03.3 | 已处理 |
| ZMODEM 完整 rz/sz 会话 | 取消入口已接到失败关闭；会话引擎未实现 | TERM-03.4 | 已处理 |
| 串口 YMODEM | SendSerialYmodem 诚实失败关闭 | TERM-03.2 | 已处理 |
| Serial/Telnet 活体设备矩阵 | 无真实硬件证据 | TERM-03.1/2 | pending |

### SFTP / 传输
- 下载/上传经 `startStreamTransfer` 接到现有 ClientFS；本地 zip 解压接到 `ExtractArchive`（SFTP-01）— 已处理
- filesystem/transfer 绑定已进入 defaultBindings；缺 ExtractArchive 时失败关闭，不再假成功 — 已处理
- 调度器 pause/resume/cancel 接到 Wails TransferService；startCompressedUpload 走本地 zip 再 Upload（SFTP-02）— 已处理
- sudo SFTP、非 UTF-8 文件名矩阵未验证 — pending
- 远程 zip 解压：下载到临时目录、zip-slip 提取、再上传（SFTP-01）— 已处理

### 系统能力
- App Lock 密码启用 / PBKDF2 verifier / Unlock/Disable 已接；生物识别未接（SYS-04）— 已处理
- deep link 二次启动 argv 入队；渲染层 drainDeepLinks + onSshDeepLink 已接；OS 协议注册未做（SYS-03）— 已处理
- 快捷键 Register 接到 ShortcutService；Wails alpha.63 无 GlobalShortcut，原生注册诚实失败关闭（SYS-02）— 已处理
- 弹出终端窗口：PopupWindowService 打开 `#/terminal-popup` 并 emit config；会话窗口角色与崩溃矩阵仍缺（FND-04）— 已处理

### 数据与同步
- 非 AI 持久化写入经 hostStorageAdapter 按域镜像（含 SFTP 书签/传输中心、session restore、port forwarding）；AI 相关存储仍直写 localStorage，硬阻塞于 P6-05；读取仍同步（SYNC-01）— 已处理
- 云同步：SyncService 暴露 Merge/Fingerprint；OAuth/S3/WebDAV 提供方仍未接（SYNC-02）— 已处理

### 插件
- Install/SetEnabled 元数据门面已接（PLUG-01）— 已处理
- WASM Instantiate 接到 wazero runtime，无 host import / WASI（PLUG-02）— 已处理
- native 进程运行时仍未接到 Wails 壳（PLUG-03）— pending

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
   `scripts/sign-wails-probe.mjs` 只记录 unsigned 原因，不伪造签名
4. **自动更新**：无签名 feed、无 N-1→N 活体演练（REL-02 probe）— pending
5. **Electron 性能基线**：CI 上 3 个 best-effort 基线 job 抖动失败
   （不阻塞 `test` workflow）— pending
6. **干净机冒烟**：P8-01 Gate 所需的 signed clean-machine 矩阵 — pending

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

---

## 五、建议优先级（下一步可执行）

1. SSH MFA / 跳板 UI 回调 — 已处理（C 级；活体服务器仍缺）
2. SFTP 高级路径接线 — 已处理（下载/上传/本地解压；传输中心 UI 仍缺）
3. App Lock 密码启用/解锁 — 已处理（生物识别仍缺）
4. CI 化三平台冒烟扩充 — 已处理（契约测试接入 migration-evidence；活体窗口仍缺）
5. 签名与安装包试点 — 已处理（诚实 unsigned probe；不伪造 signtool 成功）
6. 以上每项落地后回写矩阵/台账；凑齐 A 级证据后逐行升 `verified`，
   最后记 P6-05 gate。

---

## 六、已知的非阻塞瑕疵

- `npx tsc --noEmit` 仓库全域存在历史遗留类型错误（textZip、plugin-cli、
  port-forwarding rule 类型等），不影响 `wails:build`（Vite 不做全量 tsc）
- 输出预缓冲在首个 credit 前无上限（依赖写入速率）
- 断连-重连窗口内的 publish 可能落入垂死队列（重连走新 generation 规避）
- `.test.ts` 位于仓库根目录会被 gitignore（测试需放在已纳管目录）
