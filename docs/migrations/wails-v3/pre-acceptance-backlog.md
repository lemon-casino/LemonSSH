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

#### 任务 A1：Vault canonical cutover（SYNC-01）— 代码完成（WV3-L115）

WV3-L114 保留对 L113 的纠正；L115 在真实 adapter/boot 验证后重新推进 implemented。

- [x] Go hydrate → 内存同步读取；React 首次渲染等待 hydrateReady
- [x] differential 比较实际值，冲突以 Go 为准
- [x] AI 存储继续 localStorage，双向排除迁移
- [x] 一次性 legacy 导入、删除防复活、跨窗口刷新及显式失败状态
- [x] 20 个核心测试、208 个同步回归；main 独立延迟启动/adapter/真实 Go 检查
- [x] SYNC-01 probe → implemented；多机恢复验收仍待补齐

### Batch B — layout-modes P4 交互（有现成计划）

来源：`docs/superpowers/plans/2026-09-12-layout-modes.md`（本地计划，不随仓库跟踪）。代码切片已实现并通过 98 个定向测试；GUI 验收仍 pending（WV3-L116）。

- [x] B1 右键菜单：会话树/分组项复用 TopTabs 现有回调（关闭/重命名/复制），不复制实现
- [x] B2 拖拽重排 + 拖入工作区（两种布局下行为一致）
- [x] B3 `Ctrl+1..9` 快捷切换 + 序号徽标
- [x] B4 切换会话（快捷键/托盘跳转/AI 打开）自动展开所在分组分支并滚动到可见
- [x] B5 会话树宽度调整
- [x] B6 菜单栏溢出收纳（960px 最小宽度下收进「更多」或 ☰）
- [x] B7 菜单栏空白区窗口拖拽/双击最大化；菜单项与按钮区域不可拖动
- [x] B8 补 §5.4 缺失测试：sessionGroupTree 边界用例、跨窗口双通道断言、
  i18n 覆盖、展开状态跨重启保留
- [x] B9 ledger/matrix：layout modes 属壳层功能，确认是否需要台账记录
  （若计入 FND-04 或独立行，按 checker 规则走）

### Batch C — 终端协议与传输

- [x] C1 Mosh/ET roaming reconnect（TERM-03.3）：MOSH KEY 重用 + 漫游重连；
  保留活进程/bootstrap，不含进程死亡恢复；ET inline auth/proxy/jump
  不支持（超出原始 roaming 范围），网络漫游活体验收仍待补。
- [x] C2 原始 ZMODEM 代码（TERM-03.4）：标准 header/subpacket、CRC16/32、
  双向握手、ACK 进度及 CAN 中止；独立 zmodem.js 双向传输通过。
  **真实 lrzsz 对端验收未完成**；CRC 错误中止，不宣称自动重传兼容（WV3-L117）。
- [x] C3 sudo SFTP（SFTP-01）：Go 侧经 `sudo` 启动 sftp 子系统，失败显式报错
- [x] C4 传输中心 UI（SFTP-02）：调度器 pause/resume/cancel 已接
  StartCompressed ZIP staging 与上传使用同一 scheduler ID，压缩/上传两阶段
  pause/resume/cancel、backend epoch、List reload observation 已接；
  Go 定向测试和 main TS 33/33 通过；最终生成 20 services/142 methods/50 models
  无 warnings，Wails build 与 Go race PASS；冻结后集成 Node suite 102/102 PASS（L121）。
  压缩阶段 transfer bytes 为 0，上传阶段按实际 ZIP 字节计量（raw1000 → ZIP10 测试）；
  仅上传 ZIP、不隐式解压，沿用 UploadCompressedFolder 语义。
  最终 metric 修正后 npm run wails:build 亦 PASS / exit 0。
  共享 managed temp 已接；ClearTemp 跳过 staged prefixes，孤儿 stage 可能残留（L122）。
- [x] C5 Windows/macOS helper 供给/打包脚本（TERM-03.3）：
  `scripts/package-wails.mjs` 实现外部可信 digest + arch 校验、复制及清单生成。
  实际发布 helper 未交付：本地仅有无 manifest pin 的 win32-x64 mosh-client；
  其余 Windows/macOS mosh/et binary 和所有对应 pin 均缺，不能算捆绑完成。

### Batch D — 系统壳

