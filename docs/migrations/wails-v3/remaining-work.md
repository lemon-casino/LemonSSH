# LemonSSH 剩余工作清单（2026-09-10 快照）

本文档回答一个问题：**现在还有什么没有做完**。状态权威仍是
[capability-matrix.md](capability-matrix.md) 与 [migration-ledger.md](migration-ledger.md)；
本文是导航快照，与矩阵冲突时以矩阵为准。

当前台账头：`WV3-L067`。矩阵 34 行：implemented 6 / probe 17 / not-started 11 /
**verified 0 / migrated 0**。

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
| 项 | 缺口 | 涉及行 |
| --- | --- | --- |
| SSH MFA / keyboard-interactive | Go 支持，UI 回调未接 | SSH-01 |
| SSH 跳板链 / proxy | Go 支持，绑定参数未透出（显式拒绝） | SSH-01 |
| Mosh / ET | supervised runner 已有， reconnect 协议与产品路径未接 | TERM-03.3 |
| ZMODEM 完整 rz/sz 会话 | CRC/安全边界已有，会话引擎未实现 | TERM-03.4 |
| 串口 YMODEM | 未接 | TERM-03.2 |
| Serial/Telnet 活体设备矩阵 | 无真实硬件证据 | TERM-03.1/2 |

### SFTP / 传输
- 高级浏览路径（压缩包提取、拖拽上传、传输中心 UI）未接 Wails 桥（SFTP-01/02）
- 断点续传 / 压缩上传（P3-06 scheduler）未接
- sudo SFTP、非 UTF-8 文件名矩阵未验证

### 系统能力
- App Lock 密码启用 / PBKDF2 verifier / 生物识别未接（SYS-04）
- deep link 的 OS 协议注册与冷启动投递未做（SYS-03）
- 全局快捷键原生注册未验证（SYS-02）
- 多窗口 / 弹出终端 / 会话窗口角色（FND-04 probe）

### 数据与同步
- Vault/settings 仍走过渡适配层，渲染层 canonical 切换未做（SYNC-01）
- 云同步（S3/WebDAV/Google/OneDrive/CRDT）完全未接（SYNC-02）

### 插件
- 安装/启用/权限 broker/WASM runtime/native 进程均未接（PLUG-01/02/03）

### AI（全部）
- P7-01~P7-06：capability catalog、MCP/CLI、providers、Catty runtime、
  外部 Agent、退役 CJS 路径——**硬阻塞于 P6-05 gate**（AI-01~04）

---

## 三、验收与证据缺口（无法本机伪造）

`verified` 需要证据等级 A。以下缺失导致矩阵 verified=0：

1. **三平台活体证据**：Windows 仅有本机 C 级；macOS/Linux 无窗口、拖拽、
   托盘、PTY、数据面配对基准
2. **真实服务器矩阵**：SSH MFA/跳板、真实 SFTP 服务器、编码/符号链接
3. **签名与安装包**：无代码签名、无 msi/pkg/AppImage/deb/rpm 打包
4. **自动更新**：无签名 feed、无 N-1→N 活体演练（REL-02 probe）
5. **Electron 性能基线**：CI 上 3 个 best-effort 基线 job 抖动失败
   （不阻塞 `test` workflow）
6. **干净机冒烟**：P8-01 Gate 所需的 signed clean-machine 矩阵

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

1. **SSH MFA / 跳板 UI 回调**（键盘交互挑战 → 渲染层弹窗）——解掉最常见连接场景
2. **SFTP 高级路径接线**——复用现有 ClientFS，纯前端桥工作
3. **App Lock 密码启用/解锁**——已有 PBKDF2 owner，缺 Wails 绑定与设置 UI
4. **CI 化三平台冒烟扩充**——把 Windows 已验证的单实例/无边框/设置窗口
   行为加入 migration-evidence workflow
5. **签名与安装包试点**（Windows 先行：signtool + ico 资源已就绪）
6. 以上每项落地后回写矩阵/台账；凑齐 A 级证据后逐行升 `verified`，
   最后记 P6-05 gate。

---

## 六、已知的非阻塞瑕疵

- `npx tsc --noEmit` 仓库全域存在历史遗留类型错误（textZip、plugin-cli、
  port-forwarding rule 类型等），不影响 `wails:build`（Vite 不做全量 tsc）
- 输出预缓冲在首个 credit 前无上限（依赖写入速率）
- 断连-重连窗口内的 publish 可能落入垂死队列（重连走新 generation 规避）
- `.test.ts` 位于仓库根目录会被 gitignore（测试需放在已纳管目录）
