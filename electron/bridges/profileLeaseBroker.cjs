"use strict";

// Electron-side adapter for the Go profile writer-lease broker (P2-03).
// Spawns the netcatty-profile-broker helper and speaks its JSONL protocol so
// both shells share one lease authority. Disposable until P2-05 wires the
// migration flow; the broker binary is built alongside the migration
// evidence, never shipped inside the Electron app.

const { spawn } = require("node:child_process");
const path = require("node:path");

const DEFAULT_TIMEOUT_MS = 10_000;

function resolveBrokerBinary(repoRoot) {
  return path.join(repoRoot, "cmd", "netcatty-profile-broker", "netcatty-profile-broker.exe");
}

class ProfileLeaseBroker {
  constructor({ repoRoot, profilePath, binaryPath } = {}) {
    if (!profilePath) throw new Error("profilePath is required");
    this.profilePath = profilePath;
    this.binaryPath = binaryPath || resolveBrokerBinary(repoRoot || process.cwd());
    this.child = null;
    this.pending = [];
    this.buffer = "";
  }

  start() {
    if (this.child) return;
    this.child = spawn(this.binaryPath, [], {
      shell: false,
      windowsHide: true,
      stdio: ["pipe", "pipe", "pipe"],
      env: { ...process.env, NETCATTY_PROFILE_PATH: this.profilePath },
    });
    this.child.stdout.setEncoding("utf8");
    this.child.stdout.on("data", (chunk) => this.#onData(chunk));
    this.child.stderr.on("data", () => {});
    this.child.on("close", () => {
      const waiters = this.pending;
      this.pending = [];
      this.child = null;
      for (const waiter of waiters) waiter.reject(new Error("profile lease broker exited"));
    });
  }

  #onData(chunk) {
    this.buffer += chunk;
    let index;
    while ((index = this.buffer.indexOf("\n")) >= 0) {
      const line = this.buffer.slice(0, index).trim();
      this.buffer = this.buffer.slice(index + 1);
      if (!line) continue;
      const waiter = this.pending.shift();
      if (!waiter) continue;
      clearTimeout(waiter.timer);
      try {
        waiter.resolve(JSON.parse(line));
      } catch (error) {
        waiter.reject(error);
      }
    }
  }

  request(body, timeoutMs = DEFAULT_TIMEOUT_MS) {
    this.start();
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error("profile lease broker timeout")), timeoutMs);
      this.pending.push({ resolve, reject, timer });
      this.child.stdin.write(`${JSON.stringify(body)}\n`);
    });
  }

  async acquire(holder, ttlMs = 30_000) {
    const response = await this.request({ op: "acquire", holder, ttlMs });
    if (!response.ok) throw new Error(response.error || "lease acquire failed");
    return response.lease;
  }

  async renew(ttlMs = 30_000) {
    const response = await this.request({ op: "renew", ttlMs });
    if (!response.ok) throw new Error(response.error || "lease renew failed");
    return response.lease;
  }

  async release() {
    const response = await this.request({ op: "release" });
    if (!response.ok) throw new Error(response.error || "lease release failed");
  }

  async status() {
    const response = await this.request({ op: "status" });
    return response.lease || null;
  }

  stop() {
    if (!this.child) return;
    this.child.kill();
    this.child = null;
  }
}

module.exports = { ProfileLeaseBroker, resolveBrokerBinary };
