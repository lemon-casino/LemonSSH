# Changelog

## [Unreleased]

## [0.0.4] - 2026-10-06

### 修复
- 修复品牌迁移误改 Mosh/ET 历史发布地址导致三平台 CI 下载辅助程序均返回 HTTP 404 的问题
- 恢复辅助程序真实仓库、标签、许可证地址和构建证明来源，保持原有资产 ID、二进制版本和 SHA-256 不变
- 构建证明按历史来源严格匹配，不再通过品牌名称替换放行；新增锁文件 URL、平台清单一致性和错误来源拒绝测试
- 修复下载在收到 HTTP 200 后传输中断、以及重定向后网络错误未按预期重试的问题；404 和摘要不符仍直接失败

### 发布说明
- `v0.0.3` 标签保留，但其三平台发布任务失败，未生成 GitHub Release；本版本包含其全部迁移补全改动

## [0.0.3] - 2026-10-06

### 功能
- 补齐 Electron 到 Wails 迁移后的 CodeBuddy elicitation 交互链路、Skills/MCP 工具模式差异和聊天会话隔离
- 恢复 JavaScript 自动化脚本运行时，支持控制流、异步调用、屏幕快照、权限模式和取消
- 对齐复制会话窗口的终端生命周期和托盘面板定位行为
- 删除未接线的重复插件生命周期管理器，并同步 Wails 绑定与迁移行为文档

### 验证
- 前端完整测试：7,457 通过，41 跳过
- Go 全量测试、竞态测试和插件契约检查通过
- Windows Wails 发行包、便携 ZIP 和桌面脚本冒烟验证通过
- NSIS 因构建机未安装 `makensis` 诚实跳过；macOS/Linux 安装器需在对应宿主验证

## [Unreleased] - 2026-09-24

### 功能
- Wails 壳接入应用内更新体系：设置 > 系统 的"当前版本"显示真实版本号（Go `UpdateService` 的 `Version` 经 `getAppInfo` 提供），启动自动检查、手动"检查更新"、下载进度条与"立即安装"全部生效
- 更新通道基于 GitHub Releases：按平台匹配 `LemonSSH-{版本}-{goos}-{goarch}` 产物（scripts/package-wails.mjs 的发布形态），下载校验 `checksums.txt` 的 sha256；发布 `release-manifest.json` 且构建注入 ed25519 公钥时额外执行清单验签（internal/platform/updater）
- 安装采用自替换：将新二进制原子换入运行路径后延时重启应用；`update:*` 事件广播到所有窗口（主窗口与设置窗口同步显示进度）
- 自动更新开关持久化到 profile 存储；开启时发现更新自动开始下载。版本号为 0 的开发构建（0.0.0-*）自动禁用应用内更新并降级为打开 Releases 页

### 渠道对账
- 本通道的发布物是单文件可执行产物；下文 Electron 时代宣称的 Windows NSIS / macOS dmg / Linux deb/rpm/pacman 原生安装器自动更新不适用于 Wails 壳。未发布对应产物（或 `checksums.txt` 校验失败）时，UI 如实降级为"手动下载"入口，不再伪造进度

### 兼容性
- 随品牌迁移，远端粘贴图片上传目录更名为 `.lemonssh-paste-images/`：新会话只写入新目录；历史版本在远端主机上创建的同用途旧目录不再读取、也永不删除。远端 shell 历史中旧版本注入的 osc7 历史清理标记同样为预期残留：仅旧历史行残留展示，新会话的标记照常自清理，不构成数据丢失

## [Unreleased] - 2026-03-11（已归档：Electron 时代，未随迁移发布）

> **归档说明**：本段为 Electron 壳时代的未发布条目，未随 Electron→Wails 迁移发布。
> 所述 Windows NSIS / macOS dmg / Linux deb/rpm/pacman 原生安装器及 electron-updater
> 自动更新形态已随 Electron 壳废弃（Wails 壳的发布物是单文件可执行产物，见上方
> 2026-09-24 段）。其中"粘贴时自动上传剪贴板图片"已随 commit `83e0e987`
> 在 Wails 壳落地，其余更新器条目不适用于当前壳。以下内容按历史原样保留。

### 功能
- Linux deb/rpm/pacman 安装包启用应用内自动更新；未标记的开发包和 Snap 继续提供手动下载入口
- 终端新增"粘贴时自动上传剪贴板图片"选项：剪贴板含图片时，在远程会话中粘贴（快捷键/右键/中键）自动通过 SFTP 上传图片到远端 `.lemonssh-paste-images/` 目录并输入远端路径，否则保持原有文本粘贴行为（设置 > 终端 > 行为，默认关闭）
- 修复自动更新 IPC 事件仅发送到单个窗口的问题，改为广播所有窗口（主窗口 + 设置窗口均可收到）
- 统一手动检查更新与自动更新的状态机，消除三套并行状态
- 手动"检查更新"通过 GitHub API 检测版本，发现更新后异步触发 electron-updater 下载
- 设置窗口中点击"检查更新"后，下载进度可实时反映在 UI 中
- 应用启动后 5 秒自动触发 `electron-updater` 检查更新，无需用户手动点击
- 发现新版本后自动开始下载（`autoDownload=true`）
- 下载完成后弹出持久 toast 通知，用户点击"立即重启"即可安装
- 下载失败时弹出错误 toast，提供"打开 Releases"降级入口
- Settings > System 进度条实时展示自动下载进度，由 `useUpdateCheck` 统一驱动
- Linux Snap、未标记的开发包等不支持 electron-updater 的平台自动跳过，保持原有 GitHub API 通知行为

### 设计原理
- `broadcastToAllWindows` 替换 `getSenderWindow` 单点发送，保证所有窗口都能收到 IPC 事件
- `manualCheckStatus` 字段追踪手动检查 UI 状态（idle/checking/available/up-to-date/error），与 `autoDownloadStatus` 在 UI 层按优先级渲染
- `SettingsSystemTab` 不再持有本地 update state，单向接收 `useUpdateCheck` 统一数据
- 将原有两套独立系统（GitHub API 通知 + electron-updater 手动下载）合并为统一状态机：`useUpdateCheck` 作为唯一事实来源，同时驱动 `App.tsx` toast 和 `SettingsSystemTab` 进度条
- 全局持久化 IPC 监听器在 `autoUpdateBridge.init()` 时一次性注册，避免每次手动下载请求重复注册/清理监听器
- `autoInstallOnAppQuit=false`，不做静默安装，由用户主动触发重启

### 接口变更（SettingsSystemTabProps）
- 移除：`autoDownloadStatus`、`downloadPercent`
- 新增：`updateState`（完整 UpdateState）、`checkNow`、`installUpdate`、`openReleasePage`

### 注意事项
- `checkNow` 语义：使用 GitHub API（`performCheck`）检测是否有新版本，若发现更新且 electron-updater 尚未开始下载，则异步触发 `bridge.checkForUpdate()` 启动自动下载流程
- 此功能仅对打包后的应用（Windows NSIS、macOS dmg/zip、Linux AppImage/deb/rpm/pacman）生效，dev 模式需配合 `forceDevUpdateConfig=true` + `dev-app-update.yml` 测试（见 `.gitignore`）
- `hasUpdate` 旧 toast 在 `autoDownloadStatus !== 'idle'` 时自动抑制，避免与新 toast 重复
