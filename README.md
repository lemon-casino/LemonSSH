<p align="center">
  <img src="public/icon.png" alt="LemonSSH" width="128" height="128">
</p>

<h1 align="center">LemonSSH</h1>

<p align="center">
  <strong>🔥 AI-Powered SSH Client, SFTP Browser & Terminal Manager 🚀</strong>
</p>

<p align="center">
  A beautiful, feature-rich SSH workspace, migrating from Electron to a Go + Wails v3 runtime.<br/>
  🔥 Built-in AI Agent · Split terminals · Vault views · SFTP workflows · Custom themes — all in one.
</p>

<p align="center">
  <a href="#"><img alt="Platform" src="https://img.shields.io/badge/Platform-macOS%20%7C%20Windows%20%7C%20Linux-blue?style=for-the-badge"></a>
  &nbsp;
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/License-GPL--3.0-green?style=for-the-badge"></a>
  &nbsp;
  <a href="docs/migrations/wails-v3/README.md"><img alt="Migration" src="https://img.shields.io/badge/Migration-Wails%20v3%20%2B%20Go-informational?style=for-the-badge&logo=go"></a>
</p>

---

## Migration Status (Electron → Go + Wails v3)

This repository is the continuation of the Netcatty project, migrating its
runtime from Electron/Node.js to **Go + Wails v3** while keeping the
React/TypeScript frontend, user data and feature set intact.

| Domain | Status |
| --- | --- |
| Migration governance (docs, ledger, CI evidence workflow) | ✅ in place |
| Shell-neutral frontend ports + Wails skeleton + base contracts | ✅ done |
| Go transactional profile store, writer lease, credential providers | ✅ core done |
| Encrypted migration bundle (export/import/rollback) | ✅ core done |
| Terminal binary data plane, local PTY, SSH dial/pool, SFTP, transfer, forwarding | ✅ core done |
| Three-platform live evidence, per-domain persistence cutover | 🚧 collecting |
| Telnet/Serial/Mosh/ET/ZMODEM, system capabilities, plugins v2, sync, AI | ⏳ queued |

Authoritative status: [docs/migrations/wails-v3/capability-matrix.md](docs/migrations/wails-v3/capability-matrix.md) · ledger: [docs/migrations/wails-v3/migration-ledger.md](docs/migrations/wails-v3/migration-ledger.md) · plan: [docs/migrations/wails-v3/implementation-plan.md](docs/migrations/wails-v3/implementation-plan.md).

---

# Contents <!-- omit in toc -->

