<p align="center">
  <img src="public/icon.png" alt="LemonSSH" width="128" height="128">
</p>

<h1 align="center">LemonSSH</h1>

<p align="center">
  <strong>🔥 AI 驅動的 SSH 用戶端、SFTP 瀏覽器與終端機管理工具 🚀</strong>
</p>

<p align="center">
  基於 Go + Wails v3 與 React/TypeScript 前端的 SSH 工作空間。<br/>
  🔥 內建 AI Agent · 分割終端機 · Vault 多檢視 · SFTP 工作流 · 自訂主題 —— 一應俱全。
</p>

<p align="center">
  <a href="#"><img alt="Platform" src="https://img.shields.io/badge/Platform-macOS%20%7C%20Windows%20%7C%20Linux-blue?style=for-the-badge"></a>
  &nbsp;
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/License-GPL--3.0-green?style=for-the-badge"></a>
</p>

<p align="center">
  <a href="README.zh-CN.md">简体中文</a> · 繁體中文 · <a href="README.ja-JP.md">日本語</a> · <a href="README.md">English</a>
</p>

---

<a name="lemonssh-是什麼"></a>
# LemonSSH 是什麼

**LemonSSH** 是一款支援 macOS、Windows 與 Linux 的現代 SSH 用戶端與終端機管理器，
為需要高效管理多台遠端伺服器的開發者、系統管理員與 DevOps 工程師而設計。

- **LemonSSH 是** PuTTY、Termius、SecureCRT 與 macOS 終端機的替代選擇
- **LemonSSH 是** 具備雙窗格檔案瀏覽器的強大 SFTP 用戶端
- **LemonSSH 是** 支援分割窗格、分頁與工作階段管理的終端機工作空間
- **LemonSSH 支援** SSH、本機終端機、Telnet、Mosh 與序列連線（可用時）
- **LemonSSH 不是** shell 替代品 —— 它透過 SSH/Telnet/Mosh 或本機/序列工作階段連線 shell

---

<a name="功能特色"></a>
# 功能特色

### 🗂️ Vault

主機、金鑰、身分、代理設定、群組、程式碼片段、筆記、已知主機與連接埠轉送規則的統一保管庫。
格線、清單、樹狀三種檢視，快速搜尋。

### 🖥️ 終端機工作空間

分割窗格與可摺疊主機樹側邊欄。分頁平行工作階段。完整的工作階段還原（含工作目錄）。

### 📁 SFTP + 內建編輯器

雙窗格瀏覽器支援拖放、傳輸中心、暫停/繼續、目錄上傳、壓縮檔解壓與內建程式碼編輯器。

### 🔌 連接埠轉送

本機、遠端與動態（SOCKS5）隧道，依規則管理生命週期，支援狀態快照與系統匣一鍵開關。

### 🤖 AI Agent（Catty）

自然語言伺服器管理、即時診斷、多主機編排與一鍵複雜操作。

### 🧩 外掛系統

沙箱外掛、宣告式 UI schema、權限經紀，以及終端機/SFTP 擴充點。

### 🎨 個人化

明暗 UI 主題、終端機主題、自訂 CSS、字型、強調色、應用程式圖示與全域快速鍵。

---

<a name="螢幕截圖"></a>
# 螢幕截圖

## 主視窗

<img width="3142" height="1764" alt="Screenshot 2026-07-02 at 22 51 24" src="https://github.com/user-attachments/assets/3116165d-623a-4d3a-a28a-914befb9b72d" />

## Vault 檢視

<img width="3142" height="1764" alt="vault" src="https://github.com/user-attachments/assets/6893a259-aebc-4773-a2b1-48cbb1b3c92e" />

## 分割終端機

<img width="3142" height="1764" alt="split" src="https://github.com/user-attachments/assets/e9dcd0d8-19c4-4773-a2b1-48cbb1b3c92e" />

---

<a name="支援的發行版"></a>
# 支援的發行版

| 平台 | 要求 |
| --- | --- |
| Windows | Windows 10 22H2 (x64) + WebView2 常青版 |
| macOS | macOS 12 Monterey+（Intel 與 Apple Silicon） |
| Linux | GTK 4.14+ / WebKitGTK（Ubuntu 22.04+、Debian 12+、Fedora 38+），X11/Wayland |

---

<a name="快速開始"></a>
# 快速開始

### 環境需求

- Node.js 22+ 與 npm
- Go 1.25+
- Windows 10 22H2+ / macOS 12+ / GTK 4.14+ 的 Linux 桌面

### 開發

```bash
git clone git@github.com:lemon-casino/LemonSSH.git
cd LemonSSH
npm install
npm run wails:dev
```

---

<a name="建置與打包"></a>
# 建置與打包

```bash
npm run wails:build
node scripts/package-wails.mjs
npm test
go test ./...
```

---

<a name="技術堆疊"></a>
# 技術堆疊

| 類別 | 技術 |
|----------|------------|
| 殼 | Wails v3 + Go |
| 前端 | React 19, TypeScript, Vite 7 |
| 終端機 | xterm.js 5, Go ConPTY/Unix PTY, 回環 WebSocket 資料面 |
| SSH/SFTP | golang.org/x/crypto/ssh, pkg/sftp |
| 持久化 | Go 交易式 profile 儲存（bbolt） |
| 憑證 | OS 鑰匙圈（Windows 認證管理員 / macOS Keychain / Linux Secret Service） |
| 樣式 | Tailwind CSS 4 |

---

<a name="參與貢獻"></a>
# 參與貢獻

歡迎貢獻！請隨時提交 Pull Request。架構慣例見 [AGENTS.md](AGENTS.md)。

---

# 授權條款

基於 [GPL-3.0 License](LICENSE) 開源。
