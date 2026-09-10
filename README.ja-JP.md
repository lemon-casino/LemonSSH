<p align="center">
  <img src="public/icon.png" alt="LemonSSH" width="128" height="128">
</p>

<h1 align="center">LemonSSH</h1>

<p align="center">
  <strong>🔥 AI 搭載の SSH クライアント・SFTP ブラウザ & ターミナルマネージャー 🚀</strong>
</p>

<p align="center">
  Go + Wails v3 と React/TypeScript フロントエンドで動く SSH ワークスペース。<br/>
  🔥 AI エージェント内蔵 · 画面分割ターミナル · Vault ビュー · SFTP ワークフロー · カスタムテーマ —— すべて揃っています。
</p>

<p align="center">
  <a href="#"><img alt="Platform" src="https://img.shields.io/badge/Platform-macOS%20%7C%20Windows%20%7C%20Linux-blue?style=for-the-badge"></a>
  &nbsp;
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/License-GPL--3.0-green?style=for-the-badge"></a>
</p>

<p align="center">
  <a href="README.zh-CN.md">简体中文</a> · <a href="README.zh-TW.md">繁體中文</a> · 日本語 · <a href="README.md">English</a>
</p>

---

<a name="lemonssh-とは"></a>
# LemonSSH とは

**LemonSSH** は、macOS・Windows・Linux 向けのモダンな SSH クライアント兼ターミナルマネージャーです。
複数のリモートサーバーを効率的に管理したい開発者・システム管理者・DevOps エンジニアのために設計されています。

- **LemonSSH は** PuTTY、Termius、SecureCRT、macOS ターミナルの代替です
- **LemonSSH は** デュアルペインのファイルブラウザーを備えた高機能 SFTP クライアントです
- **LemonSSH は** 分割ペイン・タブ・セッション管理に対応したターミナルワークスペースです
- **LemonSSH は** SSH・ローカルターミナル・Telnet・Mosh・シリアル接続に対応しています（利用可能な場合）
- **LemonSSH は** シェルの置き換えではありません —— SSH/Telnet/Mosh やローカル/シリアルセッションを通じてシェルに接続します

---

<a name="機能"></a>
# 機能

### 🗂️ Vault

ホスト・鍵・ID・プロキシプロファイル・グループ・スニペット・メモ・既知ホスト・ポートフォワーディングルールを
一元管理するボールト。グリッド・リスト・ツリーの 3 種類のビューと高速検索。

### 🖥️ ターミナルワークスペース

折りたたみ可能なホストツリーサイドバー付きの分割ペイン。タブによる並列セッション。
作業ディレクトリを含む完全なセッション復元。

### 📁 SFTP + 内蔵エディター

ドラッグ & ドロップ、転送センター、一時停止/再開、ディレクトリアップロード、アーカイブ展開、
内蔵コードエディターに対応したデュアルペインブラウザー。

### 🔌 ポートフォワーディング

ローカル・リモート・ダイナミック（SOCKS5）トンネル。ルール単位のライフサイクル、
ステータススナップショット、トレイからのワンクリック切替。

### 🤖 AI エージェント（Catty）

自然言語によるサーバー管理、リアルタイム診断、マルチホストオーケストレーション、ワンクリックの複雑な操作。

### 🧩 プラグインシステム

サンドボックスプラグイン、宣言的 UI スキーマ、パーミッションブローカー、ターミナル/SFTP 拡張点。

### 🎨 パーソナライズ

ライト/ダーク UI テーマ、ターミナルテーマ、カスタム CSS、フォント、アクセントカラー、
アプリアイコン、グローバルホットキー。

---

<a name="スクリーンショット"></a>
# スクリーンショット

## メインウィンドウ

<img width="3142" height="1764" alt="Screenshot 2026-07-02 at 22 51 24" src="https://github.com/user-attachments/assets/3116165d-623a-4d3a-a28a-914befb9b72d" />

## Vault ビュー

<img width="3142" height="1764" alt="vault" src="https://github.com/user-attachments/assets/6893a259-aebc-4773-a2b1-48cbb1b3c92e" />

## 分割ターミナル

<img width="3142" height="1764" alt="split" src="https://github.com/user-attachments/assets/e9dcd0d8-19c4-4773-a2b1-48cbb1b3c92e" />

---

<a name="対応ディストリビューション"></a>
# 対応ディストリビューション

| プラットフォーム | 要件 |
| --- | --- |
| Windows | Windows 10 22H2 (x64) + WebView2 Evergreen |
| macOS | macOS 12 Monterey+（Intel と Apple Silicon） |
| Linux | GTK 4.14+ / WebKitGTK（Ubuntu 22.04+、Debian 12+、Fedora 38+）、X11/Wayland |

---

<a name="はじめに"></a>
# はじめに

### 前提条件

- Node.js 22+ と npm
- Go 1.25+
- Windows 10 22H2+ / macOS 12+ / GTK 4.14+ の Linux デスクトップ

### 開発

```bash
git clone git@github.com:lemon-casino/LemonSSH.git
cd LemonSSH
npm install
npm run wails:dev
```

---

<a name="ビルドとパッケージ"></a>
# ビルドとパッケージ

```bash
npm run wails:build
node scripts/package-wails.mjs
npm test
go test ./...
```

---

<a name="技術スタック"></a>
# 技術スタック

| カテゴリ | 技術 |
|----------|------------|
| シェル | Wails v3 + Go |
| フロントエンド | React 19, TypeScript, Vite 7 |
| ターミナル | xterm.js 5, Go ConPTY/Unix PTY, バイナリループバック WebSocket データプレーン |
| SSH/SFTP | golang.org/x/crypto/ssh, pkg/sftp |
| 永続化 | Go トランザクション プロファイルストア（bbolt） |
| 資格情報 | OS キーリング（Windows 資格情報マネージャー / macOS Keychain / Linux Secret Service） |
| スタイリング | Tailwind CSS 4 |

---

<a name="コントリビューション"></a>
# コントリビューション

コントリビューションを歓迎します。アーキテクチャ規約は [AGENTS.md](AGENTS.md) を参照してください。

---

# ライセンス

[GPL-3.0 License](LICENSE) のもとで公開されています。
