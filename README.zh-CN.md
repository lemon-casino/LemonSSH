<p align="center">
  <img src="public/icon.png" alt="LemonSSH" width="128" height="128">
</p>

<h1 align="center">LemonSSH</h1>

<p align="center">
  <strong>🔥 AI 驱动的 SSH 客户端、SFTP 浏览器 & 终端管理器 🚀</strong>
</p>

<p align="center">
  一个美观且功能丰富的 SSH 工作空间，正在从 Electron 迁移到 Go + Wails v3 运行时。<br/>
  🔥 内置 AI Agent · 分屏终端 · Vault 多视图 · SFTP 工作流 · 自定义主题 —— 一应俱全。
</p>

<p align="center">
  <a href="#"><img alt="Platform" src="https://img.shields.io/badge/Platform-macOS%20%7C%20Windows%20%7C%20Linux-blue?style=for-the-badge"></a>
  &nbsp;
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/License-GPL--3.0-green?style=for-the-badge"></a>
  &nbsp;
  <a href="docs/migrations/wails-v3/README.md"><img alt="Migration" src="https://img.shields.io/badge/Migration-Wails%20v3%20%2B%20Go-informational?style=for-the-badge&logo=go"></a>
</p>

<p align="center">
  简体中文 · <a href="README.zh-TW.md">繁體中文</a> · <a href="README.ja-JP.md">日本語</a> · <a href="README.md">English</a>
</p>

---

## 迁移状态（Electron → Go + Wails v3）

本仓库是 Netcatty 项目的延续：在保留 React/TypeScript 前端、用户数据与功能集的前提下，
将运行时从 Electron/Node.js 迁移到 **Go + Wails v3**。

| 领域 | 状态 |
| --- | --- |
| 迁移治理（文档、台账、CI 证据工作流） | ✅ 就绪 |
| 壳中立前端端口 + Wails 骨架 + 基础契约 | ✅ 完成 |
| Go 事务化 profile 存储、写者租约、凭据提供方 | ✅ 核心完成 |
| 加密迁移包（导出/导入/回滚） | ✅ 核心完成 |
| 终端二进制数据面、本地 PTY、SSH 拨号/连接池、SFTP、传输、转发 | ✅ 核心完成 |
| 三平台活体证据、逐域持久化切换 | 🚧 收集中 |
| Telnet/Serial/Mosh/ET/ZMODEM、系统能力、插件 v2、同步、AI | ⏳ 排队中 |

权威状态：[capability-matrix.md](docs/migrations/wails-v3/capability-matrix.md) · 台账：[migration-ledger.md](docs/migrations/wails-v3/migration-ledger.md) · 计划：[implementation-plan.md](docs/migrations/wails-v3/implementation-plan.md)。

---

<a name="lemonssh-是什么"></a>
# LemonSSH 是什么

**LemonSSH** 是一款面向 macOS、Windows 和 Linux 的现代 SSH 客户端与终端管理器，
为需要高效管理多台远程服务器的开发者、系统管理员和 DevOps 工程师而设计。

- **LemonSSH 是** PuTTY、Termius、SecureCRT 与 macOS 终端的替代选择
- **LemonSSH 是** 带双栏文件浏览器的强大 SFTP 客户端
- **LemonSSH 是** 支持分屏、标签页与会话管理的终端工作空间
- **LemonSSH 支持** SSH、本地终端、Telnet、Mosh 与串口连接（可用时）
- **LemonSSH 不是** shell 替代品 —— 它通过 SSH/Telnet/Mosh 或本地/串口会话连接 shell

---

<a name="功能特性"></a>
# 功能特性

### 🗂️ Vault

主机、密钥、身份、代理配置、分组、代码片段、笔记、已知主机与端口转发规则的统一保管库。
网格、列表、树三种视图，快速搜索。

### 🖥️ 终端工作空间

分屏面板与可折叠主机树侧边栏。标签页并行会话。完整的会话恢复（含工作目录）。

### 📁 SFTP + 内置编辑器

双栏浏览器支持拖放、传输中心、暂停/恢复、目录上传、压缩包提取与内置代码编辑器。

### 🤖 AI Agent（Catty）

自然语言服务器管理、实时诊断、多主机编排与一键复杂操作。

### 🎨 个性化

明暗 UI 主题、终端主题、自定义 CSS、字体、强调色、应用图标与全局快捷键。

---