- [x] D1 Windows Hello（SYS-04）：KeyCredentialManager 路径接到
  UnlockWithBiometrics（本机实测 IsSupported=false，未验证成功认证）；Touch ID（LocalAuthentication
  cgo）代码可写、验证靠后。替代密钥封存沿用 `internal/platform/credentials`/applock 现有模式
- [x] D2 macOS/Linux 协议注册（SYS-03）：macOS Info.plist CFBundleURLTypes /
  LSRegisterURL；Linux .desktop + xdg-mime。运行时注册逻辑与打包清单先写，
  三平台投递验证属验收
- [x] D3 原生全局快捷键（SYS-02）：Wails alpha.63 无 GlobalShortcut，绕法为
  平台 API 直调 —— Windows `RegisterHotKey`（本机可验证）、X11、macOS Carbon。
  失败仍 fail-closed，不破坏现有 ShortcutService 注册表语义
- [x] D4 弹窗会话窗口角色（FND-04）：session/popup 角色栅栏与崩溃清理路径的
  代码部分（多显示器/崩溃矩阵属验收）

### Batch E — 插件平台

- [x] E1 声明式 UI 贡献模型（PLUG-02）：host 渲染 UI schema + 权限 broker，
  不得依赖 Agent catalog
- [x] E2 PLUG-01 收尾：atomic on-disk 恢复（发布中断重放）、v1 包拒绝 UX、
  codegen drift CI 校验
- [ ] E3 遗留 Electron 回归仅部分完成：真实 smoke PASS；常规 suite
  仍挂起，两个既存 NUL SQLite sidecar 失败及 open handle 未清除。
  按 Wails 目标停止扩大遗留排查，不作为 Wails 产品验收。
  `npm run pack:dir` 未运行（遗留 Electron 目标），不列作本轮 Wails 构建要求。
- E1/E2：7 个 frontend tests、全部 Go/plugin、targeted cmd、scoped eslint、
  check:plugin-contract 通过；显式 broker、encrypted secrets、durable recovery 已接。
- main 集成 101/101 Node tests；`npm run wails:build` 与 `npm run build`
  PASS / exit 0；L119 当时 bindings 为 20 services / 138 methods；L121 最终为
  20 services / 142 methods / 50 models，无 warnings，metric 修正后 Wails build PASS。

### Batch F — 稳定性修复（remaining-work §六）

- [x] F1 首 credit 前输出预缓冲无上限 → 背压/丢弃策略（TERM-01 数据面）
- [x] F2 断连重连窗口内 publish 落垂死队列 → 防护（generation 规避已有，补显式拒绝）
- [x] F3 textZip、plugin-cli、port-forwarding rule 范围内 13 个 tsc 错误清零；
  全仓 tsc 仍失败，不能据此宣称全量类型检查通过
- [x] F4 根目录 `.test.ts`/`.test.tsx` 精确放行，真实 git check-ignore 回归防呆

代码勾选仅表示实现及所列本机测试，不表示原始任务中的活体或 GUI 验收通过。
C5 只交付校验/供给脚本，不表示 Windows/macOS helper 二进制已捆绑；
D1 本机 Hello IsSupported=false，成功认证与 Touch ID 真机仍待验证；
D2/D3 的 macOS/Linux crossbuild 不替代原生运行。E3 保持遗留 Electron 部分证据，不阻塞 Wails 产品构建结论。

## 建议顺序

A1（gate 关键路径）→ B（有现成计划、用户已验收 P1–P3b）→ C4（传输中心 UI，
用户可见价值最高）→ C1/C2/C3 → D1（Windows 本机可验证）→ D2/D3/D4 → E → F。

## 完成定义（本文档范围，尚未全部满足）

不能宣称全部 capability implemented：C4 代码、最终 Wails build/Go race 完成，冻结后 Node suite 102/102 PASS，metric 修正后 Wails build 亦 PASS；
ClearTemp 孤儿 stage 清理仍有限制；ET 高级认证/代理/跳板及
进程死亡恢复有上述范围限制。真实 lrzsz、GUI、原生多平台与签名证据仍缺。

- 所需代码切片完成并按合法状态转换回写矩阵/台账；复选框不等于 capability `implemented`。
  当前 B/D/SFTP 相关行仍为 `probe`；PLUG-01 已 `implemented`，PLUG-02
  因子行拆分门槛仍为 `probe`，不得将本轮写为全部 capability implemented。
- remaining-work.md §二不再有"未接/仍缺"的代码缺口（只剩活体/平台/签名证据项）；
- P6-05 之前不需要再写功能代码，进入证据收集阶段。
