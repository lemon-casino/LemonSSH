# 验收前可编码功能清单（2026-09-12）

> **面向 AI 代理的工作者：** 每个切片按既有节奏执行：测试 → 实现 →
> `check:migration-docs` → scoped commit → push。状态权威仍是
> [capability-matrix.md](capability-matrix.md) 与
> [migration-ledger.md](migration-ledger.md)；
> 本文只跟踪"验收阶段之前可以写代码完成"的功能，不重复记账。

**目标：** 把所有不依赖外部条件（活体服务器、macOS/Linux 真机、签名证书、
真实硬件）的功能代码写完，使剩余工作只剩验收/证据收集。

**来源：** [remaining-work.md](remaining-work.md)（2026-09-11 快照，
台账头 WV3-L112）+ `2026-09-12-layout-modes.md` P4。

## 范围边界（写不了代码、只能等验收条件，不在本文）

- 三平台活体证据、真实 MFA/跳板/SFTP 服务器矩阵、串口/Telnet 硬件
- 签名安装包、生产 feed 发布、干净机冒烟（P8-01）
- P4/P5 真机回归（测试代码可写并随切片落地，跑真机回归属于验收）

## 全局约束（沿用 2026-09-10 计划）

- README 只写产品，不写迁移叙事。
- 不伪造 `verified` / `NONAI-COMPLETE` / Phase 7；证据等级诚实（本机 C 级）。
- 不新造平行路径：Go facade（`cmd/netcatty/*Service.go`，无业务逻辑）→ 重新生成
  绑定（`wails3@v3.0.0-beta.12 generate bindings -d infrastructure/runtime/wails/bindings ./cmd/netcatty`）→
  `infrastructure/runtime/wails/wailsRuntimeClient.ts` 唯一 bindings 导入边界。
- 每个切片落地后回写矩阵/台账（`not-started → probe → implemented`，不跳级）。
- 存储键集中在 `infrastructure/config/storageKeys.ts`；持久化只经
  `hostStorageAdapter` / `localStorageAdapter`，组件内禁止直接 `localStorage`。
- 同仓库内按切片 scoped commit。

## 批次与切片

### Batch A — 关键路径（卡 P6-05 的核心）

#### 任务 A1：Vault canonical cutover（SYNC-01）— 完成（WV3-L113）

- [x] 非 AI 域（vault/settings/sessions/SFTP 书签/传输中心）读取从同步
  localStorage 切到 Go profile store：异步 hydrate → 内存层同步读 →
  hydrateReady 门槛（Wails boot 已 await hydrateReady，见 SYNC-01 矩阵行）
- [x] differential 对比工具：切换期间 localStorage vs Go store 数据一致性校验
  （`canonicalHydration.ts` diffCanonicalSources + 冲突 heal 计数日志）
- [x] AI 相关存储保持 localStorage 直写（isAIManagedStorageKey 双向排除，
  硬阻塞于 P6-05，禁止顺手迁移）
- [x] 迁移/回退路径：旧 localStorage 数据在首次 hydrate 时 promote 进 Go
  store；hydrate fail-open（store 损坏时本地缓存兜底，不白屏）
- [x] 测试：canonicalHydration.test.ts（hydrate/promote/heal/skip 状态机 +
  三阶段 boot 仿真 + lossless 等价）
- [x] ledger：SYNC-01 probe → implemented（WV3-L113）

### Batch B — layout-modes P4 交互（有现成计划）

来源：`docs/superpowers/plans/2026-09-12-layout-modes.md`（本地计划，不随仓库跟踪）。以下均未开工。

- [ ] B1 右键菜单：会话树/分组项复用 TopTabs 现有回调（关闭/重命名/复制），不复制实现
- [ ] B2 拖拽重排 + 拖入工作区（两种布局下行为一致）
- [ ] B3 `Ctrl+1..9` 快捷切换 + 序号徽标
- [ ] B4 切换会话（快捷键/托盘跳转/AI 打开）自动展开所在分组分支并滚动到可见
- [ ] B5 会话树宽度调整
- [ ] B6 菜单栏溢出收纳（960px 最小宽度下收进「更多」或 ☰）
- [ ] B7 菜单栏空白区窗口拖拽/双击最大化；菜单项与按钮区域不可拖动
- [ ] B8 补 §5.4 缺失测试：sessionGroupTree 边界用例、跨窗口双通道断言、
  i18n 覆盖、展开状态跨重启保留
