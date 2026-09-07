import { setActiveRuntimeClient } from "../runtimeClient";
import type { RuntimeClient } from "../runtimeClient";

// Electron RuntimeClient adapter (P1-01). This module is the only place in
// the frontend allowed to access window.netcatty (enforced by ESLint). The
// Wails adapter (P1-02) implements the same RuntimeClient contract from
// generated bindings.

export function getElectronBridge(): NetcattyBridge | undefined {
  return typeof window === "undefined" ? undefined : window.netcatty;
}

export function createElectronRuntimeClient(bridge: NetcattyBridge): RuntimeClient {
  return {
    app: bridge,
    agent: bridge,
    files: bridge,
    script: bridge,
    terminal: bridge,
    sftp: bridge,
    sync: bridge,
    system: bridge,
    plugin: bridge,
    transitionBridge: bridge,
  };
}

/** Registers the Electron adapter when the bridge exists; returns success. */
export function installElectronRuntimeClient(): boolean {
  const bridge = getElectronBridge();
  if (!bridge) return false;
  setActiveRuntimeClient(createElectronRuntimeClient(bridge));
  return true;
}
