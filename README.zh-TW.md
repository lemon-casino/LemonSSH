<p align="center">
  <img src="public/icon.png" alt="LemonSSH" width="128" height="128">
</p>

<h1 align="center">LemonSSH</h1>

<p align="center">
  <strong>🔥 AI 驅動的 SSH 用戶端、SFTP 瀏覽器與終端機管理工具 🚀</strong>
</p>

<p align="center">
  一個美觀且功能豐富的 SSH 工作空間，正在從 Electron 遷移到 Go + Wails v3 執行環境。<br/>
  🔥 內建 AI Agent · 分割終端機 · Vault 多檢視 · SFTP 工作流 · 自訂主題 —— 一應俱全。
</p>

<p align="center">
  <a href="#"><img alt="Platform" src="https://img.shields.io/badge/Platform-macOS%20%7C%20Windows%20%7C%20Linux-blue?style=for-the-badge"></a>
  &nbsp;
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/License-GPL--3.0-green?style=for-the-badge"></a>
  &nbsp;
  <a href="docs/migrations/wails-v3/README.md"><img alt="Migration" src="https://img.shields.io/badge/Migration-Wails%20v3%20%2B%20Go-informational?style=for-the-badge&logo=go"></a>
</p>

<p align="center">
  <a href="README.zh-CN.md">简体中文</a> · 繁體中文 · <a href="README.ja-JP.md">日本語</a> · <a href="README.md">English</a>
</p>

---

## 遷移狀態（Electron → Go + Wails v3）

本儲存庫是 Netcatty 專案的延續：在保留 React/TypeScript 前端、使用者資料與功能集的前提下，
將執行環境從 Electron/Node.js 遷移到 **Go + Wails v3**。

| 領域 | 狀態 |
| --- | --- |
| 遷移治理（文件、帳本、CI 證據工作流） | ✅ 就緒 |
| 殼中立前端連接埠 + Wails 骨架 + 基礎契約 | ✅ 完成 |
| Go 交易式 profile 儲存、寫入者租約、憑證提供方 | ✅ 核心完成 |
| 加密遷移套件（匯出/匯入/回滾） | ✅ 核心完成 |
| 終端機二進位資料面、本機 PTY、SSH 撥號/連線池、SFTP、傳輸、轉發 | ✅ 核心完成 |
| 三平台實機證據、逐域持久化切換 | 🚧 蒐集中 |
| Telnet/Serial/Mosh/ET/ZMODEM、系統能力、外掛 v2、同步、AI | ⏳ 排隊中 |

權威狀態：[capability-matrix.md](docs/migrations/wails-v3/capability-matrix.md) · 帳本：[migration-ledger.md](docs/migrations/wails-v3/migration-ledger.md) · 計畫：[implementation-plan.md](docs/migrations/wails-v3/implementation-plan.md)。

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

### 🤖 AI Agent（Catty）

自然語言伺服器管理、即時診斷、多主機編排與一鍵複雜操作。

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

目標平台由 [release-target-matrix.md](docs/migrations/wails-v3/release-target-matrix.md)
中的決策 `WV3-011`–`WV3-013` 凍結。

---

<a name="快速開始"></a>
# 快速開始

Electron 建置是目前的穩定發行載體；Wails/Go 殼正在逐能力落地。

### 環境需求

- Node.js 22+ 與 npm
- Go 1.25+（Wails 殼需要）
- Windows 10 22H2+ / macOS 12+ / GTK 4.14+ 的 Linux 桌面

### 開發

```bash
# 複製儲存庫
git clone git@github.com:lemon-casino/LemonSSH.git
cd LemonSSH

# 安裝相依套件
npm install

# 啟動開發模式（Vite + Electron —— 穩定殼）
npm run dev

# 或執行 Wails/Go 殼（載入同一前端）
npm run wails:dev
```

---

<a name="建置與打包"></a>
# 建置與打包

```bash
# Electron 生產建置（穩定殼）
npm run build
npm run pack:win     # Windows（NSIS 安裝包）
npm run pack:mac     # macOS（DMG + ZIP）
npm run pack:linux   # Linux（AppImage + DEB + RPM）

# Wails/Go 殼：把前端建置進 Go 二進位
npm run wails:build  # 輸出：bin/netcatty-wails.exe

# 遷移與 Go 檢查
npm run check:migration-docs          # 遷移治理
npm run check:migration-electron-baseline
npm run check:contracts               # Go 基礎契約 + TS 程式碼產生漂移
npm run check:profile-store           # 交易式 profile 儲存（race）
npm run check:credentials             # 平台鑰匙圈提供方
npm run check:terminal-dataplane-core # 終端機框架編解碼 + 路由控制器
```

---

<a name="技術堆疊"></a>
# 技術堆疊

| 類別 | 穩定基線（Electron） | 目標執行環境（Wails v3 + Go） |
|----------|---------------------------|--------------------------------|
| 殼 | Electron 40 | Wails v3 (beta.12) |
| 前端 | React 19, TypeScript, Vite 7 | React 19, TypeScript, Vite 7（不變） |
| 終端機 | xterm.js 5, node-pty, MessagePort | xterm.js 5, Go ConPTY/Unix PTY, 二進位回環 WebSocket 資料面 |
| SSH/SFTP | ssh2, ssh2-sftp-client | golang.org/x/crypto/ssh, pkg/sftp |
| 持久化 | localStorage | Go 交易式 profile 儲存（bbolt） |
| 憑證 | Electron safeStorage | OS 鑰匙圈（Windows 認證管理員 / macOS Keychain / Linux Secret Service） |
| 樣式 | Tailwind CSS 4 | Tailwind CSS 4（不變） |

---

<a name="參與貢獻"></a>
# 參與貢獻

歡迎貢獻！請隨時提交 Pull Request。

1. Fork 本儲存庫
2. 建立功能分支（`git checkout -b feature/amazing-feature`）
3. 提交變更（`git commit -m 'Add some amazing feature'`）
4. 推送分支（`git push origin feature/amazing-feature`）
5. 建立 Pull Request

架構總覽與編碼慣例見 [AGENTS.md](AGENTS.md)，遷移工作流程見
[docs/migrations/wails-v3/README.md](docs/migrations/wails-v3/README.md)。

---

# 授權條款

基於 [GPL-3.0 License](LICENSE) 開源。
