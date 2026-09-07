# Wails v3 发布目标矩阵

状态：完成。required floor 由 `WV3-011`、`WV3-012`、`WV3-013` 于 2026-09-08 冻结。

记录日期：2026-08-23；决策关闭日期：2026-09-08

## 1. 目的与权威顺序

本文件定义 Wails 成为默认发行物前必须覆盖的目标集合。它区分：

- `required`：缺少 A 级证据就禁止三平台切换；
- `best-effort`：产物可以提供，但不阻止切换；
- `unsupported`：当前迁移明确不承诺；
- `decision-required`：现有权威不足或支持边界冲突，必须先批准 decision。

当前 Electron 发行事实按以下顺序判断：

1. `.github/workflows/build.yml` 实际 release jobs；
2. `electron-builder.config.cjs` 和 package scripts；
3. `flake.nix`/`nix/package.nix`；
4. README 用户声明。

当 README 与 release workflow 冲突时，workflow 是当前可验证事实，README 漂移
单独记录，不能用于扩大 Wails required matrix。

## 2. Wails 工具链基线

截至记录日：

- Wails v3 上游状态：Beta；
- 最新公开 release：`v3.0.0-beta.12`，发布于 2026-08-21；
- 上游安装路径：`github.com/wailsapp/wails/v3/cmd/wails3`；
- `v3.0.0-beta.12` 的 `v3/go.mod` 要求 Go `1.25.0`；
- P1-02 才能固定 Netcatty 使用的精确 Wails/Go 版本；P0-01A 不把 latest
  解析为可重复构建版本；
- 上游 `v3.0.0-beta.8` release notes 声明 GTK 4.14+ Linux support baseline；
- 上游 v3 提供 Go/JS bidirectional streams，但其吞吐、credit、rebind 和
  WebView 行为仍须通过 P0-03，不能直接视为 terminal data plane 已解决。

上游证据：

- `https://github.com/wailsapp/wails/releases/tag/v3.0.0-beta.12`
- `https://raw.githubusercontent.com/wailsapp/wails/v3.0.0-beta.12/v3/go.mod`
- `https://github.com/wailsapp/wails/releases/tag/v3.0.0-beta.8`

## 3. 当前 Electron 发行事实

| Platform | Architecture | Current CI/build environment | Current artifacts | Current status |
| --- | --- | --- | --- | --- |
| Windows | x64 | `windows-latest`, MSVC x64, `pack:win-x64` | NSIS `.exe`, portable `.exe`, ZIP, update metadata | released/required today |
| Windows | arm64 | 无独立 official job；Mosh/ET 与 native rebuild 未就绪 | 配置可手工请求，但官方 workflow 明确禁用 | unsupported today |
| macOS | x64 | `macos-latest`, `pack:mac` universal targets | DMG、ZIP、signed/notarized when secrets available | released/required today |
| macOS | arm64 | 与 x64 同一 macOS build job | DMG、ZIP、signed/notarized when secrets available | released/required today |
| Linux | x64 | AlmaLinux 8, glibc 2.28 floor, gcc-toolset-13 | AppImage、deb、rpm、pacman、update metadata | released/required today |
| Linux | arm64 | Debian Bullseye, glibc 2.31 build | AppImage、deb、rpm、pacman、update metadata | released/required today |
| Nix/NixOS | x86_64-linux | wrap official x64 AppImage | flake package/app | distribution wrapper |
| Nix/NixOS | aarch64-linux | wrap official arm64 AppImage | flake package/app | distribution wrapper |

已确认文档漂移：`README.md`/本地化 README 声明 Windows x64/arm64，但 official
workflow 在 `.github/workflows/build.yml` 明确 Windows x64-only。P0-01A 不修改该
公共声明；应由独立文档修复任务决定是纠正文案还是恢复真实 ARM64 release。

## 4. Wails 默认切换 required matrix

下表只冻结无需新增产品决定即可从当前发行事实继承的维度。最低 OS/WebView 和
Linux ABI/GTK 冲突仍为显式 decision gates。

| ID | Platform/arch | Required artifact classes | Required runtime paths | OS/WebView floor | Classification |
| --- | --- | --- | --- | --- | --- |
| RT-WIN-X64 | Windows x64 | installer、portable、ZIP、signed update metadata | WebView2、ConPTY、DPAPI/Hello、tray、deep links、Explorer context menu、Mosh/ET | Windows 10 22H2 (build 19045) x64 + WebView2 Evergreen，`WV3-011` | required |
| RT-MAC-X64 | macOS x64 | DMG、ZIP、signed/notarized update metadata | WKWebView、Unix PTY、Keychain/Touch ID、tray/dock、URL/file events、Mosh/ET | macOS 12 Monterey+，`WV3-012` | required |
| RT-MAC-ARM64 | macOS arm64 | DMG、ZIP、signed/notarized update metadata | same as RT-MAC-X64 plus native arm64 helpers | macOS 12 Monterey+，`WV3-012` | required |
| RT-LINUX-X64 | Linux x64 | AppImage、deb、rpm、pacman、update metadata | WebKitGTK/GTK、Unix PTY、Secret Service、tray、desktop handlers、Mosh/ET | GTK 4.14+ / WebKitGTK（跟随 Wails v3 基线），`WV3-013` | required |
| RT-LINUX-ARM64 | Linux arm64 | AppImage、deb、rpm、pacman、update metadata | same as RT-LINUX-X64 | GTK 4.14+ / WebKitGTK（跟随 Wails v3 基线），`WV3-013` | required |

三平台同时切换的含义是：以上 5 个 required architecture rows 都获得明确 OS/
WebView floor decision，并对每个 required artifact class 通过 Gate 13/14。只在
Windows、只在一种 Linux package 或只在 Apple Silicon 通过都不能切换默认发行物。

