<p align="center">
  <img src="public/icon.png" alt="LemonSSH" width="128" height="128">
</p>

<h1 align="center">LemonSSH</h1>

<p align="center">
  <strong>🔥 AI 驱动的 SSH 客户端、SFTP 浏览器 & 终端管理器 🚀</strong>
</p>

<p align="center">
  基于 Go + Wails v3 与 React/TypeScript 前端的 SSH 工作空间。<br/>
  🔥 内置 AI Agent · 分屏终端 · Vault 多视图 · SFTP 工作流 · 自定义主题 —— 一应俱全。
</p>

<p align="center">
  <a href="#"><img alt="Platform" src="https://img.shields.io/badge/Platform-macOS%20%7C%20Windows%20%7C%20Linux-blue?style=for-the-badge"></a>
  &nbsp;
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/License-GPL--3.0-green?style=for-the-badge"></a>
</p>

<p align="center">
  简体中文 · <a href="README.zh-TW.md">繁體中文</a> · <a href="README.ja-JP.md">日本語</a> · <a href="README.md">English</a>
</p>

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

### 🔌 端口转发

本地、远程与动态（SOCKS5）隧道，按规则管理生命周期，支持状态快照与托盘一键开关。

### 🤖 AI Agent（Catty）

自然语言服务器管理、实时诊断、多主机编排与一键复杂操作。

### 🧩 插件系统

沙箱插件，声明式 UI schema、权限经纪，以及终端/SFTP 扩展点。

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

---

<a name="快速开始"></a>
# 快速开始

### 环境要求

- Node.js 22+ 与 npm
- Go 1.25+
- Windows 10 22H2+ / macOS 12+ / GTK 4.14+ 的 Linux 桌面

### 开发

```bash
git clone git@github.com:lemon-casino/LemonSSH.git
cd LemonSSH
npm install
npm run wails:dev
```

---

<a name="构建与打包"></a>
# 构建与打包

```bash
npm run wails:build
node scripts/package-wails.mjs
npm test
go test ./...
```

---

<a name="技术栈"></a>
# 技术栈

| 类别 | 技术 |
|----------|------------|
| 壳 | Wails v3 + Go |
| 前端 | React 19, TypeScript, Vite 7 |
| 终端 | xterm.js 5, Go ConPTY/Unix PTY, 回环 WebSocket 数据面 |
| SSH/SFTP | golang.org/x/crypto/ssh, pkg/sftp |
| 持久化 | Go 事务化 profile 存储（bbolt） |
| 凭据 | OS 钥匙串（Windows 凭据管理器 / macOS 钥匙串 / Linux Secret Service） |
| 样式 | Tailwind CSS 4 |

---

<a name="参与贡献"></a>
# 参与贡献

欢迎贡献！请随时提交 Pull Request。架构约定见 [AGENTS.md](AGENTS.md)。

---

# 许可证

基于 [GPL-3.0 License](LICENSE) 开源。
