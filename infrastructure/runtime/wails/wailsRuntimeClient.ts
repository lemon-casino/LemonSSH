// Wails RuntimeClient adapter (P1-02). This module is the only frontend file
// allowed to import the Wails runtime and the generated bindings (enforced by
// ESLint). Ports without a Go owner reject every call fail-closed instead of
// pretending parity; they are implemented domain by domain from P2 onward.

import * as netcattyService from "./bindings/github.com/binaricat/netcatty/cmd/netcatty/netcattyservice";
import { setActiveRuntimeClient } from "../runtimeClient";
import type { RuntimeClient } from "../runtimeClient";

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
    terminal: unimplemented("terminal"),
    sftp: unimplemented("sftp"),
    sync: unimplemented("sync"),
    system: unimplemented("system"),
    plugin: unimplemented("plugin"),
    transitionBridge: failClosedTransitionBridge as NetcattyBridge,
  };
}

export { netcattyService };

export function installWailsRuntimeClient(): boolean {
  if (!isWailsRuntime()) return false;
  setActiveRuntimeClient(createWailsRuntimeClient());
  return true;
}
