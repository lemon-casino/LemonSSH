// Pure mapping helpers for the Wails adapter (Slice C). No imports of the
// generated bindings or the Wails runtime: everything here is trivially
// unit-testable in plain Node.

import type { RemoteFile } from "../../../domain/models/workspace";

/** Subprotocol namespace announced by the Go data plane server. */
export const TERMINAL_DATA_SUBPROTOCOL = "netcatty-terminal-v1";

/** Minimal structural view of the generated sftp.Entry binding model. */
export interface WailsSftpEntry {
  name: string;
  isDir: boolean;
  size: number;
  mode: string;
  modTime: string;
  symlink?: boolean;
}

/** Minimal structural view of the generated sftp.FileInfo binding model. */
export interface WailsSftpFileInfo {
  path: string;
  isDir: boolean;
  size: number;
  mode: string;
  modTime: string;
}

/** Minimal structural view of the generated dataplane.RouteBootstrap model. */
export interface WailsRouteBootstrap {
  SessionID: string;
  Generation: number;
  DataToken: string;
  UrgentToken: string;
  WindowBytes: number;
}

/**
 * Builds the loopback WebSocket URL for one terminal route channel. The Go
 * listener binds 127.0.0.1 only and requires the exact Host, so listenAddr
 * must be the value reported by the server (host:port), never a rewritten one.
 */
export function buildTerminalSocketUrl(
  listenAddr: string,
  sessionID: string,
  generation: number,
  channel: "data" | "urgent",
): string {
  if (!listenAddr) throw new Error("terminal data plane address is not available yet");
  if (!sessionID) throw new Error("terminal session id is required");
  return `ws://${listenAddr}/v1/${channel}/${encodeURIComponent(sessionID)}?generation=${generation}`;
}

/**
 * Browser WebSocket subprotocol array carrying the one-use route token.
 * Browsers join array members with ", " which matches the server's
 * two-name Sec-WebSocket-Protocol expectation.
 */
export function terminalSocketSubprotocols(token: string): string[] {
  return [TERMINAL_DATA_SUBPROTOCOL, `route.${token}`];
}

/** Go os.FileMode string ("drwxr-xr-x") to the 9-char permission triplet. */
export function modeToPermissions(mode: string): string | undefined {
  if (mode.length < 9) return undefined;
  return mode.slice(-9);
}

/** Maps a Go SFTP listing entry to the renderer's RemoteFile contract. */
export function entryToRemoteFile(entry: WailsSftpEntry): RemoteFile {
  const type: RemoteFile["type"] = entry.symlink
    ? "symlink"
    : entry.isDir
      ? "directory"
      : "file";
  return {
    name: entry.name,
    type,
    size: String(entry.size),
    lastModified: entry.modTime,
    permissions: modeToPermissions(entry.mode),
    // Resolved symlinks report the target type; unresolved ones read null.
    linkTarget: entry.symlink ? (entry.isDir ? "directory" : "file") : null,
  };
}

/** Maps a Go SFTP stat payload to the Electron-compatible stat contract. */
export function statToSftpStatResult(stat: WailsSftpFileInfo): {
  name: string;
  type: "file" | "directory" | "symlink";
  size: number;
  lastModified: number;
  permissions?: string;
} {
  const segments = stat.path.split("/");
  return {
    name: segments[segments.length - 1] || stat.path,
    type: stat.isDir ? "directory" : "file",
    size: stat.size,
    lastModified: Date.parse(stat.modTime) || 0,
    permissions: modeToPermissions(stat.mode),
  };
}

/**
 * Encodes raw stdin bytes to base64: Go []byte parameters travel as base64
 * strings through the Wails JSON binding layer. Chunked so large pastes do
 * not blow the call stack via String.fromCharCode(...spread).
 */
export function bytesToBase64(bytes: Uint8Array): string {
  let binary = "";
  const chunkSize = 0x8000;
  for (let offset = 0; offset < bytes.length; offset += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(offset, offset + chunkSize));
  }
  if (typeof btoa === "function") return btoa(binary);
  // Node test runtime fallback.
  return Buffer.from(binary, "binary").toString("base64");
}

/** Arguments the Go TerminalService.Connect binding accepts today. */export interface WailsSSHConnectArgs {
  hostname: string;
  port: number;
  username: string;
  password: string;
  cols: number;
  rows: number;
}

/**
 * Normalizes the Electron NetcattySSHOptions to the Go Connect binding.
 * Authentication shapes the Go binding does not accept yet fail closed with
 * an explicit migration error instead of silently degrading to password auth.
 */
export function pickSSHConnectArgs(options: {
  hostname: string;
  username: string;
  port?: number;
  password?: string;
  cols?: number;
  rows?: number;
  privateKey?: string;
  certificate?: string;
  passphrase?: string;
  requiresMfa?: boolean;
  jumpHosts?: unknown[];
  proxy?: unknown;
}): WailsSSHConnectArgs {
  const unsupported = [
    ["privateKey", options.privateKey],
    ["certificate", options.certificate],
    ["passphrase", options.passphrase],
    ["jumpHosts", options.jumpHosts?.length],
    ["proxy", options.proxy],
  ].filter(([, value]) => Boolean(value));
  if (options.requiresMfa) unsupported.push(["requiresMfa", true]);
  if (unsupported.length > 0) {
    throw new Error(
      `SSH options not migrated to the Wails Connect binding yet: ${unsupported.map(([name]) => name).join(", ")}`,
    );
  }
  return {
    hostname: options.hostname,
    username: options.username,
    port: options.port ?? 22,
    password: options.password ?? "",
    cols: options.cols ?? 80,
    rows: options.rows ?? 24,
  };
}