Phase 6 可以为以上 rows 构建 package/updater/upgrade-bootstrap qualification
artifacts，以验证安装机制和 focused journeys。它们尚未包含 Phase 7 production AI
owners，不是 release candidates，也不得用于切换默认发行物。只有 P8-01 从最终
non-AI + AI source set 构建的签名 RC 可以完成完整 Gate 1-14 qualification。

## 5. Required Linux certification profiles

依据 `WV3-013`（GTK 4.14+ / WebKitGTK floor），每个 architecture 至少要为以下
profile 冻结证据：

1. 最低受支持 glibc/GTK/WebKitGTK 组合（GTK 4.14+ 基线，候选发行版为
   Ubuntu 22.04+ / Debian 12+ / Fedora 38+ 同代）；
2. 一个当前 Ubuntu/Debian 桌面；
3. 一个 RPM 系桌面；
4. X11 session；
5. Wayland session；
6. Secret Service 可用且 unlocked；
7. Secret Service 缺失/锁定的 fail-closed negative profile；
8. AppImage、deb、rpm、pacman 安装/升级/卸载；
9. Nix x86_64/aarch64 AppImage wrapper smoke。

旧 glibc 2.28/2.31 build floor 只证明了 Electron native modules 的历史事实，
不构成 Wails required target；RHEL 8/UOS/Deepin 旧版按 `WV3-013` 为 unsupported。

## 6. Best-effort 与 Unsupported

| Target | Classification | Reason / promotion requirement |
| --- | --- | --- |
| Windows ARM64 | unsupported | `WV3-011` 于 2026-09-08 确认维持 unsupported；提升为 required 需 Mosh/ET、PTY、Hello、installer、updater 和 clean-machine ARM64 evidence 及新 decision |
| Windows 10 builds older than 22H2 | unsupported | `WV3-011` required floor 为 22H2 (build 19045) |
| Windows 32-bit | unsupported | 当前产品/CI 无发布路径 |
| macOS universal single binary | best-effort | 当前是每 arch artifacts；是否合并 universal 不是用户行为要求 |
| macOS older than 12 | unsupported | `WV3-012` required floor 为 macOS 12 Monterey |
| Linux Snap/Flatpak | unsupported | 当前 official release pipeline 不产出；新增格式需 distribution/updater decision |
| Linux distros without GTK 4.14+/WebKitGTK（含 RHEL 8、UOS、Deepin 旧版） | unsupported | `WV3-013` 退休旧 glibc 2.28 兼容目标，跟随 Wails v3 基线 |
| Linux without secure Secret Service | unsupported for lossless profile migration | `WV3-007` fail-closed boundary |
| FreeBSD/其他 Unix | unsupported | 当前产品声明与 release pipeline 不覆盖 |
| Web browser/server-only build | unsupported as desktop release | 不满足 PTY/OS integration product scope |

`best-effort` target 失败不能自动阻断切换，但不得被用于代替 required row evidence。

## 7. Artifact 与功能验收维度

每个 required row 都必须覆盖：

- clean install、first launch、upgrade、rollback、uninstall；
- profile locate、writer lease、lossless migration、credential re-seal；
- local PTY、SSH、SFTP、transfer、forwarding、Mosh/ET；
- xterm WebGL/DOM fallback、IME、clipboard、Monaco；
- main/settings/tray/session/popup windows；
- deep links/file associations/context menu where applicable；
- App Lock 和平台 biometric；
- Catty、retained external Agents、MCP/CLI；
- plugin WASM/native process；
- cloud/convergent sync；
- signed update 与 package integrity；
- runtime Electron/Node-free SBOM/process-tree proof。

Pre-cutover proof 对应 `REL-03.1`，只检查最终签名 Wails artifact 的 runtime purity。
Electron release/rollback carrier 的 repository deletion 对应 `REL-03.2`，在 cutover
后的观察窗口关闭后完成，不能反向成为本矩阵的 pre-cutover circular requirement。

## 8. Matrix 变更规则

1. required target 的删除、降级或最低版本提高必须新增 `decisions.md` decision。
2. 新 architecture/package format 先进入 best-effort，不自动成为 release blocker。
3. required 证据必须引用 exact OS image/version、architecture、WebView/runtime、
   package format 和 CI/manual evidence。
4. `*-latest` 只能用于滚动 smoke，不能定义 minimum support floor。
5. Wails/Go 升级后重新检查平台 floor、streams、window lifecycle 和 packaging。
6. README、CI、packaging 和本矩阵冲突时，必须记录 drift 并修复，不得选择性引用。

## 9. 未决用户决策（已于 2026-09-08 关闭）

原 5 项决策已全部由产品所有者批准并记录为 accepted decisions：

1. Windows：required floor 固定为 Windows 10 22H2 (build 19045) x64 +
   WebView2 Evergreen；更旧 Windows 10 builds unsupported → `WV3-011`；
2. macOS：x64/arm64 最低 supported 版本为 macOS 12 Monterey → `WV3-012`；
3. Linux：跟随 Wails v3 基线（GTK 4.14+/WebKitGTK），RHEL 8/UOS/Deepin 旧版
   降为 unsupported → `WV3-013`；
4. Linux package formats：AppImage/deb/rpm/pacman 全部保持 required（继承
   `WV3-008`，无需新 decision）；
5. Windows ARM64：确认保持 unsupported（`WV3-011` 重申，`WV3-008` exclusion
   不变）。

已知的 README Windows ARM64 文档漂移（`README.md` 声称支持 ARM64）由独立的
文档修复任务处理，不属于本矩阵范围。

停止状态解除：`pause-for-user` 已于 2026-09-08 关闭；Gate 13 的 A 级目标集合
已闭合为上表 5 个 required rows。
