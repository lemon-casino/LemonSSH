import { getActiveRuntimeClient } from "../runtime/runtimeClient";

export class BridgeUnavailableError extends Error {
  constructor(message = "Netcatty bridge unavailable") {
    super(message);
    this.name = "BridgeUnavailableError";
  }
}

// Compatibility facade for callers that still consume the aggregate bridge.
// The active client is always installed by the Wails bootstrap.

export const netcattyBridge = {
  get(): NetcattyBridge | undefined {
    const active = getActiveRuntimeClient()?.transitionBridge;
    if (active) return active;
    // The Wails adapter mirrors its aggregate bridge on window.netcatty for
    // compatibility with renderer modules and isolated unit-test harnesses.
    return typeof window === "undefined" ? undefined : window.netcatty;
  },

  require(): NetcattyBridge {
    const bridge = this.get();
    if (!bridge) throw new BridgeUnavailableError();
    return bridge;
  },
};
