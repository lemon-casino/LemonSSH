// Wails RuntimeClient adapter (P1-02). This module is the only frontend file
// allowed to import the Wails runtime and the generated bindings (enforced by
// ESLint). Ports without a Go owner reject every call fail-closed instead of
// pretending parity; they are implemented domain by domain from P2 onward.

import * as netcattyService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/netcattyservice";
import * as terminalService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/terminalservice";
import * as sftpService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/sftpservice";
import {
  buildTerminalSocketUrl,
  bytesToBase64,
  entryToRemoteFile,
  pickSSHConnectArgs,
  statToSftpStatResult,
  terminalSocketSubprotocols,
} from "./terminalRoute";
import type {
  WailsRouteBootstrap,
  WailsSftpEntry,
  WailsSftpFileInfo,
} from "./terminalRoute";
import { setActiveRuntimeClient } from "../runtimeClient";
import type { RuntimeClient } from "../runtimeClient";
import type { RemoteFile } from "../../../domain/models/workspace";

export function isWailsRuntime(): boolean {
  return typeof window !== "undefined" && "_wails" in window;
}

/**
 * Builds a port whose implemented methods are backed by generated bindings
 * and whose remaining methods reject with an explicit migration error. The
 * rejection is the honest state during the migration: the Electron bridge
 * still owns those capabilities.
 */
function portWith<T extends object>(portName: string, implemented: object): T {
  return new Proxy(implemented, {
    get(target, property, receiver) {
      if (property in target) return Reflect.get(target, property, receiver);
      if (property === "__wails__") return undefined;
      throw new Error(
        `Netcatty.${portName}.${String(property)} is not migrated to the Wails runtime yet`,
      );
    },
  }) as T;
}

// Fail-closed transition bridge: property access yields a function that
// rejects on call, so netcattyBridge.get() keeps returning an object (facade
// semantics unchanged) while every call honestly reports the missing owner.
const failClosedTransitionBridge = new Proxy({}, {
  get(_target, property) {
    return () => {
      throw new Error(
        `Netcatty bridge method ${String(property)} is not available under the Wails runtime yet`,
      );
    };
  },
});

export function createWailsRuntimeClient(): RuntimeClient {
  const unimplemented = <T extends object>(portName: string): T => portWith<T>(portName, {});
  return {
    app: portWith("app", {
      // The Wails skeleton owns health/version/window-role only. The
      // Electron-owned app methods stay unimplemented until their domains
      // migrate (P4+).
      quitApp: () => {
        throw new Error("quitApp is not migrated to the Wails runtime yet");
      },
    }),
    agent: unimplemented("agent"),
    files: unimplemented("files"),
    script: unimplemented("script"),
    terminal: portWith("terminal", {
      // SSH sessions route to the Go TerminalService (Slice A). Data flows
      // through the loopback data plane WebSocket (see goTerminalSurface),
      // not the Electron onSessionData event stream.
      startSSHSession: (options: Parameters<NetcattyBridge["startSSHSession"]>[0]) => {
        const args = pickSSHConnectArgs(options);
        return terminalService.Connect(
          args.hostname,
          args.port,
          args.username,
          args.password,
          args.cols,
          args.rows,
        ) as unknown as Promise<string>;
      },
      writeToSession: (sessionID: string, data: string) =>
        terminalService.Write(sessionID, bytesToBase64(new TextEncoder().encode(data))) as unknown as void,
      resizeSession: (sessionID: string, cols: number, rows: number) =>
        terminalService.Resize(sessionID, cols, rows) as unknown as void,
      interruptSession: (sessionID: string) =>
        terminalService.Signal(sessionID, "INT") as unknown as void,
      closeSession: (sessionID: string) =>
        terminalService.Close(sessionID) as unknown as Promise<void>,
    }),
    sftp: portWith("sftp", {
      // SFTP browsing routes to the Go SFTPService over the shared SSH pool
      // (Slice B). Encoding args are accepted but only UTF-8 is supported.
      openSftp: (options: Parameters<NetcattyBridge["openSftp"]>[0]) => {
        const args = pickSSHConnectArgs(options);
        return sftpService.Open(args.hostname, args.port, args.username, args.password) as unknown as Promise<string>;
      },
      listSftp: async (sftpID: string, path: string): Promise<RemoteFile[]> => {
        const entries = (await sftpService.List(sftpID, path)) as unknown as WailsSftpEntry[];
        return entries.map(entryToRemoteFile);
      },
      mkdirSftp: (sftpID: string, path: string) =>
        sftpService.Mkdir(sftpID, path) as unknown as Promise<void>,
      deleteSftp: (sftpID: string, path: string) =>
        sftpService.Remove(sftpID, path) as unknown as Promise<void>,
      renameSftp: (sftpID: string, oldPath: string, newPath: string) =>
        sftpService.Rename(sftpID, oldPath, newPath) as unknown as Promise<void>,
      statSftp: async (sftpID: string, path: string) => {
        const stat = (await sftpService.Stat(sftpID, path)) as unknown as WailsSftpFileInfo;
        return statToSftpStatResult(stat);
      },
      closeSftp: (sftpID: string) =>
        sftpService.Close(sftpID) as unknown as Promise<void>,
    }),
    sync: unimplemented("sync"),
    system: unimplemented("system"),
    plugin: unimplemented("plugin"),
    transitionBridge: failClosedTransitionBridge as NetcattyBridge,
  };
}

/**
 * Wails-native terminal surface for the renderer: the Electron ports cannot
 * express the loopback data plane, so the Wails-terminal renderer consumes
 * this typed surface directly (WS URLs + route bootstrap + streaming SFTP).
 */
export function goTerminalSurface() {
  const listenAddr = () => terminalService.ListenAddr();
  return {
    listenAddr,
    bootstrap: async (sessionID: string) => {
      const bootstrap = (await terminalService.Bootstrap(sessionID)) as unknown as WailsRouteBootstrap;
      const address = await listenAddr();
      return {
        ...bootstrap,
        dataSocketUrl: buildTerminalSocketUrl(address, bootstrap.SessionID, bootstrap.Generation, "data"),
        urgentSocketUrl: buildTerminalSocketUrl(address, bootstrap.SessionID, bootstrap.Generation, "urgent"),
        dataSubprotocols: terminalSocketSubprotocols(bootstrap.DataToken),
        urgentSubprotocols: terminalSocketSubprotocols(bootstrap.UrgentToken),
      };
    },
    write: (sessionID: string, bytes: Uint8Array) => terminalService.Write(sessionID, bytesToBase64(bytes)),
    signal: (sessionID: string, signal: string) => terminalService.Signal(sessionID, signal),
    close: (sessionID: string) => terminalService.Close(sessionID),
    sftp: {
      download: (sftpID: string, remotePath: string, localPath: string) =>
        sftpService.Download(sftpID, remotePath, localPath),
      upload: (sftpID: string, localPath: string, remotePath: string) =>
        sftpService.Upload(sftpID, localPath, remotePath),
    },
  };
}

export { netcattyService };

export function installWailsRuntimeClient(): boolean {
  if (!isWailsRuntime()) return false;
  setActiveRuntimeClient(createWailsRuntimeClient());
  return true;
}