- [ ] B9 ledger/matrix：layout modes 属壳层功能，确认是否需要台账记录
  （若计入 FND-04 或独立行，按 checker 规则走）

### Batch C — 终端协议与传输

- [ ] C1 Mosh/ET roaming reconnect（TERM-03.3）：MOSH KEY 重用 + 漫游重连；
  活体验证可复用 Debian 13 (192.168.0.6) 的 mosh-server 1.4.0 路径
- [ ] C2 ZMODEM 原始 lrzsz 对端兼容（TERM-03.4）：长度前缀引擎已接，
  补与真 lrzsz 的握手/中止容错
- [ ] C3 sudo SFTP（SFTP-01）：Go 侧经 `sudo` 启动 sftp 子系统，失败显式报错
- [ ] C4 传输中心 UI（SFTP-02）：调度器 pause/resume/cancel 已接
  TransferService，壳内补进度/暂停/恢复/取消界面
- [ ] C5 Windows/macOS helper 打包脚本（TERM-03.3）：mosh/et 二进制捆绑进
  `scripts/package-wails.mjs` + installer 清单（写脚本不需真机；hash/arch 校验
  按 TERM-03.3 矩阵行要求）

### Batch D — 系统壳

- [ ] D1 Windows Hello（SYS-04）：KeyCredentialManager 路径接到
  UnlockWithBiometrics（本机 Windows 可验证）；Touch ID（LocalAuthentication
  cgo）代码可写、验证靠后。替代密钥封存沿用 `internal/platform/credentials`/applock 现有模式
- [ ] D2 macOS/Linux 协议注册（SYS-03）：macOS Info.plist CFBundleURLTypes /
  LSRegisterURL；Linux .desktop + xdg-mime。运行时注册逻辑与打包清单先写，
  三平台投递验证属验收
- [ ] D3 原生全局快捷键（SYS-02）：Wails alpha.63 无 GlobalShortcut，绕法为
  平台 API 直调 —— Windows `RegisterHotKey`（本机可验证）、X11、macOS Carbon。
  失败仍 fail-closed，不破坏现有 ShortcutService 注册表语义
- [ ] D4 弹窗会话窗口角色（FND-04）：session/popup 角色栅栏与崩溃清理路径的
  代码部分（多显示器/崩溃矩阵属验收）

### Batch E — 插件平台

- [ ] E1 声明式 UI 贡献模型（PLUG-02）：host 渲染 UI schema + 权限 broker，
  不得依赖 Agent catalog
- [ ] E2 PLUG-01 收尾：atomic on-disk 恢复（发布中断重放）、v1 包拒绝 UX、
  codegen drift CI 校验
- [ ] E3 跑 `npm run test:plugin-runtime` 与 `test:plugin-runtime:electron`；
  涉及打包资源时加 `npm run pack:dir`

### Batch F — 稳定性修复（remaining-work §六）

- [ ] F1 首 credit 前输出预缓冲无上限 → 背压/丢弃策略（TERM-01 数据面）
- [ ] F2 断连重连窗口内 publish 落垂死队列 → 防护（generation 规避已有，补显式拒绝）
- [ ] F3 tsc 历史遗留类型错误清理：textZip、plugin-cli、port-forwarding rule
- [ ] F4 根目录 `.test.ts` 被 gitignore → 目录约定/`.gitattributes` 防呆

## 建议顺序

A1（gate 关键路径）→ B（有现成计划、用户已验收 P1–P3b）→ C4（传输中心 UI，
用户可见价值最高）→ C1/C2/C3 → D1（Windows 本机可验证）→ D2/D3/D4 → E → F。

## 完成定义（本文档范围）

- 上列切片全部 `implemented` 并回写矩阵/台账；
- remaining-work.md §二不再有"未接/仍缺"的代码缺口（只剩活体/平台/签名证据项）；
- P6-05 之前不需要再写功能代码，进入证据收集阶段。
