<p align="center">
  <img src="public/icon.png" alt="LemonSSH" width="128" height="128">
</p>

<h1 align="center">LemonSSH</h1>

<p align="center">
  <strong>🔥 AI 搭載の SSH クライアント・SFTP ブラウザ & ターミナルマネージャー 🚀</strong>
</p>

<p align="center">
  Electron から Go + Wails v3 ランタイムへ移行中の、美しく高機能な SSH ワークスペース。<br/>
  🔥 AI エージェント内蔵 · 画面分割ターミナル · Vault ビュー · SFTP ワークフロー · カスタムテーマ —— すべて揃っています。
</p>

<p align="center">
  <a href="#"><img alt="Platform" src="https://img.shields.io/badge/Platform-macOS%20%7C%20Windows%20%7C%20Linux-blue?style=for-the-badge"></a>
  &nbsp;
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/License-GPL--3.0-green?style=for-the-badge"></a>
  &nbsp;
  <a href="docs/migrations/wails-v3/README.md"><img alt="Migration" src="https://img.shields.io/badge/Migration-Wails%20v3%20%2B%20Go-informational?style=for-the-badge&logo=go"></a>
</p>

<p align="center">
  <a href="README.zh-CN.md">简体中文</a> · <a href="README.zh-TW.md">繁體中文</a> · 日本語 · <a href="README.md">English</a>
</p>

---

## 移行状況（Electron → Go + Wails v3）

このリポジトリは Netcatty プロジェクトの後継です。React/TypeScript フロントエンド、
ユーザーデータ、機能セットを維持したまま、ランタイムを Electron/Node.js から
**Go + Wails v3** へ移行しています。

| 領域 | 状態 |
| --- | --- |
| 移行ガバナンス（ドキュメント・レジャー・CI エビデンスワークフロー） | ✅ 整備済み |
| シェル非依存フロントエンドポート + Wails スケルトン + 基本コントラクト | ✅ 完了 |
| Go トランザクション プロファイルストア・ライターリース・資格情報プロバイダー | ✅ コア完了 |
| 暗号化マイグレーションバンドル（エクスポート/インポート/ロールバック） | ✅ コア完了 |
| ターミナル バイナリデータプレーン・ローカル PTY・SSH ダイヤル/プール・SFTP・転送 | ✅ コア完了 |
| 3 プラットフォーム実機エビデンス・ドメイン別永続化切り替え | 🚧 収集中 |
| Telnet/Serial/Mosh/ET/ZMODEM・システム機能・プラグイン v2・同期・AI | ⏳ 順番待ち |

権威あるステータス：[capability-matrix.md](docs/migrations/wails-v3/capability-matrix.md) · レジャー：[migration-ledger.md](docs/migrations/wails-v3/migration-ledger.md) · 計画：[implementation-plan.md](docs/migrations/wails-v3/implementation-plan.md)。

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

### 🤖 AI エージェント（Catty）

自然言語によるサーバー管理、リアルタイム診断、マルチホストオーケストレーション、ワンクリックの複雑な操作。

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

対応ターゲットは [release-target-matrix.md](docs/migrations/wails-v3/release-target-matrix.md)
の決定 `WV3-011`–`WV3-013` により凍結されています。

---

<a name="はじめに"></a>
# はじめに

現在の安定リリースキャリアは Electron ビルドです。Wails/Go シェルは機能単位で段階的に展開中です。

### 前提条件

- Node.js 22+ と npm
- Go 1.25+（Wails シェルに必要）
- Windows 10 22H2+ / macOS 12+ / GTK 4.14+ の Linux デスクトップ

### 開発

```bash
# リポジトリをクローン
git clone git@github.com:lemon-casino/LemonSSH.git
cd LemonSSH

# 依存関係をインストール
npm install

# 開発モードを起動（Vite + Electron —— 安定シェル）
npm run dev

# または Wails/Go シェルを実行（同じフロントエンドを読み込み）
npm run wails:dev
```

---

<a name="ビルドとパッケージ"></a>
# ビルドとパッケージ

```bash
# Electron 本番ビルド（安定シェル）
npm run build
npm run pack:win     # Windows（NSIS インストーラー）
npm run pack:mac     # macOS（DMG + ZIP）
npm run pack:linux   # Linux（AppImage + DEB + RPM）

# Wails/Go シェル：フロントエンドを Go バイナリに組み込む
npm run wails:build  # 出力：bin/netcatty-wails.exe

# 移行と Go のチェック
npm run check:migration-docs          # 移行ガバナンス
npm run check:migration-electron-baseline
npm run check:contracts               # Go 基本コントラクト + TS コード生成ドリフト
npm run check:profile-store           # トランザクション プロファイルストア（race）
npm run check:credentials             # プラットフォームキーリングプロバイダー
npm run check:terminal-dataplane-core # ターミナル フレームコーデック + ルートコントローラー
```

---

<a name="技術スタック"></a>
# 技術スタック

| カテゴリ | 安定ベースライン（Electron） | 目標ランタイム（Wails v3 + Go） |
|----------|---------------------------|--------------------------------|
| シェル | Electron 40 | Wails v3 (beta.12) |
| フロントエンド | React 19, TypeScript, Vite 7 | React 19, TypeScript, Vite 7（変更なし） |
| ターミナル | xterm.js 5, node-pty, MessagePort | xterm.js 5, Go ConPTY/Unix PTY, バイナリループバック WebSocket データプレーン |
| SSH/SFTP | ssh2, ssh2-sftp-client | golang.org/x/crypto/ssh, pkg/sftp |
| 永続化 | localStorage | Go トランザクション プロファイルストア（bbolt） |
| 資格情報 | Electron safeStorage | OS キーリング（Windows 資格情報マネージャー / macOS Keychain / Linux Secret Service） |
| スタイリング | Tailwind CSS 4 | Tailwind CSS 4（変更なし） |

---

<a name="コントリビューション"></a>
# コントリビューション

コントリビューションを歓迎します！Pull Request をお気軽にどうぞ。

1. リポジトリをフォーク
2. フィーチャーブランチを作成（`git checkout -b feature/amazing-feature`）
3. 変更をコミット（`git commit -m 'Add some amazing feature'`）
4. ブランチをプッシュ（`git push origin feature/amazing-feature`）
5. Pull Request を作成

アーキテクチャの概要とコーディング規約は [AGENTS.md](AGENTS.md)、
移行ワークフローは [docs/migrations/wails-v3/README.md](docs/migrations/wails-v3/README.md) を参照してください。

---

# ライセンス

[GPL-3.0 License](LICENSE) のもとで公開されています。
