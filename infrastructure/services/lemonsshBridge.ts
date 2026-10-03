import { getActiveRuntimeClient } from "../runtime/runtimeClient";

export class BridgeUnavailableError extends Error {
  constructor(message = "LemonSSH bridge unavailable") {
    super(message);
    this.name = "BridgeUnavailableError";
  }
}

// Compatibility facade for callers that still consume the aggregate bridge.
// The active client is always installed by the Wails bootstrap.

export const lemonsshBridge = {
  get(): LemonSSHBridge | undefined {
    const active = getActiveRuntimeClient()?.transitionBridge;
    if (active) return active;
    // The Wails adapter mirrors its aggregate bridge on window.lemonssh for
    // compatibility with renderer modules and isolated unit-test harnesses.
    return typeof window === "undefined" ? undefined : window.lemonssh;
  },

  require(): LemonSSHBridge {
    const bridge = this.get();
    if (!bridge) throw new BridgeUnavailableError();
    return bridge;
  },
};
