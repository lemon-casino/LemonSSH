import type { RuntimePorts } from "./generated/runtimePorts";

// Shell-neutral RuntimeClient (P1-01). Application and UI code consumes the
// domain ports; Electron and Wails adapters implement the same structural
// contract. Only the Electron adapter may touch window.netcatty and only the
// Wails adapter may import generated Wails bindings.

export interface RuntimeClient extends RuntimePorts {
  /**
   * Transition-period unwrapped bridge so the existing netcattyBridge facade
   * keeps its exact semantics while consumers migrate port by port. New code
   * must use the domain ports. Removed once the last consumer migrated and
   * the Electron path retires.
   */
  readonly transitionBridge: NetcattyBridge;
}

let activeClient: RuntimeClient | undefined;

export function setActiveRuntimeClient(client: RuntimeClient | undefined): void {
  activeClient = client;
}

export function getActiveRuntimeClient(): RuntimeClient | undefined {
  return activeClient;
}
