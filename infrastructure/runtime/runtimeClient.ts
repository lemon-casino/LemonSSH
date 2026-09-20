import type { RuntimePorts } from "./generated/runtimePorts";

// Application and UI code consume domain ports through the Wails adapter.

export interface RuntimeClient extends RuntimePorts {
  /**
   * Aggregate bridge retained for callers that have not moved to a narrower
   * domain port yet.
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
