import assert from "node:assert/strict";
import { test } from "node:test";

import {
  TERMINAL_DATA_SUBPROTOCOL,
  buildTerminalSocketUrl,
  entryToRemoteFile,
  modeToPermissions,
  pickSSHConnectArgs,
  statToSftpStatResult,
  terminalSocketSubprotocols,
} from "./terminalRoute";

test("buildTerminalSocketUrl joins loopback host, session and generation", () => {
  assert.equal(
    buildTerminalSocketUrl("127.0.0.1:54321", "user@host:22", 1, "data"),
    "ws://127.0.0.1:54321/v1/data/user%40host%3A22?generation=1",
  );
  assert.equal(
    buildTerminalSocketUrl("127.0.0.1:54321", "s1", 3, "urgent"),
    "ws://127.0.0.1:54321/v1/urgent/s1?generation=3",
  );
});

test("buildTerminalSocketUrl rejects missing address or session", () => {
  assert.throws(() => buildTerminalSocketUrl("", "s1", 1, "data"));
  assert.throws(() => buildTerminalSocketUrl("127.0.0.1:1", "", 1, "data"));
});

test("terminalSocketSubprotocols carries namespace and route token", () => {
  assert.deepEqual(terminalSocketSubprotocols("abc123"), [
    TERMINAL_DATA_SUBPROTOCOL,
    "route.abc123",
  ]);
});

test("entryToRemoteFile maps directories, files and symlinks", () => {
  assert.deepEqual(
    entryToRemoteFile({
      name: "etc",
      isDir: true,
      size: 4096,
      mode: "drwxr-xr-x",
      modTime: "2026-09-10T00:00:00Z",
    }),
    {
      name: "etc",
      type: "directory",
      size: "4096",
      lastModified: "2026-09-10T00:00:00Z",
      permissions: "rwxr-xr-x",
      linkTarget: null,
    },
  );
  const symlink = entryToRemoteFile({
    name: "link",
    isDir: true,
    size: 10,
    mode: "Lrwxrwxrwx",
    modTime: "2026-09-10T00:00:00Z",
    symlink: true,
  });
  assert.equal(symlink.type, "symlink");
  assert.equal(symlink.linkTarget, "directory");
});

test("statToSftpStatResult maps path tail and numeric fields", () => {
  const stat = statToSftpStatResult({
    path: "/var/log/boot.log",
    isDir: false,
    size: 1234,
    mode: "-rw-r--r--",
    modTime: "2026-09-10T01:02:03Z",
  });
  assert.equal(stat.name, "boot.log");
  assert.equal(stat.type, "file");
  assert.equal(stat.size, 1234);
  assert.equal(stat.permissions, "rw-r--r--");
  assert.equal(stat.lastModified, Date.parse("2026-09-10T01:02:03Z"));
});

test("modeToPermissions keeps the last nine characters only", () => {
  assert.equal(modeToPermissions("-rw-r--r--"), "rw-r--r--");
  assert.equal(modeToPermissions("drwxr-xr-x"), "rwxr-xr-x");
  assert.equal(modeToPermissions("short"), undefined);
});

test("pickSSHConnectArgs normalizes defaults", () => {
  assert.deepEqual(
    pickSSHConnectArgs({ hostname: "h", username: "u" }),
    {
      hostname: "h",
      username: "u",
      port: 22,
      password: "",
      privateKey: "",
      passphrase: "",
      certificate: "",
      proxyUrl: "",
      enableMfa: false,
      useAgent: false,
      identityFilePaths: [],
      cols: 80,
      rows: 24,
      jumpHosts: [],
    },
  );
});

test("pickSSHConnectArgs accepts key, MFA, jump and socks proxy", () => {
  assert.deepEqual(
    pickSSHConnectArgs({
      hostname: "h",
      username: "u",
      privateKey: "PEM",
      passphrase: "pw",
      requiresMfa: true,
      proxy: { type: "socks5", host: "127.0.0.1", port: 1080 },
      jumpHosts: [{ hostname: "jump", username: "bastion", port: 2222, password: "jpw" }],
    }),
    {
      hostname: "h",
      username: "u",
      port: 22,
      password: "",
      privateKey: "PEM",
      passphrase: "pw",
      certificate: "",
      proxyUrl: "socks5://127.0.0.1:1080",
      enableMfa: true,
      useAgent: false,
      identityFilePaths: [],
      cols: 80,
      rows: 24,
      jumpHosts: [{
        hostname: "jump",
        username: "bastion",
        port: 2222,
        password: "jpw",
        privateKey: "",
        passphrase: "",
        certificate: "",
        proxyUrl: "",
        enableMfa: false,
        useAgent: false,
        identityFilePaths: [],
        cols: 80,
        rows: 24,
        jumpHosts: [],
      }],
    },
  );
});

test("pickSSHConnectArgs maps certificates and fails closed on command proxy", () => {
  const args = pickSSHConnectArgs({ hostname: "h", username: "u", certificate: "ssh-rsa-cert AAAA", privateKey: "PEM" });
  assert.equal(args.certificate, "ssh-rsa-cert AAAA");
  assert.throws(
    () => pickSSHConnectArgs({ hostname: "h", username: "u", proxy: { type: "command", command: "nc", host: "x", port: 1 } }),
    /proxy/,
  );
});

test("pickSSHConnectArgs maps agent and identity files onto Connect", () => {
  const args = pickSSHConnectArgs({
    hostname: "h",
    username: "u",
    useSshAgent: true,
    identityFilePaths: ["~/.ssh/id"],
  });
  assert.equal(args.useAgent, true);
  assert.deepEqual(args.identityFilePaths, ["~/.ssh/id"]);
});