<a name="截图"></a>
# 截图

## 主窗口

<img width="3142" height="1764" alt="Screenshot 2026-07-02 at 22 51 24" src="https://github.com/user-attachments/assets/3116165d-623a-4d3a-a28a-914befb9b72d" />

## Vault 视图

<img width="3142" height="1764" alt="vault" src="https://github.com/user-attachments/assets/6893a259-aebc-4773-a2b1-48cbb1b3c92e" />

## 分屏终端

<img width="3142" height="1764" alt="split" src="https://github.com/user-attachments/assets/e9dcd0d8-19c4-4773-a2b1-48cbb1b3c92e" />

---

<a name="支持的发行版"></a>
# 支持的发行版

| 平台 | 要求 |
| --- | --- |
| Windows | Windows 10 22H2 (x64) + WebView2 常青版 |
| macOS | macOS 12 Monterey+（Intel 与 Apple Silicon） |
| Linux | GTK 4.14+ / WebKitGTK（Ubuntu 22.04+、Debian 12+、Fedora 38+），X11/Wayland |

目标平台由 [release-target-matrix.md](docs/migrations/wails-v3/release-target-matrix.md)
中的决策 `WV3-011`–`WV3-013` 冻结。

---

<a name="快速开始"></a>
# 快速开始

Electron 构建是当前的稳定发行载体；Wails/Go 壳正在逐能力落地。

### 环境要求

- Node.js 22+ 与 npm
- Go 1.25+（Wails 壳需要）
- Windows 10 22H2+ / macOS 12+ / GTK 4.14+ 的 Linux 桌面

### 开发

```bash
# 克隆仓库
git clone git@github.com:lemon-casino/LemonSSH.git
cd LemonSSH

# 安装依赖
npm install

# 启动开发模式（Vite + Electron —— 稳定壳）
npm run dev

# 或运行 Wails/Go 壳（加载同一前端）
npm run wails:dev
```

---

<a name="构建与打包"></a>
# 构建与打包

```bash
# Electron 生产构建（稳定壳）
npm run build
npm run pack:win     # Windows（NSIS 安装包）
npm run pack:mac     # macOS（DMG + ZIP）
npm run pack:linux   # Linux（AppImage + DEB + RPM）

# Wails/Go 壳：把前端构建进 Go 二进制
npm run wails:build  # 输出：bin/netcatty-wails.exe

# 迁移与 Go 检查
npm run check:migration-docs          # 迁移治理
npm run check:migration-electron-baseline
npm run check:contracts               # Go 基础契约 + TS 代码生成漂移
npm run check:profile-store           # 事务化 profile 存储（race）
npm run check:credentials             # 平台钥匙串提供方
npm run check:terminal-dataplane-core # 终端帧编解码 + 路由控制器
```

---

<a name="技术栈"></a>
# 技术栈

| 类别 | 稳定基线（Electron） | 目标运行时（Wails v3 + Go） |
|----------|---------------------------|--------------------------------|
| 壳 | Electron 40 | Wails v3 (beta.12) |
| 前端 | React 19, TypeScript, Vite 7 | React 19, TypeScript, Vite 7（不变） |
| 终端 | xterm.js 5, node-pty, MessagePort | xterm.js 5, Go ConPTY/Unix PTY, 二进制回环 WebSocket 数据面 |
| SSH/SFTP | ssh2, ssh2-sftp-client | golang.org/x/crypto/ssh, pkg/sftp |
| 持久化 | localStorage | Go 事务化 profile 存储（bbolt） |
| 凭据 | Electron safeStorage | OS 钥匙串（Windows 凭据管理器 / macOS 钥匙串 / Linux Secret Service） |
| 样式 | Tailwind CSS 4 | Tailwind CSS 4（不变） |

---

<a name="参与贡献"></a>
# 参与贡献

欢迎贡献！请随时提交 Pull Request。

1. Fork 本仓库
2. 创建功能分支（`git checkout -b feature/amazing-feature`）
3. 提交更改（`git commit -m 'Add some amazing feature'`）
4. 推送分支（`git push origin feature/amazing-feature`）
5. 发起 Pull Request

架构总览与编码约定见 [AGENTS.md](AGENTS.md)，迁移工作流见
[docs/migrations/wails-v3/README.md](docs/migrations/wails-v3/README.md)。

---

# 许可证

基于 [GPL-3.0 License](LICENSE) 开源。