- [What is LemonSSH](#what-is-lemonssh)
- [Features](#features)
- [Screenshots](#screenshots)
- [Supported Distros](#supported-distros)
- [Getting Started](#getting-started)
- [Development](#development)
- [Build & Package](#build--package)
- [Tech Stack](#tech-stack)
- [Contributing](#contributing)
- [License](#license)

---

<a name="what-is-lemonssh"></a>
# What is LemonSSH

**LemonSSH** is a modern SSH client and terminal manager for macOS, Windows, and Linux, designed for developers, sysadmins, and DevOps engineers who need to manage multiple remote servers efficiently.

- **LemonSSH is** an alternative to PuTTY, Termius, SecureCRT, and macOS Terminal.app for SSH connections
- **LemonSSH is** a powerful SFTP client with dual-pane file browser
- **LemonSSH is** a terminal workspace with split panes, tabs, and session management
- **LemonSSH supports** SSH, local terminal, Telnet, Mosh, and Serial connections (when available)
- **LemonSSH is not** a shell replacement — it connects to shells via SSH/Telnet/Mosh or local/serial sessions

---

<a name="features"></a>
# Features

### 🗂️ Vault

A unified vault for hosts, keys, identities, proxy profiles, groups, snippets, notes, known hosts, and port forwarding rules. Grid, list, and tree views with fast search.

### 🖥️ Terminal Workspaces

Split panes with collapsible host-tree sidebar. Parallel sessions via tabs. Full session restore including working directories.

### 📁 SFTP + Built-in Editor

Dual-pane browser with drag & drop, transfer center, pause/resume, directory upload, archive extraction, and a built-in code editor.

### 🤖 AI Agent (Catty)

Natural language server management, real-time diagnostics, multi-host orchestration and one-click complex operations.

### 🎨 Personalization

Light/dark UI themes, terminal themes, custom CSS, fonts, accent colors, app icons, and global hotkeys.

---

<a name="screenshots"></a>
# Screenshots

## Main Window

<img width="3142" height="1764" alt="Screenshot 2026-07-02 at 22 51 24" src="https://github.com/user-attachments/assets/3116165d-623a-4d3a-a28a-914befb9b72d" />

## Vault Views

<img width="3142" height="1764" alt="vault" src="https://github.com/user-attachments/assets/6893a259-aebc-4773-a2b1-48cbb1b3c92e" />

## Split Terminals

<img width="3142" height="1764" alt="split" src="https://github.com/user-attachments/assets/e9dcd0d8-19c4-4773-a2b1-48cbb1b3c92e" />

---

<a name="supported-distros"></a>
# Supported Distros

| Platform | Requirement |
| --- | --- |
| Windows | Windows 10 22H2 (x64) with WebView2 Evergreen |
| macOS | macOS 12 Monterey+ (Intel & Apple Silicon) |
| Linux | GTK 4.14+ / WebKitGTK (Ubuntu 22.04+, Debian 12+, Fedora 38+), X11/Wayland |

Targets frozen by decisions `WV3-011`–`WV3-013` in
[release-target-matrix.md](docs/migrations/wails-v3/release-target-matrix.md).

---

<a name="getting-started"></a>
# Getting Started

The Electron build is today's stable release carrier; the Wails/Go shell is
landing capability by capability.

### Prerequisites

- Node.js 22+ and npm
- Go 1.25+ (for the Wails shell)
- Windows 10 22H2+ / macOS 12+ / a GTK 4.14+ Linux desktop

### Development

```bash
# Clone the repository
git clone git@github.com:lemon-casino/LemonSSH.git
cd LemonSSH

# Install dependencies
npm install

# Start development mode (Vite + Electron — the stable shell)
npm run dev

# Or run the Wails/Go shell (loads the same frontend)
npm run wails:dev
```

---

<a name="build--package"></a>
# Build & Package

```bash
# Electron production build (stable shell)
npm run build
npm run pack:win     # Windows (NSIS installer)
npm run pack:mac     # macOS (DMG + ZIP)
npm run pack:linux   # Linux (AppImage + DEB + RPM)

# Wails/Go shell: build the frontend into the Go binary
npm run wails:build  # output: bin/netcatty-wails.exe

# Migration & Go checks
npm run check:migration-docs          # migration governance
npm run check:migration-electron-baseline
npm run check:contracts               # Go base contracts + TS codegen drift
npm run check:profile-store           # transactional profile store (race)
npm run check:credentials             # platform keyring provider
npm run check:terminal-dataplane-core # terminal frame codec + route controller
```

---

<a name="tech-stack"></a>
# Tech Stack

| Category | Stable baseline (Electron) | Target runtime (Wails v3 + Go) |
|----------|---------------------------|--------------------------------|
| Shell | Electron 40 | Wails v3 (beta.12) |
| Frontend | React 19, TypeScript, Vite 7 | React 19, TypeScript, Vite 7 (unchanged) |
| Terminal | xterm.js 5, node-pty, MessagePort | xterm.js 5, Go ConPTY/Unix PTY, binary loopback WebSocket data plane |
| SSH/SFTP | ssh2, ssh2-sftp-client | golang.org/x/crypto/ssh, pkg/sftp |
| Persistence | localStorage | Go transactional profile store (bbolt) |
| Credentials | Electron safeStorage | OS keyring (Win Credential Manager / macOS Keychain / Linux Secret Service) |
| Styling | Tailwind CSS 4 | Tailwind CSS 4 (unchanged) |

---

<a name="contributing"></a>
# Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

See [AGENTS.md](AGENTS.md) for architecture overview and coding conventions,
and [docs/migrations/wails-v3/README.md](docs/migrations/wails-v3/README.md)
for the migration workflow.

---

# License

Released under the [GPL-3.0 License](LICENSE).
